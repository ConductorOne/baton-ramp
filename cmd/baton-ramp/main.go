//go:build !generate

package main

import (
	"context"
	"fmt"
	"os"

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

func main() {
	ctx := context.Background()

	_, cmd, err := config.DefineConfigurationV2(
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

	cmd.Version = version

	err = cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

const rampOAuthEndpoint = "https://api.ramp.com/developer/v1/token"

func getConnector(ctx context.Context, cc *cfg.Ramp, runTimeOpts cli.RunTimeOpts) (types.ConnectorServer, error) {
	l := ctxzap.Extract(ctx)
	if err := field.Validate(cfg.Config, cc, field.WithAuthMethod(runTimeOpts.SelectedAuthMethod)); err != nil {
		return nil, err
	}

	var connectorOpt connector.Option

	switch runTimeOpts.SelectedAuthMethod {
	case cfg.ClientCredentialsGroup:
		ccCfg := &clientcredentials.Config{
			ClientID:     cc.RampClientID,
			ClientSecret: cc.RampClientSecret,
			TokenURL:     rampOAuthEndpoint,
		}
		connectorOpt = connector.WithTokenSource(ctx, ccCfg.TokenSource(ctx))
	default:
		if cc.RampClientID != "" && cc.RampClientSecret != "" {
			ccCfg := &clientcredentials.Config{
				ClientID:     cc.RampClientID,
				ClientSecret: cc.RampClientSecret,
				TokenURL:     rampOAuthEndpoint,
			}
			connectorOpt = connector.WithTokenSource(ctx, ccCfg.TokenSource(ctx))
		} else {
			connectorOpt = connector.WithToken(ctx, cc.Token)
		}
	}

	cb, err := connector.New(ctx, connectorOpt)
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
