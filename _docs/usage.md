# Usage

Operator-facing reference for the Decloud M1 CLI. For host setup, see [`install.md`](./install.md).

Every command runs on the host that owns `/opt/decloud/`. The operator either SSHes in and runs `decloud` directly, or runs it through some other transport — Decloud does not care. There is no client binary in M1.

## 1. Quick start

A service is a directory containing a `Dockerfile` and, optionally, an `env.sh` script. Decloud builds the image from that directory, captures the environment your `env.sh` exports (if present), and runs the resulting container.

If you do not pass `--env-file`, Decloud looks for `<source-dir>/env.sh` and uses it if it exists; if it does not, the container runs with no captured environment. Passing `--env-file=<path>` to a missing file is a hard error (exit 10) — auto-discovery is silent, but explicit asks must succeed.

Caddy must already be running before the first deploy. If you have not run `decloud caddy up` since installing, do that once now (see [`install.md` §3](./install.md#3-bring-up-caddy)):

```sh
decloud caddy up
```

A minimal example. Suppose `./myservice/` contains:

```Dockerfile
# myservice/Dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /out/server ./cmd/server

FROM alpine:3.19
COPY --from=build /out/server /usr/local/bin/server
EXPOSE 8080
CMD ["/usr/local/bin/server"]
```

```sh
# myservice/env.sh
export GREETING="hello"
export PORT=8080
```

Deploy it:

```sh
decloud deploy service \
  --name myservice \
  --host myservice.example.com \
  --port 8080 \
  --readiness-path /healthz \
  ./myservice
```

Caddy reloads, fetches a TLS certificate for `myservice.example.com`, and routes traffic to the container.

## 2. The `decloud deploy service` command

Build, run, register, and route a service. M1 uses the `recreate` strategy: stop the old container, then start the new one. There is brief downtime during the swap. Zero-downtime blue/green is M4.

```text
decloud deploy service [flags] <source-dir>
```

| Flag | Type | Default | Required | Notes |
|---|---|---|---|---|
| `--name` | string | — | yes | Service name. Must match `[a-z][a-z0-9-]{0,38}`. |
| `--host` | string (repeatable) | none | no | Public hostname. Repeat for multiple. Caddy obtains a certificate per host. |
| `--port` | int | — | yes | Container's listen port. Required because every M1 service is HTTP and the readiness probe targets this port; missing or `0` fails fast with exit 2 (`--port is required`). Worker/job workloads without an HTTP listener are M5. |
| `--env-file` | string | `<source-dir>/env.sh` if present | no | Path to a bash script whose `export`s become the container's environment. Omitted: auto-discovers `<source-dir>/env.sh`; missing is fine (deploy proceeds with no captured env). Set explicitly: file must exist or the deploy fails with exit 10. |
| `--readiness-path` | string | `/healthz` | no | HTTP path probed for `200 OK` after the container starts. |
| `--readiness-timeout` | duration | `60s` | no | Total wait before the deploy fails with exit 50. |
| `--strategy` | string | `recreate` | no | Only `recreate` is accepted in M1. `blue_green` is rejected with exit 10 (M4). |
| `--dockerfile` | string | `Dockerfile` | no | Path to the Dockerfile. Relative paths resolve under `<source-dir>` regardless of the cwd you invoke `decloud` from. Absolute paths are used as-is. |
| `--mount` | string (repeatable) | none | no | Rejected with exit 10 in M1. Persistent volumes are M2. |
| `--config-root` | string | `$DECLOUD_ROOT` or `/opt/decloud` | no | Root directory of the Decloud tree. Persistent flag, applies to every subcommand. Logs are written to `<config-root>/logs/decloud.log` (the flag controls log placement as well as registry/Caddy paths). |

The `env.sh` model. The script is sourced inside a hermetic `bash` invocation; whatever it `export`s ends up in the container's environment, never baked into the image. Arbitrary shell is allowed — computed values, conditional exports, subshell calls. The script is re-evaluated only at deploy time, so restarts are fast and reproducible. Borderline cases worth knowing:

- `set +a` in the script disables auto-export; variables exported before it are captured, those after are not.
- Bash arrays capture only the first element (`MY_ARR=(a b c)` becomes `MY_ARR=a`).
- Reassigning a `readonly` variable causes the capture to fail with exit 20 (`ExitEnvCaptureFail`).

What the deploy actually does, in order:

0. Ensure the `decloud` Docker network exists. Missing networks are created on the fly; failures here surface as exit 40.
1. Capture the environment from `env.sh` (skipped if no env script is in play).
2. Build the image with `docker build`.
3. Stop and remove any previous container for this service. On a fresh deploy (no registry entry), inspect any container that already happens to be named `decloud-<name>`: if it carries the `decloud.service=<name>` label, treat it as an orphan from a prior interrupted deploy and remove it; if the label is missing or mismatched, refuse with exit 40 (see [§8](#8-interrupting-a-deploy-ctrlc)).
4. Run the new container on the `decloud` network.
5. Wait for `GET <readiness-path>` to return `200 OK` from the host (probing the container's bridge IP directly; ports are not published to the host).
6. Persist the service registration to `/opt/decloud/config/services/<name>.toml` and `/opt/decloud/secrets/<name>/env.toml`.
7. Regenerate the Caddyfile, `docker exec decloud-caddy caddy validate` against a temporary file, atomically rename it into place, and `docker exec decloud-caddy caddy reload`. Requires `decloud caddy up` to have been run; if `decloud-caddy` is not running, the deploy exits 60 with a recovery hint pointing at `decloud caddy up`.

If any step fails, the deploy aborts, surfaces a non-zero exit code, and does what it can to leave the system in a coherent state. `caddy validate` runs before the rename, so a syntactically broken Caddyfile cannot reach disk; the previous Caddyfile is preserved and Caddy keeps serving.

## 3. Exit codes

| Code | Constant | Meaning |
|---|---|---|
| `0` | `ExitOK` | Success. |
| `2` | `ExitUsageError` | Missing or unknown flag, missing arguments, internal usage misuse. |
| `10` | `ExitConfigError` | Registry rejection (unknown service, schema mismatch, bad file mode, missing secrets, `--mount` used, `--strategy` other than `recreate`); explicit `--env-file=<path>` pointing at a missing or unreadable file; `decloud stop`, `start`, `restart`, or `logs` against a container that is not registered. |
| `20` | `ExitEnvCaptureFail` | `env.sh` failed to source or capture (readonly conflict, syntax error, non-zero exit). |
| `30` | `ExitBuildFail` | `docker build` failed. |
| `40` | `ExitRunFail` | A docker driver call failed: `docker run`, `docker start`, `docker inspect`, `docker logs`, `docker network create`, or any failure from `decloud caddy up` / `decloud caddy down`. Also returned when a fresh deploy finds an existing `decloud-<name>` container that lacks the `decloud.service=<name>` label (see [§8](#8-interrupting-a-deploy-ctrlc)). `docker stop` against a non-existent container surfaces as exit 10, not 40. |
| `50` | `ExitReadinessFail` | The new container did not return `200 OK` within `--readiness-timeout`. |
| `60` | `ExitCaddyReloadFail` | `caddy validate` rejected the generated Caddyfile, `caddy reload` failed at runtime, or `decloud-caddy` is not running (run `decloud caddy up`). |
| `70` | `ExitInternal` | Anything else (unwrapped I/O error, panic-recovered error). |
| `130` | `ExitInterrupted` | The deploy was cancelled by the user (ctrl+c / SIGINT or SIGTERM). Follows the POSIX `128 + signal` convention (SIGINT = 2). Distinct from `ExitReadinessFail` so an interrupted deploy is not confused with an app that failed its health check. |

## 4. Lifecycle commands

All M1 commands listed below. Each takes `--config-root` as the only persistent flag.

- `decloud unregister <name>` — full removal. Stops and removes the container (idempotent — it is fine if the container is already gone), deletes both registry files, regenerates and reloads the Caddyfile so the service's routes disappear.
- `decloud start <name>` — start a previously deployed service. If the container is `running`, no-op. If `exited`, runs `docker start`. If gone (`absent`), re-runs the container from the previously deployed image and the saved environment. `start` does not rebuild — that is `deploy service`'s job. If the image is no longer in the local cache, `start` fails with exit 40.
- `decloud stop <name>` — `docker stop` with a 10-second grace period. The registry is not modified, no Caddy reload happens. While stopped, requests for the service's hostname return `502` from Caddy.
- `decloud restart <name>` — stop, then start. Reuses the same container; does not rebuild. To recreate from source, run `deploy service` again.
- `decloud status <name>` — runtime state plus registry view. Output is one line.
- `decloud logs <name> [-f] [--tail N]` — pass-through to `docker logs`. `-f` follows; `--tail N` shows the last N lines (`0` means all).
- `decloud caddy up` — bring up the `decloud-caddy` container on the shared `decloud` network with dual-stack publishing on `80/tcp`, `443/tcp`, `443/udp`. Idempotent: re-running while Caddy is already up logs `caddy already running` and exits 0. Pulls `caddy:2` on first run; uses named volumes `decloud_caddy_data` (ACME state, issued certs) and `decloud_caddy_config` (runtime config). Takes no flags — image and ports are fixed in M1. Run once after install; the container has `--restart=unless-stopped`, so reboots and Docker daemon restarts bring it back automatically.
- `decloud caddy down` — stop and remove the `decloud-caddy` container with a 10-second grace period. The named volumes `decloud_caddy_data` and `decloud_caddy_config` are **not** removed — wipe them with `docker volume rm` if you need a clean ACME slate. Idempotent on an already-absent container.
- `decloud caddy reload` — regenerate the Caddyfile from the registry, validate it inside the running `decloud-caddy` container via `docker exec caddy validate`, atomic-rename it into place, and `docker exec caddy reload`. Use this if you edited something out of band and need Caddy back in sync with the registry. Surface unchanged from M1.0; the implementation now `docker exec`s into the container instead of shelling a host `caddy` binary, so `decloud caddy up` must have been run first. **Warning:** this regenerates from registry state and discards any manual edits to `/opt/decloud/config/caddy/Caddyfile`. Edit the registry, not the Caddyfile.

### Status format

`decloud status <name>` writes a single line:

```text
<name> state=<state> container=<container-name> deploy=<deploy-id> deployed_at=<RFC3339>
```

State values:

- `running` — container is running.
- `stopped` — container exists and exited.
- `absent` — registry has the service but the container is gone.
- `config-only` — the config file exists but the secrets file is missing (a partial-deploy orphan). Run `decloud unregister <name>` to clean up.

`<container-name>` is `decloud-<name>` in M1.

## 5. End-to-end example

You have a Go HTTP server in `./myservice/` with a `Dockerfile` and an `env.sh` (see [§1](#1-quick-start)). Deploy it:

```sh
$ decloud deploy service \
    --name myservice \
    --host myservice.example.com \
    --port 8080 \
    --readiness-path /healthz \
    ./myservice
[...docker build output streams to stdout...]
deploy: myservice ready
```

Inspect:

```sh
$ decloud status myservice
myservice state=running container=decloud-myservice deploy=20260426-093214-7f3a9c deployed_at=2026-04-26T09:32:14Z
```

Watch the logs:

```sh
$ decloud logs myservice --tail 50 -f
2026/04/26 09:32:14 listening on :8080
...
```

Roll back. Decloud does not keep an image archive in M1; you roll back by re-deploying a previous source revision:

```sh
$ git -C ./myservice checkout <previous-sha>
$ decloud deploy service --name myservice --host myservice.example.com --port 8080 ./myservice
```

Stop without unregistering (Caddy keeps the route, requests get `502`):

```sh
$ decloud stop myservice
$ decloud start myservice
```

Remove entirely:

```sh
$ decloud unregister myservice
```

## 6. Debugging a container directly

Decloud does not publish service container ports to the host. The only container that publishes ports is `decloud-caddy` itself — it binds `80/tcp`, `443/tcp`, and `443/udp` on both `0.0.0.0` and `[::]`. Every service container exposes its port only inside the shared `decloud` Docker network; Caddy reaches each upstream by container name (`decloud-<service>`) via Docker's embedded DNS. The readiness probe reaches containers the same way, via their bridge IP.

If you need to probe a container directly from the host — for example, the readiness probe is failing and you want to bypass Caddy — use `docker exec`:

```sh
docker exec -it decloud-myservice sh
# inside the container:
wget -q -O- http://localhost:8080/healthz
```

Substitute whichever HTTP client your image has (`curl`, `wget`, or whatever the language runtime ships). Do not modify the deploy to add `-p` host port mappings on service containers; the network model is part of M1 by design.

## 7. Recovering from `caddy reload` failures (exit 60)

`caddy validate` and `caddy reload` both run as `docker exec` into the `decloud-caddy` container. The host path `/opt/decloud/config/caddy/Caddyfile` maps to `/etc/caddy/Caddyfile` inside the container via a read-only bind mount; the deploy translates host paths to container paths automatically.

The deploy validates the new Caddyfile with `caddy validate` before renaming it into place, so most reload failures fail fast with the previous Caddyfile and Caddy's running config both untouched. The error message names the host-side temporary file path; investigate from inside the container:

```sh
docker exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp
```

If validation passed but the actual `caddy reload` failed (rare — usually a runtime issue like a certificate provisioning failure or upstream DNS error), the new Caddyfile is on disk and reflects the new state, but Caddy is still serving the old config in memory. To recover:

1. Read the Caddy error log (`docker logs decloud-caddy --tail 100`).
2. If the failure is in a specific service's stanza, `decloud unregister <name>` removes that stanza and regenerates.
3. Otherwise, fix the underlying issue and run `decloud caddy reload`.

### `decloud-caddy` is not running

If the deploy exits 60 with text `service is registered and running but Caddy is not routing traffic; run 'decloud caddy up'`, the deploy succeeded — the container is healthy and the registry is updated — but Caddy itself is down. The recovery is one command:

```sh
decloud caddy up
```

If `caddy up` reports `caddy already running` but routing is still broken, follow with `decloud caddy reload` to push the current registry state into the running Caddy.

## 8. Interrupting a deploy (ctrl+c)

Pressing ctrl+c during `decloud deploy service` — most commonly while it is waiting for the readiness probe — cancels the deploy cleanly. The exit code is `130` (`ExitInterrupted`), and the new container is stopped and removed before `decloud` returns. The audit log records `deploy cancelled during readiness wait` at info level rather than `readiness failed` at error level, so an interrupted deploy is distinguishable from a real readiness failure when grepping `/opt/decloud/logs/decloud.log`.

Cleanup runs on a fresh 30-second timeout that is independent of the cancelled request context, so `docker stop` and `docker rm` still execute on the host. SIGTERM behaves identically to SIGINT.

If the cleanup itself fails (rare — typically a hung Docker daemon), `decloud` logs a warning naming the container (`cleanup failed; manual removal may be required`). Two recovery paths exist:

1. **Re-run `decloud deploy service` for the same `--name`.** A fresh deploy detects the orphaned `decloud-<name>` container, verifies it carries the `decloud.service=<name>` label that `decloud` itself attaches to every container it creates, and stops + removes it before starting the new one. The audit log records `removing orphan container from prior interrupted deploy` at warn level. This is the recommended path; you do not need to touch Docker manually.
2. **Remove the container by hand:** `docker rm -f decloud-<name>`.

A second ctrl+c during cleanup does not interrupt cleanup; the Go signal handler installed by `signal.NotifyContext` absorbs it. To force exit before the 30-second `cleanupTimeout` window completes, send SIGKILL (`kill -9 <pid>`); path (1) above still recovers on the next deploy.

### When the orphan was not created by decloud

The orphan-cleanup branch is label-gated. If a container named `decloud-<name>` already exists on the host but does NOT carry the `decloud.service=<name>` label — for example, a container you ran by hand with that name, or a container created by a different tool — `decloud deploy service` refuses with exit `40` (`ExitRunFail`) and the message:

```text
container decloud-<name> exists but was not created by decloud (label decloud.service="..." does not match "..."); refusing to remove. Run 'docker rm -f decloud-<name>' manually if you want to claim this name, or pick a different service name
```

This is a safety guardrail: `decloud` only removes containers it can prove it created. Recovery is whichever the message suggests — `docker rm -f` the container yourself, or pick a different `--name`.

### What this does not cover

`SIGKILL` (kill -9) of `decloud` while a container is running, or a host power loss between `docker run` and the registry save, both leave an orphan that no cleanup defer can reach. The next-deploy orphan recovery in path (1) above is the only mitigation; it works for these cases too because the label gate applies to any orphan, not only ctrl+c orphans.
