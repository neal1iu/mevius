package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/service"
	"mevius/internal/store"
)

type stubCredStore struct{}

func (s *stubCredStore) Resolve(ctx context.Context, ref string) ([]byte, error) {
	return []byte("test-credential"), nil
}

type captureHandler struct {
	slog.Handler
	mu  sync.Mutex
	buf *bytes.Buffer
}

func newCaptureHandler() *captureHandler {
	var buf bytes.Buffer
	return &captureHandler{
		Handler: slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}),
		buf:     &buf,
	}
}

func (h *captureHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *captureHandler) Handle(ctx context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Handle(ctx, record)
}

func (h *captureHandler) String() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.buf.String()
}

type stubAPIProvider struct {
	deployErr error
}

func (s *stubAPIProvider) Type() string { return "cloudflare" }

func (s *stubAPIProvider) Descriptor() domain.ProviderDescriptor {
	return provider.CloudflareDescriptor
}

func (s *stubAPIProvider) ValidateCredentials(ctx context.Context, conn *domain.ProviderConnection, credential []byte) (json.RawMessage, error) {
	return json.RawMessage(`{"account_id":"cf-123"}`), nil
}

func (s *stubAPIProvider) ListExternalResources(ctx context.Context, conn *domain.ProviderConnection, credential []byte, kind domain.ResourceKind) ([]domain.ExternalResource, error) {
	return []domain.ExternalResource{}, nil
}

func (s *stubAPIProvider) GetResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
	return &domain.ExternalResource{ExternalID: externalID, Meta: map[string]any{"key": "val"}}, nil
}

func (s *stubAPIProvider) TriggerDeploy(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, slot *domain.Slot) (*domain.DeployEvent, error) {
	if s.deployErr != nil {
		return nil, s.deployErr
	}
	return &domain.DeployEvent{ID: "dep-1", Status: "queued", CreatedAt: "now"}, nil
}

func (s *stubAPIProvider) ListDeployments(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding) ([]domain.DeployEvent, error) {
	return nil, nil
}

func (s *stubAPIProvider) GetBuildLogs(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, deployID string, tail int) (domain.LogChunk, error) {
	return domain.LogChunk{}, nil
}

var _ provider.Provider = (*stubAPIProvider)(nil)
var _ provider.Inspector = (*stubAPIProvider)(nil)
var _ provider.Deployer = (*stubAPIProvider)(nil)

func realDB(t *testing.T) (*sql.DB, *store.Queries) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, store.New(db)
}

func seedTestData(t *testing.T, q *store.Queries) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	if err := q.InsertProviderConnection(ctx, store.InsertProviderConnectionParams{
		ID:                  "acct-cf",
		Provider:            "cloudflare",
		Label:               "CF",
		Endpoint:            "",
		ConfigJson:          "{}",
		EncryptedCredential: "enc:cf",
		RemoteIdentityJson:  `{}`,
		CreatedAt:           now,
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := q.InsertProject(ctx, store.InsertProjectParams{
		ID: "proj-1", Name: "test-project", Description: "", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := q.InsertSlot(ctx, store.InsertSlotParams{
		ID: "slot-1", ProjectID: "proj-1", Role: "static-site", Name: "site", ConfigJson: `{}`, CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed slot: %v", err)
	}
	if err := q.InsertBinding(ctx, store.InsertBindingParams{
		ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-cf", Product: "cloudflare",
		ExternalID: "ext-1", CachedMetaJson: `{}`, SyncStatus: "never", LastSyncedAt: nil, CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed binding: %v", err)
	}
}

func testRouter(t *testing.T, token string, logger *slog.Logger) http.Handler {
	t.Helper()
	_, q := realDB(t)
	seedTestData(t, q)

	reg := provider.NewRegistry()
	stub := &stubAPIProvider{}
	reg.Register(stub)
	eng := service.NewRefreshEngine(q, reg, &stubCredStore{})

	var key [32]byte
	connSvc := service.NewConnectionService(q, key, reg)
	projSvc := service.NewProjectService(q)
	slotSvc := service.NewSlotService(q)
	bindingSvc := service.NewBindingService(q, reg, &stubCredStore{}, eng)
	deploySvc := service.NewDeployService(q, reg, &stubCredStore{}, eng)

	return NewRouter(token, logger, connSvc, projSvc, slotSvc, bindingSvc, deploySvc, q, reg, eng, &stubCredStore{})
}

func TestAuthMatrix(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := testRouter(t, "my-secret-token", logger)

	tests := []struct {
		name       string
		path       string
		method     string
		authHeader string
		body       string
		wantStatus int
	}{
		{name: "health no auth", path: "/api/v1/health", method: "GET", wantStatus: 200},

		{name: "list accounts no token", path: "/api/v1/connections", method: "GET", wantStatus: 401},
		{name: "list accounts wrong token", path: "/api/v1/connections", method: "GET", authHeader: "Bearer wrongtoken", wantStatus: 401},
		{name: "list accounts correct token", path: "/api/v1/connections", method: "GET", authHeader: "Bearer my-secret-token", wantStatus: 200},

		{name: "list projects no token", path: "/api/v1/projects", method: "GET", wantStatus: 401},
		{name: "list projects correct token", path: "/api/v1/projects", method: "GET", authHeader: "Bearer my-secret-token", wantStatus: 200},

		{name: "discover no token", path: "/api/v1/connections/acct-cf/discover?product=cloudflare.pages", method: "GET", wantStatus: 401},
		{name: "discover correct token", path: "/api/v1/connections/acct-cf/discover?product=cloudflare.pages", method: "GET", authHeader: "Bearer my-secret-token", wantStatus: 200},

		{name: "bind no token", path: "/api/v1/slots/slot-1/bindings", method: "POST", body: `{"connection_id":"acct-cf","product":"cloudflare.pages","external_id":"ext-x"}`, wantStatus: 401},
		{name: "unbind no token", path: "/api/v1/bindings/bnd-1", method: "DELETE", wantStatus: 401},
		{name: "refresh no token", path: "/api/v1/bindings/bnd-1/refresh", method: "POST", wantStatus: 401},

		{name: "deploy no token", path: "/api/v1/bindings/bnd-1/deploys", method: "POST", wantStatus: 401},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Errorf("%s %s: got status %d, want %d; body: %s", tt.method, tt.path, w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestCreateAccountValidation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := testRouter(t, "my-secret-token", logger)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "missing provider", body: `{"label":"test","token":"abc"}`, wantStatus: 400},
		{name: "missing token", body: `{"provider":"cloudflare","label":"test"}`, wantStatus: 400},
		{name: "empty body", body: `{}`, wantStatus: 400},
		{name: "invalid json", body: `{bad`, wantStatus: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/connections", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer my-secret-token")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Errorf("got %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestHealthCheckJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := testRouter(t, "my-secret-token", logger)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal health response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

func TestAccountNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := testRouter(t, "my-secret-token", logger)

	req := httptest.NewRequest("GET", "/api/v1/connections/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("got %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestProjectNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := testRouter(t, "my-secret-token", logger)

	req := httptest.NewRequest("GET", "/api/v1/projects/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("got %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestDiscoverWithoutKind(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := testRouter(t, "my-secret-token", logger)

	req := httptest.NewRequest("GET", "/api/v1/connections/acct-cf/discover", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("got %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestUnbindSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := testRouter(t, "my-secret-token", logger)

	req := httptest.NewRequest("DELETE", "/api/v1/bindings/bnd-1", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Errorf("got %d, want 204; body: %s", w.Code, w.Body.String())
	}
}

func TestRedactionInAPIResponse(t *testing.T) {
	cLog := newCaptureHandler()
	logger := slog.New(cLog)
	handler := testRouter(t, "my-secret-token", logger)

	leakedToken := "ghp_[A-Za-z0-9]{36}"
	body := `{"provider":"cloudflare","label":"test","token":"` + leakedToken + `"}`
	req := httptest.NewRequest("POST", "/api/v1/connections", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer my-secret-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	respBody := w.Body.String()
	if strings.Contains(respBody, leakedToken) {
		t.Errorf("plaintext token found in response body: %s", respBody)
	}

	logOutput := cLog.String()
	if strings.Contains(logOutput, leakedToken) {
		t.Errorf("plaintext token found in log output: %s", logOutput)
	}
}

func TestRedactionInDeployErrorResponse(t *testing.T) {
	cLog := newCaptureHandler()
	logger := slog.New(cLog)

	_, q := realDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	q.InsertProviderConnection(ctx, store.InsertProviderConnectionParams{
		ID: "acct-cf", Provider: "cloudflare", Label: "CF", Endpoint: "",
		ConfigJson: "{}", EncryptedCredential: "enc:cf", RemoteIdentityJson: `{}`, CreatedAt: now,
	})
	q.InsertProject(ctx, store.InsertProjectParams{
		ID: "proj-1", Name: "tp", Description: "", CreatedAt: now, UpdatedAt: now,
	})
	q.InsertSlot(ctx, store.InsertSlotParams{
		ID: "slot-1", ProjectID: "proj-1", Role: "static-site", Name: "s", ConfigJson: `{}`, CreatedAt: now,
	})
	q.InsertBinding(ctx, store.InsertBindingParams{
		ID: "bnd-dep", SlotID: "slot-1", ConnectionID: "acct-cf", Product: "cloudflare",
		ExternalID: "ext-dep", CachedMetaJson: `{}`, SyncStatus: "never", LastSyncedAt: nil, CreatedAt: now,
	})

	reg := provider.NewRegistry()
	stub := &stubAPIProvider{
		deployErr: &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "upstream error with ghp_[A-Za-z0-9]{36}"},
	}
	reg.Register(stub)
	eng := service.NewRefreshEngine(q, reg, &stubCredStore{})
	var key [32]byte
	connSvc := service.NewConnectionService(q, key, reg)
	projSvc := service.NewProjectService(q)
	slotSvc := service.NewSlotService(q)
	bindingSvc := service.NewBindingService(q, reg, &stubCredStore{}, eng)
	deploySvc := service.NewDeployService(q, reg, &stubCredStore{}, eng)

	handler := NewRouter("my-secret-token", logger, connSvc, projSvc, slotSvc, bindingSvc, deploySvc, q, reg, eng, &stubCredStore{})

	req := httptest.NewRequest("POST", "/api/v1/bindings/bnd-dep/deploys", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	respBody := w.Body.String()
	if strings.Contains(respBody, "ghp_") && !strings.Contains(respBody, "ghp_***") {
		t.Errorf("token not scrubbed in deploy error response: %s", respBody)
	}

	logOutput := cLog.String()
	if strings.Contains(logOutput, "ghp_") && !strings.Contains(logOutput, "ghp_***") {
		t.Errorf("token not scrubbed in deploy error log: %s", logOutput)
	}
}