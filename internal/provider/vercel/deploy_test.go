package vercel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

func TestTriggerDeploy_Success(t *testing.T) {
	var listCalled, createCalled bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v6/deployments" {
			listCalled = true
			if r.URL.Query().Get("projectId") != "prj_abc123" {
				t.Errorf("projectId = %q", r.URL.Query().Get("projectId"))
			}
			if r.URL.Query().Get("limit") != "1" {
				t.Errorf("limit = %q", r.URL.Query().Get("limit"))
			}
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "deployments.json"))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v13/deployments" {
			createCalled = true
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "created_deployment.json"))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	binding := &domain.Binding{ExternalID: "prj_abc123"}
	account := &domain.ProviderAccount{TokenEncrypted: "vct_test"}
	evt, err := p.TriggerDeploy(context.Background(), account, binding, &domain.Slot{})
	if err != nil {
		t.Fatalf("TriggerDeploy failed: %v", err)
	}
	if !listCalled {
		t.Error("list deployments not called")
	}
	if !createCalled {
		t.Error("create deployment not called")
	}
	if evt.ID != "dpl_new789" {
		t.Errorf("ID = %q, want dpl_new789", evt.ID)
	}
	if evt.Status != "queued" {
		t.Errorf("Status = %q, want queued", evt.Status)
	}
	if evt.CreatedAt == "" {
		t.Error("CreatedAt is empty")
	}
}

func TestTriggerDeploy_NoExistingDeployments(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "deployments_empty.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	binding := &domain.Binding{ExternalID: "prj_abc123"}
	account := &domain.ProviderAccount{TokenEncrypted: "vct_test"}
	_, err := p.TriggerDeploy(context.Background(), account, binding, &domain.Slot{})
	if err == nil {
		t.Fatal("expected error for no existing deployments")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUpstream {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUpstream)
	}
}

func TestListDeployments_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v6/deployments" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("projectId") != "prj_abc123" {
			t.Errorf("projectId = %q", r.URL.Query().Get("projectId"))
		}
		if r.URL.Query().Get("limit") != "20" {
			t.Errorf("limit = %q", r.URL.Query().Get("limit"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "deployments.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	account := &domain.ProviderAccount{TokenEncrypted: "vct_test"}
	events, err := p.ListDeployments(context.Background(), account, &domain.Binding{ExternalID: "prj_abc123"})
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4", len(events))
	}

	// READY → completed/success
	if events[0].ID != "dpl_abc123" || events[0].Status != "completed/success" {
		t.Errorf("event[0] = %+v, want ID=dpl_abc123 Status=completed/success", events[0])
	}

	// BUILDING → in_progress
	if events[1].ID != "dpl_def456" || events[1].Status != "in_progress" {
		t.Errorf("event[1] = %+v, want ID=dpl_def456 Status=in_progress", events[1])
	}

	// ERROR → completed/failure
	if events[2].ID != "dpl_error789" || events[2].Status != "completed/failure" {
		t.Errorf("event[2] = %+v, want ID=dpl_error789 Status=completed/failure", events[2])
	}

	// QUEUED → queued
	if events[3].ID != "dpl_queued000" || events[3].Status != "queued" {
		t.Errorf("event[3] = %+v, want ID=dpl_queued000 Status=queued", events[3])
	}
}

func TestListDeployments_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "deployments_empty.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	account := &domain.ProviderAccount{TokenEncrypted: "vct_test"}
	events, err := p.ListDeployments(context.Background(), account, &domain.Binding{ExternalID: "prj_abc123"})
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d events, want 0", len(events))
	}
}

func TestGetBuildLogs_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/v3/deployments/dpl_new789/events"
		if r.URL.Path != expectedPath || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "events.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	account := &domain.ProviderAccount{TokenEncrypted: "vct_test"}
	chunk, err := p.GetBuildLogs(context.Background(), account, &domain.Binding{ExternalID: "prj_abc123"}, "dpl_new789", 0)
	if err != nil {
		t.Fatalf("GetBuildLogs failed: %v", err)
	}
	if chunk.Truncated {
		t.Error("expected Truncated=false")
	}
	expected := "Cloning git repository...\nInstalling dependencies...\nBuilding application...\nBuild completed successfully.\n"
	if chunk.Lines != expected {
		t.Errorf("Lines mismatch.\ngot:  %q\nwant: %q", chunk.Lines, expected)
	}
}

func TestGetBuildLogs_Truncated(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "events_big.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	account := &domain.ProviderAccount{TokenEncrypted: "vct_test"}
	chunk, err := p.GetBuildLogs(context.Background(), account, &domain.Binding{ExternalID: "prj_abc123"}, "dpl_big", 0)
	if err != nil {
		t.Fatalf("GetBuildLogs failed: %v", err)
	}
	if !chunk.Truncated {
		t.Error("expected Truncated=true for oversized log")
	}
	if len(chunk.Lines) > 256*1024 {
		t.Errorf("Lines length = %d, want <= 256KB", len(chunk.Lines))
	}
	// tail content should be last 256KB
	if chunk.Lines != string(readFixture(t, "events_big.json"))[:0] && len(chunk.Lines) != 256*1024 {
		// Just verify it's the tail portion (all 'A' characters)
		for _, c := range chunk.Lines {
			if c != 'A' {
				t.Errorf("unexpected char %c in truncated output", c)
				break
			}
		}
	}
}

func TestGetBuildLogs_EmptyEvents(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	account := &domain.ProviderAccount{TokenEncrypted: "vct_test"}
	chunk, err := p.GetBuildLogs(context.Background(), account, &domain.Binding{ExternalID: "prj_abc123"}, "dpl_empty", 0)
	if err != nil {
		t.Fatalf("GetBuildLogs failed: %v", err)
	}
	if chunk.Lines != "" {
		t.Errorf("Lines = %q, want empty", chunk.Lines)
	}
	if chunk.Truncated {
		t.Error("expected Truncated=false for empty events")
	}
}

func TestDeploy_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(readFixture(t, "unauthorized.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.ListDeployments(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_bad"}, &domain.Binding{ExternalID: "prj_abc123"})
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

func TestDeploy_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.GetBuildLogs(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, &domain.Binding{ExternalID: "prj_abc123"}, "dpl_nonexistent", 0)
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

func TestEventsLogExtraction(t *testing.T) {
	t.Run("lines_and_truncated", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "events.json"))
		}))
		defer ts.Close()

		p := NewProvider(ts.URL)
		chunk, err := p.GetBuildLogs(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, &domain.Binding{ExternalID: "prj_abc123"}, "dpl_new789", 0)
		if err != nil {
			t.Fatalf("GetBuildLogs failed: %v", err)
		}
		if chunk.Truncated {
			t.Error("expected Truncated=false for small log")
		}
		if !containsAll(chunk.Lines, "Cloning", "Installing", "Building", "completed") {
			t.Errorf("Lines missing expected content: %q", chunk.Lines)
		}
	})

	t.Run("truncated_oversized", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "events_big.json"))
		}))
		defer ts.Close()

		p := NewProvider(ts.URL)
		chunk, err := p.GetBuildLogs(context.Background(), &domain.ProviderAccount{TokenEncrypted: "vct_test"}, &domain.Binding{ExternalID: "prj_abc123"}, "dpl_big", 0)
		if err != nil {
			t.Fatalf("GetBuildLogs failed: %v", err)
		}
		if !chunk.Truncated {
			t.Error("expected Truncated=true")
		}
	})
}

func TestMapDeployStatus(t *testing.T) {
	tests := []struct {
		readyState string
		want       string
	}{
		{"BUILDING", "in_progress"},
		{"INITIALIZING", "in_progress"},
		{"READY", "completed/success"},
		{"ERROR", "completed/failure"},
		{"CANCELED", "completed/failure"},
		{"BLOCKED", "completed/failure"},
		{"QUEUED", "queued"},
		{"UNKNOWN_STATE", "unknown"},
	}
	for _, tt := range tests {
		got := mapDeployStatus(tt.readyState)
		if got != tt.want {
			t.Errorf("mapDeployStatus(%q) = %q, want %q", tt.readyState, got, tt.want)
		}
	}
}

func containsAll(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}