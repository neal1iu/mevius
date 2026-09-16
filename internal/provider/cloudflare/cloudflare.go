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

type CloudflareProvider struct {
	baseURL string
}

func NewProvider(baseURL string) *CloudflareProvider {
	return &CloudflareProvider{baseURL: baseURL}
}

func (p *CloudflareProvider) Type() string {
	return "cloudflare"
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

func (p *CloudflareProvider) cfClient(token string) *provider.Client {
	return provider.NewClient(p.baseURL)
}

func (p *CloudflareProvider) doGet(ctx context.Context, token, path string) ([]byte, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	return p.cfClient(token).DoReq(ctx, http.MethodGet, path, nil, headers)
}

func (p *CloudflareProvider) doDelete(ctx context.Context, token, path string) ([]byte, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	return p.cfClient(token).DoReq(ctx, http.MethodDelete, path, nil, headers)
}

func (p *CloudflareProvider) doPut(ctx context.Context, token, path string, body []byte, contentType string) ([]byte, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  contentType,
	}
	return p.cfClient(token).DoReq(ctx, http.MethodPut, path, body, headers)
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

func (p *CloudflareProvider) ValidateCredentials(ctx context.Context, account *domain.ProviderAccount) (domain.AccountMeta, error) {
	token := account.TokenEncrypted

	body, err := p.doGet(ctx, token, "/user/tokens/verify")
	if err != nil {
		return domain.AccountMeta{}, err
	}

	resp, err := parseCFResponse(body)
	if err != nil {
		return domain.AccountMeta{}, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse verify response: %v", err),
		}
	}
	if !resp.Success {
		msg := errorMsg(resp.Errors)
		return domain.AccountMeta{}, &provider.Error{
			Kind:        provider.KindUnauthorized,
			ProviderMsg: msg,
		}
	}

	var verify cfTokenVerify
	if err := json.Unmarshal(resp.Result, &verify); err != nil {
		return domain.AccountMeta{}, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "parse token verify result",
		}
	}
	if verify.Status != "active" {
		return domain.AccountMeta{}, &provider.Error{
			Kind:        provider.KindUnauthorized,
			ProviderMsg: fmt.Sprintf("token status is %q, want active", verify.Status),
		}
	}

	body, err = p.doGet(ctx, token, "/accounts")
	if err != nil {
		return domain.AccountMeta{}, err
	}

	resp, err = parseCFResponse(body)
	if err != nil {
		return domain.AccountMeta{}, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: fmt.Sprintf("parse accounts response: %v", err),
		}
	}
	if !resp.Success {
		msg := errorMsg(resp.Errors)
		return domain.AccountMeta{}, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: msg,
		}
	}

	var accounts []cfAccount
	if err := json.Unmarshal(resp.Result, &accounts); err != nil {
		return domain.AccountMeta{}, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "parse accounts list",
		}
	}
	if len(accounts) == 0 {
		return domain.AccountMeta{}, &provider.Error{
			Kind:        provider.KindUpstream,
			ProviderMsg: "no accounts found for token",
		}
	}

	acct := accounts[0]
	return domain.AccountMeta{
		AccountID: acct.ID,
		Raw: map[string]any{
			"account_id":   acct.ID,
			"account_name": acct.Name,
		},
	}, nil
}

func (p *CloudflareProvider) ListExternalResources(ctx context.Context, account *domain.ProviderAccount, kind domain.ResourceKind) ([]domain.ExternalResource, error) {
	switch kind {
	case domain.ResourceKindCompute:
		aid := account.Meta.AccountID
		if aid == "" {
			return nil, &provider.Error{
				Kind:        provider.KindUpstream,
				ProviderMsg: "account_id is required in meta",
			}
		}

		body, err := p.doGet(ctx, account.TokenEncrypted, "/accounts/"+aid+"/workers/scripts")
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

	case domain.ResourceKindStaticSite:
		aid := account.Meta.AccountID
		if aid == "" {
			return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "account_id is required in meta"}
		}
		projects, err := p.listPagesProjects(ctx, account.TokenEncrypted, aid)
		if err != nil {
			return nil, err
		}
		resources := make([]domain.ExternalResource, 0, len(projects))
		for _, pr := range projects {
			resources = append(resources, mapPagesProjectToResource(pr))
		}
		return resources, nil

	case domain.ResourceKindDNSDomain:
		zones, err := p.listZones(ctx, account.TokenEncrypted)
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
			ProviderMsg: "cloudflare does not support resource kind: " + string(kind),
		}
	}
}

func (p *CloudflareProvider) GetResource(ctx context.Context, account *domain.ProviderAccount, externalID string) (*domain.ExternalResource, error) {
	zone, err := p.getZone(ctx, account.TokenEncrypted, externalID)
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

func (p *CloudflareProvider) CreateResource(ctx context.Context, account *domain.ProviderAccount, spec domain.ResourceSpec) (*domain.ExternalResource, error) {
	aid := account.Meta.AccountID
	if aid == "" {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "account_id is required in meta"}
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
		project, err := p.createPagesProject(ctx, account.TokenEncrypted, aid, spec.Name, branch)
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
		body, err := p.doPut(ctx, account.TokenEncrypted, path, bodyBytes, contentType)
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

func (p *CloudflareProvider) DeleteResource(ctx context.Context, account *domain.ProviderAccount, externalID string) error {
	aid := account.Meta.AccountID
	if aid == "" {
		return &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "account_id is required in meta"}
	}

	_, err := p.doDelete(ctx, account.TokenEncrypted, "/accounts/"+aid+"/workers/scripts/"+externalID)
	if err != nil {
		var pErr *provider.Error
		if errors.As(err, &pErr) && pErr.Kind == provider.KindNotFound {
			return p.deletePagesProject(ctx, account.TokenEncrypted, aid, externalID)
		}
		return err
	}
	return nil
}