package config

import (
	"testing"

	"github.com/conductorone/baton-sdk/pkg/field"
	"github.com/stretchr/testify/assert"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     *Ramp
		authMethod string
		wantErr    bool
	}{
		{
			name: "access token group: valid with token",
			config: &Ramp{
				Token: "valid-token",
			},
			authMethod: AccessTokenGroup,
			wantErr:    false,
		},
		{
			name:       "access token group: empty config is invalid",
			config:     &Ramp{},
			authMethod: AccessTokenGroup,
			wantErr:    true,
		},
		{
			name: "client credentials group: valid with id and secret",
			config: &Ramp{
				RampClientId:     "client-id",
				RampClientSecret: "client-secret",
			},
			authMethod: ClientCredentialsGroup,
			wantErr:    false,
		},
		{
			name: "client credentials group: missing secret is invalid",
			config: &Ramp{
				RampClientId: "client-id",
			},
			authMethod: ClientCredentialsGroup,
			wantErr:    true,
		},
		{
			name: "client credentials group: missing id is invalid",
			config: &Ramp{
				RampClientSecret: "client-secret",
			},
			authMethod: ClientCredentialsGroup,
			wantErr:    true,
		},
		{
			name:       "client credentials group: empty config is invalid",
			config:     &Ramp{},
			authMethod: ClientCredentialsGroup,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := field.Validate(Config, tt.config, field.WithAuthMethod(tt.authMethod))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
