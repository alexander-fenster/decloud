package cli

import (
	"fmt"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/spf13/cobra"
)

func newStartCmd(rc *rootContext) *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start a previously registered service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
			if err != nil {
				return fmt.Errorf("building lifecycle: %w", err)
			}
			return lc.Start(cmd.Context(), args[0])
		},
	}
}
