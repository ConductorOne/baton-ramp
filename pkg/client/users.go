package client

import (
	"context"
	"fmt"
	"net/http"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

func (c *Client) ListUsers(ctx context.Context, pagination string) (*UsersResponse, *v2.RateLimitDescription, error) {
	users := &UsersList{}
	reqURL := pagination
	if reqURL == "" {
		var err error
		reqURL, err = c.newUnPaginatedURL("users", nil)
		if err != nil {
			return nil, nil, err
		}
	}

	ratelimitData, err := c.query(ctx, http.MethodGet, reqURL, users)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("ramp-client: error listing users %w", err)
	}
	rv := &UsersResponse{
		Users:      users.Users,
		Pagination: users.Page.Next,
	}
	return rv, ratelimitData, nil
}
