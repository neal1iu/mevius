package service

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"mevius/internal/config"
	"mevius/internal/crypto"
	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

const oauthSessionLifetime = 10 * time.Minute

type OAuthService struct {
	q           *store.Queries
	key         [32]byte
	registry    *provider.Registry
	connections *ConnectionService
	publicURL   string
	clients     map[string]config.OAuthClient
	httpClient  *http.Client
}

type OAuthStartResult struct {
	AuthorizationURL string `json:"authorization_url"`
}

type OAuthClientConfigurationInput struct {
	ClientID         string   `json:"client_id"`
	ClientSecret     string   `json:"client_secret,omitempty"`
	AuthorizationURL string   `json:"authorization_url,omitempty"`
	TokenURL         string   `json:"token_url,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	PKCE             *bool    `json:"pkce,omitempty"`
	RedirectBaseURL  string   `json:"redirect_base_url"`
	IntegrationSlug  string   `json:"integration_slug,omitempty"`
}

type oauthProviderTemplate struct {
	AuthorizationURL string
	TokenURL         string
	Scopes           []string
	PKCE             bool
	RequiresSlug     bool
}

var oauthProviderTemplates = map[domain.ProviderID]oauthProviderTemplate{
	"github":     {AuthorizationURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", Scopes: []string{"repo", "workflow", "delete_repo", "read:org"}, PKCE: true},
	"cloudflare": {AuthorizationURL: "https://dash.cloudflare.com/oauth2/auth", TokenURL: "https://dash.cloudflare.com/oauth2/token", PKCE: true},
	"vercel":     {TokenURL: "https://api.vercel.com/v2/oauth/access_token", RequiresSlug: true},
}

type CompleteOAuthInput struct {
	SessionID string               `json:"session_id"`
	Label     string               `json:"label"`
	Scope     domain.ProviderScope `json:"scope"`
	Config    map[string]any       `json:"config,omitempty"`
}

func NewOAuthService(q *store.Queries, key [32]byte, registry *provider.Registry, connections *ConnectionService, publicURL string, clients map[string]config.OAuthClient) *OAuthService {
	return &OAuthService{q: q, key: key, registry: registry, connections: connections, publicURL: strings.TrimRight(publicURL, "/"), clients: clients, httpClient: &http.Client{Timeout: 15 * time.Second}}
}

func (s *OAuthService) Providers(ctx context.Context) ([]domain.OAuthProviderInfo, error) {
	result := make([]domain.OAuthProviderInfo, 0, len(s.clients))
	for _, descriptor := range s.registry.Descriptors() {
		template := oauthProviderTemplates[descriptor.ID]
		info := domain.OAuthProviderInfo{ProviderID: descriptor.ID, AuthorizationURL: template.AuthorizationURL, TokenURL: template.TokenURL, Scopes: append([]string(nil), template.Scopes...), PKCE: template.PKCE, RequiresSlug: template.RequiresSlug}
		client, redirectBaseURL, source, providerConfig, err := s.resolveClient(ctx, descriptor.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if err == nil {
			info.Available = true
			info.Configured = true
			info.Source = source
			info.ClientID = client.ClientID
			info.AuthorizationURL = client.AuthorizationURL
			info.TokenURL = client.TokenURL
			info.Scopes = append([]string(nil), client.Scopes...)
			info.PKCE = client.PKCE
			info.RedirectBaseURL = redirectBaseURL
			info.CallbackURL = oauthCallbackURL(redirectBaseURL, descriptor.ID)
			info.IntegrationSlug, _ = providerConfig["integration_slug"].(string)
		}
		result = append(result, info)
	}
	return result, nil
}

func (s *OAuthService) SaveConfiguration(ctx context.Context, providerID domain.ProviderID, input OAuthClientConfigurationInput) (domain.OAuthProviderInfo, error) {
	if s.registry.Provider(providerID) == nil {
		return domain.OAuthProviderInfo{}, fmt.Errorf("%w: unknown provider", ErrInvalid)
	}
	template, supported := oauthProviderTemplates[providerID]
	if !supported {
		return domain.OAuthProviderInfo{}, fmt.Errorf("%w: OAuth configuration is not supported for %s", ErrUnsupported, providerID)
	}
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.RedirectBaseURL = strings.TrimRight(strings.TrimSpace(input.RedirectBaseURL), "/")
	input.IntegrationSlug = strings.TrimSpace(input.IntegrationSlug)
	if input.ClientID == "" || input.RedirectBaseURL == "" {
		return domain.OAuthProviderInfo{}, fmt.Errorf("%w: client_id and redirect_base_url are required", ErrInvalid)
	}
	if err := validateAbsoluteHTTPURL(input.RedirectBaseURL); err != nil {
		return domain.OAuthProviderInfo{}, fmt.Errorf("%w: redirect_base_url %v", ErrInvalid, err)
	}
	authorizationURL := strings.TrimSpace(input.AuthorizationURL)
	if authorizationURL == "" {
		authorizationURL = template.AuthorizationURL
	}
	if template.RequiresSlug && authorizationURL == "" {
		if input.IntegrationSlug == "" {
			return domain.OAuthProviderInfo{}, fmt.Errorf("%w: integration_slug is required for %s", ErrInvalid, providerID)
		}
		authorizationURL = "https://vercel.com/integrations/" + url.PathEscape(input.IntegrationSlug) + "/new"
	}
	tokenURL := strings.TrimSpace(input.TokenURL)
	if tokenURL == "" {
		tokenURL = template.TokenURL
	}
	if err := validateAbsoluteHTTPURL(authorizationURL); err != nil {
		return domain.OAuthProviderInfo{}, fmt.Errorf("%w: authorization_url %v", ErrInvalid, err)
	}
	if err := validateAbsoluteHTTPURL(tokenURL); err != nil {
		return domain.OAuthProviderInfo{}, fmt.Errorf("%w: token_url %v", ErrInvalid, err)
	}
	scopes := input.Scopes
	if scopes == nil {
		scopes = template.Scopes
	}
	secret := input.ClientSecret
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if existing, err := s.q.GetOAuthClientConfiguration(ctx, string(providerID)); err == nil {
		createdAt = existing.CreatedAt
		if secret == "" {
			secret, err = crypto.Decrypt(s.key, existing.EncryptedClientSecret)
			if err != nil {
				return domain.OAuthProviderInfo{}, err
			}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return domain.OAuthProviderInfo{}, err
	}
	if secret == "" {
		return domain.OAuthProviderInfo{}, fmt.Errorf("%w: client_secret is required for a new OAuth configuration", ErrInvalid)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	pkce := template.PKCE
	if input.PKCE != nil {
		pkce = *input.PKCE
	}
	providerConfig := map[string]any{}
	if input.IntegrationSlug != "" {
		providerConfig["integration_slug"] = input.IntegrationSlug
	}
	err := s.q.UpsertOAuthClientConfiguration(ctx, store.UpsertOAuthClientConfigurationParams{ProviderID: string(providerID), ClientID: input.ClientID, EncryptedClientSecret: crypto.Encrypt(s.key, secret), AuthorizationUrl: authorizationURL, TokenUrl: tokenURL, ScopesJson: encode(scopes), Pkce: boolInt(pkce), RedirectBaseUrl: input.RedirectBaseURL, ProviderConfigJson: encode(providerConfig), CreatedAt: createdAt, UpdatedAt: now})
	if err != nil {
		return domain.OAuthProviderInfo{}, err
	}
	providers, err := s.Providers(ctx)
	if err != nil {
		return domain.OAuthProviderInfo{}, err
	}
	for _, info := range providers {
		if info.ProviderID == providerID {
			return info, nil
		}
	}
	return domain.OAuthProviderInfo{}, ErrNotFound
}

func (s *OAuthService) DeleteConfiguration(ctx context.Context, providerID domain.ProviderID) error {
	affected, err := s.q.DeleteOAuthClientConfiguration(ctx, string(providerID))
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *OAuthService) Start(ctx context.Context, providerID domain.ProviderID, endpoint string) (*OAuthStartResult, error) {
	client, redirectBaseURL, _, _, err := s.resolveClient(ctx, providerID)
	if errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("%w: OAuth is not configured for %s", ErrUnsupported, providerID)
	}
	if err != nil {
		return nil, err
	}
	if s.registry.Provider(providerID) == nil {
		return nil, fmt.Errorf("%w: unknown provider", ErrInvalid)
	}
	state, err := randomURLToken(32)
	if err != nil {
		return nil, err
	}
	verifier := ""
	challenge := ""
	if client.PKCE {
		verifier, err = randomURLToken(48)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256([]byte(verifier))
		challenge = base64.RawURLEncoding.EncodeToString(digest[:])
	}
	redirectURI := oauthCallbackURL(redirectBaseURL, providerID)
	authorizationURL, err := url.Parse(client.AuthorizationURL)
	if err != nil {
		return nil, fmt.Errorf("invalid OAuth authorization URL: %w", err)
	}
	query := authorizationURL.Query()
	query.Set("client_id", client.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("state", state)
	if len(client.Scopes) > 0 {
		query.Set("scope", strings.Join(client.Scopes, " "))
	}
	if challenge != "" {
		query.Set("code_challenge", challenge)
		query.Set("code_challenge_method", "S256")
	}
	authorizationURL.RawQuery = query.Encode()
	now := time.Now().UTC()
	expires := now.Add(oauthSessionLifetime)
	encryptedVerifier := ""
	if verifier != "" {
		encryptedVerifier = crypto.Encrypt(s.key, verifier)
	}
	_ = s.q.DeleteExpiredOAuthSessions(ctx, now.Format(time.RFC3339))
	err = s.q.InsertOAuthAuthorizationSession(ctx, store.InsertOAuthAuthorizationSessionParams{ID: uuid.NewString(), State: state, ProviderID: string(providerID), Endpoint: endpoint, RedirectUri: redirectURI, EncryptedCodeVerifier: encryptedVerifier, Status: "pending", EncryptedCredential: "", TokenMetaJson: "{}", RemoteIdentityJson: "{}", ScopesJson: "[]", PermissionsJson: "{}", ErrorJson: "{}", CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), ExpiresAt: expires.Format(time.RFC3339)})
	if err != nil {
		return nil, err
	}
	return &OAuthStartResult{AuthorizationURL: authorizationURL.String()}, nil
}

func (s *OAuthService) Callback(ctx context.Context, providerID domain.ProviderID, state, code, providerError string) (string, error) {
	row, err := s.q.GetOAuthAuthorizationSessionByState(ctx, state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: OAuth state is invalid", ErrInvalid)
	}
	if err != nil {
		return "", err
	}
	if row.ProviderID != string(providerID) || row.Status != "pending" || row.ExpiresAt <= time.Now().UTC().Format(time.RFC3339) {
		return row.ID, fmt.Errorf("%w: OAuth session is invalid or expired", ErrConflict)
	}
	if providerError != "" {
		err = fmt.Errorf("provider denied authorization: %s", providerError)
		s.fail(ctx, row.ID, err)
		return row.ID, err
	}
	if code == "" {
		err = fmt.Errorf("%w: OAuth callback code is required", ErrInvalid)
		s.fail(ctx, row.ID, err)
		return row.ID, err
	}
	credential, meta, err := s.exchange(ctx, providerID, row, code)
	if err != nil {
		s.fail(ctx, row.ID, err)
		return row.ID, err
	}
	p := s.registry.Provider(providerID)
	probe, err := p.Probe(ctx, row.Endpoint, []byte(credential.AccessToken))
	if err != nil {
		s.fail(ctx, row.ID, err)
		return row.ID, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	affected, err := s.q.AuthorizeOAuthSession(ctx, store.AuthorizeOAuthSessionParams{EncryptedCredential: crypto.Encrypt(s.key, encode(credential)), TokenMetaJson: encode(meta), RemoteIdentityJson: encode(probe.Identity), ScopesJson: encode(probe.Scopes), PermissionsJson: encode(probe.Permissions), UpdatedAt: now, ID: row.ID, ExpiresAt: now})
	if err != nil {
		return row.ID, err
	}
	if affected == 0 {
		return row.ID, fmt.Errorf("%w: OAuth session expired while authorizing", ErrConflict)
	}
	return row.ID, nil
}

func (s *OAuthService) GetSession(ctx context.Context, id string) (domain.OAuthAuthorizationSession, error) {
	row, err := s.q.GetOAuthAuthorizationSession(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OAuthAuthorizationSession{}, ErrNotFound
	}
	if err != nil {
		return domain.OAuthAuthorizationSession{}, err
	}
	result := domain.OAuthAuthorizationSession{ID: row.ID, ProviderID: domain.ProviderID(row.ProviderID), Endpoint: row.Endpoint, Status: row.Status, Identity: decode[map[string]any](row.RemoteIdentityJson), Scopes: decode[[]domain.ProviderScope](row.ScopesJson), Permissions: decode[map[domain.Capability]domain.CapabilityState](row.PermissionsJson), ExpiresAt: row.ExpiresAt}
	if row.Status == "failed" {
		result.Error = decode[map[string]string](row.ErrorJson)["message"]
	}
	return result, nil
}

func (s *OAuthService) FrontendReturnURL(ctx context.Context, sessionID string, callbackErr error) string {
	baseURL := s.publicURL
	if sessionID != "" {
		if row, err := s.q.GetOAuthAuthorizationSession(ctx, sessionID); err == nil {
			suffix := "/api/v1/connections/oauth/callback/" + url.PathEscape(row.ProviderID)
			baseURL = strings.TrimSuffix(row.RedirectUri, suffix)
		}
	}
	target, _ := url.Parse(strings.TrimRight(baseURL, "/") + "/accounts")
	query := target.Query()
	if sessionID != "" {
		query.Set("oauth_session", sessionID)
	}
	if callbackErr != nil {
		query.Set("oauth_error", callbackErr.Error())
	}
	target.RawQuery = query.Encode()
	return target.String()
}

func (s *OAuthService) Complete(ctx context.Context, input CompleteOAuthInput) (*domain.ProviderConnection, error) {
	if input.SessionID == "" || input.Scope.ID == "" {
		return nil, fmt.Errorf("%w: session_id and scope are required", ErrInvalid)
	}
	row, err := s.q.GetOAuthAuthorizationSession(ctx, input.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if row.Status != "authorized" || row.ExpiresAt <= now {
		return nil, fmt.Errorf("%w: OAuth session is not authorized or has expired", ErrConflict)
	}
	decrypted, err := crypto.Decrypt(s.key, row.EncryptedCredential)
	if err != nil {
		return nil, err
	}
	var credential domain.OAuthCredential
	if err = json.Unmarshal([]byte(decrypted), &credential); err != nil || credential.AccessToken == "" {
		return nil, fmt.Errorf("invalid stored OAuth credential")
	}
	meta := decode[map[string]any](row.TokenMetaJson)
	connection, err := s.connections.createValidated(ctx, CreateConnectionInput{ProviderID: domain.ProviderID(row.ProviderID), Label: input.Label, Endpoint: row.Endpoint, Scope: input.Scope, Config: input.Config}, []byte(credential.AccessToken), domain.AuthMethodOAuth, row.EncryptedCredential, meta)
	if err != nil {
		return nil, err
	}
	affected, err := s.q.ConsumeOAuthSession(ctx, store.ConsumeOAuthSessionParams{ConsumedAt: &now, UpdatedAt: now, ID: row.ID, ExpiresAt: now})
	if err != nil || affected == 0 {
		_, _ = s.q.DeleteProviderConnection(ctx, connection.ID)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: OAuth session was already consumed", ErrConflict)
	}
	return connection, nil
}

func (s *OAuthService) exchange(ctx context.Context, providerID domain.ProviderID, row store.OauthAuthorizationSession, code string) (domain.OAuthCredential, map[string]any, error) {
	client, _, _, _, err := s.resolveClient(ctx, providerID)
	if err != nil {
		return domain.OAuthCredential{}, nil, err
	}
	form := url.Values{"client_id": {client.ClientID}, "client_secret": {client.ClientSecret}, "code": {code}, "redirect_uri": {row.RedirectUri}, "grant_type": {"authorization_code"}}
	if row.EncryptedCodeVerifier != "" {
		verifier, err := crypto.Decrypt(s.key, row.EncryptedCodeVerifier)
		if err != nil {
			return domain.OAuthCredential{}, nil, err
		}
		form.Set("code_verifier", verifier)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return domain.OAuthCredential{}, nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := s.httpClient.Do(request)
	if err != nil {
		return domain.OAuthCredential{}, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return domain.OAuthCredential{}, nil, err
	}
	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return domain.OAuthCredential{}, nil, fmt.Errorf("OAuth token endpoint returned an invalid response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := raw["error_description"].(string)
		if message == "" {
			message, _ = raw["error"].(string)
		}
		return domain.OAuthCredential{}, nil, fmt.Errorf("OAuth token exchange failed: %s", message)
	}
	accessToken, _ := raw["access_token"].(string)
	if accessToken == "" {
		return domain.OAuthCredential{}, nil, fmt.Errorf("OAuth token response did not include access_token")
	}
	credential := domain.OAuthCredential{AccessToken: accessToken}
	credential.RefreshToken, _ = raw["refresh_token"].(string)
	credential.TokenType, _ = raw["token_type"].(string)
	credential.Scope, _ = raw["scope"].(string)
	if seconds, ok := number(raw["expires_in"]); ok {
		credential.ExpiresAt = time.Now().UTC().Add(time.Duration(seconds) * time.Second).Format(time.RFC3339)
	}
	delete(raw, "access_token")
	delete(raw, "refresh_token")
	if credential.ExpiresAt != "" {
		raw["expires_at"] = credential.ExpiresAt
	}
	return credential, raw, nil
}

func (s *OAuthService) resolveClient(ctx context.Context, providerID domain.ProviderID) (config.OAuthClient, string, string, map[string]any, error) {
	row, err := s.q.GetOAuthClientConfiguration(ctx, string(providerID))
	if err == nil {
		secret, decryptErr := crypto.Decrypt(s.key, row.EncryptedClientSecret)
		if decryptErr != nil {
			return config.OAuthClient{}, "", "", nil, decryptErr
		}
		return config.OAuthClient{ClientID: row.ClientID, ClientSecret: secret, AuthorizationURL: row.AuthorizationUrl, TokenURL: row.TokenUrl, Scopes: decode[[]string](row.ScopesJson), PKCE: row.Pkce != 0}, row.RedirectBaseUrl, "database", decode[map[string]any](row.ProviderConfigJson), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return config.OAuthClient{}, "", "", nil, err
	}
	client, ok := s.clients[string(providerID)]
	if !ok {
		return config.OAuthClient{}, "", "", nil, ErrNotFound
	}
	providerConfig := map[string]any{}
	if providerID == "vercel" {
		parts := strings.Split(strings.Trim(client.AuthorizationURL, "/"), "/")
		if len(parts) >= 2 {
			providerConfig["integration_slug"] = parts[len(parts)-2]
		}
	}
	return client, s.publicURL, "environment", providerConfig, nil
}

func oauthCallbackURL(baseURL string, providerID domain.ProviderID) string {
	return strings.TrimRight(baseURL, "/") + "/api/v1/connections/oauth/callback/" + url.PathEscape(string(providerID))
}

func validateAbsoluteHTTPURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("must be an absolute HTTP(S) URL")
	}
	return nil
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func (s *OAuthService) fail(ctx context.Context, id string, err error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.q.FailOAuthSession(ctx, store.FailOAuthSessionParams{ErrorJson: encode(map[string]string{"message": err.Error()}), UpdatedAt: now, ID: id})
}
func randomURLToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := cryptorand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
func number(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}
