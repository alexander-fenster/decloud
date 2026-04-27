# _ai library index

Tactical reference for the Declouding codebase. Each file is a dense decision record or a non-obvious gotcha. Keep entries one-line where possible; point at code or task files for detail rather than re-explaining.

## Architecture decisions

- `decisions/m1-scope.md` — M1 = server-side `decloud deploy service` with `recreate` strategy; why "client first" / "bootstrap first" / "jobs first" were rejected; full M1→M7 milestone sequence.
- `decisions/secrets-split.md` — config TOML mode 0644 + `secrets/<name>/env.toml` mode 0600 in 0700 dir; load/save/delete ordering that produces only the recoverable "config-without-secrets" failure mode.
- `decisions/schema-versioning.md` — `pelletier/go-toml/v2` strict mode + `schema_version` integer; bump only on semantic breaks, never preemptively; M1/M3 both write version 1.
- `decisions/m1-test-strategy.md` — M1 ships unit-tests-only per maintainer directive; Gomock for `Store`/`Capturer`/`Driver`/`Generator`/`Reloader`, real bash for `internal/envcap`, ten-item handoff receipt is the manual-CI bridge until GitHub Actions lands.
- `decisions/no-magic-zero-modes.md` — `--port=0` rejected at validation, NOT treated as "worker mode"; M5 workers get a separate `deploy job` command. Why folding workload shapes into one command via magic values produces 200-line if-else trees.

## Implementation patterns (reusable)

- `cobra-init-pattern.md` — `PersistentPreRunE` for filesystem-touching init + EACCES/ENOENT graceful fallback; what saves `decloud --help` from exit 70 on a fresh box.
- `explicit-inputs-not-globals.md` — `logging.Init(root string) error` takes the resolved root as a parameter, does NOT re-read `DECLOUD_ROOT`. Why structural contracts beat procedural ones (setter pattern, viper read, env-fallback all rejected).
- `cli-flag-surface-coherence.md` — every CLI flag has four surfaces (runtime check, error string, `--help` text, `_docs/usage.md`); change one, audit all four. The bug class the review-findings task fixed AND nearly re-introduced.
- `error-wrap-discipline.md` — `%w: %w` not `%w: %v`; grep recipe + the regression test that locks it in (`TestDeploy_BuildErrorPreservesInnerSentinel`).
- `optional-input-two-layer.md` — leaf-consumer defensive return + orchestrator guard, both layers, no coupling; the `Capture("")` / `if envFile != ""` shape generalizes.
- `gomock-inorder-sequencing.md` — pin orchestrator step ordering with `gomock.InOrder`; contract test not implementation test.

## Implementation gotchas

- `envcap-portable-bash.md` — the macOS-bash-3.2-portable env.sh capture mechanism; why `compgen -e` + `${!name}` + `printf '\0'` and NOT GNU `env -0` (which silently no-ops on BSD env).
- `container-naming.md` — `decloud-<name>` in M1, `decloud-<name>-<deploy-id>` from M4; the rename is an explicit M4 deliverable, route all naming through one helper.

## Backlog

- `m1x-backlog.md` — five items deferred from M1 with explicit Don/Linus sign-off (NetworkEnsure-in-Start gap, lifecycle-command dedup, `assert.ErrorIs` migration, `Capture("")` comment, log-warning quiet mode).

## Source-of-truth task artefacts

When a decision record points back at planning detail, the canonical source is the task directory:

- `_tasks/2026-04-26-readme-implementation-planning/05-plan-v2.md` — Don's M1 plan.
- `_tasks/2026-04-26-readme-implementation-planning/06-tech-plan-v2.md` — Joel's M1 tech plan.
- `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md` — Linus's approval, including borderline-but-not-blocker edge cases on env capture.
