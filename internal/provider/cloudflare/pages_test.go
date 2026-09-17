package cloudflare

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

func TestPagesListExternalResources(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/test-aid/pages/projects" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "pages_projects_list.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	resources, err := p.ListExternalResources(context.Background(), conn, []byte("test-token"), "cloudflare.pages")
	if err != nil {
		t.Fatalf("ListExternalResources failed: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(resources))
	}
	if resources[0].ExternalID != "my-site" {
		t.Errorf("resources[0].ExternalID = %q, want my-site", resources[0].ExternalID)
	}
	if resources[0].Meta["source_type"] != "github" {
		t.Errorf("resources[0].Meta[source_type] = %v, want github", resources[0].Meta["source_type"])
	}
}

func TestPagesListExternalResources_UnsupportedKind(t *testing.T) {
	p := NewProvider()
	conn := &domain.ProviderConnection{
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	_, err := p.ListExternalResources(context.Background(), conn, []byte("test"), "invalid.product")
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

func TestPagesCreateResource(t *testing.T) {
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/test-aid/pages/projects" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		capturedBody = readFixture(t, "pages_create.json")
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "pages_create.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	spec := domain.ResourceSpec{
		Name: "new-site",
		Kind: domain.ResourceKindStaticSite,
		Extra: map[string]any{
			"production_branch": "main",
		},
	}
	res, err := p.CreateResource(context.Background(), conn, []byte("test-token"), spec)
	if err != nil {
		t.Fatalf("CreateResource failed: %v", err)
	}
	if res.ExternalID != "new-site" {
		t.Errorf("ExternalID = %q, want new-site", res.ExternalID)
	}
	if res.DisplayName != "new-site" {
		t.Errorf("DisplayName = %q, want new-site", res.DisplayName)
	}
	if res.Meta["subdomain"] != "new-site.pages.dev" {
		t.Errorf("Meta[subdomain] = %v, want new-site.pages.dev", res.Meta["subdomain"])
	}
	_ = capturedBody
}

func TestPagesDeleteResource(t *testing.T) {
	var callOrder []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/accounts/test-aid/workers/scripts/my-site" && r.Method == http.MethodDelete:
			callOrder = append(callOrder, "worker-try")
			w.WriteHeader(http.StatusNotFound)
			w.Write(readFixture(t, "not_found.json"))
		case r.URL.Path == "/accounts/test-aid/pages/projects/my-site" && r.Method == http.MethodDelete:
			callOrder = append(callOrder, "pages-delete")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{},"errors":[]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	err := p.DeleteResource(context.Background(), conn, []byte("test-token"), "my-site")
	if err != nil {
		t.Fatalf("DeleteResource failed: %v", err)
	}
	if len(callOrder) != 2 || callOrder[0] != "worker-try" || callOrder[1] != "pages-delete" {
		t.Errorf("call order = %v, want [worker-try pages-delete]", callOrder)
	}
}

func TestPagesTriggerDeploy_GitConnected(t *testing.T) {
	var callOrder []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/accounts/test-aid/pages/projects/my-site":
			if r.Method != http.MethodGet {
				t.Errorf("unexpected method: %s %s", r.Method, r.URL.Path)
			}
			callOrder = append(callOrder, "inspect")
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "pages_project_git.json"))
		case "/accounts/test-aid/pages/projects/my-site/deployments/dep-latest/retry":
			if r.Method != http.MethodPost {
				t.Errorf("unexpected method: %s %s", r.Method, r.URL.Path)
			}
			callOrder = append(callOrder, "retry")
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "pages_retry_202.json"))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	ev, err := p.TriggerDeploy(context.Background(), conn, []byte("test-token"), &domain.Binding{ExternalID: "my-site"}, &domain.Slot{})
	if err != nil {
		t.Fatalf("TriggerDeploy failed: %v", err)
	}
	if ev.ID != "dep-retried" {
		t.Errorf("ev.ID = %q, want dep-retried", ev.ID)
	}
	if ev.Status != "queued" {
		t.Errorf("ev.Status = %q, want queued", ev.Status)
	}
	if len(callOrder) != 2 || callOrder[0] != "inspect" || callOrder[1] != "retry" {
		t.Errorf("call order = %v, want [inspect retry]", callOrder)
	}
}

func TestPagesTriggerDeploy_DirectUploadUnsupported(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "pages_project_direct.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	_, err := p.TriggerDeploy(context.Background(), conn, []byte("test-token"), &domain.Binding{ExternalID: "direct-site"}, &domain.Slot{})
	if err == nil {
		t.Fatal("expected unsupported error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUnsupported {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUnsupported)
	}
}

func TestPagesTriggerDeploy_InProgressConflict(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "pages_project_inprogress.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	_, err := p.TriggerDeploy(context.Background(), conn, []byte("test-token"), &domain.Binding{ExternalID: "inprogress-site"}, &domain.Slot{})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("error type = %T, want *provider.Error", err)
	}
	if pErr.Kind != provider.KindUpstream {
		t.Errorf("Kind = %q, want %q", pErr.Kind, provider.KindUpstream)
	}
	if !strings.Contains(pErr.Error(), "already in progress") {
		t.Errorf("error message = %q, want 'already in progress'", pErr.Error())
	}
}

func TestPagesTriggerMatrix(t *testing.T) {
	t.Run("git connected", func(t *testing.T) {
		var callOrder []string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/accounts/test-aid/pages/projects/git-site":
				callOrder = append(callOrder, "inspect")
				w.WriteHeader(http.StatusOK)
				w.Write(readFixture(t, "pages_project_git.json"))
			case "/accounts/test-aid/pages/projects/git-site/deployments/dep-latest/retry":
				callOrder = append(callOrder, "retry")
				w.WriteHeader(http.StatusOK)
				w.Write(readFixture(t, "pages_retry_202.json"))
			default:
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
		}))
		defer ts.Close()

		p := NewProvider()
		conn := &domain.ProviderConnection{
			Endpoint: ts.URL,
			RemoteIdentity: domain.AccountMeta{
				Raw: map[string]any{"account_id": "test-aid"},
			},
		}
		ev, err := p.TriggerDeploy(context.Background(), conn, []byte("test-token"), &domain.Binding{ExternalID: "git-site"}, &domain.Slot{})
		if err != nil {
			t.Fatalf("git connected: TriggerDeploy failed: %v", err)
		}
		if ev == nil {
			t.Fatal("git connected: ev is nil")
		}
		if ev.Status != "queued" {
			t.Errorf("git connected: Status = %q, want queued", ev.Status)
		}
		if len(callOrder) != 2 || callOrder[0] != "inspect" || callOrder[1] != "retry" {
			t.Errorf("git connected: call order = %v, want [inspect retry]", callOrder)
		}
	})

	t.Run("direct upload unsupported", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "pages_project_direct.json"))
		}))
		defer ts.Close()

		p := NewProvider()
		conn := &domain.ProviderConnection{
			Endpoint: ts.URL,
			RemoteIdentity: domain.AccountMeta{
				Raw: map[string]any{"account_id": "test-aid"},
			},
		}
		_, err := p.TriggerDeploy(context.Background(), conn, []byte("test-token"), &domain.Binding{ExternalID: "direct-site"}, &domain.Slot{})
		if err == nil {
			t.Fatal("direct upload: expected error")
		}
		var pErr *provider.Error
		if !errors.As(err, &pErr) {
			t.Fatalf("direct upload: error type = %T", err)
		}
		if pErr.Kind != provider.KindUnsupported {
			t.Errorf("direct upload: Kind = %q, want %q", pErr.Kind, provider.KindUnsupported)
		}
	})

	t.Run("in progress 409", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "pages_project_inprogress.json"))
		}))
		defer ts.Close()

		p := NewProvider()
		conn := &domain.ProviderConnection{
			Endpoint: ts.URL,
			RemoteIdentity: domain.AccountMeta{
				Raw: map[string]any{"account_id": "test-aid"},
			},
		}
		_, err := p.TriggerDeploy(context.Background(), conn, []byte("test-token"), &domain.Binding{ExternalID: "inprogress-site"}, &domain.Slot{})
		if err == nil {
			t.Fatal("in progress: expected error")
		}
		var pErr *provider.Error
		if !errors.As(err, &pErr) {
			t.Fatalf("in progress: error type = %T", err)
		}
		if pErr.Kind != provider.KindUpstream {
			t.Errorf("in progress: Kind = %q, want %q", pErr.Kind, provider.KindUpstream)
		}
	})
}

func TestPagesListDeployments(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/accounts/test-aid/pages/projects/my-site/deployments"
		if r.URL.Path != expected || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "pages_deployments_list.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	events, err := p.ListDeployments(context.Background(), conn, []byte("test-token"), &domain.Binding{ExternalID: "my-site"})
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].ID != "dep-1" {
		t.Errorf("events[0].ID = %q, want dep-1", events[0].ID)
	}
	if events[0].Status != "success" {
		t.Errorf("events[0].Status = %q, want success", events[0].Status)
	}
	if events[1].ID != "dep-2" {
		t.Errorf("events[1].ID = %q, want dep-2", events[1].ID)
	}
	if events[1].Status != "failed" {
		t.Errorf("events[1].Status = %q, want failed", events[1].Status)
	}
}

func TestPagesGetBuildLogs_Fallback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/accounts/test-aid/pages/projects/my-site/deployments/dep-1"
		if r.URL.Path != expected || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(readFixture(t, "pages_deployment_stages.json"))
	}))
	defer ts.Close()

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	chunk, err := p.GetBuildLogs(context.Background(), conn, []byte("test-token"), &domain.Binding{ExternalID: "my-site"}, "dep-1", 100)
	if err != nil {
		t.Fatalf("GetBuildLogs failed: %v", err)
	}
	if chunk.Lines == "" {
		t.Error("LogChunk.Lines is empty")
	}
	if chunk.Truncated {
		t.Error("LogChunk.Truncated = true, want false")
	}
	if chunk.Meta == nil {
		t.Fatal("LogChunk.Meta is nil")
	}
	if chunk.Meta["fallback"] != "dashboard_link" {
		t.Errorf("Meta[fallback] = %v, want dashboard_link", chunk.Meta["fallback"])
	}
	url, ok := chunk.Meta["url"].(string)
	if !ok {
		t.Fatal("Meta[url] is not a string")
	}
	if !strings.Contains(url, "dash.cloudflare.com") {
		t.Errorf("Meta[url] = %q, want dash.cloudflare.com URL", url)
	}
	if !strings.Contains(url, "dep-1") {
		t.Errorf("Meta[url] = %q, want URL containing dep-1", url)
	}
}

func TestPagesLogs(t *testing.T) {
	t.Run("fallback with stages", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(readFixture(t, "pages_deployment_stages.json"))
		}))
		defer ts.Close()

		p := NewProvider()
		conn := &domain.ProviderConnection{
			Endpoint: ts.URL,
			RemoteIdentity: domain.AccountMeta{
				Raw: map[string]any{"account_id": "test-aid"},
			},
		}
		chunk, err := p.GetBuildLogs(context.Background(), conn, []byte("test-token"), &domain.Binding{ExternalID: "my-site"}, "dep-1", 50)
		if err != nil {
			t.Fatalf("GetBuildLogs failed: %v", err)
		}
		if chunk.Lines == "" {
			t.Error("LogChunk.Lines is empty")
		}
		if chunk.Meta == nil || chunk.Meta["fallback"] != "dashboard_link" {
			t.Errorf("Meta[fallback] = %v, want dashboard_link", chunk.Meta["fallback"])
		}
		if chunk.Meta == nil {
			t.Fatal("Meta is nil")
		}
		url, ok := chunk.Meta["url"].(string)
		if !ok || !strings.Contains(url, "dash.cloudflare.com") {
			t.Errorf("Meta[url] = %q, want dash.cloudflare.com URL", url)
		}
	})
}

func TestPagesCreateResource_RequiresName(t *testing.T) {
	p := NewProvider()
	conn := &domain.ProviderConnection{
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	_, err := p.CreateResource(context.Background(), conn, []byte("test"), domain.ResourceSpec{Kind: domain.ResourceKindStaticSite})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestPagesDeleteResource_FallbackToPages(t *testing.T) {
	var callOrder []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/accounts/test-aid/workers/scripts/nonexistent-worker" && r.Method == http.MethodDelete:
			callOrder = append(callOrder, "worker-try")
			w.WriteHeader(http.StatusNotFound)
			w.Write(readFixture(t, "not_found.json"))
		case r.URL.Path == "/accounts/test-aid/pages/projects/nonexistent-worker" && r.Method == http.MethodDelete:
			callOrder = append(callOrder, "pages-fallback")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{},"errors":[]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	p := NewProvider()
	conn := &domain.ProviderConnection{
		Endpoint: ts.URL,
		RemoteIdentity: domain.AccountMeta{
			Raw: map[string]any{"account_id": "test-aid"},
		},
	}
	err := p.DeleteResource(context.Background(), conn, []byte("test-token"), "nonexistent-worker")
	if err != nil {
		t.Fatalf("DeleteResource failed: %v", err)
	}
	if len(callOrder) != 2 || callOrder[0] != "worker-try" || callOrder[1] != "pages-fallback" {
		t.Errorf("call order = %v, want [worker-try pages-fallback]", callOrder)
	}
}