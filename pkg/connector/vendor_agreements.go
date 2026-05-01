package connector

import (
	"context"
	"fmt"
	"sync"
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

// vendorSpend captures the trailing-window spend Ramp pre-aggregates on
// each vendor. Mirrored on every agreement under that vendor so consumers
// reading VendorAgreementTrait.trailing_*_spend get the source's
// authoritative numbers without needing to join back to the parent vendor.
//
// Ramp aggregates spend at the vendor (not agreement) level. When a single
// vendor has multiple agreements, every agreement carries the same spend
// triple. Consumers that aggregate spend should dedupe by vendor_id (which
// is in VendorTrait on the same resource) before summing.
type vendorSpend struct {
	Trailing30Day  *client.Money
	Trailing365Day *client.Money
	YTD            *client.Money
}

type vendorAgreementBuilder struct {
	client     *client.Client
	businessID func() string

	// vendorSpendCache memoizes per-vendor spend, populated on the first
	// call to List by paginating /developer/v1/vendors. Subsequent List
	// calls reuse the cache for the duration of the sync. Concurrency-
	// safe via sync.Mutex; loaded under sync.Once with err propagation.
	vendorSpendCacheOnce sync.Once
	vendorSpendCacheMu   sync.RWMutex
	vendorSpendCache     map[string]vendorSpend
	vendorSpendCacheErr  error
}

func (b *vendorAgreementBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return vendorAgreementResourceType
}

// List paginates through POST /developer/v1/vendors/agreements and emits
// one resource per agreement. Each resource carries both VendorTrait and
// VendorAgreementTrait — agreements name a vendor (so identity is
// available) and carry agreement-specific data, including the parent
// vendor's trailing-window spend.
//
// On the first call per sync, ensureVendorSpendCache pre-fetches every
// vendor (paginated) so each agreement can be enriched with its parent
// vendor's spend without per-agreement vendor lookups. The cache is
// reused across subsequent pages.
//
// To populate VendorAgreementTrait's line_items we'd need a per-agreement
// single-fetch on top of the list response, and Ramp's spec types
// line_items as opaque (`Record<string, unknown>[]`); promoting to typed
// LineItems requires probing a real tenant. Tracked as a follow-up.
func (b *vendorAgreementBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var annos annotations.Annotations

	rateLimitFromCacheLoad, err := b.ensureVendorSpendCache(ctx)
	for _, rl := range rateLimitFromCacheLoad {
		annos.WithRateLimiting(rl)
	}
	if err != nil {
		return nil, "", annos, err
	}

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

// ensureVendorSpendCache is a sync.Once-guarded loader that paginates the
// vendor list and builds the spend map. Returns the rate-limit annotations
// observed during load (potentially many, one per page) so the caller can
// surface them. Returns an error only on the first loading goroutine; later
// callers see cached state.
func (b *vendorAgreementBuilder) ensureVendorSpendCache(ctx context.Context) ([]*v2.RateLimitDescription, error) {
	var rls []*v2.RateLimitDescription
	b.vendorSpendCacheOnce.Do(func() {
		cache := make(map[string]vendorSpend)
		pagination := ""
		for {
			resp, rl, err := b.client.ListVendors(ctx, pagination)
			if rl != nil {
				rls = append(rls, rl)
			}
			if err != nil {
				b.vendorSpendCacheErr = fmt.Errorf("baton-ramp: failed to load vendor spend cache: %w", err)
				return
			}
			for _, v := range resp.Vendors {
				if v == nil || v.ID == "" {
					continue
				}
				cache[v.ID] = vendorSpend{
					Trailing30Day:  v.TotalSpendLast30Days,
					Trailing365Day: v.TotalSpendLast365Days,
					YTD:            v.TotalSpendYTD,
				}
			}
			if resp.Pagination == "" {
				break
			}
			pagination = resp.Pagination
		}
		b.vendorSpendCacheMu.Lock()
		b.vendorSpendCache = cache
		b.vendorSpendCacheMu.Unlock()
	})
	return rls, b.vendorSpendCacheErr
}

// vendorSpendFor returns the cached spend for a vendor id, or the zero
// vendorSpend when not present (which propagates as nil Money in the
// trait — none of the WithTrailing*Spend options will fire).
func (b *vendorAgreementBuilder) vendorSpendFor(vendorID string) vendorSpend {
	b.vendorSpendCacheMu.RLock()
	defer b.vendorSpendCacheMu.RUnlock()
	if b.vendorSpendCache == nil {
		return vendorSpend{}
	}
	return b.vendorSpendCache[vendorID]
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
// list-response shape. Both VendorTrait (identity) and VendorAgreementTrait
// (agreement payload, including the parent vendor's trailing-window spend
// from the cache) are attached.
func (b *vendorAgreementBuilder) buildAgreementResource(agreement *client.VendorAgreementListItem) (*v2.Resource, error) {
	vendorTraitOption, err := buildAgreementVendorTraitOption(agreement, b.businessID)
	if err != nil {
		return nil, fmt.Errorf("vendor trait: %w", err)
	}
	spend := b.vendorSpendFor(agreement.PayeeID)
	agreementTraitOption, err := buildAgreementTraitOption(agreement, spend)
	if err != nil {
		return nil, fmt.Errorf("agreement trait: %w", err)
	}
	return resourceSdk.NewResource(
		agreement.Name,
		vendorAgreementResourceType,
		agreement.ID,
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
// agreement list item plus its parent vendor's trailing-window spend.
//
// The spend triple comes from the parent vendor (Ramp aggregates spend
// at the vendor, not the agreement, level). Every agreement under the
// same vendor will carry the same spend numbers; consumers that
// aggregate spend across agreements should dedupe by VendorTrait.vendor_id
// before summing.
func buildAgreementTraitOption(agreement *client.VendorAgreementListItem, spend vendorSpend) (resourceSdk.ResourceOption, error) {
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
	if m := rampMoneyToTraitMoney(spend.Trailing30Day); m != nil {
		traitOpts = append(traitOpts, resourceSdk.WithTrailing30DaySpend(m))
	}
	if m := rampMoneyToTraitMoney(spend.Trailing365Day); m != nil {
		traitOpts = append(traitOpts, resourceSdk.WithTrailing365DaySpend(m))
	}
	if m := rampMoneyToTraitMoney(spend.YTD); m != nil {
		traitOpts = append(traitOpts, resourceSdk.WithYTDSpend(m))
	}
	// line_items: not emitted in v1. The list response doesn't include
	// them; the GET single-fetch returns them as opaque
	// `Record<string, unknown>[]` per Ramp spec. Promoting them to a
	// typed LineItem requires probing a real tenant.
	// pricing_model: not derivable from the API.
	// external_account_manager_*: Ramp doesn't expose these on the
	// agreement; reserved for sources like Vanta that do.
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
	return fmt.Sprintf("https://app.ramp.com/contracts/%s", agreementID)
}

func newVendorAgreementBuilder(c *client.Client, businessID func() string) *vendorAgreementBuilder {
	return &vendorAgreementBuilder{client: c, businessID: businessID}
}
