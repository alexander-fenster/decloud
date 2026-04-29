# Decloud

Decloud is a small, single-host platform-as-a-service for low-traffic services that don't need a full cloud runtime.

It replaces a few specific workflows: Cloud Run services deployed via `gcloud run deploy --source .`, long-running services running as host `systemd` units, and Cron jobs scheduled with Google Cloud Scheduler. The expected operator is one person with SSH access to one Linux host.

## Project status

Decloud is mid-build. As of April 2026, only the milestones marked SHIPPED below are usable. See the [Roadmap](#roadmap) for what's next.

**What ships today (M1 + M2):**

- **M1** — server-side `decloud deploy service` with the `recreate` strategy; lifecycle commands `start`, `stop`, `restart`, `status`, `logs`, `unregister`; `decloud-caddy` ingress on a Docker network; ports `80/tcp`, `443/tcp`, and `443/udp` on the host.
- **M2** — persistent volumes via `--mount` (bind paths and Docker named volumes; `:ro` mode flag).

**Not yet shipped:**

- **M3** — host bootstrap script (install Decloud manually for now; see [`_docs/install.md`](_docs/install.md)).
- **M4** — zero-downtime blue/green deploys.
- **M5** — scheduled jobs (`decloud deploy job`).
- **M6** — encrypted backups via `restic`; image GC.
- **M7** — client binary for laptop-side `decloud`; deploy-time secret files.

## Quick start

On a fresh Linux host with Docker and a Go toolchain installed:

```sh
# 1. Create the Decloud state tree (full chmod sequence in _docs/install.md §4).
sudo mkdir -p /opt/decloud/{config/{services,jobs,caddy},secrets,state/deploys,logs}
sudo chmod 0700 /opt/decloud/secrets

# 2. Install the binary.
go install github.com/alexander-fenster/decloud/cmd/decloud@latest
sudo install -m 0755 "$(go env GOPATH)/bin/decloud" /usr/local/bin/decloud

# 3. Bring up the Caddy ingress container.
decloud caddy up

# 4. Deploy a service. ./myservice/ contains a Dockerfile and (optionally) env.sh.
decloud deploy service \
  --name myservice \
  --host myservice.example.com \
  --port 8080 \
  ./myservice
```

DNS for `myservice.example.com` must already point at the host so Caddy can complete the ACME challenge.

For the full procedure (firewall, ACME rate limits, migrating from earlier installs), see [`_docs/install.md`](_docs/install.md). For the deploy flag reference, see [`_docs/usage.md`](_docs/usage.md).

## What you get today

- `decloud deploy service` — build a Docker image from a source dir, run it on the shared `decloud` network, route Caddy to it, persist the registration.
- `decloud start | stop | restart <name>` — lifecycle controls. `start` re-runs the container from the saved image+env; `restart` is stop-then-start; neither rebuilds.
- `decloud status <name>` — runtime + registry state on one line.
- `decloud logs <name> [-f] [--tail N]` — pass-through to `docker logs`.
- `decloud unregister <name>` — remove the container, both registry files, and the Caddy route.
- `decloud caddy up | down | reload` — bring the `decloud-caddy` container up on the shared `decloud` network, take it down, or regenerate the Caddyfile from the registry and reload.
- `--mount` for `deploy service` — bind paths (`/host:/container[:ro]`) or named volumes (`name:/container[:ro]`).
- `env.sh` capture — sourced inside a hermetic bash, exported variables become container environment, never baked into the image.
- Strategy is `recreate` — brief downtime as the old container is stopped before the new one starts. Blue/green is M4.

## Architecture in 60 seconds

One Linux host. Docker runs every workload, including Caddy itself — `decloud-caddy` is just another container on the shared `decloud` Docker network. Caddy reaches each service container by its Docker DNS name (`decloud-<service>`); service containers are not host-port-published.

All persistent state lives under `/opt/decloud/`: per-service config TOML at `config/services/<name>.toml`, secrets at `secrets/<name>/env.toml`, the generated Caddyfile at `config/caddy/Caddyfile`. One backup path covers everything that matters.

There is no Decloud daemon and no listening management port. SSH is the management transport: the operator SSHes in and runs `decloud` directly. A laptop-side client binary is on the roadmap (M7); for now the SSH-and-run-directly path is the supported flow.

## Install

Decloud is installed manually in M1+M2 — no bootstrap script yet. Target OS: Linux with Docker and systemd; tested on Ubuntu LTS and Debian.

Prerequisites: a Linux host with root or sudo, outbound HTTPS, the public ports `80/tcp`, `443/tcp`, and `443/udp` open on the host firewall, and DNS for any hostnames you plan to deploy already pointing at the host (so Caddy can complete the ACME challenge). A Go toolchain on the host is convenient but not required — you can build the binary elsewhere and `scp` it in.

Full procedure with the chmod sequence, ACME-rate-limit caveats, and migration notes for older installs: [`_docs/install.md`](_docs/install.md).

## Usage

Three illustrative commands. The full flag reference and exit-code table live in [`_docs/usage.md`](_docs/usage.md).

```sh
# Deploy a service.
decloud deploy service --name myservice --host myservice.example.com --port 8080 ./myservice

# Deploy with a persistent bind mount (volumes survive container recreation).
decloud deploy service --name myservice --host myservice.example.com --port 8080 \
  --mount /var/lib/myservice:/data ./myservice

# Inspect a deployed service.
decloud status myservice
```

See [`_docs/usage.md`](_docs/usage.md) §3 for exit codes and §4 for the lifecycle command reference.

## Roadmap

- **M1** — Server-side service deploy with `recreate` strategy. (SHIPPED)
- **M2** — Persistent volumes via `--mount`. (SHIPPED)
- **M3** — Host bootstrap script and config-file plumbing (Viper). (PLANNED)
- **M4** — Zero-downtime blue/green deploys via Caddy admin API. (PLANNED)
- **M5** — Scheduled jobs via systemd timers (`decloud deploy job`). (PLANNED)
- **M6** — Encrypted backups via `restic`; image GC (`decloud gc`). (PLANNED)
- **M7** — Laptop-side client binary; deploy-time secret files; operational polish. (PLANNED)

## Non-goals

Decloud will not provide:

- Horizontal autoscaling
- Scale-to-zero
- Deploying prebuilt Docker images
- A web management UI
- A public management API
- Multi-node orchestration
- Kubernetes compatibility
- Full Cloud Run feature parity, or Cloud Run-compatible CLI flags
- Per-application host `systemd` units (per-job timer units for scheduled jobs are fine)

## Repository layout

```text
cmd/decloud/        # the decloud binary (single main package)
internal/           # private Go packages: cli, deploy, registry, caddy, dockerdrv, envcap, ...
_docs/              # user-facing documentation (install.md, usage.md)
_ai/                # decisions, conventions, agentic-development notes
_tasks/             # per-task workflow trail (planning, review, implementation reports)
CLAUDE.md           # contributor instructions (code style, agentic workflow)
tools.go            # pinned tool dependencies (gomock)
```

M1+M2 ship from `cmd/` and `internal/`. The `_ai/` and `_tasks/` directories are the agentic-development trail and not required reading for users.

## Contributing

Build and test:

```sh
go build ./cmd/decloud
go test ./...

# Integration tests (require Docker):
DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/...
```

Code style: format with `gofmt`. CLI flags use [Cobra](https://github.com/spf13/cobra); tests use [Testify](https://github.com/stretchr/testify) and [gomock](https://github.com/uber-go/mock). The agentic-development workflow is documented in [`CLAUDE.md`](CLAUDE.md).

The dev maintainer's machine has no Docker; integration tests run on a separate Linux host. Treat `DECLOUD_INTEGRATION=1` as opt-in, not the default test flow.

## License

MIT — see [`LICENSE`](./LICENSE).
