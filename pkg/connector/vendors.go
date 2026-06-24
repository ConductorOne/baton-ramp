package connector

import (
	"context"
	"fmt"
	"sync"

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
	client         *client.Client
	vendorOwnersMu sync.RWMutex
	vendorOwners   map[string]string
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

	o.vendorOwnersMu.Lock()
	if pToken.Token == "" || o.vendorOwners == nil {
		o.vendorOwners = make(map[string]string)
	}
	o.vendorOwnersMu.Unlock()

	rv := make([]*v2.Resource, 0, len(resp.Vendors))
	for _, vendor := range resp.Vendors {
		o.vendorOwnersMu.Lock()
		o.vendorOwners[vendor.ID] = vendor.VendorOwnerID
		o.vendorOwnersMu.Unlock()
		resource, err := vendorResource(vendor)
		if err != nil {
			return nil, "", annos, fmt.Errorf("baton-ramp: failed to create vendor resource %s: %w", vendor.ID, err)
		}
		rv = append(rv, resource)
	}
	return rv, resp.Pagination, annos, nil
}

func (o *vendorBuilder) Get(ctx context.Context, resourceID *v2.ResourceId, _ *v2.ResourceId) (*v2.Resource, annotations.Annotations, error) {
	var annos annotations.Annotations
	vendor, ratelimitData, err := o.client.GetVendor(ctx, resourceID.GetResource())
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return nil, annos, fmt.Errorf("baton-ramp: failed to get vendor %s: %w", resourceID.GetResource(), err)
	}
	resource, err := vendorResource(vendor)
	if err != nil {
		return nil, annos, fmt.Errorf("baton-ramp: failed to create vendor resource %s: %w", vendor.ID, err)
	}
	return resource, annos, nil
}

// buildVendorTraitOption assembles a VendorTrait for a Ramp vendor.
//
// vendor_id           ← Ramp vendor.ID
// vendor_name         ← Ramp vendor.NameLegal (or Name when NameLegal is
//
//	empty — Ramp's Name is the friendly/trade name)
//
// vendor_dba_name     ← Ramp vendor.Name (when distinct from NameLegal)
// website_domain      ← (empty — Ramp's vendor list response doesn't
//
//	carry a website field; payee.website on
//	agreements does)
//
// external_vendor_id  ← Ramp vendor.ExternalVendorID
// deep_link_url       ← https://app.ramp.com/vendors/<id> (constructed)
// source_business_id  ← empty (the connector does not require business:read)
// source_entity_id    ← Ramp vendor.DefaultEntityID
// trailing_30d_spend  ← Ramp vendor.TotalSpendLast30Days
// trailing_365d_spend ← Ramp vendor.TotalSpendLast365Days
// ytd_spend           ← Ramp vendor.TotalSpendYTD.
func buildVendorTraitOption(vendor *client.Vendor) (resourceSdk.ResourceOption, error) {
	// In Ramp's data model, Name is the friendly/trade name and NameLegal
	// (when distinct) is the legal name. The trait's vendor_name is the
	// legal name, vendor_dba_name is the trade name. Swap accordingly.
	legalName := vendor.NameLegal
	if legalName == "" {
		legalName = vendor.Name
	}
	dbaName := ""
	if vendor.NameLegal != "" && vendor.NameLegal != vendor.Name {
		dbaName = vendor.Name
	}

	traitOpts := []resourceSdk.VendorTraitOption{
		resourceSdk.WithVendorIdentity(vendor.ID, legalName, dbaName),
		resourceSdk.WithVendorDeepLinkURL(vendorDeepLinkURL(vendor.ID)),
	}
	if vendor.ExternalVendorID != "" {
		traitOpts = append(traitOpts, resourceSdk.WithExternalVendorID(vendor.ExternalVendorID))
	}
	if vendor.DefaultEntityID != "" {
		traitOpts = append(traitOpts, resourceSdk.WithVendorSourceScoping("", vendor.DefaultEntityID))
	}

	// Trailing-window spend lives on VendorTrait. Each Ramp vendor list
	// response carries pre-aggregated trailing-30/365/YTD spend on the
	// vendor itself — no per-agreement aggregation needed.
	if m := rampMoneyToTraitMoney(vendor.TotalSpendLast30Days); m != nil {
		traitOpts = append(traitOpts, resourceSdk.WithTrailing30DaySpend(m))
	}
	if m := rampMoneyToTraitMoney(vendor.TotalSpendLast365Days); m != nil {
		traitOpts = append(traitOpts, resourceSdk.WithTrailing365DaySpend(m))
	}
	if m := rampMoneyToTraitMoney(vendor.TotalSpendYTD); m != nil {
		traitOpts = append(traitOpts, resourceSdk.WithYTDSpend(m))
	}
	return resourceSdk.WithVendorTrait(traitOpts...), nil
}

// vendorDeepLinkURL constructs the canonical Ramp app URL for a vendor.
// Ramp does not expose a per-vendor URL in API responses; the format is
// observed and stable. Validated by consumers via per-source-provider
// host allowlist.
func vendorDeepLinkURL(vendorID string) string {
	if vendorID == "" {
		return ""
	}
	return fmt.Sprintf("https://app.ramp.com/vendors/%s", vendorID)
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
	vendorID := resource.Id.Resource
	o.vendorOwnersMu.RLock()
	ownerID, ok := o.vendorOwners[vendorID]
	o.vendorOwnersMu.RUnlock()
	if !ok {
		vendor, ratelimitData, err := o.client.GetVendor(ctx, vendorID)
		annos.WithRateLimiting(ratelimitData)
		if err != nil {
			return nil, "", annos, fmt.Errorf("baton-ramp: failed to get vendor %s: %w", vendorID, err)
		}
		ownerID = vendor.VendorOwnerID
	}
	if ownerID == "" {
		return nil, "", annos, nil
	}
	principalID, err := resourceSdk.NewResourceID(userResourceType, ownerID)
	if err != nil {
		return nil, "", annos, fmt.Errorf("baton-ramp: failed to create resource ID for vendor owner %s: %w", ownerID, err)
	}
	return []*v2.Grant{
		grant.NewGrant(resource, vendorOwnerEntitlement, principalID),
	}, "", annos, nil
}

// Ramp vendors have a single owner. Granting the owner entitlement to a new
// principal overwrites any existing owner via PATCH. Revoke only clears the
// owner when the current owner matches the principal being revoked.
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

func newVendorBuilder(c *client.Client) *vendorBuilder {
	return &vendorBuilder{
		client: c,
	}
}

func vendorResource(vendor *client.Vendor) (*v2.Resource, error) {
	traitOption, err := buildVendorTraitOption(vendor)
	if err != nil {
		return nil, fmt.Errorf("baton-ramp: failed to build vendor trait for %s: %w", vendor.ID, err)
	}
	opts := []resourceSdk.ResourceOption{
		resourceSdk.WithExternalID(&v2.ExternalId{Id: vendor.ID}),
		traitOption,
	}

	return resourceSdk.NewResource(
		vendor.Name,
		vendorResourceType,
		vendor.ID,
		opts...,
	)
}
