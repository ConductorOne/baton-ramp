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

const userResourceTypeDisplayName = "User"

// The user resource type is for all user objects from the database.
var userResourceType = &v2.ResourceType{
	Id:          "user",
	DisplayName: userResourceTypeDisplayName,
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
	Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_VENDOR},
	Annotations: annotations.New(
		capabilityPermissions("vendors:read", "vendors:write"),
	),
}

// vendor_agreement carries both TRAIT_VENDOR (cross-system identity, since
// every agreement names the vendor it covers) and TRAIT_VENDOR_AGREEMENT
// (the agreement payload). Resource ID is the Ramp agreement UUID.
var vendorAgreementResourceType = &v2.ResourceType{
	Id:          "vendor_agreement",
	DisplayName: "Vendor Agreement",
	Traits: []v2.ResourceType_Trait{
		v2.ResourceType_TRAIT_VENDOR,
		v2.ResourceType_TRAIT_VENDOR_AGREEMENT,
	},
	Annotations: annotations.New(
		capabilityPermissions("vendors:read"),
		&v2.OptInRequired{},
	),
}
