package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"mevius/internal/domain"
	"mevius/internal/provider"
)

type CloudflareProvider struct{}

func NewProvider() *CloudflareProvider {
	return &CloudflareProvider{}
}

func (p *CloudflareProvider) Type() string {
	return "cloudflare"
}

func (p *CloudflareProvider) Descriptor() domain.ProviderDescriptor {
	return provider.CloudflareDescriptor
}

type cfResponse struct {
	Success  bool            `json:"success"`
	Result   json.RawMessage `json:"result"`
	Errors   []cfError       `json:"errors"`
	Messages []any           `json:"messages"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfTokenVerify struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type cfAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfWorkerMeta struct {
	ID         string `json:"id"`
	CreatedOn  string `json:"created_on"`
	ModifiedOn string `json:"modified_on"`
}

func defaultEndpoint(endpoint string) string {
	if endpoint == "" {
		return "https://api.cloudflare.com/client/v4"
	}
	return endpoint
}

func (p *CloudflareProvider) cfClient(endpoint string) *provider.Client {
	return provider.NewClient(endpoint)
}

func extractToken(credential []byte) string {
	return string(credential)
}

func (p *CloudflareProvider) doGet(ctx context.Context, token, endpoint, path string) ([]byte, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	return p.cfClient(endpoint).DoReq(ctx, http.MethodGet, path, nil, headers)
}

func (p *CloudflareProvider) doDelete(ctx context.Context, token, endpoint, path string) ([]byte, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	return p.cfClient(endpoint).DoReq(ctx, http.MethodDelete, path, nil, headers)
}

func (p *CloudflareProvider) doPut(ctx context.Context, token, endpoint, path string, body []byte, contentType string) ([]byte, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  contentType,
	}
	return p.cfClient(endpoint).DoReq(ctx, http.MethodPut, path, body, headers)
}

func parseCFResponse(body []byte) (*cfResponse, error) {
	var resp cfResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func errorMsg(errs []cfError) string {
	if len(errs) == 0 {
		return ""
	}
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Message)
	}
	return strings.Join(msgs, "; ")
}

func (p *CloudflareProvider) ValidateCredentials(ctx context.Context, conn *domain.ProviderConnection, credential []byte) (json.RawMessage, error) {
	token := extractToken(credential)
	endpoint := defaultEndpoint(conn.Endpoint)

	body, err := p.doGet(ctx, token, endpoint, "/user/tokens/verify")
	if err != nil {
		return nil, err
	}

	resp, err := parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse verify response: %v", err),
		}
	}
	if !resp.Success {
		msg := errorMsg(resp.Errors)
		return nil, &provider.Error{
			Kind:        provider.KindUnauthorized,
			ProviderMsg: msg,
		}
	}

	var verify cfTokenVerify
	if err := json.Unmarshal(resp.Result, &verify); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "parse token verify result",
		}
	}
	if verify.Status != "active" {
		return nil, &provider.Error{
			Kind:        provider.KindUnauthorized,
			ProviderMsg: fmt.Sprintf("token status is %q, want active", verify.Status),
		}
	}

	body, err = p.doGet(ctx, token, endpoint, "/accounts")
	if err != nil {
		return nil, err
	}

	resp, err = parseCFResponse(body)
	if err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse accounts response: %v", err),
		}
	}
	if !resp.Success {
		msg := errorMsg(resp.Errors)
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: msg,
		}
	}

	var accounts []cfAccount
	if err := json.Unmarshal(resp.Result, &accounts); err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "parse accounts list",
		}
	}
	if len(accounts) == 0 {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "no accounts found for token",
		}
	}

	acct := accounts[0]
	identity := map[string]any{
		"account_id":   acct.ID,
		"account_name": acct.Name,
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return nil, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "marshal identity",
		}
	}
	return json.RawMessage(raw), nil
}

func accountIDFromMeta(meta map[string]any) string {
	if v, ok := meta["account_id"].(string); ok {
		return v
	}
	return ""
}

func (p *CloudflareProvider) ListExternalResources(ctx context.Context, conn *domain.ProviderConnection, credential []byte, product domain.ProductType) ([]domain.ExternalResource, error) {
	token := extractToken(credential)
	endpoint := defaultEndpoint(conn.Endpoint)

	switch product {
	case "cloudflare.workers":
		aid := accountIDFromMeta(conn.RemoteIdentity.Raw)
		if aid == "" {
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: "account_id is required in remote identity",
			}
		}

		body, err := p.doGet(ctx, token, endpoint, "/accounts/"+aid+"/workers/scripts")
		if err != nil {
			return nil, err
		}

		resp, err := parseCFResponse(body)
		if err != nil {
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: fmt.Sprintf("parse workers list: %v", err),
			}
		}
		if !resp.Success {
			msg := errorMsg(resp.Errors)
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: msg,
			}
		}

		var workers []cfWorkerMeta
		if err := json.Unmarshal(resp.Result, &workers); err != nil {
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: "parse workers list result",
			}
		}

		var resources []domain.ExternalResource
		for _, w := range workers {
			resources = append(resources, domain.ExternalResource{
				ExternalID:  w.ID,
				DisplayName: w.ID,
				Meta: map[string]any{
					"modified_on": w.ModifiedOn,
				},
			})
		}
		return resources, nil

	case "cloudflare.pages":
		aid := accountIDFromMeta(conn.RemoteIdentity.Raw)
		if aid == "" {
			return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "account_id is required in remote identity"}
		}
		projects, err := p.listPagesProjects(ctx, token, endpoint, aid)
		if err != nil {
			return nil, err
		}
		resources := make([]domain.ExternalResource, 0, len(projects))
		for _, pr := range projects {
			resources = append(resources, mapPagesProjectToResource(pr))
		}
		return resources, nil

	case "cloudflare.dns":
		zones, err := p.listZones(ctx, token, endpoint)
		if err != nil {
			return nil, err
		}
		var resources []domain.ExternalResource
		for _, z := range zones {
			resources = append(resources, domain.ExternalResource{
				ExternalID:  z.ID,
				DisplayName: z.Name,
				Meta: map[string]any{
					"status":       z.Status,
					"name_servers": z.NameServers,
				},
			})
		}
		return resources, nil

	default:
		return nil, &provider.Error{
			Kind:        provider.KindUnsupported,
			ProviderMsg: "cloudflare does not support product: " + string(product),
		}
	}
}

func (p *CloudflareProvider) GetResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
	token := extractToken(credential)
	endpoint := defaultEndpoint(conn.Endpoint)

	zone, err := p.getZone(ctx, token, endpoint, externalID)
	if err != nil {
		return nil, err
	}
	return &domain.ExternalResource{
		ExternalID:  zone.ID,
		DisplayName: zone.Name,
		Meta: map[string]any{
			"status":       zone.Status,
			"name_servers": zone.NameServers,
		},
	}, nil
}

const placeholderScript = `export default { async fetch(request) { return new Response("mevius placeholder", { status: 200 }); } }`

func buildWorkerUploadBody() ([]byte, string, error) {
	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)

	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="metadata"`)
	hdr.Set("Content-Type", "application/json")
	metaPart, err := mp.CreatePart(hdr)
	if err != nil {
		return nil, "", err
	}
	if _, err := io.WriteString(metaPart, `{"main_module":"worker.js"}`); err != nil {
		return nil, "", err
	}

	hdr2 := make(textproto.MIMEHeader)
	hdr2.Set("Content-Disposition", `form-data; name="worker.js"; filename="worker.js"`)
	hdr2.Set("Content-Type", "application/javascript+module")
	scriptPart, err := mp.CreatePart(hdr2)
	if err != nil {
		return nil, "", err
	}
	if _, err := io.WriteString(scriptPart, placeholderScript); err != nil {
		return nil, "", err
	}

	contentType := mp.FormDataContentType()
	if err := mp.Close(); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), contentType, nil
}

func (p *CloudflareProvider) CreateResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, spec domain.ResourceSpec) (*domain.ExternalResource, error) {
	token := extractToken(credential)
	endpoint := defaultEndpoint(conn.Endpoint)
	aid := accountIDFromMeta(conn.RemoteIdentity.Raw)
	if aid == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "account_id is required in remote identity"}
	}
	if spec.Name == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "spec.name is required"}
	}

	switch spec.Kind {
	case domain.ResourceKindStaticSite:
		branch := ""
		if spec.Extra != nil {
			if b, ok := spec.Extra["production_branch"].(string); ok {
				branch = b
			}
		}
		project, err := p.createPagesProject(ctx, token, endpoint, aid, spec.Name, branch)
		if err != nil {
			return nil, err
		}
		res := mapPagesProjectToResource(*project)
		return &res, nil

	default:
		bodyBytes, contentType, err := buildWorkerUploadBody()
		if err != nil {
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: fmt.Sprintf("build upload body: %v", err),
			}
		}

		path := "/accounts/" + aid + "/workers/scripts/" + spec.Name
		body, err := p.doPut(ctx, token, endpoint, path, bodyBytes, contentType)
		if err != nil {
			return nil, err
		}

		resp, err := parseCFResponse(body)
		if err != nil {
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: fmt.Sprintf("parse create response: %v", err),
			}
		}
		if !resp.Success {
			msg := errorMsg(resp.Errors)
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: msg,
			}
		}

		var worker cfWorkerMeta
		if err := json.Unmarshal(resp.Result, &worker); err != nil {
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: "parse create worker result",
			}
		}

		return &domain.ExternalResource{
			ExternalID:  worker.ID,
			DisplayName: worker.ID,
			Meta: map[string]any{
				"modified_on": worker.ModifiedOn,
			},
		}, nil
	}
}

func (p *CloudflareProvider) DeleteResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) error {
	token := extractToken(credential)
	endpoint := defaultEndpoint(conn.Endpoint)
	aid := accountIDFromMeta(conn.RemoteIdentity.Raw)
	if aid == "" {
		return &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "account_id is required in remote identity"}
	}

	_, err := p.doDelete(ctx, token, endpoint, "/accounts/"+aid+"/workers/scripts/"+externalID)
	if err != nil {
		var pErr *provider.Error
		if errors.As(err, &pErr) && pErr.Kind == provider.KindNotFound {
			return p.deletePagesProject(ctx, token, endpoint, aid, externalID)
		}
		return err
	}
	return nil
}