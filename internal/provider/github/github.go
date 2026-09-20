package github

import (
	"context"
	"encoding/json"
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
	repos   *repositoryDriver
	actions *actionsDriver
}

func NewProvider() *GitHubProvider {
	p := &GitHubProvider{}
	p.repos = &repositoryDriver{provider: p}
	p.actions = &actionsDriver{provider: p}
	return p
}

func (p *GitHubProvider) ID() domain.ProviderID                 { return "github" }
func (p *GitHubProvider) Descriptor() domain.ProviderDescriptor { return provider.GitHubDescriptor }
func (p *GitHubProvider) Products() []provider.ProductDriver {
	return []provider.ProductDriver{p.repos, p.actions}
}

func (p *GitHubProvider) Probe(ctx context.Context, endpoint string, credential []byte) (*domain.ProbeResult, error) {
	client := p.ghClient(string(credential), endpoint)
	user, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		return nil, mapError(err)
	}

	scopes := parseScopes(resp.Response.Header.Get("X-OAuth-Scopes"))
	permissions := githubPermissions(scopes)
	identity := map[string]any{"login": user.GetLogin(), "id": user.GetID(), "name": user.GetName(), "scopes": scopes}
	result := &domain.ProbeResult{
		Identity:    identity,
		Permissions: permissions,
		Scopes:      []domain.ProviderScope{{Type: "user", ID: user.GetLogin(), Label: user.GetLogin()}},
	}
	orgs, _, orgErr := client.Organizations.List(ctx, "", &gogithub.ListOptions{PerPage: 100})
	if orgErr == nil {
		for _, org := range orgs {
			result.Scopes = append(result.Scopes, domain.ProviderScope{Type: "org", ID: org.GetLogin(), Label: org.GetLogin()})
		}
	}
	return result, nil
}

func (p *GitHubProvider) ValidateScope(ctx context.Context, endpoint string, credential []byte, scope domain.ProviderScope) (*domain.ProbeResult, error) {
	probe, err := p.Probe(ctx, endpoint, credential)
	if err != nil {
		return nil, err
	}
	for _, candidate := range probe.Scopes {
		if candidate.Type == scope.Type && candidate.ID == scope.ID {
			probe.Scopes = []domain.ProviderScope{candidate}
			return probe, nil
		}
	}
	return nil, &provider.Error{Kind: provider.KindUnauthorized, ProviderMsg: "selected GitHub scope is not accessible"}
}

type repositoryDriver struct{ provider *GitHubProvider }

func (d *repositoryDriver) Descriptor() domain.ProductDescriptor {
	return product(provider.GitHubDescriptor, "github.repositories")
}

func (d *repositoryDriver) Discover(ctx context.Context, conn *domain.ProviderConnection, credential []byte, _ domain.DiscoveryScope) ([]domain.ExternalResource, error) {
	client := d.provider.ghClient(string(credential), conn.Endpoint)
	var resources []domain.ExternalResource
	if conn.Scope.Type == "org" {
		opts := &gogithub.RepositoryListByOrgOptions{Type: "all", ListOptions: gogithub.ListOptions{PerPage: 100}}
		for {
			repos, resp, err := client.Repositories.ListByOrg(ctx, conn.Scope.ID, opts)
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
	opts := &gogithub.RepositoryListOptions{Visibility: "all", Affiliation: "owner", ListOptions: gogithub.ListOptions{PerPage: 100}}
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

func (d *repositoryDriver) Inspect(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.ExternalResource, error) {
	owner, repo := splitExternalID(instance.ExternalID)
	if owner == "" || repo == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid repository external id"}
	}
	r, _, err := d.provider.ghClient(string(credential), conn.Endpoint).Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, mapError(err)
	}
	res := repoToExternal(r)
	return &res, nil
}

func (d *repositoryDriver) Create(ctx context.Context, conn *domain.ProviderConnection, credential []byte, req domain.CreateResourceRequest) (*domain.ExternalResource, error) {
	var spec domain.RepoSpec
	if err := json.Unmarshal(req.Spec, &spec); err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid repo spec"}
	}
	if err := spec.Validate(); err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
	}
	repo := &gogithub.Repository{Name: gogithub.String(spec.Name), Private: gogithub.Bool(spec.Private), Description: gogithub.String(spec.Description)}
	owner := ""
	if conn.Scope.Type == "org" {
		owner = conn.Scope.ID
	}
	r, _, err := d.provider.ghClient(string(credential), conn.Endpoint).Repositories.Create(ctx, owner, repo)
	if err != nil {
		return nil, mapError(err)
	}
	res := repoToExternal(r)
	return &res, nil
}

func (d *repositoryDriver) Delete(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) error {
	owner, repo := splitExternalID(instance.ExternalID)
	if owner == "" || repo == "" {
		return &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid repository external id"}
	}
	_, err := d.provider.ghClient(string(credential), conn.Endpoint).Repositories.Delete(ctx, owner, repo)
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (d *repositoryDriver) RecoverCreate(ctx context.Context, conn *domain.ProviderConnection, credential []byte, req domain.CreateResourceRequest) (*domain.ExternalResource, error) {
	var spec domain.RepoSpec
	if json.Unmarshal(req.Spec, &spec) != nil || spec.Name == "" {
		return nil, nil
	}
	owner := conn.Scope.ID
	r, _, err := d.provider.ghClient(string(credential), conn.Endpoint).Repositories.Get(ctx, owner, spec.Name)
	if err != nil {
		if mapError(err).Kind == provider.KindNotFound {
			return nil, nil
		}
		return nil, mapError(err)
	}
	res := repoToExternal(r)
	return &res, nil
}

func (p *GitHubProvider) ghClient(token, endpoint string) *gogithub.Client {
	tc := &http.Client{Timeout: 15 * time.Second}
	client := gogithub.NewClient(tc).WithAuthToken(token)
	client.BaseURL = mustParseURL(endpoint)
	return client
}

func repoToExternal(r *gogithub.Repository) domain.ExternalResource {
	spec, _ := json.Marshal(domain.RepoSpec{Name: r.GetName(), Private: r.GetPrivate(), Description: r.GetDescription()})
	return domain.ExternalResource{ExternalID: r.GetFullName(), ExternalURL: r.GetHTMLURL(), DisplayName: r.GetName(), Spec: spec,
		ProviderConfig: json.RawMessage(`{"version":1}`), Meta: map[string]any{"private": r.GetPrivate(), "default_branch": r.GetDefaultBranch(), "html_url": r.GetHTMLURL()}}
}

func githubPermissions(scopes []string) map[domain.Capability]domain.CapabilityState {
	has := map[string]bool{}
	for _, s := range scopes {
		has[s] = true
	}
	state := func(ok bool, reason string) domain.CapabilityState {
		if len(scopes) == 0 {
			return domain.CapabilityState{Availability: domain.CapabilityUnknown, Reason: "token permissions are not exposed"}
		}
		if ok {
			return domain.CapabilityState{Availability: domain.CapabilityAvailable}
		}
		return domain.CapabilityState{Availability: domain.CapabilityUnavailable, Reason: reason}
	}
	return map[domain.Capability]domain.CapabilityState{
		domain.CapDiscover:        state(has["repo"] || has["public_repo"], "repo or public_repo scope required"),
		domain.CapInspect:         state(has["repo"] || has["public_repo"], "repo or public_repo scope required"),
		domain.CapCreate:          state(has["repo"] || has["public_repo"], "repo or public_repo scope required"),
		domain.CapDelete:          state(has["delete_repo"], "delete_repo scope required"),
		domain.CapPipelineTrigger: state(has["workflow"], "workflow scope required"),
		domain.CapPipelineRuns:    state(has["repo"] || has["public_repo"], "repo scope required"),
		domain.CapPipelineCancel:  state(has["workflow"], "workflow scope required"),
		domain.CapPipelineRerun:   state(has["workflow"], "workflow scope required"),
		domain.CapLogs:            state(has["repo"] || has["public_repo"], "repo scope required"),
	}
}

func parseScopes(header string) []string {
	var out []string
	for _, s := range strings.Split(strings.TrimSpace(header), ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
func splitExternalID(id string) (string, string) {
	p := strings.SplitN(id, "/", 2)
	if len(p) != 2 {
		return "", ""
	}
	return p[0], p[1]
}

func product(desc domain.ProviderDescriptor, id domain.ProductID) domain.ProductDescriptor {
	for _, p := range desc.Products {
		if p.ID == id {
			return p
		}
	}
	panic("product not found")
}

func mapError(err error) *provider.Error {
	var rate *gogithub.RateLimitError
	if errors.As(err, &rate) {
		var retry time.Duration
		if rate.Rate.Reset.Time.After(time.Now()) {
			retry = time.Until(rate.Rate.Reset.Time)
		}
		return &provider.Error{Kind: provider.KindRateLimit, RetryAfter: retry, ProviderMsg: rate.Message}
	}
	var response *gogithub.ErrorResponse
	if errors.As(err, &response) {
		headers := map[string][]string{}
		for k, v := range response.Response.Header {
			headers[k] = v
		}
		return provider.MapHTTP(response.Response.StatusCode, []byte(response.Message), headers)
	}
	return &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
}

const defaultBaseURL = "https://api.github.com/"

func mustParseURL(raw string) *url.URL {
	if raw == "" {
		raw = defaultBaseURL
	}
	if !strings.HasSuffix(raw, "/") {
		raw += "/"
	}
	u, err := url.Parse(raw)
	if err != nil {
		u, _ = url.Parse(defaultBaseURL)
	}
	return u
}
