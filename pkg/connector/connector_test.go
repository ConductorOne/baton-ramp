package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateSkipsBusinessWhenVendorManagementDisabled(t *testing.T) {
	ctx := context.Background()
	var businessRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/developer/v1/business" {
			businessRequests++
			_, _ = w.Write([]byte(`[]`))
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
}

func TestValidateCachesBusinessWhenVendorManagementEnabled(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/developer/v1/business" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"business-1","business_name_legal":"Ramp Example"}`))
	}))
	defer server.Close()

	c, err := New(ctx, WithBaseURL(server.URL), WithVendorManagement(true), WithToken(ctx, "token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Validate(ctx); err != nil {
		t.Fatal(err)
	}
	if got := c.BusinessID(); got != "business-1" {
		t.Fatalf("BusinessID = %q, want business-1", got)
	}
}
