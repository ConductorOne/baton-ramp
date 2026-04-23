package client

import (
	"context"
	"fmt"
	"net/http"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/google/uuid"
)

const (
	usersEndpoint         = "users"
	usersDeferredEndpoint = "users/deferred"
)

type CreateUserRequest struct {
	Email          string `json:"email"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Role           string `json:"role"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (c *Client) ListUsers(ctx context.Context, pagination string) (*UsersResponse, *v2.RateLimitDescription, error) {
	users := &UsersList{}
	reqURL := pagination
	if reqURL == "" {
		var err error
		reqURL, err = c.newUnPaginatedURL(usersEndpoint)
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

// POST https://api.ramp.com/developer/v1/users/deferred
// https://docs.ramp.com/developer-api/v1/api/users#post-developer-v1-users-deferred
func (c *Client) CreateUser(ctx context.Context, req *CreateUserRequest) (*DeferredTaskResponse, *v2.RateLimitDescription, error) {
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = uuid.New().String()
	}
	reqURL, err := c.newUnPaginatedURL(usersDeferredEndpoint)
	if err != nil {
		return nil, nil, err
	}
	task := &DeferredTaskResponse{}
	ratelimitData, err := c.queryWithBody(ctx, http.MethodPost, reqURL, req, task)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("ramp-client: error creating user: %w", err)
	}
	return task, ratelimitData, nil
}

// PATCH https://api.ramp.com/developer/v1/users/{user_id}/deactivate
// https://docs.ramp.com/developer-api/v1/api/users#patch-developer-v1-users-user_id-deactivate
func (c *Client) DeactivateUser(ctx context.Context, userID string) (*v2.RateLimitDescription, error) {
	reqURL, err := c.newUnPaginatedURL(fmt.Sprintf("%s/%s/deactivate", usersEndpoint, userID))
	if err != nil {
		return nil, err
	}
	ratelimitData, err := c.query(ctx, http.MethodPatch, reqURL, nil)
	if err != nil {
		return ratelimitData, fmt.Errorf("ramp-client: error deactivating user %s: %w", userID, err)
	}
	return ratelimitData, nil
}

// PATCH https://api.ramp.com/developer/v1/users/{user_id}/reactivate
// https://docs.ramp.com/developer-api/v1/api/users#patch-developer-v1-users-user_id-reactivate
func (c *Client) ReactivateUser(ctx context.Context, userID string) (*v2.RateLimitDescription, error) {
	reqURL, err := c.newUnPaginatedURL(fmt.Sprintf("%s/%s/reactivate", usersEndpoint, userID))
	if err != nil {
		return nil, err
	}
	ratelimitData, err := c.query(ctx, http.MethodPatch, reqURL, nil)
	if err != nil {
		return ratelimitData, fmt.Errorf("ramp-client: error reactivating user %s: %w", userID, err)
	}
	return ratelimitData, nil
}

