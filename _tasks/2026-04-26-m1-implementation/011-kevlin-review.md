# Step 3d — Kevlin: low-level code + docs review for M1

**Author:** Kevlin Henney (low-level reviewer)
**Status:** REVISIONS REQUESTED. Two real Blockers, several Suggestions, some Praise. Build is clean (`gofmt -l .` empty, `go vet ./...` clean, `go test ./...` green, `go generate ./...` idempotent on tracked files), but two surfaces meaningfully diverge from the docs and the plan.

---

## Half 1 — Code review

### Blockers

**B1. `decloud --help` cannot run on a fresh install.**
`cmd/decloud/main.go:14-18` calls `logging.Init()` *before* it parses any flag — including `--help`. `logging.Init()` (`internal/logging/logging.go:14-32`) unconditionally `os.MkdirAll(<root>/logs, 0755)`, which fails with `permission denied` against the default `/opt/decloud/logs` until the operator has already done step 4 of `_docs/install.md`. Result: a fresh-install operator runs `decloud --help` per `install.md` §6 and gets `Exit 70: logging init failed: mkdir /opt/decloud: permission denied`. The verify step is broken.

Reproduction (no env vars):
```
$ ./decloud --help
logging init failed: mkdir /opt/decloud: permission denied
exit status 70
```

Fix options, in order of preference:
1. Move `logging.Init()` into a `cobra.Command.PersistentPreRunE` on the root, so `--help` short-circuits before init runs. (Cobra's help/completion subcommands run without PreRunE.)
2. Make `logging.Init()` fall back to stderr-only when `MkdirAll` returns `EACCES`/`ENOENT`, with a one-line stderr warning. Don't fail-stop the binary because the operator hasn't created `/opt/decloud/logs/` yet.
3. Defer log-file open until first write.

Either #1 or #2 unblocks the install doc's verify step.

**B2. `--env-file` default behaviour is documented but not implemented.**
`internal/cli/deploy_service.go:53` registers `--env-file` with help text `"path to env.sh (default: <source-dir>/env.sh if present)"`, and `_docs/usage.md` §1 quick-start writes a `./myservice/env.sh` and runs `decloud deploy service --name myservice --host ... ./myservice` *with no `--env-file` flag*. But there's no auto-discovery anywhere — `runDeployService` (`deploy_service.go:84`) passes `f.EnvFile` straight through, `Deploy` uses `req.EnvFile` straight through (`service.go:131`), and `bashCapturer.Capture` (`internal/envcap/capture.go:35-38`) does `os.Stat(scriptPath)` immediately. With an empty path the deploy fails:
```
env.sh: stat : no such file or directory
```
And the doc's "optionally, an `env.sh`" promise is also broken — a service with no `env.sh` cannot deploy at all, because `Capture` is unconditional in the current `Deploy` flow.

Two fixes are needed:
1. `runDeployService`: when `f.EnvFile == ""`, default to `filepath.Join(abs, "env.sh")` if it exists.
2. `Deploy`: when the resolved env file is empty/absent, skip the `Capturer.Capture` call entirely and pass an empty `Env` map to `Driver.Run`. (Or have `Capture("")` return `nil, nil` rather than erroring.)

Either way, the help-text default and the quick-start example must actually work.

### Suggestions

**S1. Seven near-duplicate lifecycle command files.**
`internal/cli/{unregister,start,stop,restart,status,logs,caddy_reload}.go` are mechanical copies — same imports, same `lifecycleFactory(config.NewPaths(rc.ConfigRoot))` call, same `if err != nil { return fmt.Errorf("building lifecycle: %w", err) }`, same single-line delegation to one `lc.Method(...)`. A two-line helper would shrink each file to the bit that actually varies (the Cobra command shape and the method call):

```go
func withLifecycle(rc *rootContext, run func(deploy.Lifecycle) error) error {
    lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
    if err != nil {
        return fmt.Errorf("building lifecycle: %w", err)
    }
    return run(lc)
}
```

Each command would lose 4-5 lines of identical boilerplate. `status.go` formatting and `logs.go` flag wiring stay where they are. Not a blocker — the duplication is honest and obvious — but Kevlin's first instinct is "extract the verb."

**S2. `NewHTTPProbeForTest` is a production constructor named for tests.**
`internal/deploy/readiness.go:20` exports `NewHTTPProbeForTest(driver) ReadinessProbe` as the only public way to construct an HTTP probe. The `_ForTest` suffix telegraphs "do not call from production," yet production code in `service.go:112` constructs the unexported `newHTTPProbe` directly. So the only place the `_ForTest` constructor is *callable* from is tests — but tests `package deploy_test` don't currently call it (they inject a fake `Probe` via `Dependencies`). The exported constructor is thus dead API with a confusing name.

Either delete it (the `Dependencies.Probe` injection seam is sufficient), or rename it to `NewHTTPProbe` and remove the `newHTTPProbe` lowercase wrapper. Don't ship "ForTest" in a public symbol.

**S3. `else` after `if-return` in the readiness probe.**
`internal/deploy/readiness.go:49-58`:
```go
if ipErr == nil && ip != "" {
    url := fmt.Sprintf(...)
    if err := p.probeOnce(ctx, url); err == nil {
        return nil
    } else {
        lastErr = err
    }
} else if ipErr != nil {
    lastErr = ipErr
}
```
Two issues. First, `else` after `return` is the canonical Go nit — `golint` flags it. Drop the `else`, assign `lastErr = err` straight after the `if`. Second, the `ipErr == nil && ip == ""` case (Driver returned no error but empty IP — distinct from `ErrNoBridgeIP`) silently falls through with `lastErr` unchanged. Not a real bug today (Driver returns `ErrNoBridgeIP` on empty), but the dead branch will lie if someone changes the contract later.

**S4. Lossy error wrapping with `%w: %v` instead of `%w: %w`.**
Throughout `internal/deploy/service.go` and `internal/registry/store.go` errors are wrapped as `fmt.Errorf("%w: %v", ErrFoo, innerErr)`. Go 1.20+ supports two `%w` verbs in one Errorf — `fmt.Errorf("%w: %w", ErrFoo, innerErr)` keeps the inner chain intact for `errors.Is`/`errors.As` callers downstream. The current pattern flattens the inner error to a string. Today no caller needs the inner chain, but turning the outer-only wrap into a chained wrap is a five-character change per call site that pays back the first time anyone wants to test "did this fail because of `dockerdrv.ErrContainerNotFound` specifically?"

Concrete count: `service.go` has 9 sites; `store.go` `Save` has 3 sites in the partial-write paths; `lifecycle.go` has 7 sites; `readiness.go` has 1.

**S5. `regenerateAndReload` writes a stub it always overwrites.**
`service.go:295` calls `caddy.WriteStubIfMissing(...)` and then immediately renames a fresh tmp file over the same path on line 306. The stub-write is dead in this code path: any time we reach line 306, the rename obliterates what `WriteStubIfMissing` just wrote. The stub only matters if a deploy aborts between line 295 and line 306 (generator fail, validate fail). That window is narrow but real — so this isn't strictly wrong, just non-obvious. A two-line comment explaining "stub is here so a generator/validate failure leaves a parseable Caddyfile for systemd" would earn its keep.

**S6. `assert.True(t, errors.Is(err, X))` should be `assert.ErrorIs(t, err, X)`.**
Several CLI tests (e.g. `deploy_service_test.go:85`, `:98`) use the verbose form. Testify ships `assert.ErrorIs`/`require.ErrorIs` for exactly this. Search-and-replace, no semantic change.

**S7. Test-name drift from plan.**
The plan §3.1 listed `TestLifecycle_RestartFromAbsentReturnsErrNotFound`; Rob's implementation actually re-runs the container from `prev.Config.Build.ImageRef`, so Kent renamed the test to `TestLifecycle_RestartFromAbsentReRunsContainer` to match real semantics. The new name is more truthful than the plan name. Note this in Don's PLAN-redux so Linus doesn't flag it as a missing test.

### Praise

- **One struct, two interfaces.** `serviceDeployer` implementing both `ServiceDeployer` and `Lifecycle` from one shared `Dependencies` is the right call — Joel raised this as an open question, and the result is exactly seven extra methods on the same receiver, no duplication of constructor wiring.
- **`regenerateAndReload` extraction.** The shared private helper between `Deploy` step 8, `Unregister`, and `CaddyReload` is textbook DRY without over-abstracting. The validate-before-rename ordering is correct in all three call sites because there's only one site.
- **`writeAtomic` discipline.** Both `internal/registry/store.go:261-286` and `internal/caddy/generator.go:80-104` use `os.CreateTemp` in the destination dir → write → close → chmod → `os.Rename`, which is the only way to actually get same-filesystem atomicity. The `os.Remove(tmpPath)` cleanup on every error path is correct and not skipped.
- **`gomock.InOrder` where the order matters; plain expectations where it doesn't.** Kent applied this judgment correctly — over-specifying ordering would have made tests fragile.
- **`internal/envcap` portability.** The hermetic `/usr/bin/env -i ... /bin/bash --noprofile --norc -c '... compgen -e ... printf %s=%s\0 ...'` snippet, baseline-diff, and bash-internals stripping is precisely the M1 bash-3.2 fix the plan called for. The three explicit edge-case tests (`SetAOff`, `ArrayDeclaration`, `ReadonlyConflict`) are present.
- **`ContainerName` single-source.** Every call site in `internal/deploy/{service,lifecycle}.go` and `internal/cli/...` routes through `ids.ContainerName`. M4's rename will be one function-body change.

### Rob's four deviations — verdicts

1. **`Probe ReadinessProbe` field on `Dependencies`** — defensible. Without it, deploy tests that mock `Driver.ContainerIP(...).Return("172.18.0.5", nil)` exactly once would either time out or panic on unexpected real HTTP calls. Production wiring leaves it nil → real probe is constructed. The seam is small, well-commented, and doesn't leak. **Approve.**

2. **`fsStore.Load` overrides `cfg.Name` from filename basename** — defensible but undocumented in the plan. Filename is the canonical key; the in-file `name` is informational. The deviation is small and `_ai/decisions/` should pick up a one-line note that filename wins. **Approve, ask Raymond to record it.**

3. **`ErrUnknownField` wrap on non-strict secrets decode errors** — defensible. pelletier returns `*toml.StrictMissingError` only for top-level structs; inside `map[string]string` an unknown-type assignment surfaces as a plain decode error. Mapping both to `ErrUnknownField` keeps the exit-code mapping coherent (both are operator typos in TOML). **Approve.**

4. **`isCobraUsageError` substring matching** — pragmatic. Cobra's `MarkFlagRequired` returns `errors.New("required flag(s) ...")` with no typed sentinel; substring matching is the only path to map it to `ExitUsageError` short of forking Cobra. The five substrings are anchored enough that false positives are unlikely. Worth one extra test: a bare `errors.New("required flag")` (no service-related string) returns `ExitUsageError`, to lock in the contract. **Approve.**

### Race conditions / atomicity

- **`cmdFactory` swap** — not a race. Tests don't `t.Parallel()` (the package-global comment in `deploy_service.go:18-22` is explicit and Kent's tests respect it). The `cli_driver` and `cliReloader` factory fields are set at construction and never reassigned, so no race in production.
- **`writeAtomic`** — atomicity holds for both `registry.Save` (tmp+rename per file) and `caddy.Generator.Generate` (tmp+rename for the Caddyfile). The "config wrote, secrets failed" partial-write case is the documented `ErrPartialWrite` and the orchestrator calls `DeleteOrphanConfig`. Not atomic across the *pair* of files — which is what `ErrPartialWrite` exists to handle. Plan-aligned.

### Test coverage gaps

None worth blocking on. The 144 tests cover every behavior path in the plan. Two minor gaps:
- No test for `NewHTTPProbeForTest` actually being called from anywhere (because nothing calls it — see S2).
- No test for `decloud --help` not requiring filesystem access (because B1 means it currently requires it).

---

## Half 2 — Doc hallucination review

I verified every flag, default, exit code, state value, format string, and example against the shipped code (not against Joel's plan).

### Hallucinations / inaccuracies

**H1. Install doc step 6 verify step doesn't work.** See B1. `_docs/install.md:124` says `decloud --help` and lists expected subcommands. Won't run on a fresh install. Either fix the binary (B1) or change the doc to instruct the operator to set `DECLOUD_LOG_TO_STDERR_ONLY=1` (and document that env var, which Rob's report says we shouldn't expose to operators).

**H2. Quick-start example silently requires `--env-file`.** See B2. `_docs/usage.md` §1 invokes `decloud deploy service --name myservice --host ... ./myservice` with an `env.sh` in `./myservice/`, no `--env-file` flag. The flag's help text claims that's the default. Both are aspirational — the code doesn't auto-discover. Fix the code (B2) or fix the doc to add `--env-file ./myservice/env.sh`.

### Verified clean

- **Every flag** in `_docs/usage.md` §2 table matches `internal/cli/deploy_service.go` and `logs.go` exactly: `--name`, `--host` (string-slice), `--port`, `--env-file`, `--readiness-path` (default `/healthz`), `--readiness-timeout` (default `60s`), `--strategy` (default `recreate`), `--dockerfile` (default `Dockerfile`), `--mount` (rejected), `-f/--follow`, `--tail`, `--config-root`. Confirmed via `decloud deploy service --help` against built binary.
- **Every exit code** (0, 2, 10, 20, 30, 40, 50, 60, 70) matches `internal/cli/exit_codes.go` constants and `ExitCodeFor` mapping.
- **Status format string** in `_docs/usage.md` §4.1 (`<name> state=<state> container=<container-name> deploy=<deploy-id> deployed_at=<RFC3339>`) is byte-identical to `internal/cli/status.go:25-26`.
- **State values** `running`, `stopped`, `absent`, `config-only` all appear in `internal/deploy/lifecycle.go::Status` (`exited→stopped` mapping at line 105; `config-only` at line 93).
- **Container naming** `decloud-<name>` matches `internal/ids/ids.go::ContainerName`.
- **Deploy ID format** `YYYYMMDD-HHMMSS-XXXXXX` (six-hex) matches `internal/ids/ids.go::NewDeployID`. The example `20260426-093214-7f3a9c` parses cleanly. Raymond's noted self-catch was real — the format is now right.
- **`docker exec` debug paragraph** (`_docs/usage.md` §6) — accurate: ports are not published (`internal/dockerdrv/cli_driver.go::Run` never appends `-p`), Caddy reaches containers by container name on the `decloud` network. The example command `docker exec -it decloud-myservice sh` works given Rob's actual networking.
- **License sentence** in `_docs/install.md` §7 is the verbatim plan-§2.2.1.7 wording.
- **Path layout** in install.md §4 matches `internal/config/paths.go::NewPaths` exactly.

### Minor doc nits (non-blocking)

- **N1.** `_docs/install.md` §3 systemd unit's `ExecReload` includes `--force`, but `decloud caddy reload` shells out to `caddy reload --config <path>` without `--force` (`internal/caddy/reloader.go:38-48`). Two different reload paths exist; only one uses `--force`. Either harmonize or note the difference.

- **N2.** `_docs/usage.md` §3 row for exit 40 lists "docker stop" among the commands that can fail with 40, but `lifecycle.go::Stop` actually maps `ErrContainerNotFound` to `registry.ErrNotFound` → exit 10, not 40. Only true driver-level failures (non-existent docker, daemon down) hit 40 from a `Stop` call. Truthful but easy to misread.

---

## Verdict

**NEEDS REVISION.**

Two real Blockers (B1: `--help` requires writable `/opt/decloud`; B2: `--env-file` default is fictional) plus two doc hallucinations that ride on those Blockers. Fix the code; the docs follow without rewrites. Suggestions S1-S7 are quality-of-life and can wait for an M1.x cleanup pass.

**Top 3 issues:**
1. **B1** — `decloud --help` fails on fresh installs (`logging.Init` mkdir).
2. **B2** — `--env-file` auto-discovery and "env.sh optional" are documented but not coded.
3. **S1** — Seven near-identical lifecycle command files crying out for a 5-line helper.

Praise where due: the orchestrator sequencing, `regenerateAndReload` factoring, atomic-write discipline, and envcap portability are all clean Pike-ish work. Tests are well-structured Testify+Gomock with appropriate `gomock.InOrder` discipline.

End of Kevlin review.
