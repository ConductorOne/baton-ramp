package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
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
	reqUrl, err := url.Parse(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request URL %s: %w", requestURL, err)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqUrl.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request %s: %w", reqUrl.String(), err)
	}

	var ratelimitData v2.RateLimitDescription
	resp, err := c.Do(req,
		uhttp.WithRatelimitData(&ratelimitData),
	)
	if err != nil {
		return &ratelimitData, fmt.Errorf("failed to execute request %s: %w", reqUrl.String(), err)
	}
	defer resp.Body.Close()
	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return &ratelimitData, fmt.Errorf("failed to read response body %s: %w", reqUrl.String(), err)
	}
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusForbidden:
			return &ratelimitData, fmt.Errorf("HTTP request failed %d %s: %s", resp.StatusCode, string(rawResp), "Forbidden - check your API token or permissions")
		default:
			return &ratelimitData, fmt.Errorf("HTTP request failed %d %s", resp.StatusCode, string(rawResp))
		}
	}
	if err := json.Unmarshal(rawResp, res); err != nil {
		return &ratelimitData, err
	}
	return &ratelimitData, nil
}
