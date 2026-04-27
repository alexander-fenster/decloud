# Linus's review of the fix-three-findings plan

**Verdict: APPROVED. Ship it.**

I spot-checked every cited file:line. Don's facts hold. Joel's facts hold.
The plan is small, the layering is right, and nobody got cute. I went
looking for the usual sins — over-engineering, hidden globals, premature
abstractions, tests that test the mock — and they're not here.

I'm going to be specific about what's right, then list the few minor
things I want you to keep in mind. Then approval.

---

## Finding 1 — `--dockerfile` resolution: CORRECT LAYER

You asked me whether the resolution belongs in CLI or `dockerdrv`. The
answer is unambiguously CLI, and Don picked correctly.

The contract — "relative to `<source-dir>`" — is **a CLI-shaped concept**.
"Source dir" is something the CLI invents from a positional argument and
`filepath.Abs`. `dockerdrv` doesn't know what a "source dir" is; it knows
about a build context and a `-f` argument. The driver is a faithful shell
around `docker build`. Pushing the join down would couple a generic Docker
driver to a CLI convention. That is a one-way ratchet toward spaghetti and
I would have rejected that choice.

The six-line implementation is the smallest correct fix:

```
dockerfile := f.Dockerfile
if dockerfile == "" {
    dockerfile = "Dockerfile"
}
if !filepath.IsAbs(dockerfile) {
    dockerfile = filepath.Join(abs, dockerfile)
}
```

Joel's defense of the empty-string guard (G8 / R2) is correct and
load-bearing. `filepath.Join(abs, "")` returns `abs` — a directory — and
`docker build -f <dir>` produces a confusing error. Keep the guard.

### Edge cases you asked about

- **`..` paths** — `filepath.Join` cleans them. The result may escape the
  build context; Docker rejects with "Forbidden path outside the build
  context"; we surface as `ErrBuild` (exit 30). Correct error class,
  correct exit code, no pre-validation needed. Don explicitly punted this
  in §"Edge cases I'm explicitly NOT addressing" and he's right.
- **Symlinks** — Docker's problem, not ours. A symlinked Dockerfile inside
  the context resolves to its target and Docker tars the target. Outside
  the context, Docker rejects. Same outcome as above.
- **`--dockerfile=""`** — Joel handled this with the empty-string guard.
  Without it, `filepath.Join(abs, "")` => `abs` and Docker errors with
  "is a directory". With it, behavior is identical to omitting the flag.
  Defensive but tiny; cost of the check is one comparison.
- **`--dockerfile=/abs/path/inside/context`** — preserved as-is. Docker's
  `-f` accepts absolute paths and validates context membership itself.
  That's the right division of labor.

### Contract cleanliness

Yes, the contract is clean: CLI promises "relative paths resolve under
`<source-dir>`, absolute paths pass through". `dockerdrv` promises
"whatever `-f` you gave me, that's what `docker build -f` sees". Each
layer's contract is documentable in one sentence. That's the bar.

### One thing I want noted (not blocking)

`req.Dockerfile` stored in the registry will become an absolute path
post-fix. Joel correctly verified no other code path re-reads this value
(`grep -rn "Build\.Dockerfile"` returns only the deploy producer + the
write site). An absolute path is **better provenance** — operators
debugging "which Dockerfile got used?" will thank you. But if anyone
later writes a "rebuild from registry" feature that reads
`Build.Dockerfile` and tries to re-shell-out to `docker build`, they get
a working absolute path for free. Good outcome of doing it right.

---

## Finding 2 — `logging.Init(root)`: CORRECT MECHANISM

You asked whether to change the signature, use a setter, or use a global.
Don picked the signature change. He's right and I'll defend it.

### Why signature is right

A function that **takes its inputs explicitly** is debuggable in five
seconds. A function that reads a global, a setter, or Viper is
debuggable in five minutes when someone discovers the setter was called
from a unit test in a different package and never reset.

The setter pattern (`logging.SetRoot(...)` then `logging.Init()`) creates
exactly the kind of two-call ordering bug that produced the original
finding: somebody adds a config knob, somebody else "knows" it doesn't
apply to logging, and a year later you're answering pages about logs in
the wrong directory. The fix Don picked makes the contract structural,
not procedural. Wrong call site can't compile.

A Viper read inside `logging.Init` would be even worse — now the package
depends on a viper instance being initialized, which it currently isn't,
and we'd be inventing infrastructure to fix a one-line bug. Hard pass.

The empty-string-fallback-to-`config.DefaultRoot` is the right policy
because it mirrors `config.NewPaths` (`paths.go:24-27`). Two functions
with one fallback rule. If we ever change "what no root means", we
change it in `config.DefaultRoot` and both functions follow. That's how
"one source of truth" actually works — not by inventing a third
mechanism.

### Backwards-compat concerns

There is exactly **one** production caller of `logging.Init` in the
whole tree (`internal/cli/root.go:23`). Everything else is tests.
Joel's grep is correct; I re-ran it and got the same result.
There's no public API contract to worry about — `internal/logging` is,
by Go convention, internal to this module. Anybody importing it from
outside this repo is doing something they were never invited to do.

The signature break is a single-character change for the production
caller and a single-line change per test. This is the cheapest possible
breaking change.

### One thing to watch

Joel's G7 nails a subtle point: **the env-fallback-to-default is only
correct because Cobra fills `--config-root` with `config.RootFromEnv()`
as its default value at flag-declaration time.** If somebody later
"refactors" `root.go:26` to lazy-resolve the default (e.g., compute it
inside `PersistentPreRunE`), the env-vs-flag precedence has to be
preserved by whatever new code goes in. This isn't a problem with the
plan — it's a property of the existing design that the plan correctly
relies on. Worth a sentence in the test we're adding so a future
maintainer understands what the test is locking in. Kent: consider a
one-line comment in `TestRoot_ConfigRootFlagControlsLogPlacement` that
says "this test relies on Cobra's flag-default-from-env mechanism in
root.go:26".

---

## Finding 3 — Reject port=0: CORRECT POLICY, NOT A CORNER

You asked whether rejecting at validation paints us into a corner for M5
workers. **No.** Here's why I checked the M5 plan before signing off.

`README.md:215` documents `decloud deploy job` as a **separate
subcommand**. `_ai/decisions/m1-scope.md:32` confirms the milestone
ordering: M5 = jobs via systemd timers, where the lifecycle is
"container starts, runs, exits". Jobs in decloud's design do **not**
have HTTP listeners. They are batch processes. They have no readiness
probe to skip in the first place — they don't need one because their
readiness signal is "the container exited 0".

So the M5 worker isn't a port-less variant of `deploy service`. It's a
different command (`deploy job`) with a different lifecycle, a different
config, a different validation set. Treating "port=0 means worker" as a
mode of `deploy service` would be **exactly the wrong shape** — it would
fold two unrelated workloads into one command and force the deployer to
branch internally on `port == 0` to decide whether to skip readiness,
register routes, write the Caddy config, etc. That's the kind of "magic
zero" mode that produces 200-line if-else trees three milestones from now.

Don's reasoning in §Finding 3 is correct and his rejection of options B
("skip readiness") and C ("both") is correct. Option B in particular is
the dangerous one — a container that immediately exits 0 would record a
"successful deploy". Lying to operators about deploy success is the
worst possible outcome and would get us paged in production within a
week.

The "explicit `--port=0` is also rejected" test (Test
`TestDeployService_PortZeroExplicitReturnsExitUsageError`) is good
defensive engineering. It locks in the policy against a future
maintainer who "knows" port=0 should mean something special.

### One thing to sanity-check

The error message Don specified —

```
"--port is required (services must expose an HTTP port for the readiness probe): %w"
```

— is good but I'd actually drop the parenthetical. The user already
knows they're running `deploy service`. The reason "services must expose
an HTTP port" is documented in `_docs/usage.md`, not in a CLI error.
Short error: `"--port is required: %w"` wrapping `errUsage`. Same exit
code, less noise. **Don's call. Not blocking.**

If you keep the parenthetical, fine — it's correct and not harmful.
This is bikeshed territory; ship whichever Don prefers.

---

## Test plan: SUFFICIENT, NOT REDUNDANT

I scrutinized the test list. Specifically:

### Finding 1 tests — correct count and shape

Four tests, each catching a distinct failure mode:

1. Default flag value — most common invocation.
2. Explicit relative path with subdirectory — catches a `filepath.Base`
   regression.
3. Absolute passthrough — catches an "always-join" regression.
4. **Cwd-relative source dir, run from a different cwd** — this is the
   regression test for the actual bug. Without it the diff is correct
   but unguarded against the original reviewer-reported failure mode.

I considered whether Test 4 alone subsumes Tests 1-3. It doesn't —
Test 4 happens to also exercise the default flag and a relative source,
but if it ever needs to be skipped (e.g., on a CI runner with a weird
cwd policy), Tests 1-3 give independent coverage. The four-test split
is correct.

I considered adding a `..`-segment test. Don explicitly rejected this
and Joel echoed the rejection (G5). They're right — that's an
integration test against the real `docker build` and produces a known
exit-30 failure. Unit-testing it would test `filepath.Join`'s
behavior, which is Go's responsibility, not ours.

### Finding 2 tests — correct count, one redundancy worth keeping

Two new logging tests + one CLI integration test + four existing
signature updates.

- `TestInit_UsesPassedRootNotEnv` — locks in the env-is-ignored
  contract. Belt-and-suspenders negative assertion (`/path/that/must/
  not/be/written/to` must not exist). Correct.
- `TestInit_EmptyStringRootFallsBackToDefault` — Joel correctly notes
  this is a weak test (it only proves Init("") doesn't panic). That's
  fine. The full DefaultRoot path can't be exercised in a unit test
  without writing to `/opt/decloud`, which is a non-starter.
- `TestRoot_ConfigRootFlagControlsLogPlacement` — the end-to-end
  regression test. This is the one that would have caught the original
  bug.

The four existing-test signature updates are mechanical. Joel's G10
self-correction (drop `t.Setenv("DECLOUD_ROOT")` from the three
filesystem tests, keep the dedicated env-precedence test as the single
guardian of that contract) is the right call. One test owns one
assertion.

### Finding 3 tests — correct, including the silent-regression catch

Two new tests + four fixture updates. The most important catch in the
whole plan is Joel's R1 finding: **`TestDeployService_ExplicitEnvFile
MissingReturnsExitConfigError` would silently change which error code
it asserts** if Kent forgets to add `--port 8080`. The test would still
pass — wrong reasons. That's the worst kind of test rot.

Joel caught it. Kent: do not skip this fixture update.

### What's missing

Nothing significant. I considered:

- A "Dockerfile resolution unit test that doesn't go through Cobra" —
  not worth it. The Cobra-driven tests exercise the exact production
  path. Adding a separate unit test for a six-line code block would be
  ceremony.
- A "logging.Init with `DECLOUD_LOG_TO_STDERR_ONLY` and explicit root"
  test — already implicitly covered by the short-circuit test, which
  proves the env var dominates regardless of root.
- An integration test for the dockerfile fix that actually invokes
  `docker build` — out of scope for unit tests, would require Docker
  in CI. The unit-level "request shape" assertion is the right level.

---

## Anything else worth doing while you're touching these files

I considered the "since we're here" temptations. Reject all of them:

- **Refactor `runDeployService` validation into a helper** — three
  checks, one function, fine where it is. Joel's S5 already addressed
  this. No.
- **Add a `--port` Cobra-required marker** — Cobra can't distinguish
  unset from `--port=0`. The explicit RunE check is more robust. Joel's
  S1. No.
- **Hermetic-ize the existing logging tests with
  `DECLOUD_LOG_TO_STDERR_ONLY=1`** — out of scope, would add four
  unrelated test changes. Existing tests already write to `t.TempDir()`,
  which is OS-cleaned. No.
- **Update `_ai/MEMORY.md` to record the contract decisions** — Ward's
  job at FINALIZATION, not Raymond's at EXECUTION. The plan correctly
  does not touch `_ai/`.
- **Add a `Build.Dockerfile` round-trip test in `registry/store_test`
  with the new absolute-path shape** — existing round-trip is
  shape-agnostic. Adding a path-shape assertion would be a
  change-detector test, which CLAUDE.md explicitly bans.

The plan resists scope creep. That's harder than it looks and I
appreciate it.

---

## Procedural notes for execution

- Joel correctly notes that **fixture-update tests will pass before
  Rob's diff** (forward-compatible). The "red bar" comes from the six
  new tests + four logging-test compile failures. Kent's report must
  state exactly which tests fail for what reason — the table in §6 is
  the spec.
- Order of operations is right: Kent first (must demonstrate red),
  Rob second, Raymond third, Kevlin/Linus fourth.
- The cwd-mutating test (Test 4 of Finding 1) requires
  `t.Cleanup(os.Chdir(origCwd))`. Joel's G2 covers this. Kent: be
  paranoid about this — a leaked cwd will make subsequent tests in
  the same package non-deterministic.

---

## Approval

The plan is approved. **Move to EXECUTION.**

- Don's strategic decisions are right at every layer.
- Joel's tactical expansion is correct and catches one silent-regression
  case (R1) that Don's plan would have missed.
- The test plan is sufficient and not redundant.
- Nothing is being deferred that should be done now.
- The fix is small (eleven production lines net), in the right places,
  and resists the temptations to over-engineer or extend public surface.

Three findings, three fixes, three layers. Each fix where its contract
lives. No new abstractions, no new flags, no new files. This is what
small correct work looks like.

Kent and Rob: execute the plan as written. Don't deviate without
flagging it back to PLAN.

— Linus
