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

const (
	roleIDAuditor             = "AUDITOR"
	roleIDBusinessAdmin       = "BUSINESS_ADMIN"
	roleIDBusinessBookkeeper  = "BUSINESS_BOOKKEEPER"
	roleIDBusinessOwner       = "BUSINESS_OWNER"
	roleIDBusinessUser        = "BUSINESS_USER"
	roleIDGuestUser           = "GUEST_USER"
	roleIDITAdmin             = "IT_ADMIN"
	roleIDUnbundledAdmin      = "UNBUNDLED_ADMIN"
	roleIDUnbundledBookkeeper = "UNBUNDLED_BOOKKEEPER"
	roleIDUnbundledOwner      = "UNBUNDLED_OWNER"
	roleIDUnbundledUser       = "UNBUNDLED_USER"

	roleNameAdmin               = "Admin"
	roleNameAuditor             = "Auditor"
	roleNameBookkeeper          = "Bookkeeper"
	roleNameGuest               = "Guest"
	roleNameITAdmin             = "IT Admin"
	roleNameOwner               = "Owner"
	roleNameUnbundledAdmin      = "Unbundled Admin"
	roleNameUnbundledBookkeeper = "Unbundled Bookkeeper"
	roleNameUnbundledOwner      = "Unbundled Owner"
	roleNameUnbundledUser       = "Unbundled User"
	roleNameUser                = "User"
)

// roles is the complete set of role values returned by GET /developer/v1/users.
// See https://docs.ramp.com/developer-api/v1/api/users for the role enum.
var roles = []client.Role{
	{
		ID:   roleIDBusinessAdmin,
		Name: roleNameAdmin,
	},
	{
		ID:   roleIDBusinessUser,
		Name: roleNameUser,
	},
	{
		ID:   roleIDBusinessOwner,
		Name: roleNameOwner,
	},
	{
		ID:   roleIDBusinessBookkeeper,
		Name: roleNameBookkeeper,
	},
	{
		ID:   roleIDITAdmin,
		Name: roleNameITAdmin,
	},
	{
		ID:   roleIDAuditor,
		Name: roleNameAuditor,
	},
	{
		ID:   roleIDGuestUser,
		Name: roleNameGuest,
	},
	{
		ID:   roleIDUnbundledAdmin,
		Name: roleNameUnbundledAdmin,
	},
	{
		ID:   roleIDUnbundledBookkeeper,
		Name: roleNameUnbundledBookkeeper,
	},
	{
		ID:   roleIDUnbundledOwner,
		Name: roleNameUnbundledOwner,
	},
	{
		ID:   roleIDUnbundledUser,
		Name: roleNameUnbundledUser,
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
	var annos annotations.Annotations

	roleID := normalizeRampRole(strings.TrimPrefix(resource.Id.Resource, "role:"))

	var usersResponse *client.UsersResponse
	var ratelimitData *v2.RateLimitDescription
	var err error
	if canFilterUsersByRole(roleID) {
		usersResponse, ratelimitData, err = o.client.ListUsersByRole(ctx, roleID, pToken.Token)
	} else {
		usersResponse, ratelimitData, err = o.client.ListUsers(ctx, pToken.Token)
	}
	if err != nil {
		return nil, "", nil, fmt.Errorf("baton-ramp: error listing users for role %s: %w", resource.Id.Resource, err)
	}
	annos.WithRateLimiting(ratelimitData)

	rv := []*v2.Grant{}
	for _, user := range usersResponse.Users {
		if roleID != normalizeRampRole(user.Role) {
			continue
		}

		resourceID, err := resourceSdk.NewResourceID(userResourceType, user.ID)
		if err != nil {
			return nil, "", annos, fmt.Errorf("baton-ramp: failed to create resource ID for user %s: %w", user.ID, err)
		}

		rv = append(rv, grant.NewGrant(
			resource,
			"member",
			resourceID,
		))
	}
	return rv, usersResponse.Pagination, annos, nil
}

func normalizeRampRole(role string) string {
	return strings.ToUpper(strings.TrimSpace(role))
}

func canFilterUsersByRole(role string) bool {
	switch normalizeRampRole(role) {
	case roleIDAuditor,
		roleIDBusinessAdmin,
		roleIDBusinessBookkeeper,
		roleIDBusinessOwner,
		roleIDBusinessUser,
		roleIDGuestUser,
		roleIDITAdmin:
		return true
	default:
		return false
	}
}

func isKnownRampRole(role string) bool {
	role = normalizeRampRole(role)
	for _, knownRole := range roles {
		if role == normalizeRampRole(knownRole.ID) {
			return true
		}
	}
	return false
}

func newRoleBuilder(client *client.Client) *roleBuilder {
	return &roleBuilder{
		client: client,
	}
}
