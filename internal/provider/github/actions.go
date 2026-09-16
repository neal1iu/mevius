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
	"time"

	"mevius/internal/domain"
	"mevius/internal/provider"

	gogithub "github.com/google/go-github/v66/github"
)

func (p *GitHubProvider) TriggerDeploy(ctx context.Context, account *domain.ProviderAccount, binding *domain.Binding, slot *domain.Slot) (*domain.DeployEvent, error) {
	owner, repo := splitExternalID(binding.ExternalID)
	if owner == "" || repo == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid external_id: expected owner/repo"}
	}

	var cfg domain.RepoConfig
	if err := parseSlotConfig(slot, &cfg); err != nil {
		return nil, err
	}

	if cfg.WorkflowID == "" {
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: "no workflow_id configured in slot"}
	}

	ref := cfg.WorkflowRef
	if ref == "" {
		ref = "main"
	}

	client := p.ghClient(account.TokenEncrypted)
	event := gogithub.CreateWorkflowDispatchEventRequest{Ref: ref}
	_, err := client.Actions.CreateWorkflowDispatchEventByFileName(ctx, owner, repo, cfg.WorkflowID, event)
	if err != nil {
		return nil, mapError(err)
	}

	return &domain.DeployEvent{Status: "queued"}, nil
}

func (p *GitHubProvider) ListDeployments(ctx context.Context, account *domain.ProviderAccount, binding *domain.Binding) ([]domain.DeployEvent, error) {
	owner, repo := splitExternalID(binding.ExternalID)
	if owner == "" || repo == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid external_id: expected owner/repo"}
	}

	client := p.ghClient(account.TokenEncrypted)
	opts := &gogithub.ListWorkflowRunsOptions{
		ListOptions: gogithub.ListOptions{PerPage: 20},
	}
	runs, _, err := client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
	if err != nil {
		return nil, mapError(err)
	}

	events := make([]domain.DeployEvent, 0, len(runs.WorkflowRuns))
	for _, run := range runs.WorkflowRuns {
		events = append(events, domain.DeployEvent{
			ID:        strconv.FormatInt(run.GetID(), 10),
			Status:    runStatus(run),
			CreatedAt: run.GetCreatedAt().Format(time.RFC3339),
		})
	}
	return events, nil
}

func runStatus(run *gogithub.WorkflowRun) string {
	s := run.GetStatus()
	if s == "completed" {
		return run.GetConclusion()
	}
	return s
}

func (p *GitHubProvider) GetBuildLogs(ctx context.Context, account *domain.ProviderAccount, binding *domain.Binding, deployID string, tail int) (domain.LogChunk, error) {
	owner, repo := splitExternalID(binding.ExternalID)
	if owner == "" || repo == "" {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid external_id: expected owner/repo"}
	}

	runID, err := strconv.ParseInt(deployID, 10, 64)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid deploy_id: expected integer run ID"}
	}

	if tail <= 0 {
		tail = 256 * 1024
	}

	client := p.ghClient(account.TokenEncrypted)
	u := fmt.Sprintf("repos/%s/%s/actions/runs/%d/logs", owner, repo, runID)
	req, err := client.NewRequest("GET", u, nil)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
	}

	noRedirectClient := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noRedirectClient.Do(req)
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
		zipURL := resp.Header.Get("Location")
		if zipURL == "" {
			return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "no redirect URL in logs response"}
		}
		return downloadAndTailLogs(ctx, zipURL, tail)
	default:
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: fmt.Sprintf("unexpected response status: %d", resp.StatusCode)}
	}
}

func downloadAndTailLogs(ctx context.Context, zipURL string, maxBytes int) (domain.LogChunk, error) {
	dlClient := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
	}

	resp, err := dlClient.Do(req)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
	}
	defer resp.Body.Close()

	limitedReader := io.LimitReader(resp.Body, 2*1024*1024+1)
	zipData, err := io.ReadAll(limitedReader)
	if err != nil {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: err.Error()}
	}

	if len(zipData) > 2*1024*1024 {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "log archive exceeds 2MB limit"}
	}

	lines, truncated := tailZipContent(zipData, maxBytes)
	return domain.LogChunk{Lines: lines, Truncated: truncated}, nil
}

func tailZipContent(zipData []byte, maxBytes int) (string, bool) {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", false
	}

	var fileNames []string
	files := make(map[string][]byte, len(reader.File))
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		fileNames = append(fileNames, f.Name)
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = content
	}
	sort.Strings(fileNames)

	var buf bytes.Buffer
	for _, name := range fileNames {
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.Write(files[name])
	}

	content := buf.Bytes()
	if len(content) <= maxBytes {
		return string(content), false
	}

	start := len(content) - maxBytes
	return string(content[start:]), true
}

func parseSlotConfig(slot *domain.Slot, cfg *domain.RepoConfig) *provider.Error {
	if slot.Config == nil {
		return &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "slot has no config"}
	}
	if err := json.Unmarshal(slot.Config, cfg); err != nil {
		return &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "invalid slot config: " + err.Error()}
	}
	return nil
}