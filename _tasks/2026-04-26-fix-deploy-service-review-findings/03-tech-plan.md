# Joel's tech plan: fix three review findings on M1 deploy service

Don's plan (`02-plan.md`) is correct. I verified every file:line claim against
the actual source. This document expands his plan into the level of detail
where Kent can write the failing tests without architectural decisions and
Rob can apply the production diff line-for-line.

If you disagree with anything below, push back BEFORE writing code. After
this document is approved, Kent and Rob execute strictly.

---

## 0. Verification of Don's claims (so Linus does not have to re-walk)

I traced every file:line in Don's plan. All match. Specifically:

- `internal/cli/deploy_service.go:31-41` — `deployServiceFlags` struct.
- `internal/cli/deploy_service.go:43-64` — `newDeployServiceCmd` flag bindings;
  `--dockerfile` default `"Dockerfile"`, help `"Dockerfile path relative to
  <source-dir>"` (line 61). `--port` is `IntVar` with default `0` (line 55).
- `internal/cli/deploy_service.go:66-101` — `runDeployService`. Validation at
  73 (host without port), `filepath.Abs(sourceDir)` at 76, `Dockerfile:
  f.Dockerfile` straight-copy at 88.
- `internal/cli/exit_codes.go:26` — `errUsage` is **lowercase, package-local**.
  Visible from `package cli` test files only. Both existing test files use
  `package cli` (not `cli_test`), so `errors.Is(err, errUsage)` is callable
  from new tests in `internal/cli/deploy_service_test.go`.
- `internal/cli/exit_codes.go:35-46` — `errUsage` => exit 2; verified.
- `internal/cli/root.go:22-27` — `PersistentPreRunE` calls `logging.Init()`
  with no argument. `--config-root` flag declared at 26 with default
  `config.RootFromEnv()`. `rc.ConfigRoot` is filled by Cobra **before**
  `PersistentPreRunE` runs — verified by the existing
  `TestRoot_ConfigRootFlagOverridesEnv` (root_test.go:46-53) which already
  exercises this ordering.
- `internal/logging/logging.go:21-43` — `Init() error`. Reads
  `DECLOUD_LOG_TO_STDERR_ONLY` first; on `"1"` short-circuits. Then
  `config.RootFromEnv()` => `<root>/logs/decloud.log`. Filesystem failure
  paths fall back to stderr-only with a single warning line.
- `internal/dockerdrv/cli_driver.go:34-43` — `Build` shells out
  `docker build -t <ref> -f <Dockerfile> <abs source dir>`. `req.Dockerfile`
  passes through unmodified.
- `internal/deploy/service.go:160-169` — `BuildRequest.Dockerfile = req.Dockerfile`.
- `internal/deploy/service.go:213` — `d.probe.Wait(ctx, containerName, spec, req.Port)`,
  unconditional.
- `internal/deploy/service.go:236` — `BuildSpec{Dockerfile: req.Dockerfile, ...}`
  is what gets persisted to `config/services/<name>.toml`.
- `internal/deploy/readiness.go:55` — `fmt.Sprintf("http://%s:%d%s", ip, port, spec.HTTPPath)`.
  `port=0` => `http://<ip>:0/healthz`. Confirmed.
- `internal/config/paths.go:8` — `const DefaultRoot = "/opt/declouding"` is
  exported. Don's "empty string => `config.DefaultRoot`" mapping is exactly
  what `config.NewPaths` does at line 25-27. Same fallback policy.

### Cross-cutting concerns I verified

**Callers of `logging.Init()` (whole repo):**

```
internal/cli/root.go:23                 — production (will change)
internal/logging/logging_test.go:17     — TestInit_StderrOnlyShortCircuit
internal/logging/logging_test.go:25     — TestInit_DefaultWritesToFileAndStderr
internal/logging/logging_test.go:45     — TestInit_PermissionDeniedFallsBackToStderr
internal/logging/logging_test.go:61     — TestInit_LogFileOpenFailureFallsBackToStderr
```

Five callers total: one production, four tests. The signature break is fully
contained. Kent updates the four tests in lockstep with Rob's signature change.

**Callers of `req.Dockerfile`:**

```
internal/deploy/service.go:163  — passes to dockerdrv.BuildRequest
internal/deploy/service.go:236  — persists to registry as Build.Dockerfile
```

Plus `req.Dockerfile` is set in `internal/cli/deploy_service.go:88`. No other
reader. The registry persisted value will become an absolute path post-fix.
That is FINE because:

1. Registry loaders never re-execute `docker build` from the persisted path
   (`decloud start`, `restart`, `stop`, `logs`, `status` all reuse the
   already-built `Build.ImageRef`, never re-resolve `Build.Dockerfile`).
   Verified by `grep -rn "Build\.Dockerfile" internal --include='*.go'` —
   no readers outside the deploy path.
2. `internal/registry/store_test.go` round-trips arbitrary string content;
   it does not assert any particular path shape on disk.
3. The TOML file is operator-readable; an absolute path is **better**
   provenance than a bare `"Dockerfile"` because it tells the operator
   exactly which file was used.

**No other callers of "dockerfile resolution" exist.** The CLI is the only
producer of `req.Dockerfile`. The fix lives at the producer. Done.

**`isCobraUsageError`** (internal/cli/exit_codes.go:69-79) detects Cobra's
own pre-run validation errors via substring matching. Our new check uses
`errUsage` directly so it goes through the typed branch at line 35, NOT the
substring branch. No regression risk.

---

## 1. Finding 1 — `--dockerfile` resolved against the wrong directory

### What changes

**File:** `internal/cli/deploy_service.go` — function `runDeployService`,
between line 79 (`return ...err`) and line 80 (`envFile, err := resolveEnvFile`).

### Production diff (Rob)

Insert after the `filepath.Abs(sourceDir)` block, before `resolveEnvFile`:

```go
dockerfile := f.Dockerfile
if dockerfile == "" {
    dockerfile = "Dockerfile"
}
if !filepath.IsAbs(dockerfile) {
    dockerfile = filepath.Join(abs, dockerfile)
}
```

Then change line 88:

```go
        Dockerfile:       f.Dockerfile,
```

to:

```go
        Dockerfile:       dockerfile,
```

That is the entire production change for Finding 1.

**Why the empty-string guard is not vestigial.** Cobra's default makes
`f.Dockerfile` `"Dockerfile"` unless the user passed `--dockerfile`. But a
user *can* pass `--dockerfile=""` from the shell, and Cobra accepts it.
Without the guard we would send `filepath.Join(abs, "")` which equals `abs`
— a directory, not a file. Docker would then emit a confusing "is a
directory" error. The defensive `dockerfile == ""` => `"Dockerfile"` keeps
the user-typed-empty-string case sane. Six lines total. No new helper,
no new exported surface.

### What does NOT change for Finding 1

- `internal/dockerdrv/cli_driver.go` — untouched. `dockerdrv` stays a thin
  shell around `docker build`. Argument vector is unchanged: `[build, -t,
  ref, -f, <whatever>, <sourceDir>]`. The existing `TestCLIDriver_BuildArgs`
  must continue to pass with `Dockerfile: "Dockerfile"` because it asserts
  the literal `-f Dockerfile`. Rob does not touch that test or that code.
- `internal/deploy/service.go` — untouched. It blindly forwards
  `req.Dockerfile`.
- The `--dockerfile` flag default in `newDeployServiceCmd` (line 61) —
  untouched. The flag default `"Dockerfile"` is the right user-visible
  contract.

### Tests Kent must write — Finding 1

All in `internal/cli/deploy_service_test.go`. Use the existing
`installMockDeployer`/`runRoot` harness. Capture the `deploy.Request` via
`DoAndReturn`, exactly as `TestDeployService_BuildsExpectedRequest`
(lines 47-72) already does.

The four tests, all single-test functions (no table-driven required —
each has different setup):

#### Test 1: `TestDeployService_DefaultDockerfileIsJoinedWithSourceDir`

```go
func TestDeployService_DefaultDockerfileIsJoinedWithSourceDir(t *testing.T) {
    mock := installMockDeployer(t)
    var got deploy.Request
    mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
        DoAndReturn(func(_ context.Context, req deploy.Request) error {
            got = req
            return nil
        })

    sourceDir := t.TempDir()
    _, _, err := runRoot(t, "deploy", "service",
        "--name", "foo",
        "--port", "8080",
        sourceDir,
    )
    require.NoError(t, err)
    assert.Equal(t, filepath.Join(sourceDir, "Dockerfile"), got.Dockerfile)
}
```

Note the `--port 8080`: required because Finding 3's new validation will
reject any deploy without `--port`. **Every new Finding 1 test must include
`--port 8080`.**

#### Test 2: `TestDeployService_RelativeDockerfileIsJoinedWithSourceDir`

```go
func TestDeployService_RelativeDockerfileIsJoinedWithSourceDir(t *testing.T) {
    mock := installMockDeployer(t)
    var got deploy.Request
    mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
        DoAndReturn(func(_ context.Context, req deploy.Request) error {
            got = req
            return nil
        })

    sourceDir := t.TempDir()
    _, _, err := runRoot(t, "deploy", "service",
        "--name", "foo",
        "--port", "8080",
        "--dockerfile", "docker/prod.Dockerfile",
        sourceDir,
    )
    require.NoError(t, err)
    assert.Equal(t, filepath.Join(sourceDir, "docker", "prod.Dockerfile"), got.Dockerfile)
}
```

#### Test 3: `TestDeployService_AbsoluteDockerfileIsPreserved`

```go
func TestDeployService_AbsoluteDockerfileIsPreserved(t *testing.T) {
    mock := installMockDeployer(t)
    var got deploy.Request
    mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
        DoAndReturn(func(_ context.Context, req deploy.Request) error {
            got = req
            return nil
        })

    abs := "/etc/shared/X.Dockerfile"
    _, _, err := runRoot(t, "deploy", "service",
        "--name", "foo",
        "--port", "8080",
        "--dockerfile", abs,
        t.TempDir(),
    )
    require.NoError(t, err)
    assert.Equal(t, abs, got.Dockerfile)
}
```

This test does NOT require the absolute path to actually exist — we never
stat it, we never open it, we just record the request and the mock returns
nil. The `t.TempDir()` for `<source-dir>` exists; Cobra is happy.

#### Test 4: `TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved`

This is the **regression test for the bug**. Run from a working directory
that is NOT the source dir.

```go
func TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved(t *testing.T) {
    mock := installMockDeployer(t)
    var got deploy.Request
    mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
        DoAndReturn(func(_ context.Context, req deploy.Request) error {
            got = req
            return nil
        })

    parent := t.TempDir()
    sub := filepath.Join(parent, "svc")
    require.NoError(t, os.MkdirAll(sub, 0o755))

    origCwd, err := os.Getwd()
    require.NoError(t, err)
    require.NoError(t, os.Chdir(parent))
    t.Cleanup(func() { _ = os.Chdir(origCwd) })

    _, _, err = runRoot(t, "deploy", "service",
        "--name", "foo",
        "--port", "8080",
        "./svc",
    )
    require.NoError(t, err)
    assert.True(t, filepath.IsAbs(got.Dockerfile),
        "Dockerfile must be absolute, got %q", got.Dockerfile)
    assert.Equal(t, filepath.Join(sub, "Dockerfile"), got.Dockerfile)
}
```

Note: `os.Getwd`/`os.Chdir` are **process-global state**. The package-level
constraint already documented in `deploy_service.go:22-24` (no `t.Parallel()`
in `internal/cli` tests due to `deployerFactory` global) covers us here too.
Kent: do NOT call `t.Parallel()` in this test. Use `t.Cleanup` to restore
cwd unconditionally even on test failure.

### Why these four tests, not three or five

- Test 1 covers the default-flag path (the most common user invocation).
- Test 2 covers an explicit relative path containing a subdirectory
  (catches a regression where someone might `filepath.Base` the input).
- Test 3 covers the absolute-path passthrough (catches a regression where
  someone might unconditionally join, breaking shared-Dockerfile workflows).
- Test 4 reproduces the bug from the user request: a relative source dir
  invoked from outside the source dir. Without this test, the diff is
  technically correct but the regression test against the original bug is
  missing.

Three tests would skip the cwd-relative case. Five would add a
`..`-segment case which Don explicitly punted (filepath.Join cleans `..`,
Docker enforces build-context, not our problem). Stick to four.

### Tests that must keep passing unchanged for Finding 1

- `TestCLIDriver_BuildArgs` (`internal/dockerdrv/cli_driver_test.go:49-67`).
  Driver semantics unchanged. The literal `-f Dockerfile` assertion stands.
- `TestDeployService_BuildsExpectedRequest`
  (`internal/cli/deploy_service_test.go:46-72`). Asserts SourceDir suffix
  and other fields, does NOT assert `Dockerfile`. Will pass after change.
  (`/srv/foo` does not need to exist; `os.Stat` of `/srv/foo/env.sh`
  returns ENOENT silently per `resolveEnvFile`.)

---

## 2. Finding 2 — `--config-root` does not affect log placement

### Production diffs (Rob)

#### Diff A — `internal/logging/logging.go`

Change the function signature and body. Replace lines 21-26:

```go
func Init() error {
    if os.Getenv("DECLOUD_LOG_TO_STDERR_ONLY") == "1" {
        setStderrOnly()
        return nil
    }
    root := config.RootFromEnv()
```

with:

```go
func Init(root string) error {
    if os.Getenv("DECLOUD_LOG_TO_STDERR_ONLY") == "1" {
        setStderrOnly()
        return nil
    }
    if root == "" {
        root = config.DefaultRoot
    }
```

The rest of the function (lines 27-43) is unchanged.

Update the doc comment block (lines 13-20) to reflect the new contract.
Replace it with:

```go
// Init configures the default slog handler. JSON output goes to stderr and,
// when filesystem access permits, also to <root>/logs/decloud.log. If root
// is the empty string, config.DefaultRoot is used (matching config.NewPaths
// semantics). If the log directory cannot be created OR the log file cannot
// be opened, Init falls back to stderr-only and emits one warning line to
// stderr describing the cause.
//
// DECLOUD_LOG_TO_STDERR_ONLY=1 short-circuits before any filesystem access
// and is the deterministic test escape hatch.
```

The `config` import stays — we now reference `config.DefaultRoot` instead
of `config.RootFromEnv()`. Do NOT remove the import.

#### Diff B — `internal/cli/root.go`

Change line 23 from:

```go
            return logging.Init()
```

to:

```go
            return logging.Init(rc.ConfigRoot)
```

That is the entire production change for Finding 2. No other files change.

### Tests Kent must write/update — Finding 2

#### Update existing `internal/logging/logging_test.go`

All four existing tests must be updated to call `logging.Init(root)`
instead of `logging.Init()`. The `t.Setenv("DECLOUD_ROOT", root)` lines
become irrelevant for the new logic but should be **kept** — they prove
that after the change, the env var is no longer consulted by `Init`.

Specifically:

**`TestInit_StderrOnlyShortCircuit` (lines 13-18):** Pass any string as
the root argument; the short-circuit fires before any access. Use
`"/path/that/cannot/exist/for/test"` to match the existing intent.
Remove the now-irrelevant `t.Setenv("DECLOUD_ROOT", ...)` line — the test
no longer needs to demonstrate the env path is bypassed (a new dedicated
test does that). Final form:

```go
func TestInit_StderrOnlyShortCircuit(t *testing.T) {
    t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "1")
    require.NoError(t, logging.Init("/path/that/cannot/exist/for/test"))
}
```

**`TestInit_DefaultWritesToFileAndStderr` (lines 20-31):** Drop
`t.Setenv("DECLOUD_ROOT", root)`. Pass `root` directly. Final form:

```go
func TestInit_DefaultWritesToFileAndStderr(t *testing.T) {
    root := t.TempDir()
    t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")

    require.NoError(t, logging.Init(root))

    logPath := filepath.Join(root, "logs", "decloud.log")
    info, err := os.Stat(logPath)
    require.NoError(t, err)
    assert.False(t, info.IsDir())
}
```

**`TestInit_PermissionDeniedFallsBackToStderr` (lines 33-46):** Drop
`t.Setenv("DECLOUD_ROOT", root)`. Pass `root` directly. Otherwise unchanged.

**`TestInit_LogFileOpenFailureFallsBackToStderr` (lines 48-62):** Drop
`t.Setenv("DECLOUD_ROOT", root)`. Pass `root` directly. Otherwise unchanged.

#### Add new `TestInit_UsesPassedRootNotEnv`

This locks in the behavior change: `DECLOUD_ROOT` is **no longer consulted
by `Init`**. The flag wins because the caller decided what root to pass.

```go
func TestInit_UsesPassedRootNotEnv(t *testing.T) {
    t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")
    t.Setenv("DECLOUD_ROOT", "/path/that/must/not/be/written/to")

    root := t.TempDir()
    require.NoError(t, logging.Init(root))

    logPath := filepath.Join(root, "logs", "decloud.log")
    _, err := os.Stat(logPath)
    require.NoError(t, err, "log must be written under the passed root, not env")

    // Negative assertion: the env path must not exist as a directory.
    _, err = os.Stat("/path/that/must/not/be/written/to")
    assert.True(t, os.IsNotExist(err),
        "DECLOUD_ROOT must not be consulted by Init when a root is passed")
}
```

Note: the negative-existence check on `/path/that/must/not/be/written/to`
is belt-and-suspenders. It catches any future regression where someone
sneaks `os.MkdirAll(config.RootFromEnv()...)` back in.

#### Add new `TestInit_EmptyStringRootFallsBackToDefault`

Locks in the empty-string => `DefaultRoot` mapping. We cannot exercise the
real `/opt/declouding` path from a unit test, so use the stderr-only
short-circuit to prove `Init("")` does not panic and reaches the empty-string
branch without crashing.

```go
func TestInit_EmptyStringRootFallsBackToDefault(t *testing.T) {
    t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "1")
    require.NoError(t, logging.Init(""))
}
```

This test is intentionally weak — it only verifies the empty-string code
path does not panic. The full `config.DefaultRoot` write path is not
exercisable in a unit test. That is fine; the equivalence with
`config.NewPaths` semantics is the real contract, and `config.NewPaths`
has its own tests at `internal/config/paths_test.go`.

#### Add new `TestRoot_ConfigRootFlagControlsLogPlacement`

In `internal/cli/root_test.go`. End-to-end test that proves the Finding 2
fix works through the full CLI tree.

```go
func TestRoot_ConfigRootFlagControlsLogPlacement(t *testing.T) {
    envRoot := t.TempDir()
    flagRoot := t.TempDir()
    t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")
    t.Setenv("DECLOUD_ROOT", envRoot)

    captureConfigRoot(t,
        "--config-root", flagRoot,
        "deploy", "service",
        "--name", "foo",
        "--port", "8080",
        t.TempDir(),
    )

    flagLog := filepath.Join(flagRoot, "logs", "decloud.log")
    _, err := os.Stat(flagLog)
    require.NoError(t, err, "log must be at flagRoot/logs/decloud.log")

    envLog := filepath.Join(envRoot, "logs", "decloud.log")
    _, err = os.Stat(envLog)
    assert.True(t, os.IsNotExist(err),
        "log must NOT be at envRoot/logs/decloud.log")
}
```

This test reuses `captureConfigRoot` (root_test.go:18-36) which already
installs a mock deployer. The `t.TempDir()` source dir does not need a
Dockerfile — the deployer is mocked. The `--port 8080` is required by the
new Finding 3 validation.

### Tests that must keep passing unchanged for Finding 2

- `TestRoot_ConfigRootDefaultsToDecloudRootEnv` (root_test.go:38-44).
  After Rob's change, `logging.Init("/tmp/from-env")` runs; that creates
  `/tmp/from-env/logs/decloud.log`. The test does not assert anything
  about logs — it asserts `seenRoot == "/tmp/from-env"`. Still passes.
- `TestRoot_ConfigRootFlagOverridesEnv` (root_test.go:46-53). Same logic:
  flag wins, that's what we already do, no change.
- `TestRoot_HelpDoesNotRequireFilesystem` (root_test.go:55-65). Cobra
  skips `PersistentPreRunE` on `--help`, so `logging.Init` is never called.
  No regression risk.

### Side-effect note (no test required)

After the change, `TestRoot_ConfigRootDefaultsToDecloudRootEnv` and
`TestRoot_ConfigRootFlagOverridesEnv` will write real log files to
`/tmp/from-env/logs/decloud.log` and `/tmp/from-flag/logs/decloud.log`.
This is **not new** — they were already writing those files via the
old `RootFromEnv()` path. The `t.Setenv` calls limit blast radius to
the test process. No cleanup needed; `/tmp` is OS-managed.

If a future maintainer wants to make those tests hermetic, the right
move is `t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "1")`. That is **not in
scope for this task**.

---

## 3. Finding 3 — Deploy without `--port` and without `--host` passes validation

### Production diff (Rob)

**File:** `internal/cli/deploy_service.go`, function `runDeployService`.

Replace lines 73-75:

```go
    if len(f.Hosts) > 0 && f.Port == 0 {
        return fmt.Errorf("--port is required when --host is set: %w", errUsage)
    }
```

with:

```go
    if f.Port == 0 {
        return fmt.Errorf("--port is required (services must expose an HTTP port for the readiness probe): %w", errUsage)
    }
```

That is the entire production change for Finding 3.

The new check fires earlier in the validation chain than the old
host-without-port check (because `f.Port == 0` is a strict superset).
The error message specifically names `--port`, satisfies Don's specified
text, and wraps `errUsage` so `ExitCodeFor` returns `ExitUsageError` (2).

### What does NOT change for Finding 3

- `internal/deploy/service.go` — line 213 stays
  `d.probe.Wait(ctx, containerName, spec, req.Port)`. The deployer is
  correct GIVEN the contract. We enforce the contract at the boundary.
- `internal/deploy/readiness.go` — unchanged.
- `internal/cli/deploy_service.go:55` — `--port` flag stays `IntVar` with
  default `0`. We do NOT use `cmd.MarkFlagRequired("port")` because
  Cobra's required-flag detection only fires when the flag is unset on
  the command line; it cannot distinguish "user passed `--port=0`" from
  "user did not pass `--port`". Our explicit `f.Port == 0` check covers
  both cases uniformly.

### Tests Kent must write/update — Finding 3

#### Add `TestDeployService_NoPortReturnsExitUsageError`

```go
func TestDeployService_NoPortReturnsExitUsageError(t *testing.T) {
    installMockDeployer(t)
    _, _, err := runRoot(t,
        "deploy", "service",
        "--name", "foo",
        t.TempDir(),
    )
    require.Error(t, err)
    assert.True(t, errors.Is(err, errUsage))
    assert.Equal(t, ExitUsageError, ExitCodeFor(err))
}
```

#### Add `TestDeployService_PortZeroExplicitReturnsExitUsageError`

```go
func TestDeployService_PortZeroExplicitReturnsExitUsageError(t *testing.T) {
    installMockDeployer(t)
    _, _, err := runRoot(t,
        "deploy", "service",
        "--name", "foo",
        "--port", "0",
        t.TempDir(),
    )
    require.Error(t, err)
    assert.True(t, errors.Is(err, errUsage))
    assert.Equal(t, ExitUsageError, ExitCodeFor(err))
}
```

Belt-and-suspenders: prevents a future "treat 0 as unset" regression.

#### Update `TestDeployService_HostWithoutPortReturnsExitUsageError`
(lines 107-117)

The test still passes after the diff because `--host x` without `--port`
implies `f.Port == 0`. **No change required.** The error message text
will differ (now names `--port` directly instead of "when --host is
set"), but the test asserts only `ExitUsageError` from `ExitCodeFor`.
Still green.

#### Update `TestDeployService_DefaultStrategyIsRecreate` (lines 119-131)

Currently invokes `deploy service --name foo /srv/foo` with no `--port`.
After the new rule it will fail. Add `--port 8080`:

```go
_, _, err := runRoot(t,
    "deploy", "service",
    "--name", "foo",
    "--port", "8080",
    "/srv/foo",
)
```

The intent of the test (default strategy is `recreate`) is unchanged.

#### Update `TestDeployService_AutoDiscoversEnvSh` (lines 133-149)

Same fix — add `--port 8080`. Otherwise unchanged.

#### Update `TestDeployService_NoEnvShIsValid` (lines 151-165)

Same fix — add `--port 8080`. Otherwise unchanged.

#### Update `TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError`
(lines 167-179)

Currently has no `--port`. The test exits with `ExitConfigError` (10) for
the missing env file, but **the new `--port` check fires FIRST**, which
would change the exit code from `ExitConfigError` to `ExitUsageError`.
That is a regression in test intent.

**Fix:** add `--port 8080` so the `--port` check passes and the env-file
check still fires. Final form:

```go
_, _, err := runRoot(t,
    "deploy", "service",
    "--name", "foo",
    "--port", "8080",
    "--env-file", "/no/such/file",
    t.TempDir(),
)
```

This is critical; Kent must not miss it. Without the fix, the test
regresses from "asserts envcap error path" to "asserts port-missing error
path" while keeping its old name. That would be a silent behavior loss.

#### Update `TestDeployService_MissingNameReturnsExitUsageError` (lines 74-79)

This test currently has no `--port` and no `--name`. The Cobra-level
required-flag check on `--name` (declared via `cmd.MarkFlagRequired("name")`
at line 62) fires during `cmd.Execute` argument parsing — that is **before
RunE**. Our new `f.Port == 0` check is in RunE. So the Cobra check fires
first and the test still exits with `ExitUsageError` (mapped via the
`isCobraUsageError` substring match in `exit_codes.go:69-79`). **No
change required.**

But add a sanity comment for the next maintainer if you wish. Kent's
call.

#### Update `TestDeployService_MountFlagReturnsErrMountsNotSupported`
(lines 81-92)

Currently has no `--port`. The mount check at `deploy_service.go:67-69`
fires **before** the new port check (line ordering matters). Test still
passes. **No change required.** Same pattern as `TestDeployService_StrategyBlueGreenReturnsErrInvalidStrategy`.

### Test that DOES need careful regression check

`TestDeployService_BuildsExpectedRequest` (lines 46-72) already passes
`--port 8080`. It is the canonical happy-path test for this command.
**No change required.** Verify Kent does not accidentally break it.

### Order-of-validation requirement

The validation block in `runDeployService` (after Rob's diffs) becomes:

```
1. Mount check       (line 67-69)  → ErrMountsNotSupported  → ExitConfigError (10)
2. Strategy check    (line 70-72)  → ErrInvalidStrategy     → ExitConfigError (10)
3. Port check        (line 73-75)  → errUsage               → ExitUsageError (2)
4. filepath.Abs      (line 76-79)  → wrapped FS error       → ExitInternal (70)
5. resolveEnvFile    (line 80-83)  → ErrEnvScriptMissing... → ExitConfigError (10)
6. Dockerfile resolve(NEW, see §1) → no error path
7. construct request (line 84+)
```

Rob: do NOT reorder. The mount/strategy checks must stay above the
port check so that an obviously-bad `--mount` or `--strategy` value
still surfaces its specific error code. The port check stays above
filepath.Abs because we want fast user feedback before any system call.

---

## 4. Files that change

### Production (Rob)

| File | Lines | Finding |
|---|---|---|
| `internal/cli/deploy_service.go` | replace 73-75; insert 6 lines after 79; change 88 | F1 + F3 |
| `internal/cli/root.go` | line 23 | F2 |
| `internal/logging/logging.go` | doc comment 13-20; signature 21; body 22-26 | F2 |

### Tests (Kent — BEFORE Rob)

| File | Action | Tests touched |
|---|---|---|
| `internal/cli/deploy_service_test.go` | add 4 + 2 new tests; update 4 fixtures | F1 (new), F3 (new), F3 (fixture updates) |
| `internal/logging/logging_test.go` | sig-update 4 tests; add 2 new tests | F2 |
| `internal/cli/root_test.go` | add 1 new test | F2 |

### Docs (Raymond — AFTER Rob)

| File | Section | Change |
|---|---|---|
| `_docs/usage.md` | §2 table row `--port` | Required column: `only if --host is set` → `yes` |
| `_docs/usage.md` | §2 table row `--dockerfile` | Notes: append "Relative paths resolve under `<source-dir>`. Absolute paths are used as-is." |
| `_docs/usage.md` | §2 table row `--config-root` | Notes: append (one short sentence) "Logs are written to `<config-root>/logs/decloud.log`." |

No new files created. No files renamed or deleted.

### Files that intentionally do NOT change

- `internal/dockerdrv/cli_driver.go` — driver stays a thin shell.
- `internal/deploy/service.go` — deployer is correct given contracts.
- `internal/deploy/readiness.go` — probe is correct given contracts.
- `internal/registry/types.go` and store_test.go — no schema change.
- `cmd/decloud/main.go` — does not touch logging directly.
- `_ai/*.md` — no API surface change worth recording at AI-doc layer.
  Ward decides at FINALIZATION whether anything from this task is worth
  preserving as a learning. Raymond does not touch `_ai/`.

---

## 5. "Here be dragons" — gotchas Kent and Rob must not miss

### G1. Test parallelism is forbidden in `internal/cli`

`deploy_service.go:22-24` documents that `deployerFactory` is a
package-global test seam. Existing tests do not call `t.Parallel()`.
**New tests MUST NOT call `t.Parallel()`.** This applies especially to
`TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved`
(which mutates cwd) and to all six new deploy-service tests.

### G2. `os.Chdir` test discipline

The cwd-relative test (Test 4 in §1) calls `os.Chdir(parent)`. Use
`t.Cleanup` with `_ = os.Chdir(origCwd)` (ignoring the error is correct
here — if cleanup fails the test process is doomed anyway and `t.Cleanup`
is best-effort). Restore cwd BEFORE other tests run. Since this file
disallows `t.Parallel`, sequential execution makes restoration safe.

### G3. `--port` placement in updated fixtures

When adding `--port 8080` to existing tests, place it BEFORE the
`<source-dir>` positional argument. Cobra consumes flags greedily until
it sees a positional, but using the conventional `--flag value` style
keeps argv readable and matches existing test style.

### G4. `errUsage` is unexported

`errors.Is(err, errUsage)` works only because the test file is `package
cli`, not `package cli_test`. Kent: keep the new tests in
`internal/cli/deploy_service_test.go` exactly as-is. Do not "promote" to
`cli_test` for hygiene — that would silently demote the assertion.

### G5. `filepath.Join` cleans `..`

`filepath.Join("/a/b", "../c")` returns `/a/c`. Don explicitly punted
this case: Docker rejects "Forbidden path outside the build context" and
the deploy fails at exit 30 (`ErrBuild`). We do not pre-validate. Kent:
do NOT write a test for `--dockerfile=../foo`. That's an integration test
against real `docker build` and is out of scope for unit tests.

### G6. `filepath.IsAbs` is platform-aware

On Linux/macOS `IsAbs("/foo")` is true and `IsAbs("foo")` is false. On
Windows `IsAbs("C:\\foo")` is true. The `t.TempDir()` returns
platform-appropriate absolute paths, so the regression test (Test 4) is
portable. Rob's `if !filepath.IsAbs(dockerfile)` is correct on all
platforms `decloud` supports.

### G7. Env var contract is "the env IS the default"

Don's plan §"Edge cases I'm explicitly NOT addressing" notes that
`--config-root` flag default is `config.RootFromEnv()` (root.go:26).
That means **before** the flag is parsed, `rc.ConfigRoot` is already the
env value; if the user passes `--config-root`, Cobra overwrites it.
After Rob's change, `logging.Init(rc.ConfigRoot)` sees this resolved
value. The flag-wins-over-env semantics are preserved at the Cobra
layer, not in `logging.Init`. **Do NOT add an env-fallback inside
`logging.Init`.** That would be redundant and would defeat the
flag-controls-everything design.

### G8. Empty `f.Dockerfile` defensive guard is load-bearing

Without `if dockerfile == "" { dockerfile = "Dockerfile" }`,
`filepath.Join(abs, "")` returns `abs` (a directory). Docker would emit
a confusing "is a directory" error. Keep the guard.

### G9. Test ordering is a property of the test file

Go test execution within a single package runs tests in the order they
appear in the source unless `-shuffle=on`. Kent should place the new
tests at the bottom of `deploy_service_test.go` to minimize churn for
reviewers.

### G10. The four updated logging tests must keep `t.Setenv("DECLOUD_ROOT", ...)`

After the signature change, `logging.Init` no longer reads `DECLOUD_ROOT`.
But removing the `t.Setenv` lines from the four existing tests would
hide a regression: if someone re-introduces `RootFromEnv()` in `Init`,
the `TestInit_UsesPassedRootNotEnv` test would catch it, but the
four legacy tests would silently start using whatever the developer's
env had. **Keep the `t.Setenv("DECLOUD_ROOT", ...)` calls in the four
existing tests** — they document "this env var must not affect Init".

Wait — re-reading my own §2 update instructions: I said "drop
`t.Setenv("DECLOUD_ROOT", root)`" in three of the four tests. Let me
reconsider.

The intent of the existing tests is to exercise filesystem behavior
under a controlled root. After the change, the controlled root is the
function argument, not the env var. **The `t.Setenv` calls become dead
weight** in those three tests. Removing them is the cleaner choice.
The dedicated `TestInit_UsesPassedRootNotEnv` test is where we lock in
"env is ignored". One test owns one assertion.

**Final decision: drop `t.Setenv("DECLOUD_ROOT", root)` from the three
filesystem tests; keep it (or rather, leave the original) in the
short-circuit test. Add `TestInit_UsesPassedRootNotEnv` as the dedicated
guardian of the env-is-ignored contract.** This is what §2 says above;
G10 was a momentary self-doubt. Kent: follow §2.

---

## 6. Kent's red-bar evidence — what failure looks like before Rob

For each new test, Kent's report MUST state the exact failure mode
before Rob applies the production diffs. This is how we know the test
catches the bug.

| Test | Expected failure mode before fix |
|---|---|
| `TestDeployService_DefaultDockerfileIsJoinedWithSourceDir` | `assert.Equal` mismatch: got `"Dockerfile"`, want `<tempdir>/Dockerfile` |
| `TestDeployService_RelativeDockerfileIsJoinedWithSourceDir` | `assert.Equal` mismatch: got `"docker/prod.Dockerfile"`, want `<tempdir>/docker/prod.Dockerfile` |
| `TestDeployService_AbsoluteDockerfileIsPreserved` | passes vacuously today (no join); document that it begins to provide regression coverage AFTER the fix |
| `TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved` | `assert.True(filepath.IsAbs(...))` fails: got `"Dockerfile"`, not absolute |
| `TestDeployService_NoPortReturnsExitUsageError` | `require.Error` fails: deploy succeeds and reaches the mocked deployer |
| `TestDeployService_PortZeroExplicitReturnsExitUsageError` | same as above |
| `TestInit_UsesPassedRootNotEnv` | log written to env path, not passed path; either the positive `os.Stat(logPath)` fails OR the negative existence check fails |
| `TestInit_EmptyStringRootFallsBackToDefault` | passes vacuously today (the short-circuit fires regardless); after sig change must compile with the new signature |
| `TestRoot_ConfigRootFlagControlsLogPlacement` | log appears under env root, not flag root; the negative `os.IsNotExist` assertion fails |

The four sig-updated logging tests fail to **compile** before Rob's
signature change because they call `logging.Init(root)` against the
old `Init() error` signature. That is the correct red bar for those
tests — compile failure is the hardest possible bar.

The four updated deploy-service fixture tests will **fail at runtime**
before Rob's port-validation diff, because they now pass `--port 8080`
which bypasses the (still-old, host-without-port-only) check. Wait —
they would PASS before Rob's diff, because the old check only fires
on `host && !port`. Adding `--port 8080` is forward-compatible.

So the right order is:

1. Kent commits the new tests AND the fixture updates.
2. Run `go test ./...` BEFORE Rob touches production. Expect:
   - 4 logging tests fail to compile (signature mismatch).
   - 6 new deploy-service tests fail at runtime (red).
   - 4 updated fixture tests pass (green; forward-compatible).
   - All other tests pass.
3. Kent's report records this exact split. Donald Knuth is on call
   if any test fails for a reason other than the documented one.
4. Rob applies the three production diffs.
5. Run `go test ./...`. All green. Rob's report confirms.

---

## 7. Simplification opportunities (worth noting, not in scope)

Things I considered and decided NOT to do, so nobody asks later:

### S1. Make `--port` Cobra-required

Cobra's `MarkFlagRequired("port")` would let us delete the explicit
RunE check. But Cobra cannot distinguish `--port` unset from
`--port=0`, so we would still need an explicit check OR live with the
risk that someone passes `--port=0` and gets through. Don's choice
(explicit check, no MarkFlagRequired) is more robust.

### S2. Push dockerfile resolution into `dockerdrv`

Tempting (single source of truth), but couples a generic Docker driver
to a CLI-layer convention. The driver should accept whatever `-f` value
it is given. CLI owns the contract. Don is right.

### S3. Add `--quiet` to suppress logging in tests

Would hermetic-ize the cwd-relative test. Out of scope; the existing
`DECLOUD_LOG_TO_STDERR_ONLY=1` escape hatch is fine. If hermetic tests
become important later, that's a separate task.

### S4. Read `DECLOUD_ROOT` inside `logging.Init` as a fallback

Would let callers omit the argument and "just work". But it would
defeat the explicit-input contract Don is establishing, and we already
have flag-wins-over-env at the Cobra layer. Adding a second layer of
env-reading is exactly the kind of "convenience" that breeds subtle
bugs. Hard pass.

### S5. Wrap the new validation in a separate helper

Two `f.Port == 0` checks in `runDeployService` would justify a helper.
We have one. No helper. YAGNI.

---

## 8. Where I disagree with Don (none material)

I read Don's plan three times. I cannot find a substantive disagreement.
Two minor refinements I made above which Don did not specify:

**R1.** Don's plan does not call out that the
`TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError` test
needs `--port 8080` added (he listed three other tests but not this
one). I added it in §3 above. Without that fix, that test would
silently change which assertion it's actually verifying — a behavior
loss, not a behavior break. Kent: do NOT skip this fixture update.

**R2.** Don wrote `if dockerfile == "" { dockerfile = "Dockerfile" }`
without saying why. I documented the rationale (Cobra accepts
`--dockerfile=""` from the shell; without the guard,
`filepath.Join(abs, "")` returns `abs`). Defensive; small; correct.

Neither is a disagreement on architecture, exit codes, layering, or
test strategy. The plan is technically sound. Linus should sign off.

---

## 9. Acceptance criteria (echoing Don's, with my numeric specifics)

A successful task ends with all of:

- [ ] `go test ./...` reports 0 failures.
- [ ] `go vet ./...` reports nothing new.
- [ ] `gofmt -l internal/ cmd/` is empty.
- [ ] The three production diffs in §4 are the only production changes.
- [ ] Six new tests in `internal/cli/deploy_service_test.go` (four for
      Finding 1, two for Finding 3) all green AFTER Rob's diff and all
      red BEFORE Rob's diff (per §6 table).
- [ ] Two new tests in `internal/logging/logging_test.go` (passed-root
      precedence; empty-string fallback) all green.
- [ ] One new test in `internal/cli/root_test.go` (config-root flag
      controls log placement) all green.
- [ ] Four existing tests in `internal/logging/logging_test.go` updated
      to the new signature; all green.
- [ ] Four existing tests in `internal/cli/deploy_service_test.go`
      updated to include `--port 8080`; all green; intent unchanged.
- [ ] `_docs/usage.md` updated per §4.
- [ ] No changes to `internal/dockerdrv/`, `internal/deploy/`,
      `internal/registry/`, `cmd/decloud/`, or `_ai/`.
- [ ] Manual smoke: `decloud --config-root /tmp/X deploy service ...`
      writes logs to `/tmp/X/logs/decloud.log`, never
      `/opt/declouding/logs/decloud.log`.
- [ ] Manual smoke: `decloud deploy service --name foo ./bar` from any
      cwd builds with `-f /abs/path/to/bar/Dockerfile`.
- [ ] Manual smoke: `decloud deploy service --name foo ./bar` (no
      `--port`) exits 2 with a message naming `--port`.

---

## 10. Why this plan ships

Three defects, three fixes, three layers. Each fix lives where its
contract is documented:

- The CLI layer enforces CLI-shaped contracts (relative-path resolution,
  required flags). Six lines for Finding 1, two lines for Finding 3.
- The logging package takes its inputs explicitly. Three lines diff.
- The Docker driver stays a dumb shell around `docker build`. Zero
  lines.

Eleven production lines of net change. Twelve test functions touched
(seven new, five updated). One doc page touched. No new abstractions.
No new flags. No new files. No new packages.

That is what a small correct fix looks like.
