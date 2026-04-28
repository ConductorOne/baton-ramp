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

// rampOAuthScopes are the OAuth scopes requested when authenticating via client credentials.
// users:read    — list users (ListUsers)
// users:write   — create/deactivate/reactivate users (CreateUser, DeactivateUser, ReactivateUser)
// vendors:read  — list vendors (ListVendors, GetVendor)
// vendors:write — update vendor owner (UpdateVendorOwner) for vendor owner grant/revoke
var rampOAuthScopes = []string{"users:read", "users:write", "vendors:read", "vendors:write"}

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
			Scopes:       rampOAuthScopes,
		}
		authOpt = connector.WithTokenSource(ctx, ccCfg.TokenSource(ctx))
	case cfg.AccessTokenGroup:
		authOpt = connector.WithToken(ctx, cc.Token)
	default:
		return nil, fmt.Errorf("baton-ramp-connector: unknown auth method %q", authMethod)
	}

	cb, err := connector.New(ctx, connector.WithBaseURL(cc.RampBaseUrl), authOpt)
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
