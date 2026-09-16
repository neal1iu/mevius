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

func TestGetResource_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/test-account-id/workers/scripts/my-worker" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "script_detail.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	res, err := p.GetResource(context.Background(), &domain.ProviderAccount{
		TokenEncrypted: "test-token",
		Meta:           domain.AccountMeta{AccountID: "test-account-id"},
	}, "my-worker")
	if err != nil {
		t.Fatalf("GetResource failed: %v", err)
	}
	if res.ExternalID != "my-worker" {
		t.Errorf("ExternalID = %q, want my-worker", res.ExternalID)
	}
	if res.DisplayName != "my-worker" {
		t.Errorf("DisplayName = %q, want my-worker", res.DisplayName)
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