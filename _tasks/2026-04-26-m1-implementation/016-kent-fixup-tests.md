# Iter2 Step 3a — Kent: fix-up tests

**Author:** Kent Beck (test engineer)
**Status:** Tests added/modified per Joel's `14-fixup-tech-plan.md`. `gofmt -l .` clean. `go vet ./...` clean. `go test ./...` fails ONLY on the new and inverted assertions; all untouched tests still pass.
**Plan basis:** `14-fixup-tech-plan.md` items 1–6 (code-side) + Linus's approval in `15-linus-fixup-plan-review.md`.

---

## 1. New tests added (13 mandatory + 1 optional)

| # | Test name | File:line | Item |
|---|---|---|---|
| 1 | `TestInit_PermissionDeniedFallsBackToStderr` | `internal/logging/logging_test.go:33` | 1 |
| 2 | `TestInit_LogFileOpenFailureFallsBackToStderr` | `internal/logging/logging_test.go:48` | 1 |
| 3 | `TestRoot_HelpDoesNotRequireFilesystem` | `internal/cli/root_test.go:55` | 1 |
| 4 | `TestEnvcap_EmptyPathReturnsNilNil` | `internal/envcap/capture_test.go:118` | 2 |
| 5 | `TestEnvcap_MissingPathReturnsErrEnvScriptMissing` | `internal/envcap/capture_test.go:124` | 2 |
| 6 | `TestEnvcap_DirectoryPathReturnsErrEnvScriptUnreadable` | `internal/envcap/capture_test.go:130` | 2 |
| 7 | `TestDeploy_NoEnvScript_SkipsCapturerEntirely` | `internal/deploy/service_test.go:447` | 2 |
| 8 | `TestDeployService_AutoDiscoversEnvSh` | `internal/cli/deploy_service_test.go:131` | 2 |
| 9 | `TestDeployService_NoEnvShIsValid` | `internal/cli/deploy_service_test.go:149` | 2 |
| 10 | `TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError` | `internal/cli/deploy_service_test.go:165` | 2 |
| 11 | `TestDeploy_NetworkEnsureCalledFirst` | `internal/deploy/service_test.go:469` | 3 |
| 12 | `TestDeploy_NetworkEnsureFailureReturnsErrRun` | `internal/deploy/service_test.go:489` | 3 |
| 13 | `TestReadiness_EmptyIPNoErrorReportsErrNoBridgeIP` | `internal/deploy/readiness_test.go:130` | 5 |
| 14 (opt.) | `TestDeploy_BuildErrorPreservesInnerSentinel` | `internal/deploy/service_test.go:503` | 6 |

**By item:** Item 1 = 3, Item 2 = 7, Item 3 = 2, Item 5 = 1, Item 6 = 1 optional. Total = 14.

`TestRoot_HelpDoesNotRequireFilesystem` already passes (`--help` short-circuits today before any logging.Init). It locks in the contract for when Rob adds `PersistentPreRunE`.

---

## 2. Existing tests modified

### `internal/deploy/service_test.go` — 15 tests + 1 fixture (Item 3 audit list + Item 2 fixture)

Added `h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)` to every Deploy-driving test:

| Test | Line |
|---|---|
| `TestDeploy_HappyPathFirstDeploy` | 137 (head of InOrder block) |
| `TestDeploy_HappyPathRedeploy` | 158 (head of InOrder block) |
| `TestDeploy_LoadPreviousErrSecretsMissingTreatedAsFirstDeploy` | 178 |
| `TestDeploy_EnvCaptureFailureAbortsBeforeAnythingChanges` | 196 |
| `TestDeploy_BuildFailureAbortsBeforeStoppingOld` | 207 |
| `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` | 220 |
| `TestDeploy_RunNewFailureRollsBackToOld` | 237 |
| `TestDeploy_ReadinessFailureRollsBackToOld` | 263 |
| `TestDeploy_SaveErrPartialWriteRollsBackAndDeletesOrphanConfig` | 284 |
| `TestDeploy_SaveFailsBeforePartialWriteSkipsDeleteOrphanConfig` | 303 |
| `TestDeploy_CaddyValidateFailureLeavesOldFileAndKeepsNewContainer` | 319 |
| `TestDeploy_CaddyReloadFailureDoesNotRollBackContainer` | 337 |
| `TestDeploy_DeployIDIsStableThroughoutOneDeploy` | 357 |
| `TestDeploy_CapturedEnvNotLoggedAsKeysOrValues` | 387 |
| `TestDeploy_RunRequestUsesCapturedEnvAndDecloudNetwork` | 412 |

`newRequest()` fixture (`service_test.go:106`): added `EnvFile: "/srv/foo/env.sh"` line so the existing happy-path `Capture` expectations still match the orchestrator's new `if envFile != ""` branch.

### `internal/deploy/readiness_test.go` — 6 call sites (Item 4 rename)

`deploy.NewHTTPProbeForTest` → `deploy.NewHTTPProbe` at lines 59, 68, 90, 101, 121, 141 (replace_all).

### `internal/envcap/capture_test.go` — 1 test inverted

`TestEnvcap_ScriptDoesNotExist` (line 113):

- **Before:** `assert.Contains(t, err.Error(), "env.sh")` (string-match)
- **After:** `assert.True(t, errors.Is(err, envcap.ErrEnvScriptMissing))` (typed sentinel)

### `internal/cli/deploy_service_test.go` — 1 test extended

`TestDeployService_BuildsExpectedRequest` (line 70 onward): added a final assertion `assert.Equal(t, "", got.EnvFile, ...)` to lock in "no env.sh discovered at /srv/foo → empty EnvFile passed through".

---

## 3. Inverted assertion (Item 1 migration)

**File:** `internal/logging/logging_test.go`

**Before** (was `TestInit_FileOpenFailureReturnsError`, lines 33-47):
```go
err := logging.Init()
require.Error(t, err)
```

**After** (renamed to `TestInit_LogFileOpenFailureFallsBackToStderr`, lines 48-62):
```go
require.NoError(t, logging.Init())
```

The test fixture is otherwise identical (chmod 0500, root-skip guard, env wiring); only the terminal assertion flipped from "error returned" to "fallback engaged, no error". Plus a sibling `TestInit_PermissionDeniedFallsBackToStderr` at line 33 that exercises the MkdirAll branch (chmod the parent of `logs`, not `logs` itself).

---

## 4. Stub additions to keep tests compiling

Joel's plan §item 4 says the rename is mechanical (no new tests). Joel §item 2 introduces new sentinel errors (`ErrEnvScriptMissing`, `ErrEnvScriptUnreadable`). To keep `go test ./...` compile-clean while leaving runtime behavior unchanged for Rob to fix, I added the minimum production-side hooks:

- **`internal/envcap/capture.go`:** added `var ErrEnvScriptMissing = errors.New(...)` and `var ErrEnvScriptUnreadable = errors.New(...)` plus the `errors` import. The `Capture()` body is **untouched** — Rob still owns the empty-path/missing/dir-path branching per Joel §item 2.
- **`internal/deploy/readiness.go`:** renamed `NewHTTPProbeForTest` → `NewHTTPProbe` (Joel §item 4 calls this a pure rename; no behavior change).

No new methods on any interface. No mock regeneration needed. Existing mocks (`internal/dockerdrv/mocks/mock_driver.go`, etc.) already include `NetworkEnsure` from iter1.

---

## 5. `go test ./...` output (excerpted)

Untouched packages still pass:
```
ok  	github.com/alexander-fenster/decloud/internal/caddy
ok  	github.com/alexander-fenster/decloud/internal/config
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv
ok  	github.com/alexander-fenster/decloud/internal/ids
ok  	github.com/alexander-fenster/decloud/internal/registry
```

Failing packages (expected — Rob's runway):
```
FAIL	github.com/alexander-fenster/decloud/internal/cli   (2 new tests fail)
FAIL	github.com/alexander-fenster/decloud/internal/deploy (15 modified + 4 new tests fail; ErrRun, NetworkEnsure-first, no-env-script branching, build-sentinel)
FAIL	github.com/alexander-fenster/decloud/internal/envcap (4 tests fail; ErrEnvScriptMissing/Unreadable wiring + empty-path semantics)
FAIL	github.com/alexander-fenster/decloud/internal/logging (2 tests fail; fallback-on-EACCES + fallback-on-ENOENT)
```

Failure breakdown:

- **internal/logging (2):** `TestInit_PermissionDeniedFallsBackToStderr`, `TestInit_LogFileOpenFailureFallsBackToStderr` — both fail with `permission denied` because `Init()` returns the FS error today instead of falling back. Rob's fix in `logging.go` makes them pass.
- **internal/envcap (4):** the inverted `TestEnvcap_ScriptDoesNotExist` plus the three new sentinel/empty-path tests. All fail because `Capture()` still wraps every stat error as `env.sh: %w`.
- **internal/deploy (19):** 15 happy-path-or-partial happy-path tests fail with `missing call(s) to NetworkEnsure` because Rob hasn't added step 0 yet. Plus the 4 new tests (`NoEnvScript_SkipsCapturerEntirely`, `NetworkEnsureCalledFirst`, `NetworkEnsureFailureReturnsErrRun`, `BuildErrorPreservesInnerSentinel`).
- **internal/cli (2):** `TestDeployService_AutoDiscoversEnvSh` (no auto-discovery yet → empty EnvFile), `TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError` (no `resolveEnvFile` yet, error not `ErrEnvScriptMissing`). `TestDeployService_NoEnvShIsValid` already passes.

No panics. No build errors. Every failure points at a specific Joel-item fix.

`gofmt -l .` clean. `go vet ./...` clean.

---

## 6. Anything ambiguous in Joel's plan

1. **`TestRoot_HelpDoesNotRequireFilesystem`** — Joel called for a "sentinel" approach to verify Init is never called. I went with the FS-side proof: point `DECLOUD_ROOT` at an unwritable path AND clear `DECLOUD_LOG_TO_STDERR_ONLY`, then assert `--help` returns nil. Today this passes vacuously (cobra short-circuits help before any RunE/PersistentPreRunE). After Rob adds `PersistentPreRunE`, the test still passes because cobra's help machinery short-circuits before PersistentPreRunE fires (Joel confirmed this in §item 1 architectural notes). If Linus prefers a stricter sentinel ("Init was not called at all"), the easiest extension is to add a `var initCallCount int` test seam in `logging.go`; flagging but not implementing.

2. **`TestEnvcap_DirectoryPathReturnsErrEnvScriptUnreadable`** — Joel's §item 2 sentinel definition says `ErrEnvScriptUnreadable` covers "stat or open denied" AND "is a directory". I went with the directory case for the test because it's deterministic on every platform (no chmod, no root guard). The "0000 perms file" case Joel names in his table is implicitly covered by Rob's stat→`fs.ErrPermission`→`ErrEnvScriptUnreadable` mapping; an explicit chmod-0000 test could be added but it duplicates the macro behavior.

3. **`TestDeploy_BuildErrorPreservesInnerSentinel`** — Joel marked as optional ("not a DONE-criterion") but called it the only test that proves the `%w:%w` fix is real. I included it. If Linus wants it dropped, one-line removal.

4. **`newRequest()` EnvFile value** — Joel said "any non-empty path; mock matches gomock.Any()". I used `/srv/foo/env.sh` to mirror the SourceDir prefix. The path is never stat'd (the mock intercepts).

---

## 7. Summary table

| Category | Count |
|---|---|
| New tests added | 14 (13 mandatory + 1 optional) |
| Existing tests modified to add `NetworkEnsure` expectation | 15 |
| Existing tests with `NewHTTPProbeForTest` → `NewHTTPProbe` rename | 6 |
| Existing tests inverted (assertion flip) | 1 (`TestEnvcap_ScriptDoesNotExist`) + 1 (`TestInit_FileOpenFailureReturnsError` → `TestInit_LogFileOpenFailureFallsBackToStderr`) |
| Existing tests extended (one new assertion) | 1 (`TestDeployService_BuildsExpectedRequest`) |
| Test fixtures touched | 1 (`newRequest()` adds `EnvFile`) |
| Production stubs added (sentinel errors + rename only) | 3 (`ErrEnvScriptMissing`, `ErrEnvScriptUnreadable`, `NewHTTPProbe`) |

Pre-fixup test total: 144. Post-fixup test total: 144 + 14 = **158**. Of those 158, 27 currently fail; the other 131 pass. Rob's job is to drive the 27 failing tests to green by implementing items 1–6.

End of Kent fix-up report.
