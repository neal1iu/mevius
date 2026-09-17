// Redeploy recipe spike conclusion:
// Optimal trigger: POST /v13/deployments with {"deploymentId":"<latest>","name":"<project>","target":"production"}
// The deploymentId approach inherits all project settings, env vars, and git metadata
// automatically, avoiding the complexity of reconstructing gitSource from deployment
// metadata. This is the officially documented redeploy mechanism and is simpler
// than extracting gitSource from the latest deployment's meta field.
//
// Request body example:
//
//	POST https://api.vercel.com/v13/deployments
//	{"deploymentId":"dpl_abc123","name":"my-project","target":"production"}
package vercel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

func (p *VercelProvider) TriggerDeploy(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, slot *domain.Slot) (*domain.DeployEvent, error) {
	cl := p.client(conn)

	latestID, name, err := p.latestDeployment(ctx, cl, conn, credential, binding)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"deploymentId": latestID,
		"name":         name,
		"target":       "production",
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal deploy request: %w", err)
	}

	path := p.buildPath("/v13/deployments", conn)
	headers := p.authHeaders(credential)
	headers["Content-Type"] = "application/json"

	body, err := cl.DoReq(ctx, "POST", path, reqBody, headers)
	if err != nil {
		return nil, err
	}

	var createResp struct {
		ID         string `json:"id"`
		ReadyState string `json:"readyState"`
		URL        string `json:"url"`
		CreatedAt  int64  `json:"createdAt"`
	}
	if err := json.Unmarshal(body, &createResp); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse create deployment response: %v", err),
		}
	}

	evt := &domain.DeployEvent{
		ID:        createResp.ID,
		Status:    mapDeployStatus(createResp.ReadyState),
		CreatedAt: formatTimestamp(createResp.CreatedAt),
	}

	return evt, nil
}

func (p *VercelProvider) latestDeployment(ctx context.Context, cl *provider.Client, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding) (string, string, error) {
	path := p.buildPath(fmt.Sprintf("/v6/deployments?projectId=%s&limit=1", url.QueryEscape(binding.ExternalID)), conn)
	body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(credential))
	if err != nil {
		return "", "", err
	}

	var listResp struct {
		Deployments []struct {
			UID  string `json:"uid"`
			Name string `json:"name"`
		} `json:"deployments"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		return "", "", &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse deployments list: %v", err),
		}
	}
	if len(listResp.Deployments) == 0 {
		return "", "", &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "no existing deployments to redeploy from",
		}
	}
	return listResp.Deployments[0].UID, listResp.Deployments[0].Name, nil
}

func (p *VercelProvider) ListDeployments(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding) ([]domain.DeployEvent, error) {
	cl := p.client(conn)
	path := p.buildPath(fmt.Sprintf("/v6/deployments?projectId=%s&limit=20", url.QueryEscape(binding.ExternalID)), conn)

	body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(credential))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Deployments []struct {
			UID        string `json:"uid"`
			ReadyState string `json:"readyState"`
			URL        string `json:"url"`
			CreatedAt  int64  `json:"createdAt"`
		} `json:"deployments"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse deployments response: %v", err),
		}
	}

	events := make([]domain.DeployEvent, 0, len(resp.Deployments))
	for _, d := range resp.Deployments {
		evt := domain.DeployEvent{
			ID:        d.UID,
			Status:    mapDeployStatus(d.ReadyState),
			CreatedAt: formatTimestamp(d.CreatedAt),
		}
		events = append(events, evt)
	}
	return events, nil
}

func (p *VercelProvider) GetBuildLogs(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, deployID string, tail int) (domain.LogChunk, error) {
	cl := p.client(conn)
	path := p.buildPath("/v3/deployments/"+url.PathEscape(deployID)+"/events", conn)

	body, err := cl.DoReq(ctx, "GET", path, nil, p.authHeaders(credential))
	if err != nil {
		return domain.LogChunk{}, err
	}

	var events []struct {
		Payload struct {
			Text string `json:"text"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &events); err != nil {
		return domain.LogChunk{}, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse events response: %v", err),
		}
	}

	var buf bytes.Buffer
	for _, e := range events {
		if e.Payload.Text != "" {
			buf.WriteString(e.Payload.Text)
		}
	}
	lines := buf.String()

	truncated := false
	maxBytes := 256 * 1024
	if len(lines) > maxBytes {
		lines = lines[len(lines)-maxBytes:]
		truncated = true
	}

	return domain.LogChunk{Lines: lines, Truncated: truncated}, nil
}

func mapDeployStatus(readyState string) string {
	switch readyState {
	case "BUILDING", "INITIALIZING":
		return "in_progress"
	case "READY":
		return "completed/success"
	case "ERROR", "CANCELED", "BLOCKED":
		return "completed/failure"
	case "QUEUED":
		return "queued"
	default:
		return "unknown"
	}
}

func formatTimestamp(ms int64) string {
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

var (
	_ provider.Deployer   = (*VercelProvider)(nil)
	_ provider.LogFetcher = (*VercelProvider)(nil)
)