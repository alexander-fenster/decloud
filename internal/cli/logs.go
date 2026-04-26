package cli

import (
	"fmt"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/deploy"
	"github.com/spf13/cobra"
)

func newLogsCmd(rc *rootContext) *cobra.Command {
	var follow bool
	var tail int
	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Stream logs for a service container",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
			if err != nil {
				return fmt.Errorf("building lifecycle: %w", err)
			}
			return lc.Logs(cmd.Context(), args[0], deploy.LogOptions{Follow: follow, Tail: tail})
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().IntVar(&tail, "tail", 0, "number of trailing lines to show (0 = all)")
	return cmd
}
