package vercel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeReturnsPersonalAndTeamScopes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/user":
			_, _ = w.Write([]byte(`{"user":{"id":"user-1","username":"ada"}}`))
		case "/v2/teams":
			_, _ = w.Write([]byte(`{"teams":[{"id":"team-1","name":"Platform"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	result, err := NewProvider().Probe(context.Background(), server.URL, []byte("token"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Scopes) != 2 || result.Scopes[0].Type != "personal" || result.Scopes[1].Type != "team" {
		t.Fatalf("unexpected scopes: %#v", result.Scopes)
	}
}
