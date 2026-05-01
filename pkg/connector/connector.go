package connector

import (
	"context"
	"fmt"
	"io"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"golang.org/x/oauth2"
)

type Connector struct {
	client                  *client.Client
	baseURL                 string
	vendorManagementEnabled bool

	// businessID is the Ramp business id, populated during Validate().
	// Used as source_business_id on emitted VendorTrait /
	// VendorAgreementTrait annotations so consumers can disambiguate
	// when one customer manages multiple Ramp businesses.
	businessID string
}

type Option func(*Connector) error

// WithBaseURL overrides the default Ramp API base URL (e.g. https://demo-api.ramp.com for sandbox).
// Must be applied before WithToken or WithTokenSource so they build the client against it.
func WithBaseURL(baseURL string) Option {
	return func(c *Connector) error {
		c.baseURL = baseURL
		return nil
	}
}

// WithVendorManagement opts the connector into the vendor-management
// surface: emits VendorTrait on vendor resources, syncs vendor_agreement
// resources, and exposes the audit-log incremental-sync feed. Default
// false. Existing installs see no change unless this is enabled.
func WithVendorManagement(enabled bool) Option {
	return func(c *Connector) error {
		c.vendorManagementEnabled = enabled
		return nil
	}
}

// ResourceSyncers returns a ResourceSyncer for each resource type that should be synced from the upstream service.
func (d *Connector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncer {
	syncers := []connectorbuilder.ResourceSyncer{
		newUserBuilder(d.client),
		newRoleBuilder(d.client),
		newVendorBuilder(d.client, d.vendorManagementEnabled, d.BusinessID),
	}
	if d.vendorManagementEnabled {
		syncers = append(syncers, newVendorAgreementBuilder(d.client, d.BusinessID))
	}
	return syncers
}

// EventFeeds advertises the audit-log feed when vendor-management is
// enabled. Required for incremental sync via the platform's
// BatonFeedConsumerWorkflow. Returns nil otherwise (no feeds advertised
// = zero audit-log API calls).
func (d *Connector) EventFeeds(ctx context.Context) []connectorbuilder.EventFeed {
	if !d.vendorManagementEnabled {
		return nil
	}
	return []connectorbuilder.EventFeed{newAuditEventFeed(d.client)}
}

// Asset takes an input AssetRef and attempts to fetch it using the connector's authenticated http client
// It streams a response, always starting with a metadata object, following by chunked payloads for the asset.
func (d *Connector) Asset(ctx context.Context, asset *v2.AssetRef) (string, io.ReadCloser, error) {
	return "", nil, nil
}

// Metadata returns metadata about the connector.
func (d *Connector) Metadata(ctx context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "Baton Ramp Connector",
		Description: "This connector integrates with Ramp to manage users, roles, and vendors.",
		AccountCreationSchema: &v2.ConnectorAccountCreationSchema{
			FieldMap: map[string]*v2.ConnectorAccountCreationSchema_Field{
				"email": {
					DisplayName: "Email",
					Required:    true,
					Order:       1,
					Field: &v2.ConnectorAccountCreationSchema_Field_StringField{
						StringField: &v2.ConnectorAccountCreationSchema_StringField{},
					},
				},
				"first_name": {
					DisplayName: "First Name",
					Required:    true,
					Order:       2,
					Field: &v2.ConnectorAccountCreationSchema_Field_StringField{
						StringField: &v2.ConnectorAccountCreationSchema_StringField{},
					},
				},
				"last_name": {
					DisplayName: "Last Name",
					Required:    true,
					Order:       3,
					Field: &v2.ConnectorAccountCreationSchema_Field_StringField{
						StringField: &v2.ConnectorAccountCreationSchema_StringField{},
					},
				},
				"role": {
					DisplayName: "Role",
					Description: "One of: AUDITOR, BUSINESS_ADMIN, BUSINESS_BOOKKEEPER, BUSINESS_USER, GUEST_USER, IT_ADMIN",
					Required:    true,
					Order:       4,
					Field: &v2.ConnectorAccountCreationSchema_Field_StringField{
						StringField: &v2.ConnectorAccountCreationSchema_StringField{},
					},
				},
			},
		},
	}, nil
}

// Validate exercises the configured credentials. Calls
// GET /developer/v1/business: a cheap, scope-checked probe that also
// caches the Ramp business id for use as source_business_id on emitted
// vendor / vendor_agreement traits.
func (d *Connector) Validate(ctx context.Context) (annotations.Annotations, error) {
	var annos annotations.Annotations
	if d.client == nil {
		return annos, fmt.Errorf("baton-ramp: connector client not configured")
	}
	business, ratelimitData, err := d.client.GetBusiness(ctx)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return annos, fmt.Errorf("baton-ramp: validate failed: %w", err)
	}
	d.businessID = business.ID
	return annos, nil
}

// BusinessID returns the cached Ramp business id (populated by Validate).
// Empty when Validate has not run yet.
func (d *Connector) BusinessID() string {
	return d.businessID
}

// WithToken configures the connector to use an access token.
func WithToken(ctx context.Context, token string) Option {
	return func(c *Connector) error {
		client, err := client.New(ctx, client.Token{AccessToken: token}, c.baseURL)
		if err != nil {
			return fmt.Errorf("error creating ramp client: %w", err)
		}
		c.client = client
		return nil
	}
}

// WithTokenSource configures the connector to use a pre-configured token source.
func WithTokenSource(ctx context.Context, tokenSource oauth2.TokenSource) Option {
	return func(c *Connector) error {
		client, err := client.New(ctx, tokenSource, c.baseURL)
		if err != nil {
			return fmt.Errorf("error creating ramp client: %w", err)
		}
		c.client = client
		return nil
	}
}

// New returns a new instance of the connector.
func New(ctx context.Context, opts ...Option) (*Connector, error) {
	c := &Connector{}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	return c, nil
}
