package config

import (
	"github.com/conductorone/baton-sdk/pkg/field"
)

const (
	AccessTokenGroup          = "access_token"
	ClientCredentialsGroup    = "client_credentials"
)

var (
	Token = field.StringField("token",
		field.WithIsSecret(true),
		field.WithRequired(true),
		field.WithDisplayName("Ramp Access Token"),
	)

	ClientID = field.StringField("client-id",
		field.WithRequired(true),
		field.WithDisplayName("Ramp OAuth Client ID"),
	)

	ClientSecret = field.StringField("client-secret",
		field.WithIsSecret(true),
		field.WithRequired(true),
		field.WithDisplayName("Ramp OAuth Client Secret"),
	)

	ConfigurationFields = []field.SchemaField{Token, ClientID, ClientSecret}

	FieldRelationships = []field.SchemaFieldRelationship{
		field.FieldsMutuallyExclusive(Token, ClientID),
		field.FieldsMutuallyExclusive(Token, ClientSecret),
		field.FieldsRequiredTogether(ClientID, ClientSecret),
	}

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
			HelpText:    "Authenticate using OAuth 2.0 client credentials.",
			Fields:      []field.SchemaField{ClientID, ClientSecret},
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
	field.WithHelpUrl("/docs/baton/ramp-v2"),
)
