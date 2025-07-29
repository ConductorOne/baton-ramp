package config

import (
	"testing"

	"github.com/conductorone/baton-sdk/pkg/field"
	"github.com/stretchr/testify/assert"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Ramp
		wantErr bool
	}{
		{
			name: "valid config with token",
			config: &Ramp{
				Token: "valid-token",
			},
			wantErr: false,
		},
		{
			name: "valid config without token",
			config: &Ramp{
				Token: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := field.Validate(Config, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
