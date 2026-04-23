# Declouding

Declouding is an in-house deployment tool for low-traffic services that do not need a full cloud runtime. The target is one virtual server running Docker containers, with Caddy handling routing and TLS certificates automatically.

The project starts as a replacement for a few existing Google Cloud and systemd workflows:

1. Cloud Run services deployed with `gcloud run deploy --source .`
2. Long-running services currently deployed as host `systemd` units
3. Cron jobs currently scheduled with Google Cloud Scheduler

The goal is not to rebuild Cloud Run, Kubernetes, or a management API. SSH access to the server is enough. The expected interface is a small set of CLI tools for deploying, unregistering, starting, stopping, and inspecting services.

## Operating Model

One host runs the platform components:

- Docker runs all application workloads.
- Caddy terminates TLS, obtains and renews certificates, and routes public hostnames to containers.
- A single host `systemd` unit starts and supervises the Declouding control layer on boot.
- Host cron can trigger scheduled jobs, preferably by launching Docker containers instead of running application code directly on the host.

The platform should keep host-level configuration small and predictable. Application-specific process supervision should live in Docker and Declouding metadata, not as many separate host `systemd` units.

## Workload Types

### Web Services

Web services are the closest replacement for Cloud Run services. They are usually Node.js, TypeScript, or Express applications that serve API endpoints, websites, or both.

Unlike Cloud Run, Declouding does not need to stop containers on inactivity or scale them horizontally. A web service can start when deployed, restart on host reboot, and keep running until explicitly stopped or replaced.

Expected behavior:

- Build an application container from source using a Dockerfile.
- Run one container per deployed service by default.
- Route one or more hostnames through Caddy to the container.
- Keep the container filesystem ephemeral by default.
- Mount explicit volumes for any required local persistence.
- Pass environment variables and secret files at deploy time.

Cloud Run-compatible assumptions should be supported where practical, especially for applications that expect credentials through environment variables or files.

### Long-Running Services

Long-running services are workloads that must stay active but are not necessarily HTTP web services. Today these may be deployed with scripts that create or update host `systemd` units.

In Declouding, these should also run as Docker containers. They should be started by the same host-level Declouding service that starts web services, rather than by individual host `systemd` units.

Expected behavior:

- Deploy and restart containers through the same CLI workflow as web services.
- Configure environment variables, secret files, mounts, and restart behavior.
- Avoid modifying host `systemd` for each service.
- Allow no public route when the service does not expose HTTP traffic.

### Cron Jobs

Cron jobs replace Google Cloud Scheduler jobs.

The simplest initial model is to use host cron as the scheduler and Docker as the execution environment. Cron entries should trigger Declouding commands or directly run configured job containers. Application code should not need to be installed on the host.

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

This layout is provisional. The important constraint is that all persistent Declouding state should be explicit and backup-friendly.

## Secrets and Environment

Services need a practical replacement for Cloud Run's default service account credentials.

Supported input forms should include:

- Environment variables.
- An `env.sh` or similar shell file for environment setup.
- Secret files mounted into the container.
- Service account JSON files.
- Generated files from heredocs in a deployment script, when that is more convenient than copying separate files.

For Google Cloud client libraries, the usual path is likely to mount a service account JSON file and set `GOOGLE_APPLICATION_CREDENTIALS` inside the container.

Secrets should not be baked into Docker images. They should be stored on the host with restrictive permissions and mounted into containers at runtime.

## Routing and TLS

Caddy is responsible for public routing and certificate management.

Declouding should generate or maintain Caddy configuration from registered service metadata. A web service registration should include enough information to route traffic:

- Public hostname
- Container name or Docker network target
- Internal port
- Optional path-based routing if needed later

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

## CLI Shape

The first implementation should probably provide a CLI rather than a daemon API.

Possible commands:

```text
decloud deploy web
decloud deploy service
decloud deploy job
decloud unregister
decloud start
decloud stop
decloud restart
decloud status
decloud logs
decloud caddy reload
```

The CLI can be run over SSH by a human or by deployment scripts. It should be possible to deploy from a local project directory using a workflow similar in spirit to `gcloud run deploy --source .`.

The likely split is:

1. A local client packages the source tree that contains the Dockerfile.
2. The client uploads that package to the server over SSH.
3. The server builds the Docker image locally.
4. The server replaces the running container and updates any related routing or schedule metadata.

This avoids duplicating packaging logic in every project while keeping container builds off the developer laptop. It also matches the useful part of the Cloud Run `--source` model without requiring Cloud Build or flag compatibility.

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
- Host-level `systemd` units per application

## Open Design Questions

These are intentionally unresolved until implementation options are evaluated:

- How to represent service metadata: YAML, JSON, shell files, or generated unit-like files.
- How cron registration should be stored and synchronized with host cron.
- Whether logs need a lightweight aggregation story beyond `docker logs`.

## Initial Direction

Start with the smallest useful platform:

1. A host bootstrap step installs Docker, Caddy, cron integration, and one Declouding systemd unit.
2. A CLI registers web services, long-running services, and cron jobs in plain host-side configuration.
3. Web service registration updates Caddy and reloads it.
4. Deployments upload source packages, build Docker images on the server, then replace the running container.
5. Secrets and persistent data are always explicit mounts.

This should cover the current low-traffic workloads while keeping the implementation understandable and operable over SSH.
