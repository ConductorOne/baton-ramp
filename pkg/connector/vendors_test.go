package connector

import (
	"testing"

	"github.com/conductorone/baton-ramp/pkg/client"
	resourceSdk "github.com/conductorone/baton-sdk/pkg/types/resource"
)

func TestVendorResourceTypeUsesVendorTrait(t *testing.T) {
	if !resourceSdk.IsVendorResource(vendorResourceType) {
		t.Fatal("expected vendor resource type to declare TRAIT_VENDOR")
	}
}

func TestVendorResourceIncludesVendorTrait(t *testing.T) {
	vendor := &client.Vendor{
		ID:   "vendor-123",
		Name: "Acme",
	}

	resource, err := vendorResource(vendor)
	if err != nil {
		t.Fatalf("expected vendor resource, got error: %v", err)
	}

	trait, err := resourceSdk.GetVendorTrait(resource)
	if err != nil {
		t.Fatalf("expected vendor trait, got error: %v", err)
	}
	if trait.GetVendorId() != vendor.ID {
		t.Fatalf("expected vendor ID %q, got %q", vendor.ID, trait.GetVendorId())
	}
	if trait.GetVendorName() != vendor.Name {
		t.Fatalf("expected vendor name %q, got %q", vendor.Name, trait.GetVendorName())
	}
}
