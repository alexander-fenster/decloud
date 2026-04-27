# Kevlin's low-level review

I traced every claim in `02-plan.md` / `03-tech-plan.md` against the actual
diff and the running code. I ran `go test ./...`, `gofmt -l`, `go vet
./...`. Everything is green. The fixes are minimal, the layering is right,
and the tests interrogate the right behaviour.

I have **one concrete defect** to flag (production code, easy fix, blocks
approval) and a couple of small observations that are not blocking but
worth recording.

---

## Verification matrix

| Check | Result |
|---|---|
| `go test ./...` (full tree, `-count=1`) | all green |
| `gofmt -l . internal cmd` | empty |
| `go vet ./...` | empty |
| Diff is restricted to the files in the plan | yes (production: 3; tests: 3; docs: 3) |
| `internal/dockerdrv/`, `internal/deploy/`, `internal/registry/`, `cmd/` untouched | confirmed via `git --no-pager diff main` |

## Production code — correctness against the plan

### `internal/cli/deploy_service.go`

- Lines 73-75 (Finding 3): `if f.Port == 0 { return fmt.Errorf("--port is required: %w", errUsage) }` — matches the plan, wraps `errUsage`, mapped to `ExitUsageError` by `internal/cli/exit_codes.go:35-36`. Strict superset of the old host-without-port check.
- Lines 80-86 (Finding 1): six-line resolution block, exactly as Don and Joel specified. The order — empty-string guard → `IsAbs` short-circuit → `Join(abs, ...)` — is correct. `abs` here is the resolved source dir from `filepath.Abs(sourceDir)` two lines up.
- Line 95: `Dockerfile: dockerfile` (resolved value) instead of `f.Dockerfile`. Correct.
- The validation order (mount → strategy → port → abs → envFile → dockerfile-resolve → request) is preserved; mount/strategy still surface their typed sentinels with `ExitConfigError` rather than being shadowed by the new `--port` check.

### `internal/logging/logging.go`

- Signature: `func Init(root string) error` — matches.
- Body: stderr short-circuit BEFORE filesystem access (preserved); empty-string root falls back to `config.DefaultRoot` (matches `config.NewPaths` semantics in `internal/config/paths.go:24-27`).
- Doc comment now describes the new contract accurately.
- `config` import retained (now references `config.DefaultRoot` instead of `config.RootFromEnv()`).

### `internal/cli/root.go`

- Line 23: `return logging.Init(rc.ConfigRoot)`. Cobra populates `rc.ConfigRoot` from the persistent `--config-root` flag (default `config.RootFromEnv()`) before `PersistentPreRunE` runs, so the flag-wins-over-env precedence is preserved at the Cobra layer. No env fallback inside `Init` — exactly the explicit-input contract Don wanted.

## Issue 1 — Stale `--port` flag help (minor, but blocks approval)

`internal/cli/deploy_service.go:55`:

```go
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required if --host set)")
```

After Finding 3, `--port` is now unconditionally required. `_docs/usage.md`
already says `Required: yes`. The CLI's own `--help` output still tells
operators "(required if --host set)", which contradicts both the docs and
the runtime behaviour. The first thing an operator hits when something
breaks is `decloud deploy service --help`; getting two different stories
from `--help` and the manual is exactly the kind of drift this whole task
is supposed to be fixing.

Fix (one-liner):

```go
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required)")
```

Ship this together with the rest of the task. Otherwise we have
introduced a fresh CLI/docs inconsistency in the act of fixing one.

## Issue 2 — `TestDeployService_HostWithoutPortReturnsExitUsageError` is now slightly mis-named (non-blocking)

After Rob's diff, `--host foo.example.com` with no `--port` fails for the
generic "no port" reason rather than the specific "host without port"
reason. The test still passes because both paths return `errUsage` and
`ExitUsageError`. Nothing rotted into a wrong assertion (the silent
regression Joel caught for `TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError`
does NOT apply here).

The plan explicitly accepted this in Don's §"Finding 3 / Tests Kent must
write": *"the new check fires first. No assertion change needed."* I am
recording it only so a future reader does not waste cycles wondering
whether the test still earns its name. Not blocking.

## Issue 3 — Cosmetic: docstring says `Init(string) {}` (non-blocking)

`_ai/cobra-init-pattern.md` now reads:

> Do not collapse to `Init(string) {}` — keep the error return for future use.

`Init(string) {}` is not valid Go; the intent is clearly "do not collapse
to a body-less / error-less variant". The original phrasing
(`Init() {}`) had the same pseudo-Go quality. Trivial; ignore unless
Raymond is touching the file again.

## Test review

### Did the new tests verify the right behaviour?

Yes. Spot-checks:

- `TestDeployService_DefaultDockerfileIsJoinedWithSourceDir`,
  `TestDeployService_RelativeDockerfileIsJoinedWithSourceDir` — assert
  on the resolved absolute join via `filepath.Join`. Distinguishable
  from a `filepath.Base`-only regression.
- `TestDeployService_AbsoluteDockerfileIsPreserved` — passes the absolute
  path through unchanged. Catches an "always-join" regression.
- `TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved` —
  runs from a different cwd than the source dir. This is the regression
  test against the original reviewer-reported failure mode. The
  `filepath.EvalSymlinks` portability fix is justified (macOS resolves
  `/var` → `/private/var`); only blemish is that the test now relies on
  symlink resolution going one direction, but Rob's note in §2 of
  `06-rob-impl.md` is correct that this is a test-only portability fix.
- `TestDeployService_NoPortReturnsExitUsageError` — uses `errors.Is(err, errUsage)`, exactly the right typed-sentinel assertion (rather than substring).
- `TestDeployService_PortZeroExplicitReturnsExitUsageError` — locks in that explicit `--port=0` is rejected, preventing a future "treat 0 as unset" regression.
- `TestInit_UsesPassedRootNotEnv` — positive (`os.Stat(passedRoot/logs/decloud.log)`) AND negative (`os.IsNotExist("/path/that/must/not/be/written/to")`). Belt-and-suspenders against future reintroduction of env-reading.
- `TestInit_EmptyStringRootFallsBackToDefault` — Joel correctly notes this is intentionally weak (only proves no panic). Strong-form testing would require writing to `/opt/declouding`, which is a non-starter. Acceptable.
- `TestRoot_ConfigRootFlagControlsLogPlacement` — end-to-end. Asserts the log appears under `flagRoot` and NOT under `envRoot`. Linus's requested explanatory comment is present at root_test.go:69-72.

### Change-detector tests sneaking in?

I checked. None. Every new test asserts behaviour that the production
code is responsible for, not internal structure. The fixture updates
(adding `--port 8080` to four existing tests) preserve test intent — the
"default strategy is recreate" test still asserts the strategy field.

### Test isolation

- `t.Parallel()` is correctly absent (gotcha G1).
- `os.Chdir` cleanup uses `t.Cleanup(func() { _ = os.Chdir(origCwd) })`. The leading `_ =` is correct — best-effort restore is the only sane choice.
- `errUsage` reference works because `deploy_service_test.go` is `package cli`, not `package cli_test`. Verified.

## Documentation review

### `_docs/usage.md` — three table rows

I cross-checked every claim against the production diff:

| Claim | Source of truth | Match |
|---|---|---|
| `--port` Required: `yes`; default `—` | `deploy_service.go:73-75` (unconditional check); cosmetic dash chosen for the same reason `--name` is presented as `—` | yes |
| `--port` error message `--port is required` | `deploy_service.go:74` literal string | yes |
| `--port` exits 2 | `exit_codes.go:35-36` (`errUsage` → `ExitUsageError`) | yes |
| Worker/job workloads are M5 | `_ai/decisions/m1-scope.md`, `README.md` | yes (existing references) |
| `--dockerfile` "Relative paths resolve under `<source-dir>` regardless of cwd" | `deploy_service.go:80-86`; locked in by `TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved` | yes |
| `--dockerfile` "Absolute paths are used as-is" | `deploy_service.go:84` (`filepath.IsAbs` skip) | yes |
| `--config-root` "Logs are written to `<config-root>/logs/decloud.log`" | `logging.go:30,36` (`logsDir := filepath.Join(root, "logs")`, log file `decloud.log`); `root.go:23` (`Init(rc.ConfigRoot)`) | yes |

**No hallucinations.** Field names, paths, exit codes, error strings —
every claim maps to a file:line in the production code. The table
formatting is consistent with the existing rows.

### `_ai/cobra-init-pattern.md`

- Signature in recipe block: `Init(root string) error` — matches `logging.go:22`.
- Caller body: `return logging.Init(rc.ConfigRoot)` — matches `root.go:23`.
- Line range `22-46` — matches; I confirmed `Init` opens at line 22 and the closing brace is at line 46.
- Step list re-numbered from 4 to 5 to insert the new "empty-string root → DefaultRoot" step at #2. Order is correct: env short-circuit → empty-string fallback → MkdirAll → OpenFile → return nil.

### `_ai/m1x-backlog.md`

- New line numbers `:32` and `:39` for the two `Fprintf` warnings — verified directly with `grep -n fmt.Fprintf internal/logging/logging.go`. Both correct.
- Note about `--config-root <t.TempDir()>` reducing the warning noise — accurate (Rob noted the same in §5 of his impl report).

## Defensive code

- The empty-string `dockerfile` guard (`if dockerfile == ""`) is justified and load-bearing: a shell-passed `--dockerfile=""` would otherwise produce `filepath.Join(abs, "")` → `abs`, which Docker would reject with a confusing "is a directory" error. Joel's G8/R2 analysis is correct. Keep.
- The `if root == ""` guard in `logging.Init` mirrors `config.NewPaths`. One source of truth (`config.DefaultRoot`); two consumers; same fallback rule. Correct.
- `_ = os.Chdir(origCwd)` in test cleanup — best-effort restore is correct here; if it fails the process is already in trouble and `t.Cleanup` is best-effort by design.

## Defensive code that's missing — none

- The deployer/probe/driver are correct given the contract; the contract is now enforced at the CLI boundary. No need for a `port == 0` short-circuit in `internal/deploy/service.go` or `internal/deploy/readiness.go`. Adding one would split the policy across layers.
- No need to pre-validate that the resolved Dockerfile path exists or is inside the build context — Docker enforces both with clear error messages. Don's "edge cases I'm explicitly NOT addressing" is right.

---

## Verdict

**NEEDS REVISION** — solely because of Issue 1 above.

`internal/cli/deploy_service.go:55` still has the stale flag help text
`"container listen port (required if --host set)"`. The behaviour is now
"required, full stop", and the user-facing manual already says so. The
fix is one line, takes seconds, and cannot be deferred without leaving
`decloud deploy service --help` lying to operators while the docs tell
the truth. Fix that, re-run `go test ./...`, and we're good.

Everything else — the production diffs, the test plan, the doc updates,
the AI decision-record updates — is small, correct, and lives where its
contract is documented. Three findings, three fixes, three layers, no
new abstractions, no new flags, no new files. This is what small
correct work looks like, with one cosmetic loose thread that needs to
be tucked in before we ship.

— Kevlin
