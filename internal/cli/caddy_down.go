package cli

import (
	"fmt"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/spf13/cobra"
)

func newCaddyDownCmd(rc *rootContext) *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop and remove the decloud-caddy container (volumes preserved)",
		Long: `Stop and remove the decloud-caddy container.

Stopping Caddy interrupts ingress for ALL services routed by this Decloud
host. Live traffic will fail until 'decloud caddy up' is run again.

The named volumes decloud_caddy_data (ACME state, issued certs) and
decloud_caddy_config (runtime config) are NOT removed. Re-running
'decloud caddy up' brings Caddy back with the same certificates and
runtime state. Wipe the volumes manually with 'docker volume rm
decloud_caddy_data decloud_caddy_config' only if you intend to discard
ACME state — that forces fresh Let's Encrypt issuance and risks tripping
LE rate limits on hosts with many domains.

Idempotent: if the container is already absent, this command exits 0.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := caddyManagerFactory(config.NewPaths(rc.ConfigRoot))
			if err != nil {
				return fmt.Errorf("building caddy manager: %w", err)
			}
			return mgr.Down(cmd.Context())
		},
	}
}
