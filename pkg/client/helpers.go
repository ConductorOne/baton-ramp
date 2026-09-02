package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	APIPath    = "developer"
	APIVersion = "v1"
)

// ResolveBaseURL trims trailing slashes from the supplied base URL.
func ResolveBaseURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/")
}

// TokenURL returns the Ramp OAuth 2.0 client-credentials token endpoint for the given base URL.
func TokenURL(baseURL string) string {
	return fmt.Sprintf("%s/%s/%s/token", ResolveBaseURL(baseURL), APIPath, APIVersion)
}

func (c *Client) apiBaseURL() string {
	return ResolveBaseURL(c.baseURL)
}

func (c *Client) newUnPaginatedURL(path string) (string, error) {
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

// queryCollection issues a request against a collection (list/pagination)
// endpoint on the *sync* path, classifying an HTTP 404 as a fault rather than a
// benign miss.
//
// uhttp maps 404 to codes.NotFound, and the SDK's IsSyncPreservable
// (baton-sdk/pkg/sync/syncer.go) lists codes.NotFound as recoverable. For a
// single-resource GET that is correct -- the resource genuinely may be gone.
// For a collection endpoint it is not: /developer/v1/users always exists, so a
// 404 there means the request failed, not that there are no more users. Left
// as NotFound, a 404 mid-pagination lets a partial sync be preserved and
// ingested as though it had completed, silently dropping every record past the
// page that failed. codes.Internal is not on the preservable list, so the sync
// fails instead.
//
// Scope: sync list endpoints only. The audit-log event feed deliberately keeps
// plain query and its NotFound classification -- no c1z is at stake there, so
// artifact preservation is not the concern, and a 404 from that endpoint is a
// plausible tenant-configuration signal (audit logs not enabled) that should
// not be reported as a connector fault. See the note in ListAuditLogEvents.
//
// The original error is joined rather than formatted in, so errors.Is/errors.As
// still reach the uhttp cause; codes.Internal wins because WrapErrors puts its
// own status first in the joined chain.
func (c *Client) queryCollection(ctx context.Context, method, requestURL string, body, res any) (*v2.RateLimitDescription, error) {
	ratelimitData, err := c.queryWithBody(ctx, method, requestURL, body, res)
	if err != nil && status.Code(err) == codes.NotFound {
		return ratelimitData, uhttp.WrapErrors(codes.Internal,
			"collection endpoint returned 404, which is never a benign empty result", err)
	}
	return ratelimitData, err
}

// warnIfTruncatedPage logs when a page comes back full but carries no next
// cursor. That is the signature of a silently truncated list: with no cursor to
// follow the caller stops early, and the sync completes "successfully" holding
// only part of the collection.
//
// This is legitimate when the collection size is an exact multiple of pageSize,
// so it warns rather than failing. See the note in ListUsers.
func warnIfTruncatedPage(ctx context.Context, endpoint string, returned, pageSize int, next string, extra ...zap.Field) {
	if next != "" || pageSize <= 0 || returned < pageSize {
		return
	}
	fields := append([]zap.Field{
		zap.String("endpoint", endpoint),
		zap.Int("returned", returned),
		zap.Int("page_size", pageSize),
	}, extra...)
	ctxzap.Extract(ctx).Warn(
		"ramp-client: full page returned with no next cursor; the list may be truncated",
		fields...,
	)
}

// effectivePageSize reports the page_size actually sent on reqURL, or 0 if it
// carries none. Cursor URLs come back from Ramp with their own page_size, so
// this reads the value off the request rather than assuming the default.
func effectivePageSize(reqURL string) int {
	parsed, err := url.Parse(reqURL)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(parsed.Query().Get("page_size"))
	if err != nil {
		return 0
	}
	return n
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
