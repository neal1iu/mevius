// Package cloudflare implements Cloudflare provider integration.
//
// SPIKE CONCLUSION: CF Pages Build Logs
// The Cloudflare Pages REST API does not expose raw build log output (stdout/stderr)
// via any public REST endpoint. The deployment detail endpoint provides stage-level
// status and timing metadata but not the actual log lines visible in the Cloudflare
// Dashboard UI. Therefore GetBuildLogs uses a documented fallback: it summarizes
// deployment stages as structured text and provides a dashboard deep-link URL in
// LogChunk.Meta so consumers can open the full log view in the Cloudflare Dashboard.
// This fallback is intentional and not a temporary workaround — no fake log
// capability is synthesized.
package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

type cfPagesStage struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	StartedAt *string `json:"started_at,omitempty"`
	EndedAt   *string `json:"ended_at,omitempty"`
}

type cfPagesDeployment struct {
	ID         string          `json:"id"`
	Status     string          `json:"status"`
	CreatedOn  string          `json:"created_on"`
	ModifiedOn string          `json:"modified_on"`
	Stages     []cfPagesStage  `json:"stages,omitempty"`
}

type cfPagesSource struct {
	Type   string           `json:"type"`
	Config *cfPagesGitConfig `json:"config,omitempty"`
}

type cfPagesGitConfig struct {
	Owner            string `json:"owner"`
	RepoName         string `json:"repo_name"`
	ProductionBranch string `json:"production_branch"`
}

type cfPagesProject struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	CreatedOn        string               `json:"created_on"`
	ModifiedOn       string               `json:"modified_on"`
	Source           *cfPagesSource       `json:"source,omitempty"`
	LatestDeployment *cfPagesDeployment   `json:"latest_deployment,omitempty"`
	Subdomain        string               `json:"subdomain,omitempty"`
}

type cfPagesCreateReq struct {
	Name             string `json:"name"`
	ProductionBranch string `json:"production_branch"`
}

func (p *CloudflareProvider) listPagesProjects(ctx context.Context, token, accountID string) ([]cfPagesProject, error) {
	body, err := p.doGet(ctx, token, "/accounts/"+accountID+"/pages/projects")
	if err != nil {
		return nil, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: fmt.Sprintf("parse pages projects list: %v", err)}
	}
	if !resp.Success {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: errorMsg(resp.Errors)}
	}
	var projects []cfPagesProject
	if err := json.Unmarshal(resp.Result, &projects); err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "parse pages projects result"}
	}
	return projects, nil
}

func (p *CloudflareProvider) getPagesProject(ctx context.Context, token, accountID, name string) (*cfPagesProject, error) {
	body, err := p.doGet(ctx, token, "/accounts/"+accountID+"/pages/projects/"+name)
	if err != nil {
		return nil, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: fmt.Sprintf("parse pages project detail: %v", err)}
	}
	if !resp.Success {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: errorMsg(resp.Errors)}
	}
	var project cfPagesProject
	if err := json.Unmarshal(resp.Result, &project); err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "parse pages project result"}
	}
	return &project, nil
}

func (p *CloudflareProvider) createPagesProject(ctx context.Context, token, accountID, name, branch string) (*cfPagesProject, error) {
	req := cfPagesCreateReq{Name: name, ProductionBranch: branch}
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: fmt.Sprintf("marshal create req: %v", err)}
	}
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}
	body, err := p.cfClient(token).DoReq(ctx, "POST", "/accounts/"+accountID+"/pages/projects", bodyBytes, headers)
	if err != nil {
		return nil, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: fmt.Sprintf("parse create pages project: %v", err)}
	}
	if !resp.Success {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: errorMsg(resp.Errors)}
	}
	var project cfPagesProject
	if err := json.Unmarshal(resp.Result, &project); err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "parse create pages project result"}
	}
	return &project, nil
}

func (p *CloudflareProvider) deletePagesProject(ctx context.Context, token, accountID, name string) error {
	_, err := p.doDelete(ctx, token, "/accounts/"+accountID+"/pages/projects/"+name)
	return err
}

func (p *CloudflareProvider) listPagesDeployments(ctx context.Context, token, accountID, projectName string) ([]cfPagesDeployment, error) {
	body, err := p.doGet(ctx, token, "/accounts/"+accountID+"/pages/projects/"+projectName+"/deployments")
	if err != nil {
		return nil, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: fmt.Sprintf("parse pages deployments: %v", err)}
	}
	if !resp.Success {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: errorMsg(resp.Errors)}
	}
	var deployments []cfPagesDeployment
	if err := json.Unmarshal(resp.Result, &deployments); err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "parse pages deployments result"}
	}
	return deployments, nil
}

func (p *CloudflareProvider) retryPagesDeployment(ctx context.Context, token, accountID, projectName, deployID string) (*cfPagesDeployment, error) {
	path := "/accounts/" + accountID + "/pages/projects/" + projectName + "/deployments/" + deployID + "/retry"
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	body, err := p.cfClient(token).DoReq(ctx, "POST", path, nil, headers)
	if err != nil {
		return nil, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: fmt.Sprintf("parse retry deployment: %v", err)}
	}
	if !resp.Success {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: errorMsg(resp.Errors)}
	}
	var deploy cfPagesDeployment
	if err := json.Unmarshal(resp.Result, &deploy); err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "parse retry deployment result"}
	}
	return &deploy, nil
}

func mapPagesProjectToResource(p cfPagesProject) domain.ExternalResource {
	meta := map[string]any{
		"created_on":  p.CreatedOn,
		"modified_on": p.ModifiedOn,
	}
	if p.Subdomain != "" {
		meta["subdomain"] = p.Subdomain
	}
	if p.Source != nil {
		meta["source_type"] = p.Source.Type
		if p.Source.Config != nil {
			meta["source_repo"] = p.Source.Config.Owner + "/" + p.Source.Config.RepoName
			meta["production_branch"] = p.Source.Config.ProductionBranch
		}
	}
	return domain.ExternalResource{
		ExternalID:  p.Name,
		DisplayName: p.Name,
		Meta:        meta,
	}
}

func mapPagesDeployToEvent(d cfPagesDeployment, projectName string) domain.DeployEvent {
	_ = summarizeStages(d.Stages)
	return domain.DeployEvent{
		ID:        d.ID,
		Status:    d.Status,
		CreatedAt: d.CreatedOn,
		UpdatedAt: d.ModifiedOn,
	}
}

func summarizeStages(stages []cfPagesStage) string {
	if len(stages) == 0 {
		return ""
	}
	var parts []string
	for _, s := range stages {
		start := ""
		if s.StartedAt != nil {
			start = *s.StartedAt
		}
		end := ""
		if s.EndedAt != nil {
			end = *s.EndedAt
		}
		parts = append(parts, fmt.Sprintf("[%s] %s (start=%s, end=%s)", s.Status, s.Name, start, end))
	}
	return strings.Join(parts, "\n")
}

func (p *CloudflareProvider) dashboardURL(accountID, projectName, deployID string) string {
	if accountID == "" {
		return fmt.Sprintf("https://dash.cloudflare.com/?to=pages/view=%s", projectName)
	}
	if deployID != "" {
		return fmt.Sprintf("https://dash.cloudflare.com/%s/pages/view/%s/%s", accountID, projectName, deployID)
	}
	return fmt.Sprintf("https://dash.cloudflare.com/%s/pages/view/%s", accountID, projectName)
}

// ---------------------------------------------------------------------------
// Deployer interface
// ---------------------------------------------------------------------------

func (p *CloudflareProvider) TriggerDeploy(ctx context.Context, account *domain.ProviderAccount, binding *domain.Binding, slot *domain.Slot) (*domain.DeployEvent, error) {
	aid := account.Meta.AccountID
	if aid == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "account_id is required in meta"}
	}
	projectName := binding.ExternalID
	if projectName == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "binding.external_id (project name) is required"}
	}

	project, err := p.getPagesProject(ctx, account.TokenEncrypted, aid, projectName)
	if err != nil {
		return nil, err
	}

	if project.Source == nil || project.Source.Type != "github" {
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: fmt.Sprintf("project %q is a Direct Upload project; trigger not supported (use wrangler CLI)", projectName)}
	}

	if project.LatestDeployment != nil && project.LatestDeployment.Status == "in_progress" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "a deployment is already in progress"}
	}

	if project.LatestDeployment == nil {
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: fmt.Sprintf("project %q has no previous deployment to retry", projectName)}
	}

	deploy, err := p.retryPagesDeployment(ctx, account.TokenEncrypted, aid, projectName, project.LatestDeployment.ID)
	if err != nil {
		return nil, err
	}

	ev := mapPagesDeployToEvent(*deploy, projectName)
	return &ev, nil
}

func (p *CloudflareProvider) ListDeployments(ctx context.Context, account *domain.ProviderAccount, binding *domain.Binding) ([]domain.DeployEvent, error) {
	aid := account.Meta.AccountID
	if aid == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "account_id is required in meta"}
	}
	projectName := binding.ExternalID
	if projectName == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "binding.external_id (project name) is required"}
	}

	deployments, err := p.listPagesDeployments(ctx, account.TokenEncrypted, aid, projectName)
	if err != nil {
		return nil, err
	}

	events := make([]domain.DeployEvent, 0, len(deployments))
	for _, d := range deployments {
		events = append(events, mapPagesDeployToEvent(d, projectName))
	}
	return events, nil
}

// GetBuildLogs returns deployment build logs.
// SPIKE: CF Pages REST has no raw log endpoint; fallback = stage summary + dashboard link.
func (p *CloudflareProvider) GetBuildLogs(ctx context.Context, account *domain.ProviderAccount, binding *domain.Binding, deployID string, tail int) (domain.LogChunk, error) {
	aid := account.Meta.AccountID
	if aid == "" {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "account_id is required in meta"}
	}
	projectName := binding.ExternalID
	if projectName == "" {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "binding.external_id (project name) is required"}
	}

	body, err := p.doGet(ctx, account.TokenEncrypted, "/accounts/"+aid+"/pages/projects/"+projectName+"/deployments/"+deployID)
	if err != nil {
		return domain.LogChunk{}, err
	}
	resp, err := parseCFResponse(body)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: fmt.Sprintf("parse deployment detail: %v", err)}
	}
	if !resp.Success {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: errorMsg(resp.Errors)}
	}
	var deploy cfPagesDeployment
	if err := json.Unmarshal(resp.Result, &deploy); err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "parse deployment detail result"}
	}

	lines := summarizeStages(deploy.Stages)
	if lines == "" {
		lines = "deployment " + deployID + " status: " + deploy.Status + " (no stage details available)"
	}

	meta := map[string]any{
		"fallback": "dashboard_link",
		"url":      p.dashboardURL(aid, projectName, deployID),
	}

	return domain.LogChunk{
		Lines:     lines,
		Truncated: false,
		Meta:      meta,
	}, nil
}

var _ provider.Provisioner = (*CloudflareProvider)(nil)
var _ provider.Deployer = (*CloudflareProvider)(nil)
var _ provider.LogFetcher = (*CloudflareProvider)(nil)