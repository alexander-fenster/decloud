package cli

import (
	"fmt"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/spf13/cobra"
)

func newCaddyUpCmd(rc *rootContext) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Run the decloud-caddy container on the decloud network",
		Long: `Run the decloud-caddy container on the decloud network.

Ensures the decloud Docker network exists, writes the Caddyfile stub if
missing, and starts (or runs) the decloud-caddy container with dual-stack
publishing on 80/tcp, 443/tcp, and 443/udp (both 0.0.0.0 and [::]).

The container uses image caddy:2 and named volumes decloud_caddy_data (ACME
state and issued certs) and decloud_caddy_config (runtime config). These
named volumes survive container removal — running 'decloud caddy down'
stops and removes the container but does NOT remove the volumes. Wipe them
manually with 'docker volume rm' only if you intend to discard ACME state.

Idempotent: if the container is already running, this command logs
'caddy already running' and exits 0. If the container exists but is
stopped, it is started in place.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := caddyManagerFactory(config.NewPaths(rc.ConfigRoot))
			if err != nil {
				return fmt.Errorf("building caddy manager: %w", err)
			}
			return mgr.Up(cmd.Context())
		},
	}
}
