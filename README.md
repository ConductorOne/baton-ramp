![Baton Logo](./baton-logo.png)

# `baton-ramp` [![Go Reference](https://pkg.go.dev/badge/github.com/conductorone/baton-ramp.svg)](https://pkg.go.dev/github.com/conductorone/baton-ramp) ![main ci](https://github.com/conductorone/baton-ramp/actions/workflows/main.yaml/badge.svg)

`baton-ramp` is a connector for built using the [Baton SDK](https://github.com/conductorone/baton-sdk).

Check out [Baton](https://github.com/conductorone/baton) to learn more the project in general.

# Prerequisites

A Ramp account with one of the following:
- A Ramp API access token, **or**
- An OAuth 2.0 client ID and secret issued by Ramp

# Getting Started

## brew

```
brew install conductorone/baton/baton conductorone/baton/baton-ramp

# Access token auth
BATON_TOKEN=<your-ramp-token> baton-ramp
baton resources

# OAuth 2.0 client credentials
BATON_RAMP_CLIENT_ID=<client-id> BATON_RAMP_CLIENT_SECRET=<client-secret> baton-ramp
baton resources
```

## docker

```
# Access token auth
docker run --rm -v $(pwd):/out -e BATON_TOKEN=<your-ramp-token> ghcr.io/conductorone/baton-ramp:latest -f "/out/sync.c1z"
docker run --rm -v $(pwd):/out ghcr.io/conductorone/baton:latest -f "/out/sync.c1z" resources

# OAuth 2.0 client credentials
docker run --rm -v $(pwd):/out -e BATON_RAMP_CLIENT_ID=<client-id> -e BATON_RAMP_CLIENT_SECRET=<client-secret> ghcr.io/conductorone/baton-ramp:latest -f "/out/sync.c1z"
docker run --rm -v $(pwd):/out ghcr.io/conductorone/baton:latest -f "/out/sync.c1z" resources
```

## source

```
go install github.com/conductorone/baton/cmd/baton@main
go install github.com/conductorone/baton-ramp/cmd/baton-ramp@main

BATON_TOKEN=<your-ramp-token> baton-ramp

baton resources
```

# Data Model

`baton-ramp` will pull down information about the following resources:
- Users
- Roles
- Vendors

`baton-ramp` supports:
- **Account provisioning**: create user accounts in Ramp
- **Entitlement provisioning**: grant and revoke vendor ownership

See [docs/api-support-matrix.md](docs/api-support-matrix.md) for the Ramp API field support matrix.

# Contributing, Support and Issues

We started Baton because we were tired of taking screenshots and manually
building spreadsheets. We welcome contributions, and ideas, no matter how
small&mdash;our goal is to make identity and permissions sprawl less painful for
everyone. If you have questions, problems, or ideas: Please open a GitHub Issue!

See [CONTRIBUTING.md](https://github.com/ConductorOne/baton/blob/main/CONTRIBUTING.md) for more details.

# `baton-ramp` Command Line Usage

```
baton-ramp

Usage:
  baton-ramp [flags]
  baton-ramp [command]

Available Commands:
  capabilities       Get connector capabilities
  completion         Generate the autocompletion script for the specified shell
  config             Get the connector config schema
  health-check       Check the health of a running connector
  help               Help about any command

Flags:
      --auth-method string                               Authentication method: "access_token" or "client_credentials" ($BATON_AUTH_METHOD)
      --client-id string                                 The client ID used to authenticate with ConductorOne ($BATON_CLIENT_ID)
      --client-secret string                             The client secret used to authenticate with ConductorOne ($BATON_CLIENT_SECRET)
      --external-resource-c1z string                     The path to the c1z file to sync external baton resources with ($BATON_EXTERNAL_RESOURCE_C1Z)
      --external-resource-entitlement-id-filter string   The entitlement that external users, groups must have access to sync external baton resources ($BATON_EXTERNAL_RESOURCE_ENTITLEMENT_ID_FILTER)
  -f, --file string                                      The path to the c1z file to sync with ($BATON_FILE) (default "sync.c1z")
      --health-check                                     Enable the HTTP health check endpoint ($BATON_HEALTH_CHECK)
      --health-check-port int                            Port for the HTTP health check endpoint ($BATON_HEALTH_CHECK_PORT) (default 8081)
  -h, --help                                             help for baton-ramp
      --http-timeout-seconds int                         HTTP client timeout in seconds (max 1800) ($BATON_HTTP_TIMEOUT_SECONDS) (default 300)
      --log-format string                                The output format for logs: json, console ($BATON_LOG_FORMAT) (default "json")
      --log-level string                                 The log level: debug, info, warn, error ($BATON_LOG_LEVEL) (default "info")
      --otel-collector-endpoint string                   The endpoint of the OpenTelemetry collector to send observability data to ($BATON_OTEL_COLLECTOR_ENDPOINT)
  -p, --provisioning                                     This must be set in order for provisioning actions to be enabled ($BATON_PROVISIONING)
      --ramp-base-url string                             Override the Ramp API base URL. Defaults to https://api.ramp.com. Use https://demo-api.ramp.com for sandbox. ($BATON_RAMP_BASE_URL) (default "https://api.ramp.com")
      --ramp-client-id string                            Ramp OAuth client ID — required when using client_credentials auth ($BATON_RAMP_CLIENT_ID)
      --ramp-client-secret string                        Ramp OAuth client secret — required when using client_credentials auth ($BATON_RAMP_CLIENT_SECRET)
      --skip-full-sync                                   This must be set to skip a full sync ($BATON_SKIP_FULL_SYNC)
      --sync-resources strings                           The resource IDs to sync ($BATON_SYNC_RESOURCES)
      --ticketing                                        This must be set to enable ticketing support ($BATON_TICKETING)
      --token string                                     Ramp API access token — required when using access_token auth ($BATON_TOKEN)
      --workers int                                      The number of sync workers to use. -1 for auto-detect, 0 for sequential, >0 for parallel ($BATON_WORKERS)
  -v, --version                                          version for baton-ramp

Use "baton-ramp [command] --help" for more information about a command.
```
