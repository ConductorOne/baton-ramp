package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	resourceSdk "github.com/conductorone/baton-sdk/pkg/types/resource"
)

// contractOwnerEntitlement is granted to each Ramp user listed in the
// agreement's contract_owners array. Read-only in v1 (no Grant/Revoke).
const contractOwnerEntitlement = "contract_owner"

type vendorAgreementBuilder struct {
	client     *client.Client
	businessID func() string
}

func (b *vendorAgreementBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return vendorAgreementResourceType
}

// List paginates through POST /developer/v1/vendors/agreements and emits
// one resource per agreement. Each resource carries both VendorTrait
// (vendor identity, with the agreement's deep link to the contract page)
// and VendorAgreementTrait (agreement-specific data: term, renewal
// status, total value).
//
// Trailing-window spend lives on the vendor resource's VendorTrait
// (see vendors.go), not on agreements — Ramp aggregates spend at the
// vendor level and SDK v0.8.31+ carries spend on VendorTrait
// accordingly. Consumers that want spend for a given agreement read it
// from the parent vendor's resource (joined by vendor_id_in_source).
//
// To populate VendorAgreementTrait's line_items we'd need a per-agreement
// single-fetch on top of the list response, and Ramp's spec types
// line_items as opaque (`Record<string, unknown>[]`); promoting to typed
// LineItems requires probing a real tenant. Tracked as a follow-up.
func (b *vendorAgreementBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var annos annotations.Annotations

	resp, ratelimitData, err := b.client.ListVendorAgreements(ctx, &client.VendorAgreementsListRequest{}, pToken.Token)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return nil, "", annos, fmt.Errorf("baton-ramp: failed to list vendor agreements: %w", err)
	}

	rv := make([]*v2.Resource, 0, len(resp.Agreements))
	for _, agreement := range resp.Agreements {
		resource, err := b.buildAgreementResource(agreement)
		if err != nil {
			return nil, "", annos, fmt.Errorf("baton-ramp: failed to build agreement resource %s: %w", agreement.ID, err)
		}
		rv = append(rv, resource)
	}
	return rv, resp.Pagination, annos, nil
}

// Entitlements declares the contract_owner entitlement, granted to Ramp
// users who appear in the agreement's contract_owners array.
func (b *vendorAgreementBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return []*v2.Entitlement{
		entitlement.NewAssignmentEntitlement(
			resource,
			contractOwnerEntitlement,
			entitlement.WithDescription(fmt.Sprintf("Contract owner of vendor agreement %s", resource.DisplayName)),
			entitlement.WithDisplayName(fmt.Sprintf("Is contract owner of %s in Ramp", resource.DisplayName)),
			entitlement.WithGrantableTo(userResourceType),
		),
	}, "", nil, nil
}

// Grants reads the agreement's contract_owners and emits one Grant per
// owner. Requires a per-agreement GET because the list endpoint only
// returns owner emails/ids inside the listing — we re-call to ensure we
// see the full array even on agreements where the list response was
// truncated.
//
// Note: the list response *does* include contract_owners, so this could
// avoid the extra round-trip in the common case. We re-fetch to keep the
// shape consistent across List / Grants and to read the richer single-
// fetch payee data that becomes useful in v1.5+. The per-agreement
// 1 req cost is bounded by the 50 req/min vendor-management surface
// rate-limit budget.
func (b *vendorAgreementBuilder) Grants(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	var annos annotations.Annotations
	agreement, ratelimitData, err := b.client.GetVendorAgreement(ctx, resource.Id.Resource)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return nil, "", annos, fmt.Errorf("baton-ramp: failed to get agreement %s: %w", resource.Id.Resource, err)
	}

	grants := make([]*v2.Grant, 0, len(agreement.ContractOwners))
	for _, owner := range agreement.ContractOwners {
		if owner.ID == "" {
			continue
		}
		principalID, err := resourceSdk.NewResourceID(userResourceType, owner.ID)
		if err != nil {
			return nil, "", annos, fmt.Errorf("baton-ramp: failed to build resource id for contract owner %s: %w", owner.ID, err)
		}
		grants = append(grants, grant.NewGrant(resource, contractOwnerEntitlement, principalID))
	}
	return grants, "", annos, nil
}

// buildAgreementResource constructs a vendor_agreement resource from the
// list-response shape. Both VendorTrait (identity, deep link to the
// agreement page) and VendorAgreementTrait (agreement payload) are
// attached. Trailing-window spend is NOT on the agreement; consumers
// read it from the parent vendor's resource.
func (b *vendorAgreementBuilder) buildAgreementResource(agreement *client.VendorAgreementListItem) (*v2.Resource, error) {
	vendorTraitOption, err := buildAgreementVendorTraitOption(agreement, b.businessID)
	if err != nil {
		return nil, fmt.Errorf("vendor trait: %w", err)
	}
	agreementTraitOption, err := buildAgreementTraitOption(agreement)
	if err != nil {
		return nil, fmt.Errorf("agreement trait: %w", err)
	}
	return resourceSdk.NewResource(
		agreement.Name,
		vendorAgreementResourceType,
		agreement.ID,
		resourceSdk.WithExternalID(&v2.ExternalId{Id: agreement.ID}),
		vendorTraitOption,
		agreementTraitOption,
	)
}

// buildAgreementVendorTraitOption fills VendorTrait from an agreement.
// vendor_id  ← agreement.PayeeID
// vendor_name ← agreement.PayeeName
// deep_link_url ← https://app.ramp.com/contracts/<agreement_id>
//
// We use the agreement's deep link (not the vendor's), so VendorTrait on
// an agreement points at the agreement page in Ramp. The companion
// VendorTrait emitted on the parent vendor resource (see vendors.go)
// points at the vendor page.
func buildAgreementVendorTraitOption(agreement *client.VendorAgreementListItem, businessIDFn func() string) (resourceSdk.ResourceOption, error) {
	vendorName := agreement.PayeeName
	if vendorName == "" {
		// PayeeName is required per spec, but be defensive: a missing name
		// would crash the trait constructor's required-field check.
		vendorName = agreement.Name
	}
	traitOpts := []resourceSdk.VendorTraitOption{
		resourceSdk.WithVendorIdentity(agreement.PayeeID, vendorName, ""),
		resourceSdk.WithVendorDeepLinkURL(agreementDeepLinkURL(agreement.ID)),
	}
	businessID := ""
	if businessIDFn != nil {
		businessID = businessIDFn()
	}
	if businessID != "" {
		traitOpts = append(traitOpts, resourceSdk.WithVendorSourceScoping(businessID, ""))
	}
	return resourceSdk.WithVendorTrait(traitOpts...), nil
}

// buildAgreementTraitOption fills VendorAgreementTrait from a Ramp
// agreement list item.
//
// Spend is intentionally NOT emitted on the agreement: spend is a
// vendor-level property in Ramp (and per the SDK trait split, spend
// fields live on VendorTrait, not VendorAgreementTrait). Consumers
// that want spend for an agreement read it from the parent vendor's
// resource.
func buildAgreementTraitOption(agreement *client.VendorAgreementListItem) (resourceSdk.ResourceOption, error) {
	startDate, err := parseRampDate(agreement.StartDate)
	if err != nil {
		return nil, fmt.Errorf("start_date: %w", err)
	}
	endDate, err := parseRampDate(agreement.EndDate)
	if err != nil {
		return nil, fmt.Errorf("end_date: %w", err)
	}
	lastDate, err := parseRampDate(agreement.LastDateToTerminate)
	if err != nil {
		return nil, fmt.Errorf("last_date_to_terminate: %w", err)
	}

	traitOpts := []resourceSdk.VendorAgreementTraitOption{
		resourceSdk.WithAgreementTerm(agreement.Name, startDate, endDate, agreement.AutoRenewal),
	}
	if !lastDate.IsZero() {
		traitOpts = append(traitOpts, resourceSdk.WithLastDateToTerminate(lastDate))
	}
	if status := mapRampRenewalStatus(agreement.RenewalStatus); status != v2.VendorAgreementTrait_RENEWAL_STATUS_UNSPECIFIED || agreement.RenewalStatus != "" {
		traitOpts = append(traitOpts, resourceSdk.WithRenewalStatus(status, agreement.RenewalStatus))
	}
	if agreement.TotalValue != nil && agreement.TotalValue.CurrencyCode != "" {
		traitOpts = append(traitOpts, resourceSdk.WithTotalValue(rampMoneyToTraitMoney(agreement.TotalValue)))
	}
	// line_items: not emitted in v1. The list response doesn't include
	// them; the GET single-fetch returns them as opaque
	// `Record<string, unknown>[]` per Ramp spec. Promoting them to a
	// typed LineItem requires probing a real tenant.
	// pricing_model: not derivable from the API.
	// external_account_manager_*: Ramp doesn't expose these on the
	// agreement; reserved for sources like Vanta that do.
	// trailing_*_spend / ytd_spend: SDK split puts these on VendorTrait,
	// not VendorAgreementTrait. Emitted on the vendor resource only.
	return resourceSdk.WithVendorAgreementTrait(traitOpts...), nil
}

// mapRampRenewalStatus maps Ramp's six-value enum to the trait's enum.
// Identity mapping; both sides have the same six values. Returns
// UNSPECIFIED when the source value is empty or unknown — the caller
// should still set renewal_status_raw to preserve the literal.
func mapRampRenewalStatus(raw string) v2.VendorAgreementTrait_RenewalStatus {
	switch raw {
	case "NOT_STARTED":
		return v2.VendorAgreementTrait_RENEWAL_STATUS_NOT_STARTED
	case "INITIATED":
		return v2.VendorAgreementTrait_RENEWAL_STATUS_INITIATED
	case "RENEWED":
		return v2.VendorAgreementTrait_RENEWAL_STATUS_RENEWED
	case "CANCELLED":
		return v2.VendorAgreementTrait_RENEWAL_STATUS_CANCELLED
	case "REJECTED":
		return v2.VendorAgreementTrait_RENEWAL_STATUS_REJECTED
	case "EXPIRED":
		return v2.VendorAgreementTrait_RENEWAL_STATUS_EXPIRED
	}
	return v2.VendorAgreementTrait_RENEWAL_STATUS_UNSPECIFIED
}

// rampMoneyToTraitMoney converts a Ramp Money to the trait's Money,
// translating major-unit decimal to int64 minor units. Returns nil when
// the source has no currency code (i.e. the field is structurally absent
// from the response, which can happen when no spend data is recorded).
func rampMoneyToTraitMoney(m *client.Money) *v2.Money {
	if m == nil || m.CurrencyCode == "" {
		return nil
	}
	return resourceSdk.NewMoney(m.MinorUnits(), m.CurrencyCode)
}

// parseRampDate parses Ramp's ISO 8601 date strings. Ramp returns dates
// in two formats: full RFC 3339 timestamps and date-only "YYYY-MM-DD".
// Both round-trip to a midnight-UTC time.Time. Empty input yields the
// zero time.Time.
func parseRampDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unrecognized date format %q", s)
}

// agreementDeepLinkURL constructs the canonical Ramp app URL for an
// agreement. Format observed and stable; consumers validate via per-
// source-provider host allowlist.
func agreementDeepLinkURL(agreementID string) string {
	if agreementID == "" {
		return ""
	}
	return fmt.Sprintf("https://app.ramp.com/contracts/%s", agreementID)
}

func newVendorAgreementBuilder(c *client.Client, businessID func() string) *vendorAgreementBuilder {
	return &vendorAgreementBuilder{client: c, businessID: businessID}
}
