package config

import (
	"github.com/conductorone/baton-sdk/pkg/field"
)

const (
	AccessTokenGroup       = "access_token"
	ClientCredentialsGroup = "client_credentials"
)

var (
	Token = field.StringField("token",
		field.WithIsSecret(true),
		field.WithRequired(true),
		field.WithDisplayName("Ramp Access Token"),
	)

	RampClientID = field.StringField("ramp-client-id",
		field.WithRequired(true),
		field.WithDisplayName("Ramp OAuth Client ID"),
	)

	RampClientSecret = field.StringField("ramp-client-secret",
		field.WithIsSecret(true),
		field.WithRequired(true),
		field.WithDisplayName("Ramp OAuth Client Secret"),
	)

	RampBaseURL = field.StringField("ramp-base-url",
		field.WithDisplayName("Ramp API Base URL"),
		field.WithDescription("Override the Ramp API base URL. Defaults to https://api.ramp.com. Use https://demo-api.ramp.com for sandbox."),
		field.WithDefaultValue("https://api.ramp.com"),
	)

	// VendorManagement opts the connector into the vendor-management
	// surface: vendor agreements, the audit-log incremental-sync feed,
	// and the VendorTrait emitted on existing vendor resources. Default
	// off; existing installs are unaffected. Requires the
	// vendor_agreements:read OAuth scope (added at runtime when this
	// flag is true).
	VendorManagement = field.BoolField("vendor-management",
		field.WithDisplayName("Sync vendor management data"),
		field.WithDescription(
			"Enable to sync vendor agreements (contracts), pre-aggregated spend, "+
				"and audit-log change events from Ramp's vendor-management surface. "+
				"Adds the vendor_agreements:read OAuth scope. Default off.",
		),
		field.WithDefaultValue(false),
	)

	ConfigurationFields = []field.SchemaField{Token, RampClientID, RampClientSecret, RampBaseURL, VendorManagement}

	// Field groups gate which fields are validated per selected auth method.
	// No top-level relationship constraints: FieldsMutuallyExclusive rejects
	// fields carrying WithRequired(true), and WithRequired already produces
	// a per-group "required but zero-value" error via string rules.
	FieldRelationships = []field.SchemaFieldRelationship{}

	ConfigurationFieldGroups = []field.SchemaFieldGroup{
		{
			Name:        AccessTokenGroup,
			DisplayName: "Access Token",
			HelpText:    "Authenticate using a Ramp API access token.",
			Fields:      []field.SchemaField{Token, RampBaseURL, VendorManagement},
			Default:     true,
		},
		{
			Name:        ClientCredentialsGroup,
			DisplayName: "OAuth 2.0 Client Credentials",
			HelpText:    "Authenticate using OAuth 2.0 client credentials issued by Ramp.",
			Fields:      []field.SchemaField{RampClientID, RampClientSecret, RampBaseURL, VendorManagement},
		},
	}
)

//go:generate go run -tags=generate ./gen
var Config = field.NewConfiguration(
	ConfigurationFields,
	field.WithConstraints(FieldRelationships...),
	field.WithFieldGroups(ConfigurationFieldGroups),
	field.WithConnectorDisplayName("Ramp"),
	field.WithIconUrl("/static/app-icons/ramp.svg"),
	field.WithHelpUrl("/docs/baton/ramp"),
)
