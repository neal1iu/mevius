package github

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mevius/internal/domain"
	"mevius/internal/provider"

	gogithub "github.com/google/go-github/v66/github"
)

type GitHubProvider struct {
	baseURL string
}

func NewProvider(baseURL string) *GitHubProvider {
	return &GitHubProvider{baseURL: baseURL}
}

func (p *GitHubProvider) Type() string {
	return "github"
}

func (p *GitHubProvider) ValidateCredentials(ctx context.Context, account *domain.ProviderAccount) (domain.AccountMeta, error) {
	client := p.ghClient(account.TokenEncrypted)
	user, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		return domain.AccountMeta{}, mapError(err)
	}

	scopesHeader := resp.Response.Header.Get("X-OAuth-Scopes")
	scopes := parseScopes(scopesHeader)

	required := map[string]bool{"repo": false, "workflow": false, "delete_repo": false}
	for _, s := range scopes {
		required[s] = true
	}
	var missing []string
	for _, req := range []string{"repo", "workflow", "delete_repo"} {
		if !required[req] {
			missing = append(missing, req)
		}
	}

	login := user.GetLogin()
	meta := domain.AccountMeta{
		AccountID: login,
		Raw: map[string]any{
			"login":  login,
			"scopes": scopes,
		},
	}
	if len(missing) > 0 {
		meta.Raw["missing_scopes"] = missing
	}
	return meta, nil
}

func (p *GitHubProvider) ListExternalResources(ctx context.Context, account *domain.ProviderAccount, kind domain.ResourceKind) ([]domain.ExternalResource, error) {
	if kind != domain.ResourceKindRepo {
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: "github only supports repo resources"}
	}

	client := p.ghClient(account.TokenEncrypted)
	opts := &gogithub.RepositoryListOptions{
		Visibility:  "all",
		Affiliation: "owner",
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}

	var resources []domain.ExternalResource
	for {
		repos, resp, err := client.Repositories.List(ctx, "", opts)
		if err != nil {
			return nil, mapError(err)
		}
		for _, r := range repos {
			resources = append(resources, repoToExternal(r))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return resources, nil
}

func (p *GitHubProvider) GetResource(ctx context.Context, account *domain.ProviderAccount, externalID string) (*domain.ExternalResource, error) {
	owner, repo := splitExternalID(externalID)
	if owner == "" || repo == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid external_id: expected owner/name"}
	}

	client := p.ghClient(account.TokenEncrypted)
	r, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, mapError(err)
	}
	res := repoToExternal(r)
	return &res, nil
}

func (p *GitHubProvider) CreateResource(ctx context.Context, account *domain.ProviderAccount, spec domain.ResourceSpec) (*domain.ExternalResource, error) {
	client := p.ghClient(account.TokenEncrypted)
	ghRepo := &gogithub.Repository{
		Name:        gogithub.String(spec.Name),
		Private:     gogithub.Bool(spec.Private),
		Description: gogithub.String(spec.Description),
	}
	r, _, err := client.Repositories.Create(ctx, "", ghRepo)
	if err != nil {
		return nil, mapError(err)
	}
	res := repoToExternal(r)
	return &res, nil
}

func (p *GitHubProvider) DeleteResource(ctx context.Context, account *domain.ProviderAccount, externalID string) error {
	owner, repo := splitExternalID(externalID)
	if owner == "" || repo == "" {
		return &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid external_id: expected owner/name"}
	}

	client := p.ghClient(account.TokenEncrypted)
	_, err := client.Repositories.Delete(ctx, owner, repo)
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (p *GitHubProvider) ghClient(token string) *gogithub.Client {
	tc := &http.Client{Timeout: 15 * time.Second}
	client := gogithub.NewClient(tc).WithAuthToken(token)
	client.BaseURL = mustParseURL(p.baseURL)
	return client
}

func repoToExternal(r *gogithub.Repository) domain.ExternalResource {
	return domain.ExternalResource{
		ExternalID:  r.GetFullName(),
		DisplayName: r.GetName(),
		Meta: map[string]any{
			"private":        r.GetPrivate(),
			"default_branch": r.GetDefaultBranch(),
			"html_url":       r.GetHTMLURL(),
		},
	}
}

func parseScopes(header string) []string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	var scopes []string
	for _, s := range strings.Split(header, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes
}

func splitExternalID(externalID string) (string, string) {
	parts := strings.SplitN(externalID, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func mapError(err error) *provider.Error {
	var rateLimitErr *gogithub.RateLimitError
	if errors.As(err, &rateLimitErr) {
		retryAfter := time.Duration(0)
		if rateLimitErr.Rate.Reset.Time.After(time.Now()) {
			retryAfter = time.Until(rateLimitErr.Rate.Reset.Time)
		}
		return &provider.Error{
			Kind:        provider.KindRateLimit,
			RetryAfter:  retryAfter,
			ProviderMsg: rateLimitErr.Message,
		}
	}

	var errResp *gogithub.ErrorResponse
	if errors.As(err, &errResp) {
		h := make(map[string][]string)
		for k, v := range errResp.Response.Header {
			h[k] = v
		}
		body := errResp.Message
		if errResp.Response.Body != nil {
			body = errResp.Message
		}
		return provider.MapHTTP(errResp.Response.StatusCode, []byte(body), h)
	}

	return &provider.Error{
		Kind:        provider.KindUpstream,
		ProviderMsg: err.Error(),
	}
}

const defaultBaseURL = "https://api.github.com/"

func mustParseURL(rawURL string) *url.URL {
	if rawURL == "" {
		u, _ := url.Parse(defaultBaseURL)
		return u
	}
	if !strings.HasSuffix(rawURL, "/") {
		rawURL += "/"
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		u, _ = url.Parse(defaultBaseURL)
	}
	return u
}