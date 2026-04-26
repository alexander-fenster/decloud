package cli

import (
	"fmt"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/spf13/cobra"
)

func newCaddyReloadCmd(rc *rootContext) *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Regenerate Caddyfile from registry, validate, and reload Caddy",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
			if err != nil {
				return fmt.Errorf("building lifecycle: %w", err)
			}
			return lc.CaddyReload(cmd.Context())
		},
	}
}
