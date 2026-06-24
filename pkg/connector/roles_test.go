package connector

import "testing"

func TestRolesMatchRampReadEnum(t *testing.T) {
	expected := map[string]string{
		roleIDAuditor:             roleNameAuditor,
		roleIDBusinessAdmin:       roleNameAdmin,
		roleIDBusinessBookkeeper:  roleNameBookkeeper,
		roleIDBusinessOwner:       roleNameOwner,
		roleIDBusinessUser:        roleNameUser,
		roleIDGuestUser:           roleNameGuest,
		roleIDITAdmin:             roleNameITAdmin,
		roleIDUnbundledAdmin:      roleNameUnbundledAdmin,
		roleIDUnbundledBookkeeper: roleNameUnbundledBookkeeper,
		roleIDUnbundledOwner:      roleNameUnbundledOwner,
		roleIDUnbundledUser:       roleNameUnbundledUser,
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
		roleIDAuditor:            true,
		roleIDBusinessAdmin:      true,
		roleIDBusinessBookkeeper: true,
		roleIDBusinessUser:       true,
		roleIDGuestUser:          true,
		roleIDITAdmin:            true,
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
