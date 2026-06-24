package connector

import (
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
)

func capabilityPermissions(perms ...string) *v2.CapabilityPermissions {
	cp := &v2.CapabilityPermissions{}
	for _, p := range perms {
		cp.Permissions = append(cp.Permissions, &v2.CapabilityPermission{Permission: p})
	}
	return cp
}

// Ramp OAuth scopes reference: https://docs.ramp.com/developer-api/v1/authorization

// The user resource type is for all user objects from the database.
var userResourceType = &v2.ResourceType{
	Id:          "user",
	DisplayName: roleNameUser,
	Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_USER},
	Annotations: annotations.New(
		capabilityPermissions("users:read", "users:write"),
	),
}

var roleResourceType = &v2.ResourceType{
	Id:          "role",
	DisplayName: "Role",
	Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_ROLE},
	Annotations: annotations.New(
		capabilityPermissions("users:read"),
	),
}

var vendorResourceType = &v2.ResourceType{
	Id:          "vendor",
	DisplayName: "Vendor",
	Annotations: annotations.New(
		capabilityPermissions("vendors:read", "vendors:write"),
	),
}
