package connector

import (
	"context"
	"fmt"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	resourceSdk "github.com/conductorone/baton-sdk/pkg/types/resource"
)

type userBuilder struct {
	client *client.Client
}

func (o *userBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return userResourceType
}

func userResource(u *client.User) (*v2.Resource, error) {
	status := v2.UserTrait_Status_STATUS_ENABLED
	if u.Status != "USER_ACTIVE" && u.Status != "USER_ONBOARDING" {
		status = v2.UserTrait_Status_STATUS_DISABLED
	}

	return resourceSdk.NewResource(
		fmt.Sprintf("%s %s", u.FirstName, u.LastName),
		userResourceType,
		u.ID,
		resourceSdk.WithUserTrait(
			resourceSdk.WithEmail(u.Email, true),
			resourceSdk.WithStatus(status),
		),
	)
}

// List returns all the users from the database as resource objects.
// Users include a UserTrait because they are the 'shape' of a standard user.
func (o *userBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var annos annotations.Annotations

	usersResponse, ratelimitData, err := o.client.ListUsers(ctx, pToken.Token)
	if err != nil {
		return nil, "", nil, err
	}
	annos.WithRateLimiting(ratelimitData)

	rv := make([]*v2.Resource, 0, len(usersResponse.Users))
	for _, u := range usersResponse.Users {
		resource, err := userResource(u)
		if err != nil {
			return nil, "", annos, fmt.Errorf("baton-ramp: failed to create resource for user %s: %w", u.ID, err)
		}
		rv = append(rv, resource)
	}

	return rv, usersResponse.Pagination, annos, nil
}

// Entitlements always returns an empty slice for users.
func (o *userBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

// Grants always returns an empty slice for users since they don't have any entitlements.
func (o *userBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func newUserBuilder(client *client.Client) *userBuilder {
	return &userBuilder{
		client: client,
	}
}
