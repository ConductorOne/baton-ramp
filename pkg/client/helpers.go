package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

const (
	DefaultBaseURL = "https://api.ramp.com"
	APIPath        = "developer"
	APIVersion     = "v1"
)

// ResolveBaseURL returns the supplied base URL, falling back to DefaultBaseURL when empty.
func ResolveBaseURL(baseURL string) string {
	if baseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(baseURL, "/")
}

// TokenURL returns the Ramp OAuth 2.0 client-credentials token endpoint for the given base URL.
func TokenURL(baseURL string) string {
	return fmt.Sprintf("%s/%s/%s/token", ResolveBaseURL(baseURL), APIPath, APIVersion)
}

func (c *Client) apiBaseURL() string {
	return ResolveBaseURL(c.baseURL)
}

func (c *Client) newUnPaginatedURL(path string, v url.Values) (string, error) {
	reqUrl, err := url.Parse(fmt.Sprintf("%s/%s/%s/%s", c.apiBaseURL(), APIPath, APIVersion, path))
	if err != nil {
		return "", err
	}
	return reqUrl.String(), nil
}

func (c *Client) query(ctx context.Context, method string, requestURL string, res any) (*v2.RateLimitDescription, error) {
	return c.queryWithBody(ctx, method, requestURL, nil, res)
}

func (c *Client) queryWithBody(ctx context.Context, method, requestURL string, body any, res any) (*v2.RateLimitDescription, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	reqUrl, err := url.Parse(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request URL %s: %w", requestURL, err)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqUrl.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request %s: %w", reqUrl.String(), err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	var ratelimitData v2.RateLimitDescription
	doOpts := []uhttp.DoOption{uhttp.WithRatelimitData(&ratelimitData)}
	if res != nil {
		doOpts = append(doOpts, uhttp.WithJSONResponse(res))
	}
	resp, err := c.Do(req, doOpts...)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if resp != nil {
			logBody(ctx, resp.Body)
		}
		return &ratelimitData, fmt.Errorf("failed to execute request %s: %w", reqUrl.String(), err)
	}
	return &ratelimitData, nil
}

func logBody(ctx context.Context, bodyCloser io.ReadCloser) {
	if bodyCloser == nil {
		return
	}
	defer bodyCloser.Close()
	l := ctxzap.Extract(ctx)
	body := make([]byte, 1024*1024)
	n, err := bodyCloser.Read(body)
	if err != nil {
		l.Error("error reading response body", zap.Error(err))
		return
	}
	l.Debug("response body: ", zap.String("body", string(body[:n])))
}
