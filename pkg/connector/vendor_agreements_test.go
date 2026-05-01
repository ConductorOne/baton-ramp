package connector

import (
	"testing"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
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

