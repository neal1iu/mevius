package vercel

import (
	"context"
	"encoding/json"
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

	p := NewProvider()
	raw, err := p.ValidateCredentials(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"))
	if err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}

	var info map[string]any
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if info["user_id"] != "u_abc123" {
		t.Errorf("user_id = %v, want u_abc123", info["user_id"])
	}
	if info["username"] != "testuser" {
		t.Errorf("username = %v, want testuser", info["username"])
	}
}

func TestValidateCredentials_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(readFixture(t, "unauthorized.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	_, err := p.ValidateCredentials(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_bad"))
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

	p := NewProvider()
	_, err := p.ValidateCredentials(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"))
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

	p := NewProvider()
	resources, err := p.ListExternalResources(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "vercel.projects")
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

	p := NewProvider()
	resources, err := p.ListExternalResources(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "vercel.dns")
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
	p := NewProvider()
	_, err := p.ListExternalResources(context.Background(), &domain.ProviderConnection{}, []byte("vct_test"), "unknown.product")
	if err == nil {
		t.Fatal("expected error for unsupported product")
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
	var callCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			if r.URL.Path != "/v5/domains/prj_abc123" || r.Method != http.MethodGet {
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(http.StatusNotFound)
			w.Write(readFixture(t, "not_found.json"))
			return
		}
		if r.URL.Path != "/v9/projects/prj_abc123" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "project.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	res, err := p.GetResource(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "prj_abc123")
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

	p := NewProvider()
	_, err := p.GetResource(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "prj_nonexistent")
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

	p := NewProvider()
	spec := domain.ResourceSpec{
		Name: "my-new-site",
		Extra: map[string]any{
			"framework": "nextjs",
		},
	}
	res, err := p.CreateResource(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), spec)
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

	p := NewProvider()
	err := p.DeleteResource(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "prj_abc123")
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

	p := NewProvider()
	err := p.DeleteResource(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "prj_nonexistent")
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

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		Config: map[string]any{
			"team_id": "team_xyz",
		},
	}
	raw, err := p.ValidateCredentials(context.Background(), conn, []byte("vct_test"))
	if err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}
	var info map[string]any
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if info["team_id"] != "team_xyz" {
		t.Errorf("preserved team_id = %v, want team_xyz", info["team_id"])
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

	p := NewProvider()
	_, err := p.ValidateCredentials(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"))
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

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		Config: map[string]any{
			"team_id": "team_xyz",
		},
	}
	_, err := p.ListExternalResources(context.Background(), conn, []byte("vct_test"), "vercel.projects")
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

	p := NewProvider()
	_, err := p.ListExternalResources(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "vercel.projects")
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

func TestDNSListRecords_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/domains/example.com/records" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "dns_records_list.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	records, err := p.ListRecords(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "example.com")
	if err != nil {
		t.Fatalf("ListRecords failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].ID != "rec_abc123" {
		t.Errorf("records[0].ID = %q, want rec_abc123", records[0].ID)
	}
	if records[0].Type != "A" {
		t.Errorf("records[0].Type = %q, want A", records[0].Type)
	}
	if records[0].Name != "www" {
		t.Errorf("records[0].Name = %q, want www", records[0].Name)
	}
	if records[0].Content != "192.0.2.1" {
		t.Errorf("records[0].Content = %q, want 192.0.2.1", records[0].Content)
	}
	if records[0].TTL != 60 {
		t.Errorf("records[0].TTL = %d, want 60", records[0].TTL)
	}
	if records[0].ZoneID != "example.com" {
		t.Errorf("records[0].ZoneID = %q, want example.com", records[0].ZoneID)
	}
	if records[1].ID != "rec_def456" {
		t.Errorf("records[1].ID = %q, want rec_def456", records[1].ID)
	}
	if records[1].Type != "CNAME" {
		t.Errorf("records[1].Type = %q, want CNAME", records[1].Type)
	}
}

func TestDNSListRecords_DomainNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	_, err := p.ListRecords(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "nonexistent.com")
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

func TestDNSCreateRecord_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/domains/example.com/records" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "dns_record_created.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	record := domain.DNSRecord{
		Type:    "A",
		Name:    "test",
		Content: "192.0.2.1",
		TTL:     3600,
	}
	created, err := p.CreateRecord(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "example.com", record)
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}
	if created.ID != "rec_new789" {
		t.Errorf("created.ID = %q, want rec_new789", created.ID)
	}
	if created.Type != "A" {
		t.Errorf("created.Type = %q, want A", created.Type)
	}
	if created.Name != "test" {
		t.Errorf("created.Name = %q, want test", created.Name)
	}
	if created.Content != "192.0.2.1" {
		t.Errorf("created.Content = %q, want 192.0.2.1", created.Content)
	}
	if created.TTL != 3600 {
		t.Errorf("created.TTL = %d, want 3600", created.TTL)
	}
	if created.ZoneID != "example.com" {
		t.Errorf("created.ZoneID = %q, want example.com", created.ZoneID)
	}
}

func TestDNSCreateRecord_ZeroTTL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/domains/example.com/records" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "dns_record_created.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	record := domain.DNSRecord{
		Type:    "A",
		Name:    "test",
		Content: "192.0.2.1",
		TTL:     0,
	}
	created, err := p.CreateRecord(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "example.com", record)
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}
	if created.ID != "rec_new789" {
		t.Errorf("created.ID = %q, want rec_new789", created.ID)
	}
}

func TestDNSUpdateRecord_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/domains/records/rec_abc123" || r.Method != http.MethodPatch {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "dns_record_updated.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	record := domain.DNSRecord{
		Type:    "A",
		Name:    "updated-www",
		Content: "203.0.113.10",
		TTL:     300,
	}
	updated, err := p.UpdateRecord(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "example.com", "rec_abc123", record)
	if err != nil {
		t.Fatalf("UpdateRecord failed: %v", err)
	}
	if updated.ID != "rec_abc123" {
		t.Errorf("updated.ID = %q, want rec_abc123", updated.ID)
	}
	if updated.Type != "A" {
		t.Errorf("updated.Type = %q, want A", updated.Type)
	}
	if updated.Name != "updated-www" {
		t.Errorf("updated.Name = %q, want updated-www", updated.Name)
	}
	if updated.Content != "203.0.113.10" {
		t.Errorf("updated.Content = %q, want 203.0.113.10", updated.Content)
	}
	if updated.TTL != 300 {
		t.Errorf("updated.TTL = %d, want 300", updated.TTL)
	}
}

func TestDNSDeleteRecord_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/domains/example.com/records/rec_abc123" || r.Method != http.MethodDelete {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	p := NewProvider()
	err := p.DeleteRecord(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "example.com", "rec_abc123")
	if err != nil {
		t.Fatalf("DeleteRecord failed: %v", err)
	}
}

func TestDNSDeleteRecord_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	err := p.DeleteRecord(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "example.com", "rec_nonexistent")
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

func TestGetResource_DomainVerified(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/domains/example.com" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "domain_detail.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	res, err := p.GetResource(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "example.com")
	if err != nil {
		t.Fatalf("GetResource failed: %v", err)
	}
	if res.ExternalID != "example.com" {
		t.Errorf("ExternalID = %q, want example.com", res.ExternalID)
	}
	if res.DisplayName != "example.com" {
		t.Errorf("DisplayName = %q, want example.com", res.DisplayName)
	}
	if res.Meta["verified"] != true {
		t.Errorf("Meta[verified] = %v, want true", res.Meta["verified"])
	}
}

func TestGetResource_DomainUnverified(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/domains/unverified.org" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "domain_detail_unverified.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	res, err := p.GetResource(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "unverified.org")
	if err != nil {
		t.Fatalf("GetResource failed: %v", err)
	}
	if res.Meta["verified"] != false {
		t.Errorf("Meta[verified] = %v, want false", res.Meta["verified"])
	}
}

func TestGetResource_DomainNotFound(t *testing.T) {
	first := true
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if first {
			first = false
			if r.URL.Path != "/v5/domains/nonexistent.com" {
				t.Errorf("first: unexpected path: %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusNotFound)
			w.Write(readFixture(t, "not_found.json"))
			return
		}
		if r.URL.Path != "/v9/projects/nonexistent.com" {
			t.Errorf("second: unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	_, err := p.GetResource(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "nonexistent.com")
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

func TestDNSRateLimit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":"rate_limited"}}`))
	}))
	defer ts.Close()

	p := NewProvider()
	_, err := p.ListRecords(context.Background(), &domain.ProviderConnection{Endpoint: ts.URL}, []byte("vct_test"), "example.com")
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

func TestDescriptor(t *testing.T) {
	p := NewProvider()
	desc := p.Descriptor()
	if desc.Type != domain.ProviderTypeVercel {
		t.Errorf("Type = %q, want %q", desc.Type, domain.ProviderTypeVercel)
	}
	if len(desc.Products) == 0 {
		t.Error("expected at least one product")
	}
}