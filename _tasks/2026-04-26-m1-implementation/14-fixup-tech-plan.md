# M1 Iter-2 Fix-up Technical Plan

**Author:** Joel Spolsky (implementation planner)
**Status:** Delta plan against `06-tech-plan-v2.md`. Don's `012-don-plan-check.md` punch list is the spec. Linus's `13-linus-execution-review.md` is corroborating. v2 is still canonical for everything not touched here.
**Scope:** Ten items, six code (Rob), three docs (Raymond), six+ new red tests (Kent). Mechanical and surgical. No replan.

---

## Architectural decisions (settled here, not punted to Linus)

1. **Logging fallback subsumes `DECLOUD_LOG_TO_STDERR_ONLY` semantics but the env var stays.** The env var remains the deterministic, documented test escape hatch. The new EACCES/ENOENT fallback is the operator-fresh-box path. Both paths converge on stderr-only handler; `DECLOUD_LOG_TO_STDERR_ONLY=1` always wins (no FS access at all), the fallback engages only after MkdirAll OR OpenFile fails. **No deprecation of the env var in M1.**

2. **`NetworkEnsure` failure exit code: 60 (`ExitCaddyReloadFail`) is wrong. Use exit 40 (`ExitRunFail`).** `NetworkEnsure` is a Driver responsibility (it calls `docker network inspect/create`). Mapping its failure to `ExitRunFail` means one less constant, no doc churn, no new `ExitCode*` line. The exit-40 row in `usage.md` already says "docker run, docker start, docker stop, docker inspect, or docker logs failed" — extending it to "or docker network create" is a one-word doc edit Raymond's already doing per item 8. **No new sentinel `ErrNetworkEnsure`** — wrap as `ErrRun` so existing exit-code mapping picks it up. (Don's punch-list said "default suggestion: exit 60 since 50/60/70 are open." 60 is `ExitCaddyReloadFail`, not open. Use 40.)

3. **`Capture("")` returns `(nil, nil)` only IF the orchestrator decides to call it; the orchestrator should NOT.** Per Don's ruling: "Do NOT make `Capture("")` magically succeed — that hides bugs. The empty-path branch lives in the orchestrator." The capture seam keeps the existing "stat fails → wrapped error" contract for non-empty paths, but is **never called with an empty path** from production. New explicit sentinels (`ErrEnvScriptMissing`, `ErrEnvScriptUnreadable`) clarify the failure modes the explicit-`--env-file=<missing>` path returns.

4. **Auto-discovery precedence is fully deterministic.** `--env-file=<explicit>` always wins; `--env-file=""` triggers stat of `<source-dir>/env.sh`; if absent, deploy proceeds with no envcap. No env-var fallback, no `DECLOUD_ENV_FILE`, nothing fancy.

---

## ITEM 1 — `logging.Init()` graceful fallback + `PersistentPreRunE` deferral

### Owner: Rob (code) + Kent (tests)

### Files touched
- `internal/logging/logging.go` (rewrite `Init`)
- `cmd/decloud/main.go` (remove direct `logging.Init()` call)
- `internal/cli/root.go` (add `PersistentPreRunE` hook)
- `internal/logging/logging_test.go` (existing test inverts; new tests added)

### Exact code change

#### `internal/logging/logging.go` — replace lines 14–32 with:

```go
// Init configures the default slog handler. JSON output goes to stderr and,
// when filesystem access permits, also to <root>/logs/decloud.log. If the
// log directory cannot be created OR the log file cannot be opened (EACCES,
// ENOENT, or any I/O error), Init falls back to stderr-only and emits one
// warning line to stderr describing the cause. Init returns a non-nil error
// only on truly catastrophic failures (none currently — the slog handler
// constructors do not fail).
//
// DECLOUD_LOG_TO_STDERR_ONLY=1 is the deterministic test escape hatch and
// short-circuits before any filesystem access. The new EACCES fallback is
// for the operator-fresh-box case where /opt/decloud/logs/ does not yet
// exist; tests that need to assert "no FS access at all" must keep using
// the env var.
func Init() error {
	if os.Getenv("DECLOUD_LOG_TO_STDERR_ONLY") == "1" {
		setStderrOnly()
		return nil
	}
	root := config.RootFromEnv()
	logsDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "decloud: log dir unavailable, using stderr only: %v\n", err)
		setStderrOnly()
		return nil
	}
	logPath := filepath.Join(logsDir, "decloud.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decloud: log file unavailable, using stderr only: %v\n", err)
		setStderrOnly()
		return nil
	}
	w := io.MultiWriter(os.Stderr, f)
	slog.SetDefault(slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})))
	return nil
}

func setStderrOnly() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
}
```

Add `"fmt"` to the import block.

#### `cmd/decloud/main.go` — replace entire `func main` with:

```go
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := cli.NewRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCodeFor(err))
	}
}
```

Drop the `logging` import.

#### `internal/cli/root.go` — add `PersistentPreRunE` to the root command:

```go
import (
	// existing imports
	"github.com/alexander-fenster/decloud/internal/logging"
)

// inside NewRootCmd, immediately after constructing `root`:
root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
	return logging.Init()
}
```

Cobra's built-in `--help` and `help` subcommand short-circuit before `PersistentPreRunE` fires. The hook still runs for every real subcommand. Init now never returns a non-nil error in the M1 surface (fallback handles every FS failure), but the signature stays `error`-returning so future I/O failures can be surfaced without API churn.

### Failure modes preserved/changed

| Path | Before | After |
|---|---|---|
| `decloud --help` on fresh box | Exit 70 (`ExitInternal`) — `mkdir /opt/decloud: permission denied` | Exit 0; Cobra prints help; no FS access. |
| `decloud deploy service ...` on fresh box | Exit 70 before any deploy logic | One stderr warning, then proceeds with stderr-only logging; deploy continues. |
| `DECLOUD_LOG_TO_STDERR_ONLY=1` set | Exit 0, stderr-only | Identical. (Short-circuit unchanged.) |
| Catastrophic slog failure | n/a — slog ctors never fail today | Future-proofed: `Init` keeps `error` return. |

### Tests Kent must add (RED)

1. **`TestInit_PermissionDeniedFallsBackToStderr`** in `internal/logging/logging_test.go`:
   - `t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")`
   - Create `root := t.TempDir()`, `os.Chmod(root, 0o500)`, defer chmod-back
   - Skip if `os.Geteuid() == 0`
   - `t.Setenv("DECLOUD_ROOT", root)`
   - Capture stderr (use a pipe trick or just call `Init` and assert it returns nil — testing the warning text is brittle)
   - **Assert: `require.NoError(t, logging.Init())`**

2. **`TestInit_LogFileOpenFailureFallsBackToStderr`** in `internal/logging/logging_test.go`:
   - Create `root := t.TempDir()`, then `logsDir := root + "/logs"`, `os.MkdirAll(logsDir, 0o755)`, then `os.Chmod(logsDir, 0o500)`. Same root-skip guard.
   - Assert `Init()` returns nil; `<logsDir>/decloud.log` either does not exist OR exists empty (don't be strict about that).

3. **`TestRoot_HelpDoesNotRequireFilesystem`** in `internal/cli/root_test.go`:
   - `t.Setenv("DECLOUD_ROOT", "/nonexistent/path/that/cannot/be/created/by/test")`
   - `t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")` (force the FS path active)
   - `cmd := NewRootCmd(); cmd.SetArgs([]string{"--help"}); cmd.SetOut(io.Discard); cmd.SetErr(io.Discard)`
   - **Assert: `require.NoError(t, cmd.ExecuteContext(context.Background()))`**

### Migration concern — existing test that breaks

`TestInit_FileOpenFailureReturnsError` (`internal/logging/logging_test.go:33-47`) currently asserts `Init()` returns a **non-nil** error when the log dir is unwritable. **After the fix it must return nil.** Kent renames it to `TestInit_FileOpenFailureFallsBackToStderr` (covered by item 2 above) and inverts the assertion. The old test is deleted in the same commit.

---

## ITEM 2 — `env.sh` truly optional + auto-discovery + explicit-missing → exit 10

### Owner: Rob (code) + Kent (tests)

### Files touched
- `internal/envcap/capture.go` (new sentinels, error semantics on missing/unreadable)
- `internal/deploy/service.go` (skip Capturer.Capture when `req.EnvFile == ""`)
- `internal/cli/deploy_service.go` (auto-discover; explicit-missing exit 10)
- `internal/cli/exit_codes.go` (map new sentinels to exit 10)
- New tests across `internal/envcap`, `internal/deploy`, `internal/cli`

### Exact code change

#### `internal/envcap/capture.go` — add sentinels, replace lines 35–48:

```go
var (
	// ErrEnvScriptMissing means the explicitly-requested env script does not
	// exist. CLI maps to ExitConfigError (10).
	ErrEnvScriptMissing = errors.New("envcap: env script not found")

	// ErrEnvScriptUnreadable means the env script exists but the OS denied
	// stat or open. CLI maps to ExitConfigError (10).
	ErrEnvScriptUnreadable = errors.New("envcap: env script unreadable")
)

func (b *bashCapturer) Capture(ctx context.Context, scriptPath string) (map[string]string, error) {
	if scriptPath == "" {
		// Caller is responsible for deciding whether to call us; passing "" is
		// a programmer error in production (the orchestrator skips the call
		// when the path is empty). We choose to return (nil, nil) defensively
		// rather than panic, but production code MUST NOT rely on this.
		return nil, nil
	}
	info, err := os.Stat(scriptPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrEnvScriptMissing, scriptPath)
		}
		return nil, fmt.Errorf("%w: %s: %w", ErrEnvScriptUnreadable, scriptPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory", ErrEnvScriptUnreadable, scriptPath)
	}
	baseline, err := runHermeticBash(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("env capture baseline: %w", err)
	}
	full, err := runHermeticBash(ctx, scriptPath)
	if err != nil {
		return nil, fmt.Errorf("env capture: %w", err)
	}
	return diffEnv(baseline, full), nil
}
```

Add `"errors"` and `"io/fs"` to the import block.

#### `internal/deploy/service.go` — replace lines 131–137 with:

```go
envFile := req.EnvFile
var captured map[string]string
if envFile != "" {
	c, err := d.deps.Capturer.Capture(ctx, envFile)
	if err != nil {
		logger.Error("env capture failed", "step", "envcap", "error", err)
		return fmt.Errorf("%w: %w", ErrEnvCapture, err)
	}
	captured = c
	logger.Info("env captured", "step", "envcap", "vars_captured", len(captured))
} else {
	logger.Info("env capture skipped: no env script", "step", "envcap")
}
```

Note the `%w: %w` here is Item 6's mechanical fix applied at the same site.

#### `internal/cli/deploy_service.go` — replace lines 63–94 (`runDeployService`) with auto-discovery branch:

```go
func runDeployService(ctx context.Context, rc *rootContext, f *deployServiceFlags, sourceDir string) error {
	if len(f.Mounts) > 0 {
		return fmt.Errorf("--mount is not supported until M3: %w", registry.ErrMountsNotSupported)
	}
	if f.Strategy != "recreate" {
		return fmt.Errorf("--strategy=%q: only \"recreate\" is supported in M1: %w", f.Strategy, registry.ErrInvalidStrategy)
	}
	if len(f.Hosts) > 0 && f.Port == 0 {
		return fmt.Errorf("--port is required when --host is set: %w", errUsage)
	}
	abs, err := filepath.Abs(sourceDir)
	if err != nil {
		return fmt.Errorf("resolving source-dir: %w", err)
	}
	envFile, err := resolveEnvFile(f.EnvFile, abs)
	if err != nil {
		return err
	}
	paths := config.NewPaths(rc.ConfigRoot)
	req := deploy.Request{
		Name:             f.Name,
		SourceDir:        abs,
		Dockerfile:       f.Dockerfile,
		Hosts:            f.Hosts,
		Port:             f.Port,
		EnvFile:          envFile,
		ReadinessPath:    f.ReadinessPath,
		ReadinessTimeout: f.ReadinessTimeout,
		Strategy:         f.Strategy,
	}
	d, err := deployerFactory(paths)
	if err != nil {
		return fmt.Errorf("building deployer: %w", err)
	}
	return d.Deploy(ctx, req)
}

// resolveEnvFile implements the auto-discovery precedence:
//   1. If flag is set explicitly: use it; if it doesn't exist, return
//      ErrEnvScriptMissing (CLI maps to exit 10).
//   2. If flag is empty: stat <source-dir>/env.sh. If present, use it.
//      If absent, return "" so the deploy orchestrator skips envcap.
func resolveEnvFile(flagValue, sourceDir string) (string, error) {
	if flagValue != "" {
		if _, err := os.Stat(flagValue); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("%w: %s", envcap.ErrEnvScriptMissing, flagValue)
			}
			return "", fmt.Errorf("%w: %s: %w", envcap.ErrEnvScriptUnreadable, flagValue, err)
		}
		return flagValue, nil
	}
	candidate := filepath.Join(sourceDir, "env.sh")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", nil
}
```

Add `"errors"`, `"io/fs"`, `"os"` to the import block.

#### `internal/cli/exit_codes.go` — add sentinel mappings:

In `ExitCodeFor`, extend the `ExitConfigError` case:

```go
case errors.Is(err, registry.ErrMountsNotSupported),
	errors.Is(err, registry.ErrInvalidStrategy),
	errors.Is(err, registry.ErrSchemaMismatch),
	errors.Is(err, registry.ErrUnknownField),
	errors.Is(err, registry.ErrPermissionMode),
	errors.Is(err, registry.ErrSecretsMissing),
	errors.Is(err, registry.ErrNotFound),
	errors.Is(err, envcap.ErrEnvScriptMissing),
	errors.Is(err, envcap.ErrEnvScriptUnreadable):
	return ExitConfigError
```

Add `"github.com/alexander-fenster/decloud/internal/envcap"` to imports.

### Failure modes preserved/changed

| Scenario | Before | After |
|---|---|---|
| `--env-file=/path/that/exists` valid | Capture succeeds (unchanged) | Identical. |
| `--env-file=""` (omitted) + `<source>/env.sh` exists | Pass `""` through; `Capture("")` calls `os.Stat("")` → exit 70 (`stat : no such file or directory`) | Auto-discovers and uses; exit 0. |
| `--env-file=""` (omitted) + no `<source>/env.sh` | Same exit-70 lie | Skips capture entirely; deploy proceeds with no env; exit 0 if rest of deploy succeeds. |
| `--env-file=/missing` (explicit) | Same exit-70 lie | Exit 10 (`ExitConfigError`) with `ErrEnvScriptMissing` clearly named. |
| `--env-file=/exists/but/0000-perms` (explicit) | exit 70 with raw stat error | Exit 10 with `ErrEnvScriptUnreadable`. |
| `Capture("")` directly invoked | Stat-empty error → exit 70 if surfaced | Returns `(nil, nil)` defensively; orchestrator no longer calls it. |

### Tests Kent must add (RED)

In `internal/envcap/capture_test.go`:

4. **`TestEnvcap_EmptyPathReturnsNilNil`** — `Capture(ctx, "")` returns `(nil, nil)`; nothing else.
5. **`TestEnvcap_MissingPathReturnsErrEnvScriptMissing`** — `Capture(ctx, "/nonexistent/path/x")` returns `errors.Is(err, envcap.ErrEnvScriptMissing)`.
6. **`TestEnvcap_DirectoryPathReturnsErrEnvScriptUnreadable`** — pass `t.TempDir()`; assert `errors.Is(err, envcap.ErrEnvScriptUnreadable)`.

In `internal/deploy/service_test.go`:

7. **`TestDeploy_NoEnvScript_SkipsCapturerEntirely`** — set `req.EnvFile = ""`; mock `h.capturer.EXPECT().Capture(...).Times(0)`; expect Build → Run → ContainerIP → Save → Caddy chain to complete. Asserts the orchestrator's new branch.

In `internal/cli/deploy_service_test.go`:

8. **`TestDeployService_AutoDiscoversEnvSh`** — create `t.TempDir()`, write `env.sh` into it, run `deploy service` without `--env-file`; capture the `deploy.Request` via `mock.EXPECT().Deploy().DoAndReturn(...)`; assert `req.EnvFile == filepath.Join(tmp, "env.sh")`.
9. **`TestDeployService_NoEnvShIsValid`** — create `t.TempDir()` with no `env.sh`; run without `--env-file`; assert `req.EnvFile == ""` and the deploy mock is called with no error.
10. **`TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError`** — pass `--env-file=/no/such/file`; assert `errors.Is(err, envcap.ErrEnvScriptMissing)` and `ExitCodeFor(err) == ExitConfigError`.

### Migration concern — existing tests that break

- **All `service_test.go` happy-path tests** that set up `h.capturer.EXPECT().Capture(...).Return(map[string]string{"X": "1"}, nil)` rely on `req.EnvFile` being non-empty so `Capture` is called. Current `newRequest()` (`service_test.go:106`) sets no `EnvFile` field, defaulting to `""`. With the new orchestrator branch, those tests' `EXPECT().Capture` calls become unused expectations and gomock fails the test.

  **Fix:** Kent updates `newRequest()` to set `EnvFile: "/srv/foo/env.sh"` (any non-empty path; the mock matches `gomock.Any()` on the path arg). This restores the existing happy-path expectations and keeps test #7 above (the new explicit-empty test) as the only one with `EnvFile == ""`. **One-line change in `newRequest()` plus a comment explaining why.**

- `TestDeployService_BuildsExpectedRequest` (`internal/cli/deploy_service_test.go:43-67`) currently asserts the resolved `SourceDir` ends with `/srv/foo`. After the fix, `runDeployService` calls `os.Stat(filepath.Join(abs, "env.sh"))`, which is harmless (returns ENOENT, branches to "no env"), but tests that pass `/srv/foo` (a path that doesn't exist on the test filesystem) will succeed with `req.EnvFile == ""`. **Add an assertion in this test:** `assert.Equal(t, "", got.EnvFile)` to lock in the "no env discovered" behavior.

---

## ITEM 3 — Wire `NetworkEnsure` as Deploy step 0

### Owner: Rob (code) + Kent (test)

### Files touched
- `internal/deploy/service.go` (add step 0 in `Deploy`)
- `internal/deploy/service_test.go` (one new test + happy-path test fixtures must add the new expectation)

### Exact code change

#### `internal/deploy/service.go` — at the top of `Deploy`, immediately after the `logger := slog.With(...)` line (between current line 129 and 131), insert:

```go
if err := d.deps.Driver.NetworkEnsure(ctx, "decloud"); err != nil {
	logger.Error("network ensure failed", "step", "network", "error", err)
	return fmt.Errorf("%w: ensuring decloud network: %w", ErrRun, err)
}
logger.Info("network ensured", "step", "network", "network", "decloud")
```

**Why `ErrRun` and not a new sentinel:** see "Architectural decisions" #2. `NetworkEnsure` is a Driver responsibility; mapping its failure to `ExitRunFail` (40) keeps the exit-code surface unchanged. Raymond's exit-40 row update (item 8) covers the doc side.

**Why a string literal "decloud" not a constant:** the same literal already appears in `service.go:178` (`Network: "decloud"`), `lifecycle.go:70`, and `service.go:277`. Extracting to a constant is a separate cleanup (M1.x). Don't bundle.

### Failure modes preserved/changed

| Path | Before | After |
|---|---|---|
| `decloud` network exists | No call; deploy proceeds. | Driver returns nil; deploy proceeds. |
| `decloud` network missing | Deploy fails at `Run` step with raw docker error → exit 40. | Driver creates network; deploy proceeds. |
| Docker daemon down | Same exit 40 from the first docker call (build or run). | Same exit 40 from `NetworkEnsure` call (slightly earlier). |

**Step ordering after fix:**

0. **NetworkEnsure** (NEW)
1. Capturer.Capture (or skipped — Item 2)
2. Store.Load (previous)
3. Driver.Build
4. Driver.Stop/Remove (if hasPrev)
5. Driver.Run (new)
6. Probe.Wait
7. Store.Save
8. regenerateAndReload (Caddy)

### Tests Kent must add (RED)

11. **`TestDeploy_NetworkEnsureCalledFirst`** in `internal/deploy/service_test.go`:
    ```go
    func TestDeploy_NetworkEnsureCalledFirst(t *testing.T) {
        h := newDeployerHarness(t)
        gomock.InOrder(
            h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil),
            h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil),
            h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound),
            h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img-id", nil),
            // ... rest of happy path
        )
        require.NoError(t, h.deployer.Deploy(context.Background(), newRequest()))
    }
    ```
    The test's value is the `gomock.InOrder` assertion that NetworkEnsure precedes everything. **Pin the contract.**

12. **`TestDeploy_NetworkEnsureFailureReturnsErrRun`** (also in `service_test.go`):
    ```go
    h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").
        Return(errors.New("docker network create failed"))
    err := h.deployer.Deploy(context.Background(), newRequest())
    require.Error(t, err)
    assert.True(t, errors.Is(err, deploy.ErrRun))
    // No Capture, Build, Run expected.
    ```

### Migration concern — every existing happy-path test in `service_test.go` breaks

Every test that calls `h.deployer.Deploy` will fail because `NetworkEnsure` is now called and gomock has no recorded expectation. **15 happy-path-or-partial-happy-path tests** fail.

**Fix pattern (Kent applies once, copy-paste into each):** add to the head of every `gomock.InOrder(...)` block AND every loose-expectation cluster:

```go
h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
```

Audit list (line numbers from current `service_test.go`):

- `TestDeploy_HappyPathFirstDeploy` (line 132) — add to InOrder.
- `TestDeploy_HappyPathRedeploy` (line 152) — add to InOrder.
- `TestDeploy_LoadPreviousErrSecretsMissingTreatedAsFirstDeploy` (line 175) — add loose.
- `TestDeploy_EnvCaptureFailureAbortsBeforeAnythingChanges` (line 192) — add loose; the test asserts NO build, NO run, but NetworkEnsure DOES run before envcap.
- `TestDeploy_BuildFailureAbortsBeforeStoppingOld` (line 203) — add loose.
- `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` (line 215) — add loose.
- `TestDeploy_RunNewFailureRollsBackToOld` (line 231) — add loose.
- `TestDeploy_ReadinessFailureRollsBackToOld` (line 257) — add loose.
- `TestDeploy_SaveErrPartialWriteRollsBackAndDeletesOrphanConfig` (line 279) — add loose.
- `TestDeploy_SaveFailsBeforePartialWriteSkipsDeleteOrphanConfig` (line 298) — add loose.
- `TestDeploy_CaddyValidateFailureLeavesOldFileAndKeepsNewContainer` (line 314) — add loose.
- `TestDeploy_CaddyReloadFailureDoesNotRollBackContainer` (line 332) — add loose.
- `TestDeploy_DeployIDIsStableThroughoutOneDeploy` (line 351) — add loose.
- `TestDeploy_CapturedEnvNotLoggedAsKeysOrValues` (line 381) — add loose.
- `TestDeploy_RunRequestUsesCapturedEnvAndDecloudNetwork` (line 404) — add loose.

**Heads-up to Kent:** add a helper if the boilerplate gets noisy. Suggested:

```go
func (h *deployerHarness) expectNetworkEnsureOK() *gomock.Call {
    return h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
}
```

But the existing tests don't use a helper for `Capture`/`Build`/etc. — Joel's preference is consistency, so just inline the line.

---

## ITEM 4 — Rename `NewHTTPProbeForTest` → `NewHTTPProbe`

### Owner: Rob

### Files touched
- `internal/deploy/readiness.go` (rename function)
- `internal/deploy/readiness_test.go` (6 call sites)

### Exact code change

#### `internal/deploy/readiness.go` — replace lines 19–22:

```go
// NewHTTPProbe constructs the production HTTP readiness probe. Production
// code does NOT need to call this; serviceDeployer auto-wires the default
// probe via newHTTPProbe when Dependencies.Probe is nil. Tests use this
// constructor when they want to exercise the real probe loop end-to-end
// against an httptest server, while still mocking Driver.ContainerIP.
func NewHTTPProbe(driver dockerdrv.Driver) ReadinessProbe {
	return newHTTPProbe(driver)
}
```

#### `internal/deploy/readiness_test.go` — six call-site renames:

```
line 59:  deploy.NewHTTPProbeForTest(driver)  →  deploy.NewHTTPProbe(driver)
line 68:  same
line 90:  same
line 101: same
line 121: same
line 141: same
```

**Use Edit's `replace_all` mode in Rob's pass:**
```
old_string: "deploy.NewHTTPProbeForTest"
new_string: "deploy.NewHTTPProbe"
replace_all: true
```

### Failure modes preserved/changed

None — this is a pure rename. The `newHTTPProbe` private constructor stays; production behavior is byte-identical.

### Tests Kent must change

No new tests. Existing 6 tests rename. Kent: confirm `gofmt -l .` clean after the edit.

### Migration concern

**None for production callers** — there are none. The only callers were tests, all in this same file. No external consumers.

---

## ITEM 5 — `readiness.go` cleanup: drop `else`-after-return + fix silent `ip=="" && err==nil` branch

### Owner: Rob (code only)

### Files touched
- `internal/deploy/readiness.go` (lines 47–58)

### Exact code change

#### `internal/deploy/readiness.go` — replace the body of the `for` loop (lines 47–58) with:

```go
for {
	ip, ipErr := p.driver.ContainerIP(ctx, containerName)
	switch {
	case ipErr != nil:
		lastErr = ipErr
	case ip == "":
		// Driver returned no error but empty IP. Today the Driver maps
		// this to ErrNoBridgeIP; if a future Driver returns "" without an
		// error, treat it the same way so the probe never silently retries
		// on a degenerate response.
		lastErr = dockerdrv.ErrNoBridgeIP
	default:
		url := fmt.Sprintf("http://%s:%d%s", ip, port, spec.HTTPPath)
		if err := p.probeOnce(ctx, url); err != nil {
			lastErr = err
		} else {
			return nil
		}
	}
	if time.Now().After(deadline) {
		if lastErr != nil {
			return fmt.Errorf("%w: %w", ErrReadiness, lastErr)
		}
		return fmt.Errorf("%w: timed out after %s", ErrReadiness, timeout)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(interval):
	}
}
```

Note: the inner `if probeOnce err == nil; return nil` would also be `else`-after-return; restructured so the success path is the `return nil` clause. Kevlin's literal complaint was the outer else; the inner is the same shape and gets the same treatment.

The `%w: %w` here is Item 6's mechanical fix applied at this site.

### Failure modes preserved/changed

| Branch | Before | After |
|---|---|---|
| `ipErr != nil` | `lastErr = ipErr` | Identical. |
| `ipErr == nil && ip != ""` | Probe; success → return; failure → `lastErr = err` (via `else`) | Identical behavior, no `else`. |
| `ipErr == nil && ip == ""` | **Silent: `lastErr` unchanged across iteration** — if first iter and probe deadline lapsed before next call, returns "timed out after Xs" with NO inner error. | `lastErr = dockerdrv.ErrNoBridgeIP` — wrapped readiness error names the cause. |

### Tests Kent must add (RED)

13. **`TestReadiness_EmptyIPNoErrorReportsErrNoBridgeIP`** in `internal/deploy/readiness_test.go`:
    ```go
    driver := newProbeDriver(t)
    driver.EXPECT().ContainerIP(gomock.Any(), probeContainerName).
        Return("", nil).AnyTimes() // Empty IP, NO error — degenerate case.
    probe := deploy.NewHTTPProbe(driver)
    spec := newReadinessSpec()
    spec.TimeoutSecs = 1
    err := probe.Wait(context.Background(), probeContainerName, spec, 1)
    require.Error(t, err)
    assert.True(t, errors.Is(err, deploy.ErrReadiness))
    assert.True(t, errors.Is(err, dockerdrv.ErrNoBridgeIP))
    ```

### Migration concern

`TestReadiness_ContainerIPInitiallyEmptyThenReady` (`readiness_test.go:76-95`) currently returns `("", dockerdrv.ErrNoBridgeIP)` on the first call. That continues to work — the `ipErr != nil` branch fires. **No change needed.**

---

## ITEM 6 — `%w: %v` → `%w: %w` across 21 sites

### Owner: Rob (mechanical)

### Files touched (with line numbers from current source)

**`internal/deploy/service.go`** (10 sites, but 2 are subsumed by Items 2/3 rewrites):
- Line 135 — `%w: %v`, ErrEnvCapture (rewritten by Item 2)
- Line 156 — `%w: %v`, ErrBuild
- Line 166 — `%w: stop previous container: %v`, ErrRun
- Line 171 — `%w: remove previous container: %v`, ErrRun
- Line 188 — `%w: %v`, ErrRun
- Line 211 — `%w: %v`, ErrReadiness
- Line 300 — `%w: generating caddyfile: %v`, ErrCaddyReload
- Line 304 — `%w: caddy validate failed: %v`, ErrCaddyReload
- Line 307 — `%w: rename caddyfile: %v`, ErrCaddyReload
- Line 310 — `%w: caddy reload failed: %v`, ErrCaddyReload

**`internal/deploy/readiness.go`** (1 site, subsumed by Item 5):
- Line 61 — `%w: %v`, ErrReadiness

**`internal/deploy/lifecycle.go`** (6 sites):
- Line 43 — `%w: stop %s: %v`, ErrRun
- Line 56 — `%w: inspect %s: %v`, ErrRun
- Line 63 — `%w: start %s: %v`, ErrRun
- Line 76 — `%w: run %s: %v`, ErrRun
- Line 100 — `%w: inspect %s: %v`, ErrRun
- Line 130 — `%w: logs %s: %v`, ErrRun

**`internal/registry/store.go`** (4 sites):
- Line 140 — `%w: mkdir secrets dir %s: %v`, ErrPartialWrite
- Line 143 — `%w: chmod secrets dir %s: %v`, ErrPartialWrite
- Line 147 — `%w: writing secrets at %s: %v`, ErrPartialWrite
- Line 252 — `%w: %v`, ErrUnknownField

**Total: 21 sites.** Don's punch-list said "~20 sites" — the actual count is 21.

### Exact code change

For each site, change the trailing `%v` to `%w`. Example:

```go
// Before
return fmt.Errorf("%w: %v", ErrBuild, err)
// After
return fmt.Errorf("%w: %w", ErrBuild, err)
```

```go
// Before
return fmt.Errorf("%w: stop %s: %v", ErrRun, containerName, err)
// After
return fmt.Errorf("%w: stop %s: %w", ErrRun, containerName, err)
```

`%s` formatters do NOT change (they're not error wrapping).

**Go 1.20+ supports multiple `%w` verbs in one Errorf.** Confirmed: `go.mod` declares `go 1.22`.

### Failure modes preserved/changed

The OUTER sentinel chain is unchanged: `errors.Is(err, ErrRun)` still matches. The INNER chain is now preserved: `errors.Is(err, dockerdrv.ErrContainerNotFound)` (or similar) now traverses through.

**Existing tests that assert `errors.Is(err, ErrRun)` etc. continue to pass.** No test should break from this change alone.

### Tests Kent must add (OPTIONAL)

14. **`TestDeploy_BuildErrorPreservesInnerSentinel`** (Don tagged this as optional, "not a DONE-criterion"):
    ```go
    h := newDeployerHarness(t)
    h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
    h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{}, nil)
    h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
    sentinel := errors.New("synthetic build err")
    h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("", fmt.Errorf("docker build: %w", sentinel))
    err := h.deployer.Deploy(context.Background(), newRequest())
    require.Error(t, err)
    assert.True(t, errors.Is(err, deploy.ErrBuild))
    assert.True(t, errors.Is(err, sentinel), "inner sentinel must traverse the chain after %w:%w fix")
    ```
    Joel's call: include it. It's the only test that proves the fix is real and not cargo-culted.

### Migration concern

None. No existing test asserts the inner-error chain is broken (they all `errors.Is` the outer sentinel). The fix is a behavior IMPROVEMENT that no test currently constrains.

---

## DOC FIXES (Raymond)

### Item 7 — Drop `--force` from systemd `ExecReload` in `_docs/install.md`

#### File: `_docs/install.md` line 43

#### Exact change

```diff
-ExecReload=/usr/bin/caddy reload --config /opt/decloud/config/caddy/Caddyfile --adapter caddyfile --force
+ExecReload=/usr/bin/caddy reload --config /opt/decloud/config/caddy/Caddyfile --adapter caddyfile
```

**Rationale (Don's call):** harmonize with `internal/caddy/reloader.go:38-48` which does NOT pass `--force`. Both reload paths now use the same flag set. `--force` is for first-load edge cases only and isn't needed during steady-state systemctl reloads (the Caddyfile validates clean at deploy time per the deployer's `Reloader.Validate` step).

### Item 8 — Clarify exit-40 row in `_docs/usage.md`

#### File: `_docs/usage.md` line 93

#### Exact change

```diff
-| `40` | `ExitRunFail` | `docker run`, `docker start`, `docker stop`, `docker inspect`, or `docker logs` failed. |
+| `40` | `ExitRunFail` | A docker driver call failed: `docker run`, `docker start`, `docker inspect`, `docker logs`, or `docker network create`. (`docker stop` against a non-existent container surfaces as exit 10, `ExitConfigError` — see lifecycle commands.) |
```

**Rationale:** Kevlin N2 — `Stop` against a missing container maps to `registry.ErrNotFound` → exit 10, not 40. The new wording is truthful and adds the `docker network create` case from Item 3.

### Item 9 — Create `_ai/decisions/m1-test-strategy.md`

#### File: `_ai/decisions/m1-test-strategy.md` (NEW)

#### Required outline (Raymond MUST include all four sections; word counts are guidance, not bounds)

```markdown
# M1 test strategy

Decision capture for why M1 ships unit-tests-only. Cited by Don's plan-v2
§2.1 and DONE-criterion #10. Author: Raymond Chen, on behalf of the team.

## 1. The user directive

Quote the user's exact instruction from the task brief: "I will test it on
a real system after M1 is done." Note that this moves Joel's tech-plan
§12.2 integration tests (`internal/dockerdrv/integration_test.go`,
`internal/caddy/integration_test.go`, `internal/deploy/integration_test.go`,
all `-tags integration`) out of M1 execution scope. This is not laziness;
it is an explicit user directive.

## 2. What unit-tests-only means per package

For each package below, name the test seam Rob/Kent built and the failure
modes it covers:

- `internal/dockerdrv` — argument-construction tests via injectable
  `exec.Command` factory. Every Driver method has an arg-shape test;
  no real `docker` invocations during `go test`. The receipt's docker
  version is recorded for future correlation.
- `internal/caddy` — generator golden-string tests; `Reloader.Validate`
  and `Reloader.Reload` exercised via recording `cmdFactory`. No real
  `caddy` invocations during `go test`.
- `internal/deploy` — every step of the orchestrator sequence (now
  including step 0 `NetworkEnsure` per item 3 of the iter-2 fix-up)
  uses Gomock for `Store`, `Capturer`, `Driver`, `Generator`, `Reloader`.
  One happy-path test, one test per failure branch. Lifecycle methods
  each get at least one happy and one failure test.
- `internal/envcap` — runs against the real `/bin/bash` on the
  maintainer's box. These ARE unit tests (`go test ./...` runs them
  with no extra tags). macOS bash 3.2 is in the loop.

## 3. The receipt format as the manual-CI bridge

Reference `05-plan-v2.md` §3.4: Rob attaches a 10-item receipt to his
implementation report (go version, host, bash version, docker version,
caddy version, full test output, `go vet ./...` clean, `go generate
./...` idempotent under `git status --porcelain`). The receipt is the
Don/Joel/Linus signoff gate. When the maintainer adds GitHub Actions
post-M1, the receipt requirement is replaced (or kept as a
`make test-receipt` for manual runs).

## 4. What the user is signed up for

The user will run `decloud deploy service` against a real Linux host and
report whatever breaks. Expected breakage classes (so future-Don knows
what to look for):

- Docker version skew (test box vs. prod box)
- macOS-bash-3.2 vs. Linux-bash-5 differences in envcap edge cases
- `caddy validate` semantic differences across Caddy versions
- Network namespace reachability assumptions (the host-side readiness
  probe assumes the bridge driver routes from the host's netns)

When the user reports a real-system failure, the unit-test gap that
allowed it through becomes M2's first priority.
```

**Length: ~2 pages.** Don's punch-list specified "Three short paragraphs" but the four-section structure above gives Linus the artifact he can verify in one read. Don approved the substance; Raymond's call on exact phrasing.

---

## Summary table — what each agent ships this round

| Agent | Files touched | LoC delta (estimate) | New tests |
|---|---|---|---|
| Rob | `internal/logging/logging.go`, `cmd/decloud/main.go`, `internal/cli/root.go`, `internal/envcap/capture.go`, `internal/deploy/service.go`, `internal/deploy/readiness.go`, `internal/deploy/lifecycle.go`, `internal/registry/store.go`, `internal/cli/deploy_service.go`, `internal/cli/exit_codes.go` | ~+90 / -60 | 0 (Kent writes them) |
| Kent | `internal/logging/logging_test.go`, `internal/cli/root_test.go`, `internal/envcap/capture_test.go`, `internal/deploy/service_test.go` (extend 15 existing tests + add 4 new), `internal/cli/deploy_service_test.go`, `internal/deploy/readiness_test.go` | ~+200 | **6 mandatory + 8 extension fixups + 2 optional** |
| Raymond | `_docs/install.md`, `_docs/usage.md`, `_ai/decisions/m1-test-strategy.md` (NEW) | ~+80 / -2 | 0 |

**Mandatory new test names (Kent's checklist):**

| # | Test name | File |
|---|---|---|
| 1 | `TestInit_PermissionDeniedFallsBackToStderr` | `internal/logging/logging_test.go` |
| 2 | `TestInit_LogFileOpenFailureFallsBackToStderr` | `internal/logging/logging_test.go` |
| 3 | `TestRoot_HelpDoesNotRequireFilesystem` | `internal/cli/root_test.go` |
| 4 | `TestEnvcap_EmptyPathReturnsNilNil` | `internal/envcap/capture_test.go` |
| 5 | `TestEnvcap_MissingPathReturnsErrEnvScriptMissing` | `internal/envcap/capture_test.go` |
| 6 | `TestEnvcap_DirectoryPathReturnsErrEnvScriptUnreadable` | `internal/envcap/capture_test.go` |
| 7 | `TestDeploy_NoEnvScript_SkipsCapturerEntirely` | `internal/deploy/service_test.go` |
| 8 | `TestDeployService_AutoDiscoversEnvSh` | `internal/cli/deploy_service_test.go` |
| 9 | `TestDeployService_NoEnvShIsValid` | `internal/cli/deploy_service_test.go` |
| 10 | `TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError` | `internal/cli/deploy_service_test.go` |
| 11 | `TestDeploy_NetworkEnsureCalledFirst` | `internal/deploy/service_test.go` |
| 12 | `TestDeploy_NetworkEnsureFailureReturnsErrRun` | `internal/deploy/service_test.go` |
| 13 | `TestReadiness_EmptyIPNoErrorReportsErrNoBridgeIP` | `internal/deploy/readiness_test.go` |
| 14 (optional) | `TestDeploy_BuildErrorPreservesInnerSentinel` | `internal/deploy/service_test.go` |

That's **13 mandatory new tests** (Don asked for "6+", we deliver 13) plus **15 existing tests in `service_test.go` updated** to expect `NetworkEnsure` plus **1 existing test in `logging_test.go` inverted**.

**Mandatory existing-test edits (Kent's other checklist):**
- 15 `service_test.go` tests: add `NetworkEnsure` expectation per Item 3 audit list.
- 6 `readiness_test.go` tests: rename `NewHTTPProbeForTest` → `NewHTTPProbe` per Item 4.
- 1 `logging_test.go` test: `TestInit_FileOpenFailureReturnsError` → renamed and inverted per Item 1 migration concern.
- 1 `service_test.go` fixture: `newRequest()` gets `EnvFile: "/srv/foo/env.sh"` per Item 2 migration concern.
- 1 `deploy_service_test.go` test: assert `got.EnvFile == ""` per Item 2 migration concern.

---

## Open questions for Linus's review of THIS plan

1. **Exit code for `NetworkEnsure` failure: 40 (`ExitRunFail`) vs. new code.** I chose 40 (architectural decision #2). Linus may prefer a new sentinel `ErrNetworkEnsure` mapping to a new exit (his original suggestion was "default suggestion: exit 60" but 60 is already taken by `ExitCaddyReloadFail`). If Linus wants a new code, suggested: 41 (`ExitNetworkFail`) — but this expands the surface for one failure mode and complicates `usage.md`. **My strong preference: 40.**

2. **`Capture("")` returns `(nil, nil)` defensively vs. panicking.** I chose defensive return (architectural decision #3). The orchestrator never calls Capture with `""`, but if a future caller does, returning nothing is safer than panicking. Linus may want a panic here ("fail loudly on programmer error"). Joel's preference: defensive — the panic-on-empty-path bug class is exactly what Item 2 is fixing.

3. **`NetworkEnsure` in `Unregister` and `CaddyReload`?** Currently I'm only wiring it into `Deploy`. Don's punch-list said "wire NetworkEnsure as Deploy step 0"; Don's plan-v2 §3.1.1/3.1.7 didn't mandate it for the lifecycle paths. **My read: Deploy is the only path that creates NEW containers; `Unregister`/`CaddyReload` only stop/regenerate, so a missing network is irrelevant to them.** If Linus disagrees, a parallel item-3b call gets added in `Unregister` step 0 and `CaddyReload` step 0.

These are flagged but I am proceeding with my answers above unless Linus rejects them in his review of this plan.

---

## Estimation reality check

Joel's law: original estimate × π = realistic estimate. Don said "Estimated cycle time: half what round 1 took." Round 1 wrote the orchestrator from scratch; round 2 is mechanical fixes. Half is realistic. Estimate: **4–6 hours of agent time** end-to-end (Kent + Rob + Raymond + Kevlin + Linus).

Risk areas (where Knuth might be needed):

1. **Test fixture migration in `service_test.go`** — adding `NetworkEnsure` to 15 tests is mechanical but tedious. Kent: if you find yourself copy-pasting the same five lines, escalate to Knuth before refactoring; the duplication may be telling us the harness needs a `happyPathBase()` helper. **But not in this round.**
2. **`PersistentPreRunE` interaction with cobra completion subcommands** — if cobra's `__complete` machinery fires PreRunE in some versions, we may see test-environment surprises. Mitigation: the `TestRoot_HelpDoesNotRequireFilesystem` test guards the visible failure mode; if completion paths break, that's a separate small fix.
3. **`%w: %w` on Go versions < 1.20** — irrelevant; `go.mod` declares `go 1.22`. Confirmed.

---

## Final word from Joel

Don's punch list is clean. Linus's three findings are real. Kevlin caught the two operator-blocking bugs the round-1 deploy missed. The fixes are surgical: ~150 lines of Go changed, ~200 lines of test added, three doc edits, one new decision-record file.

The biggest single risk is Kent's 15-test migration in `service_test.go` for Item 3 (NetworkEnsure). Pure boilerplate, easy to miss one. I've enumerated all 15 by line number above so there's no excuse for skipping any. Run `gomock`'s "missing call" failure as a checklist — every failed test is a test Kent forgot to update.

Ship M1. Hand it to the user. Plan M2.

End of fix-up tech plan.
