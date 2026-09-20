package cloudflare

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeReturnsEveryAccessibleAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/user/tokens/verify":
			_, _ = w.Write([]byte(`{"success":true,"result":{"status":"active"}}`))
		case "/accounts":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"a","name":"Alpha"},{"id":"b","name":"Beta"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	result, err := NewProvider().Probe(context.Background(), server.URL, []byte("token"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Scopes) != 2 || result.Scopes[1].ID != "b" {
		t.Fatalf("unexpected scopes: %#v", result.Scopes)
	}
}
