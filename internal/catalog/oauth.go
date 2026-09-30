package catalog

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"mevius/internal/config"
	"mevius/internal/crypto"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type OAuthService struct {
	service   *Service
	publicURL string
	clients   map[string]config.OAuthClient
	HTTP      *http.Client
}
type OAuthConfiguration struct {
	ClientID         string   `json:"client_id"`
	ClientSecret     string   `json:"client_secret,omitempty"`
	AuthorizationURL string   `json:"authorization_url"`
	TokenURL         string   `json:"token_url"`
	Scopes           []string `json:"scopes"`
	PKCE             bool     `json:"pkce"`
	RedirectBaseURL  string   `json:"redirect_base_url"`
	IntegrationSlug  string   `json:"integration_slug,omitempty"`
}
type OAuthInfo struct {
	ProviderID    string             `json:"provider_id"`
	Configured    bool               `json:"configured"`
	Configuration OAuthConfiguration `json:"configuration"`
}
type OAuthSession struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"provider_instance_id"`
	Status     string    `json:"status"`
	ErrorCode  string    `json:"error_code,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func NewOAuthService(s *Service, publicURL string, clients map[string]config.OAuthClient) *OAuthService {
	return &OAuthService{service: s, publicURL: strings.TrimRight(publicURL, "/"), clients: clients, HTTP: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func defaultOAuth(providerID string) OAuthConfiguration {
	switch providerID {
	case "github":
		return OAuthConfiguration{AuthorizationURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", Scopes: []string{"repo", "workflow", "delete_repo", "read:org"}, PKCE: true}
	case "cloudflare":
		return OAuthConfiguration{AuthorizationURL: "https://dash.cloudflare.com/oauth2/auth", TokenURL: "https://dash.cloudflare.com/oauth2/token", PKCE: true}
	case "vercel":
		return OAuthConfiguration{TokenURL: "https://api.vercel.com/v2/oauth/access_token", Scopes: []string{}}
	}
	return OAuthConfiguration{Scopes: []string{}}
}
func httpURL(value string) bool {
	u, e := url.Parse(value)
	return e == nil && u.Host != "" && u.User == nil && u.Fragment == "" && (u.Scheme == "https" || u.Scheme == "http")
}
func (s *OAuthService) client(ctx context.Context, providerID string) (OAuthConfiguration, error) {
	v := defaultOAuth(providerID)
	if env, ok := s.clients[providerID]; ok {
		return OAuthConfiguration{ClientID: env.ClientID, ClientSecret: env.ClientSecret, AuthorizationURL: env.AuthorizationURL, TokenURL: env.TokenURL, Scopes: env.Scopes, PKCE: env.PKCE, RedirectBaseURL: s.publicURL}, nil
	}
	var enc, scopes string
	e := s.service.DB.QueryRowContext(ctx, `SELECT client_id,encrypted_client_secret,authorization_url,token_url,scopes_json,pkce,redirect_base_url FROM oauth_client_configuration WHERE provider_id=?`, providerID).Scan(&v.ClientID, &enc, &v.AuthorizationURL, &v.TokenURL, &scopes, &v.PKCE, &v.RedirectBaseURL)
	if e != nil {
		return v, dbError(e)
	}
	v.ClientSecret, e = crypto.Decrypt(s.service.key, enc)
	if e != nil {
		return v, e
	}
	return v, decode(scopes, &v.Scopes)
}
func (s *OAuthService) Providers(ctx context.Context) ([]OAuthInfo, error) {
	out := []OAuthInfo{}
	for id := range s.service.Registry.providers {
		v, e := s.client(ctx, id)
		if e != nil && e != ErrNotFound {
			return nil, e
		}
		v.ClientSecret = ""
		out = append(out, OAuthInfo{ProviderID: id, Configured: e == nil, Configuration: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProviderID < out[j].ProviderID })
	return out, nil
}
func (s *OAuthService) Save(ctx context.Context, id string, v OAuthConfiguration) error {
	if s.service.Registry.providers[id] == nil {
		return ErrInvalid
	}
	d := defaultOAuth(id)
	if v.AuthorizationURL == "" {
		v.AuthorizationURL = d.AuthorizationURL
	}
	if id == "vercel" && v.IntegrationSlug != "" {
		v.AuthorizationURL = "https://vercel.com/integrations/" + url.PathEscape(v.IntegrationSlug) + "/new"
	}
	if v.TokenURL == "" {
		v.TokenURL = d.TokenURL
	}
	if v.RedirectBaseURL == "" {
		v.RedirectBaseURL = s.publicURL
	}
	if v.ClientID == "" || !httpURL(v.AuthorizationURL) || !httpURL(v.TokenURL) || !httpURL(v.RedirectBaseURL) {
		return ErrInvalid
	}
	if v.ClientSecret == "" {
		previous, e := s.client(ctx, id)
		if e != nil {
			return ErrInvalid
		}
		v.ClientSecret = previous.ClientSecret
	}
	if v.Scopes == nil {
		v.Scopes = d.Scopes
	}
	if v.Scopes == nil {
		v.Scopes = []string{}
	}
	now := time.Now().UnixMilli()
	_, e := s.service.DB.ExecContext(ctx, `INSERT INTO oauth_client_configuration VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(provider_id) DO UPDATE SET client_id=excluded.client_id,encrypted_client_secret=excluded.encrypted_client_secret,authorization_url=excluded.authorization_url,token_url=excluded.token_url,scopes_json=excluded.scopes_json,pkce=excluded.pkce,redirect_base_url=excluded.redirect_base_url,updated_at=excluded.updated_at`, uuid.NewString(), id, v.ClientID, crypto.Encrypt(s.service.key, v.ClientSecret), v.AuthorizationURL, v.TokenURL, string(jsonBytes(v.Scopes)), v.PKCE, strings.TrimRight(v.RedirectBaseURL, "/"), now, now)
	return e
}
func (s *OAuthService) Delete(ctx context.Context, id string) error {
	_, e := s.service.DB.ExecContext(ctx, `DELETE FROM oauth_client_configuration WHERE provider_id=?`, id)
	return e
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (s *OAuthService) cleanup(ctx context.Context) error {
	_, e := s.service.DB.ExecContext(ctx, `UPDATE oauth_authorization_session SET encrypted_credential='',encrypted_code_verifier='',status='failed',error_code='expired',updated_at=? WHERE expires_at<? AND status IN ('pending','authorized')`, time.Now().UnixMilli(), time.Now().UnixMilli())
	return e
}
func (s *OAuthService) Start(ctx context.Context, instanceID string) (map[string]any, error) {
	if e := s.cleanup(ctx); e != nil {
		return nil, e
	}
	i, e := s.service.Instance(ctx, instanceID)
	if e != nil {
		return nil, e
	}
	v, e := s.client(ctx, i.ProviderID)
	if e != nil {
		return nil, e
	}
	state, e := randomToken()
	if e != nil {
		return nil, e
	}
	verifier, e := randomToken()
	if e != nil {
		return nil, e
	}
	callback := v.RedirectBaseURL + "/api/v1/connections/oauth/callback/" + i.ProviderID
	now := time.Now()
	sessionID := uuid.NewString()
	_, e = s.service.DB.ExecContext(ctx, `INSERT INTO oauth_authorization_session VALUES(?,?,?,?,?,?,?,?,?,?,?)`, sessionID, state, i.ID, callback, crypto.Encrypt(s.service.key, verifier), "pending", "", "", now.UnixMilli(), now.UnixMilli(), now.Add(10*time.Minute).UnixMilli())
	if e != nil {
		return nil, e
	}
	u, e := url.Parse(v.AuthorizationURL)
	if e != nil {
		return nil, ErrInvalid
	}
	q := u.Query()
	q.Set("client_id", v.ClientID)
	q.Set("redirect_uri", callback)
	q.Set("response_type", "code")
	q.Set("state", state)
	if len(v.Scopes) > 0 {
		q.Set("scope", strings.Join(v.Scopes, " "))
	}
	if v.PKCE {
		hash := sha256.Sum256([]byte(verifier))
		q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(hash[:]))
		q.Set("code_challenge_method", "S256")
	}
	u.RawQuery = q.Encode()
	return map[string]any{"session_id": sessionID, "authorization_url": u.String()}, nil
}
func (s *OAuthService) Session(ctx context.Context, id string) (*OAuthSession, error) {
	if e := s.cleanup(ctx); e != nil {
		return nil, e
	}
	v := &OAuthSession{}
	var expires int64
	e := s.service.DB.QueryRowContext(ctx, `SELECT id,provider_instance_id,status,error_code,expires_at FROM oauth_authorization_session WHERE id=?`, id).Scan(&v.ID, &v.InstanceID, &v.Status, &v.ErrorCode, &expires)
	v.ExpiresAt = timestamp(expires)
	return v, dbError(e)
}
func (s *OAuthService) Callback(ctx context.Context, providerID, state, code string) (string, error) {
	var id, instanceID, redirect, enc, status string
	var expires int64
	e := s.service.DB.QueryRowContext(ctx, `SELECT id,provider_instance_id,redirect_uri,encrypted_code_verifier,status,expires_at FROM oauth_authorization_session WHERE state=?`, state).Scan(&id, &instanceID, &redirect, &enc, &status, &expires)
	if e != nil {
		return "", ErrInvalid
	}
	i, e := s.service.Instance(ctx, instanceID)
	if e != nil || i.ProviderID != providerID || status != "pending" || expires < time.Now().UnixMilli() || code == "" {
		return id, ErrInvalid
	}
	v, e := s.client(ctx, providerID)
	if e != nil {
		return id, e
	}
	params := url.Values{"grant_type": {"authorization_code"}, "client_id": {v.ClientID}, "client_secret": {v.ClientSecret}, "code": {code}, "redirect_uri": {redirect}}
	if v.PKCE {
		verifier, e := crypto.Decrypt(s.service.key, enc)
		if e != nil {
			return id, e
		}
		params.Set("code_verifier", verifier)
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, v.TokenURL, strings.NewReader(params.Encode()))
	if e != nil {
		return id, e
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, e := s.HTTP.Do(req)
	if e != nil {
		return id, s.fail(ctx, id)
	}
	defer resp.Body.Close()
	payload, e := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if e != nil || len(payload) > 1<<20 || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return id, s.fail(ctx, id)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(payload, &token) != nil || token.AccessToken == "" {
		return id, s.fail(ctx, id)
	}
	if _, e = s.service.Registry.providers[i.ProviderID].Validate(ctx, *i, []byte(token.AccessToken)); e != nil {
		return id, s.fail(ctx, id)
	}
	result, e := s.service.DB.ExecContext(ctx, `UPDATE oauth_authorization_session SET encrypted_credential=?,encrypted_code_verifier='',status='authorized',updated_at=? WHERE id=? AND status='pending' AND expires_at>?`, crypto.Encrypt(s.service.key, token.AccessToken), time.Now().UnixMilli(), id, time.Now().UnixMilli())
	if e != nil {
		return id, e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return id, ErrConflict
	}
	return id, nil
}
func (s *OAuthService) fail(ctx context.Context, id string) error {
	_, e := s.service.DB.ExecContext(ctx, `UPDATE oauth_authorization_session SET status='failed',error_code='authorization_failed',encrypted_code_verifier='',encrypted_credential='',updated_at=? WHERE id=?`, time.Now().UnixMilli(), id)
	if e != nil {
		return e
	}
	return ErrForbidden
}
func (s *OAuthService) Complete(ctx context.Context, id, label string) (*Connection, error) {
	session, e := s.Session(ctx, id)
	if e != nil {
		return nil, e
	}
	if session.Status != "authorized" {
		return nil, ErrConflict
	}
	i, e := s.service.Instance(ctx, session.InstanceID)
	if e != nil {
		return nil, e
	}
	var enc string
	if e = s.service.DB.QueryRowContext(ctx, `SELECT encrypted_credential FROM oauth_authorization_session WHERE id=?`, id).Scan(&enc); e != nil {
		return nil, e
	}
	credential, e := crypto.Decrypt(s.service.key, enc)
	if e != nil {
		return nil, e
	}
	auth, e := s.service.Registry.providers[i.ProviderID].Validate(ctx, *i, []byte(credential))
	if e != nil {
		return nil, safeError(e)
	}
	var principal map[string]any
	if e = json.Unmarshal(auth.Principal, &principal); e != nil {
		return nil, e
	}
	public := map[string]any{}
	for _, k := range []string{"id", "login", "name", "email"} {
		if v, ok := principal[k]; ok {
			switch v.(type) {
			case string, float64:
				public[k] = v
			}
		}
	}
	now := time.Now().UTC()
	c := &Connection{ID: uuid.NewString(), InstanceID: i.ID, Label: label, AuthScheme: "oauth", Revision: 1, Principal: Document{1, jsonBytes(public)}, State: "valid", CreatedAt: now, UpdatedAt: now}
	tx, e := s.service.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	res, e := tx.ExecContext(ctx, `UPDATE oauth_authorization_session SET status='consumed',encrypted_credential='',encrypted_code_verifier='',updated_at=? WHERE id=? AND status='authorized' AND expires_at>?`, millis(now), id, millis(now))
	if e != nil {
		return nil, e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, ErrConflict
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO provider_connection VALUES(?,?,?,?,?,?,?,?,?,?,?)`, c.ID, c.InstanceID, c.Label, "oauth", enc, 1, 1, string(c.Principal.Data), "valid", millis(now), millis(now))
	if e != nil {
		return nil, e
	}
	return c, tx.Commit()
}
func (s *OAuthService) ReturnURL(ctx context.Context, id string) string {
	var instanceID string
	if s.service.DB.QueryRowContext(ctx, `SELECT provider_instance_id FROM oauth_authorization_session WHERE id=?`, id).Scan(&instanceID) != nil {
		return s.publicURL + "/accounts"
	}
	i, e := s.service.Instance(ctx, instanceID)
	if e != nil {
		return s.publicURL + "/accounts"
	}
	v, e := s.client(ctx, i.ProviderID)
	if e != nil {
		return s.publicURL + "/accounts"
	}
	return strings.TrimRight(v.RedirectBaseURL, "/") + "/accounts?oauth_session=" + url.QueryEscape(id)
}

func (s *OAuthService) ExpireSessions(ctx context.Context) error { return s.cleanup(ctx) }
