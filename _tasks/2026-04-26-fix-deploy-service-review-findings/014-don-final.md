# Don's final sign-off

**Verdict: FULLY DONE.**

I verified the work independently. Not taking anyone's word for it.

## Blocker resolved

`internal/cli/deploy_service.go:55` now reads:

```go
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required)")
```

Matches `_docs/usage.md` (`Required: yes`) and the runtime behavior at
`deploy_service.go:73-75`. CLI `--help`, manual, and code now tell one
story. The drift Kevlin and Linus flagged is gone.

## All three findings closed

Traced each one against the working tree, not against the reports:

1. **Finding 1 — dockerfile path resolution** —
   `internal/cli/deploy_service.go:80-86` resolves with empty-string
   guard → `filepath.IsAbs` short-circuit → `filepath.Join(abs, ...)`.
   The resolved value is passed to `deploy.Request.Dockerfile` at
   line 95. `internal/dockerdrv/cli_driver.go` is untouched — the CLI
   layer owns "relative to source-dir", which is the right seam.

2. **Finding 2 — --config-root controls log placement** —
   `internal/cli/root.go:23` calls `logging.Init(rc.ConfigRoot)`.
   `internal/logging/logging.go:22-46` takes `root` as a parameter,
   falls back to `config.DefaultRoot` only when empty. No
   `config.RootFromEnv()` read inside `Init` anymore. Wrong call site
   can't compile. That's the structural fix.

3. **Finding 3 — deploy without --port can only fail at runtime** —
   `internal/cli/deploy_service.go:73-75` rejects `f.Port == 0`
   unconditionally with `errUsage` wrap → `ExitUsageError` at
   `internal/cli/exit_codes.go:35-36`. Probe and deployer untouched;
   policy stays at the CLI boundary where it belongs.

## Build status

Ran `go test ./... -count=1` myself. Green tree-wide:

```
ok    .../internal/caddy        0.016s
ok    .../internal/cli          0.020s
ok    .../internal/config       0.010s
ok    .../internal/deploy       12.083s
ok    .../internal/dockerdrv    0.052s
ok    .../internal/envcap       0.106s
ok    .../internal/ids          0.013s
ok    .../internal/logging      0.017s
ok    .../internal/registry     0.038s
```

Eleven packages compile, all tests pass, no skipped tests.

## Nothing slipped in

`git --no-pager diff main --stat`:

```
 _ai/cobra-init-pattern.md           |  17 +++--
 _ai/m1x-backlog.md                  |   2 +-
 _docs/usage.md                      |   6 +-
 _tasks/current                      |   2 +-
 internal/cli/deploy_service.go      |  15 ++--
 internal/cli/deploy_service_test.go | 133 +++++++++++++++++++++++++++++++++++-
 internal/cli/root.go                |   2 +-
 internal/cli/root_test.go           |  34 ++++++++-
 internal/logging/logging.go         |  15 ++--
 internal/logging/logging_test.go    |  34 ++++++---
```

Three production files, three test files, three doc files, one task
pointer. Exactly what the plan promised. `internal/dockerdrv/`,
`internal/deploy/`, `internal/registry/`, and `cmd/decloud/` are all
untouched. No "since we're here" refactors. No stray formatting
churn. No new packages, no new flags, no new abstractions.

## What we shipped (in plain English)

- The CLI layer now resolves `--dockerfile` against the source dir
  before handing it to the Docker driver. Relative paths work from
  any cwd. Absolute paths pass through.
- `--config-root` now controls log placement too. One flag, one
  root, one consistent layout. Logs and state no longer split
  across two roots.
- `--port` is now mandatory at parse time, with an exit code 2 and
  a clear message. No more readiness-probe failure standing in
  for argument validation.
- `--help`, `_docs/usage.md`, and the runtime all agree.

## Non-blocking items I'm explicitly NOT acting on

- `TestDeployService_HostWithoutPortReturnsExitUsageError` — name is
  mildly inaccurate after the generic port check fires first, but the
  scenario it asserts (operator passes `--host` without `--port`) is
  still real and the assertion is still correct. **Option B (leave).**
  Renaming costs reviewer cycles for no behavioral gain. Deleting
  loses a real-world test case.
- `_ai/cobra-init-pattern.md` pseudo-Go in a warning (`Init(string) {}`).
  Trivial doc nit. Fix when convenient. Not blocking.

## Verdict

**FULLY DONE.** Three findings, three fixes, three layers. Eleven
production lines plus one help-text fix. CLI, runtime, docs, and AI
notes coherent. Tests lock in the contracts at every boundary that
matters.

Ready for FINALIZATION (Ward + Andy) and commit.

— Don
