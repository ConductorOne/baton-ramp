package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
)

func TestValidateDoesNotCallBusiness(t *testing.T) {
	ctx := context.Background()
	var businessRequests int
	var usersRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/developer/v1/business" {
			businessRequests++
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if r.URL.Path == "/developer/v1/users" {
			usersRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[],"page":{"next":""}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	c, err := New(ctx, WithBaseURL(server.URL), WithToken(ctx, "token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Validate(ctx); err != nil {
		t.Fatal(err)
	}
	if businessRequests != 0 {
		t.Fatalf("business endpoint called %d times", businessRequests)
	}
	if usersRequests != 1 {
		t.Fatalf("users endpoint called %d times, want 1", usersRequests)
	}
}

func TestValidateExercisesCredentials(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer server.Close()

	c, err := New(ctx, WithBaseURL(server.URL), WithToken(ctx, "token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Validate(ctx); err == nil {
		t.Fatal("expected validate to fail when API credentials are rejected")
	}
}

func TestVendorAgreementsAndAuditLogEventsAreIndependent(t *testing.T) {
	ctx := context.Background()

	base, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(base.ResourceSyncers(ctx)); got != 4 {
		t.Fatalf("base ResourceSyncers = %d, want 4", got)
	}
	if got := len(base.EventFeeds(ctx)); got != 0 {
		t.Fatalf("base EventFeeds = %d, want 0", got)
	}

	auditOnly, err := New(ctx, WithVendorAgreements(false), WithAuditLogEvents(true))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(auditOnly.ResourceSyncers(ctx)); got != 3 {
		t.Fatalf("audit-log-only ResourceSyncers = %d, want 3", got)
	}
	if got := len(auditOnly.EventFeeds(ctx)); got != 1 {
		t.Fatalf("audit-log-only EventFeeds = %d, want 1", got)
	}

	withAgreements, err := New(ctx, WithVendorAgreements(true))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(withAgreements.ResourceSyncers(ctx)); got != 4 {
		t.Fatalf("vendor agreement ResourceSyncers = %d, want 4", got)
	}
	if got := len(withAgreements.EventFeeds(ctx)); got != 0 {
		t.Fatalf("vendor-agreement-only EventFeeds = %d, want 0", got)
	}
}

func TestVendorAgreementResourceTypeIsOptInRequired(t *testing.T) {
	annos := annotations.Annotations(vendorAgreementResourceType.GetAnnotations())
	if !annos.Contains(&v2.OptInRequired{}) {
		t.Fatal("expected vendor_agreement resource type to require opt-in")
	}
}
