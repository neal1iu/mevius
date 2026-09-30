package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

const (
	defaultTimeout = 15 * time.Second
	userAgent      = "mevius/v1"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

func (c *Client) DoReq(ctx context.Context, method, path string, body []byte, headers map[string]string) ([]byte, error) {
	reqURL := c.baseURL + path
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil {
		return nil, err
	}

	if len(respBody) > 16<<20 {
		return nil, &Error{Kind: KindUpstream, ProviderMsg: "provider response exceeds 16MB limit"}
	}
	if resp.StatusCode >= 400 {
		h := make(map[string][]string)
		for k, v := range resp.Header {
			h[k] = v
		}
		return respBody, MapHTTP(resp.StatusCode, respBody, h)
	}

	return respBody, nil
}
