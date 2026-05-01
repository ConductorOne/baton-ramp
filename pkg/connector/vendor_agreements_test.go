package connector

import (
	"testing"
	"time"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	resourceSdk "github.com/conductorone/baton-sdk/pkg/types/resource"
)

func TestParseRampDate(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    time.Time
		wantErr bool
	}{
		{"empty", "", time.Time{}, false},
		{"date only", "2026-01-15", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"rfc3339", "2026-01-15T12:34:56Z", time.Date(2026, 1, 15, 12, 34, 56, 0, time.UTC), false},
		{"unrecognized", "01/15/2026", time.Time{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRampDate(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMapRampRenewalStatus(t *testing.T) {
	cases := []struct {
		in   string
		want v2.VendorAgreementTrait_RenewalStatus
	}{
		{"NOT_STARTED", v2.VendorAgreementTrait_RENEWAL_STATUS_NOT_STARTED},
		{"INITIATED", v2.VendorAgreementTrait_RENEWAL_STATUS_INITIATED},
		{"RENEWED", v2.VendorAgreementTrait_RENEWAL_STATUS_RENEWED},
		{"CANCELLED", v2.VendorAgreementTrait_RENEWAL_STATUS_CANCELLED},
		{"REJECTED", v2.VendorAgreementTrait_RENEWAL_STATUS_REJECTED},
		{"EXPIRED", v2.VendorAgreementTrait_RENEWAL_STATUS_EXPIRED},
		{"", v2.VendorAgreementTrait_RENEWAL_STATUS_UNSPECIFIED},
		{"UNKNOWN_NEW_VALUE", v2.VendorAgreementTrait_RENEWAL_STATUS_UNSPECIFIED},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := mapRampRenewalStatus(tc.in); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVendorDeepLinkURL(t *testing.T) {
	if got, want := vendorDeepLinkURL("v123"), "https://app.ramp.com/vendors/v123"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestAgreementDeepLinkURL(t *testing.T) {
	if got, want := agreementDeepLinkURL("a456"), "https://app.ramp.com/contracts/a456"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestIsAgreementURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"/contracts/abc", true},
		{"https://app.ramp.com/contracts/xyz", true},
		{"/vendors/v1", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := isAgreementURL(tc.in); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVendorSpendFor_EmptyCache(t *testing.T) {
	b := &vendorAgreementBuilder{}
	got := b.vendorSpendFor("any-vendor")
	if got.Trailing30Day != nil || got.Trailing365Day != nil || got.YTD != nil {
		t.Fatalf("expected zero vendorSpend, got %+v", got)
	}
}

func TestVendorSpendFor_PopulatedCache(t *testing.T) {
	b := &vendorAgreementBuilder{
		vendorSpendCache: map[string]vendorSpend{
			"vendor-1": {
				Trailing30Day:  &client.Money{Amount: 100, CurrencyCode: "USD", MinorUnitConversionRate: 100},
				Trailing365Day: &client.Money{Amount: 1200, CurrencyCode: "USD", MinorUnitConversionRate: 100},
			},
		},
	}
	got := b.vendorSpendFor("vendor-1")
	if got.Trailing30Day == nil || got.Trailing30Day.Amount != 100 {
		t.Fatalf("expected populated 30d spend, got %+v", got.Trailing30Day)
	}
	if got.Trailing365Day == nil || got.Trailing365Day.Amount != 1200 {
		t.Fatalf("expected populated 365d spend, got %+v", got.Trailing365Day)
	}
	if got.YTD != nil {
		t.Fatalf("expected nil YTD spend, got %+v", got.YTD)
	}

	// Missing vendor returns zero.
	missing := b.vendorSpendFor("vendor-not-cached")
	if missing.Trailing30Day != nil {
		t.Fatalf("expected nil for missing vendor, got %+v", missing)
	}
}

// TestBuildAgreementTraitOption_WithSpend verifies that when a vendorSpend
// is provided, all three trailing-window money fields land on the trait
// in minor units.
func TestBuildAgreementTraitOption_WithSpend(t *testing.T) {
	agreement := &client.VendorAgreementListItem{
		ID:            "agreement-1",
		Name:          "Annual Subscription 2026",
		StartDate:     "2026-01-01",
		EndDate:       "2026-12-31",
		AutoRenewal:   true,
		RenewalStatus: "NOT_STARTED",
		Currency:      "USD",
		PayeeID:       "vendor-1",
		PayeeName:     "Acme Software",
	}
	spend := vendorSpend{
		Trailing30Day:  &client.Money{Amount: 100, CurrencyCode: "USD", MinorUnitConversionRate: 100},
		Trailing365Day: &client.Money{Amount: 1200, CurrencyCode: "USD", MinorUnitConversionRate: 100},
		YTD:            &client.Money{Amount: 200, CurrencyCode: "USD", MinorUnitConversionRate: 100},
	}

	opt, err := buildAgreementTraitOption(agreement, spend)
	if err != nil {
		t.Fatal(err)
	}

	rt := &v2.ResourceType{}
	rt.SetTraits([]v2.ResourceType_Trait{v2.ResourceType_TRAIT_VENDOR_AGREEMENT})
	r, err := resourceSdk.NewResource(agreement.Name, rt, agreement.ID, opt)
	if err != nil {
		t.Fatal(err)
	}

	trait, err := resourceSdk.GetVendorAgreementTrait(r)
	if err != nil {
		t.Fatal(err)
	}

	if got := trait.GetTrailing_30DSpend(); got == nil || got.GetAmountMinor() != 10000 || got.GetCurrencyCode() != "USD" {
		t.Fatalf("trailing 30d spend = %+v, want amount_minor=10000 USD", got)
	}
	if got := trait.GetTrailing_365DSpend(); got == nil || got.GetAmountMinor() != 120000 || got.GetCurrencyCode() != "USD" {
		t.Fatalf("trailing 365d spend = %+v, want amount_minor=120000 USD", got)
	}
	if got := trait.GetYtdSpend(); got == nil || got.GetAmountMinor() != 20000 || got.GetCurrencyCode() != "USD" {
		t.Fatalf("ytd spend = %+v, want amount_minor=20000 USD", got)
	}
}

// TestBuildAgreementTraitOption_NoSpend verifies that when vendorSpend is
// the zero value (vendor not in the cache), no spend fields are emitted.
func TestBuildAgreementTraitOption_NoSpend(t *testing.T) {
	agreement := &client.VendorAgreementListItem{
		ID:            "agreement-1",
		Name:          "Annual Subscription 2026",
		StartDate:     "2026-01-01",
		EndDate:       "2026-12-31",
		RenewalStatus: "NOT_STARTED",
		Currency:      "USD",
		PayeeID:       "vendor-not-cached",
		PayeeName:     "Acme Software",
	}

	opt, err := buildAgreementTraitOption(agreement, vendorSpend{})
	if err != nil {
		t.Fatal(err)
	}

	rt := &v2.ResourceType{}
	rt.SetTraits([]v2.ResourceType_Trait{v2.ResourceType_TRAIT_VENDOR_AGREEMENT})
	r, err := resourceSdk.NewResource(agreement.Name, rt, agreement.ID, opt)
	if err != nil {
		t.Fatal(err)
	}

	trait, err := resourceSdk.GetVendorAgreementTrait(r)
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

// TestBuildAgreementTraitOption_PartialSpend verifies that when only one
// spend window is available (e.g. 30d but no 365d), only the populated
// option fires.
func TestBuildAgreementTraitOption_PartialSpend(t *testing.T) {
	agreement := &client.VendorAgreementListItem{
		ID:        "agreement-1",
		Name:      "Annual Subscription 2026",
		PayeeID:   "vendor-1",
		PayeeName: "Acme",
	}
	spend := vendorSpend{
		Trailing30Day: &client.Money{Amount: 50, CurrencyCode: "USD", MinorUnitConversionRate: 100},
		// 365d intentionally nil.
		YTD: &client.Money{Amount: 0, CurrencyCode: ""}, // currency-empty: should be skipped
	}

	opt, err := buildAgreementTraitOption(agreement, spend)
	if err != nil {
		t.Fatal(err)
	}

	rt := &v2.ResourceType{}
	rt.SetTraits([]v2.ResourceType_Trait{v2.ResourceType_TRAIT_VENDOR_AGREEMENT})
	r, err := resourceSdk.NewResource(agreement.Name, rt, agreement.ID, opt)
	if err != nil {
		t.Fatal(err)
	}

	trait, err := resourceSdk.GetVendorAgreementTrait(r)
	if err != nil {
		t.Fatal(err)
	}

	if got := trait.GetTrailing_30DSpend(); got == nil || got.GetAmountMinor() != 5000 {
		t.Fatalf("trailing 30d spend = %+v, want 5000", got)
	}
	if trait.GetTrailing_365DSpend() != nil {
		t.Fatalf("expected nil 365d spend, got %+v", trait.GetTrailing_365DSpend())
	}
	if trait.GetYtdSpend() != nil {
		t.Fatalf("expected nil YTD spend (currency_code empty), got %+v", trait.GetYtdSpend())
	}
}
