# Declouding

Declouding is an in-house deployment tool for low-traffic services that do not need a full cloud runtime. The target is one virtual server running Docker containers, with Caddy handling routing and TLS certificates automatically.

The project starts as a replacement for a few existing Google Cloud and systemd workflows:

1. Cloud Run services deployed with `gcloud run deploy --source .`
2. Long-running services currently deployed as host `systemd` units
3. Cron jobs currently scheduled with Google Cloud Scheduler

The goal is not to rebuild Cloud Run, Kubernetes, or a management API. SSH access to the server is enough. The expected interface is a small set of CLI tools for deploying, unregistering, starting, stopping, and inspecting services.

Tools like Dokku, CapRover, and Coolify occupy roughly this space. Declouding is a deliberately narrower, bespoke solution rather than an adoption of one of those: the workload set is small and known, the operator is one person, and agentic coding makes a tailored implementation cheaper to write and easier to operate than configuring a general-purpose platform designed for a much broader audience.

## Operating Model

One host runs the platform components:

- Docker runs all application workloads.
- Caddy terminates TLS, obtains and renews certificates, and routes public hostnames to containers.
- A single host `systemd` unit starts and supervises the Declouding control layer on boot.
- Systemd timers trigger scheduled jobs, which Declouding runs as short-lived Docker containers. Application code is never installed on the host.

The platform should keep host-level configuration small and predictable. Long-running application supervision lives in Docker and Declouding metadata, not in many separate host `systemd` service units. Per-job timer + oneshot units are fine — they wire the scheduler, not supervise application processes.

Developer laptops install a slim `decloud` client package whose only job is to package and upload source trees to the server; it holds no other platform state. See [CLI Shape](#cli-shape) for the split.

## Workload Types

Declouding has two workload types: **services** (long-running containers) and **jobs** (containers that run on a schedule and exit).

The cloud split between Cloud Run (ephemeral, request-driven) and systemd processes on a VM (persistent, can initiate work on their own) collapses here. Both become long-running Docker containers on the same host; the only runtime difference is whether Caddy routes a public hostname to the container. A single service can expose HTTP *and* do background work — for example, an HTTP backend that also sends FCM push notifications on its own schedule is one service, not two.

Orthogonal attributes of a service are configured per-deploy:

- Zero or more public hostnames routed through Caddy.
- Whether it needs persistent storage (explicit mounted volumes).
- Environment variables, secret files, and restart behavior.

### Services

Services replace both Cloud Run services and host `systemd` units. They run as long-running Docker containers, start on deploy and host reboot, and keep running until explicitly stopped, replaced, or unregistered.

Expected behavior:

- Build an application container from source using a Dockerfile.
- Run one container per deployed service by default.
- Route zero or more public hostnames through Caddy to the container.
- Keep the container filesystem ephemeral by default.
- Mount explicit volumes for any required local persistence.
- Pass environment variables and secret files at deploy time.
- Restart on crash, through the same host-level Declouding supervisor rather than per-service host `systemd` units.

Unlike Cloud Run, Declouding does not stop containers on inactivity or scale them horizontally. Cloud Run-compatible assumptions should be supported where practical, especially for applications that expect credentials through environment variables or files.

### Jobs

Jobs replace Google Cloud Scheduler jobs. They run on a schedule, execute in a Docker container, and exit.

The scheduler is systemd timers and the execution environment is Docker. Per job, Declouding writes a timer unit and a oneshot service unit; the service unit invokes Declouding to launch the job's container. Application code is never installed on the host — jobs are always containers, never raw commands on the host.

Systemd timers are preferred over cron because the platform already depends on systemd for its control unit, and `journalctl -u <job>` plus `systemctl list-timers` give usable operational visibility without a separate log pipeline.

Expected behavior:

- Register a schedule and command/image for each job.
- Run jobs in Docker containers.
- Pass environment variables, secret files, and mounts.
- Record enough metadata to inspect what is registered.
- Support unregistering and updating jobs cleanly.

## Configuration

Deployments need a durable configuration directory on the host. It should describe services, routes, secrets, mounts, and cron jobs in plain files so the system is understandable over SSH.

Likely host layout:

```text
/opt/declouding/
  config/
    services/
    jobs/
    caddy/
  secrets/
  state/
  logs/
```

This layout is provisional. All persistent Declouding state lives under this one directory so that a single backup path covers everything that matters — see [Backup and Restore](#backup-and-restore).

## Environment and Mounted Files

A service declares two things at deploy time: the environment variables the container sees, and any files to mount into the container.

### Environment variables

An `env.sh` shell script is sourced at deploy time; every variable it exports becomes an environment variable on the container. Arbitrary shell is allowed — computed values, conditional exports, subshell invocations — because Declouding only cares about the resulting environment after the script runs. This lifts the Cloud Run constraint of reducing everything to `KEY=VALUE` literals, which in the existing workflow required parsing `env.sh` back out into a flat list before handing it to `gcloud`.

The captured environment is persisted with the service registration and injected into the container at `docker run` time — never baked into the image, so secrets stay out of image layers. This mirrors Cloud Run's model, where env lives on the revision rather than inside the container. `env.sh` is re-evaluated only on redeploy, so restarts are fast and reproducible and the script does not need to be idempotent.

### Mounted files

Some content does not fit cleanly into environment variables — Google service account JSON is the common case. These are declared as file mounts: a host path is mounted read-only into the container at a given in-container path. The Google Cloud client libraries then read credentials through `GOOGLE_APPLICATION_CREDENTIALS`, which `env.sh` sets.

Producing a mount file inline is a pure shell concern. A deploy script that wants to generate a file on the fly uses a standard heredoc to write it, then declares the mount. Declouding needs no special heredoc support — the shell already does it.

### Handling secrets

Sensitive inputs — whether they reach the container as environment variables or as mounted files — are never baked into Docker images. `env.sh` and any deploy-provided secret files are stored on the host under `/opt/declouding/secrets/<service>/` with owner-read-only permissions, injected or mounted at container start, and included in the encrypted `restic` backup. The backup encryption key is what ultimately protects them.

## Routing and TLS

Caddy is responsible for public routing and certificate management. It joins the same Docker network as service containers so that upstreams can be addressed by container name over Docker DNS rather than through host-published ports.

A service registration with a public route should include enough information to route traffic:

- Public hostname
- Container name or Docker network target
- Internal port
- Optional path-based routing if needed later

Declouding maintains a Caddyfile on disk as the persisted source of truth, generated from registered service metadata, regenerated on registration changes, and reloaded with `caddy reload`. During the hot path of a zero-downtime deploy, upstream swaps should go through Caddy's admin API (PATCH the specific route) rather than a full reload, so the flip is atomic and does not re-parse unrelated config.

DNS remains a manual responsibility. Once DNS points at the server, Caddy should obtain and renew certificates automatically.

## Container Lifecycle

Containers should be treated as replaceable runtime artifacts.

Default assumptions:

- Containers start on deploy and host reboot.
- Containers keep running until stopped, replaced, or unregistered.
- Application files inside the container are ephemeral.
- Persistent application state must use mounted volumes or external services.
- Logs should be available through Docker logs at minimum.

The exact `docker run` flags are an implementation detail, but deployed containers should avoid depending on mutable state inside the container filesystem.

## Deploy Lifecycle

Service deploys are zero-downtime by default. The sequence:

1. Build the new image.
2. Start the new container on the shared Docker network that Caddy joins. It listens on its internal port; Docker's container DNS makes it addressable by name.
3. Wait for readiness. Each service declares one readiness signal in its registration — either the image's own `HEALTHCHECK` or an HTTP probe path (e.g., `GET /healthz` returning 200).
4. Flip Caddy's upstream for the service's hostname(s) from the old container to the new one via the admin API.
5. Send SIGTERM to the old container, wait a grace period (default ~10s) for in-flight requests to finish, then SIGKILL and remove.

Concurrent deploys of the same service could race through this sequence and leave orphan containers. v1 assumes a single operator who does not run concurrent deploys; a per-service deploy lock is a simple later addition if it ever becomes a real concern.

### Rollback

- If readiness fails at step 3, kill the new container and leave the old one serving. Nothing has flipped.
- If the new container crashes after the flip, the restart policy brings it back. An optional later addition: a crash-budget check that auto-reverts to the previous image on rapid post-swap failures.

### When blue/green is not safe

Some services cannot safely run two copies at once:

- **Exclusive local volumes** — SQLite, BoltDB, or anything with file locks can corrupt state if two processes touch the same mount.
- **Other exclusive resources** — a service that holds an OS-level lock or binds to a unique external port.

These services should register with `strategy: recreate` (stop-then-start), accepting short downtime. The default is blue/green.

### Scope

Declouding delivers mechanical zero-downtime — no dropped requests during the swap itself. Semantic zero-downtime across a database schema migration or other backward-incompatible change remains the application's responsibility.

## Image Housekeeping

Server-side builds accumulate image layers and BuildKit cache over time. A weekly systemd timer runs:

- `docker system prune -a --filter until=168h` to remove stopped containers, dangling images, and unused images older than a week.
- `docker builder prune --filter until=168h` to trim the BuildKit layer cache.

Images in use by running containers are never pruned regardless of age. The one-week age filter also preserves the immediately-previous image per service long enough for a quick manual rollback and sidesteps any edge case where a deploy is mid-flight when the timer fires. Volumes are never pruned — they are managed explicitly through service registrations.

A timer is preferred over pruning after each deploy so that GC failures cannot affect deploy success and so the prune never has to reason about deploy state. `decloud gc` runs the same prune on demand.

## Backup and Restore

v1 workloads are stateless or file-backed with no live database writers, so the backup strategy is simple: push everything to encrypted object storage on a timer.

What gets backed up:

- `/opt/declouding/` in full — config, state, secrets, logs, last-deployed source bundles.
- All declared service volume mounts, discovered from service registrations.
- Caddy's data directory (ACME state), so a restore does not re-hit Let's Encrypt rate limits.

Docker images are not backed up. They rebuild from the source bundles on a fresh host.

The mechanism is `restic` pushing an encrypted, deduplicating snapshot to an S3-compatible bucket (Backblaze B2, Cloudflare R2, AWS S3 — any works). A nightly systemd timer runs it. The repo password lives outside the host (password manager or equivalent); losing it means losing the backups.

The target bucket should have object-versioning enabled so a compromised host cannot destroy older snapshots by overwriting the repo. Whatever whole-disk snapshot feature the VM provider offers is a cheap safety net on top.

Out of scope for v1 and easy to add later when a workload actually needs them:

- Per-service `pre_backup` hooks that produce a consistent dump before the snapshot (only matters once a live-database workload lands).
- Git-backed mirroring of the non-secret config, independent of the backup path above.

## CLI Shape

The project is called Declouding — the action of getting rid of cloud runtime. The CLI binary is `decloud`, shipped as two separate packages because the client and the server-side run in very different environments and have very different responsibilities:

- **Client**, installed on developer laptops from the standard language-ecosystem registry — e.g. `npm install -g decloud-client` or `go install github.com/alexander-fenster/decloud/client@latest`. Narrow surface: package a source tree, push it to the server, stream the remote output back, exit with the remote exit code. That is essentially its whole job. Heterogeneous install base (macOS / Linux / occasionally Windows).
- **Server-side**, installed on the single host that runs containers. Full CLI for deploying, inspecting, managing, backing up, and garbage-collecting services. Installed once during host bootstrap; one Linux target.

The transport between them is SSH. There is no daemon, no HTTP management API, and no listening port — the client invokes the server-side `decloud` over SSH and proxies its I/O. This keeps "SSH access is enough" intact as the operator contract, and the server-side CLI is equally usable by a human who SSH'd in directly.

Client commands (run on a laptop):

```text
decloud deploy service [--host example.com ...]
decloud deploy job
```

Server-side commands (run on the host, either directly over SSH or under the hood from the client's `deploy`):

```text
decloud unregister <name>
decloud start <name>
decloud stop <name>
decloud restart <name>
decloud status [<name>]
decloud logs <name>
decloud caddy reload
decloud backup run
decloud backup list
decloud backup restore <snapshot> [--service <name>]
decloud gc
```

### Deploy flow

1. Client packages the source tree containing the Dockerfile, honoring `.dockerignore` or an equivalent ignore file.
2. Client uploads the package to the server over SSH.
3. Server builds the Docker image.
4. Server runs the [deploy lifecycle](#deploy-lifecycle): start new container, wait for readiness, flip Caddy, stop old container.

This avoids duplicating packaging logic in every project, keeps container builds off the developer laptop, and matches the useful part of `gcloud run deploy --source .` without inheriting Cloud Build or its CLI flag compatibility.

The first implementation should use Docker directly, likely through the Docker CLI. Docker Compose is not part of the intended design.

## Non-Goals

Declouding does not need to provide:

- Horizontal autoscaling
- Scale-to-zero
- Deploying prebuilt Docker images
- A web management UI
- A public management API
- Multi-node orchestration
- Kubernetes compatibility
- Full Cloud Run feature parity
- Cloud Run-compatible CLI flags
- Host-level `systemd` supervision units per long-running application (per-job timer units for scheduled jobs are fine)

## Open Design Questions

These are intentionally unresolved until implementation options are evaluated:

- How to represent service metadata: YAML, JSON, shell files, or generated unit-like files.
- Whether logs need a lightweight aggregation story beyond `docker logs` and `journalctl`.

## Initial Direction

Start with the smallest useful platform:

1. A host bootstrap step installs Docker and Caddy and enables one Declouding systemd unit. Systemd timers handle scheduled jobs; no separate cron daemon is required.
2. A CLI registers services and jobs in plain host-side configuration.
3. Registering a service with a public hostname updates Caddy and reloads it.
4. Deployments upload source packages, build Docker images on the server, then replace the running container.
5. Secrets and persistent data are always explicit mounts.
6. A nightly systemd timer pushes an encrypted `restic` snapshot of all persistent state to object storage.

This should cover the current low-traffic workloads while keeping the implementation understandable and operable over SSH.
