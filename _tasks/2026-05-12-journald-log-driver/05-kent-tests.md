# Kent — failing tests for journald log driver

STEP 3a (EXECUTION — tests). Plan v2 approved by Linus; tests now committed
in the red state per TDD.

## What I changed

### Production code — minimal symbol additions (so the tests compile)

`internal/dockerdrv/driver.go`:

- Added `Service string` field to `RunRequest` and `RunOptions`, placed
  between `Name` and `Image` as specified in tech plan §11.5. Doc-commented
  inline: `// service name (populates journald tag decloud/<Service>);
  required, must not contain '/'`.
- Added two sentinel errors next to the existing `ErrContainerNotFound` /
  `ErrNoBridgeIP` block (tech plan §3.3 / §5.1):
  - `ErrEmptyService` — "dockerdrv: Service is empty; populate Service in
    RunRequest/RunOptions".
  - `ErrInvalidService` — "dockerdrv: Service contains '/'; journald tag
    would be ambiguous under journalctl CONTAINER_TAG= prefix queries".

NO change to `internal/dockerdrv/cli_driver.go`. The guard logic and
journald-flag emission are Rob's job — leaving them unimplemented is what
makes the new behavioural tests fail (red).

### Test code

`internal/dockerdrv/cli_driver_test.go`:

| Test | tech-plan ref | t.Name() |
| --- | --- | --- |
| Empty `Service` on `Run` | §6.2.1 | `TestCLIDriver_RunReturnsErrEmptyServiceWhenServiceIsEmpty` |
| Empty `Service` on `RunWithOptions` | §6.2.2 | `TestCLIDriver_RunWithOptionsReturnsErrEmptyServiceWhenServiceIsEmpty` |
| Slash-in-`Service` on `Run` | §6.2.3 | `TestCLIDriver_RunReturnsErrInvalidServiceWhenServiceContainsSlash` |
| Slash-in-`Service` on `RunWithOptions` | §6.2.4 | `TestCLIDriver_RunWithOptionsReturnsErrInvalidServiceWhenServiceContainsSlash` |
| Tag-literal lock on `Run` | §6.2.5 | `TestCLIDriver_RunEmitsJournaldFlagsWithSlashTagLiteral` |
| Tag-literal lock on `RunWithOptions` (caddy) | §6.2.6 | `TestCLIDriver_RunWithOptionsEmitsJournaldFlagsWithCaddyTag` |
| `docker start` does NOT re-emit log flags (existing test, extended) | §6.2.7 | `TestCLIDriver_StartArgs` (added two `assert.NotContains`) |

Plus a small package-local helper `indexOf(args []string, needle string) int`
used by the two tag-literal tests (§6.2.5–§6.2.6). Single source of
truth, no duplication.

`caddyRunOptionsFixture()` now carries `Service: "caddy"` so §6.2.6 can
exercise it without separate construction. §6.1.4 (the existing
`TestCLIDriver_RunWithOptionsCaddyShape`) currently passes because the
expected argv slice doesn't yet include the journald flags. Rob will
update that expected slice as part of the implementation commit per
§6.1.

`internal/integration/mount_test.go`:

- One-line edit at line 70: `Service: "mounttest",` added to the
  `RunRequest` literal so the file still compiles after Rob ships the
  guard. This is `//go:build integration` so `go test ./...` on the dev
  box does not run it, but `go vet -tags=integration ./...` and `go
  build -tags=integration ./...` do compile it — both pass.

## What the tests assert (behaviour, not byte-equality)

1. **Sentinel-error contract** (§6.2.1–§6.2.4). Empty / slash-containing
   `Service` causes `Run` / `RunWithOptions` to return a non-nil error
   that `errors.Is`-matches the right sentinel and `errors.Is`-DOES-NOT
   match the other sentinel (the two failure modes must be
   distinguishable in caller code).

2. **Guard fires BEFORE `cmd.Run`** (§6.2.1–§6.2.4). Each rejection test
   asserts `assert.Empty(t, records)` — no `docker` process is spawned.
   This is the load-bearing assertion that proves the guard runs ahead
   of the exec. Without it the test would pass even if Rob put the
   guard after the docker invocation (leaving the bad tag in argv for
   one transient call before erroring out).

3. **Tag literal is `decloud/<service>`** (§6.2.5–§6.2.6). The four
   journald tokens (`--log-driver journald --log-opt tag=…`) must
   appear contiguously and in that order. §6.2.5 checks the service
   path with `tag=decloud/foo`; §6.2.6 checks the caddy path with
   `tag=decloud/caddy`. Locks the tag schema other tools (`journalctl
   CONTAINER_TAG=decloud/<service>`) will depend on.

4. **`docker start` does NOT re-emit log flags** (§6.2.7). The
   `HostConfig.LogConfig` is sealed at create time; a future
   "consistency" refactor adding `--log-driver` to `docker start` argv
   would now fail two assertions named with the exact invariant they
   defend.

## Current state — red as expected

`go test ./internal/dockerdrv/`:

- 6 FAIL — the six new behavioural tests (§6.2.1–§6.2.6).
- 1 PASS — `TestCLIDriver_StartArgs` (§6.2.7 extension passes from day
  one; it's a regression-lock, not a behaviour-to-implement test).
- All other existing tests in the package PASS (existing argv asserts
  in `TestCLIDriver_RunArgsWithEnvSorted` etc. don't yet require the
  `Service` field because no guard logic has shipped).

Failure modes are clean — `require.Error(t, err)` reports "An error is
expected but got nil", and the tag-literal tests report `--log-driver
must appear in argv`. No compile errors. No spurious failures from
unrelated packages: `internal/caddy`, `internal/cli`, `internal/config`,
`internal/deploy`, `internal/envcap`, `internal/ids`,
`internal/logging`, `internal/registry` all pass.

`gofmt -l .` — clean. `go vet ./...` — clean. `go vet -tags=integration
./...` — clean.

## Deviations from Joel's spec

NONE in terms of new tests. The §6.2.1–§6.2.7 set is implemented
verbatim per tech plan.

I deliberately did NOT touch the §6.1 / §6.3 / §6.4 / §6.5 existing
test updates (the `Service:` splices into existing fixtures and the
`assert.Equal` argv updates in `TestCLIDriver_RunArgsWithEnvSorted`,
`TestCLIDriver_RunWithOptionsCaddyShape`, `TestDeploy_*`, the
lifecycle and caddy fixtures). Rationale: those updates are part of
the green-step Rob ships when he implements the guard and the argv
splice. Touching them now (before the guard exists) would either:

(a) make the existing assertions inconsistent with the still-unchanged
production code (red on the wrong axis — argv-mismatch instead of
missing-implementation), OR

(b) require pre-emptively shipping the guard, which is Rob's job.

Joel's §12 ("Sequencing for Kent and Rob") agrees: my deliverable is
the §6.2 tests plus the mount_test.go compile-fix; Rob's deliverable
covers the production code AND the §6.1 / §6.3 / §6.4 / §6.5 fixture
updates that flip those tests to the new argv shape.

## Files touched

- `internal/dockerdrv/driver.go` — added `Service` field x2; added two
  sentinel-error declarations.
- `internal/dockerdrv/cli_driver_test.go` — added 6 new tests; extended
  `TestCLIDriver_StartArgs`; added `indexOf` helper; added `Service:
  "caddy"` to `caddyRunOptionsFixture()`.
- `internal/integration/mount_test.go` — added `Service: "mounttest"`
  to the `RunRequest` literal at line 70.

## Hand-off to Rob

Run `go test ./internal/dockerdrv/` to see the six red tests. Implement
the guard + the four-token journald-flag splice in both `Run` and
`RunWithOptions` per tech plan §5.2, then update the §6.1 / §6.3 /
§6.4 / §6.5 existing fixtures to flip those tests to the new shape.
After your change, all tests should be green.
