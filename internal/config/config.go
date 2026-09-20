// Package config loads and validates the mevius runtime configuration from
// environment variables. Validation is fail-fast: any missing or malformed
// value produces an error naming the offending variable.
package config

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	EnvAPIToken  = "MEVIUS_API_TOKEN"
	EnvMasterKey = "MEVIUS_MASTER_KEY"
	EnvDBPath    = "MEVIUS_DB_PATH"
	EnvAddr      = "MEVIUS_ADDR"
	EnvPublicURL = "MEVIUS_PUBLIC_URL"
)

const (
	DefaultDBPath = "./mevius.db"
	DefaultAddr   = ":8080"
)

// masterKeyHexLen is 32 raw bytes rendered as hex, i.e. a 256-bit key.
const masterKeyHexLen = 64

type Config struct {
	// APIToken guards every /api/v1 route except health. Never log it.
	APIToken string

	// MasterKey is the decoded 32-byte envelope encryption key.
	MasterKey []byte

	DBPath       string
	Addr         string
	PublicURL    string
	OAuthClients map[string]OAuthClient
}

type OAuthClient struct {
	ClientID         string
	ClientSecret     string
	AuthorizationURL string
	TokenURL         string
	Scopes           []string
	PKCE             bool
}

func Load() (*Config, error) {
	cfg := &Config{
		DBPath:       DefaultDBPath,
		Addr:         DefaultAddr,
		PublicURL:    strings.TrimRight(os.Getenv(EnvPublicURL), "/"),
		OAuthClients: map[string]OAuthClient{},
	}

	token := os.Getenv(EnvAPIToken)
	if token == "" {
		return nil, fmt.Errorf("%s is required and must not be empty", EnvAPIToken)
	}
	cfg.APIToken = token

	rawKey := os.Getenv(EnvMasterKey)
	if rawKey == "" {
		return nil, fmt.Errorf("%s is required and must not be empty", EnvMasterKey)
	}
	if len(rawKey) != masterKeyHexLen {
		return nil, fmt.Errorf(
			"%s must be exactly %d hexadecimal characters, got %d",
			EnvMasterKey, masterKeyHexLen, len(rawKey),
		)
	}
	key, err := hex.DecodeString(rawKey)
	if err != nil {
		return nil, fmt.Errorf("%s must be valid hexadecimal: %w", EnvMasterKey, err)
	}
	cfg.MasterKey = key

	if v := os.Getenv(EnvDBPath); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv(EnvAddr); v != "" {
		cfg.Addr = v
	}

	loadOAuth := func(provider, prefix, defaultAuthURL, defaultTokenURL string, defaultScopes []string, pkce bool) error {
		clientID := os.Getenv(prefix + "_CLIENT_ID")
		clientSecret := os.Getenv(prefix + "_CLIENT_SECRET")
		if clientID == "" && clientSecret == "" {
			return nil
		}
		if clientID == "" || clientSecret == "" {
			return fmt.Errorf("%s_CLIENT_ID and %s_CLIENT_SECRET must be configured together", prefix, prefix)
		}
		authURL := defaultAuthURL
		if value := os.Getenv(prefix + "_AUTH_URL"); value != "" {
			authURL = value
		}
		tokenURL := defaultTokenURL
		if value := os.Getenv(prefix + "_TOKEN_URL"); value != "" {
			tokenURL = value
		}
		scopes := defaultScopes
		if value := os.Getenv(prefix + "_SCOPES"); value != "" {
			scopes = splitScopes(value)
		}
		cfg.OAuthClients[provider] = OAuthClient{ClientID: clientID, ClientSecret: clientSecret, AuthorizationURL: authURL, TokenURL: tokenURL, Scopes: scopes, PKCE: pkce}
		return nil
	}
	if err := loadOAuth("github", "MEVIUS_GITHUB_OAUTH", "https://github.com/login/oauth/authorize", "https://github.com/login/oauth/access_token", []string{"repo", "workflow", "delete_repo", "read:org"}, true); err != nil {
		return nil, err
	}
	if err := loadOAuth("cloudflare", "MEVIUS_CLOUDFLARE_OAUTH", "https://dash.cloudflare.com/oauth2/auth", "https://dash.cloudflare.com/oauth2/token", nil, true); err != nil {
		return nil, err
	}
	vercelSlug := os.Getenv("MEVIUS_VERCEL_OAUTH_SLUG")
	vercelAuthURL := ""
	if vercelSlug != "" {
		vercelAuthURL = "https://vercel.com/integrations/" + url.PathEscape(vercelSlug) + "/new"
	}
	if err := loadOAuth("vercel", "MEVIUS_VERCEL_OAUTH", vercelAuthURL, "https://api.vercel.com/v2/oauth/access_token", nil, false); err != nil {
		return nil, err
	}
	if _, ok := cfg.OAuthClients["vercel"]; ok && vercelAuthURL == "" && os.Getenv("MEVIUS_VERCEL_OAUTH_AUTH_URL") == "" {
		return nil, fmt.Errorf("MEVIUS_VERCEL_OAUTH_SLUG is required when Vercel OAuth is enabled")
	}
	if len(cfg.OAuthClients) > 0 {
		if cfg.PublicURL == "" {
			return nil, fmt.Errorf("%s is required when OAuth is enabled", EnvPublicURL)
		}
		parsed, err := url.Parse(cfg.PublicURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("%s must be an absolute HTTP(S) URL", EnvPublicURL)
		}
	}

	return cfg, nil
}

func splitScopes(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
}
