# Usage

Operator-facing reference for the Declouding M1 CLI. For host setup, see [`install.md`](./install.md).

Every command runs on the host that owns `/opt/declouding/`. The operator either SSHes in and runs `decloud` directly, or runs it through some other transport — Declouding does not care. There is no client binary in M1.

## 1. Quick start

A service is a directory containing a `Dockerfile` and, optionally, an `env.sh` script. Declouding builds the image from that directory, captures the environment your `env.sh` exports (if present), and runs the resulting container.

If you do not pass `--env-file`, Declouding looks for `<source-dir>/env.sh` and uses it if it exists; if it does not, the container runs with no captured environment. Passing `--env-file=<path>` to a missing file is a hard error (exit 10) — auto-discovery is silent, but explicit asks must succeed.

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
| `--mount` | string (repeatable) | none | no | Rejected with exit 10 in M1. Persistent volumes are M3. |
| `--config-root` | string | `$DECLOUD_ROOT` or `/opt/declouding` | no | Root directory of the Declouding tree. Persistent flag, applies to every subcommand. Logs are written to `<config-root>/logs/decloud.log` (the flag controls log placement as well as registry/Caddy paths). |

The `env.sh` model. The script is sourced inside a hermetic `bash` invocation; whatever it `export`s ends up in the container's environment, never baked into the image. Arbitrary shell is allowed — computed values, conditional exports, subshell calls. The script is re-evaluated only at deploy time, so restarts are fast and reproducible. Borderline cases worth knowing:

- `set +a` in the script disables auto-export; variables exported before it are captured, those after are not.
- Bash arrays capture only the first element (`MY_ARR=(a b c)` becomes `MY_ARR=a`).
- Reassigning a `readonly` variable causes the capture to fail with exit 20 (`ExitEnvCaptureFail`).

What the deploy actually does, in order:

0. Ensure the `decloud` Docker network exists. Missing networks are created on the fly; failures here surface as exit 40.
1. Capture the environment from `env.sh` (skipped if no env script is in play).
2. Build the image with `docker build`.
3. Stop and remove any previous container for this service.
4. Run the new container on the `decloud` network.
5. Wait for `GET <readiness-path>` to return `200 OK` from the host (probing the container's bridge IP directly; ports are not published to the host).
6. Persist the service registration to `/opt/declouding/config/services/<name>.toml` and `/opt/declouding/secrets/<name>/env.toml`.
7. Regenerate the Caddyfile, run `caddy validate` against a temporary file, atomically rename it into place, and ask Caddy to reload.

If any step fails, the deploy aborts, surfaces a non-zero exit code, and does what it can to leave the system in a coherent state. `caddy validate` runs before the rename, so a syntactically broken Caddyfile cannot reach disk; the previous Caddyfile is preserved and Caddy keeps serving.

## 3. Exit codes

| Code | Constant | Meaning |
|---|---|---|
| `0` | `ExitOK` | Success. |
| `2` | `ExitUsageError` | Missing or unknown flag, missing arguments, internal usage misuse. |
| `10` | `ExitConfigError` | Registry rejection (unknown service, schema mismatch, bad file mode, missing secrets, `--mount` used, `--strategy` other than `recreate`); explicit `--env-file=<path>` pointing at a missing or unreadable file; `decloud stop`, `start`, `restart`, or `logs` against a container that is not registered. |
| `20` | `ExitEnvCaptureFail` | `env.sh` failed to source or capture (readonly conflict, syntax error, non-zero exit). |
| `30` | `ExitBuildFail` | `docker build` failed. |
| `40` | `ExitRunFail` | A docker driver call failed: `docker run`, `docker start`, `docker inspect`, `docker logs`, or `docker network create` (the deployer ensures the `decloud` network on every deploy). `docker stop` against a non-existent container surfaces as exit 10, not 40. |
| `50` | `ExitReadinessFail` | The new container did not return `200 OK` within `--readiness-timeout`. |
| `60` | `ExitCaddyReloadFail` | `caddy validate` rejected the generated Caddyfile, or `caddy reload` failed at runtime. |
| `70` | `ExitInternal` | Anything else (unwrapped I/O error, panic-recovered error). |

## 4. Lifecycle commands

All seven ship in M1. Each takes `--config-root` as the only persistent flag.

- `decloud unregister <name>` — full removal. Stops and removes the container (idempotent — it is fine if the container is already gone), deletes both registry files, regenerates and reloads the Caddyfile so the service's routes disappear.
- `decloud start <name>` — start a previously deployed service. If the container is `running`, no-op. If `exited`, runs `docker start`. If gone (`absent`), re-runs the container from the previously deployed image and the saved environment. `start` does not rebuild — that is `deploy service`'s job. If the image is no longer in the local cache, `start` fails with exit 40.
- `decloud stop <name>` — `docker stop` with a 10-second grace period. The registry is not modified, no Caddy reload happens. While stopped, requests for the service's hostname return `502` from Caddy.
- `decloud restart <name>` — stop, then start. Reuses the same container; does not rebuild. To recreate from source, run `deploy service` again.
- `decloud status <name>` — runtime state plus registry view. Output is one line.
- `decloud logs <name> [-f] [--tail N]` — pass-through to `docker logs`. `-f` follows; `--tail N` shows the last N lines (`0` means all).
- `decloud caddy reload` — regenerate the Caddyfile from the registry, validate it, atomic-rename it into place, and tell Caddy to reload. Use this if you edited something out of band and need Caddy back in sync with the registry. **Warning:** this regenerates from registry state and discards any manual edits to `/opt/declouding/config/caddy/Caddyfile`. Edit the registry, not the Caddyfile.

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

Roll back. Declouding does not keep an image archive in M1; you roll back by re-deploying a previous source revision:

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

Declouding deliberately does not publish container ports to the host (`docker run -p ...` is never invoked). Caddy is the only public ingress, and it reaches each container by name over the shared `decloud` Docker network. The readiness probe reaches containers the same way, via their bridge IP.

If you need to probe a container directly from the host — for example, the readiness probe is failing and you want to bypass Caddy — use `docker exec`:

```sh
docker exec -it decloud-myservice sh
# inside the container:
wget -q -O- http://localhost:8080/healthz
```

Substitute whichever HTTP client your image has (`curl`, `wget`, or whatever the language runtime ships). Do not modify the deploy to add `-p` host port mappings; the network model is part of M1 by design.

## 7. Recovering from `caddy reload` failures (exit 60)

The deploy validates the new Caddyfile with `caddy validate` before writing it to disk, so most reload failures fail fast with the previous Caddyfile and Caddy's running config both untouched. The error message names the temporary file path; investigate with `caddy validate --config <tmp-path>`.

If validation passed but the actual `caddy reload` failed (rare — usually a runtime issue like a port already bound, certificate provisioning failure, or upstream DNS error), the new Caddyfile is on disk and reflects the new state, but Caddy is still serving the old config in memory. To recover:

1. Read the Caddy error log (`journalctl -u caddy`).
2. If the failure is in a specific service's stanza, `decloud unregister <name>` removes that stanza and regenerates.
3. Otherwise, fix the underlying issue and run `decloud caddy reload`.
