//go:build !generate

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/conductorone/baton-ramp/pkg/client"
	cfg "github.com/conductorone/baton-ramp/pkg/config"
	"github.com/conductorone/baton-ramp/pkg/connector"
	"github.com/conductorone/baton-sdk/pkg/cli"
	"github.com/conductorone/baton-sdk/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/connectorrunner"
	"github.com/conductorone/baton-sdk/pkg/field"
	"github.com/conductorone/baton-sdk/pkg/types"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"golang.org/x/oauth2/clientcredentials"
)

var version = "dev"

// baseRampOAuthScopes are the OAuth scopes requested by every install.
// users:read    — list users (ListUsers)
// users:write   — create/deactivate/reactivate users (CreateUser, DeactivateUser, ReactivateUser)
// vendors:read  — list vendors (ListVendors, GetVendor)
// vendors:write — update vendor owner (UpdateVendorOwner) for vendor owner grant/revoke
// business:read — fetch business id for source_business_id and access audit-log events
var baseRampOAuthScopes = []string{"users:read", "users:write", "vendors:read", "vendors:write", "business:read"}

// vendorManagementScope is appended only when the vendor-management flag
// is true. Listing and reading vendor agreements requires it.
const vendorManagementScope = "vendor_agreements:read"

// buildOAuthScopes returns the OAuth scopes for the connector based on the
// resolved config. Adding new scopes here must remain backward-compatible
// with existing installs (i.e. only append; never remove from baseRampOAuthScopes).
func buildOAuthScopes(cc *cfg.Ramp) []string {
	scopes := make([]string, 0, len(baseRampOAuthScopes)+1)
	scopes = append(scopes, baseRampOAuthScopes...)
	if cc.VendorManagement {
		scopes = append(scopes, vendorManagementScope)
	}
	return scopes
}

func main() {
	ctx := context.Background()

	v, cmd, err := config.DefineConfigurationV2(
		ctx,
		"baton-ramp",
		getConnector,
		cfg.Config,
		connectorrunner.WithDefaultCapabilitiesConnectorBuilder(&connector.Connector{}),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	cfg.AutoSelectAuthMethod(v, cmd)

	cmd.Version = version

	err = cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func getConnector(ctx context.Context, cc *cfg.Ramp, runTimeOpts cli.RunTimeOpts) (types.ConnectorServer, error) {
	l := ctxzap.Extract(ctx)

	authMethod := runTimeOpts.SelectedAuthMethod
	if authMethod == "" {
		authMethod = cfg.AccessTokenGroup
	}

	if err := field.Validate(cfg.Config, cc, field.WithAuthMethod(authMethod)); err != nil {
		return nil, err
	}

	var authOpt connector.Option
	switch authMethod {
	case cfg.ClientCredentialsGroup:
		ccCfg := &clientcredentials.Config{
			ClientID:     cc.RampClientId,
			ClientSecret: cc.RampClientSecret,
			TokenURL:     client.TokenURL(cc.RampBaseUrl),
			Scopes:       buildOAuthScopes(cc),
		}
		authOpt = connector.WithTokenSource(ctx, ccCfg.TokenSource(ctx))
	case cfg.AccessTokenGroup:
		authOpt = connector.WithToken(ctx, cc.Token)
	default:
		return nil, fmt.Errorf("baton-ramp-connector: unknown auth method %q", authMethod)
	}

	cb, err := connector.New(
		ctx,
		connector.WithBaseURL(cc.RampBaseUrl),
		connector.WithVendorManagement(cc.VendorManagement),
		authOpt,
	)
	if err != nil {
		l.Error("error creating connector", zap.Error(err))
		return nil, err
	}
	c, err := connectorbuilder.NewConnector(ctx, cb)
	if err != nil {
		l.Error("error creating connector", zap.Error(err))
		return nil, err
	}
	return c, nil
}
