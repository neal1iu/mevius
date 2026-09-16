package provider

import (
	"net/http"
	"testing"
	"time"
)

func TestMapHTTP(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		headers    map[string]string
		wantKind   Kind
		wantRetry  bool
		wantMsgSub string
	}{
		{name: "401 unauthorized", status: 401, body: `{"message":"bad credentials"}`, wantKind: KindUnauthorized, wantRetry: false, wantMsgSub: "bad credentials"},
		{name: "404 not found", status: 404, body: "not found", wantKind: KindNotFound, wantRetry: false},
		{name: "429 rate limit", status: 429, body: "rate limited", headers: (map[string]string{"Retry-After": "60"}), wantKind: KindRateLimit, wantRetry: true},
		{name: "429 rate limit lowercase header", status: 429, body: "rate limited", headers: (map[string]string{"retry-after": "30"}), wantKind: KindRateLimit, wantRetry: true},
		{name: "429 no retry header", status: 429, body: "too many requests", wantKind: KindRateLimit, wantRetry: false},
		{name: "501 unsupported", status: 501, body: "not implemented", wantKind: KindUnsupported, wantRetry: false},
		{name: "502 upstream error", status: 502, body: "bad gateway", wantKind: KindUpstream, wantRetry: false},
		{name: "500 upstream error", status: 500, body: "internal error", wantKind: KindUpstream, wantRetry: false},
		{name: "403 upstream error", status: 403, body: "forbidden", wantKind: KindUpstream, wantRetry: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := make(http.Header)
			for k, v := range tt.headers {
				h.Set(k, v)
			}
			m := make(map[string][]string)
			for k, v := range h {
				m[k] = v
			}
			err := MapHTTP(tt.status, []byte(tt.body), m)
			if err.Kind != tt.wantKind {
				t.Errorf("MapHTTP(%d).Kind = %v, want %v", tt.status, err.Kind, tt.wantKind)
			}
			if tt.wantMsgSub != "" && !contains(err.ProviderMsg, tt.wantMsgSub) {
				t.Errorf("MapHTTP(%d).ProviderMsg = %q, want substring %q", tt.status, err.ProviderMsg, tt.wantMsgSub)
			}
			if tt.wantRetry && err.RetryAfter == 0 {
				t.Errorf("MapHTTP(%d).RetryAfter = 0, want > 0", tt.status)
			}
			if !tt.wantRetry && err.RetryAfter != 0 {
				t.Errorf("MapHTTP(%d).RetryAfter = %v, want 0", tt.status, err.RetryAfter)
			}
		})
	}
}

func TestMapHTTP_RetryAfterDuration(t *testing.T) {
	h := (map[string][]string{"Retry-After": {"60"}})
	err := MapHTTP(429, []byte("rate limited"), h)
	if err.RetryAfter != 60*time.Second {
		t.Errorf("RetryAfter = %v, want 60s", err.RetryAfter)
	}
}

func TestMapHTTP_EmptyBody(t *testing.T) {
	err := MapHTTP(500, nil, nil)
	if err.Kind != KindUpstream {
		t.Errorf("Kind = %v, want upstream", err.Kind)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}