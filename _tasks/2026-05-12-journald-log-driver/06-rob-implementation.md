# Rob — implementation report for journald log driver

STEP 3b (EXECUTION — implementation). Green step for Kent's failing tests
in commit `2e3f103`. All work done on branch `task/journald-log-driver`.

## Files changed

### Production code

- `internal/dockerdrv/cli_driver.go` — `Run` and `RunWithOptions` both
  gain an empty-Service guard, then a slash-Service guard (in that
  order, fired BEFORE the args literal is built so no `cmd.Run` is
  ever spawned on rejection). Four new tokens spliced into the args
  literal next to `--name` / `--restart`:
  `"--log-driver", "journald", "--log-opt", "tag=decloud/" + req.Service`.
  The `strings.TrimPrefix(req.Name, "decloud-")` smell at line 64 is
  replaced with `req.Service` — the explicit field is now the source
  of truth for the `decloud.service` label.
- `internal/deploy/service.go` — `Service: req.Name` in the fresh-deploy
  `RunRequest`; `Service: prev.Config.Name` in the rollback path.
- `internal/deploy/lifecycle.go` — `Service: name` in the absent-branch
  `RunRequest` (uses the function arg per Joel §11.6).
- `internal/caddy/manager.go` — `Service: "caddy"` in `runOpts()`.

### Test fixtures (per Joel §6.1, §6.3, §6.4, §6.5)

- `internal/dockerdrv/cli_driver_test.go`:
  - §6.1.1 — `TestCLIDriver_RunArgsWithEnvSorted`: `Service: "foo"` in
    the request fixture; four new tokens spliced into the expected
    argv after `--restart unless-stopped`; hand-typed comment refreshed.
  - §6.1.2 — `TestCLIDriver_RunArgsWithEmptyEnv`: `Service: "foo"` and
    four new `assert.Contains` assertions for the journald flags
    (covers the empty-env branch).
  - §6.1.3 — `TestCLIDriver_RunPassesVolumeFlags`: `Service: "foo"`.
  - §6.1.4 — `TestCLIDriver_RunWithOptionsCaddyShape`: four new tokens
    spliced into expected argv (caddy tag `decloud/caddy`).
  - §6.1.5 — seven `RunWithOptions` helper-based tests
    (`TestCLIDriver_RunWithOptionsDualStackPorts`,
    `…BindReadOnly`, `…NamedVolumeNotReadOnly`, `…LabelsSorted`,
    `…PortsDeclaredOrder`, `…PortDefaultProto`, `…EmptyHostBind`)
    each gain `Service: "x"`.
- `internal/deploy/service_test.go`:
  - §6.3.1 — `TestDeploy_RunRequestUsesCapturedEnvAndDecloudNetwork`
    gains `assert.Equal(t, "foo", seen.Service, …)`.
  - §6.3.2 — `TestDeploy_RestoreOldContainerPassesVolumesToDriver` now
    captures `rollbackSvc` from the rollback `Run` closure and
    asserts it equals `"foo"`.
- `internal/deploy/lifecycle_test.go`:
  - §6.4 — `TestLifecycle_StartFromAbsentReRunsContainer` and
    `TestLifecycle_StartAbsentBranchPassesVolumesToDriver` both gain
    `assert.Equal(t, "foo", seen.Service, …)`.
- `internal/caddy/manager_test.go`:
  - §6.5 — `expectedCaddyRunOptions` carries `Service: "caddy"`. Every
    test that uses the fixture picks the field up via gomock `Eq`.

The `internal/integration/mount_test.go` was already updated by Kent
in commit `2e3f103` (line 71: `Service: "mounttest"`). Confirmed —
`go test -tags=integration ./...` compiles and skips cleanly with no
Docker on the dev box.

## TrimPrefix smell — confirmed removed

```
$ grep -rn "TrimPrefix.*decloud-" internal/ cmd/
(no matches)
```

The only remaining occurrences of `TrimPrefix.*decloud-` are inside
`_tasks/` markdown files documenting the prior smell — no production
code or test files reference it. The `strings` import in
`cli_driver.go` is still required (used by `isNotFound`, `ContainerIP`,
and the new `strings.ContainsRune` slash guard).

## Verification

- `go test ./...` — 246 PASS, 0 FAIL.
- `go test -tags=integration ./...` — clean compile, all packages OK
  (the real-Docker integration tests skip on the dev box per the
  `DECLOUD_INTEGRATION=1` env-var gate).
- `gofmt -l .` — clean.
- `go vet ./...` — clean.
- `go generate ./...` — no diff to mocks (adding a struct field does
  not change the `Driver` interface signature, as Joel §5.6 predicted).

## Deviations from Joel's spec

None. The implementation follows §5.1–§5.5 verbatim:

- Guards in declared order (empty first, slash second) at the top of
  both `Run` and `RunWithOptions`.
- Literal-splice into the args slice (not `append`) per §11.1.
- `name` over `prev.Config.Name` in `lifecycle.Start` per §11.6.
- `Service: "caddy"` hardcoded literal in `caddy.runOpts()` per §7.
- `strings.TrimPrefix(req.Name, "decloud-")` replaced with `req.Service`
  in the `decloud.service` label per §5.2.

## Hand-off to Raymond

Implementation is green and ready for documentation. Raymond's
deliverables are listed in tech-plan §13 (Docs section): `_docs/usage.md`
§2/§4/§6 additions, `_docs/install.md` §1 single sentence,
`_ai/decisions/journald-log-driver.md` new file, and the
`_ai/m1x-backlog.md` deferred-followups entries.
