# Journald log driver with per-service tag

Originating task: `_tasks/2026-05-12-journald-log-driver/`. Plan: `02-plan.md`. Tech plan: `03-tech-plan.md` (revision 2). Linus approval: `04-linus-plan-review.md` and the follow-up rev 2 sign-off (commit `8c0dfad`).

## Context

Docker's default `json-file` log driver stores container stdout/stderr inside the container's writable layer. When Decloud redeploys a service the old container is `docker stop`+`docker rm`'d (M1 `recreate` strategy), and with it the log history disappears — both `decloud logs` and `docker logs` against the new container start from zero. The same hits `decloud-caddy` every time the operator runs `decloud caddy down && up`.

Two host-side log drivers solve this: `syslog` (per-service files on the host) and `journald` (the systemd journal). We picked journald.

## Decision

**Every container Decloud starts — service containers and `decloud-caddy` — is created with `--log-driver=journald --log-opt tag=decloud/<service>`.** No flag, no env var, no opt-out. The Docker daemon already runs under systemd on every Decloud install (see `_docs/install.md` §2), so journald is universally available.

The tag literal is `decloud/<service>`:

- Service containers: `decloud/myservice` (the operator-chosen `--name`).
- The Caddy manager container: `decloud/caddy` (hardcoded in `internal/caddy/manager.go:127`).

`journalctl CONTAINER_TAG=decloud/<service>` returns every line that container ever wrote, across redeploys and host reboots. The leading `decloud/` namespace is purely presentation — journald stores `/` verbatim in `CONTAINER_TAG`, no sanitisation, no escape. `journalctl FIELD=VALUE` matches exactly (no prefix form, no glob); operators wanting "everything Decloud wrote" pass one `CONTAINER_TAG=decloud/<name>` per service, OR'd together by `journalctl`'s same-field disjunction rule. The leading `decloud/` namespace is a human-reading aid, not a queryable prefix.

The flag pair is emitted in both `Run` and `RunWithOptions` (`internal/dockerdrv/cli_driver.go:58` and `:232`), spliced immediately after `--restart` and before any env/label/port/volume flags — matches existing flag-order discipline.

## Why journald and not syslog

- `docker logs` continues to work natively. Journald is one of the few non-default drivers where the Docker daemon can read logs back from the underlying store (the daemon shells out to `journalctl` internally). `decloud logs` is a thin pass-through to `docker logs` and required zero changes; with syslog we would have had to either teach the deploy command to read syslog files or accept that `decloud logs` stopped working.
- One backing store, queryable with one well-known tool (`journalctl`). Syslog spawns a per-service file management problem (rotation, retention, mode bits) that Decloud would have to either implement or punt to the operator's syslog config.
- Cross-restart history survives without any per-service setup. Journald is already running, already indexed, already rotated by `journald.conf`.

Rejected: a config knob to choose driver. Decloud already targets systemd hosts (Docker daemon is supervised by `systemctl`); there is no M1–M5 use case for opting out of journald. Optionality here would buy nothing and force every test fixture to pick a value.

## Why `Service` flows explicitly, not derived from `Name`

`cli_driver.go` used to derive the `decloud.service` label by `strings.TrimPrefix(req.Name, "decloud-")` — a fragile stringly-typed trick. This task added `Service string` as an explicit field on `RunRequest` and `RunOptions`. Every caller (`internal/deploy/service.go:246` for fresh deploy, `:379` for rollback, `internal/deploy/lifecycle.go:69` for absent-branch re-run, `internal/caddy/manager.go:127` for Caddy) populates it from the source of truth available at the call site.

The `TrimPrefix` smell is gone; the same field now feeds both the journald tag and the `decloud.service` label. Important for M4: when blue/green ships and container names become `decloud-<name>-<deploy-id>` (see `_ai/container-naming.md`), the tag derivation does NOT change with it — `Service` stays the service name, not the container name. A future contributor who tries to re-derive the tag from `Name` would silently introduce per-deploy tag drift that breaks `journalctl CONTAINER_TAG=decloud/<service>` queries across deploys.

## The two sentinel errors

`internal/dockerdrv/driver.go:22-34` declares:

- `ErrEmptyService` — returned by `Run` and `RunWithOptions` when `Service` is empty. Programmer error at the call site.
- `ErrInvalidService` — returned when `Service` contains `/`. Defends the journald-tag-unambiguity invariant: with `decloud/foo/bar` as a tag, `journalctl CONTAINER_TAG=decloud/foo` would partial-match and surface lines from an unrelated container.

Both guards fire in both `Run` and `RunWithOptions` BEFORE the `args` slice is built, so no `docker` process is ever spawned on a bad input. The guards are two-line `if` blocks at the top of each function.

The slash guard is defensive — no caller in M1–M2 can actually introduce a `/` through the normal flow (service names come from `--name` which is documented as `[a-z][a-z0-9-]{0,38}` in `internal/cli/deploy_service.go:57`). But the documented regex is NOT code-enforced anywhere (`internal/registry/store.go` validates non-empty only); the driver-level guard locks the journald-tag invariant against future regressions without depending on upstream validation we don't actually have.

## `docker start` does NOT re-emit log flags

`HostConfig.LogConfig` is sealed at `docker create`/`docker run` time and persists across `docker start`. `internal/dockerdrv/cli_driver.go` deliberately does not pass `--log-driver` or `--log-opt` to `docker start` — the log configuration was set when the container was created, and `docker start` reuses it. Locked by `TestCLIDriver_StartArgs` with two `assert.NotContains` assertions. A future "consistency" refactor that adds log flags to `docker start` argv would now fail the test with an error message naming the invariant.

## Acceptance evidence on a live host

- `docker inspect decloud-<svc> --format '{{.HostConfig.LogConfig.Type}}'` → `journald`.
- `docker inspect decloud-<svc> --format '{{index .HostConfig.LogConfig.Config "tag"}}'` → `decloud/<svc>`.
- `decloud logs <svc>` and `decloud logs <svc> -f --tail 50` behave identically to the pre-change `json-file` shape.
- After `decloud unregister <svc>` + `decloud deploy service --name <svc> ...`, `journalctl CONTAINER_TAG=decloud/<svc>` shows lines from BOTH the pre-redeploy and post-redeploy container instances. Linux-only — not covered by a unit test, validated manually.

## Forward-looking notes

- **`decloud logs --history`.** Surfacing the journald archive through `decloud logs` (so the operator does not need to know about `journalctl` or `CONTAINER_TAG`) is a follow-up; logged in `_ai/m1x-backlog.md` item 12. Out of scope here.
- **Log retention.** Journald has its own retention (`journald.conf`'s `SystemMaxUse`, `MaxRetentionSec`). Decloud does not tune it. Operator's concern.
- **Docker daemon without systemd.** Surfaces as a `docker run` stderr at container-start time; Docker's daemon raises the error directly from the journald log-driver plugin when it cannot reach the systemd journal. Loud and operator-visible; not our problem to detect at deploy time. Documented in `_docs/install.md` §1.
- **M4 container renaming.** When `decloud-<name>-<deploy-id>` lands, the `Service` field stays `<name>` (the service identity), not the container name. `_ai/container-naming.md` is the canonical record of the M4 rename; this decision and that one need to stay aligned.

## Why this isn't in `_docs/`

`_docs/usage.md` §6 documents how operators query journald (`journalctl CONTAINER_TAG=decloud/<service>`) and what `decloud logs` does and does not see. This file documents **why** the driver and tag look the way they do — the alternatives we rejected, the field-flow rule, the two sentinel errors, the `docker start` invariant. Operators do not need this to use the system; future contributors need it to avoid relitigating "should we add a `--log-driver` flag" or "can we just derive the tag from `Name`".
