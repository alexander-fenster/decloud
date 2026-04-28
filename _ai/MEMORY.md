# _ai library index

Tactical reference for the Decloud codebase. Each file is a dense decision record or a non-obvious gotcha. Keep entries one-line where possible; point at code or task files for detail rather than re-explaining.

## Architecture decisions

- `decisions/m1-scope.md` — M1 = server-side `decloud deploy service` with `recreate` strategy; why "client first" / "bootstrap first" / "jobs first" were rejected; full M1→M7 milestone sequence.
- `decisions/secrets-split.md` — config TOML mode 0644 + `secrets/<name>/env.toml` mode 0600 in 0700 dir; load/save/delete ordering that produces only the recoverable "config-without-secrets" failure mode.
- `decisions/schema-versioning.md` — `pelletier/go-toml/v2` strict mode + `schema_version` integer; bump only on semantic breaks, never preemptively; M1/M2/M7 all write version 1 (mounts populate at M2, secret-files at M7).
- `decisions/m1-test-strategy.md` — M1 ships unit-tests-only per maintainer directive; Gomock for `Store`/`Capturer`/`Driver`/`Generator`/`Reloader`, real bash for `internal/envcap`, ten-item handoff receipt is the manual-CI bridge until GitHub Actions lands.
- `decisions/no-magic-zero-modes.md` — `--port=0` rejected at validation, NOT treated as "worker mode"; M5 workers get a separate `deploy job` command. Why folding workload shapes into one command via magic values produces 200-line if-else trees.
- `decisions/caddy-runs-in-container.md` — Caddy is `decloud-caddy` on the `decloud` Docker network, not a host systemd unit; why every host-side variant (host.docker.internal, --network host, /etc/hosts injection, resolvers 127.0.0.11, dnsmasq, sidecar) was rejected; dual-stack publishing, named-volume ACME state, deploy-failure recovery contract.

## Implementation patterns (reusable)

- `cobra-init-pattern.md` — `PersistentPreRunE` for filesystem-touching init + EACCES/ENOENT graceful fallback; what saves `decloud --help` from exit 70 on a fresh box.
- `explicit-inputs-not-globals.md` — `logging.Init(root string) error` takes the resolved root as a parameter, does NOT re-read `DECLOUD_ROOT`. Why structural contracts beat procedural ones (setter pattern, viper read, env-fallback all rejected).
- `cli-flag-surface-coherence.md` — every CLI flag has four surfaces (runtime check, error string, `--help` text, `_docs/usage.md`); change one, audit all four. The bug class the review-findings task fixed AND nearly re-introduced.
- `error-wrap-discipline.md` — `%w: %w` not `%w: %v`; grep recipe + the regression test that locks it in (`TestDeploy_BuildErrorPreservesInnerSentinel`).
- `optional-input-two-layer.md` — leaf-consumer defensive return + orchestrator guard, both layers, no coupling; the `Capture("")` / `if envFile != ""` shape generalizes.
- `gomock-inorder-sequencing.md` — pin orchestrator step ordering with `gomock.InOrder`; contract test not implementation test.
- `cleanup-context-discipline.md` — orchestrator cleanup MUST run on a `context.Background()`-derived bounded context, never the caller's request ctx; otherwise SIGINT cancels the cleanup it triggered. Helper: `newCleanupContext()` in `internal/deploy/service.go`.
- `label-gated-orphan-recovery.md` — recover orphaned named artefacts on next run by gating on a creator-set label, not on the name alone; `decloud.service=<name>` plus the `Inspect → state+labels` JSON shape.
- `exit-code-sentinel-not-context-err.md` — CLI exit-code mapping matches the package sentinel (`deploy.ErrInterrupted`) only; bare `context.Canceled` / `context.DeadlineExceeded` route to `ExitInternal` and the negative test cases lock that contract.
- `gomock-fifo-matching.md` — `go.uber.org/mock` matches expectations FIFO, not LIFO; harness `AnyTimes()` defaults need an explicit opt-out option for tests that want a different response.
- `cancellation-symmetry-audit.md` — when fixing a `context.Canceled` mis-wrap at one site, audit every sibling forward-progress branch on the same request ctx; Linus's iter2 Issue 1 was the §3.4.5 sibling caught only on impl re-review, not in the v2 plan pass.

## Implementation gotchas

- `envcap-portable-bash.md` — the macOS-bash-3.2-portable env.sh capture mechanism; why `compgen -e` + `${!name}` + `printf '\0'` and NOT GNU `env -0` (which silently no-ops on BSD env).
- `container-naming.md` — `decloud-<name>` in M1, `decloud-<name>-<deploy-id>` from M4; the rename is an explicit M4 deliverable, route all naming through one helper.
- `docker-bridge-dns.md` — `decloud-<x>` short names only resolve from inside the `decloud` user-defined bridge; host processes fall through to host resolver and dial the host's own AAAA. Class-of-bug that shipped as M1.0 Caddy. Also: when a tech plan corrects an architectural assumption, promote it to `_ai/decisions/` — corrections buried in tech plans don't get audited against new code.

## Review discipline

- `doc-grep-discipline.md` — when `_docs/*.md` shows a literal error string, `grep -F` it against the source. Two M1 doc-fab incidents (`install.md:173`, `:189`) both showed renderings that didn't match the wrap chain the code actually emitted. If the bytes are genuinely variable, frame them as variable — don't fabricate a clean example. Extension: applies to slog messages quoted in operator runbooks too (`usage.md:235`/`:237` drift across deploy-cleanup iter2).
- `fix-now-while-fresh.md` — Don's repeated rule for in-task defects: fix in scope when mechanical + same-file + <5-minute floor + on-theme; defer when it requires new architecture or a different package's review surface. Captures the lockdown rationale across deploy-cleanup v2.1 and v2.2.
- `stderr-substring-canary.md` — branching on a third-party tool's stderr is fundamentally brittle; the mitigation is a canary test that fails loudly when the upstream wording shifts. Match canonical strings only, co-locate detection with its single caller (not the driver), lock with sub-tests-per-substring + a negative branch assertion. Live example: `isPortsBoundErr` in `internal/caddy/manager.go`.

## Cross-references for shapes worth borrowing

- **`PortMap.HostBind` as a first-class field, not a string-formatting trick.** Dual-stack publishing (`0.0.0.0` + `[::]`) is six PortMap entries with explicit host-bind, NOT a `formatPortMap` that conditionally brackets IPv6. The formatter splices verbatim and the doc-comment forbids auto-bracketing. See `internal/dockerdrv/driver.go::PortMap` and `cli_driver.go::formatPortMap`, locked by `TestFormatPortMap_DoesNotAutoBracketIPv6`. Rationale lives in `decisions/caddy-runs-in-container.md`.

## Backlog

- `m1x-backlog.md` — five items deferred from M1 with explicit Don/Linus sign-off (NetworkEnsure-in-Start gap, lifecycle-command dedup, `assert.ErrorIs` migration, `Capture("")` comment, log-warning quiet mode).

## Source-of-truth task artefacts

When a decision record points back at planning detail, the canonical source is the task directory:

- `_tasks/2026-04-26-readme-implementation-planning/05-plan-v2.md` — Don's M1 plan.
- `_tasks/2026-04-26-readme-implementation-planning/06-tech-plan-v2.md` — Joel's M1 tech plan.
- `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md` — Linus's approval, including borderline-but-not-blocker edge cases on env capture.
- `_tasks/2026-04-27-caddy-container-connection-refused/005-don-plan-v2.md` — root-cause analysis (host Caddy can't resolve `decloud-<x>`) and the Caddy-in-container fix.
- `_tasks/2026-04-27-caddy-container-connection-refused/004-linus-review.md` — enumeration of the seven rejected alternatives (`host.docker.internal`, `--network host`, `--network container:`, sidecar, `/etc/hosts` injection, `--resolvers 127.0.0.11`, host-local `dnsmasq`).
- `_tasks/2026-04-28-milestone-resequence/` — 2026-04-28 maintainer-priority resequence: M2/M3 swap, M3b client deferred to M7, secret-files-on-disk deferred to M7. Doesn't change M1, M4, M5, M6 in content.
