package client

import (
	"context"
	"fmt"
	"net/http"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

const vendorsPath = "vendors"

// GET https://api.ramp.com/developer/v1/vendors
// Required scope: vendors:read.
// https://docs.ramp.com/developer-api/v1/api/vendors#get-developer-v1-vendors
func (c *Client) ListVendors(ctx context.Context, pagination string) (*VendorsResponse, *v2.RateLimitDescription, error) {
	vendors := &VendorsList{}
	reqURL := pagination
	if reqURL == "" {
		var err error
		reqURL, err = c.newUnPaginatedURL(vendorsPath)
		if err != nil {
			return nil, nil, err
		}
	}
	ratelimitData, err := c.query(ctx, http.MethodGet, reqURL, vendors)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("baton-ramp: error listing vendors: %w", err)
	}
	return &VendorsResponse{
		Vendors:    vendors.Vendors,
		Pagination: vendors.Page.Next,
	}, ratelimitData, nil
}

// GET https://api.ramp.com/developer/v1/vendors/{vendor_id}
// Required scope: vendors:read.
// https://docs.ramp.com/developer-api/v1/api/vendors#get-developer-v1-vendors-id
func (c *Client) GetVendor(ctx context.Context, vendorID string) (*Vendor, *v2.RateLimitDescription, error) {
	reqURL, err := c.newUnPaginatedURL(fmt.Sprintf("%s/%s", vendorsPath, vendorID))
	if err != nil {
		return nil, nil, err
	}
	vendor := &Vendor{}
	ratelimitData, err := c.query(ctx, http.MethodGet, reqURL, vendor)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("baton-ramp: error getting vendor %s: %w", vendorID, err)
	}
	return vendor, ratelimitData, nil
}

// PATCH https://api.ramp.com/developer/v1/vendors/{vendor_id}
// Required scope: vendors:write.
// https://docs.ramp.com/developer-api/v1/api/vendors#patch-developer-v1-vendors-id
// Sets or clears vendor_owner_id. Pass empty string ownerID to clear the owner.
func (c *Client) UpdateVendorOwner(ctx context.Context, vendorID, ownerID string) (*v2.RateLimitDescription, error) {
	reqURL, err := c.newUnPaginatedURL(fmt.Sprintf("%s/%s", vendorsPath, vendorID))
	if err != nil {
		return nil, err
	}
	type patchBody struct {
		VendorOwnerID *string `json:"vendor_owner_id"`
	}
	body := patchBody{}
	if ownerID != "" {
		body.VendorOwnerID = &ownerID
	}
	var result Vendor
	ratelimitData, err := c.queryWithBody(ctx, http.MethodPatch, reqURL, body, &result)
	if err != nil {
		return ratelimitData, fmt.Errorf("baton-ramp: error updating vendor owner for %s: %w", vendorID, err)
	}
	return ratelimitData, nil
}
