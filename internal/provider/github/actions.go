package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"mevius/internal/domain"
	"mevius/internal/provider"

	gogithub "github.com/google/go-github/v66/github"
)

type actionsDriver struct{ provider *GitHubProvider }

type pipelineConfig struct {
	Version              int    `json:"version"`
	RepositoryExternalID string `json:"repository_external_id"`
	WorkflowID           int64  `json:"workflow_id"`
	WorkflowPath         string `json:"workflow_path,omitempty"`
	DefaultRef           string `json:"default_ref,omitempty"`
}

func (d *actionsDriver) Descriptor() domain.ProductDescriptor {
	return product(provider.GitHubDescriptor, "github.actions")
}

func (d *actionsDriver) Discover(ctx context.Context, conn *domain.ProviderConnection, credential []byte, scope domain.DiscoveryScope) ([]domain.ExternalResource, error) {
	if scope.Parent == nil || scope.Parent.ResourceKind != domain.ResourceKindGitRepo {
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: "github.actions discovery requires a git_repo parent"}
	}
	owner, repo := splitExternalID(scope.Parent.ExternalID)
	if owner == "" || repo == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid repository external id"}
	}
	workflows, _, err := d.provider.ghClient(string(credential), conn.Endpoint).Actions.ListWorkflows(ctx, owner, repo, &gogithub.ListOptions{PerPage: 100})
	if err != nil {
		return nil, mapError(err)
	}
	result := make([]domain.ExternalResource, 0, len(workflows.Workflows))
	for _, workflow := range workflows.Workflows {
		defaultRef, _ := scope.Parent.CachedMeta["default_branch"].(string)
		cfg := pipelineConfig{Version: 1, RepositoryExternalID: scope.Parent.ExternalID, WorkflowID: workflow.GetID(), WorkflowPath: workflow.GetPath(), DefaultRef: defaultRef}
		cfgJSON, _ := json.Marshal(cfg)
		result = append(result, domain.ExternalResource{
			ExternalID: pipelineExternalID(scope.Parent.ExternalID, workflow.GetID()), ExternalURL: workflow.GetHTMLURL(), DisplayName: workflow.GetName(),
			ProviderConfig: cfgJSON, Meta: map[string]any{"state": workflow.GetState(), "path": workflow.GetPath()},
		})
	}
	return result, nil
}

func (d *actionsDriver) Inspect(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) (*domain.ExternalResource, error) {
	cfg, err := pipelineConfigFor(instance)
	if err != nil {
		return nil, err
	}
	owner, repo := splitExternalID(cfg.RepositoryExternalID)
	wf, _, ghErr := d.provider.ghClient(string(credential), conn.Endpoint).Actions.GetWorkflowByID(ctx, owner, repo, cfg.WorkflowID)
	if ghErr != nil {
		return nil, mapError(ghErr)
	}
	cfg.WorkflowPath = wf.GetPath()
	cfgJSON, _ := json.Marshal(cfg)
	return &domain.ExternalResource{ExternalID: instance.ExternalID, ExternalURL: wf.GetHTMLURL(), DisplayName: wf.GetName(), ProviderConfig: cfgJSON, Meta: map[string]any{"state": wf.GetState(), "path": wf.GetPath()}}, nil
}

func (d *actionsDriver) TriggerPipeline(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, req domain.TriggerPipelineRequest) (*domain.Execution, error) {
	cfg, err := pipelineConfigFor(instance)
	if err != nil {
		return nil, err
	}
	owner, repo := splitExternalID(cfg.RepositoryExternalID)
	ref := req.Ref
	if ref == "" {
		ref = cfg.DefaultRef
	}
	if ref == "" {
		ref = "main"
	}
	inputs := make(map[string]interface{}, len(req.Inputs))
	for key, value := range req.Inputs {
		inputs[key] = value
	}
	event := gogithub.CreateWorkflowDispatchEventRequest{Ref: ref, Inputs: inputs}
	if _, err := d.provider.ghClient(string(credential), conn.Endpoint).Actions.CreateWorkflowDispatchEventByID(ctx, owner, repo, cfg.WorkflowID, event); err != nil {
		return nil, mapError(err)
	}
	return &domain.Execution{Status: domain.ExecutionQueued, ProviderStatus: "dispatched", Ref: ref, CreatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (d *actionsDriver) ListPipelineRuns(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance) ([]domain.Execution, error) {
	cfg, err := pipelineConfigFor(instance)
	if err != nil {
		return nil, err
	}
	owner, repo := splitExternalID(cfg.RepositoryExternalID)
	runs, _, ghErr := d.provider.ghClient(string(credential), conn.Endpoint).Actions.ListWorkflowRunsByID(ctx, owner, repo, cfg.WorkflowID, &gogithub.ListWorkflowRunsOptions{ListOptions: gogithub.ListOptions{PerPage: 20}})
	if ghErr != nil {
		return nil, mapError(ghErr)
	}
	result := make([]domain.Execution, 0, len(runs.WorkflowRuns))
	for _, run := range runs.WorkflowRuns {
		result = append(result, workflowRun(run))
	}
	return result, nil
}

func (d *actionsDriver) GetPipelineRun(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, runID string) (*domain.Execution, error) {
	cfg, err := pipelineConfigFor(instance)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(runID, 10, 64)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid run id"}
	}
	owner, repo := splitExternalID(cfg.RepositoryExternalID)
	run, _, ghErr := d.provider.ghClient(string(credential), conn.Endpoint).Actions.GetWorkflowRunByID(ctx, owner, repo, id)
	if ghErr != nil {
		return nil, mapError(ghErr)
	}
	result := workflowRun(run)
	return &result, nil
}

func (d *actionsDriver) CancelPipelineRun(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, runID string) error {
	cfg, err := pipelineConfigFor(instance)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(runID, 10, 64)
	if err != nil {
		return &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid run id"}
	}
	owner, repo := splitExternalID(cfg.RepositoryExternalID)
	_, ghErr := d.provider.ghClient(string(credential), conn.Endpoint).Actions.CancelWorkflowRunByID(ctx, owner, repo, id)
	if ghErr != nil {
		return mapError(ghErr)
	}
	return nil
}

func (d *actionsDriver) RerunPipeline(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, runID string) (*domain.Execution, error) {
	cfg, err := pipelineConfigFor(instance)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(runID, 10, 64)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid run id"}
	}
	owner, repo := splitExternalID(cfg.RepositoryExternalID)
	if _, ghErr := d.provider.ghClient(string(credential), conn.Endpoint).Actions.RerunWorkflowByID(ctx, owner, repo, id); ghErr != nil {
		return nil, mapError(ghErr)
	}
	return &domain.Execution{ID: runID, Status: domain.ExecutionQueued, ProviderStatus: "rerun_requested", CreatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (d *actionsDriver) GetLogs(ctx context.Context, conn *domain.ProviderConnection, credential []byte, instance *domain.ResourceInstance, executionID string, tail int) (domain.LogChunk, error) {
	cfg, err := pipelineConfigFor(instance)
	if err != nil {
		return domain.LogChunk{}, err
	}
	owner, repo := splitExternalID(cfg.RepositoryExternalID)
	runID, err := strconv.ParseInt(executionID, 10, 64)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid run id"}
	}
	if tail <= 0 {
		tail = 256 * 1024
	}
	client := d.provider.ghClient(string(credential), conn.Endpoint)
	req, err := client.NewRequest("GET", fmt.Sprintf("repos/%s/%s/actions/runs/%d/logs", owner, repo, runID), nil)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
	}
	noRedirect := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusGone:
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindNotFound, ProviderMsg: "logs expired"}
	case http.StatusNotFound:
		return domain.LogChunk{}, provider.MapHTTP(resp.StatusCode, nil, nil)
	case http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect:
		if location := resp.Header.Get("Location"); location != "" {
			return downloadAndTailLogs(ctx, location, tail)
		}
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "missing logs redirect"}
	default:
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: fmt.Sprintf("unexpected logs status %d", resp.StatusCode)}
	}
}

func pipelineConfigFor(instance *domain.ResourceInstance) (pipelineConfig, error) {
	var cfg pipelineConfig
	if err := json.Unmarshal(instance.ProviderConfig, &cfg); err != nil || cfg.RepositoryExternalID == "" || cfg.WorkflowID == 0 {
		parts := strings.Split(instance.ExternalID, "#")
		if len(parts) == 2 {
			cfg.RepositoryExternalID = parts[0]
			cfg.WorkflowID, _ = strconv.ParseInt(parts[1], 10, 64)
		}
	}
	if cfg.RepositoryExternalID == "" || cfg.WorkflowID == 0 {
		return cfg, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "pipeline provider config is incomplete"}
	}
	return cfg, nil
}

func pipelineExternalID(repo string, workflowID int64) string {
	return fmt.Sprintf("%s#%d", repo, workflowID)
}

func workflowRun(run *gogithub.WorkflowRun) domain.Execution {
	providerStatus := run.GetStatus()
	if providerStatus == "completed" {
		providerStatus = run.GetConclusion()
	}
	status := domain.ExecutionUnknown
	switch providerStatus {
	case "queued", "requested", "waiting", "pending":
		status = domain.ExecutionQueued
	case "in_progress":
		status = domain.ExecutionRunning
	case "success":
		status = domain.ExecutionSucceeded
	case "failure", "timed_out", "action_required", "startup_failure", "stale":
		status = domain.ExecutionFailed
	case "cancelled", "skipped":
		status = domain.ExecutionCancelled
	}
	return domain.Execution{ID: strconv.FormatInt(run.GetID(), 10), Status: status, ProviderStatus: providerStatus, Ref: run.GetHeadBranch(), CommitSHA: run.GetHeadSHA(), ExternalURL: run.GetHTMLURL(), CreatedAt: run.GetCreatedAt().Format(time.RFC3339), FinishedAt: run.GetUpdatedAt().Format(time.RFC3339)}
}

func downloadAndTailLogs(ctx context.Context, zipURL string, maxBytes int) (domain.LogChunk, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
	}
	if len(data) > 2*1024*1024 {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "log archive exceeds 2MB limit"}
	}
	lines, truncated := tailZipContent(data, maxBytes)
	return domain.LogChunk{Lines: lines, Truncated: truncated}, nil
}

func tailZipContent(data []byte, maxBytes int) (string, bool) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", false
	}
	files := map[string][]byte{}
	names := []string{}
	for _, file := range r.File {
		if file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			continue
		}
		content, _ := io.ReadAll(rc)
		rc.Close()
		names = append(names, file.Name)
		files[file.Name] = content
	}
	sort.Strings(names)
	var buf bytes.Buffer
	for _, name := range names {
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.Write(files[name])
	}
	content := buf.Bytes()
	if len(content) <= maxBytes {
		return string(content), false
	}
	return string(content[len(content)-maxBytes:]), true
}
