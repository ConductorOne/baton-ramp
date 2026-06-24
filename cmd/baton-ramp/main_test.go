//go:build !generate

package main

import (
	"slices"
	"testing"

	cfg "github.com/conductorone/baton-ramp/pkg/config"
)

func TestBuildOAuthScopes(t *testing.T) {
	baseScopes := buildOAuthScopes(&cfg.Ramp{})
	for _, scope := range []string{"users:read", "users:write", "vendors:read", "vendors:write"} {
		if !slices.Contains(baseScopes, scope) {
			t.Fatalf("base scopes missing %q: %v", scope, baseScopes)
		}
	}
	for _, scope := range []string{businessScope, vendorManagementScope} {
		if slices.Contains(baseScopes, scope) {
			t.Fatalf("base scopes should not include %q: %v", scope, baseScopes)
		}
	}

	vendorManagementScopes := buildOAuthScopes(&cfg.Ramp{VendorManagement: true})
	for _, scope := range []string{businessScope, vendorManagementScope} {
		if !slices.Contains(vendorManagementScopes, scope) {
			t.Fatalf("vendor-management scopes missing %q: %v", scope, vendorManagementScopes)
		}
	}
}
