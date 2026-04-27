# 014 — Linus Review, Cycle 2 PLAN

Author: Linus Torvalds (high-level review agent)
Date: 2026-04-27
Status: Cycle-2 PLAN review of `013-joel-tech-plan-cycle2.md` against Don's
`012-don-final-review.md` 5-item list.

## 0. Reading log

Read in this order, in full:

1. `_tasks/.../012-don-final-review.md` — Don's cycle-2 list and decisions.
2. `_tasks/.../013-joel-tech-plan-cycle2.md` — Joel's expansion.

Cross-checked against the source on disk so I'm not reviewing fiction:

- `internal/caddy/manager.go` (the `Up` wrap point Joel is editing).
- `internal/caddy/reloader.go` (the `isNotRunningStderr` precedent Joel cites
  for placement).
- `internal/dockerdrv/cli_driver.go:239` (the `; stderr=%q` wrap that feeds
  into the substring match).
- `internal/cli/caddy_up.go`, `internal/cli/caddy_down.go` (currently
  `Short:`-only — Joel is right).
- `_docs/install.md:160-200` (the two doc-fab paragraphs).
- `_ai/m1x-backlog.md` (currently 59 lines — the append target exists).

The plan accurately reflects the codebase. No fabricated line numbers, no
imagined wrap shapes.

---

## 1. The two substrings — right set?

**Verdict: yes, these are the correct two substrings, and Joel's
justification for not adding more is sound.**

Walking through the question:

- `address already in use` is the kernel `errno` text via `strerror(EADDRINUSE)`.
  Docker's userland-proxy path surfaces it verbatim in the stderr chain when
  `bind(2)` fails. Joel's example is the canonical Docker 20.10+ shape.
- `port is already allocated` is Docker's allocator-side variant. It fires
  when another docker-managed container already published the port and
  Docker's allocator rejects the bind before the syscall even runs. This
  string has no `bind:` prefix, which is why a `bind:`-only match would miss
  it. Joel's analysis on this is correct.

**Anything missing?** I went looking. Three plausible candidates:

1. **`Only one usage of each socket address`** — the Windows variant from
   WSAEADDRINUSE. M1 supports Linux-only docker-ce per `_docs/install.md` §1.
   Don't add it.
2. **`cannot assign requested address`** — different errno (EADDRNOTAVAIL),
   different failure mode (bind to non-local IP). Not a port-conflict; would
   misdirect operators. Don't add it.
3. **`rootlessport: error response from daemon`** — rootless-mode error path.
   Decloud assumes root docker-ce. Don't add it.

So no, nothing missing for the supported platform. The two-substring set is
correct and minimal.

**Locale/casing already named.** Joel's §1.4 is honest about the brittleness:
case-sensitive, `LANG=C`/`en_US.UTF-8` only, no normalization. He resists
"fragility-by-imagination" widening. Right call.

**One tightening I want:** the test in §1.5 should ALSO assert that the
generic-wrap path (`TestManager_UpRunFailsWithoutRollback` or equivalent)
still fires when stderr contains NEITHER substring. Joel mentions this in
§9 acceptance criterion C2-3 but it's leaning on an existing test passing
unchanged — fine if that test's sentinel error string really does miss both
substrings, but Kent/Rob should grep-verify before declaring victory.
That's a verification note, not a plan change.

## 2. Helper placement — `manager.go` co-located vs. `stderr_match.go`

**Verdict: Joel's choice (co-located in `manager.go`) is correct. No
extraction now.**

Joel asked me directly. Here's the call:

The argument FOR a shared `stderr_match.go`:
- Symmetry with `isNotRunningStderr` in `reloader.go`.
- Pre-positions the codebase for a third helper.

The argument AGAINST (Joel's, and I agree):
- The two existing helpers detect entirely different categories of failure
  in entirely different code paths. They share `strings.Contains` as a
  mechanism. That's not a basis for co-location. Grouping by mechanism
  rather than purpose is exactly the kind of "library of generic helpers"
  pattern that turns into a junk drawer.
- `isPortsBoundErr` is package-private to `caddy`. It has one caller
  (`Manager.Up`). Sitting at the bottom of `manager.go` means the reader
  who sees the call site can scroll 30 lines down and find the helper.
  Pulling it into `stderr_match.go` adds a file hop for no readability win.
- The "what if a third helper appears" hypothetical is the rule-of-three
  principle. Two helpers is below threshold. When the third arrives — and
  if it ever does — the cost of extracting `stderr_match.go` then is the
  same as extracting it now: trivial.

The "feels symmetric" instinct that wants to extract now is over-engineering.
Joel was right to call it out as a non-blocking nit in cycle 1, and he's
right to keep the helpers co-located in cycle 2. Approved as-is.

**Don't refactor in anticipation of a future you can't predict.**

## 3. Error message wording — fit for the operator who hit M1.0

The proposed wording from `manager.go::Up`:

```
caddy: up failed: ports 80/443 already in use; if you ran the M1.0 install,
run 'systemctl disable --now caddy && systemctl mask caddy' or
'apt-get remove -y caddy' to make the change persistent
```

**Verdict: good, with one minor suggestion that Don can take or leave.**

What works:
1. Names the symptom in plain English up front (`ports 80/443 already in use`).
2. Names the most likely cause without overclaiming (`if you ran the M1.0
   install`). Doesn't assume — gives a conditional.
3. Provides two concrete recovery commands, copy-pasteable, with a
   semantically correct OR between them.
4. Names WHY `disable --now` alone is insufficient (implicit in "to make
   the change persistent"). The word "persistent" is doing a lot of work
   here, but it's pulling its weight.

What I'd consider tightening (Don's call):

- **Length.** 207 characters in one line. On an 80-column terminal that
  wraps to 3 lines. Cobra's default error rendering doesn't word-wrap, so
  the operator gets one long line. Acceptable but a touch verbose. Could
  drop "to make the change persistent" — it adds 28 chars and the
  parenthetical justification is in the docs (§3.1 in Joel's plan). The
  trimmed version: "ports 80/443 already in use; if you ran the M1.0
  install, run 'systemctl disable --now caddy && systemctl mask caddy' or
  'apt-get remove -y caddy'". Same actionability, 30% shorter.

- **Two recovery paths in one line.** Some operators will paste the whole
  thing including the OR. Not catastrophic — `bash: OR: command not found`
  is a clear "user error", and they'll figure it out. But a more careful
  rendering would put the commands on their own lines, which Cobra error
  rendering won't do. Trade-off: keep the single-line format because the
  doc (`_docs/install.md` §3.1) renders the recovery commands in a proper
  fenced code block. The error string is a pointer to "do something",
  the docs are the carrier of the exact recipe. That's the right division.

**My take:** ship it as-is. Don's directive in §1.3 was "verbatim from v2
§1.5 row 1" and Joel matched that. Don't bikeshed the wording at this
stage. The trim suggestion above is a future-Don improvement, not a
cycle-2 blocker.

The "if you ran the M1.0 install" framing deserves explicit credit — it
correctly addresses the audience (operators upgrading from M1.0, who are
THE population that hits this) without alienating fresh installers (who
will read it as a mild non-sequitur and move on).

## 4. The `Long` help-text approach via Cobra

**Verdict: the approach is fine; the prose is fine; one observation.**

The approach (Cobra `Long:` strings, raw backtick literals, no embedding,
no template indirection) is exactly right for the volume of text involved.
Joel correctly rejected `//go:embed` as over-engineering in §11.4.

The prose itself I read carefully:

**`caddy up --help` (`Long`):**

- Paragraph 1: what it does. Correct and concise.
- Paragraph 2: dual-stack listener detail. Names the protocol/port matrix
  precisely. Good.
- Paragraph 3: image, named volumes, retention semantics, `docker volume
  rm` warning. THIS is the operationally-loaded paragraph. Correct on
  every detail I can verify against `manager.go::runOpts`.
- Paragraph 4: idempotency. States the running-then-exit and stopped-then-
  start branches. Matches `manager.go::Up` lines 75-89. Accurate.

**`caddy down --help` (`Long`):**

- Paragraph 1: what it does. Correct.
- Paragraph 2: ingress impact. The "interrupts ingress for ALL services"
  framing is the right severity. Operators need to feel that.
- Paragraph 3: volume retention + the LE-rate-limit warning. The LE warning
  is the move that makes this text earn its rent. Without it, "you can wipe
  the volumes" reads as "go ahead, wipe them" — the rate-limit consequence
  is what makes operators actually pause. Good prose-engineering.
- Paragraph 4: idempotency note.

**One observation that's NOT a blocker:** the `Long` text on `caddy down`
uses the phrase "ACME state, issued certs" twice (paragraph 2 and 3
implicitly via the data-volume description). It's not a duplication — it's
emphasis. Fine.

**Cobra-mechanical check:** `Long` is rendered by Cobra independent of
`Short`; existing `TestCaddyUp_NoFlags` and `TestCaddyDown_NoFlags`
regression tests are unaffected. `gofmt` validates the literal compiles.
Joel's §4.3 is correct that no new tests are needed.

**One thing I want Rob to do that the plan doesn't say explicitly:**
manually run `decloud caddy up --help` and `decloud caddy down --help`
once after the change and eyeball the rendered output. Cobra's word-wrap
on `Long` strings in raw backtick literals can produce surprising line
breaks if the literal has long unbroken lines. Joel's prose looks
short-line-friendly to me, but a 30-second smoke test is cheap insurance.
Add this to Phase 5 (Verification gate).

## 5. Anything Don should let go of?

**Verdict: no, Don's 5-item list is correctly scoped. But one item is
ALMOST trim-eligible.**

Walking the five:

1. **Port-conflict substring detection.** Required. Joel v2 §1.5 specced it,
   Rob skipped it, the plan-vs-ship gap matters. Keep.

2. **`_docs/install.md:189` IPv6 reword.** Required. It's a doc fabrication;
   we have a hallucination-check discipline; we don't ship hallucinations.
   Keep.

3. **`_docs/install.md:173` ports reword.** Required, AND its content
   depends on item #1 landing first. Joel's Phase 4 ordering captures this.
   Keep.

4. **`Long` help text on `caddy up`/`caddy down`.** Required. Specced in v2
   §1.6, never implemented. Specifically the volume-retention + LE rate-limit
   warning has real operator value. Keep.

5. **Append integration-test backlog to `_ai/m1x-backlog.md`.** This is the
   trim-eligible one. It's a single-bullet append to a backlog file. The
   acceptance criterion #15 from v2 §11 said it goes in the backlog file;
   Don is right that "let Ward pick it up in step 4" creates a traceability
   problem (criterion #15 stays MISS through cycle 2). I considered arguing
   for letting Ward handle it — Ward's role is knowledge capture, and
   "what's deferred to M2" is exactly that. But Joel's §5 entry is
   substantive (3 paragraphs of where/why/fix-shape), and the file's
   existing items use that voice, so a Ward-style append wouldn't be a
   meaningfully different artifact. Keep it in cycle 2.

The "items NOT in this cycle" list (reloader `%q` quoting, `"decloud"`
literal cleanup, wrap-text duplication, stdout-prefix cosmetics) is
correctly held. Each was named and justified; Don's calls were right;
Joel respected them. No scope creep.

**Net: don't trim. The cycle is small enough that all five items can land
in one Kent + Rob + Raymond pass. Trimming saves ~10 minutes of agent work
and costs traceability on at least two acceptance criteria.**

## 6. Cycle-2 plan structure — observations

These are not blockers; they're things I want Don / Joel to be aware of for
this cycle's execution and the next plan they author.

### 6.1 The three-impl-files-but-one-test-file shape

Cycle 2 modifies three Go files (`manager.go`, `caddy_up.go`, `caddy_down.go`)
but only adds one new test (`TestManager_UpPortsBoundActionableError`). The
`Long`-text changes are correctly untested per Cobra rendering semantics.
But this means the cycle's blast radius on test surface area is small —
Linus-1 (me, cycle-1) flagged "tests are right-sized"; cycle 2 keeps that
discipline. Approved.

### 6.2 Phase ordering with parallel-safe phases

Joel marks Phase 2 and Phase 3 as parallel-safe, and Phase 4's §2 edit and
§5 backlog edit as parallel with Rob's work. This is correct dependency
analysis and reflects an understanding that not every phase needs to be
serial. Good.

### 6.3 The "Raymond copies the literal from manager.go" handoff

Joel's §3.1 closing note ("If Rob's implementation in §1.3 deviates from the
spec'd literal: Raymond updates the doc to match the implementation, NOT
the other way around") is exactly the right escalation rule. This is the
hallucination-check discipline working as designed: the code is the source
of truth, the docs follow. Approved.

### 6.4 No driver / cli_driver test churn

Joel correctly scopes the placement decision to keep `cli_driver_test.go`
untouched (§7). One fewer surface to regress.

## 7. Verdict

**APPROVED — proceed to EXECUTION.**

The two substrings are right. The helper placement is right. The error
wording is acceptable (with optional trim noted; Don's call). The `Long`
help text is well-pitched, with the LE rate-limit warning earning its
rent. The five-item list is correctly scoped; none of it is
let-go-able without paying a traceability cost. The phase ordering and
parallel-safe analysis are correct. No fabricated line numbers, no imagined
APIs, no scope creep.

Two notes for Kent/Rob/Raymond during EXECUTION:

1. **Add a Phase-5 manual smoke test of `decloud caddy up --help` and
   `decloud caddy down --help`** to catch any Cobra word-wrap surprises
   in the `Long` strings. Cheap insurance.
2. **Verify `TestManager_UpRunFailsWithoutRollback` (or whichever existing
   generic-wrap test you rely on for criterion C2-3) genuinely contains
   neither substring** before declaring it covers the negative case. A
   `grep -F "address already in use"` and `grep -F "port is already
   allocated"` against the test sentinel error confirms it.

Neither blocks EXECUTION. Both are verification-gate items.

— Linus
