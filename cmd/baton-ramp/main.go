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
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/oauth2/clientcredentials"
)

var version = "dev"

// rampOAuthScopes are the OAuth scopes requested when authenticating via client credentials.
// users:read  — list users (ListUsers)
// users:write — create/deactivate/reactivate users (CreateUser, DeactivateUser, ReactivateUser)
var rampOAuthScopes = []string{"users:read", "users:write"}

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

	// When no auth method is explicitly set, pick the group whose credentials
	// are populated so OAuth-only deployments don't trip the SDK's default-group
	// validation (which requires BATON_TOKEN). Read the flag directly rather
	// than through viper because cobra binds flags into viper later than
	// PersistentPreRunE fires.
	priorPreRun := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		if priorPreRun != nil {
			if err := priorPreRun(c, args); err != nil {
				return err
			}
		}
		authMethodFlag, _ := c.Flags().GetString("auth-method")
		if authMethodFlag != "" || os.Getenv("BATON_AUTH_METHOD") != "" {
			return nil
		}
		if os.Getenv("BATON_RAMP_CLIENT_ID") != "" || os.Getenv("BATON_RAMP_CLIENT_SECRET") != "" {
			v.Set("auth-method", cfg.ClientCredentialsGroup)
		}
		return nil
	}

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
