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
	"golang.org/x/oauth2"
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

// readScopes returns the OAuth scopes used for sync, validation and the event feed.
func readScopes(cc *cfg.Ramp) []string {
	scopes := make([]string, 0, len(baseRampOAuthScopes)+1)
	scopes = append(scopes, baseRampOAuthScopes...)
	if cc.AuditLogEvents {
		scopes = append(scopes, auditLogsScope)
	}
	return scopes
}

// usersWriteScopes returns the read scopes plus users:write.
func usersWriteScopes(cc *cfg.Ramp) []string {
	return append(readScopes(cc), usersWriteScope)
}

// vendorsWriteScopes returns the read scopes plus vendors:write.
func vendorsWriteScopes(cc *cfg.Ramp) []string {
	return append(readScopes(cc), vendorsWriteScope)
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
		tokenSource := func(scopes []string) oauth2.TokenSource {
			ccCfg := &clientcredentials.Config{
				ClientID:     cc.RampClientId,
				ClientSecret: cc.RampClientSecret,
				TokenURL:     client.TokenURL(cc.RampBaseUrl),
				Scopes:       scopes,
			}
			return ccCfg.TokenSource(ctx)
		}
		authOpt = connector.WithTokenSources(ctx, client.TokenSources{
			Read:         tokenSource(readScopes(cc)),
			UsersWrite:   tokenSource(usersWriteScopes(cc)),
			VendorsWrite: tokenSource(vendorsWriteScopes(cc)),
			ReadScopes:   readScopes(cc),
		})
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
