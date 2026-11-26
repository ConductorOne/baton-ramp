package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

const (
	departmentsPath            = "departments"
	locationsPath              = "locations"
	defaultDepartmentsPageSize = 100
	defaultLocationsPageSize   = 100
)

// GET https://api.ramp.com/developer/v1/departments
// Required scope: departments:read.
// https://docs.ramp.com/developer-api/v1/api/departments
func (c *Client) ListDepartments(ctx context.Context, pagination string) (*DepartmentsResponse, *v2.RateLimitDescription, error) {
	departments := &DepartmentsList{}
	reqURL := pagination
	if reqURL == "" {
		var err error
		reqURL, err = c.newUnPaginatedURL(departmentsPath)
		if err != nil {
			return nil, nil, err
		}
	}
	reqURL, err := addDepartmentsListParams(reqURL)
	if err != nil {
		return nil, nil, err
	}
	ratelimitData, err := c.query(ctx, http.MethodGet, reqURL, departments)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("ramp-client: error listing departments: %w", err)
	}
	return &DepartmentsResponse{
		Departments: departments.Departments,
		Pagination:  departments.Page.Next,
	}, ratelimitData, nil
}

func addDepartmentsListParams(reqURL string) (string, error) {
	parsedURL, err := url.Parse(reqURL)
	if err != nil {
		return "", fmt.Errorf("ramp-client: failed to parse departments URL: %w", err)
	}
	query := parsedURL.Query()
	if query.Get("page_size") == "" {
		query.Set("page_size", strconv.Itoa(defaultDepartmentsPageSize))
	}
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

// GET https://api.ramp.com/developer/v1/departments/{department_id}
// Required scope: departments:read.
// https://docs.ramp.com/developer-api/v1/api/departments
func (c *Client) GetDepartment(ctx context.Context, departmentID string) (*Department, *v2.RateLimitDescription, error) {
	reqURL, err := c.newUnPaginatedURL(fmt.Sprintf("%s/%s", departmentsPath, url.PathEscape(departmentID)))
	if err != nil {
		return nil, nil, err
	}
	department := &Department{}
	ratelimitData, err := c.query(ctx, http.MethodGet, reqURL, department)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("ramp-client: error getting department %s: %w", departmentID, err)
	}
	return department, ratelimitData, nil
}

// GET https://api.ramp.com/developer/v1/locations
// Required scope: locations:read.
// https://docs.ramp.com/developer-api/v1/api/locations
func (c *Client) ListLocations(ctx context.Context, pagination string) (*LocationsResponse, *v2.RateLimitDescription, error) {
	locations := &LocationsList{}
	reqURL := pagination
	if reqURL == "" {
		var err error
		reqURL, err = c.newUnPaginatedURL(locationsPath)
		if err != nil {
			return nil, nil, err
		}
	}
	reqURL, err := addLocationsListParams(reqURL)
	if err != nil {
		return nil, nil, err
	}
	ratelimitData, err := c.query(ctx, http.MethodGet, reqURL, locations)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("ramp-client: error listing locations: %w", err)
	}
	return &LocationsResponse{
		Locations:  locations.Locations,
		Pagination: locations.Page.Next,
	}, ratelimitData, nil
}

func addLocationsListParams(reqURL string) (string, error) {
	parsedURL, err := url.Parse(reqURL)
	if err != nil {
		return "", fmt.Errorf("ramp-client: failed to parse locations URL: %w", err)
	}
	query := parsedURL.Query()
	if query.Get("page_size") == "" {
		query.Set("page_size", strconv.Itoa(defaultLocationsPageSize))
	}
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

// GET https://api.ramp.com/developer/v1/locations/{location_id}
// Required scope: locations:read.
// https://docs.ramp.com/developer-api/v1/api/locations
func (c *Client) GetLocation(ctx context.Context, locationID string) (*Location, *v2.RateLimitDescription, error) {
	reqURL, err := c.newUnPaginatedURL(fmt.Sprintf("%s/%s", locationsPath, url.PathEscape(locationID)))
	if err != nil {
		return nil, nil, err
	}
	location := &Location{}
	ratelimitData, err := c.query(ctx, http.MethodGet, reqURL, location)
	if err != nil {
		return nil, ratelimitData, fmt.Errorf("ramp-client: error getting location %s: %w", locationID, err)
	}
	return location, ratelimitData, nil
}
