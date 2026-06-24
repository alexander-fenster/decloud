# Rob — Implementation Report: Docker network IPv6 support

## Summary

Implemented the minimal change from Don `002` / Joel `003` / Linus `004` to make
Kent's extended test (`005`) pass. Two production files touched. No interface
change, no mock regeneration, no test edits.

## Verification before implementing

- Confirmed `bytes` is already imported in `cli_driver.go` (line 4) — the
  `stderr=%q` completion needs no new import.
- Confirmed `caddy.NetworkName = "decloud"` exists (`internal/caddy/manager.go:20`)
  and `deploy/service.go` already imports `caddy` (`service.go:12`). No new
  import, no import cycle (`caddy` does not import `deploy`).
- Read Kent's test diff: it references the package-level `decloudIPv6Subnet`
  const (white-box) and uses the existing `indexOf` helper. The expected
  compile-failure was `undefined: decloudIPv6Subnet`.

## Changes

### `internal/dockerdrv/cli_driver.go`

1. Added unexported `const decloudIPv6Subnet = "fd00:dec0:11d::/64"` immediately
   above `NetworkEnsure`, with the WHY comment (ULA/RFC 4193 + NAT66; internal
   driver detail, not an operator knob).
2. Changed ONLY the create call. Lines 177-179 (`docker network inspect` +
   early-return) are byte-for-byte unchanged. The create now passes variadic
   args `"network", "create", "--ipv6", "--subnet", decloudIPv6Subnet, name`.
3. Per Joel §7, completed the create error wrap to the file's house style:
   captured the create command's stderr into a `bytes.Buffer` and added
   `stderr=%q`, matching the other shell-outs (Run, Stop, Inspect, logs, pull).
   No IPv4 `--subnet`, no `--driver` — default bridge preserved for the
   readiness probe.

### `internal/deploy/service.go`

4. Replaced the two `"decloud"` string literals at lines 159 (the
   `NetworkEnsure` arg) and 163 (the log field) with `caddy.NetworkName`.
   Value-identical consolidation onto the single source of truth.

## Did NOT do (correctly out of scope per plan)

- No `EnableIPv6` inspection, no `--format` on the inspect call.
- No `docker network rm` / recreate / reconcile.
- No test edits — implementation satisfies Kent's test as written.
- No broader `NetworkName` audit beyond the two literals on the touched path.

## Verification output

`go build ./...` → `BUILD OK` (exit 0).

`gofmt -l internal/dockerdrv/cli_driver.go internal/deploy/service.go` → empty
(clean).

`go test ./internal/dockerdrv/... ./internal/deploy/...`:

```
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv	0.071s
?   	github.com/alexander-fenster/decloud/internal/dockerdrv/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/deploy	12.090s
```

`go test ./internal/dockerdrv/... -run TestCLIDriver_NetworkEnsure -v`:

```
=== RUN   TestCLIDriver_NetworkEnsureWhenAbsent
--- PASS: TestCLIDriver_NetworkEnsureWhenAbsent (0.01s)
=== RUN   TestCLIDriver_NetworkEnsureWhenPresent
--- PASS: TestCLIDriver_NetworkEnsureWhenPresent (0.00s)
PASS
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv	0.014s
```

`go test ./...` → all packages `ok` (full suite green; no Docker daemon needed,
`scriptedFactory` stubs the `docker` binary via `/bin/sh -c`).

## Notes for downstream (Raymond / docs)

- Fresh installs create the `decloud` network with `--ipv6 --subnet
  fd00:dec0:11d::/64`. The IPv4 subnet is still auto-allocated by Docker (no
  IPv4 `--subnet` pinned), so `ContainerIP`'s IPv4 read is unchanged.
- An already-existing IPv4-only network is left untouched (the inspect
  early-return). There is no docker command to toggle `EnableIPv6`, so an
  existing network is upgraded out-of-band by the operator.

## Follow-up (Don adjudication, addendum to `002-plan.md`, commit 96637fb)

Kevlin found three more raw `Network: "decloud"` literals in
`internal/deploy/service.go` naming the SAME network consolidated at :159/:163.
Don ruled them in scope so the "single source of truth" claim is actually true.
Swapped all three to `caddy.NetworkName` (verified each existed first):

- `service.go:254` — `dockerdrv.RunRequest{...}` (deploy path).
- `service.go:324` — `registry.RunSpec{...}` (persisted to registry config; the
  field is a plain `string`, so assigning the const is fine — no new import).
- `service.go:387` — `dockerdrv.RunRequest{...}` (redeploy path).

Behavior-neutral (every value is `"decloud"` today). `grep '"decloud"'` on
`service.go` now returns nothing. The redeploy-semantics question (whether :387
should read `prev.Config.Run.Network`) is explicitly OUT OF SCOPE per Don.

Verification: `gofmt -l internal/deploy/service.go` clean; `go build ./...` OK;
`go test -count=1 ./internal/deploy/...` → `ok` (12.077s); `go test ./...` all
packages `ok`.
