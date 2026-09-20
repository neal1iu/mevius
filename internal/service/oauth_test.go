package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"mevius/internal/config"
	"mevius/internal/crypto"
	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

type oauthConfigurationTestProvider struct{}

func (oauthConfigurationTestProvider) ID() domain.ProviderID { return "github" }
func (oauthConfigurationTestProvider) Descriptor() domain.ProviderDescriptor {
	return domain.ProviderDescriptor{ID: "github", DisplayName: "GitHub"}
}
func (oauthConfigurationTestProvider) Products() []provider.ProductDriver { return nil }
func (oauthConfigurationTestProvider) Probe(context.Context, string, []byte) (*domain.ProbeResult, error) {
	return &domain.ProbeResult{}, nil
}
func (oauthConfigurationTestProvider) ValidateScope(context.Context, string, []byte, domain.ProviderScope) (*domain.ProbeResult, error) {
	return &domain.ProbeResult{}, nil
}

func TestOAuthAuthorizationCreatesScopedConnectionWithoutExposingToken(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("code") != "authorization-code" {
			t.Errorf("code=%q", r.Form.Get("code"))
		}
		if r.Form.Get("code_verifier") == "" {
			t.Error("PKCE verifier missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"oauth-secret","refresh_token":"refresh-secret","token_type":"bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "oauth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	q := store.New(db)
	product := &testProduct{}
	registry := provider.NewRegistry()
	registry.Register(&testProvider{product: product})
	var key [32]byte
	copy(key[:], []byte("0123456789abcdef0123456789abcdef"))
	connections := NewConnectionService(q, key, registry)
	oauth := NewOAuthService(q, key, registry, connections, "https://mevius.example", map[string]config.OAuthClient{
		"test": {ClientID: "client", ClientSecret: "secret", AuthorizationURL: "https://provider.example/authorize", TokenURL: tokenServer.URL, Scopes: []string{"resource:write"}, PKCE: true},
	})

	start, err := oauth.Start(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, _ := url.Parse(start.AuthorizationURL)
	if authorizationURL.Query().Get("code_challenge") == "" || authorizationURL.Query().Get("state") == "" {
		t.Fatalf("missing OAuth protections: %s", start.AuthorizationURL)
	}
	sessionID, err := oauth.Callback(context.Background(), "test", authorizationURL.Query().Get("state"), "authorization-code", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := oauth.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(session)
	if string(serialized) == "" || containsSecret(string(serialized), "oauth-secret") || containsSecret(string(serialized), "refresh-secret") {
		t.Fatalf("session exposed credential: %s", serialized)
	}
	connection, err := oauth.Complete(context.Background(), CompleteOAuthInput{SessionID: sessionID, Label: "OAuth test", Scope: session.Scopes[0]})
	if err != nil {
		t.Fatal(err)
	}
	if connection.AuthMethod != domain.AuthMethodOAuth {
		t.Fatalf("auth method=%s", connection.AuthMethod)
	}
	resolved, err := provider.NewCredentialStore(key, q).Resolve(context.Background(), connection.ID)
	if err != nil || string(resolved) != "oauth-secret" {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}
	row, err := q.GetProviderConnection(context.Background(), connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := crypto.Decrypt(key, row.EncryptedCredential)
	if err != nil || !containsSecret(decrypted, "refresh-secret") {
		t.Fatalf("refresh token was not retained securely: %v", err)
	}
	if _, err = oauth.Complete(context.Background(), CompleteOAuthInput{SessionID: sessionID, Label: "Again", Scope: session.Scopes[0]}); err == nil {
		t.Fatal("expected one-time OAuth session conflict")
	}
}

func TestOAuthClientConfigurationCanBeSavedFromUI(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "oauth-config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	q := store.New(db)
	registry := provider.NewRegistry()
	registry.Register(oauthConfigurationTestProvider{})
	var key [32]byte
	copy(key[:], []byte("0123456789abcdef0123456789abcdef"))
	oauth := NewOAuthService(q, key, registry, NewConnectionService(q, key, registry), "", nil)

	providers, err := oauth.Providers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].Available || providers[0].AuthorizationURL != "https://github.com/login/oauth/authorize" {
		t.Fatalf("unexpected unconfigured provider template: %+v", providers)
	}

	info, err := oauth.SaveConfiguration(context.Background(), "github", OAuthClientConfigurationInput{ClientID: "client-id", ClientSecret: "client-secret", RedirectBaseURL: "https://mevius.example", Scopes: []string{"repo"}})
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.Source != "database" || info.CallbackURL != "https://mevius.example/api/v1/connections/oauth/callback/github" || !info.PKCE {
		t.Fatalf("unexpected saved provider: %+v", info)
	}
	row, err := q.GetOAuthClientConfiguration(context.Background(), "github")
	if err != nil {
		t.Fatal(err)
	}
	if row.EncryptedClientSecret == "client-secret" {
		t.Fatal("OAuth client secret was stored in plaintext")
	}
	secret, err := crypto.Decrypt(key, row.EncryptedClientSecret)
	if err != nil || secret != "client-secret" {
		t.Fatalf("stored secret=%q err=%v", secret, err)
	}

	// Editing non-secret fields may leave the secret blank and retain the encrypted value.
	if _, err = oauth.SaveConfiguration(context.Background(), "github", OAuthClientConfigurationInput{ClientID: "client-id-2", RedirectBaseURL: "https://new.example", Scopes: []string{"repo", "workflow"}}); err != nil {
		t.Fatal(err)
	}
	updated, _ := q.GetOAuthClientConfiguration(context.Background(), "github")
	retained, _ := crypto.Decrypt(key, updated.EncryptedClientSecret)
	if retained != "client-secret" {
		t.Fatalf("secret was not retained: %q", retained)
	}
	start, err := oauth.Start(context.Background(), "github", "")
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, _ := url.Parse(start.AuthorizationURL)
	if authorizationURL.Query().Get("client_id") != "client-id-2" || authorizationURL.Query().Get("redirect_uri") != "https://new.example/api/v1/connections/oauth/callback/github" {
		t.Fatalf("unexpected authorization URL: %s", start.AuthorizationURL)
	}
}

func containsSecret(value, secret string) bool {
	return len(secret) > 0 && len(value) >= len(secret) && func() bool {
		for i := 0; i+len(secret) <= len(value); i++ {
			if value[i:i+len(secret)] == secret {
				return true
			}
		}
		return false
	}()
}
