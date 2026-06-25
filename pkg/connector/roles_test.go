package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conductorone/baton-ramp/pkg/client"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	resourceSdk "github.com/conductorone/baton-sdk/pkg/types/resource"
)

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

func TestRoleGrantsFiltersBundledRolesServerSide(t *testing.T) {
	ctx := context.Background()
	var request *http.Request

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r.Clone(ctx)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"user-1","email":"one@example.com","first_name":"One","last_name":"User","role":" business_user ","status":"USER_ACTIVE"},
			{"id":"user-2","email":"two@example.com","first_name":"Two","last_name":"User","role":"BUSINESS_ADMIN","status":"USER_ACTIVE"}
		],"page":{"next":""}}`))
	}))
	defer server.Close()

	c, err := client.New(ctx, client.Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	builder := newRoleBuilder(c)
	role, err := resourceSdk.NewResource(roleNameUser, roleResourceType, "role:BUSINESS_USER", resourceSdk.WithRoleTrait())
	if err != nil {
		t.Fatal(err)
	}

	grants, nextPage, _, err := builder.Grants(ctx, role, &pagination.Token{})
	if err != nil {
		t.Fatal(err)
	}
	if nextPage != "" {
		t.Fatalf("next page = %q, want empty", nextPage)
	}
	if request == nil {
		t.Fatal("expected users request")
	}
	query := request.URL.Query()
	if query.Get("role") != roleIDBusinessUser {
		t.Fatalf("role query = %q, want %s", query.Get("role"), roleIDBusinessUser)
	}
	if query.Get("page_size") != "100" {
		t.Fatalf("page_size query = %q, want 100", query.Get("page_size"))
	}
	if len(grants) != 1 {
		t.Fatalf("expected 1 grant after normalized role comparison, got %d", len(grants))
	}
	if grants[0].GetPrincipal().GetId().GetResource() != "user-1" {
		t.Fatalf("grant principal = %q, want user-1", grants[0].GetPrincipal().GetId().GetResource())
	}
}

func TestRoleGrantsDoesNotUseUnsupportedUnbundledRoleFilter(t *testing.T) {
	ctx := context.Background()
	var request *http.Request

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r.Clone(ctx)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"user-1","email":"one@example.com","first_name":"One","last_name":"User","role":" unbundled_admin ","status":"USER_ACTIVE"},
			{"id":"user-2","email":"two@example.com","first_name":"Two","last_name":"User","role":"BUSINESS_ADMIN","status":"USER_ACTIVE"}
		],"page":{"next":""}}`))
	}))
	defer server.Close()

	c, err := client.New(ctx, client.Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	builder := newRoleBuilder(c)
	role, err := resourceSdk.NewResource(roleNameUnbundledAdmin, roleResourceType, "role:UNBUNDLED_ADMIN", resourceSdk.WithRoleTrait())
	if err != nil {
		t.Fatal(err)
	}

	grants, _, _, err := builder.Grants(ctx, role, &pagination.Token{})
	if err != nil {
		t.Fatal(err)
	}
	if request == nil {
		t.Fatal("expected users request")
	}
	query := request.URL.Query()
	if query.Get("role") != "" {
		t.Fatalf("role query = %q, want empty for unsupported UNBUNDLED_* filter", query.Get("role"))
	}
	if query.Get("page_size") != "100" {
		t.Fatalf("page_size query = %q, want 100", query.Get("page_size"))
	}
	if len(grants) != 1 {
		t.Fatalf("expected 1 grant after normalized unbundled role comparison, got %d", len(grants))
	}
	if grants[0].GetPrincipal().GetId().GetResource() != "user-1" {
		t.Fatalf("grant principal = %q, want user-1", grants[0].GetPrincipal().GetId().GetResource())
	}
}

func TestRoleFilterSupportMatchesRampUsersAPI(t *testing.T) {
	filtered := map[string]bool{
		roleIDAuditor:             true,
		roleIDBusinessAdmin:       true,
		roleIDBusinessBookkeeper:  true,
		roleIDBusinessOwner:       true,
		roleIDBusinessUser:        true,
		roleIDGuestUser:           true,
		roleIDITAdmin:             true,
		roleIDUnbundledAdmin:      false,
		roleIDUnbundledBookkeeper: false,
		roleIDUnbundledOwner:      false,
		roleIDUnbundledUser:       false,
	}
	for _, role := range roles {
		want, ok := filtered[role.ID]
		if !ok {
			t.Fatalf("missing filter expectation for role %q", role.ID)
		}
		if got := canFilterUsersByRole(role.ID); got != want {
			t.Fatalf("canFilterUsersByRole(%q) = %v, want %v", role.ID, got, want)
		}
	}
	if !isKnownRampRole(" business_user ") {
		t.Fatal("expected role normalization to handle mixed case and whitespace")
	}
	if isKnownRampRole("UNKNOWN_ROLE") {
		t.Fatal("expected unknown role to remain unknown")
	}
}
