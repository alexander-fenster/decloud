# Don's plan — journald log driver

Task: every Decloud-managed Docker container is started with
`--log-driver=journald --log-opt tag=decloud/<service>` so that logs survive
container redeployment, while `docker logs` and `decloud logs` keep working
unchanged.

## 1. Headline decisions

1. **Always-on, no opt-out flag.** Decloud already targets systemd hosts
   (install.md §2 enables docker via `systemctl`, M1 uses `--restart=unless-stopped`
   under the docker daemon's systemd unit). Journald is universally available.
   A config knob is unjustified weight for zero use case in M1–M5.
2. **Tag format: `decloud/<service>`** — literal, no Go-template placeholders.
   The slash is purely a presentation namespace; journald stores it verbatim in
   the `CONTAINER_TAG` field and `journalctl CONTAINER_TAG=decloud/<service>`
   filters cleanly. Service names are DOCUMENTED as `[a-z][a-z0-9-]{0,38}` in
   a Cobra help string (`internal/cli/deploy_service.go:57`), but Joel's review
   caught that this regex is NOT enforced anywhere in code — `validateForSave`
   in `internal/registry/store.go:206-226` only checks non-empty. The tag is
   safe in practice today (no caller can introduce a bad name through the CLI
   or hand-edited TOML in normal flow), but a new driver-level invariant locks
   the tag-ambiguity failure mode for this task — see §1.7.
3. **Caddy gets the same treatment.** The `decloud-caddy` container is just as
   redeploy-prone as a service container (`decloud caddy down && up` wipes
   logs today). Tag: `decloud/caddy`. Same namespace, same query shape.
4. **Single chokepoint: `internal/dockerdrv/cli_driver.go`.** Both `Run` and
   `RunWithOptions` build the `docker run` argv. The two flags are appended
   there, immediately after `--restart`, before env/labels/ports/volumes —
   matches the existing flag-order discipline.
5. **Service name flows in explicitly, not by stripping `decloud-` off the
   container name.** The current `Run` implementation derives the
   `decloud.service` label by `strings.TrimPrefix(req.Name, "decloud-")`
   (`cli_driver.go:64`). That trick is already a smell (Kevlin flagged it once)
   and journald-tag derivation should NOT depend on it. Add a `Service string`
   field to `RunRequest` and `RunOptions`; callers populate it explicitly.
6. **Out of scope for this task:** `decloud logs` UX changes to surface
   cross-restart history via `journalctl`. Note it as a follow-up; do not
   build it here.
7. **Slash-in-`Service` guard is IN SCOPE — alongside the empty-`Service`
   guard, not deferred.** Linus's review (§2.1) flagged that this task ships
   a new downstream consumer (the journald tag) of an unvalidated string,
   and that `/` in a service name would cause `journalctl
   CONTAINER_TAG=decloud/foo` to return partial results because
   `decloud/foo/bar` would also match the prefix. The defended invariant
   ("the tag is unambiguous for `journalctl`") is BORN in this task; the
   right place to defend it is this task. The marginal cost is two lines in
   the driver guard plus two tests (one per `Run`/`RunWithOptions` path),
   and Kent is already writing empty-`Service` rejection tests in the same
   shape — the slash-rejection tests are a near-mechanical copy. Deferring
   would externalise defensive work onto a future commit cycle for a
   failure mode this task SPECIFICALLY introduces, and the broader
   centralised-validator follow-up is unaffected either way. Chose Option B
   over Option A. See §3.2 for the exact guard shape and §4.1 for the
   tests.

## 2. What I traced (proof, not assumptions)

### 2.1 Every `docker run` chokepoint

```
internal/dockerdrv/cli_driver.go:46   func (d *cliDriver) Run(...)            // services
internal/dockerdrv/cli_driver.go:212  func (d *cliDriver) RunWithOptions(...) // caddy
```

These are the only two functions that build a `docker run -d ...` argv. Every
caller goes through one of them. Verified by:

```
grep -rn '"run"' internal/dockerdrv/ internal/deploy/ internal/caddy/
```

Returns only the two `args := []string{"run", "-d", ...}` lines above.
There is no other shell-out to `docker run` anywhere in the repo. `docker
start` (`cli_driver.go:91`) does not need log flags — they were set at
`docker run` time and persist on the container.

### 2.2 Every caller of `Run` / `RunWithOptions`

```
internal/deploy/service.go:253   d.deps.Driver.Run(ctx, runReq)             // fresh deploy
internal/deploy/service.go:385   d.deps.Driver.Run(cleanupCtx, runReq)      // rollback to old
internal/deploy/lifecycle.go:76  d.deps.Driver.Run(ctx, runReq)             // start absent → re-run
internal/caddy/manager.go:95     m.cfg.Driver.RunWithOptions(ctx, m.runOpts())
```

Four `docker run` paths total. Each one knows the service name at the call site:

- `service.go:253` — `req.Name` (the deploy request)
- `service.go:385` — `prev.Config.Name` in `restoreOldContainer`
- `lifecycle.go:76` — `prev.Config.Name` loaded from the store
- `manager.go:95` — hardcoded `"caddy"` (the manager-internal name, not the
  container name)

Service name flow into the tag is trivial in every case.

### 2.3 `docker start` does NOT relaunch the container

`internal/deploy/lifecycle.go:62-65` calls `Driver.Start` on the `exited`
branch. `docker start` reuses the original `HostConfig.LogConfig` — the
journald driver and tag survive. Confirmed by docker docs and by the fact
that `--restart=unless-stopped` recovers logs identically today. No code
change needed for the `exited → running` path.

### 2.4 Journald tag semantics (verified)

- The journald docker driver writes `CONTAINER_TAG=<tag>` and
  `SYSLOG_IDENTIFIER=<tag>` for every line. `journalctl CONTAINER_TAG=decloud/foo`
  matches exactly.
- Tags are byte-literal in journald — `/` is fine, no sanitisation. Service
  names being `[a-z0-9-]` only means the full tag matches `[a-z0-9/-]+`.
- The journald driver supports `docker logs` natively (one of the few drivers
  that does). No change to `decloud logs` / `Driver.Logs` is required.
- The docker daemon must be running under systemd (it is — see install.md §2);
  if a host ever ran the docker daemon without systemd, journald logging
  fails at container-start time with a clear daemon error. Not our problem
  to detect: docker surfaces it as a `docker run` stderr.

## 3. The change, surface by surface

### 3.1 `internal/dockerdrv/driver.go`

Add `Service string` to both shapes, plus TWO sentinel errors (empty and
invalid):

```go
type RunRequest struct {
    Name    string
    Service string         // NEW: service name; populates journald tag
    Image   string
    Network string
    Env     map[string]string
    Restart string
    Port    int
    Volumes []VolumeMount
}

type RunOptions struct {
    Name    string
    Service string         // NEW: service name; populates journald tag
    Image   string
    Network string
    Restart string
    Ports   []PortMap
    Volumes []VolumeMount
    Labels  map[string]string
    Env     map[string]string
}
```

`Service` is required (non-empty) and must NOT contain `/`. Both are
programmer-error conditions; both return a non-nil error from
`Run`/`RunWithOptions` BEFORE shelling out (no docker process spawned).

Two sentinels (Joel: pick the exact message wording in 03):

- `ErrEmptyService` — empty `Service` is a programmer error at the call
  site. Message conveys "populate `Service` in the `RunRequest`/`RunOptions`
  literal."
- `ErrInvalidService` — non-empty `Service` containing `/`. Message conveys
  "service names must not contain `/`; the journald tag would become
  ambiguous under `journalctl CONTAINER_TAG=decloud/<service>` because a
  prefix match would also match `decloud/<service>/...`."

Distinct sentinels (not folded into one) so callers can `errors.Is`-discriminate
in tests, and so the failure-mode story is legible in stack traces.

### 3.2 `internal/dockerdrv/cli_driver.go`

In both `Run` (line 46) and `RunWithOptions` (line 212), prepend a
two-line guard at the very top of the function (BEFORE the args literal),
then emit the journald flags. Joel's §11.1 chose literal-splice over
`append`; Linus endorsed it. The guard shape (substitute `opts` for `req`
in `RunWithOptions`):

```go
if req.Service == "" {
    return "", ErrEmptyService
}
if strings.ContainsRune(req.Service, '/') {
    return "", ErrInvalidService
}
args := []string{
    "run", "-d",
    "--name", req.Name,
    "--network", req.Network,
    "--restart", req.Restart,
    "--log-driver", "journald",
    "--log-opt", "tag=decloud/" + req.Service,
}
```

Note: the `strings` package is already imported in `cli_driver.go`
(used by `strings.TrimSpace`, `strings.Contains`, `strings.ToLower`).
Removing the `TrimPrefix` line does not remove the import.

Also: replace `strings.TrimPrefix(req.Name, "decloud-")` at line 64 with
`req.Service` for the `decloud.service` label, since we now have the explicit
field. One less stringly-typed derivation.

### 3.3 `internal/deploy/service.go`

Two `RunRequest` constructions:

- Line 244 (`Deploy`): set `Service: req.Name`.
- Line 376 (`restoreOldContainer`): set `Service: prev.Config.Name`.

### 3.4 `internal/deploy/lifecycle.go`

One `RunRequest` construction:

- Line 67 (`Start`, absent branch): set `Service: prev.Config.Name`.

### 3.5 `internal/caddy/manager.go`

`m.runOpts()` (line 124): set `Service: "caddy"`. Tag becomes `decloud/caddy`.

## 4. Tests to change / add

### 4.1 `internal/dockerdrv/cli_driver_test.go`

Update existing argv-assertion tests to expect the two new flags in the
right position. Specifically:

- `TestCLIDriver_RunArgsWithEnvSorted` (line 71) — splice `"--log-driver",
  "journald", "--log-opt", "tag=decloud/foo"` after `--restart unless-stopped`,
  before `--env`.
- `TestCLIDriver_RunArgsWithEmptyEnv` (line 100) — same.
- `TestCLIDriver_RunPassesVolumeFlags` (line 408) — same; tag is
  `decloud/foo` since `Service: "foo"`.
- `TestCLIDriver_RunWithOptionsCaddyShape` (line 360) — splice
  `"--log-driver", "journald", "--log-opt", "tag=decloud/caddy"` after
  `--restart unless-stopped`, before `--label`. Update
  `caddyRunOptionsFixture` (line 526) with `Service: "caddy"`.

New tests:

- `TestCLIDriver_RunRejectsEmptyService` — `Service: ""` returns
  `ErrEmptyService` and does NOT call `docker`.
- `TestCLIDriver_RunWithOptionsRejectsEmptyService` — same shape.
- `TestCLIDriver_RunRejectsServiceContainingSlash` — `Service: "foo/bar"`
  returns `ErrInvalidService` and does NOT call `docker`. Defends the
  journald-tag-ambiguity invariant from §1.7. Use the same
  `assert.Empty(t, records)` guard-before-exec assertion shape as the
  empty-Service tests.
- `TestCLIDriver_RunWithOptionsRejectsServiceContainingSlash` — same
  shape, against `RunWithOptions`.
- `TestCLIDriver_RunEmitsJournaldFlagsInDeclaredOrder` — explicit check that
  `--log-driver journald --log-opt tag=decloud/<svc>` is exactly the pair
  emitted, in that order, and the tag literal is `decloud/<svc>` (not
  `decloud-<svc>`, not `<svc>`, not Go-template `{{.Name}}`).
- `TestCLIDriver_RunWithOptionsEmitsCaddyTagLiteral` — Joel's previously
  "optional" §6.2.4 test, now REQUIRED per Linus §5.2. Same shape as the
  tag-literal test above but on the `RunWithOptions` path, asserting
  `tag=decloud/caddy`. The caddy tag is the only literal that differs
  between the two paths; without this test a future refactor that
  hardcodes a wrong tag in `RunWithOptions` would only surface in Linux
  integration smoke.
- `TestCLIDriver_StartArgs` (existing, `cli_driver_test.go:128`) gains
  ONE assertion per Linus §5.1: `assert.NotContains(t, records[0].Args,
  "--log-driver")`. This locks the invariant that `docker start` does
  NOT re-emit journald flags (the `HostConfig.LogConfig` is sealed at
  create time). Defends against the future drive-by "consistency"
  refactor that thinks `docker start` should also know about logging.

### 4.2 `internal/deploy/service_test.go`

Tests that match argv at the `Driver.Run` mock layer are mock-based (they
check `RunRequest` fields, not the rendered argv). They need the new
`Service` field populated in the gomock expectation. Specifically the
`Driver.EXPECT().Run(gomock.Any()).DoAndReturn(...)` sites that capture and
inspect `req` — search for `RunRequest{` in `service_test.go` and
`lifecycle_test.go` and update fixtures. Existing tests:

- `TestDeploy_RunRequestUsesCapturedEnvAndDecloudNetwork` (line 831) —
  extend to assert `req.Service == "<service-name>"`.
- `TestDeploy_RestoreOldContainerPassesVolumesToDriver` (line 1011) —
  extend to assert `req.Service == prev.Config.Name`.

New test:

- `TestDeploy_RunRequestIncludesServiceName` — assert the deploy populates
  `RunRequest.Service` so the driver-level test's tag derivation is wired.

### 4.3 `internal/deploy/lifecycle_test.go`

Lifecycle `Start` absent-branch test fixture: extend the `Run` expectation
to assert `req.Service == "<name>"`.

### 4.4 `internal/caddy/manager_test.go`

`expectedCaddyRunOptions` (line 55) gains `Service: "caddy"`. Every test
that uses that fixture (Up fresh install, Up after exit + restart, etc.)
picks up the change for free via the gomock matcher.

## 5. Documentation changes

### 5.1 `_docs/usage.md`

- §2 "What the deploy actually does, in order" — add a bullet after step 4:
  "Container runs with `--log-driver=journald --log-opt tag=decloud/<name>`,
  so logs survive container redeployment. `decloud logs` keeps working
  (the journald driver supports `docker logs` natively); cross-redeploy
  history is queryable on the host with `journalctl CONTAINER_TAG=decloud/<name>`."
- §4 "Lifecycle commands" — annotate `decloud logs <name>` to note that
  `-f`/`--tail` only see the live container's log range; the host-side
  cross-redeploy archive is on journald (`journalctl CONTAINER_TAG=...`).
  Keep it short — actual UX changes to `decloud logs` are out of scope.
- §6 "Debugging a container directly" — add a one-liner pointing at
  `journalctl CONTAINER_TAG=decloud/<service>` as the way to see logs from
  a previous container instance.

### 5.2 `_docs/install.md`

- §1 Prerequisites — note that the host must run docker under systemd (it
  already does in the install procedure, but this is the place to make the
  journald dependency explicit). Single sentence: "The journald log driver
  requires the docker daemon to run under systemd; the default Docker Engine
  install (`systemctl enable --now docker`) satisfies this."

### 5.3 `_ai/decisions/` (new)

New file: `_ai/decisions/journald-log-driver.md`. Records:

- Why journald over syslog (single sentence each).
- Why always-on, no flag.
- Tag format and the service-name regex that constrains its char set.
- The "Service flows explicitly, not by string trick on Name" rule and why
  (so the next person doesn't think they can re-derive the tag from `Name`
  if a future M4 container-naming change adds a deploy-ID suffix — see
  `_ai/container-naming.md`).

### 5.4 `_ai/m1x-backlog.md`

Add entry: "`decloud logs --history` (or similar) to read cross-redeploy
log archive via journald." Cite this task. Mark as deliberately deferred
out-of-scope.

## 6. Acceptance criteria

1. `docker inspect decloud-foo --format '{{.HostConfig.LogConfig.Type}}'`
   prints `journald` for any service container created by `decloud deploy`,
   `decloud start` (absent → re-run), `decloud restart` (via Start), or the
   rollback path. Same for `decloud-caddy`.
2. `docker inspect decloud-foo --format '{{index .HostConfig.LogConfig.Config "tag"}}'`
   prints `decloud/foo`. `decloud-caddy` prints `decloud/caddy`.
3. `decloud logs foo` and `decloud logs foo -f --tail 50` still work
   identically to today (journald supports `docker logs`).
4. After `decloud unregister foo` followed by `decloud deploy service --name
   foo ...`, `journalctl CONTAINER_TAG=decloud/foo` shows lines from BOTH
   the pre-redeploy and post-redeploy container instances. (Manual smoke
   check on a Linux host; not a unit-test assertion. Linux-only — note for
   Kent.)
5. `go test ./...` passes.
6. Any `RunRequest{` or `RunOptions{` constructed with an empty `Service`
   field returns `ErrEmptyService` before shelling out to docker. Any
   `Service` containing `/` returns `ErrInvalidService` before shelling
   out to docker.
7. `docker start` argv does NOT contain `--log-driver` or `--log-opt` —
   the log configuration is sealed at `docker run` time and `docker
   start` must not duplicate it. Locked by the new
   `assert.NotContains` on `TestCLIDriver_StartArgs`.

## 7. Edge cases and what could break this

- **Docker daemon without systemd.** Surfaces as a `docker run` stderr at
  container-start time. No code change; the error is loud and operator-visible.
  Documented in §3.4 above (install.md note).
- **`docker logs` for the `created` state on the journald driver.** Some
  older docker releases (≤20.10) had a known quirk where `docker logs` on
  a freshly-`created` (not yet started) container returns nothing under
  journald. M1 only runs `docker logs` on running/exited containers; this
  does not bite us.
- **Service name with `/` somehow.** Defended in code: the driver guard
  rejects with `ErrInvalidService` before shelling out (§3.2, §1.7).
  Note: the upstream "regex" claim is a Cobra help-string lie (Joel's
  catch), not a code-enforced constraint. Today no caller can introduce
  a `/` through normal flow, but the driver invariant doesn't depend on
  that. The tag is guaranteed to be `decloud/<service>` with at most one
  `/` and unambiguous under `journalctl CONTAINER_TAG=` prefix queries.
- **Container created by external tooling carrying our container name but
  not our log driver.** Out of scope — the orphan-removal label gate
  (`_docs/usage.md` §8) already handles that case; we don't try to enforce
  journald on containers we didn't create.
- **`docker logs` against an exited container after a host reboot.** Works
  fine under journald — that's the whole point of this change.
- **`decloud restart` path.** `Stop` then `Start`; `Start` either calls
  `docker start` (which preserves the original log config) or
  `Driver.Run` with the new `Service` field. Both paths covered.

## 8. Explicit out-of-scope

- Any change to `decloud logs` UX. The journald archive is queryable via
  `journalctl CONTAINER_TAG=decloud/<service>` today; surfacing it through
  `decloud logs --history` (or a flag) is a separate task. Added to
  `_ai/m1x-backlog.md`.
- Log rotation / retention. Journald has its own retention; we don't tune
  it from Decloud. Operator's problem.
- Migrating historical logs from existing service containers. Anyone
  running an older Decloud build will see the change kick in on the next
  `decloud deploy service` for each service. Pre-redeploy logs in the
  old container's `json-file` store are lost on the next redeploy as
  before, which is the bug we're fixing for future redeploys.

## 9. File list

Code:

- `internal/dockerdrv/driver.go` — add `Service` field to `RunRequest` and
  `RunOptions`; add `ErrEmptyService` and `ErrInvalidService` sentinels.
- `internal/dockerdrv/cli_driver.go` — emit journald flags in both `Run`
  and `RunWithOptions`; reject empty `Service` AND `Service` containing
  `/`; replace `TrimPrefix` trick with `req.Service` in the
  label-derivation step.
- `internal/deploy/service.go` — populate `Service` in two `RunRequest`s.
- `internal/deploy/lifecycle.go` — populate `Service` in one `RunRequest`.
- `internal/caddy/manager.go` — populate `Service: "caddy"` in `runOpts()`.

Tests:

- `internal/dockerdrv/cli_driver_test.go` — update existing argv asserts;
  add six new tests (empty-Service rejection x2, slash-Service rejection
  x2, tag-literal check on `Run`, caddy tag-literal check on
  `RunWithOptions`); extend `TestCLIDriver_StartArgs` with the
  `assert.NotContains(args, "--log-driver")` assertion (Linus §5.1).
- `internal/deploy/service_test.go` — update fixtures, add Service-field
  assertion test.
- `internal/deploy/lifecycle_test.go` — extend Start-absent test.
- `internal/caddy/manager_test.go` — update `expectedCaddyRunOptions`.

Mocks: `internal/dockerdrv/mocks/mock_driver.go` regenerates from
`go generate ./...`; no hand-edit.

Docs:

- `_docs/usage.md` — three small additions (§2 deploy steps, §4 logs
  annotation, §6 debugging).
- `_docs/install.md` — one sentence in §1.
- `_ai/decisions/journald-log-driver.md` — NEW.
- `_ai/m1x-backlog.md` — one new entry for the deferred `decloud logs
  --history` follow-up.

## 10. Hand-off to Joel

This is the second iteration. Joel: in the next pass on
`03-tech-plan.md`, fold in the three items from Linus's review:

1. **Slash-in-`Service` guard (§1.7, §3.2, §4.1, §6, §7).** I picked
   Option B. Add the two-line guard alongside the empty-Service guard,
   add the `ErrInvalidService` sentinel to the `var (…)` block at
   `driver.go:12-21`, and write the two new rejection tests in the same
   shape as the empty-Service tests (`assert.Empty(t, records)` to
   prove the guard fires BEFORE `cmd.Run`). Acceptance criterion 6 is
   updated.
2. **`TestCLIDriver_StartArgs` negative assertion (Linus §5.1).** Add
   `assert.NotContains(t, records[0].Args, "--log-driver")` to the
   existing `docker start` test (or alongside it). Locks the invariant
   that `docker start` does not re-emit log flags. Acceptance
   criterion 7 added.
3. **Caddy tag-literal test promoted to required (Linus §5.2).** Your
   §6.2.4 was "optional, recommended"; Linus called it required and
   so do I. It's the only thing locking `tag=decloud/caddy` on the
   `RunWithOptions` path.

Other items already settled in this iteration: the §1.2 wording
correction (the regex is documented, not code-enforced — Linus §7.1
addressed); literal-splice over append (your §11.1 — endorsed); `name`
vs. `prev.Config.Name` in `lifecycle.Start` (your §11.6 — endorsed);
`ErrEmptyService` returned not panicked (endorsed). The longer-message
wording for `ErrEmptyService` (your §11.2) is your call; pick.
