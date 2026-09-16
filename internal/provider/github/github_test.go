package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestValidateCredentials_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("X-OAuth-Scopes", "repo, workflow, delete_repo")
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "user.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	meta, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "ghp_test"})
	if err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}
	if meta.AccountID != "testuser" {
		t.Errorf("AccountID = %q, want %q", meta.AccountID, "testuser")
	}
	raw := meta.Raw
	if raw == nil {
		t.Fatal("Raw is nil")
	}
	if raw["login"] != "testuser" {
		t.Errorf("login = %v, want testuser", raw["login"])
	}
	scopes, ok := raw["scopes"].([]string)
	if !ok {
		t.Fatal("scopes not []string")
	}
	if len(scopes) != 3 || scopes[0] != "repo" || scopes[1] != "workflow" || scopes[2] != "delete_repo" {
		t.Errorf("scopes = %v, want [repo workflow delete_repo]", scopes)
	}
	if _, exists := raw["missing_scopes"]; exists {
		t.Error("missing_scopes should not exist when all scopes present")
	}
}

func TestValidateCredentials_MissingScopes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo")
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "user.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	meta, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "ghp_test"})
	if err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}
	missing, ok := meta.Raw["missing_scopes"].([]string)
	if !ok {
		t.Fatal("missing_scopes not []string")
	}
	expected := []string{"workflow", "delete_repo"}
	if len(missing) != len(expected) {
		t.Errorf("missing_scopes = %v, want %v", missing, expected)
	}
	for i, v := range expected {
		if missing[i] != v {
			t.Errorf("missing_scopes[%d] = %q, want %q", i, missing[i], v)
		}
	}
}

func TestValidateCredentials_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(readFixture(t, "unauthorized.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "ghp_bad"})
	if err == nil {
		t.Fatal("expected error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUnauthorized {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUnauthorized)
	}
}

func TestValidateCredentials_RateLimit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		w.Write(readFixture(t, "rate_limit.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "ghp_test"})
	if err == nil {
		t.Fatal("expected error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindRateLimit {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindRateLimit)
	}
}

func TestListExternalResources_Repos(t *testing.T) {
	page1Called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("visibility") != "all" {
			t.Errorf("visibility = %q, want all", r.URL.Query().Get("visibility"))
		}
		if r.URL.Query().Get("affiliation") != "owner" {
			t.Errorf("affiliation = %q, want owner", r.URL.Query().Get("affiliation"))
		}

		page := r.URL.Query().Get("page")
		if page == "" || page == "1" {
			page1Called = true
			w.Header().Set("Link", `</user/repos?page=2>; rel="next", </user/repos?page=2>; rel="last"`)
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "repos_page1.json"))
		} else if page == "2" {
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "repos_page2.json"))
		} else {
			t.Errorf("unexpected page: %s", page)
		}
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	resources, err := p.ListExternalResources(context.Background(), &domain.ProviderAccount{TokenEncrypted: "ghp_test"}, domain.ResourceKindRepo)
	if err != nil {
		t.Fatalf("ListExternalResources failed: %v", err)
	}
	if !page1Called {
		t.Error("page 1 was never called")
	}
	if len(resources) != 3 {
		t.Errorf("got %d resources, want 3", len(resources))
	}
	if resources[0].ExternalID != "testuser/my-repo" {
		t.Errorf("resources[0].ExternalID = %q, want testuser/my-repo", resources[0].ExternalID)
	}
	if resources[0].DisplayName != "my-repo" {
		t.Errorf("resources[0].DisplayName = %q, want my-repo", resources[0].DisplayName)
	}
	if resources[0].Meta["private"] != false {
		t.Errorf("resources[0].Meta[private] = %v, want false", resources[0].Meta["private"])
	}
	if resources[0].Meta["default_branch"] != "main" {
		t.Errorf("resources[0].Meta[default_branch] = %q, want main", resources[0].Meta["default_branch"])
	}
	if resources[0].Meta["html_url"] != "https://github.com/testuser/my-repo" {
		t.Errorf("resources[0].Meta[html_url] = %q, want https://github.com/testuser/my-repo", resources[0].Meta["html_url"])
	}
	if resources[1].Meta["private"] != true {
		t.Errorf("resources[1].Meta[private] = %v, want true", resources[1].Meta["private"])
	}
}

func TestListExternalResources_UnsupportedKind(t *testing.T) {
	p := NewProvider("")
	resources, err := p.ListExternalResources(context.Background(), &domain.ProviderAccount{TokenEncrypted: "test"}, domain.ResourceKindStaticSite)
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
	if resources != nil {
		t.Errorf("resources = %v, want nil", resources)
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUnsupported {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUnsupported)
	}
}

func TestGetResource_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/testuser/my-repo" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "repo.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	res, err := p.GetResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "ghp_test"}, "testuser/my-repo")
	if err != nil {
		t.Fatalf("GetResource failed: %v", err)
	}
	if res.ExternalID != "testuser/my-repo" {
		t.Errorf("ExternalID = %q, want testuser/my-repo", res.ExternalID)
	}
	if res.DisplayName != "my-repo" {
		t.Errorf("DisplayName = %q, want my-repo", res.DisplayName)
	}
}

func TestGetResource_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.GetResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "ghp_test"}, "testuser/nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindNotFound {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindNotFound)
	}
}

func TestGetResource_InvalidExternalID(t *testing.T) {
	p := NewProvider("")
	_, err := p.GetResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "test"}, "invalid")
	if err == nil {
		t.Fatal("expected error for invalid external_id")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUpstream {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUpstream)
	}
}

func TestCreateResource_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write(readFixture(t, "created_repo.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	spec := domain.ResourceSpec{
		Name:    "new-repo",
		Private: false,
		Kind:    domain.ResourceKindRepo,
	}
	res, err := p.CreateResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "ghp_test"}, spec)
	if err != nil {
		t.Fatalf("CreateResource failed: %v", err)
	}
	if res.ExternalID != "testuser/new-repo" {
		t.Errorf("ExternalID = %q, want testuser/new-repo", res.ExternalID)
	}
	if res.DisplayName != "new-repo" {
		t.Errorf("DisplayName = %q, want new-repo", res.DisplayName)
	}
}

func TestDeleteResource_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/testuser/my-repo" || r.Method != http.MethodDelete {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	err := p.DeleteResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "ghp_test"}, "testuser/my-repo")
	if err != nil {
		t.Fatalf("DeleteResource failed: %v", err)
	}
}

func TestDeleteResource_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	err := p.DeleteResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "ghp_test"}, "testuser/nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindNotFound {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindNotFound)
	}
}

func TestDeleteResource_InvalidExternalID(t *testing.T) {
	p := NewProvider("")
	err := p.DeleteResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "test"}, "invalid")
	if err == nil {
		t.Fatal("expected error for invalid external_id")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUpstream {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUpstream)
	}
}