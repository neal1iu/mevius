package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mevius/internal/provider"
	"mevius/internal/service"
)

func TestCatalogRequiresAuthAndContainsProducts(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewStubProvider("github"))
	handler := NewRouter("secret", slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil, nil, nil, nil, registry)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/providers", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/providers", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("catalog status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "github.actions") {
		t.Fatalf("catalog missing pipeline product: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"resource_roles"`) || !strings.Contains(response.Body.String(), `"compatible_roles":["automation"]`) {
		t.Fatalf("catalog missing role taxonomy: %s", response.Body.String())
	}
}

func TestUnprocessableStatus(t *testing.T) {
	if got := statusFor(service.ErrUnprocessable); got != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d", got)
	}
}
