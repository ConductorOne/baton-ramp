package config

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// AutoSelectAuthMethod wires a PersistentPreRunE hook that picks an auth
// method group when the user did not pass --auth-method / BATON_AUTH_METHOD.
// It runs before the SDK's field-group validation, which would otherwise
// require the default group's fields even when only OAuth credentials are set.
func AutoSelectAuthMethod(v *viper.Viper, cmd *cobra.Command) {
	prior := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		if prior != nil {
			if err := prior(c, args); err != nil {
				return err
			}
		}
		if c.Flags().Changed("auth-method") || v.GetString("auth-method") != "" {
			return nil
		}
		if v.GetString("ramp-client-id") != "" || v.GetString("ramp-client-secret") != "" {
			v.Set("auth-method", ClientCredentialsGroup)
		}
		return nil
	}
}
