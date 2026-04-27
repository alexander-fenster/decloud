# 016 — Rob's Cycle-2 Implementation Report

Author: Rob Pike (implementation engineer)
Date: 2026-04-27
Status: Cycle-2 EXECUTION 3.2 complete. Kent's `TestManager_UpPortsBoundActionableError` passes (both sub-tests). `Long` help text rendered for both `caddy up` and `caddy down`. Full suite green.

## Reading log

1. `_tasks/.../013-joel-tech-plan-cycle2.md` — §1.3 (exact code shape), §1.5 (test contract), §4.1/§4.2 (verbatim `Long` text).
2. `_tasks/.../015-kent-tests-cycle2.md` — verified the test's actual assertions, especially the `NotContains(": docker run: docker run:")` branch-choice assertion.
3. `internal/caddy/manager.go` — current `Up` body and `runOpts` location for helper placement.
4. `internal/caddy/manager_test.go` lines 179-208 — Kent's added test (literal assertions).
5. `internal/cli/caddy_up.go` and `internal/cli/caddy_down.go` — current command literals.

## Files changed

### `internal/caddy/manager.go`

1. Added `"strings"` to imports.
2. Replaced the `RunWithOptions` failure branch (was lines 94-96):
   ```go
   if _, err := m.cfg.Driver.RunWithOptions(ctx, m.runOpts()); err != nil {
       if isPortsBoundErr(err) {
           return fmt.Errorf("%w: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent", ErrCaddyUp)
       }
       return fmt.Errorf("%w: docker run: %w", ErrCaddyUp, err)
   }
   ```
3. Appended the helper at the bottom of the file (after `runOpts`):
   ```go
   // isPortsBoundErr matches the two stderr substrings docker emits when a
   // publish-target port is already bound on the host: kernel bind(2) failure
   // ("address already in use") and docker allocator failure ("port is already
   // allocated"). Brittle to docker error-text wording; locked by
   // TestManager_UpPortsBoundActionableError so a docker upgrade that reworded
   // the message would fail loudly.
   func isPortsBoundErr(err error) bool {
       s := err.Error()
       return strings.Contains(s, "address already in use") ||
           strings.Contains(s, "port is already allocated")
   }
   ```

The actionable branch wraps `ErrCaddyUp` with `%w` only — it does NOT also wrap the inner driver error. That's deliberate: re-wrapping `err` would re-introduce the doubled `docker run: docker run:` chain that Kent's `NotContains` assertion locks against. The driver-side error chain is intentionally suppressed on the actionable branch because the actionable text already names the recovery commands; the chain added noise without information. The generic-wrap fall-through path is unchanged.

### `internal/cli/caddy_up.go`

Added `Long:` field with the verbatim text from Joel §4.1 — three paragraphs covering: what the command does (network ensure + Caddyfile stub + dual-stack publishing on 80/443), named-volume retention warning, and idempotency contract.

### `internal/cli/caddy_down.go`

Added `Long:` field with the verbatim text from Joel §4.2 — three paragraphs covering: ingress interruption warning, named-volume retention plus the LE rate-limit warning if volumes are wiped, and idempotency contract.

## Verification

### `go build ./...`

```
$ go build ./...
(no output, exit 0)
```

### `go vet ./...`

```
$ go vet ./...
(no output, exit 0)
```

### `gofmt -l internal/`

```
$ gofmt -l internal/
(no output)
```

### `go test ./internal/caddy/... -run TestManager_UpPortsBound -count=1 -v`

```
=== RUN   TestManager_UpPortsBoundActionableError
=== RUN   TestManager_UpPortsBoundActionableError/kernel_bind
=== RUN   TestManager_UpPortsBoundActionableError/docker_allocator
--- PASS: TestManager_UpPortsBoundActionableError (0.00s)
    --- PASS: TestManager_UpPortsBoundActionableError/kernel_bind (0.00s)
    --- PASS: TestManager_UpPortsBoundActionableError/docker_allocator (0.00s)
PASS
ok  	github.com/alexander-fenster/decloud/internal/caddy	0.008s
```

Both sub-tests green. All five assertions per sub-test pass:

1. `errors.Is(err, caddy.ErrCaddyUp)` — sentinel preserved via `%w`.
2. `Contains(err.Error(), "ports 80/443 already in use")` — actionable symptom.
3. `Contains(err.Error(), "systemctl disable --now caddy && systemctl mask caddy")` — recovery #1.
4. `Contains(err.Error(), "apt-get remove -y caddy")` — recovery #2.
5. `NotContains(err.Error(), ": docker run: docker run:")` — branch-choice locked (the actionable branch does not double-wrap the driver chain).

### `go test ./... -count=1`

```
ok  	github.com/alexander-fenster/decloud/internal/caddy	0.021s
ok  	github.com/alexander-fenster/decloud/internal/cli	0.022s
ok  	github.com/alexander-fenster/decloud/internal/config	0.012s
ok  	github.com/alexander-fenster/decloud/internal/deploy	12.073s
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv	0.071s
ok  	github.com/alexander-fenster/decloud/internal/envcap	0.104s
ok  	github.com/alexander-fenster/decloud/internal/ids	0.016s
ok  	github.com/alexander-fenster/decloud/internal/logging	0.017s
ok  	github.com/alexander-fenster/decloud/internal/registry	0.038s
```

All packages green. No regressions. `TestManager_UpRunFailsWithoutRollback` continues to pass — its sentinel `errors.New("port allocation failed")` does not contain either of the two canonical Docker substrings (`"address already in use"` / `"port is already allocated"`), so the generic-wrap branch still fires for it. C2-3 of Joel §9 acceptance is satisfied.

### Rendered `--help` output

`go run ./cmd/decloud caddy up --help`:

```
Run the decloud-caddy container on the decloud network.

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
stopped, it is started in place.

Usage:
  decloud caddy up [flags]

Flags:
  -h, --help   help for up

Global Flags:
      --config-root string   root directory for /opt/decloud-style layout (env: DECLOUD_ROOT) (default "/opt/decloud")
```

`go run ./cmd/decloud caddy down --help`:

```
Stop and remove the decloud-caddy container.

Stopping Caddy interrupts ingress for ALL services routed by this Decloud
host. Live traffic will fail until 'decloud caddy up' is run again.

The named volumes decloud_caddy_data (ACME state, issued certs) and
decloud_caddy_config (runtime config) are NOT removed. Re-running
'decloud caddy up' brings Caddy back with the same certificates and
runtime state. Wipe the volumes manually with 'docker volume rm
decloud_caddy_data decloud_caddy_config' only if you intend to discard
ACME state — that forces fresh Let's Encrypt issuance and risks tripping
LE rate limits on hosts with many domains.

Idempotent: if the container is already absent, this command exits 0.

Usage:
  decloud caddy down [flags]

Flags:
  -h, --help   help for down

Global Flags:
      --config-root string   root directory for /opt/decloud-style layout (env: DECLOUD_ROOT) (default "/opt/decloud")
```

Both renderings include the operationally-critical warnings: dual-stack publishing details and named-volume retention on `up`; ingress-interruption warning, named-volume retention, and LE rate-limit warning on `down`.

## Notes for Raymond

The literal error string emitted by the new actionable branch is:

```
caddy: up failed: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent
```

This is what `_docs/install.md` line 173 needs to match byte-for-byte per Joel §3.1. The `caddy: up failed:` prefix comes from `ErrCaddyUp.Error()` interpolated via `%w`; the rest is the literal string in the `fmt.Errorf` at `internal/caddy/manager.go::Up`.

## Acceptance criteria coverage (Joel §9)

- C2-1 (kernel-bind substring → actionable): green via `kernel_bind` sub-test.
- C2-2 (allocator substring → actionable): green via `docker_allocator` sub-test.
- C2-3 (generic wrap path still fires for non-port-conflict errors): `TestManager_UpRunFailsWithoutRollback` continues to pass — sentinel does not contain either substring.
- C2-6 (`caddy up --help` includes volume-retention + dual-stack note): rendered above.
- C2-7 (`caddy down --help` includes ingress-interruption + LE rate-limit warnings): rendered above.
- C2-9 (`gofmt -l .` empty): verified.
- C2-10 (`go test ./... -count=1` green): verified.

C2-4, C2-5, C2-8 are Raymond's remit (doc edits + backlog).

## Files touched

- `internal/caddy/manager.go` — added `"strings"` import, added port-conflict branch in `Up`, added `isPortsBoundErr` helper.
- `internal/cli/caddy_up.go` — added `Long` field.
- `internal/cli/caddy_down.go` — added `Long` field.

— Rob
