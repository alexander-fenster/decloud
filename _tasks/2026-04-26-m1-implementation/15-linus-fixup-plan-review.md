# Linus — review of `14-fixup-tech-plan.md`

## VERDICT: APPROVED. Kent and Rob proceed.

Joel addressed all 10 of Don's punch-list items with exact file/line precision. Spot-checked his line citations against the live tree:

- Item 6 (`%w: %v` sites): `grep -rEn '%w:.*%v' internal/` returns exactly 21 matches. Every line number Joel cited (service.go: 135, 156, 166, 171, 188, 211, 300, 304, 307, 310; lifecycle.go: 43, 56, 63, 76, 100, 130; store.go: 140, 143, 147, 252; readiness.go: 61) matches the live source. Count is right; sites are right.
- Item 5 (readiness.go cleanup): the proposed `switch` collapses BOTH the outer `else`-after-return AND the silent `ipErr==nil && ip==""` branch in one rewrite. The new `default` branch's inverted check (`if err != nil { lastErr = err } else { return nil }`) is acceptable Go — not a fresh `else`-after-return.
- Item 1 (logging): `cmd/decloud/main.go:14-18` confirmed broken. Joel's `setStderrOnly()` helper, `PersistentPreRunE` deferral, and dropping the `logging` import from main are all coherent and compile.
- Item 2 (env-file): the auto-discovery precedence (`flagValue != "" → stat-or-error`; `flagValue == "" → stat candidate, fall through to ""`) is unambiguous. Docs only mention `env.sh` — no `env.bash` precedence ambiguity exists.
- Code snippets are real Go, not pseudocode. Symbol references (`dockerdrv.ErrNoBridgeIP`, `envcap.ErrEnvScriptMissing`, `registry.ErrNotFound`) all exist or are introduced in the same file.

The 15-test `service_test.go` migration list is accurate (I counted 15 `Deploy(ctx, newRequest())` callers). The `logging_test.go` inversion tests the genuine new contract (graceful fallback), not papering over a bug — the OLD assertion encoded the bug Don is fixing.

## Decisions on Joel's three punted questions

1. **Exit code for `NetworkEnsure` failure → APPROVE 40 (`ExitRunFail`).** Don's punch list said "60 is open" but `internal/cli/exit_codes.go:19` shows 60 IS `ExitCaddyReloadFail`. Joel caught Don's slip and correctly mapped to 40. NetworkEnsure IS a docker driver call; 40's existing description trivially extends to `docker network create`. No new sentinel, no usage.md churn beyond Item 8's already-planned edit. **Joel's call stands.**

2. **`Capture("")` returns `(nil, nil)` defensively → APPROVE.** A panic on empty path would be cathartic but wrong here. The orchestrator has the explicit `if envFile != ""` guard (Item 2's service.go rewrite), so Capture is never called with `""` from production. The defensive return is a belt-and-suspenders against future callers, with a clear contract comment. Panicking would make the seam more brittle for future callers without making the bug class easier to find — Item 2 already eliminates the original bug class at the orchestrator level. **Joel's call stands.**

3. **`NetworkEnsure` only in Deploy, not lifecycle paths → APPROVE WITH FOLLOW-UP NOTE.** `CaddyReload` and `Unregister` don't run new containers; they don't need the network. But `Start` from absent state (`lifecycle.go:66-78`, the `default` branch) DOES run a fresh container on `decloud` network — a missing network there fails opaquely. `Restart` inherits the same gap. Don's punch list explicitly limited the wiring to Deploy, so Joel followed instructions. **Approved as-scoped, but Don should add to M1.x backlog: "wire `NetworkEnsure` into `Start`'s absent→Run branch."** Not a blocker for this round.

## Anything dangerous Joel hand-waved

- **Cobra completion subcommands and PersistentPreRunE.** Joel flagged this in Risk Areas #2. Confirmed via cobra issue #1543 that completion paths can interact oddly. The mitigation (`TestRoot_HelpDoesNotRequireFilesystem`) only exercises `--help`, not `__complete`. If completion breaks in execution, that's a small follow-up — not worth blocking on now. Acceptable.
- **`Start`-from-absent network gap.** Captured above; not a blocker.

Nothing else dangerous. The plan is mechanical, surgical, and well-enumerated. Ship it.

## Final word

Joel's plan is the cleanest delta-spec we've had this whole task. File/line precision, migration concerns called out per item, an exact 21-site count for the `%w` fix, and even caught Don's exit-code slip without making a fuss about it. Don's punch list was tight; Joel's expansion is tighter.

Kent: write the 13 mandatory new tests. Rob: implement items 1-6 in order. Raymond: ship items 7-9. Don't deviate from Joel's file/line spec without flagging.

Linus signs off. Proceed to Step 3.

Sources:
- [cobra issue #1967 — `--help` bypasses PersistentPreRunE](https://github.com/spf13/cobra/issues/1967)
- [cobra issue #1543 — PersistentPreRun/completion interactions](https://github.com/spf13/cobra/issues/1543)
