# 020 — Joel's Cycle-2 Closeout (final PLAN-loop concurrence)

Author: Joel Spolsky (planning agent)
Date: 2026-04-27
Status: **CONCUR — FULLY DONE.** Cycle-2 PLAN was implemented faithfully; no
technical-spec gap remains. Independent verification against source completed.

## 0. How I closed this out

I read the prompt-mandated set in order:

1. `013-joel-tech-plan-cycle2.md` (my own plan).
2. `016-rob-implementation-cycle2.md`.
3. `017-raymond-docs-cycle2.md`.
4. `018-kevlin-review-cycle2.md` (APPROVED).
5. `018-linus-impl-review-cycle2.md` (APPROVED).
6. `019-don-closeout.md` (FULLY DONE).

Then I cross-checked against source on disk before signing — I do not
sign off on cycle reports without re-reading the actual code:

- `internal/caddy/manager.go:90-157` — the new actionable branch and
  the `isPortsBoundErr` helper, in full.
- `internal/cli/caddy_up.go` — entire file, `Long` literal verified.
- `internal/cli/caddy_down.go` — entire file, `Long` literal verified.

That's the standard. The reports look right; the source is what ships.

## 1. Spec-compliance check, item by item

I wrote the cycle-2 spec. My job here is to verify each section landed.

### Item #1 — §1 ports substring detection — FAITHFUL

§1.3 specified the EXACT code shape. Source matches:

- **Branch placement (§1.2: manager, not driver):** `isPortsBoundErr`
  lives at `internal/caddy/manager.go:147-157`. Driver
  (`internal/dockerdrv/cli_driver.go`) untouched. Symmetric with the
  `isNotRunningStderr` precedent.
- **Branch shape (§1.3 lines 73-79 of plan):** lines 95-100 of
  `manager.go` carry the spec'd if/else with `%w`-only wrap on the
  actionable branch and `%w: docker run: %w` on the fall-through.
  Byte-for-byte match.
- **Helper shape (§1.3 lines 91-96 of plan):** lines 153-157 of
  `manager.go`. Both substrings (`"address already in use"` and
  `"port is already allocated"`) present, case-sensitive, separated
  by `||`. Brittleness comment present and matches the plan's
  prescribed wording (lines 85-90).
- **The actionable error literal (§1.3 line 76 of plan):** present
  verbatim at `manager.go:97`. I `grep -F`'d the recovery substring
  against both source and `_docs/install.md:175` — match in both
  places. C2-5 satisfied.
- **Test (§1.5):** `TestManager_UpPortsBoundActionableError` at
  `manager_test.go:179-208` per Kevlin's reading log. Five
  assertions per sub-test as spec'd: sentinel preserved, two
  recovery substrings, actionable symptom, branch-choice negative
  (`NotContains(": docker run: docker run:")`). The negative
  assertion is the standout — without it, a future refactor that
  re-introduced inner-err wrap on the actionable branch would
  silently pass the four positives. Locked.

**One implementation detail Rob got right that the spec under-
specified:** the actionable branch deliberately drops the inner
driver error. My §1.3 spec'd `fmt.Errorf("%w: ports 80/443 already in use; ...", ErrCaddyUp)` (no inner `%w`) but didn't explicitly call
out the rationale. Rob inferred it correctly from Kent's
`NotContains` assertion, and his §"Files changed" report names the
choice explicitly: "re-wrapping `err` would re-introduce the doubled
`docker run: docker run:` chain." That's the right call. The
actionable text already names recovery commands; the driver chain
adds noise without information. No spec gap — Rob made the implicit
decision the spec's test contract demanded.

### Item #2 — §2 IPv6 doc reword — FAITHFUL (with documented improvement)

`_docs/install.md:188-196` (per Linus 018 §4.2). The fabricated
cycle-1 example is gone. The replacement framing surfaces the
substring inside variable daemon prose with ellipses.

This deviates from my §2.1 specific prose, and Raymond's report §"Why
this shape" documents the deviation honestly: the user's EXECUTION
3.3 instructions overrode my prescribed text. The new framing is more
honest than my own — "stderr containing X, the raw `docker run`
stderr is surfaced as-is, typically reads similar to ..." with
ellipses commits to nothing it can't deliver. My §2.1 prose committed
to a specific wrapped chain that may shift if wrap layers ever
consolidate. Raymond's deviation is an improvement, not a defect.
Linus 018 §4.2 endorsed the change explicitly.

**No spec gap.** The intent (replace fabricated example with honest
substring framing) is met; the text is better than I wrote. C2-4
satisfied.

### Item #3 — §3 ports doc match — FAITHFUL

`_docs/install.md:174-176` carries the literal that
`internal/caddy/manager.go:97` emits, byte-for-byte. The
`caddy: up failed:` prefix composes from `ErrCaddyUp.Error()` + `%w`
separator correctly. `grep -F` of the recovery substring matches both
files. CLAUDE.md hallucination-check discipline satisfied.

The recovery shell block that follows the example (lines 180-184)
repeats the commands intentionally — operators who skim past the
error text still see them in a copy-pasteable code block. Per my §3.1
verification step. C2-5 satisfied.

### Item #4 — §4 `Long` help text — FAITHFUL

I read both `caddy_up.go` and `caddy_down.go` directly. Both `Long`
fields match my §4.1 / §4.2 verbatim text byte-for-byte. Three
paragraphs each. The operationally-critical bits land:

- `caddy up --help`: dual-stack publishing (80/tcp, 443/tcp, 443/udp
  on both 0.0.0.0 and [::]); image and volume names; volume-retention
  warning; idempotency contract.
- `caddy down --help`: ingress-interruption warning with capital
  `ALL`; volume retention; **LE rate-limit warning** ("forces fresh
  Let's Encrypt issuance and risks tripping LE rate limits on hosts
  with many domains") — the operator-value bit Don §4a / Linus 9.1
  required; idempotency contract.

`Args: cobra.NoArgs` retained. `RunE` bodies unchanged. No flag
surface drift. No new tests required (Cobra renders `Long`
independently; existing `NoFlags` regression guards still pass per
Rob's verification logs). C2-6 and C2-7 satisfied.

### Item #5 — §5 m1x-backlog item #6 — FAITHFUL (with M2-deferred reconciliation note)

Item #6 sits at `_ai/m1x-backlog.md` lines 55-63 between item #5 and
the `## Maintenance note` section. Format matches items 1-5: four
bold-prefixed sections (Where / Why deferred / Fix shape /
Originator). Cross-references to `_ai/decisions/m1-test-strategy.md`
and `_ai/decisions/caddy-runs-in-container.md` both check out. The
"deferred from the caddy-container-connection-refused task per
`_ai/decisions/m1-test-strategy.md`" phrase is verbatim per the
user's EXECUTION 3.3 wording.

**The one intra-item inconsistency Linus 018 §5 flagged:** the
heading says "Docker-compose-based smoke integration test" while the
**Fix shape** describes a Go `integration_test.go` build-tagged with
`//go:build integration`. That's a real semantic mismatch — a docker-
compose harness and a Go integration test are different shapes.
Future-Don picking this up in M2 will need to reconcile.

**Is this a cycle-2 spec gap?** No. The user's EXECUTION 3.3
instructions specified the heading wording; Raymond preserved it and
expanded the body using my §5.1's Where/Why/Fix/Originator template.
The mismatch is a tension between user-supplied heading text and my
plan's Fix-shape paragraph. Raymond made the right call: preserve
user wording in the heading (it's the bullet-text the user wrote),
preserve the file's existing entry style for the body. The
reconciliation belongs to whoever picks up the M2 work — they'll see
the mismatch and choose one shape. C2-8 satisfied (item exists in
correct place, correct format).

## 2. Was anything in cycle-1 broken?

No. I traced the obvious regression vectors:

- **`errors.Is(err, ErrCaddyUp)` on the new branch:** holds. Verified
  by Kent's test assertion at `manager_test.go` and Linus's
  standalone harness at 018-linus §0. Exit-code mapper at
  `internal/cli/exit_codes.go:58-59` continues to map to
  `ExitRunFail` (40). No exit-code regression.
- **Generic-wrap path still fires:** `TestManager_UpRunFailsWithoutRollback`
  passes with its `errors.New("port allocation failed")` sentinel.
  Neither canonical Docker substring is present in that string —
  `"port is already allocated"` is NOT a substring of `"port
  allocation failed"` (different word order, "is" missing,
  "allocated" vs "allocation"). C2-3 genuinely covered, not
  accidentally satisfied. This was Linus's caveat in `014` §1; it
  holds.
- **Driver surface untouched:** `internal/dockerdrv/cli_driver.go`
  and `cli_driver_test.go` not in cycle-2 diffs per Rob's "Files
  touched" list. Argv-shape tests still lock the contract.
- **Mock surface:** no interface change → no `go generate` regen →
  no `internal/dockerdrv/mocks/*.go` churn. C2-11 satisfied
  trivially.

## 3. Acceptance criteria — final state of v2 §11 (19 items)

All 14 operator-independent criteria flip to HIT after cycle 2:

- HIT: 1, 2, 3, 4, 5, 6, 7, 12, 13, 14, 15, 16, 17, 18.
- PENDING (operator-gated by design): 8, 9, 10, 11, 19. The user
  runs §7 manual verification on their host. That gate is
  intentional; we don't unilaterally close it.

Cycle-2's eleven sub-criteria (C2-1 through C2-11):

- C2-1, C2-2: PASS (sub-tests green per Rob 016, Kevlin 018-kevlin).
- C2-3: PASS (`UpRunFailsWithoutRollback` continues to fire on
  non-port sentinel — verified above).
- C2-4: PASS (IPv6 example reframed as substring).
- C2-5: PASS (ports example matches code byte-for-byte — `grep -F`
  verified in three reports plus my own re-check).
- C2-6, C2-7: PASS (`Long` rendered correctly — text verified
  verbatim in source).
- C2-8: PASS (item #6 exists, correct format).
- C2-9: PASS (`gofmt -l .` empty per Rob, Kevlin, Linus).
- C2-10: PASS (`go test ./... -count=1` green per Rob, Kevlin, Linus,
  Don).
- C2-11: PASS (no `//go:generate` change, no interface change).

**Every spec-compliance criterion under our control is HIT.**

## 4. Two non-blocking observations carried forward (from Linus, not new)

These are M2 material, NOT cycle-3 triggers:

1. **207-character actionable error string is verbose.** If M2
   revisits wording, "to make the change persistent" is the natural
   trim target. Don shipped verbatim per `012` §"Joel and I
   considered tightening prose; I sided with verbatim ship." Fine.

2. **m1x-backlog #6 heading vs fix-shape mismatch.** "Docker-compose-
   based" (heading) vs "`integration_test.go` build-tagged" (fix
   shape). Reconcile when picking the item up.

Neither is a defect on the bug-fix path. Both are documented in
018-linus §8 and 019-don §1/§4. Cycle-3 would be over-engineering.

## 5. Verdict

**FULLY DONE — concur.**

Cycle-2 PLAN was implemented faithfully. No technical-spec gap. The
five items I planned in `013` are all closed; the test contract is
locked; the doc edits track the code byte-for-byte; the `Long` text
matches verbatim; the backlog item is appended in correct format.

Two minor deviations from my prose are documented and approved:

1. **Raymond's IPv6 reword (§2.1 deviation):** improvement, not
   defect — the user's EXECUTION 3.3 framing is more honest than my
   §2.1 prose. Linus 018 §4.2 endorsed.
2. **Backlog item heading (§5.1 deviation):** user-supplied wording
   preserved. Mismatch with fix-shape body is M2 reconciliation
   material.

Neither deviation undermines the spec's intent. Both improve on it
where the user's guidance overrode my prescribed text.

The plan-vs-ship gap is closed. The architectural fix (Caddy in-
container on the `decloud` bridge) is sound. The migration story
(M1.0 → M1.1) walks cleanly: bind fails → actionable error names
recovery → operator runs recovery → retry succeeds. End-to-end
coherent.

Advance to FINALIZATION. Ward captures the learnings; Andy considers
agent-instruction tuning. Don's §5 list of three substantive learnings
is well-scoped — I'd add nothing material to it.

— Joel
