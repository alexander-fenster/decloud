# Linus — High-Level Review of M1 Execution

**VERDICT: REVISIONS REQUESTED.**

Overall the orchestrator hangs together. Joel's eight flagged items from
plan-v2 all landed. 144 tests, no failures. But three architectural items
keep this from "ship it."

---

## L1. `NetworkEnsure` is implemented, tested, and never called

`internal/dockerdrv/cli_driver.go:159-167` ships the inspect-or-create
logic. The `Driver` interface declares it
(`internal/dockerdrv/driver.go:60`). The gomock stub records it
(`internal/dockerdrv/mocks/mock_driver.go:104-115`). Two unit tests
exercise it (`internal/dockerdrv/cli_driver_test.go:187,203`).

Now grep for callers in the only places that would matter:

```
$ grep -n "NetworkEnsure" internal/deploy/*.go internal/cli/*.go
(no output)
```

Zero. The deployer never self-heals the `decloud` network. Plan-v2 §2.2.1
step 5 and tech-plan-v2 §13.6 explicitly required this. Operator forgets
the install step or runs `docker network rm decloud` while debugging —
deploy fails with an opaque `ExitRunFail (40)` from `docker run --network
decloud …`, which is exactly the failure mode the self-heal was supposed
to eliminate.

**Impact**: Functional regression vs. the approved plan. Dead exported
API in the public driver interface. Two tests that prove a behavior the
product never uses.

**Options**:
- **A (Minimal)**: Call `deps.Driver.NetworkEnsure(ctx, "decloud")` as
  step 0 of `Deploy` before the first container action. Pros: closes the
  plan gap with one line; matches §13.6 verbatim. Cons: none.
- **B (Proper)**: Same as A, plus a `Deploy_EnsuresNetwork` integration
  test that verifies the call ordering via the mock recorder. Pros:
  proves it stays wired. Cons: ~15 lines of test.
- **C (Defer)**: Delete `NetworkEnsure`, the interface method, the mock,
  and the two tests; revisit when needed. Pros: removes dead code. Cons:
  reverts a planned feature; user pain returns.

**My recommendation**: **B**. We agreed to self-heal; ship it with a
test that pins the contract. This is a five-minute fix.

**DON: decide A vs B vs C.**

---

## L2. `Capturer.Capture` unconditionally requires a script

`internal/envcap/capture.go:35-38`:

```go
func (b *bashCapturer) Capture(ctx context.Context, scriptPath string) (map[string]string, error) {
    if _, err := os.Stat(scriptPath); err != nil {
        return nil, fmt.Errorf("env.sh: %w", err)
    }
```

This is the root of Kevlin's B2 finding, but Kevlin diagnosed it at the
wrong layer. The interface contract is broken: docs and plan-v2 say
`env.sh` is **optional**, yet the only production call site
(`internal/deploy/service.go:132`) passes whatever `envFile` it
constructs and the capturer mandates the path exists. Today the CLI
always supplies a path, so the bug is masked — but the day someone wires
a deploy-without-env path (which the docs already promise), this
explodes.

**Impact**: Interface contract diverges from documented contract. The
masking via "CLI always passes a value" is exactly the kind of latent
landmine that bites six months later.

**Options**:
- **A (Minimal)**: Add `if scriptPath == "" { return nil, nil }` at the
  top of `Capture`. Pros: 1 line; honors documented contract. Cons:
  doesn't fix CLI auto-discovery.
- **B (Proper)**: A, plus CLI auto-discovers `<source-dir>/env.sh` and
  passes `""` when absent. Pros: matches plan-v2 promise end-to-end.
  Cons: ~10 lines across CLI + a test.
- **C (Defer)**: Document that `env.sh` is currently required, file an
  issue. Pros: ships now. Cons: doc lies elsewhere; you'd be papering
  over the lie.

**My recommendation**: **B**. The interface contract fix (A) is
load-bearing — the auto-discovery (B) is the user-visible promise. Ten
lines for both.

**DON: decide A vs B vs C.**

---

## L3. `NewHTTPProbeForTest` is dead exported API

`internal/deploy/readiness.go:19-22`:

```go
// NewHTTPProbeForTest constructs the production HTTP probe.
func NewHTTPProbeForTest(driver dockerdrv.Driver) ReadinessProbe {
    return newHTTPProbe(driver)
}
```

Rob added the right seam (`Dependencies.Probe` injection in
`internal/deploy/service.go:67,112`) **and** kept this `_ForTest`
exported wrapper. Six test call sites use the wrapper; nobody else does.
The wrapper exists solely to dodge unexported access from the `_test`
package, which is fine — but only if `Dependencies.Probe` doesn't also
exist. We have both.

**Impact**: Two seams for one job. New contributors will guess wrong.
Exported "ForTest" symbols are the canonical smell of "I couldn't decide
which seam to use."

**Options**:
- **A (Minimal)**: Delete `NewHTTPProbeForTest` and its `newHTTPProbe`
  wrapper. Convert tests to inject via `Dependencies.Probe`, OR move
  tests into package `deploy` (not `deploy_test`) so they can call
  `newHTTPProbe` directly. Pros: one seam. Cons: ~6 test call sites to
  update.
- **B (Proper)**: A, but also audit if any other `_ForTest` exports
  crept in. Pros: thoroughness. Cons: time.
- **C (Defer)**: Leave it. Pros: zero work. Cons: the smell persists; we
  pay interest forever.

**My recommendation**: **A**. Pick the `Dependencies.Probe` seam (the
correct one) and delete the other.

**DON: decide A vs B vs C.**

---

## Missing artifact

`_ai/decisions/m1-test-strategy.md` — plan-v2 §5 required this
deliverable. Current `_ai/decisions/` contains `m1-scope.md`,
`schema-versioning.md`, `secrets-split.md`. No test-strategy doc.
Raymond either missed it or chose not to write it; either way, it's a
plan deviation.

**DON: decide whether Raymond writes it now or it gets dropped.**

---

## What landed correctly (credit where due)

- Orchestrator sequencing in `Deploy` (validate → render → containers →
  envcap → readiness → swap → reload).
- Validate-before-rename ordering for compose generation.
- Host-side IP probe via `docker inspect` rather than in-container curl.
- Single-struct lifecycle (no scattered state machines).
- `regenerateAndReload` factoring.
- Atomic writes throughout the materialization paths.
- Bash-3.2-compatible envcap (`compgen -e` + null-delimited stream).
- All eight items Joel flagged in plan-v2 are present.
- 144 tests, zero failures.

This is good work. Fix the three above and we ship M1.
