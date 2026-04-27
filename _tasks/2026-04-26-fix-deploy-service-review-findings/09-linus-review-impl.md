# Linus's high-level architectural review of the implementation

**Verdict: REQUEST CHANGES.**

One concrete defect — the same one Kevlin already flagged. It cannot ship
in this state without contradicting the very contract this task is
fixing. Everything else is right.

I'm signing off on the architecture. I'm not signing off on the diff
until Issue 1 is fixed.

---

## Build status

- `go test ./... -count=1`: green across the entire tree.
- `go vet ./...`: clean.
- `gofmt -l internal cmd`: empty.
- Diff scope matches the plan: three production files, three test files,
  three doc files. Nothing else touched. `internal/dockerdrv/`,
  `internal/deploy/`, `internal/registry/`, `cmd/decloud/` all untouched.

---

## What was actually done vs. the approved plan

I diff'd the working tree against the plan (`02-plan.md` + `03-tech-plan.md`).
Three production diffs landed exactly where promised:

- `internal/cli/deploy_service.go:73-86,95` — Finding 3 port check (now
  unconditional `f.Port == 0`); Finding 1 dockerfile-resolve block (six
  lines: empty-string guard → `IsAbs` short-circuit → `Join(abs, ...)`);
  request-field assignment uses the resolved `dockerfile` variable.
- `internal/cli/root.go:23` — `logging.Init(rc.ConfigRoot)`.
- `internal/logging/logging.go:14-29` — signature `Init(root string) error`;
  empty-string falls back to `config.DefaultRoot`; doc comment updated.

Eleven net production lines, three layers. The plan promised this
exactly and the diff delivers it exactly. No scope creep. No surprise
packages touched. No "since we're here" temptations indulged.

The error message Don/Joel originally specified (with the parenthetical
"services must expose an HTTP port for the readiness probe") was
simplified to my suggested shorter form `"--port is required"`. Rob
flagged this as "Don's call per the task instructions" and went short.
Fine. The exit code and `errUsage` wrap are unchanged.

The only test-file change beyond what Kent shipped is Rob's macOS
portability fix (`filepath.EvalSymlinks(t.TempDir())` in the
cwd-relative test). The justification — `/var` → `/private/var`
symlink resolution on macOS — is correct, contained, and only affects
the test's expected-path computation. The production code does not
change shape based on platform; the test now compares apples to apples.
This is the right call.

---

## Architectural seams: layering held

I went looking for the usual sins one more time and didn't find them.

### The CLI is the only place that knows about "source dir relative"

`internal/dockerdrv/cli_driver.go` is unchanged. It still takes whatever
`-f` value it's given and shells out. The driver's contract is the same
sentence it was before this task: "I shell out `docker build`; you give
me a tag, a `-f` value, and a context dir." That is exactly what a
generic Docker driver should be. Pushing the resolution down would have
been a one-way ratchet toward spaghetti and we did not take it.

### Logging takes its inputs explicitly

The signature change `Init() error` → `Init(root string) error` is
structurally correct — debugging "why is the log file in a weird
place?" is now a one-call grep (`logging.Init(...)` in `root.go:23`).
Before the change, the answer required you to know about
`config.RootFromEnv()` reading `DECLOUD_ROOT` deep inside an
unrelated package. After the change, the input is a function argument.
Wrong call site can't compile. That is what "fix the contract
structurally, not procedurally" looks like.

The empty-string fallback to `config.DefaultRoot` mirrors
`config.NewPaths` (`internal/config/paths.go:24-27`). One source of
truth (`config.DefaultRoot`); two consumers; identical fallback rule.
If "what no root means" ever changes, it changes in `config.DefaultRoot`
and both callers follow. That is how "single source of truth" actually
works — not by inventing a third mechanism.

### The deploy/probe layer is correct given the contract

The deployer (`internal/deploy/service.go:213`) still calls
`d.probe.Wait(..., req.Port)` unconditionally. The probe
(`internal/deploy/readiness.go:55`) still builds
`http://<ip>:<port><path>` unconditionally. Neither got a `port == 0`
special case. The contract that "every service has an HTTP port" is
enforced at the CLI boundary, where it belongs, and the deeper layers
get to stay simple. If someone later writes `decloud deploy job` for
M5 workers, that command will have its own command, its own validation,
its own deployer logic — not a magic-zero mode of `deploy service`. The
right shape.

### `req.Dockerfile` in the registry is now an absolute path

Joel verified no other code re-reads this field, and the persisted value
becoming absolute is **better provenance** for operators
debugging "which Dockerfile got used?". If anyone later writes a
"rebuild from registry" feature, they get a working absolute path for
free. Doing the right thing pays dividends. Acknowledged.

---

## Tests interrogate the right behavior

Kent's six new tests, two new logging tests, and one new CLI integration
test all assert on production behavior the code is responsible for, not
on internal structure. I spot-checked the most important ones:

- `TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved`
  — the regression test for the original bug. `os.Chdir`s to a parent,
  invokes with `./svc`, asserts the resolved Dockerfile is absolute and
  ends in the right place. Cleanup uses `t.Cleanup` with best-effort
  restore. Without this test, the diff would be technically correct
  but unguarded against the reviewer's reported failure mode. This is
  the test that earns its keep.
- `TestDeployService_PortZeroExplicitReturnsExitUsageError` — locks in
  the "no magic zero" policy against a future maintainer who "knows"
  port=0 should mean something special. Belt-and-suspenders, low cost,
  high payoff.
- `TestInit_UsesPassedRootNotEnv` — positive (`os.Stat(passedRoot/...)`)
  AND negative (`os.IsNotExist("/path/that/must/not/be/written/to")`).
  Two assertions in one test; both lock in the env-is-ignored contract.
- `TestRoot_ConfigRootFlagControlsLogPlacement` — the end-to-end
  regression test. Sets env to one TempDir, flag to another, asserts
  the log appears under the flag dir and NOT under the env dir. This
  is the test that would have caught the original Finding 2 bug. The
  one-line comment about Cobra's flag-default-from-env mechanism (per
  my earlier note in `04-linus-review.md`) is present.

No change-detector tests. No tests-that-test-the-mock. No fixture
updates that quietly demoted an assertion. Joel's R1 catch — that
`TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError` would
silently rot from "asserts envcap error" to "asserts port-missing
error" without `--port 8080` — was honored. Good test discipline all
the way through.

---

## Issue 1 — Stale flag help text (BLOCKING)

Kevlin caught this in `08-kevlin-review.md` §"Issue 1". I confirm:

`internal/cli/deploy_service.go:55`:

```go
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required if --host set)")
```

After Finding 3, `--port` is unconditionally required. `_docs/usage.md`
already says `Required: yes` with the new error wording. The CLI's own
`--help` output still tells operators "(required if --host set)".

This is **exactly the CLI/docs drift this task was meant to fix.** We
fixed Finding 1 (CLI vs docs disagreement on Dockerfile resolution) and
Finding 2 (CLI vs docs disagreement on log placement) — and we shipped
a fresh CLI vs docs disagreement on `--port` semantics in the same diff.
That cannot ship.

The fix is one line:

```go
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required)")
```

Or even shorter — `"container listen port"` with no parenthetical, since
the help line is no longer about a conditional contract. Either is fine.
Don's call.

**Don's decision required:** pick the new help string and have Rob
apply the one-line fix; re-run `go test ./...`; ship.

This is non-negotiable because it's the same class of bug the task is
fixing. Shipping with this in place would mean we partially fixed
"docs lie about flag contract" while introducing "help lies about flag
contract" — the same defect with a different surface.

---

## Issue 2 — `TestDeployService_HostWithoutPortReturnsExitUsageError`
mild misnomer (NON-BLOCKING)

After the diff, `--host foo` without `--port` fails for the generic
`--port is required` reason rather than the specific
`--port is required when --host is set` reason. The test still passes
(both paths return `errUsage` → `ExitUsageError`), so no silent
regression. The test name is now slightly misleading but the assertion
it makes (`ExitCodeFor(err) == ExitUsageError`) is still correct under
the new semantics.

Joel called this out in his plan §"Tests Kent must write/update";
Kevlin re-flagged it. Both treat it as non-blocking. I agree.

**Options for Don:**
- Option A (rename): rename the test to
  `TestDeployService_HostSetButPortMissingReturnsExitUsageError` or
  similar. Pros: name matches what's being asserted. Cons: cosmetic
  churn for no behavioral gain.
- Option B (leave): the test still asserts a real-world misuse case
  (operator passes a host but forgets the port). The fact that the
  underlying check is now generic doesn't invalidate the scenario.
  Pros: zero churn. Cons: name implies a more specific check than
  what fires.
- Option C (delete): the test is now subsumed by
  `TestDeployService_NoPortReturnsExitUsageError`. Pros: remove
  redundancy. Cons: loses the specific "host set but port missing"
  scenario; future maintainer who refactors validation might
  inadvertently drop this user error path without noticing.

**My take:** Option B. The scenario is real. The name is mildly
inaccurate but harmless. Renaming costs reviewer cycles; deleting
loses a real-world test case. Leave it.

**Don's decision required:** pick A/B/C — defer is fine.

---

## Issue 3 — `_ai/cobra-init-pattern.md` doc has pseudo-Go in a warning
(NON-BLOCKING)

Kevlin flagged this in `08-kevlin-review.md` §"Issue 3". The text
"Do not collapse to `Init(string) {}`" uses pseudo-Go that wouldn't
compile. The intent is "don't drop the `error` return". Trivial doc
nit. Raymond can fix if he's touching the file again; otherwise leave.

**My take:** ignore. Fix when convenient. Not blocking.

---

## What didn't need doing — and the team correctly didn't do it

I checked for the temptations one more time. None of these were taken,
and that is the right answer:

- No refactor of `runDeployService` validation into a helper. Three
  checks, one function, fine where it is.
- No `cmd.MarkFlagRequired("port")`. Cobra can't distinguish unset
  from `--port=0`; the explicit RunE check is more robust.
- No env-fallback inside `logging.Init`. Would defeat the
  explicit-input contract; would re-introduce exactly the kind of
  hidden-input bug we just fixed.
- No `port == 0` short-circuit in `internal/deploy/service.go` or
  `internal/deploy/readiness.go`. Policy stays at the CLI boundary;
  deeper layers stay simple.
- No `Build.Dockerfile` round-trip test in
  `internal/registry/store_test.go` with the new path shape. Existing
  round-trip is shape-agnostic; adding a path-shape assertion would
  be a change-detector test, which CLAUDE.md explicitly bans.
- No `_ai/MEMORY.md` update for the contract decisions. That's
  Ward's job at FINALIZATION; correctly not Raymond's at EXECUTION.

The team resisted scope creep. That is harder than it looks and I
appreciate it.

---

## Technical debt (none introduced; one piece notably absent)

This task did not introduce any technical debt. It in fact retired
two pieces of debt:

1. The hidden-input pattern in `logging.Init` (env read inside the
   function) is gone. New code added later cannot accidentally re-introduce
   the bug because the input is a function argument now, not a
   global read.
2. The CLI/docs drift on `--dockerfile` semantics is gone — the doc
   now matches what the code does, with regression tests on both
   sides locking the contract in.

The one piece of debt that survives in this task — and **must** be
retired before ship — is the stale `--port` flag help text (Issue 1).
It is debt we just created. Retire it before commit.

---

## Procedural gate

The plan said "if anything in `go test ./...` ever regresses on
`internal/cli` or `internal/logging`, suspect: the `--port = 0`
validation being moved or weakened; the `filepath.IsAbs`/`filepath.Join`
ordering being reversed; `logging.Init` reintroducing
`config.RootFromEnv()` instead of taking the argument."

None of those regressed. The new tests are positioned exactly to catch
those three regression classes. Future-proofing: solid.

---

## Recommendation to Don

**Block the commit until Issue 1 is fixed.** The fix is one line in
`internal/cli/deploy_service.go:55`. Have Rob change the flag help to
`"container listen port (required)"` (or shorter — your call), re-run
`go test ./...`, and we ship.

After that one-line fix lands:

- All three findings are properly closed. CLI, runtime, docs, AI
  notes — all coherent.
- Eleven net production lines plus one help-text-fix. Three layers.
  Three findings. No new abstractions, no new flags, no new files.
- Tests lock in the contracts at every boundary that matters.

This is what a small correct fix looks like, with one CLI/docs
loose-thread that snuck in. Tuck the thread; ship.

---

## Verdict

**REQUEST CHANGES.** One blocking issue (Kevlin's Issue 1, which I
confirm independently). Two non-blocking observations recorded for
Don's decision.

The architecture, layering, scope discipline, test design, and doc
coherence are right. The only thing standing between this task and
"DONE" is the stale `--port` flag help text.

Fix that, then we're done.

— Linus
