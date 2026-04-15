package connector

import (
	"context"
	"fmt"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	resourceSdk "github.com/conductorone/baton-sdk/pkg/types/resource"
)

const vendorOwnerEntitlement = "owner"

type vendorBuilder struct {
	client *client.Client
}

func (o *vendorBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return vendorResourceType
}

func (o *vendorBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var annos annotations.Annotations
	resp, ratelimitData, err := o.client.ListVendors(ctx, pToken.Token)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return nil, "", annos, fmt.Errorf("baton-ramp: failed to list vendors: %w", err)
	}

	rv := make([]*v2.Resource, 0, len(resp.Vendors))
	for _, vendor := range resp.Vendors {
		resource, err := resourceSdk.NewResource(
			vendor.Name,
			vendorResourceType,
			vendor.ID,
		)
		if err != nil {
			return nil, "", annos, fmt.Errorf("baton-ramp: failed to create vendor resource %s: %w", vendor.ID, err)
		}
		rv = append(rv, resource)
	}
	return rv, resp.Pagination, annos, nil
}

func (o *vendorBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return []*v2.Entitlement{
		entitlement.NewAssignmentEntitlement(
			resource,
			vendorOwnerEntitlement,
			entitlement.WithDescription(fmt.Sprintf("Owner of vendor %s", resource.DisplayName)),
			entitlement.WithDisplayName(fmt.Sprintf("Is owner of %s in Ramp", resource.DisplayName)),
			entitlement.WithGrantableTo(userResourceType),
		),
	}, "", nil, nil
}

func (o *vendorBuilder) Grants(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	var annos annotations.Annotations
	vendor, ratelimitData, err := o.client.GetVendor(ctx, resource.Id.Resource)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return nil, "", annos, fmt.Errorf("baton-ramp: failed to get vendor %s: %w", resource.Id.Resource, err)
	}
	if vendor.VendorOwnerID == "" {
		return nil, "", annos, nil
	}
	principalID, err := resourceSdk.NewResourceID(userResourceType, vendor.VendorOwnerID)
	if err != nil {
		return nil, "", annos, fmt.Errorf("baton-ramp: failed to create resource ID for vendor owner %s: %w", vendor.VendorOwnerID, err)
	}
	return []*v2.Grant{
		grant.NewGrant(resource, vendorOwnerEntitlement, principalID),
	}, "", annos, nil
}

func (o *vendorBuilder) Grant(ctx context.Context, principal *v2.Resource, ent *v2.Entitlement) ([]*v2.Grant, annotations.Annotations, error) {
	vendorID := ent.Resource.Id.Resource
	userID := principal.Id.Resource
	var annos annotations.Annotations

	vendor, ratelimitData, err := o.client.GetVendor(ctx, vendorID)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return nil, annos, fmt.Errorf("baton-ramp: failed to get vendor %s: %w", vendorID, err)
	}
	if vendor.VendorOwnerID == userID {
		annos.Append(&v2.GrantAlreadyExists{})
		return nil, annos, nil
	}

	ratelimitData, err = o.client.UpdateVendorOwner(ctx, vendorID, userID)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return nil, annos, fmt.Errorf("baton-ramp: failed to grant vendor owner for vendor %s: %w", vendorID, err)
	}
	principalID, err := resourceSdk.NewResourceID(userResourceType, userID)
	if err != nil {
		return nil, annos, fmt.Errorf("baton-ramp: failed to create resource ID for user %s: %w", userID, err)
	}
	return []*v2.Grant{grant.NewGrant(ent.Resource, vendorOwnerEntitlement, principalID)}, annos, nil
}

func (o *vendorBuilder) Revoke(ctx context.Context, g *v2.Grant) (annotations.Annotations, error) {
	vendorID := g.Entitlement.Resource.Id.Resource
	userID := g.Principal.Id.Resource
	var annos annotations.Annotations

	vendor, ratelimitData, err := o.client.GetVendor(ctx, vendorID)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return annos, fmt.Errorf("baton-ramp: failed to get vendor %s: %w", vendorID, err)
	}
	if vendor.VendorOwnerID != userID {
		annos.Append(&v2.GrantAlreadyRevoked{})
		return annos, nil
	}

	ratelimitData, err = o.client.UpdateVendorOwner(ctx, vendorID, "")
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return annos, fmt.Errorf("baton-ramp: failed to revoke vendor owner for vendor %s: %w", vendorID, err)
	}
	return annos, nil
}

func newVendorBuilder(client *client.Client) *vendorBuilder {
	return &vendorBuilder{client: client}
}
