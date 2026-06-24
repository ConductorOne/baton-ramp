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
// users:read   — list users (ListUsers)
// vendors:read — list vendors and vendor agreements (ListVendors, GetVendor, ListVendorAgreements)
const (
	usersReadScope   = "users:read"
	vendorsReadScope = "vendors:read"

	// users:write   — create/deactivate/reactivate users (CreateUser, DeactivateUser, ReactivateUser)
	usersWriteScope = "users:write"
	// vendors:write — update vendor owner (UpdateVendorOwner) for vendor owner grant/revoke
	vendorsWriteScope = "vendors:write"

	// auditLogsScope is appended only when the audit-log-events flag is true.
	// It reads audit-log events for the incremental sync feed.
	auditLogsScope = "audit_logs:read"

	vendorAgreementResourceTypeID = "vendor_agreement"
)

var baseRampOAuthScopes = []string{usersReadScope, vendorsReadScope}

// provisioningRampOAuthScopes are requested only when provisioning is enabled.
var provisioningRampOAuthScopes = []string{usersWriteScope, vendorsWriteScope}

// buildOAuthScopes returns the OAuth scopes for the connector based on the
// resolved config.
func buildOAuthScopes(cc *cfg.Ramp) []string {
	scopes := make([]string, 0, len(baseRampOAuthScopes)+len(provisioningRampOAuthScopes)+1)
	scopes = append(scopes, baseRampOAuthScopes...)
	if cc.Provisioning {
		scopes = append(scopes, provisioningRampOAuthScopes...)
	}
	if cc.AuditLogEvents {
		scopes = append(scopes, auditLogsScope)
	}
	return scopes
}

func vendorAgreementEnabled(resourceTypeIDs []string) bool {
	if len(resourceTypeIDs) == 0 {
		return true
	}
	for _, rt := range resourceTypeIDs {
		if rt == vendorAgreementResourceTypeID {
			return true
		}
	}
	return false
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
		connector.WithVendorAgreements(vendorAgreementEnabled(runTimeOpts.SyncResourceTypeIDs)),
		connector.WithAuditLogEvents(cc.AuditLogEvents),
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
