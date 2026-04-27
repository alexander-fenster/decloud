# 007 — Linus Review v2: containerised Caddy plan

Author: Linus Torvalds (high-level review agent)
Date: 2026-04-27
Reviews: `005-don-plan-v2.md` (Don) and `006-joel-tech-plan-v2.md` (Joel) against `004-linus-review.md`'s seven required revisions.

---

## TL;DR

All seven revisions are folded in correctly, not just mentioned. The shapes are right — in particular, the dual-stack publishing is at the type level (`PortMap.HostBind`) instead of bolted on, and the no-rollback contract is enforced by an explicit test (`TestManager_UpRunFailsWithoutRollback`) rather than left as absence. Don and Joel did the work.

**Verdict: APPROVED.**

Non-blocking nits below for Kent / Rob / Raymond. None of them gate execution.

---

## Revision-by-revision audit

### #1 Readiness loop cut — CONFIRMED

Don §4.2 and Joel §4.2 both spell out a five-step `Up` algorithm with no probe. Joel §9.3 articulates exactly why the race is architecturally pre-empted (the deploy flow itself spends 5–60s before invoking the reloader). Joel also names the right home for a future retry if it ever materialises — `cliReloader.execCaddy`, not `Manager.Up`. Joel-v1's §11.3 is gone; Joel-v2 §6.1's "Tests explicitly NOT in v2" list calls out `TestManager_UpReadinessProbe*` by name as deleted.

### #2 Rollback cut — CONFIRMED, AND HARDENED

Don §4.2 explicitly: "If any step fails, return the wrapped error. No cleanup." Joel §4.2 mirrors. What I asked for was the cut; what Joel did was the cut **plus** add `TestManager_UpRunFailsWithoutRollback` (§6.1) which asserts that **no** subsequent `Stop`/`Remove` is called when `RunWithOptions` errors. Locking the no-rollback contract with a positive assertion is better than my original ask.

### #3 `--image` / Viper / TOML cut — CONFIRMED

Don §4.1 cites `_ai/decisions/m1-scope.md:18` directly. Joel §1.1 says "zero flags." `ManagerConfig` (§4.1) does not carry an `Image` field; the Note explicitly says "If M2 introduces Viper-driven overrides, the field gets added then." `caddy.DefaultImage = "caddy:2"` is the only image source (§5). `TestCaddyUp_NoFlags` (§6.4) locks the contract — passing `--image foo` produces a usage error.

`_ai/cli-flag-surface-coherence.md` is correctly marked "NO CHANGE in this task" (§8.6). Good — that's exactly the doc that would have been falsified by a quiet `--image` re-introduction.

### #4 Dual-stack IPv6 publishing — CONFIRMED, AT THE RIGHT LAYER

This was the one I was most worried about being implemented as a string-list hack. It isn't. Joel §3.1 introduces `HostBind` as a first-class field on `PortMap`. Two `PortMap` entries express dual-stack — one per stack — and `formatPortMap` (§4.8) splices `HostBind` literally so the IPv6 brackets `[::]` flow through unchanged. The argv emission (§3.3) preserves declared order so the test reads exactly like the operator's mental model.

The six-entry `Ports` slice (§3.2) hits all three protos × both stacks. Two tests lock it: `TestCLIDriver_RunWithOptionsCaddyShape` for the end-to-end shape and `TestCLIDriver_RunWithOptionsDualStackPorts` for the dual-stack semantics independent of Caddy. The IPv6-disabled-host failure mode is explicitly named (Don §4.3 footnote 1; Joel §1.5 row 4) — loud failure, recognisable error text, install doc names it. That's the right tradeoff.

Joel §9.9 defends `formatPortMap` against future temptation to auto-bracket IPv6. Pre-empting that bug is the kind of detail v1 missed.

### #5 Deploy error text — CONFIRMED

Joel §4.9 gives exact wrap text on both validate and reload legs: "service is registered and running but Caddy is not routing traffic; run 'decloud caddy up' (and then 'decloud caddy reload' if needed) to restore routing." `%w: %w` chain preserved; no new sentinel. The reloader's actionable error ("container 'decloud-caddy' is not running; run 'decloud caddy up' first") composes cleanly through the inner `%w`.

The duplication-vs-constant tradeoff is named explicitly in §4.9 ("two sites is below the rule-of-three threshold... if a third site appears, refactor"). I'd have made the same call.

§6.6 confirms the existing `errors.Is(err, ErrCaddyReload)` property holds after the change, which is exactly the regression I'd worry about.

### #6 `cmdFactory` deleted — CONFIRMED

Joel §2.2 explicitly: "Delete `cmdFactory` field, `newCLIReloaderWithFactory`, and the package-private `cmdFactory` type alias." §4.5 shows the resulting `cliReloader` struct with only `driver` and `hostCaddyDir`. §6.2 drops `recordingFactory` and `failingFactory` helpers. Acceptance criterion #21 (Joel §12) locks the deletion at sign-off. No way for `cmdFactory` to sneak back in.

### #7 ACME migration warning strengthened — CONFIRMED

Don §4.6 makes the volume-copy recipe the **default**, not the alternative. The cold-restart is gated on "1-2 hostnames or no production traffic" — the right default flip. The 7-day LE rate-limit recovery window is named in the doc text (Don §4.6, Joel §8.1). The error-text update for ports-already-bound names `systemctl mask` and `apt-get remove` (Don §4.6 second half; Joel §1.5 row 1) — `disable --now` alone is correctly called out as insufficient against package upgrades.

The recipes live inline in the install doc (Joel §8.1), with the full rationale duplicated in the decision record (§8.3 item 7). That's the right split — operators don't read decision records.

---

## Things I checked beyond the seven revisions

### Phasing and Raymond's decision-doc draft

Joel §7 puts Raymond's draft of `_ai/decisions/caddy-runs-in-container.md` in **Phase 4**, reviewed alongside the CLI surface — exactly per my §5.7 ask. Phase 6 finalises it. Kevlin's hallucination check has the draft as ground truth during implementation review, not retrospectively. Right sequence.

### `caddy.NetworkName` constant in new code

Joel §5 explicitly mandates the new manager, reloader, `caddy_up.go`, and `caddy_down.go` reference `caddy.NetworkName` rather than introducing a seventh `"decloud"` literal. The existing six literals' cleanup stays in M1.x backlog. Matches my §5.6 call.

### Reloader interface contract on the doc-comment

Joel §4.5 puts the bind-mount-path constraint as a doc-comment on the `Reloader` **interface**, not just on the implementation:

```
// IMPORTANT CONTRACT: configPath passed to Validate/Reload MUST be a host
// path inside the bind-mounted Caddyfile directory (Paths.CaddyDir). Paths
// outside that directory return an error with no exec attempt.
```

That's per my §5.4 ask. Future callers of the interface see the constraint without reading the impl.

### Path translation tests

Three tests for `translatePath`: `TestReloader_PathTranslationCanonicalForm` (positive), `TestReloader_PathTranslationOutsideBindMount` (negative), and `TestReloader_PathTranslationParentEscape` (negative with `..`). The third one I didn't ask for and Joel added anyway — locking down `/opt/decloud/config/caddy/../../etc/passwd` is exactly the kind of test that catches a regression where someone "simplifies" the `Rel` check.

### `usage.md:182` rewrite

Joel §8.2 specifies a **rewrite**, not a patch, with the exact replacement text. Per my §5.5. `_docs/install.md:61-62` paragraph specified as DELETED, not edited (Joel §8.1; Don §10 criterion #12). Per my §6 #3.

### `RunWithOptions` argv ordering

Joel §3.3 specifies declared-order for `Ports` and `Volumes`, sorted-key for `Labels` and `Env`. Independent test for declared-order (`TestCLIDriver_RunWithOptionsPortsDeclaredOrder`) prevents a later "let's sort them" cleanup from breaking the dual-stack pairing in argv-shape tests. Defensive, but correctly so.

### Acceptance criteria

Don §10 has 19 criteria; Joel §12 adds #20 and #21 specifically locking the driver interface signature and the `cmdFactory` deletion. Concrete, verifiable. The criteria include the manual `ss -tlnp` check for both `0.0.0.0` and `::` listeners (Don §10 #11; Don §7 step 5) — that's the regression test for dual-stack publishing on the actual host.

---

## Non-blocking nits

These do NOT gate execution. Listed for Kent / Rob / Raymond's awareness.

1. **`isNotRunningStderr` substring match is fragile.** Joel §4.5 matches `strings.Contains(strings.ToLower(s), "is not running")`. That's the Docker daemon's English error text. If Docker localises or rewords (unlikely but possible across Docker versions), the actionable error degrades to a generic exec-failed text. Acceptable — `ErrContainerNotFound` covers the absent case; this only matters for the `exited` case, and the operator still sees the underlying stderr. Flag for Kent's test (`TestReloader_ContainerExitedSurfacesActionableError`) which locks the current shape; if it ever fails on a Docker upgrade, the test tells us where to update.

2. **`ports already bound` substring detection.** Joel §1.5 says "best-effort substring matching against `docker run` stderr (`address already in use` or `port is already allocated`)." Same fragility class as #1. Same mitigation: an explicit test locks the current shape; if it breaks on a Docker upgrade, we know.

3. **Wrap-text duplication in `service.go:314-322`.** Joel §4.9 documents the choice not to factor the suffix into a constant ("two sites is below the rule-of-three threshold"). Fine. If a third call site ever appears (e.g., M4 blue-green adds a second reload point), refactor at that point, not before.

4. **Stdout shape on cold-start has two lines, warm paths have one.** Joel §1.2. Operators scripting around `caddy up` should match against the `caddy up:` prefix on cold-start lines but the bare `caddy already running` / `caddy started` on warm paths. Acceptable inconsistency — the warm paths don't need a verb prefix because the state transition IS the message — but Raymond's usage doc should mention it for operators who pipe to `grep`.

5. **`docker exec` does not pass `-i`.** Joel §4.6 explicitly chose not to pass `-i`/`-t`. Caddy's `validate`/`reload` are non-interactive so this is correct. If anyone ever extends `Driver.Exec` for an interactive use case (which we don't have in M1 or M2), the contract on `ExecOptions` will need to grow. Not a today problem.

6. **`PortMap` with empty `HostBind` is a contract-clean fallback only, not used in M1.** Joel §3.3 and §4.8 both call this out. The test (`TestCLIDriver_RunWithOptionsEmptyHostBind`) locks the behaviour. Fine for me; flag for Kent that this is testing a M2+ contract path.

7. **`_docs/usage.md` quick-start rewrite.** Joel §8.2: "If you have not yet, run `decloud caddy up` once to bring Caddy online." Belongs as the first step under §1, not buried later. Raymond's call.

---

## Why I'm approving rather than holding for v3

I held v1 because three of the seven items were architectural mistakes — the readiness loop was a wrong-layer fix, the dual-stack omission was a shipped IPv6 regression, and the Viper/TOML wiring was a scope violation. Those are the kinds of things that are cheaper to argue about in plan than to undo in code.

v2 fixes all three in their right form, not in a "minimum to satisfy the reviewer" form:

- The readiness loop isn't just removed; the no-rollback consequence is removed too, AND a positive test locks the no-rollback contract.
- Dual-stack isn't bolted on; `HostBind` is a first-class field on `PortMap` and the formatter respects it without auto-bracketing IPv6.
- The Viper/TOML cut isn't just absent code; the `ManagerConfig` struct documents that the field doesn't exist and `TestCaddyUp_NoFlags` locks "no flags" as a contract.

The remaining four (deploy error text, `cmdFactory` deletion, ACME migration warning, Reloader contract doc-comments) are smaller items that v2 also handled cleanly.

The bones were right in v1 and they remain right in v2, just trimmed. Kent and Rob can execute against this plan as-is. Raymond can write the install-doc rewrite and the decision record from Joel §8 directly.

— Linus

---

## Verdict

**APPROVED** — proceed to EXECUTION step.

Non-blocking nits above are FYI for Kent/Rob/Raymond, not gating items.
