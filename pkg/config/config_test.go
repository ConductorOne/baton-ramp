package config

import (
	"testing"

	"github.com/conductorone/baton-sdk/pkg/field"
	"github.com/stretchr/testify/assert"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *Ramp
		authGroup string
		wantErr   bool
	}{
		{
			name: "valid config with token",
			config: &Ramp{
				Token: "valid-token",
			},
			authGroup: AccessTokenGroup,
			wantErr:   false,
		},
		{
			name: "invalid config without token in access_token group",
			config: &Ramp{
				Token: "",
			},
			authGroup: AccessTokenGroup,
			wantErr:   true,
		},
		{
			name: "valid config with client credentials",
			config: &Ramp{
				ClientId:     "my-client-id",
				ClientSecret: "my-client-secret",
			},
			authGroup: ClientCredentialsGroup,
			wantErr:   false,
		},
		{
			name: "invalid config with only client id",
			config: &Ramp{
				ClientId: "my-client-id",
			},
			authGroup: ClientCredentialsGroup,
			wantErr:   true,
		},
		{
			name: "invalid config with only client secret",
			config: &Ramp{
				ClientSecret: "my-client-secret",
			},
			authGroup: ClientCredentialsGroup,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := field.Validate(Config, tt.config, field.WithAuthMethod(tt.authGroup))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
