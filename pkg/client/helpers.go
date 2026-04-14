package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

const (
	APIDomain  = "api.ramp.com"
	APIPath    = "developer"
	APIVersion = "v1"
)

func (c *Client) newUnPaginatedURL(path string, v url.Values) (string, error) {
	reqUrl, err := url.Parse(fmt.Sprintf("https://%s/%s/%s/%s", APIDomain, APIPath, APIVersion, path))
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
	if err != nil {
		if resp != nil {
			logBody(ctx, resp.Body)
		}
		return &ratelimitData, fmt.Errorf("failed to execute request %s: %w", reqUrl.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logBody(ctx, resp.Body)
		return &ratelimitData, fmt.Errorf("unexpected status code %d for request %s: %s", resp.StatusCode, reqUrl.String(), http.StatusText(resp.StatusCode))
	}
	return &ratelimitData, nil
}

func logBody(ctx context.Context, bodyCloser io.ReadCloser) {
	defer bodyCloser.Close()
	l := ctxzap.Extract(ctx)
	body := make([]byte, 1024*1024)
	n, err := bodyCloser.Read(body)
	if err != nil {
		l.Error("error reading response body", zap.Error(err))
		return
	}
	l.Info("response body: ", zap.String("body", string(body[:n])))
}
