package cloudflare

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	var callOrder []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/user/tokens/verify":
			callOrder = append(callOrder, "verify")
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "verify.json"))
		case "/accounts":
			callOrder = append(callOrder, "accounts")
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "accounts.json"))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	meta, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "test-token"})
	if err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}
	if meta.AccountID != "test-account-id" {
		t.Errorf("AccountID = %q, want %q", meta.AccountID, "test-account-id")
	}
	raw := meta.Raw
	if raw == nil {
		t.Fatal("Raw is nil")
	}
	if raw["account_id"] != "test-account-id" {
		t.Errorf("account_id = %v, want test-account-id", raw["account_id"])
	}
	if raw["account_name"] != "Test Account" {
		t.Errorf("account_name = %v, want Test Account", raw["account_name"])
	}
	if len(callOrder) != 2 || callOrder[0] != "verify" || callOrder[1] != "accounts" {
		t.Errorf("call order = %v, want [verify accounts]", callOrder)
	}
}

func TestValidateCredentials_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(readFixture(t, "unauthorized.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "bad-token"})
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

func TestValidateCredentials_VerifyFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":false,"errors":[{"code":0,"message":"token inactive"}],"result":null}`))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "bad-token"})
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

func TestListExternalResources_Workers(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/test-account-id/workers/scripts" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "scripts_list.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	resources, err := p.ListExternalResources(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
		Meta:           domain.AccountMeta{AccountID: "test-account-id"},
	}, domain.ResourceKindCompute)
	if err != nil {
		t.Fatalf("ListExternalResources failed: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(resources))
	}
	if resources[0].ExternalID != "my-worker" {
		t.Errorf("resources[0].ExternalID = %q, want my-worker", resources[0].ExternalID)
	}
	if resources[0].DisplayName != "my-worker" {
		t.Errorf("resources[0].DisplayName = %q, want my-worker", resources[0].DisplayName)
	}
	if resources[0].Meta["modified_on"] != "2025-06-01T12:00:00Z" {
		t.Errorf("resources[0].Meta[modified_on] = %v, want 2025-06-01T12:00:00Z", resources[0].Meta["modified_on"])
	}
	if resources[1].ExternalID != "another-worker" {
		t.Errorf("resources[1].ExternalID = %q, want another-worker", resources[1].ExternalID)
	}
}

func TestListExternalResources_UnsupportedKind(t *testing.T) {
	p := NewProvider("")
	resources, err := p.ListExternalResources(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test",
		Meta:           domain.AccountMeta{AccountID: "test-account-id"},
	}, domain.ResourceKindRepo)
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

func TestGetResource_Worker_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/my-worker" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "zone_detail.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	res, err := p.GetResource(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
	}, "my-worker")
	if err != nil {
		t.Fatalf("GetResource failed: %v", err)
	}
	if res.ExternalID != "zone-001" {
		t.Errorf("ExternalID = %q, want zone-001", res.ExternalID)
	}
}

func TestGetResource_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.GetResource(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
		Meta:           domain.AccountMeta{AccountID: "test-account-id"},
	}, "nonexistent")
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
	var capturedBody []byte
	var capturedContentType string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/test-account-id/workers/scripts/new-worker" || r.Method != http.MethodPut {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		capturedContentType = r.Header.Get("Content-Type")
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		r.Body.Close()
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "create_response.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	spec := domain.ResourceSpec{
		Name: "new-worker",
		Kind: domain.ResourceKindCompute,
	}
	res, err := p.CreateResource(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
		Meta:           domain.AccountMeta{AccountID: "test-account-id"},
	}, spec)
	if err != nil {
		t.Fatalf("CreateResource failed: %v", err)
	}
	if res.ExternalID != "new-worker" {
		t.Errorf("ExternalID = %q, want new-worker", res.ExternalID)
	}
	if res.DisplayName != "new-worker" {
		t.Errorf("DisplayName = %q, want new-worker", res.DisplayName)
	}
	if !strings.Contains(capturedContentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q, want multipart/form-data", capturedContentType)
	}
	if !strings.Contains(string(capturedBody), "mevius placeholder") {
		t.Error("request body does not contain 'mevius placeholder'")
	}
	if !strings.Contains(string(capturedBody), "main_module") {
		t.Error("request body does not contain metadata")
	}
}

func TestDeleteResource_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/test-account-id/workers/scripts/my-worker" || r.Method != http.MethodDelete {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"result":{},"errors":[]}`))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	err := p.DeleteResource(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
		Meta:           domain.AccountMeta{AccountID: "test-account-id"},
	}, "my-worker")
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
	err := p.DeleteResource(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
		Meta:           domain.AccountMeta{AccountID: "test-account-id"},
	}, "nonexistent")
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

func TestCreateResource_RequiresName(t *testing.T) {
	p := NewProvider("")
	_, err := p.CreateResource(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test",
		Meta:           domain.AccountMeta{AccountID: "test-account-id"},
	}, domain.ResourceSpec{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestInvalidToken_MappedUnauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/tokens/verify" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write(readFixture(t, "unauthorized.json"))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.ValidateCredentials(context.Background(), &domain.ProviderAccount{TokenEncrypted: "invalid"})
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

func TestListExternalResources_DNSDomains(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "zones.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	resources, err := p.ListExternalResources(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
	}, domain.ResourceKindDNSDomain)
	if err != nil {
		t.Fatalf("ListExternalResources failed: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(resources))
	}
	if resources[0].ExternalID != "zone-001" {
		t.Errorf("resources[0].ExternalID = %q, want zone-001", resources[0].ExternalID)
	}
	if resources[0].DisplayName != "example.com" {
		t.Errorf("resources[0].DisplayName = %q, want example.com", resources[0].DisplayName)
	}
	if resources[0].Meta["status"] != "active" {
		t.Errorf("resources[0].Meta[status] = %v, want active", resources[0].Meta["status"])
	}
	if resources[1].ExternalID != "zone-002" {
		t.Errorf("resources[1].ExternalID = %q, want zone-002", resources[1].ExternalID)
	}
	if resources[1].DisplayName != "test.org" {
		t.Errorf("resources[1].DisplayName = %q, want test.org", resources[1].DisplayName)
	}
}

func TestGetResource_Zone(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/zone-001" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "zone_detail.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	res, err := p.GetResource(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
	}, "zone-001")
	if err != nil {
		t.Fatalf("GetResource failed: %v", err)
	}
	if res.ExternalID != "zone-001" {
		t.Errorf("ExternalID = %q, want zone-001", res.ExternalID)
	}
	if res.DisplayName != "example.com" {
		t.Errorf("DisplayName = %q, want example.com", res.DisplayName)
	}
	if res.Meta["status"] != "active" {
		t.Errorf("Meta[status] = %v, want active", res.Meta["status"])
	}
}

func TestListRecords_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/zone-001/dns_records" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "dns_records.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	records, err := p.ListRecords(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
	}, "zone-001")
	if err != nil {
		t.Fatalf("ListRecords failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].ID != "rec-001" {
		t.Errorf("records[0].ID = %q, want rec-001", records[0].ID)
	}
	if records[0].Type != "A" {
		t.Errorf("records[0].Type = %q, want A", records[0].Type)
	}
	if records[0].Content != "192.0.2.1" {
		t.Errorf("records[0].Content = %q, want 192.0.2.1", records[0].Content)
	}
	if records[0].TTL != 120 {
		t.Errorf("records[0].TTL = %d, want 120", records[0].TTL)
	}
	if records[0].Proxied == nil || !*records[0].Proxied {
		t.Error("records[0].Proxied = nil or false, want true")
	}
	if records[1].Priority == nil || *records[1].Priority != 10 {
		t.Errorf("records[1].Priority = %v, want 10", records[1].Priority)
	}
}

func TestCreateRecord_Success(t *testing.T) {
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/zone-001/dns_records" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		r.Body.Close()
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "dns_record_create.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	proxied := true
	rec, err := p.CreateRecord(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
	}, "zone-001", domain.DNSRecord{
		Type:    "CNAME",
		Name:    "blog.example.com",
		Content: "example.github.io",
		TTL:     1,
		Proxied: &proxied,
	})
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}
	if rec.ID != "rec-003" {
		t.Errorf("ID = %q, want rec-003", rec.ID)
	}
	if rec.Type != "CNAME" {
		t.Errorf("Type = %q, want CNAME", rec.Type)
	}
	if !strings.Contains(string(capturedBody), `"name":"blog.example.com"`) {
		t.Error("request body does not contain blog.example.com")
	}
}

func TestUpdateRecord_Success(t *testing.T) {
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/zone-001/dns_records/rec-001" || r.Method != http.MethodPatch {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		r.Body.Close()
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "dns_record_update.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	proxied := true
	rec, err := p.UpdateRecord(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
	}, "zone-001", "rec-001", domain.DNSRecord{
		Type:    "A",
		Name:    "www.example.com",
		Content: "203.0.113.1",
		TTL:     120,
		Proxied: &proxied,
	})
	if err != nil {
		t.Fatalf("UpdateRecord failed: %v", err)
	}
	if rec.ID != "rec-001" {
		t.Errorf("ID = %q, want rec-001", rec.ID)
	}
	if rec.Content != "203.0.113.1" {
		t.Errorf("Content = %q, want 203.0.113.1", rec.Content)
	}
	if !strings.Contains(string(capturedBody), `"content":"203.0.113.1"`) {
		t.Error("request body does not contain updated content")
	}
}

func TestDeleteRecord_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/zone-001/dns_records/rec-001" || r.Method != http.MethodDelete {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"result":{},"errors":[]}`))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	err := p.DeleteRecord(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
	}, "zone-001", "rec-001")
	if err != nil {
		t.Fatalf("DeleteRecord failed: %v", err)
	}
}

func TestDeleteRecord_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	err := p.DeleteRecord(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
	}, "zone-001", "nonexistent")
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

func TestZeroNetworkCalls(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"result":[],"errors":[],"messages":[]}`))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.ListExternalResources(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test",
		Meta:           domain.AccountMeta{AccountID: "test-aid"},
	}, domain.ResourceKindCompute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}