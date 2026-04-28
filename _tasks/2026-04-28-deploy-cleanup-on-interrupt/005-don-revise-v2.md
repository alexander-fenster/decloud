# Don's revise-v2 coordination report

Status: planning iteration 2 complete. All six of Linus's REVISE items resolved. `02-plan.md` and `03-tech-plan.md` both revised in place with v2 markers and revision-history headers. Backlog entries added to `_ai/m1x-backlog.md` items 7 and 8.

Next step: Linus must re-review `02-plan.md` v2 + `03-tech-plan.md` v2 and confirm APPROVE. Until that happens, planning is not complete and Kent does not write tests.

---

## 1. What I did this round

1. Read all four prior task documents (`01-user-request.md`, `02-plan.md`, `03-tech-plan.md`, `04-linus-review.md`).
2. Verified Linus's claims by reading the actual code (`internal/dockerdrv/cli_driver.go`, `internal/dockerdrv/driver.go`, `internal/deploy/service.go`, `_ai/m1x-backlog.md`).
3. Decided each of the six issues with reasoning (not rubber-stamping).
4. Edited `02-plan.md` in place: §3.2 (probe wrap), §3.3 (label-gated orphan), and new §12 (review resolutions).
5. Edited `03-tech-plan.md` in place: §3.3, §3.4.1, §3.5 (with new §3.5.1 for driver interface widening), §3.6, §4.4, §5.8, §5.9, plus new §5.10 (label-mismatch tests) and new §5.11 (driver-level Inspect parsing tests). Added §12 v2 summary at the end.
6. Appended backlog entries 7 (caddy cleanup symmetry) and 8 (`restoreOldContainer` surfacing) to `_ai/m1x-backlog.md`.

---

## 2. My decisions on each Linus issue

| Issue | Linus's recommendation | My decision | Defense |
|---|---|---|---|
| 1: orphan label gating | Option B (label gate) | **Agreed: Option B** | Don's original spec said "label/name does not correspond to any registry entry" — Joel softened it. Linus is right to flag. The cost is small (one field on `InspectResult`), the safety win is decisive. |
| 2: ExitCodeFor over-broad | Option A (drop context.* matches) | **Agreed: Option A** | Same shape of trap the readiness probe fell into. `Deploy` is the choke point; wrap-and-match-the-wrap is the right discipline. |
| 3: probe wrap shape | Raw `ctx.Err()` + log fork | **Agreed** | "readiness:" prefix is misleading; the whole point of the change is to STOP treating cancellation as a readiness failure. Joel's v1 rationale was self-cancelling (it pointed to a test that v1 itself was changing). |
| 4: 11 test edits | Option C (harness AnyTimes default) | **Agreed: Option C** | gomock matches most-recently-added expectations first, so explicit InOrder Inspect expectations in §5.3/§5.4/§5.5/§5.10 take priority over the harness default. Verified mentally; Kent will verify in practice. |
| 5: caddy backlog | Backlog entry | **Agreed** | Done. Item 7 in `_ai/m1x-backlog.md`. |
| 6: restoreOldContainer surfacing | Backlog entry | **Agreed** | Done. Item 8 in `_ai/m1x-backlog.md`. |

I considered defending v1 on Issue 3 (the audit prefix has SOME value) but Linus's argument is correct: the prefix lies about what happened, and Joel's test-driven justification was already being undone by the same plan. Concede.

I considered defending v1 on Issue 4 (the 11 edits ARE a contract reflection) but the harness default is functionally equivalent and saves 11 mechanical edits. The contract is still tested by §5.3/§5.4/§5.10/§5.11 which DO assert specific Inspect calls. Concede.

No issues where I disagreed with Linus.

---

## 3. Material changes in plan v2 vs v1

### 3.1 Strategy changes (`02-plan.md`)

- **§3.2 (probe wrap):** chose raw `ctx.Err()` (was: Shape A vs Shape B undecided in v1). Added the orchestrator log-line fork requirement.
- **§3.3 (defensive orphan):** added label-gating requirement and the failure-mode matrix. Explicit driver interface widening note.
- **§12 (new):** review resolutions, item-by-item. Includes the backlog-entry text for items 7 and 8.

### 3.2 Implementation changes (`03-tech-plan.md`)

- **§3.3:** v1 wrapped probe error with `fmt.Errorf("readiness: %w", ctx.Err())`. v2: leave the existing `return ctx.Err()` unchanged.
- **§3.4.1:** added the audit-log fork (`logger.Info("deploy cancelled during readiness wait", ...)` for cancellation, `logger.Error("readiness failed", ...)` for real failure).
- **§3.5:** label-gated. Refusal path returns `ErrRun` with manual recovery hint. Failure-mode matrix added.
- **§3.5.1 (new):** driver interface widening. `InspectResult.Labels map[string]string`. New JSON `--format` for `cliDriver.Inspect`.
- **§3.6:** `ExitCodeFor` matches only `ErrInterrupted`. Possible drop of `context` import noted.
- **§4.4 + §5.9:** harness `AnyTimes()` default for `Driver.Inspect`. 11 mechanical edits collapse to one harness change.
- **§5.3, §5.4, §5.5:** updated to include `Labels: map[string]string{"decloud.service": "foo"}` in Inspect returns where the orphan should pass the label gate.
- **§5.8:** dropped `context.Canceled` and `context.DeadlineExceeded` table cases. Added optional negative-test cases (mapping to `ExitInternal` not `ExitInterrupted`).
- **§5.10 (new):** two label-refusal tests (missing label, mismatched label).
- **§5.11 (new):** driver-level `cli_driver_test.go` updates for the new `--format` argv and JSON parsing.
- **§12 (new):** v2 revision summary table.
- **P.P.S.:** updated implementation order (driver-level changes first, since they're the foundation; tests-first within each layer).

### 3.3 Backlog updates (`_ai/m1x-backlog.md`)

- **Item 7 (new):** apply cleanup-context pattern to `caddy/manager.go`.
- **Item 8 (new):** `restoreOldContainer` failure surfacing.

---

## 4. What I expect Linus to scrutinize on re-review

1. **Did I get the gomock precedence story right in §5.9?** The harness AnyTimes default plus explicit InOrder expectations is supposed to "just work." If gomock actually matches FIFO instead of LIFO (or matches by specificity in a way I'm misremembering), the §5.3/§5.4/§5.10 tests would fall through to the wrong expectation. Kent will catch this when running tests; if Linus wants belt-and-suspenders he can require the harness default to use a more-specific matcher (e.g. `Inspect(gomock.Any(), gomock.Not("decloud-foo"))` so it doesn't match the test's specific service name). I think that's overkill.

2. **Is the JSON `--format` for `docker inspect` correct?** I sketched `--format='{"id":{{json .Id}},"state":{{json .State.Status}},"labels":{{json .Config.Labels}}}'`. Rob will verify against actual `docker inspect` output. The `{{json}}` template function is standard Go template syntax that Docker supports for JSON-encoding.

3. **Does the label-mismatch error message text help users?** I included the offending label value (`decloud.service="bar"`) so the user can SEE why decloud refused. Linus may push back that the error message is too long; if so, shorten while preserving the recovery hint and the "was not created by decloud" phrasing.

4. **Should the negative test cases in §5.8 be required or optional?** I marked them optional. Linus may want them required as a guard against future regressions.

5. **Is item 8 (restoreOldContainer surfacing) correctly scoped as backlog?** The cleanup-context fix in this task means rollback now actually RUNS, which is a strict improvement. Surfacing rollback failures is its own task. Linus already acknowledged this in his review; just confirming in v2.

---

## 5. Files changed (relative paths)

- `_tasks/2026-04-28-deploy-cleanup-on-interrupt/02-plan.md` — v2 revision header, §3.2 rewrite, §3.3 rewrite, new §12.
- `_tasks/2026-04-28-deploy-cleanup-on-interrupt/03-tech-plan.md` — v2 revision header, TL;DR boast updated, §3.3 rewrite, §3.4.1 audit-log fork, §3.5 + §3.5.1 rewrite, §3.6 simplification, §4.4 update, §5.3/§5.4/§5.5 label additions, §5.8 update, §5.9 rewrite, new §5.10, new §5.11, new §12, P.P.S. for Rob.
- `_ai/m1x-backlog.md` — items 7 and 8 appended.

No production code or test code changed (planning phase only, per user directive).

---

## 6. Next step

Spawn Linus to re-review `02-plan.md` v2 + `03-tech-plan.md` v2.

Per the user's instructions, the loop continues until Linus says APPROVE. If Linus returns REVISE again on remaining details, I iterate. If Linus returns REJECT (unlikely given the resolutions cleanly map to his options), I escalate.

When Linus APPROVES, save the approval as the next-numbered file (likely `006-linus-approval.md` or similar, per bureau numbering).

— Don
