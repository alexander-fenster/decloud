package cli

import (
	"fmt"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/spf13/cobra"
)

func newRestartCmd(rc *rootContext) *cobra.Command {
	return &cobra.Command{
		Use:   "restart <name>",
		Short: "Stop and start a service container",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
			if err != nil {
				return fmt.Errorf("building lifecycle: %w", err)
			}
			return lc.Restart(cmd.Context(), args[0])
		},
	}
}
