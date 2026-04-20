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

	ConfigurationFields = []field.SchemaField{Token, RampClientID, RampClientSecret}

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
			Fields:      []field.SchemaField{Token},
			Default:     true,
		},
		{
			Name:        ClientCredentialsGroup,
			DisplayName: "OAuth 2.0 Client Credentials",
			HelpText:    "Authenticate using OAuth 2.0 client credentials issued by Ramp.",
			Fields:      []field.SchemaField{RampClientID, RampClientSecret},
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
