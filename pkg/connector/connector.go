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
	client *client.Client
}

type Option func(*Connector) error

// ResourceSyncers returns a ResourceSyncer for each resource type that should be synced from the upstream service.
func (d *Connector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncer {
	return []connectorbuilder.ResourceSyncer{
		newUserBuilder(d.client),
		newRoleBuilder(d.client),
	}
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
		Description: "This connector integrates with Ramp to manage users and roles.",
	}, nil
}

// Validate is called to ensure that the connector is properly configured. It should exercise any API credentials
// to be sure that they are valid.
func (d *Connector) Validate(ctx context.Context) (annotations.Annotations, error) {
	return nil, nil
}

// WithToken configures the connector to use an access token.
func WithToken(ctx context.Context, token string) Option {
	return func(c *Connector) error {
		client, err := client.New(ctx, client.Token{AccessToken: token})
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
		client, err := client.New(ctx, tokenSource)
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
