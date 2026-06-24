//go:build !generate

package main

import (
	"slices"
	"testing"

	cfg "github.com/conductorone/baton-ramp/pkg/config"
)

func TestBuildOAuthScopes(t *testing.T) {
	baseScopes := buildOAuthScopes(&cfg.Ramp{})
	for _, scope := range []string{usersReadScope, vendorsReadScope} {
		if !slices.Contains(baseScopes, scope) {
			t.Fatalf("base scopes missing %q: %v", scope, baseScopes)
		}
	}
	for _, scope := range []string{usersWriteScope, vendorsWriteScope, auditLogsScope} {
		if slices.Contains(baseScopes, scope) {
			t.Fatalf("base scopes should not include %q: %v", scope, baseScopes)
		}
	}

	provisioningScopes := buildOAuthScopes(&cfg.Ramp{Provisioning: true})
	for _, scope := range []string{usersWriteScope, vendorsWriteScope} {
		if !slices.Contains(provisioningScopes, scope) {
			t.Fatalf("provisioning scopes missing %q: %v", scope, provisioningScopes)
		}
	}

	auditLogScopes := buildOAuthScopes(&cfg.Ramp{AuditLogEvents: true})
	if !slices.Contains(auditLogScopes, auditLogsScope) {
		t.Fatalf("audit-log scopes missing %q: %v", auditLogsScope, auditLogScopes)
	}
	for _, scope := range []string{usersWriteScope, vendorsWriteScope} {
		if slices.Contains(auditLogScopes, scope) {
			t.Fatalf("audit-log scopes should not include %q without provisioning: %v", scope, auditLogScopes)
		}
	}

	fullScopes := buildOAuthScopes(&cfg.Ramp{Provisioning: true, AuditLogEvents: true})
	for _, scope := range []string{usersReadScope, vendorsReadScope, usersWriteScope, vendorsWriteScope, auditLogsScope} {
		if !slices.Contains(fullScopes, scope) {
			t.Fatalf("full scopes missing %q: %v", scope, fullScopes)
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
