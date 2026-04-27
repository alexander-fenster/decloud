# Don's plan: fix three review findings on M1 deploy service

Reviewer raised three defects on the M1 deploy service. All three are real. All
three have one correct fix each. No new abstractions. No new flags.

## Codebase facts I verified before writing this

I traced every claim in the review. Citing file:line so Linus does not have to.

- `internal/cli/deploy_service.go:61` declares `--dockerfile` default `"Dockerfile"`,
  flag help says `"Dockerfile path relative to <source-dir>"`.
- `internal/cli/deploy_service.go:76-95` resolves `sourceDir` to an absolute
  path with `filepath.Abs`, but copies `f.Dockerfile` straight into
  `deploy.Request.Dockerfile` with no join.
- `internal/deploy/service.go:160-169` passes `req.Dockerfile` straight into
  `dockerdrv.BuildRequest.Dockerfile`.
- `internal/dockerdrv/cli_driver.go:34-43` shells out:
  `docker build -t <ref> -f <Dockerfile> <abs source dir>`. `docker build -f`
  resolves a relative path relative to the **caller's cwd**, not the build
  context (Docker docs and verified against `docker build` behavior — `-f`
  paths are caller-cwd relative).
- `_docs/usage.md:64` documents the contract as
  `--dockerfile ... default Dockerfile ... Path to the Dockerfile, relative to <source-dir>.`
  So the docs and the flag help match each other. The implementation is
  the thing that drifted.
- `internal/cli/root.go:22-27` calls `logging.Init()` from `PersistentPreRunE`,
  with no argument. `rc.ConfigRoot` is populated by Cobra **before**
  `PersistentPreRunE` runs (Cobra parses persistent flags during
  `Execute`/`ExecuteContext`'s pre-traversal — verified by
  `internal/cli/root_test.go:46-53` which exercises that exact ordering).
- `internal/logging/logging.go:21-43` derives the log dir from
  `config.RootFromEnv()`. Short-circuits on `DECLOUD_LOG_TO_STDERR_ONLY=1`.
  Falls back to stderr-only on any filesystem error and prints a warning.
- `internal/cli/deploy_service.go:73` validation: rejects `--host` set
  without `--port`. Does **not** reject `--port=0 && len(--host)==0`.
- `internal/deploy/service.go:213` always calls `d.probe.Wait(..., req.Port)`
  unconditionally.
- `internal/deploy/readiness.go:55` builds `http://<ip>:<port><path>`
  unconditionally — port=0 produces `http://<ip>:0/healthz`, which fails.
- `internal/cli/exit_codes.go:35-46`: `errUsage` -> exit 2;
  registry/envcap config errors -> exit 10. The host-without-port rejection
  uses `errUsage` (exit 2). I will preserve that for the new check too.

## Finding 1 — `--dockerfile` is resolved against the wrong directory (HIGH)

### Decision

**Fix in the CLI layer (`runDeployService`). Do not touch `dockerdrv`.**

The user-facing contract — "relative to `<source-dir>`" — lives in the CLI and
in `_docs/usage.md`. `dockerdrv` is correctly a thin shell around `docker build`;
it should keep accepting "whatever path you want passed to `-f`". Pushing the
join down into `dockerdrv` would couple a generic Docker driver to a
CLI-layer convention. We don't do that.

### Behavior to preserve

1. Default `--dockerfile=Dockerfile` (relative) keeps working — joined to the
   resolved absolute source dir.
2. Explicit relative paths like `--dockerfile=docker/prod.Dockerfile` work and
   resolve under `<source-dir>`.
3. Explicit **absolute** paths like `--dockerfile=/etc/decloud/shared.Dockerfile`
   are passed through unchanged. We don't second-guess absolute paths — the
   operator typed them on purpose.
4. The path is **not** required to live inside `<source-dir>` after the
   join — Docker itself enforces that and returns a clear error if the file
   is outside the build context. We don't pre-validate. (Docker may symlink-
   reach outside; not our problem.)
5. `req.Dockerfile` stored in the registry is the **resolved** path (absolute
   if absolute, otherwise the joined absolute path). This is what we built
   from; that's what we record.

### Implementation

`internal/cli/deploy_service.go`, in `runDeployService`, after the
`filepath.Abs(sourceDir)` line:

```
dockerfile := f.Dockerfile
if dockerfile == "" {
    dockerfile = "Dockerfile"
}
if !filepath.IsAbs(dockerfile) {
    dockerfile = filepath.Join(abs, dockerfile)
}
```

Then assign `Dockerfile: dockerfile` into the `deploy.Request`.

That's the entire production change for Finding 1. No change to
`internal/deploy/service.go`. No change to `internal/dockerdrv/cli_driver.go`.

### Tests Kent must write (in `internal/cli/deploy_service_test.go`)

All using the existing `installMockDeployer` harness, capturing the
`deploy.Request` Cobra hands to the deployer:

- **`TestDeployService_DefaultDockerfileIsJoinedWithSourceDir`** —
  No `--dockerfile`. Source dir is a `t.TempDir()`. Expect
  `req.Dockerfile == filepath.Join(<absSourceDir>, "Dockerfile")`.
- **`TestDeployService_RelativeDockerfileIsJoinedWithSourceDir`** —
  `--dockerfile=docker/prod.Dockerfile`. Expect
  `req.Dockerfile == filepath.Join(<absSourceDir>, "docker/prod.Dockerfile")`.
- **`TestDeployService_AbsoluteDockerfileIsPreserved`** —
  `--dockerfile=/etc/shared/X.Dockerfile`. Expect
  `req.Dockerfile == "/etc/shared/X.Dockerfile"`.
- **`TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved`** —
  Pass a relative source dir like `./svc`. Expect `req.Dockerfile` is an
  absolute path (i.e. starts with `/`) and ends with `svc/Dockerfile` —
  proves the join is against the **resolved abs** source dir, not the raw
  argument. This is the regression that motivated the finding.

The existing `TestCLIDriver_BuildArgs` in `internal/dockerdrv/cli_driver_test.go`
must keep passing unchanged — `dockerdrv` semantics did not change.

## Finding 2 — `--config-root` does not affect log placement (MEDIUM)

### Decision

**Pass `rc.ConfigRoot` into `logging.Init`.** Change `Init`'s signature to
accept the resolved root explicitly. No new globals. No extra env-var read.

This is the only place `logging.Init` is called in the production tree, so
the signature break is contained.

### Behavior to preserve

1. `DECLOUD_LOG_TO_STDERR_ONLY=1` short-circuit still happens **before** any
   filesystem access. (The escape hatch is exactly what tests rely on.)
2. Empty-string root passed in => default to `config.DefaultRoot` (mirrors
   `config.NewPaths`'s contract — there is exactly one place that decides what
   "no root" means).
3. Filesystem failure (mkdir or open) still falls back to stderr-only and
   emits the existing warning to stderr.
4. The single warning line on fallback keeps its existing wording — Linus
   already approved that string.
5. `--config-root /tmp/X` => logs go to `/tmp/X/logs/decloud.log`, period.
   `DECLOUD_ROOT=/tmp/Y` with no flag => logs go to `/tmp/Y/logs/decloud.log`.
   Flag wins over env, because `rc.ConfigRoot` was already initialized from
   `config.RootFromEnv()` as the flag's default.

### Implementation

1. `internal/logging/logging.go`: change to `func Init(root string) error`.
   Replace `root := config.RootFromEnv()` with:
   ```
   if root == "" {
       root = config.DefaultRoot
   }
   ```
2. `internal/cli/root.go`: in `PersistentPreRunE`, call
   `logging.Init(rc.ConfigRoot)` instead of `logging.Init()`.
3. `cmd/decloud/main.go`: nothing. `main` just calls `NewRootCmd().Execute()`;
   it does not touch logging directly. (Verified.)

### Tests Kent must update / write (in `internal/logging/logging_test.go`)

The existing tests pass the root via `t.Setenv("DECLOUD_ROOT", root)` and call
`logging.Init()` with no arg. After the signature change those tests must call
`logging.Init(root)` directly. Required new behavior:

- **`TestInit_UsesPassedRootNotEnv`** — set `DECLOUD_ROOT=/from/env` (a path
  that does not exist), pass `t.TempDir()` as the root argument. Verify the
  log file appears under the **passed** root, not under the env path.
  Locks in that the env var is no longer consulted by `Init`.
- **`TestInit_EmptyStringRootFallsBackToDefault`** — call `logging.Init("")`
  with `DECLOUD_LOG_TO_STDERR_ONLY=1`. Must not panic, must succeed. (We
  cannot assert writes to `/opt/declouding` from a unit test; the
  short-circuit is enough to prove the empty-string branch is reached
  without crashing.)
- Update the existing four tests to pass `root` as an argument.

Also a CLI-level regression test in `internal/cli/root_test.go`:

- **`TestRoot_ConfigRootFlagControlsLogPlacement`** — set
  `t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")`, set `DECLOUD_ROOT` to a
  TempDir A, pass `--config-root B` (a different TempDir), run any cheap
  subcommand. Assert `B/logs/decloud.log` exists and `A/logs/decloud.log`
  does not. This is the test that would have caught the original bug.

## Finding 3 — Deploy without `--port` and without `--host` passes validation (MEDIUM)

### Decision

**Reject up front in `runDeployService`. Pick option A: validation.
Do not skip readiness.**

I considered three options:

| Option | What it does | Why I rejected it |
|---|---|---|
| A. Reject port=0 in validation | Hard fail with `errUsage` (exit 2) before anything runs | Picked. Fast feedback, clear error message, no surprise late failures. |
| B. Skip readiness when port=0 | Deploy proceeds without any health check | Hides container crashes — a container that exits 0 immediately would record a "successful" deploy. Lying to operators about success is the worst possible outcome. |
| C. Both | Validation + skip-readiness as belt-and-suspenders | Two policies for one decision. Pick one. |

**Option A is correct because in M1 every service is an HTTP service.**
There is no concept of a worker/cron without an HTTP listener — that's M5
(jobs). A service with no port is a service that nobody — not Caddy, not
the readiness probe, not the operator — can talk to. That is misuse, not a
valid configuration. Reject it.

When M5 introduces workers, the right answer is a **separate command**
(`decloud deploy job`) or a **separate `--kind=worker` flag** with its own
validation. Not a magic `--port=0 means don't probe` mode. Don't build
hooks for hypothetical future shapes.

### Behavior to preserve

1. Existing rejection of `--host` without `--port` (`errUsage` -> exit 2)
   is unchanged in spirit — the new rule is a strict superset.
2. The new rule is: **`--port` is required, period.** Cobra cannot
   express "required if not zero", so we keep the validation in the
   RunE function, mirroring the existing host/port check.
3. Error message must be specific:
   `"--port is required (services must expose an HTTP port for the readiness probe)"`,
   wrapped with `errUsage`. Exit code 2.
4. Cobra's own `MarkFlagRequired` is **not** the right tool here —
   `--port` is an `int` with a default of `0`, and Cobra's required-flag
   detection only fires on truly unset flags. The cleanest pattern is the
   explicit check we already use for the host/port pair.

### Implementation

`internal/cli/deploy_service.go`, in `runDeployService`, replace the existing
host/port check with:

```
if f.Port == 0 {
    return fmt.Errorf("--port is required (services must expose an HTTP port for the readiness probe): %w", errUsage)
}
```

Drop the now-redundant `len(f.Hosts) > 0 && f.Port == 0` check — `f.Port == 0`
already covers it.

No change to `internal/deploy/service.go`. No change to
`internal/deploy/readiness.go`. The deployer is correct as-is *given* the
contract that callers always pass a non-zero port. We're enforcing the
contract at the boundary, where it belongs.

### Tests Kent must write (in `internal/cli/deploy_service_test.go`)

- **`TestDeployService_NoPortReturnsExitUsageError`** — no `--host`, no
  `--port`. Expect `ExitUsageError`. Expect `errors.Is(err, errUsage)` (use
  the existing `ExitCodeFor` helper as the existing host-without-port test does).
- **`TestDeployService_PortZeroExplicitReturnsExitUsageError`** — explicit
  `--port=0`. Same expectation. (Belt-and-suspenders: prevents a future
  refactor from accidentally accepting `--port=0`.)
- **Update** `TestDeployService_HostWithoutPortReturnsExitUsageError` —
  still passes; the new check fires first. No assertion change needed.
- **Update** `TestDeployService_DefaultStrategyIsRecreate`,
  `TestDeployService_AutoDiscoversEnvSh`, `TestDeployService_NoEnvShIsValid`
  — these currently invoke deploy with `--name foo /srv/foo` and **no port**.
  After the new rule they will fail. Add `--port 8080` to each. The
  intent of those tests is unchanged; the fixture is now valid.

The existing `TestDeployService_HostWithoutPortReturnsExitUsageError` keeps
passing because `--host x` without `--port` still has port=0.

## Files that change

Production:

- `internal/cli/deploy_service.go` — Findings 1 and 3 (CLI-layer fixes).
- `internal/cli/root.go` — Finding 2 (pass `rc.ConfigRoot` to `logging.Init`).
- `internal/logging/logging.go` — Finding 2 (signature change: `Init(root string) error`).

Tests (Kent writes these BEFORE Rob touches production code):

- `internal/cli/deploy_service_test.go` — new tests for Findings 1 and 3,
  fixture updates for tests that previously deployed without `--port`.
- `internal/logging/logging_test.go` — signature update, new tests for
  passed-root precedence and empty-string fallback.
- `internal/cli/root_test.go` — new integration test that
  `--config-root` controls log placement end-to-end.

Docs (Raymond updates AFTER Rob is done):

- `_docs/usage.md` — table at §2: change `--port` row from
  `Required: only if --host is set` to `Required: yes`. Tighten the
  `--dockerfile` row note: "Relative paths are resolved against
  `<source-dir>`. Absolute paths are used as-is." Confirm the
  `--config-root` row already says it applies to every subcommand —
  if it implies "and logs go under it", great; otherwise add one
  short clarifying sentence.

No other files change. No new files are created.

## Order of operations

Strict TDD per CLAUDE.md. **Kent first, then Rob.** No exceptions.

1. **Kent** writes all new tests + updates the obsolete fixtures listed
   above. Every new test must fail (red) against the current production
   code. Kent commits a report stating: which tests are red, which are
   merely fixture updates, and the exact failure mode of each red test.
2. **Rob** implements the three production changes. After his work, every
   test in the repo passes — `go test ./...` clean. Rob commits a report
   summarizing each change against the plan section that authorized it.
3. **Raymond** updates `_docs/usage.md`. No code changes from Raymond.
4. **Kevlin and Linus** review in parallel. Kevlin reviews each diff for
   low-level correctness — especially that the `filepath.Join` does the
   right thing for `..` segments and absolute paths. Linus reviews the
   architectural choices: did we fix it in the right layer, did we
   preserve the public contracts, did we resist over-engineering.
5. **Back to PLAN.** Don/Joel/Linus reapprove or iterate.

## Edge cases I'm explicitly NOT addressing

So nobody wastes cycles asking later:

- **`--dockerfile=../sibling/Dockerfile`** — `filepath.Join` cleans `..`
  segments. The result may be a path outside `<source-dir>`. Docker will
  reject that with "Forbidden path outside the build context: ..." and
  the deploy fails with `ErrBuild` (exit 30). Correct error, correct exit
  code. We do not pre-validate.
- **Symlinked Dockerfile** — Docker's behavior. Not our problem.
- **`--port=0` becoming legal in M5** — when M5 lands, the right move is
  a new flag or a new subcommand, not a magic-zero. Decided above.
- **Logging during `--help`** — `PersistentPreRunE` doesn't run on
  `--help`; verified by the existing `TestRoot_HelpDoesNotRequireFilesystem`
  test. So the logging signature change cannot regress help.
- **Config-root passed via env vs flag** — the precedence is established by
  Cobra's flag default mechanism in `root.go:26`
  (`config.RootFromEnv()` as the default for `--config-root`). I am not
  changing that. The flag wins over env exactly because Cobra fills the
  flag with the env value first and then overwrites if the user passed
  `--config-root` explicitly. Same behavior as today.

## Acceptance criteria

- All three findings fixed by exactly the changes above. Nothing more.
- `go test ./...` green.
- New tests fail before Rob's changes and pass after — Kent must
  demonstrate this in his report.
- `decloud --config-root /tmp/X deploy service ...` writes logs to
  `/tmp/X/logs/decloud.log`, never to `/opt/declouding/logs/decloud.log`,
  unless `DECLOUD_LOG_TO_STDERR_ONLY=1`.
- `decloud deploy service --name foo ./bar` from any cwd builds with
  `-f /abs/path/to/bar/Dockerfile`, not with `-f Dockerfile`.
- `decloud deploy service --name foo ./bar` (no `--port`) fails with
  exit 2 and a message that names `--port` as the missing flag.

## Why this plan is RIGHT, not just working

Three defects, three fixes, three layers — and each fix lives in exactly
the layer where the contract it enforces is documented. The CLI enforces
CLI-shaped contracts (relative paths, required flags). The logging package
takes its inputs explicitly instead of reading globals. The Docker driver
stays a dumb shell around `docker build` and gains no new responsibilities.

That's the maintenance test: at 2 AM in production, when somebody asks
"why is the log file in a weird place?", the answer is one function call
in `root.go`. When somebody asks "why does `-f` resolve here?", the answer
is six lines in the CLI command, right next to the flag definition. Nobody
has to grep across three packages to reconstruct the rule.

We do not invent. We do not extend the public surface. We do not add
flags. We do the smallest correct thing in the right place and we ship.
