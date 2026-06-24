package connector

import "testing"

func TestRolesMatchRampReadEnum(t *testing.T) {
	expected := map[string]string{
		"AUDITOR":              "Auditor",
		"BUSINESS_ADMIN":       "Admin",
		"BUSINESS_BOOKKEEPER":  "Bookkeeper",
		"BUSINESS_OWNER":       "Owner",
		"BUSINESS_USER":        "User",
		"GUEST_USER":           "Guest",
		"IT_ADMIN":             "IT Admin",
		"UNBUNDLED_ADMIN":      "Unbundled Admin",
		"UNBUNDLED_BOOKKEEPER": "Unbundled Bookkeeper",
		"UNBUNDLED_OWNER":      "Unbundled Owner",
		"UNBUNDLED_USER":       "Unbundled User",
	}

	if len(roles) != len(expected) {
		t.Fatalf("expected %d roles, got %d", len(expected), len(roles))
	}

	for _, role := range roles {
		expectedName, ok := expected[role.ID]
		if !ok {
			t.Fatalf("unexpected role %q", role.ID)
		}
		if role.Name != expectedName {
			t.Fatalf("expected role %q to have display name %q, got %q", role.ID, expectedName, role.Name)
		}
		delete(expected, role.ID)
	}

	for roleID := range expected {
		t.Fatalf("missing role %q", roleID)
	}
}

func TestValidCreateRolesMatchRampCreateEnum(t *testing.T) {
	expected := map[string]bool{
		"AUDITOR":             true,
		"BUSINESS_ADMIN":      true,
		"BUSINESS_BOOKKEEPER": true,
		"BUSINESS_OWNER":      true,
		"BUSINESS_USER":       true,
		"GUEST_USER":          true,
		"IT_ADMIN":            true,
	}

	if len(validCreateRoles) != len(expected) {
		t.Fatalf("expected %d create roles, got %d", len(expected), len(validCreateRoles))
	}

	for roleID := range validCreateRoles {
		if !expected[roleID] {
			t.Fatalf("unexpected create role %q", roleID)
		}
		delete(expected, roleID)
	}

	for roleID := range expected {
		t.Fatalf("missing create role %q", roleID)
	}
}
