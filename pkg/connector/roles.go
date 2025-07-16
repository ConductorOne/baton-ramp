package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	resourceSdk "github.com/conductorone/baton-sdk/pkg/types/resource"
)

type roleBuilder struct {
	client *client.Client
}

var roles = []client.Role{
	{
		ID:   "BUSINESS_ADMIN",
		Name: "Admin",
	},
	{
		ID:   "BUSINESS_USER",
		Name: "User",
	},
	{
		ID:   "BUSINESS_OWNER",
		Name: "Owner",
	},
	{
		ID:   "BUSINESS_BOOKKEEPER",
		Name: "Bookkeeper",
	},
	{
		ID:   "IT_ADMIN",
		Name: "IT Admin",
	},
}

func (o *roleBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return roleResourceType
}

func (o *roleBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	rv := make([]*v2.Resource, 0, len(roles))
	for _, role := range roles {
		resource, err := resourceSdk.NewResource(
			role.Name,
			roleResourceType,
			fmt.Sprintf("role:%s", role.ID),
			resourceSdk.WithRoleTrait(),
		)
		if err != nil {
			return nil, "", nil, err
		}
		rv = append(rv, resource)
	}
	return rv, "", nil, nil
}

func (o *roleBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return []*v2.Entitlement{
		entitlement.NewAssignmentEntitlement(
			resource,
			"member",
			entitlement.WithDescription(fmt.Sprintf("%s Role", resource.DisplayName)),
			entitlement.WithDisplayName(fmt.Sprintf("Has the %s role in Ramp", resource.DisplayName)),
		),
	}, "", nil, nil
}

func (o *roleBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	roleID := resource.Id.Resource

	usersResponse, ratelimitData, err := o.client.ListUsers(ctx, pToken.Token)
	if err != nil {
		return nil, "", nil, fmt.Errorf("baton-ramp: error listing users for role %s: %w", resource.Id.Resource, err)
	}
	var annos *annotations.Annotations
	annos = annos.WithRateLimiting(ratelimitData)

	rv := []*v2.Grant{}
	for _, user := range usersResponse.Users {
		if !strings.Contains(roleID, user.Role) {
			continue
		}

		resourceId, err := resourceSdk.NewResourceID(userResourceType, user.ID)
		if err != nil {
			return nil, "", *annos, fmt.Errorf("baton-ramp: failed to create resource ID for user %s: %w", user.ID, err)
		}

		rv = append(rv, grant.NewGrant(
			resource,
			"member",
			resourceId,
		))
	}
	return rv, usersResponse.Pagination, *annos, nil
}

func newRoleBuilder(client *client.Client) *roleBuilder {
	return &roleBuilder{
		client: client,
	}
}
