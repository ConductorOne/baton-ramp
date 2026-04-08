package client

import (
	"context"
	"fmt"
	"net/http"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

type CreateUserRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role,omitempty"`
	ManagerID string `json:"manager_id,omitempty"`
}

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

// POST https://api.ramp.com/developer/v1/users
// https://docs.ramp.com/developer-api/v1/api/users#post-developer-v1-users
func (c *Client) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, *v2.RateLimitDescription, error) {
	reqURL, err := c.newUnPaginatedURL("users", nil)
	if err != nil {
		return nil, nil, err
	}
	user := &User{}
	ratelimitData, err := c.queryWithBody(ctx, http.MethodPost, reqURL, req, user)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("ramp-connector: error creating user: %w", err)
	}
	return user, ratelimitData, nil
}

// DELETE https://api.ramp.com/developer/v1/users/{user_id}
// https://docs.ramp.com/developer-api/v1/api/users#delete-developer-v1-users-id
func (c *Client) DeleteUser(ctx context.Context, userID string) (*v2.RateLimitDescription, error) {
	reqURL, err := c.newUnPaginatedURL(fmt.Sprintf("users/%s", userID), nil)
	if err != nil {
		return nil, err
	}
	ratelimitData, err := c.queryWithBody(ctx, http.MethodDelete, reqURL, nil, nil)
	if err != nil {
		return ratelimitData, fmt.Errorf("ramp-connector: error deleting user %s: %w", userID, err)
	}
	return ratelimitData, nil
}
