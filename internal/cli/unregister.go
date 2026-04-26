package cli

import (
	"fmt"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/spf13/cobra"
)

func newUnregisterCmd(rc *rootContext) *cobra.Command {
	return &cobra.Command{
		Use:   "unregister <name>",
		Short: "Remove a registered service (stop, remove, delete config+secrets, regenerate Caddyfile)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
			if err != nil {
				return fmt.Errorf("building lifecycle: %w", err)
			}
			return lc.Unregister(cmd.Context(), args[0])
		},
	}
}
