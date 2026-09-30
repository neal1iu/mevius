package catalog_test

import (
	"context"
	"encoding/json"
	"mevius/internal/catalog"
	fxt "mevius/internal/catalogtest"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOAuthExchangeConsumptionAndSecretErasure(t *testing.T) {
	f := fxt.New(t)
	instance := f.Instance(t, "public")
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if e := r.ParseForm(); e != nil {
			t.Error(e)
		}
		if r.Form.Get("code") != "valid" || r.Form.Get("code_verifier") == "" {
			t.Error("missing PKCE/code")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"oauth-access-secret","refresh_token":"ignore-refresh-secret","webhook_secret":"ignore-product-secret"}`))
	}))
	defer tokenServer.Close()
	oauth := catalog.NewOAuthService(f.Service, "http://localhost:3000", nil)
	if e := oauth.Save(ctx, "fixture", catalog.OAuthConfiguration{ClientID: "client", ClientSecret: "oauth-client-secret", AuthorizationURL: tokenServer.URL + "/authorize", TokenURL: tokenServer.URL, Scopes: []string{"read"}, PKCE: true, RedirectBaseURL: "http://localhost:3000"}); e != nil {
		t.Fatal(e)
	}
	infos, e := oauth.Providers(ctx)
	if e != nil || strings.Contains(string(fxt.Raw(infos)), "oauth-client-secret") {
		t.Fatal("client secret leaked")
	}
	start, e := oauth.Start(ctx, instance.ID)
	if e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(start["authorization_url"].(string))
	if e != nil {
		t.Fatal(e)
	}
	if u.Query().Get("code_challenge") == "" {
		t.Fatal("missing PKCE")
	}
	sessionID := start["session_id"].(string)
	if _, e = oauth.Callback(ctx, "other", u.Query().Get("state"), "valid"); e == nil {
		t.Fatal("provider confusion")
	}
	if _, e = oauth.Callback(ctx, "fixture", u.Query().Get("state"), "valid"); e != nil {
		t.Fatal(e)
	}
	session, e := oauth.Session(ctx, sessionID)
	if e != nil || session.Status != "authorized" {
		t.Fatal(session, e)
	}
	if strings.Contains(string(fxt.Raw(session)), "secret") {
		t.Fatal("session exposes credential")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := oauth.Complete(context.Background(), sessionID, "OAuth"); e == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("session consumed %d times", success)
	}
	var credential, verifier, status string
	if e = f.Service.DB.QueryRow(`SELECT encrypted_credential,encrypted_code_verifier,status FROM oauth_authorization_session WHERE id=?`, sessionID).Scan(&credential, &verifier, &status); e != nil {
		t.Fatal(e)
	}
	if credential != "" || verifier != "" || status != "consumed" {
		t.Fatal("consumed session retained authorization secrets")
	}
	connections, e := f.Service.Connections(ctx)
	if e != nil || len(connections) != 1 || connections[0].AuthScheme != "oauth" {
		t.Fatal(connections, e)
	}
	var raw string
	f.Service.DB.QueryRow(`SELECT encrypted_credential FROM provider_connection`).Scan(&raw)
	if strings.Contains(raw, "oauth-access-secret") {
		t.Fatal("credential not encrypted")
	}
	var payload any
	if json.Unmarshal([]byte(raw), &payload) == nil {
		t.Fatal("ciphertext unexpectedly JSON")
	}
	start, e = oauth.Start(ctx, instance.ID)
	if e != nil {
		t.Fatal(e)
	}
	expired := start["session_id"].(string)
	f.Service.DB.Exec(`UPDATE oauth_authorization_session SET expires_at=? WHERE id=?`, time.Now().Add(-time.Minute).UnixMilli(), expired)
	session, e = oauth.Session(ctx, expired)
	if e != nil || session.Status != "failed" {
		t.Fatal("expired session valid")
	}
	f.Service.DB.QueryRow(`SELECT encrypted_code_verifier FROM oauth_authorization_session WHERE id=?`, expired).Scan(&verifier)
	if verifier != "" {
		t.Fatal("expired verifier retained")
	}
}
