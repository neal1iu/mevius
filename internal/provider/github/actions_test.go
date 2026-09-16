package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

func TestActionsTriggerDeploy_Success(t *testing.T) {
	var method, path string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	slot := slotWithWorkflow("ci.yml", "main")
	ev, err := p.TriggerDeploy(context.Background(), acct(), binding("testuser/my-repo"), slot)
	if err != nil {
		t.Fatalf("TriggerDeploy failed: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	expectedPath := "/repos/testuser/my-repo/actions/workflows/ci.yml/dispatches"
	if path != expectedPath {
		t.Errorf("path = %s, want %s", path, expectedPath)
	}
	if ev.Status != "queued" {
		t.Errorf("Status = %q, want queued", ev.Status)
	}
}

func TestActionsTriggerDeploy_NoWorkflowID(t *testing.T) {
	p := NewProvider("")
	slot := &domain.Slot{Config: json.RawMessage(`{"name":"test"}`)}
	_, err := p.TriggerDeploy(context.Background(), acct(), binding("testuser/my-repo"), slot)
	if err == nil {
		t.Fatal("expected error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUnsupported {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUnsupported)
	}
}

func TestActionsTriggerDeploy_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "dispatch_not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	slot := slotWithWorkflow("nonexistent.yml", "main")
	_, err := p.TriggerDeploy(context.Background(), acct(), binding("testuser/my-repo"), slot)
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

func TestActionsTriggerDeploy_InvalidExternalID(t *testing.T) {
	p := NewProvider("")
	slot := slotWithWorkflow("ci.yml", "main")
	_, err := p.TriggerDeploy(context.Background(), acct(), binding("invalid"), slot)
	if err == nil {
		t.Fatal("expected error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUpstream {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUpstream)
	}
}

func TestActionsListDeployments_Success(t *testing.T) {
	var calledPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPath = r.URL.Path
		if r.URL.Query().Get("per_page") != "20" {
			t.Errorf("per_page = %s, want 20", r.URL.Query().Get("per_page"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "actions_runs.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	events, err := p.ListDeployments(context.Background(), acct(), binding("testuser/my-repo"))
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}
	if calledPath != "/repos/testuser/my-repo/actions/runs" {
		t.Errorf("path = %s, want /repos/testuser/my-repo/actions/runs", calledPath)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].ID != "1001" {
		t.Errorf("events[0].ID = %q, want 1001", events[0].ID)
	}
	if events[0].Status != "success" {
		t.Errorf("events[0].Status = %q, want success", events[0].Status)
	}
	if events[0].CreatedAt != "2026-09-16T10:00:00Z" {
		t.Errorf("events[0].CreatedAt = %q, want 2026-09-16T10:00:00Z", events[0].CreatedAt)
	}
	if events[1].Status != "in_progress" {
		t.Errorf("events[1].Status = %q, want in_progress", events[1].Status)
	}
}

func TestActionsListDeployments_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	events, err := p.ListDeployments(context.Background(), acct(), binding("testuser/my-repo"))
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d events, want 0", len(events))
	}
}

func TestActionsListDeployments_InvalidExternalID(t *testing.T) {
	p := NewProvider("")
	_, err := p.ListDeployments(context.Background(), acct(), binding("invalid"))
	if err == nil {
		t.Fatal("expected error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUpstream {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUpstream)
	}
}

func buildSyntheticZip(t *testing.T, contentSize int) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("0-build.log")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	content := make([]byte, contentSize)
	for i := range content {
		content[i] = byte('A' + (i % 26))
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("write zip content: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}

func TestLogsTruncation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download" {
			zipData := buildSyntheticZip(t, 300*1024)
			w.WriteHeader(http.StatusOK)
			w.Write(zipData)
			return
		}
		redirectURL := "http://" + r.Host + "/download"
		w.Header().Set("Location", redirectURL)
		w.WriteHeader(http.StatusFound)
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	chunk, err := p.GetBuildLogs(context.Background(), acct(), binding("testuser/my-repo"), "42", 256*1024)
	if err != nil {
		t.Fatalf("GetBuildLogs failed: %v", err)
	}
	if !chunk.Truncated {
		t.Error("Truncated = false, want true (300KB > 256KB)")
	}
	if len(chunk.Lines) > 256*1024 {
		t.Errorf("lines length = %d, want <= %d", len(chunk.Lines), 256*1024)
	}
	if !strings.HasPrefix(chunk.Lines, "YZABCDEFGHIJKLMNOPQR") {
		t.Errorf("lines should start with tail content (letter T), got prefix: %q", chunk.Lines[:20])
	}
}

func TestLogsNotTruncated(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download" {
			zipData := buildSyntheticZip(t, 100)
			w.WriteHeader(http.StatusOK)
			w.Write(zipData)
			return
		}
		redirectURL := "http://" + r.Host + "/download"
		w.Header().Set("Location", redirectURL)
		w.WriteHeader(http.StatusFound)
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	chunk, err := p.GetBuildLogs(context.Background(), acct(), binding("testuser/my-repo"), "42", 256*1024)
	if err != nil {
		t.Fatalf("GetBuildLogs failed: %v", err)
	}
	if chunk.Truncated {
		t.Error("Truncated = true, want false (100B < 256KB)")
	}
	if len(chunk.Lines) != 100 {
		t.Errorf("lines length = %d, want 100", len(chunk.Lines))
	}
}

func TestLogsExpired(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.GetBuildLogs(context.Background(), acct(), binding("testuser/my-repo"), "42", 256*1024)
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
	if !strings.Contains(pErr.ProviderMsg, "expired") {
		t.Errorf("ProviderMsg = %q, want it to contain 'expired'", pErr.ProviderMsg)
	}
}

func TestLogsNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(readFixture(t, "not_found.json"))
	}))
	defer ts.Close()

	p := NewProvider(ts.URL)
	_, err := p.GetBuildLogs(context.Background(), acct(), binding("testuser/my-repo"), "99999", 256*1024)
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

func TestLogsInvalidDeployID(t *testing.T) {
	p := NewProvider("")
	_, err := p.GetBuildLogs(context.Background(), acct(), binding("testuser/my-repo"), "not-a-number", 256*1024)
	if err == nil {
		t.Fatal("expected error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUpstream {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUpstream)
	}
}

func TestLogsInvalidExternalID(t *testing.T) {
	p := NewProvider("")
	_, err := p.GetBuildLogs(context.Background(), acct(), binding("invalid"), "42", 256*1024)
	if err == nil {
		t.Fatal("expected error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUpstream {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUpstream)
	}
}

func slotWithWorkflow(workflowID, workflowRef string) *domain.Slot {
	cfg := domain.RepoConfig{
		Name:        "test",
		WorkflowID:  workflowID,
		WorkflowRef: workflowRef,
	}
	data, _ := json.Marshal(cfg)
	return &domain.Slot{Config: data}
}

func binding(externalID string) *domain.Binding {
	return &domain.Binding{ExternalID: externalID}
}

func acct() *domain.ProviderAccount {
	return &domain.ProviderAccount{TokenEncrypted: "ghp_test"}
}