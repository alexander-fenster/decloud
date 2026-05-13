package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/deploy"
	"github.com/spf13/cobra"
)

func newStatusCmd(rc *rootContext) *cobra.Command {
	return &cobra.Command{
		Use:   "status [name]",
		Short: "Show status of one or all registered services",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
			if err != nil {
				return fmt.Errorf("building lifecycle: %w", err)
			}
			if len(args) == 1 {
				return runStatusOne(cmd.Context(), lc, cmd.OutOrStdout(), args[0])
			}
			return runStatusAll(cmd.Context(), lc, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func runStatusOne(ctx context.Context, lc deploy.Lifecycle, out io.Writer, name string) error {
	st, err := lc.Status(ctx, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s state=%s container=%s deploy=%s deployed_at=%s\n",
		st.Name, st.State, st.ContainerName, st.LastDeployID, st.LastDeployedAt.Format(time.RFC3339))
	return nil
}

func runStatusAll(ctx context.Context, lc deploy.Lifecycle, out, errw io.Writer) error {
	statuses, err := lc.StatusAll(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATE\tCONTAINER\tDEPLOY\tDEPLOYED_AT")
	for _, st := range statuses {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			st.Name,
			st.State,
			dashIfEmpty(st.ContainerName),
			dashIfEmpty(st.LastDeployID),
			rfc3339OrDash(st.LastDeployedAt),
		)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flushing status table: %w", err)
	}
	for _, st := range statuses {
		if st.ErrorDetail != "" {
			fmt.Fprintf(errw, "status: %s: %s\n", st.Name, st.ErrorDetail)
		}
	}
	return nil
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func rfc3339OrDash(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}
