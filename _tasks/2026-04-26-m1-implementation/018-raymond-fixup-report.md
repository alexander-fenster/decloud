# Iter2 Raymond fix-up report

**Author:** Raymond Chen (doc writer)
**Status:** Pass. Three deliverables written, every claim verified against shipped code.
**Spec:** Don's punch-list items 7, 8, 9 in `012-don-plan-check.md`; Joel's outline in `14-fixup-tech-plan.md`; Rob's iter2 notes in `017-rob-fixup-impl.md`.

---

## Files modified

| File | Change |
|---|---|
| `/Users/fenster/dev/decloud/_docs/install.md` | Dropped `--force` from the systemd `ExecReload` line (item 7). The `cliReloader.Reload` in `internal/caddy/reloader.go:38` shells out to `caddy reload --config <path>` with no `--force`; harmonizing the systemd unit with shipped code. |
| `/Users/fenster/dev/decloud/_docs/usage.md` | Item 8 plus three operator-facing clarifications driven by iter2 behavior changes. |
| `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md` | NEW. Item 9 — the missing decision-record file plan-v2 §2.1 and DONE-criterion #10 cite by name. |
| `/Users/fenster/dev/decloud/_ai/MEMORY.md` | Added the one-line index entry for the new decision memo, matching the existing `decisions/*.md` row format. |

`_docs/` ships as plain markdown (no Next.js build); the `cd _docs && next build` step from Raymond's instruction file does not apply here. Both files render directly on GitHub.

## Verification trail (every claim cross-referenced to code)

1. **`--force` mismatch (item 7).** Spec said line 43, fix was `--force`. Confirmed by reading `internal/caddy/reloader.go:33-49`: `Validate` and `Reload` both call `runCaddy(ctx, sub, configPath)`, which runs `caddy <sub> --config <configPath>` only. No `--force` flag is passed in either path. The systemd unit drop-in now matches.

2. **Exit-40 row (item 8).** Verified against `internal/cli/exit_codes.go:51` (`ErrRun → ExitRunFail = 40`) and `internal/deploy/service.go:131-134` (`Driver.NetworkEnsure` failure wraps as `ErrRun`). Confirmed `docker network create` belongs in the exit-40 row. Also confirmed by reading `internal/deploy/lifecycle.go:39-44` that `Driver.Stop` returning `ErrContainerNotFound` is mapped to `registry.ErrNotFound` (exit 10, not 40); the row now states this explicitly.

3. **Exit-10 row additions.** Confirmed three sources:
   - `internal/cli/deploy_service.go:103-110` — explicit `--env-file=<missing>` returns `envcap.ErrEnvScriptMissing`, which `exit_codes.go:44` maps to `ExitConfigError` (10).
   - `internal/deploy/lifecycle.go:40-42` — `Stop` against a missing container returns `registry.ErrNotFound` (exit 10).
   - `internal/deploy/lifecycle.go:51`, `:127-128` — `Start`, `Restart`, and `Logs` propagate `registry.ErrNotFound` for unknown services. Restart goes through `Stop` first; `Start` calls `Store.Load` which returns `ErrNotFound`. All three end up at exit 10.

4. **`env.sh` now optional in §1 and §2.** Verified by `internal/deploy/service.go:137-149` (the `if envFile != ""` branch skips `Capturer.Capture` and logs `env capture skipped: no env script`). The quick-start §1 now states the auto-discovery rule honestly; §2 step 1 says "skipped if no env script is in play"; step 0 (NEW) names the network-ensure as the very first thing the deploy does.

5. **`NetworkEnsure` step 0 documented in §2.** Step 0 added to the deploy sequence list since `internal/deploy/service.go:131-135` calls `Driver.NetworkEnsure(ctx, "decloud")` before envcap.

## Hallucinations caught and corrected

None in the iter2 deliverables. I read the four code paths Joel's plan implicates (`exit_codes.go`, `deploy_service.go`, `service.go`, `lifecycle.go`) plus `caddy/reloader.go` before drafting any sentence. The only nontrivial check was confirming that Rob's claim "Stop on missing container surfaces as exit 10" is accurate — `lifecycle.go:40-42` returns `registry.ErrNotFound` (exit 10) on `dockerdrv.ErrContainerNotFound`, which is what the doc now says.

## Operator-facing behavior changes that needed more than the spec'd doc fix

Three places where iter2 changed shipped behavior in ways the maintainer should see:

1. **`env.sh` auto-discovery is silent.** Joel's plan §item 2 mandates auto-discovery for an absent `--env-file`, with the deploy proceeding silently if `<source-dir>/env.sh` is missing. Operators who type `decloud deploy service ... ./myservice` with no env vars expected something to break; instead it just works, which is the right behavior but needs to be loud in §1. The new sentence in §1 ("auto-discovery is silent, but explicit asks must succeed") draws the line.

2. **Network self-heal is now a deploy step.** `Driver.NetworkEnsure` runs before envcap on every deploy. The install doc still tells the operator to `docker network create decloud` once during bootstrap (correct — gives a clear "alive" signal on first install), but the usage doc now mentions the self-heal so an operator who removed the network out of band understands why the next deploy still works.

3. **The exit-40 row was a documentation lie before iter2.** It listed `docker stop` as an exit-40 source; the shipped lifecycle path returns exit 10 for missing-container `Stop`. Even pre-iter2 the row was wrong. The fix is to be precise in both directions: list the four real exit-40 driver calls plus the new `docker network create`, and explicitly call out the exit-10 case so operators do not chase a phantom exit-40 bug.

The install doc did NOT need a paragraph on env.sh now being optional — install.md is a host-bootstrap document and never claimed env.sh was required. The usage doc was the right home for that change.

## What Linus and Kevlin should know

- The systemd `ExecReload` change (`--force` removed) is a behavior change for hosts that already installed Caddy from an earlier draft of `install.md`. Operators who copied the old unit will keep running `caddy reload --force`; that still works, just inconsistent with what the deployer does. Not a regression. Documented in the install doc; no migration note added because no released version of this doc has a Caddy host installed against it.
- The `m1-test-strategy.md` memo includes a §5 ("Local test-run tip") covering Rob's `DECLOUD_LOG_TO_STDERR_ONLY=1` note. That section is for the maintainer running tests locally, not for end-users.
- `_ai/MEMORY.md` index is the single source of truth for "which decision memos exist." Future agents discover the new memo through that index, not by `ls _ai/decisions/`.

End of report.
