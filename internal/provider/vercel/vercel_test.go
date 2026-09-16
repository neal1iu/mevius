package vercel

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
		if r.URL.Path != "/v2/user" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer vct_test" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "user.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	meta, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"})
	if err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}
	if meta.AccountID != "u_abc123" {
		t.Errorf("AccountID = %q, want u_abc123", meta.AccountID)
	}
	if meta.Raw["username"] != "testuser" {
		t.Errorf("username = %v, want testuser", meta.Raw["username"])
	}
}

func TestValidateCredentials_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(readFixture(t, "unauthorized.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_bad"})
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
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":"rate_limited","message":"Rate limit exceeded"}}`))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"})
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

func TestListExternalResources_Projects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v9/projects" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "projects.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	resources, err := p.ListExternalResources(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, domain.ResourceKindStaticSite)
	if err != nil {
		t.Fatalf("ListExternalResources failed: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(resources))
	}
	if resources[0].ExternalID != "prj_abc123" {
		t.Errorf("resources[0].ExternalID = %q, want prj_abc123", resources[0].ExternalID)
	}
	if resources[0].DisplayName != "my-site" {
		t.Errorf("resources[0].DisplayName = %q, want my-site", resources[0].DisplayName)
	}
	if resources[0].Meta["framework"] != "nextjs" {
		t.Errorf("resources[0].Meta[framework] = %v, want nextjs", resources[0].Meta["framework"])
	}
	if _, ok := resources[1].Meta["framework"]; ok {
		t.Error("resources[1].Meta[framework] should be absent for null framework")
	}
}

func TestListExternalResources_Domains(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/domains" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "domains.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	resources, err := p.ListExternalResources(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, domain.ResourceKindDNSDomain)
	if err != nil {
		t.Fatalf("ListExternalResources failed: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(resources))
	}
	if resources[0].ExternalID != "example.com" {
		t.Errorf("resources[0].ExternalID = %q, want example.com", resources[0].ExternalID)
	}
	if resources[0].DisplayName != "example.com" {
		t.Errorf("resources[0].DisplayName = %q, want example.com", resources[0].DisplayName)
	}
	if resources[0].Meta["verified"] != true {
		t.Errorf("resources[0].Meta[verified] = %v, want true", resources[0].Meta["verified"])
	}
	if resources[1].Meta["verified"] != false {
		t.Errorf("resources[1].Meta[verified] = %v, want false", resources[1].Meta["verified"])
	}
}

func TestListExternalResources_UnsupportedKind(t *testing.T) {
	p := NewProvider("")
	_, err := p.ListExternalResources(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, domain.ResourceKindCompute)
	if err == nil {
		t.Fatal("expected error for unsupported kind")
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
		if r.URL.Path != "/v9/projects/prj_abc123" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "project.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	res, err := p.GetResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, "prj_abc123")
	if err != nil {
		t.Fatalf("GetResource failed: %v", err)
	}
	if res.ExternalID != "prj_abc123" {
		t.Errorf("ExternalID = %q, want prj_abc123", res.ExternalID)
	}
	if res.DisplayName != "my-site" {
		t.Errorf("DisplayName = %q, want my-site", res.DisplayName)
	}
	if res.Meta["framework"] != "nextjs" {
		t.Errorf("Meta[framework] = %v, want nextjs", res.Meta["framework"])
	}
}

func TestGetResource_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.GetResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, "prj_nonexistent")
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

func TestCreateResource_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v10/projects" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusCreated)
		w.Write(readFixture(t, "created_project.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	spec := domain.ResourceSpec{
		Name: "my-new-site",
		Kind: domain.ResourceKindStaticSite,
		Extra: map[string]any{
			"framework": "nextjs",
		},
	}
	res, err := p.CreateResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, spec)
	if err != nil {
		t.Fatalf("CreateResource failed: %v", err)
	}
	if res.ExternalID != "prj_new789" {
		t.Errorf("ExternalID = %q, want prj_new789", res.ExternalID)
	}
	if res.DisplayName != "my-new-site" {
		t.Errorf("DisplayName = %q, want my-new-site", res.DisplayName)
	}
	if res.Meta["framework"] != "nextjs" {
		t.Errorf("Meta[framework] = %v, want nextjs", res.Meta["framework"])
	}
}

func TestDeleteResource_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v9/projects/prj_abc123" || r.Method != http.MethodDelete {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	err := p.DeleteResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, "prj_abc123")
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
	err := p.DeleteResource(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, "prj_nonexistent")
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

func TestTeamIDPropagation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("teamId") != "team_xyz" {
			t.Errorf("teamId query param = %q, want team_xyz", r.URL.Query().Get("teamId"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "user.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	account := &domain.ProviderAccount{
		TokenEncrypted: "vct_test",
		Meta: domain.AccountMeta{
			Raw: map[string]any{
				"team_id": "team_xyz",
			},
		},
	}
	meta, err := p.ValidateCredentials(context.Background(), account)
	if err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}
	if meta.Raw["team_id"] != "team_xyz" {
		t.Errorf("preserved team_id = %v, want team_xyz", meta.Raw["team_id"])
	}
}

func TestTeamIDPropagation_NoTeam(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("teamId") != "" {
			t.Errorf("teamId query param = %q, want empty", r.URL.Query().Get("teamId"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "user.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	account := &domain.ProviderAccount{
		TokenEncrypted: "vct_test",
		Meta:           domain.AccountMeta{},
	}
	_, err := p.ValidateCredentials(context.Background(), account)
	if err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}
}

func TestTeamIDPropagation_OnProjectsList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("teamId") != "team_xyz" {
			t.Errorf("teamId query param = %q, want team_xyz", r.URL.Query().Get("teamId"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "projects.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	account := &domain.ProviderAccount{
		TokenEncrypted: "vct_test",
		Meta: domain.AccountMeta{
			Raw: map[string]any{
				"team_id": "team_xyz",
			},
		},
	}
	_, err := p.ListExternalResources(context.Background(), account, domain.ResourceKindStaticSite)
	if err != nil {
		t.Fatalf("ListExternalResources failed: %v", err)
	}
}

func TestRateLimit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":"rate_limited"}}`))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.ListExternalResources(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, domain.ResourceKindStaticSite)
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
	if pErr.RetryAfter == 0 {
		t.Error("RetryAfter = 0, want > 0")
	}
}