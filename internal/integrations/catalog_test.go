package integrations_test

import (
	"context"
	"fmt"
	"mevius/internal/catalog"
	fxt "mevius/internal/catalogtest"
	"mevius/internal/integrations"
	"mevius/internal/provider/cloudflare"
	"mevius/internal/provider/github"
	"mevius/internal/provider/vercel"
	"mevius/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestProductionContractsAndStableGitHubUID(t *testing.T) {
	db, e := store.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := catalog.NewService(db, catalog.NewRegistry(), [32]byte{1})
	integrations.Register(s, github.NewProvider())
	integrations.Register(s, cloudflare.NewProvider())
	integrations.Register(s, vercel.NewProvider())
	if e = s.Registry.ValidateReady(); e != nil {
		t.Fatal(e)
	}
	var renamed atomic.Bool
	var missingUID atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-OAuth-Scopes", "repo, workflow, delete_repo, read:org")
		switch r.URL.Path {
		case "/user":
			w.Write([]byte(`{"id":1,"login":"me"}`))
		case "/user/orgs":
			w.Write([]byte(`[{"id":2,"login":"acme"}]`))
		case "/repos/acme/app", "/repositories/55":
			if missingUID.Load() {
				w.Write([]byte(`{"name":"app","full_name":"acme/app"}`))
				return
			}
			name := "app"
			if renamed.Load() {
				name = "renamed"
			}
			fmt.Fprintf(w, `{"id":55,"name":%q,"full_name":%q,"html_url":"https://github.com/acme/app","private":false,"default_branch":"main"}`, name, "acme/"+name)
		default:
			t.Errorf("unexpected upstream %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	ctx := context.Background()
	instance, e := s.CreateInstance(ctx, "github", "fixture-cloud", fxt.Doc(map[string]any{"url": upstream.URL}))
	if e != nil {
		t.Fatal(e)
	}
	c1, e := s.CreateConnection(ctx, instance.ID, "one", "token", []byte("one"))
	if e != nil {
		t.Fatal(e)
	}
	c2, e := s.CreateConnection(ctx, instance.ID, "two", "token", []byte("two"))
	if e != nil {
		t.Fatal(e)
	}
	b1, e := s.BindScope(ctx, c1.ID, catalog.ScopeCandidate{Type: "user", IdentityParts: []string{"untrusted"}, Locator: fxt.Raw(map[string]any{"id": "me"})})
	if e != nil {
		t.Fatal(e)
	}
	b2, e := s.BindScope(ctx, c2.ID, catalog.ScopeCandidate{Type: "org", IdentityParts: []string{"untrusted"}, Locator: fxt.Raw(map[string]any{"id": "acme"})})
	if e != nil {
		t.Fatal(e)
	}
	r1, e := s.Import(ctx, b1.ID, "github.repositories", fxt.Doc(map[string]any{"external_id": "acme/app"}))
	if e != nil {
		t.Fatal(e)
	}
	r2, e := s.Import(ctx, b2.ID, "github.repositories", fxt.Doc(map[string]any{"external_id": "acme/app"}))
	if e != nil {
		t.Fatal(e)
	}
	if r1.ID != r2.ID || r1.IdentityKey != `["55"]` {
		t.Fatal("GitHub identity is credential- or locator-dependent")
	}
	accesses, e := s.Accesses(ctx, r1.ID)
	if e != nil || len(accesses) != 2 {
		t.Fatal(accesses, e)
	}
	renamed.Store(true)
	r1, e = s.Refresh(ctx, r1.ID, accesses[0].ID)
	if e != nil || r1.Name != "renamed" || r1.ID != r2.ID {
		t.Fatal("rename changed directory identity", r1, e)
	}
	missingUID.Store(true)
	if _, e = s.Import(ctx, b1.ID, "github.repositories", fxt.Doc(map[string]any{"external_id": "acme/app"})); e == nil {
		t.Fatal("missing UID was guessed as zero")
	}

}
