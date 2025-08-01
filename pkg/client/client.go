package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Client struct {
	*uhttp.BaseHttpClient
	TokenSource oauth2.TokenSource
}

type openGraphHttpTransport struct {
	base        http.RoundTripper
	rateLimit   *v2.RateLimitDescription
	tokenSource oauth2.TokenSource
}

type Token struct {
	AccessToken string
}

func (c Token) Token() (*oauth2.Token, error) {
	if c.AccessToken == "" {
		return nil, fmt.Errorf("access token is empty")
	}
	return &oauth2.Token{
		AccessToken: c.AccessToken,
	}, nil
}

func New(ctx context.Context, tokenSource oauth2.TokenSource) (*Client, error) {
	httpClient, err := uhttp.NewClient(ctx, uhttp.WithLogger(true, ctxzap.Extract(ctx)))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}
	httpClient.Transport = &openGraphHttpTransport{
		base:        httpClient.Transport,
		tokenSource: tokenSource,
	}
	return &Client{
		BaseHttpClient: uhttp.NewBaseHttpClient(httpClient),
		TokenSource:    tokenSource,
	}, nil
}

func (t *openGraphHttpTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.rateLimit = nil // clear previous

	// Get fresh token for each request
	token, err := t.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Set Authorization header with fresh token
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))

	resp, err := t.base.RoundTrip(request)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		// retry after a minute https://docs.ramp.com/developer-api/v1/overview/rate-limiting
		t.rateLimit = &v2.RateLimitDescription{
			Status:  v2.RateLimitDescription_STATUS_OVERLIMIT,
			ResetAt: timestamppb.New(time.Now().Add(time.Minute)),
		}
	}

	return resp, nil
}
