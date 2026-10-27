//go:build !generate

package main

import (
	"slices"
	"testing"

	cfg "github.com/conductorone/baton-ramp/pkg/config"
)

func TestScopeBuilders(t *testing.T) {
	for _, auditLogEvents := range []bool{false, true} {
		cc := &cfg.Ramp{AuditLogEvents: auditLogEvents}
		wantRead := []string{usersReadScope, vendorsReadScope}
		if auditLogEvents {
			wantRead = append(wantRead, auditLogsScope)
		}

		read := readScopes(cc)
		if !slices.Equal(read, wantRead) {
			t.Fatalf("audit=%v: read scopes = %v, want %v", auditLogEvents, read, wantRead)
		}

		usersWrite := usersWriteScopes(cc)
		if want := append(slices.Clone(wantRead), usersWriteScope); !slices.Equal(usersWrite, want) {
			t.Fatalf("audit=%v: users write scopes = %v, want %v", auditLogEvents, usersWrite, want)
		}

		vendorsWrite := vendorsWriteScopes(cc)
		if want := append(slices.Clone(wantRead), vendorsWriteScope); !slices.Equal(vendorsWrite, want) {
			t.Fatalf("audit=%v: vendors write scopes = %v, want %v", auditLogEvents, vendorsWrite, want)
		}

		usersWrite[0] = "mutated"
		if slices.Contains(vendorsWrite, "mutated") || slices.Contains(readScopes(cc), "mutated") {
			t.Fatalf("audit=%v: scope builders share a backing array", auditLogEvents)
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
