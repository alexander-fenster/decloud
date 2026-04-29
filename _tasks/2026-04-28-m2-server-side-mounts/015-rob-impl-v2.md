# 015 — Rob's M2 EXECUTION v2 implementation report

EXECUTION v2 per Don's closeout (`012-don-closeout.md`), Joel's
addendum-v2 (`013-joel-tech-plan-addendum-v2.md`), and Linus's APPROVED
review (`014-linus-addendum-v2-review.md`). Four fixes, one commit,
plus a hand-off note for the run-log artifact that cannot be produced
on this dev box (no Docker on Mac).

## TL;DR

All four fixes applied mechanically against Joel's addendum-v2. Sanity
checks (`go build ./...`, `go build -tags integration ./...`,
`go vet ./...`, `go test ./...`, `gofmt -l .`) all clean. The
real-Docker run-log artifact (`integration-test-run-log.txt`) is the
maintainer's responsibility post-merge; hand-off note explaining why and
how is committed alongside this report at
`_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md`.

## Fix 1 — integration test image swap

**File:** `/Users/fenster/dev/decloud/internal/integration/mount_test.go`
**Line:** 23

**Before:**

```go
	mountTestImage       = "alpine:3.19"
```

**After:**

```go
	mountTestImage       = "nginx:alpine"
```

No other changes to the file. The test's setup, cleanup discipline,
network ensure, image pull, `driver.Run` shape, and `driver.Exec`
assertion all remain identical. nginx:alpine idles in the foreground via
`nginx -g daemon off;`, so the container stays alive long enough for
`docker exec cat /data/marker.txt` to succeed against the bind mount —
the failure mode that `alpine:3.19`'s `/bin/sh` default CMD created
under `docker run -d`. Linus Option A per `011 §5`, ratified by Don §1
and Joel §1.4.

## Fix 2 — Mount.HostPath doc-comment

**File:** `/Users/fenster/dev/decloud/internal/registry/types.go`
**Line:** 60 (above the `HostPath` field of `Mount`)

**Before:**

```go
type Mount struct {
	HostPath      string `toml:"host_path"`
	ContainerPath string `toml:"container_path"`
	ReadOnly      bool   `toml:"read_only"`
}
```

**After:**

```go
type Mount struct {
	// HostPath is the mount source. For bind mounts it is an absolute host
	// path starting with "/"; for named volumes it is the volume name. The
	// TOML key is historically named host_path. Use Mount.IsNamed() to
	// distinguish at runtime.
	HostPath      string `toml:"host_path"`
	ContainerPath string `toml:"container_path"`
	ReadOnly      bool   `toml:"read_only"`
}
```

Three-line doc-comment promoting the named-volume-aliasing convention
(currently only documented on `Mount.IsNamed()` in `mount.go`) to first
sight when reading `types.go`. No field types changed, no TOML tags
changed, no behaviour delta. Kevlin Fix A per `010 §10`, locked
verbatim by Joel §2.2.

## Fix 3 — _docs/usage.md tense slip

**File:** `/Users/fenster/dev/decloud/_docs/usage.md`
**Line:** 3

**Before:**

```
Operator-facing reference for the Decloud M1 CLI. For host setup, see [`install.md`](./install.md).
```

**After:**

```
Operator-facing reference for the Decloud CLI. For host setup, see [`install.md`](./install.md).
```

Single change: dropped "M1 " from "Decloud M1 CLI" → "Decloud CLI".
M2 has shipped a real new flag end-to-end, so the "M1 CLI" framing was
a stale tense slip. Kevlin Fix B per `010 §10`, locked verbatim by
Joel §3.2.

## Fix 4 — m1x-backlog.md item 6 rewrite + item 11 future-author note

**File:** `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`

### 4.a — Item 6 "M2 delivery" paragraph rewrite (line 59)

**Before:**

> **M2 delivery:** `internal/integration/mount_test.go` with
> `//go:build integration` tag, gated on `DECLOUD_INTEGRATION=1`.
> Brings up `decloud caddy up`, builds a tiny test image, deploys with
> `--mount=<tmpdir>:/data:ro`, asserts `docker exec` reads the marker
> file, and tears down through `t.Cleanup` with idempotent
> `docker rm -f`. Mount-only — no curl-through-Caddy step (split per
> Joel decision 8 of the M2 tech plan).

**After:**

> **M2 delivery:** `internal/integration/mount_test.go` with
> `//go:build integration` tag, gated on `DECLOUD_INTEGRATION=1`.
> Pulls `nginx:alpine` via the real `dockerdrv.CLIDriver`, calls
> `driver.Run` directly with a `Volumes: [...]` shape carrying one
> bind ro mount, and asserts `docker exec cat /data/marker.txt`
> returns the marker bytes. Cleanup via `t.Cleanup` with idempotent
> `docker rm -f decloud-mounttest`. Does NOT exercise the deploy
> orchestrator (build, readiness, Caddyfile generation, reload) —
> those are split to item 10 (curl-through-Caddy ingress test) per
> Joel decision 8 of the M2 tech plan. The `nginx:alpine` choice
> (rather than alpine) is deliberate: nginx idles in the foreground
> via `nginx -g daemon off;`, so the container stays alive long enough
> for `docker exec`; alpine's default `/bin/sh` CMD exits under
> `docker run -d` (Linus's catch in `011-linus-impl-review.md` §5,
> fix in EXECUTION v2).

Reflects shipped reality (driver-direct test, no `decloud caddy up`, no
test image build, no deploy orchestrator). The trailing sentence
explaining why nginx:alpine was chosen is the load-bearing self-
documentation Linus called out at §3 of his addendum review: without
it, future-Don would read the backlog and ask "why nginx?". Kevlin Fix
C per `010 §10`, with Joel §4.2's two deltas (image name post-fix +
explanatory sentence).

### 4.b — Item 11 future-author note (after line 111 "Originator")

Appended a new paragraph after item 11's `**Originator:**` line, before
the `---` separator:

```
**Future-author note (Linus Observation A, recorded at M2 closeout):**
When picking up this consolidation, the unified `RunOptions` should
grow `Cmd []string` so future integration tests (or one-shot
job/migration runners at M5+) don't need to pick a specific image with
an idle CMD. The M2 integration test exposed this gap: `alpine:3.19`
exits under `docker run -d` because its default CMD is `/bin/sh`
reading closed stdin; M2 worked around this by switching the test to
`nginx:alpine` (which idles in the foreground). Adding `Cmd []string`
to the consolidated `RunOptions` removes that constraint and aligns
the run path with `ExecOptions.Cmd`. Source:
`_tasks/2026-04-28-m2-server-side-mounts/011-linus-impl-review.md`
§"Observation A".
```

Names the gap, names the workaround, names where to look for full
context, and explicitly says "consolidated `RunOptions`" so future-
author lands the field on the post-consolidation struct (not on the
deprecated `RunRequest`). Linus Observation A per `011`, locked by
Joel §4.3.

## Sanity-check results

All five gates clean.

### `go build ./...`

```
(empty output — clean build)
```

### `go build -tags integration ./...`

```
(empty output — clean build with integration tag, which compiles
mount_test.go itself)
```

This gate is what catches an accidentally-broken constant change.
Empty output = `mount_test.go` still compiles after the
`mountTestImage` rename.

### `go vet ./...`

```
(empty output — clean)
```

### `go test ./...`

```
?   	github.com/alexander-fenster/decloud/cmd/decloud	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/caddy	(cached)
?   	github.com/alexander-fenster/decloud/internal/caddy/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/cli	(cached)
?   	github.com/alexander-fenster/decloud/internal/cli/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/config	(cached)
ok  	github.com/alexander-fenster/decloud/internal/deploy	(cached)
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv	(cached)
?   	github.com/alexander-fenster/decloud/internal/dockerdrv/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/envcap	(cached)
?   	github.com/alexander-fenster/decloud/internal/envcap/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/ids	(cached)
ok  	github.com/alexander-fenster/decloud/internal/logging	(cached)
ok  	github.com/alexander-fenster/decloud/internal/registry	(cached)
?   	github.com/alexander-fenster/decloud/internal/registry/mocks	[no test files]
```

All packages green. Integration is build-tagged out of `go test ./...`,
so `mount_test.go` is excluded here — which is correct.

### `gofmt -l .`

```
(empty output — every Go file in the tree is gofmt-clean)
```

## Hand-off note — run-log not produced here

Per Don §1, Joel §5, and Linus §2, the closeout gate is a real-Docker
run-log committed to
`_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log.txt`
showing `--- PASS: TestIntegration_MountBindRoundTrip`. **That file
cannot be produced in this dev environment** because the dev box is a
Mac with no Docker installed and no plan to install it.

I committed a hand-off note at
`_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md`
explaining:

- Why the run-log file is missing (no Docker on Mac).
- The exact command the maintainer needs to run on his Linux host:
  `DECLOUD_INTEGRATION=1 go test -tags integration -v ./internal/integration/... 2>&1 | tee _tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log.txt`
- What PASS line to grep for.
- That the squash-merge into `main` MUST NOT happen until this
  run-log lands and shows PASS.

The hand-off note explicitly references all three closeout-gate
sign-offs (Don §1, Joel §5.1, Linus §2) so it's evident at file-read
time why the gate is non-negotiable.

## Files touched in this commit

Production code (one file):

- `/Users/fenster/dev/decloud/internal/integration/mount_test.go` — Fix 1
  (line 23 image-constant change).

Doc files (three files):

- `/Users/fenster/dev/decloud/internal/registry/types.go` — Fix 2
  (three-line doc-comment above `Mount.HostPath`).
- `/Users/fenster/dev/decloud/_docs/usage.md` — Fix 3 (line 3 tense
  slip).
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md` — Fix 4 (item 6
  paragraph rewrite + item 11 future-author note).

Task-dir files (two files):

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md`
  — new, hand-off note for the missing run-log artifact.
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/015-rob-impl-v2.md`
  — this report.

Six files total, one commit, message
`fix(m2): closeout fixes — nginx:alpine integration test + doc tidy`.

## What I did NOT do

- I did NOT run the integration test against real Docker (no Docker on
  this box). That's the maintainer's job per the hand-off note.
- I did NOT amend Kent's report (`007-kent-tests.md`) or my own v1
  report (`008-rob-impl.md`) — Linus §3.4 explicitly said no amendment
  needed; both are snapshots of their respective commits. v2 work has
  its own report (this file).
- I did NOT touch any other M2 surface code beyond the four locked
  fixes. The `Driver.Run` argv shape, the `Mount` struct field shape,
  the `--mount` flag wiring, the loader/runtime path, the unit-test
  surface — all unchanged from `ae87320`.
- I did NOT add `Cmd []string` to `RunRequest` or `RunOptions`. That's
  m1x-item-11 territory per Don §1, Joel §1.2, Linus §1; the
  future-author note in item 11 records the gap for whoever picks it
  up.
- I did NOT pin `nginx:alpine` to a SHA digest. Linus §2 explicitly
  flagged that as a tempting-but-out-of-scope hardening; if the digest
  ever matters it can be a future m1x-backlog item.

## Sources read

- `_tasks/2026-04-28-m2-server-side-mounts/012-don-closeout.md`
- `_tasks/2026-04-28-m2-server-side-mounts/013-joel-tech-plan-addendum-v2.md`
- `_tasks/2026-04-28-m2-server-side-mounts/014-linus-addendum-v2-review.md`
- `internal/integration/mount_test.go`
- `internal/registry/types.go`
- `_docs/usage.md`
- `_ai/m1x-backlog.md`
