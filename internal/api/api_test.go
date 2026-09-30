package api_test

import (
	"bytes"
	"io"
	"log/slog"
	"mevius/internal/api"
	"mevius/internal/catalog"
	fxt "mevius/internal/catalogtest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogAPIContractAndAuthBoundary(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	oauth := catalog.NewOAuthService(f.Service, "http://localhost", nil)
	handler := api.NewCatalogRouter("local-secret", slog.New(slog.NewTextHandler(io.Discard, nil)), f.Service, oauth)
	request := func(method, path, payload string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(payload))
		if auth {
			r.Header.Set("Authorization", "Bearer local-secret")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/v1/catalog", "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("GET", "/api/v1/catalog", "", true); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("fixture.database")) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("POST", "/api/v1/connections", `{"provider_instance_id":"`+i.ID+`","label":"bad","credential":"bad"}`, true); w.Code == 401 || bytes.Contains(w.Body.Bytes(), []byte("upstream-secret")) {
		t.Fatal("upstream auth failure invalidated local session or leaked error", w.Code, w.Body.String())
	}
	if w := request("POST", "/api/v1/projects", `{"name":"ok","unknown":"secret"}`, true); w.Code != 400 {
		t.Fatal("unknown request field accepted")
	}
	if w := request("POST", "/api/v1/projects", `{"name":"ok"}{"name":"trailing"}`, true); w.Code != 400 {
		t.Fatal("trailing JSON accepted")
	}
	if w := request("POST", "/api/v1/projects", `{"name":"ok","description":"purpose"}`, true); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("T")) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("GET", "/healthz", "", false); w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
}
