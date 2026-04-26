package cli

import (
	"fmt"
	"time"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/spf13/cobra"
)

func newStatusCmd(rc *rootContext) *cobra.Command {
	return &cobra.Command{
		Use:   "status <name>",
		Short: "Show runtime + registry status of a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
			if err != nil {
				return fmt.Errorf("building lifecycle: %w", err)
			}
			st, err := lc.Status(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s state=%s container=%s deploy=%s deployed_at=%s\n",
				st.Name, st.State, st.ContainerName, st.LastDeployID, st.LastDeployedAt.Format(time.RFC3339))
			return nil
		},
	}
}
