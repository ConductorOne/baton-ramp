//go:build !generate

package main

import (
	"slices"
	"testing"

	cfg "github.com/conductorone/baton-ramp/pkg/config"
)

func TestScopeBuilders(t *testing.T) {
	for _, auditLogEvents := range []bool{false, true} {
		for _, provisioningAttributes := range []bool{false, true} {
			cc := &cfg.Ramp{AuditLogEvents: auditLogEvents, ProvisioningAttributes: provisioningAttributes}
			wantRead := []string{usersReadScope, vendorsReadScope}
			if auditLogEvents {
				wantRead = append(wantRead, auditLogsScope)
			}
			if provisioningAttributes {
				wantRead = append(wantRead, departmentsReadScope, locationsReadScope)
			}

			read := readScopes(cc)
			if !slices.Equal(read, wantRead) {
				t.Fatalf("audit=%v prov=%v: read scopes = %v, want %v", auditLogEvents, provisioningAttributes, read, wantRead)
			}

			usersWrite := usersWriteScopes(cc)
			if want := append(slices.Clone(wantRead), usersWriteScope); !slices.Equal(usersWrite, want) {
				t.Fatalf("audit=%v prov=%v: users write scopes = %v, want %v", auditLogEvents, provisioningAttributes, usersWrite, want)
			}

			vendorsWrite := vendorsWriteScopes(cc)
			if want := append(slices.Clone(wantRead), vendorsWriteScope); !slices.Equal(vendorsWrite, want) {
				t.Fatalf("audit=%v prov=%v: vendors write scopes = %v, want %v", auditLogEvents, provisioningAttributes, vendorsWrite, want)
			}

			usersWrite[0] = "mutated"
			if slices.Contains(vendorsWrite, "mutated") || slices.Contains(readScopes(cc), "mutated") {
				t.Fatalf("audit=%v prov=%v: scope builders share a backing array", auditLogEvents, provisioningAttributes)
			}
		}
	}
}

func TestVendorAgreementEnabled(t *testing.T) {
	if !vendorAgreementEnabled(nil) {
		t.Fatal("expected vendor agreements enabled when no resource-type filter is provided")
	}
	if vendorAgreementEnabled([]string{"user", "vendor"}) {
		t.Fatal("expected vendor agreements disabled when explicit filter omits them")
	}
	if !vendorAgreementEnabled([]string{"user", vendorAgreementResourceTypeID}) {
		t.Fatal("expected vendor agreements enabled when explicit filter includes them")
	}
}
