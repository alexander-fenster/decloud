# Kent's test report: red bar established for all three findings

I executed Joel's tech plan exactly as written. All new tests fail in
the documented way and all updated fixture tests pass forward-compatibly.
Rob is unblocked.

---

## Files touched

### `internal/cli/deploy_service_test.go`

- **Updated four existing fixtures to add `--port 8080`** (forward-compatible
  with Rob's pending Finding 3 diff):
  - `TestDeployService_DefaultStrategyIsRecreate`
  - `TestDeployService_AutoDiscoversEnvSh`
  - `TestDeployService_NoEnvShIsValid`
  - `TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError`
    (Joel's R1 catch — without `--port 8080` this test would have silently
    rotted from "asserts envcap error" to "asserts port-missing error".)

- **Added six new tests at the bottom of the file** (per gotcha G9):
  - `TestDeployService_DefaultDockerfileIsJoinedWithSourceDir` — Finding 1
  - `TestDeployService_RelativeDockerfileIsJoinedWithSourceDir` — Finding 1
  - `TestDeployService_AbsoluteDockerfileIsPreserved` — Finding 1
  - `TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved` — Finding 1 (regression test for the original bug; mutates cwd with `t.Cleanup` restore per G2)
  - `TestDeployService_NoPortReturnsExitUsageError` — Finding 3
  - `TestDeployService_PortZeroExplicitReturnsExitUsageError` — Finding 3

- **Did not touch** (verified still pass after changes):
  - `TestDeployService_BuildsExpectedRequest` — already passes `--port 8080`
  - `TestDeployService_MissingNameReturnsExitUsageError` — Cobra required-flag check fires first
  - `TestDeployService_MountFlagReturnsErrMountsNotSupported` — mount check fires before port check
  - `TestDeployService_StrategyBlueGreenReturnsErrInvalidStrategy` — strategy check fires before port check
  - `TestDeployService_HostWithoutPortReturnsExitUsageError` — `--host x` without `--port` still has port=0; new check is a strict superset

### `internal/logging/logging_test.go`

- **Updated all four existing tests** to call `logging.Init(root)` instead
  of `logging.Init()`. Dropped the now-irrelevant `t.Setenv("DECLOUD_ROOT", ...)`
  lines from the three filesystem tests, per Joel's §2 / G10 final decision.
  The dedicated `TestInit_UsesPassedRootNotEnv` is the single guardian of
  the env-is-ignored contract.

- **Added two new tests**:
  - `TestInit_UsesPassedRootNotEnv` — passes a TempDir as root and points
    `DECLOUD_ROOT` at `/path/that/must/not/be/written/to`. Locks in that
    `Init` does not consult the env var. Includes belt-and-suspenders
    negative existence check on the env path.
  - `TestInit_EmptyStringRootFallsBackToDefault` — uses the
    `DECLOUD_LOG_TO_STDERR_ONLY=1` short-circuit to prove `Init("")` does
    not panic (cannot exercise the real `/opt/declouding` path from a unit
    test, as Joel notes).

### `internal/cli/root_test.go`

- **Updated two existing tests** to add `--port 8080` to the args
  (forward-compatible with Rob's Finding 3 diff):
  - `TestRoot_ConfigRootDefaultsToDecloudRootEnv`
  - `TestRoot_ConfigRootFlagOverridesEnv`

- **Added one new test**:
  - `TestRoot_ConfigRootFlagControlsLogPlacement` — end-to-end test that
    proves the Finding 2 fix works through the full CLI tree. Asserts
    that `--config-root B` writes logs under `B`, not under
    `DECLOUD_ROOT=A`. Per Linus's note in `04-linus-review.md`, I added
    a one-line comment above the function explaining that the test
    relies on Cobra's flag-default-from-env mechanism in `root.go`.

- **Added imports** for `os` and `path/filepath` (used by the new test).

---

## Red-bar evidence (output from `go test ./...` BEFORE Rob applies fixes)

### A. Compile failure in `internal/logging` (4 sig-updated + 2 new tests)

```
# github.com/alexander-fenster/decloud/internal/logging_test [github.com/alexander-fenster/decloud/internal/logging.test]
internal/logging/logging_test.go:15:34: too many arguments in call to logging.Init
	have (string)
	want ()
internal/logging/logging_test.go:22:34: too many arguments in call to logging.Init
	have (string)
	want ()
internal/logging/logging_test.go:41:34: too many arguments in call to logging.Init
	have (string)
	want ()
internal/logging/logging_test.go:56:34: too many arguments in call to logging.Init
	have (string)
	want ()
internal/logging/logging_test.go:64:34: too many arguments in call to logging.Init
	have (string)
	want ()
internal/logging/logging_test.go:77:34: too many arguments in call to logging.Init
	have (string)
	want ()
FAIL	github.com/alexander-fenster/decloud/internal/logging [build failed]
```

Compile failure is the hardest red bar, exactly as Joel specified in §6.
Rob's signature change (`Init() error` -> `Init(root string) error`) makes
all six call sites compile and their assertions pass.

### B. Runtime failure in `internal/cli` (matches §6 table line-for-line)

| Test | Observed failure | Plan's prediction |
|---|---|---|
| `TestDeployService_DefaultDockerfileIsJoinedWithSourceDir` | got `"Dockerfile"`, want `<tempdir>/Dockerfile` | `assert.Equal` mismatch ✓ |
| `TestDeployService_RelativeDockerfileIsJoinedWithSourceDir` | got `"docker/prod.Dockerfile"`, want `<tempdir>/docker/prod.Dockerfile` | `assert.Equal` mismatch ✓ |
| `TestDeployService_AbsoluteDockerfileIsPreserved` | passes vacuously (no join happens today) | passes vacuously ✓ |
| `TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved` | `assert.True(filepath.IsAbs("Dockerfile"))` fails — got `"Dockerfile"`, not absolute | matches ✓ |
| `TestDeployService_NoPortReturnsExitUsageError` | `require.Error` fails — deploy succeeds and reaches the mocked deployer (gomock raises "no expected calls") | matches ✓ |
| `TestDeployService_PortZeroExplicitReturnsExitUsageError` | same as above | matches ✓ |
| `TestRoot_ConfigRootFlagControlsLogPlacement` | `os.Stat(flagRoot/logs/decloud.log)` returns ENOENT — log went somewhere else (in fact `/opt/declouding`, which mkdir-fails as expected; warning visible in test output) | matches ✓ |

Sample output excerpts:

```
--- FAIL: TestDeployService_DefaultDockerfileIsJoinedWithSourceDir (0.00s)
    deploy_service_test.go:210: 
        Error: Not equal: 
            expected: "/var/folders/.../001/Dockerfile"
            actual  : "Dockerfile"

--- FAIL: TestDeployService_NoPortReturnsExitUsageError (0.00s)
    deploy_service.go:100: Unexpected call to *mocks.MockServiceDeployer.Deploy(
        [context.Background {foo /var/folders/.../001 Dockerfile [] 0  /healthz 1m0s recreate}]
    ) ... because: there are no expected calls of the method "Deploy" for that receiver

--- FAIL: TestRoot_ConfigRootFlagControlsLogPlacement (0.00s)
    root_test.go:89: 
        Error: Received unexpected error:
            stat /var/folders/.../logs/decloud.log: no such file or directory
        Messages: log must be at flagRoot/logs/decloud.log
```

The `decloud: log dir unavailable, using stderr only: mkdir /opt/declouding: permission denied`
warning lines that appear in the test output are pre-existing behavior of
the unfixed `logging.Init()` — every test invocation tries to mkdir
`/opt/declouding` because `Init` ignores the config root entirely. After
Rob's fix, those warnings disappear because `Init(rc.ConfigRoot)` will
mkdir under the test's `t.TempDir()`. This is itself indirect proof of
the bug.

### C. Forward-compatible passes (the fixture updates)

Verified explicitly with a targeted run of the unchanged-intent tests
(updated to include `--port 8080`):

```
--- PASS: TestDeployService_BuildsExpectedRequest
--- PASS: TestDeployService_MissingNameReturnsExitUsageError
--- PASS: TestDeployService_MountFlagReturnsErrMountsNotSupported
--- PASS: TestDeployService_StrategyBlueGreenReturnsErrInvalidStrategy
--- PASS: TestDeployService_HostWithoutPortReturnsExitUsageError
--- PASS: TestDeployService_DefaultStrategyIsRecreate
--- PASS: TestDeployService_AutoDiscoversEnvSh
--- PASS: TestDeployService_NoEnvShIsValid
--- PASS: TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError
--- PASS: TestDeployService_AbsoluteDockerfileIsPreserved
PASS
ok  github.com/alexander-fenster/decloud/internal/cli
```

All ten previously-passing tests stay green after the `--port 8080`
fixture updates. The intent of each test is unchanged; the fixture is
now valid under Rob's pending stricter validation.

### D. Other packages

All other test packages remain green:

```
ok      github.com/alexander-fenster/decloud/internal/caddy
ok      github.com/alexander-fenster/decloud/internal/config
ok      github.com/alexander-fenster/decloud/internal/deploy
ok      github.com/alexander-fenster/decloud/internal/dockerdrv
ok      github.com/alexander-fenster/decloud/internal/envcap
ok      github.com/alexander-fenster/decloud/internal/ids
ok      github.com/alexander-fenster/decloud/internal/registry
```

`internal/dockerdrv` in particular — `TestCLIDriver_BuildArgs` — keeps
its literal `-f Dockerfile` assertion. The driver's contract is
unchanged, exactly as Don and Joel specified.

---

## Discipline notes (for Rob and reviewers)

1. **No `t.Parallel()` was added.** Per gotcha G1, the `internal/cli`
   package mutates `deployerFactory` globally and the cwd-relative test
   mutates `os.Getwd`. All new tests are sequential.

2. **Cwd restoration uses `t.Cleanup`** with `_ = os.Chdir(origCwd)`,
   per gotcha G2. Best-effort restore; if cleanup fails the process is
   doomed anyway.

3. **`errUsage` reference works** because the test file is `package
   cli`, not `package cli_test` (gotcha G4). Verified — both
   `TestDeployService_NoPortReturnsExitUsageError` and
   `TestDeployService_PortZeroExplicitReturnsExitUsageError` call
   `errors.Is(err, errUsage)` and the package-level identifier resolves.
   They fail only because no error is currently produced (validation
   gap), which is the correct red bar.

4. **`--port` placement** in updated fixtures: always before the
   positional `<source-dir>` argument, per gotcha G3.

5. **gofmt is clean** on all three modified files.

6. **No production code was touched.** Tests only, exactly per the
   instruction to leave production code for Rob.

7. **Linus's one-line comment** about Cobra's flag-default-from-env
   mechanism is in `TestRoot_ConfigRootFlagControlsLogPlacement`
   (root_test.go), per his note in `04-linus-review.md`.

---

## Hand-off to Rob

Rob: apply the three production diffs from Joel's tech plan §1, §2, §3.
Specifically:

- `internal/cli/deploy_service.go`: insert the six-line dockerfile
  resolution block after the `filepath.Abs` call; change line 88 to use
  the resolved `dockerfile` variable; replace lines 73-75 with the
  unconditional `f.Port == 0` check.
- `internal/cli/root.go`: change `logging.Init()` to `logging.Init(rc.ConfigRoot)`.
- `internal/logging/logging.go`: change the signature to `Init(root string) error`,
  add the empty-string fallback to `config.DefaultRoot`, update the doc
  comment.

After your diffs, `go test ./...` must be 100% green. The acceptance
criteria are in tech plan §9.

If you hit a test failure that does not match the table above, stop and
escalate to Donald Knuth — that would be a sign that something in the
plan or in production is not what we believe it to be.
