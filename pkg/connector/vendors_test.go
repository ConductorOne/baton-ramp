package connector

import (
	"testing"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	resourceSdk "github.com/conductorone/baton-sdk/pkg/types/resource"
)

func TestVendorResourceTypeUsesVendorTrait(t *testing.T) {
	if !resourceSdk.IsVendorResource(vendorResourceType) {
		t.Fatal("expected vendor resource type to declare TRAIT_VENDOR")
	}
}

func TestVendorResourceIncludesVendorTrait(t *testing.T) {
	vendor := &client.Vendor{
		ID:   "vendor-123",
		Name: "Acme",
	}

	resource, err := vendorResource(vendor, false, nil)
	if err != nil {
		t.Fatalf("expected vendor resource, got error: %v", err)
	}

	trait, err := resourceSdk.GetVendorTrait(resource)
	if err != nil {
		t.Fatalf("expected vendor trait, got error: %v", err)
	}
	if trait.GetVendorId() != vendor.ID {
		t.Fatalf("expected vendor ID %q, got %q", vendor.ID, trait.GetVendorId())
	}
	if trait.GetVendorName() != vendor.Name {
		t.Fatalf("expected vendor name %q, got %q", vendor.Name, trait.GetVendorName())
	}
}

// fixedBusinessID returns a function-typed source for a static business
// id, matching the signature buildVendorTraitOption expects.
func fixedBusinessID(s string) func() string {
	return func() string { return s }
}

// TestBuildVendorTraitOption_FullPopulation walks every field on
// VendorTrait through buildVendorTraitOption to confirm round-trip,
// including the trailing-window spend that lives on VendorTrait (not
// VendorAgreementTrait) since SDK v0.8.31.
func TestBuildVendorTraitOption_FullPopulation(t *testing.T) {
	vendor := &client.Vendor{
		ID:                    "vendor-1",
		Name:                  "Acme",
		NameLegal:             "Acme Software, Inc.",
		ExternalVendorID:      "acme-prod",
		DefaultEntityID:       "entity-1",
		TotalSpendLast30Days:  &client.Money{Amount: 1000, CurrencyCode: "USD", MinorUnitConversionRate: 100},
		TotalSpendLast365Days: &client.Money{Amount: 12000, CurrencyCode: "USD", MinorUnitConversionRate: 100},
		TotalSpendYTD:         &client.Money{Amount: 2000, CurrencyCode: "USD", MinorUnitConversionRate: 100},
	}

	opt, err := buildVendorTraitOption(vendor, fixedBusinessID("biz-1"))
	if err != nil {
		t.Fatal(err)
	}

	rt := &v2.ResourceType{}
	rt.SetTraits([]v2.ResourceType_Trait{v2.ResourceType_TRAIT_VENDOR})
	r, err := resourceSdk.NewResource(vendor.Name, rt, vendor.ID, opt)
	if err != nil {
		t.Fatal(err)
	}

	trait, err := resourceSdk.GetVendorTrait(r)
	if err != nil {
		t.Fatal(err)
	}

	// Identity: legal name -> vendor_name, trade name -> dba.
	if got := trait.GetVendorName(); got != "Acme Software, Inc." {
		t.Fatalf("vendor_name = %q, want %q", got, "Acme Software, Inc.")
	}
	if got := trait.GetVendorDbaName(); got != "Acme" {
		t.Fatalf("vendor_dba_name = %q, want %q", got, "Acme")
	}
	if got := trait.GetExternalVendorId(); got != "acme-prod" {
		t.Fatalf("external_vendor_id = %q, want %q", got, "acme-prod")
	}
	if got := trait.GetDeepLinkUrl(); got != "https://app.ramp.com/vendors/vendor-1" {
		t.Fatalf("deep_link_url = %q", got)
	}
	if got := trait.GetSourceBusinessId(); got != "biz-1" {
		t.Fatalf("source_business_id = %q", got)
	}
	if got := trait.GetSourceEntityId(); got != "entity-1" {
		t.Fatalf("source_entity_id = %q", got)
	}

	// Spend: USD major-unit decimals -> int64 minor units.
	if got := trait.GetTrailing_30DSpend(); got == nil || got.GetAmountMinor() != 100_000 || got.GetCurrencyCode() != "USD" {
		t.Fatalf("trailing 30d spend = %+v, want amount_minor=100000 USD", got)
	}
	if got := trait.GetTrailing_365DSpend(); got == nil || got.GetAmountMinor() != 1_200_000 || got.GetCurrencyCode() != "USD" {
		t.Fatalf("trailing 365d spend = %+v, want amount_minor=1200000 USD", got)
	}
	if got := trait.GetYtdSpend(); got == nil || got.GetAmountMinor() != 200_000 || got.GetCurrencyCode() != "USD" {
		t.Fatalf("ytd spend = %+v, want amount_minor=200000 USD", got)
	}
}

// TestBuildVendorTraitOption_NoSpend verifies that a vendor with no spend
// data yields a trait with no trailing-window options set.
func TestBuildVendorTraitOption_NoSpend(t *testing.T) {
	vendor := &client.Vendor{
		ID:        "vendor-1",
		Name:      "Acme",
		NameLegal: "Acme Software, Inc.",
	}

	opt, err := buildVendorTraitOption(vendor, nil)
	if err != nil {
		t.Fatal(err)
	}

	rt := &v2.ResourceType{}
	rt.SetTraits([]v2.ResourceType_Trait{v2.ResourceType_TRAIT_VENDOR})
	r, err := resourceSdk.NewResource(vendor.Name, rt, vendor.ID, opt)
	if err != nil {
		t.Fatal(err)
	}

	trait, err := resourceSdk.GetVendorTrait(r)
	if err != nil {
		t.Fatal(err)
	}

	if trait.GetTrailing_30DSpend() != nil {
		t.Fatalf("expected nil 30d spend, got %+v", trait.GetTrailing_30DSpend())
	}
	if trait.GetTrailing_365DSpend() != nil {
		t.Fatalf("expected nil 365d spend, got %+v", trait.GetTrailing_365DSpend())
	}
	if trait.GetYtdSpend() != nil {
		t.Fatalf("expected nil YTD spend, got %+v", trait.GetYtdSpend())
	}
}

// TestBuildVendorTraitOption_PartialSpend verifies that only populated
// windows emit. A spend Money with empty currency_code is treated as
// missing and skipped (rampMoneyToTraitMoney guard).
func TestBuildVendorTraitOption_PartialSpend(t *testing.T) {
	vendor := &client.Vendor{
		ID:                   "vendor-1",
		Name:                 "Acme",
		NameLegal:            "Acme",
		TotalSpendLast30Days: &client.Money{Amount: 50, CurrencyCode: "USD", MinorUnitConversionRate: 100},
		// 365d intentionally nil.
		TotalSpendYTD: &client.Money{Amount: 0, CurrencyCode: ""}, // currency-empty: skipped
	}

	opt, err := buildVendorTraitOption(vendor, nil)
	if err != nil {
		t.Fatal(err)
	}

	rt := &v2.ResourceType{}
	rt.SetTraits([]v2.ResourceType_Trait{v2.ResourceType_TRAIT_VENDOR})
	r, err := resourceSdk.NewResource(vendor.Name, rt, vendor.ID, opt)
	if err != nil {
		t.Fatal(err)
	}

	trait, err := resourceSdk.GetVendorTrait(r)
	if err != nil {
		t.Fatal(err)
	}

	if got := trait.GetTrailing_30DSpend(); got == nil || got.GetAmountMinor() != 5_000 {
		t.Fatalf("trailing 30d spend = %+v, want amount_minor=5000", got)
	}
	if trait.GetTrailing_365DSpend() != nil {
		t.Fatalf("expected nil 365d spend, got %+v", trait.GetTrailing_365DSpend())
	}
	if trait.GetYtdSpend() != nil {
		t.Fatalf("expected nil YTD spend (currency_code empty), got %+v", trait.GetYtdSpend())
	}
}
