package client

import (
	"context"
	"fmt"
	"net/http"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

const businessPath = "business"

// GetBusiness returns the calling client's Ramp business. Used to populate
// `source_business_id` on emitted vendor / vendor_agreement traits so a
// consumer can disambiguate one Ramp customer's multiple businesses.
//
// Required scope: business:read.
//
// https://docs.ramp.com/developer-api/v1/api/business
func (c *Client) GetBusiness(ctx context.Context) (*Business, *v2.RateLimitDescription, error) {
	reqURL, err := c.newUnPaginatedURL(businessPath)
	if err != nil {
		return nil, nil, err
	}
	b := &Business{}
	ratelimitData, err := c.query(ctx, http.MethodGet, reqURL, b)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("baton-ramp: error getting business: %w", err)
	}
	return b, ratelimitData, nil
}
