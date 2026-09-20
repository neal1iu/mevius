package config

import (
	"strings"
	"testing"
)

const testMasterKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func clearOAuthEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		EnvPublicURL,
		"MEVIUS_GITHUB_OAUTH_CLIENT_ID", "MEVIUS_GITHUB_OAUTH_CLIENT_SECRET", "MEVIUS_GITHUB_OAUTH_AUTH_URL", "MEVIUS_GITHUB_OAUTH_TOKEN_URL", "MEVIUS_GITHUB_OAUTH_SCOPES",
		"MEVIUS_CLOUDFLARE_OAUTH_CLIENT_ID", "MEVIUS_CLOUDFLARE_OAUTH_CLIENT_SECRET", "MEVIUS_CLOUDFLARE_OAUTH_AUTH_URL", "MEVIUS_CLOUDFLARE_OAUTH_TOKEN_URL", "MEVIUS_CLOUDFLARE_OAUTH_SCOPES",
		"MEVIUS_VERCEL_OAUTH_CLIENT_ID", "MEVIUS_VERCEL_OAUTH_CLIENT_SECRET", "MEVIUS_VERCEL_OAUTH_AUTH_URL", "MEVIUS_VERCEL_OAUTH_TOKEN_URL", "MEVIUS_VERCEL_OAUTH_SCOPES", "MEVIUS_VERCEL_OAUTH_SLUG",
	} {
		t.Setenv(key, "")
	}
	t.Setenv(EnvAPIToken, "api-token")
	t.Setenv(EnvMasterKey, testMasterKey)
}

func TestLoadWithoutOAuthRemainsSupported(t *testing.T) {
	clearOAuthEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.OAuthClients) != 0 || cfg.PublicURL != "" {
		t.Fatalf("unexpected OAuth config: %+v", cfg)
	}
}

func TestLoadOAuthRequiresCompleteClientAndPublicURL(t *testing.T) {
	clearOAuthEnvironment(t)
	t.Setenv("MEVIUS_GITHUB_OAUTH_CLIENT_ID", "client")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "must be configured together") {
		t.Fatalf("expected paired credential error, got %v", err)
	}

	t.Setenv("MEVIUS_GITHUB_OAUTH_CLIENT_SECRET", "secret")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), EnvPublicURL) {
		t.Fatalf("expected public URL error, got %v", err)
	}

	t.Setenv(EnvPublicURL, "ftp://mevius.example")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "HTTP(S)") {
		t.Fatalf("expected HTTP URL error, got %v", err)
	}
}

func TestLoadOAuthProviderDefaultsAndOverrides(t *testing.T) {
	clearOAuthEnvironment(t)
	t.Setenv(EnvPublicURL, "https://mevius.example/")
	t.Setenv("MEVIUS_GITHUB_OAUTH_CLIENT_ID", "client")
	t.Setenv("MEVIUS_GITHUB_OAUTH_CLIENT_SECRET", "secret")
	t.Setenv("MEVIUS_GITHUB_OAUTH_SCOPES", "repo, workflow")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	client := cfg.OAuthClients["github"]
	if cfg.PublicURL != "https://mevius.example" || !client.PKCE || client.AuthorizationURL != "https://github.com/login/oauth/authorize" {
		t.Fatalf("unexpected GitHub OAuth defaults: public=%q client=%+v", cfg.PublicURL, client)
	}
	if len(client.Scopes) != 2 || client.Scopes[0] != "repo" || client.Scopes[1] != "workflow" {
		t.Fatalf("unexpected scopes: %#v", client.Scopes)
	}
}

func TestLoadVercelOAuthRequiresSlug(t *testing.T) {
	clearOAuthEnvironment(t)
	t.Setenv(EnvPublicURL, "https://mevius.example")
	t.Setenv("MEVIUS_VERCEL_OAUTH_CLIENT_ID", "client")
	t.Setenv("MEVIUS_VERCEL_OAUTH_CLIENT_SECRET", "secret")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SLUG") {
		t.Fatalf("expected Vercel slug error, got %v", err)
	}
}
