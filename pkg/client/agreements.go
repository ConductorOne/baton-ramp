package client

import (
	"context"
	"fmt"
	"net/http"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

const (
	vendorAgreementsPath            = "vendors/agreements"
	defaultVendorAgreementsPageSize = 100
)

// ListVendorAgreements posts a search request to
// POST /developer/v1/vendors/agreements and returns one page of results.
//
// Required scope: vendors:read.
//
// pagination is the opaque `page.next` cursor from a previous response, or
// "" for the first page. When pagination is non-empty, it's used verbatim
// as the request URL (Ramp returns absolute URLs). The endpoint is POST-only
// with a required body, so cursor pages preserve the same method and search body.
//
// https://docs.ramp.com/developer-api/v1/api/vendor-agreements
func (c *Client) ListVendorAgreements(
	ctx context.Context,
	req *VendorAgreementsListRequest,
	pagination string,
) (*VendorAgreementsResponse, *v2.RateLimitDescription, error) {
	reqURL := pagination
	if reqURL == "" {
		var err error
		reqURL, err = c.newUnPaginatedURL(vendorAgreementsPath)
		if err != nil {
			return nil, nil, err
		}
	}
	if req == nil {
		req = &VendorAgreementsListRequest{}
	}
	if req.PageSize <= 0 {
		reqCopy := *req
		reqCopy.PageSize = defaultVendorAgreementsPageSize
		req = &reqCopy
	}
	list := &VendorAgreementsList{}
	ratelimitData, err := c.queryCollection(ctx, http.MethodPost, reqURL, req, list)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("baton-ramp: error listing vendor agreements: %w", err)
	}
	// This endpoint is POST-only, so the page size is in the body rather than
	// the URL and effectivePageSize does not apply. req.PageSize is normalized
	// above, so it is always positive here.
	warnIfTruncatedPage(ctx, vendorAgreementsPath, len(list.Data), req.PageSize, list.Page.Next)
	return &VendorAgreementsResponse{
		Agreements: list.Data,
		Pagination: list.Page.Next,
	}, ratelimitData, nil
}

// GetVendorAgreement fetches a single agreement, including line items.
//
// Required scope: vendors:read.
//
// https://docs.ramp.com/developer-api/v1/api/vendor-agreements
func (c *Client) GetVendorAgreement(
	ctx context.Context,
	agreementID string,
) (*VendorAgreement, *v2.RateLimitDescription, error) {
	reqURL, err := c.newUnPaginatedURL(fmt.Sprintf("%s/%s", vendorAgreementsPath, agreementID))
	if err != nil {
		return nil, nil, err
	}
	agreement := &VendorAgreement{}
	ratelimitData, err := c.query(ctx, http.MethodGet, reqURL, agreement)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("baton-ramp: error getting vendor agreement %s: %w", agreementID, err)
	}
	return agreement, ratelimitData, nil
}
