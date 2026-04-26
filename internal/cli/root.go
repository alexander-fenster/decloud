package cli

import (
	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/logging"
	"github.com/spf13/cobra"
)

// rootContext holds the resolved config root after persistent-flag parsing.
type rootContext struct {
	ConfigRoot string
}

// NewRootCmd returns the root cobra command tree.
func NewRootCmd() *cobra.Command {
	rc := &rootContext{}
	root := &cobra.Command{
		Use:           "decloud",
		Short:         "Declouding: a personal-scale platform-as-a-service",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return logging.Init()
		},
	}
	root.PersistentFlags().StringVar(&rc.ConfigRoot, "config-root", config.RootFromEnv(),
		"root directory for /opt/declouding-style layout (env: DECLOUD_ROOT)")

	deploy := &cobra.Command{Use: "deploy", Short: "Deploy a workload"}
	deploy.AddCommand(newDeployServiceCmd(rc))
	root.AddCommand(deploy)

	root.AddCommand(newUnregisterCmd(rc))
	root.AddCommand(newStartCmd(rc))
	root.AddCommand(newStopCmd(rc))
	root.AddCommand(newRestartCmd(rc))
	root.AddCommand(newStatusCmd(rc))
	root.AddCommand(newLogsCmd(rc))

	caddy := &cobra.Command{Use: "caddy", Short: "Caddy management"}
	caddy.AddCommand(newCaddyReloadCmd(rc))
	root.AddCommand(caddy)

	return root
}
