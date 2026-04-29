# 016 — Kevlin's re-review (M2 EXECUTION v2)

Scope: re-review of commit `0152134` ("fix(m2): closeout fixes —
nginx:alpine integration test + doc tidy") against Joel's addendum-v2
(`013`) and my v1 review (`010`). Linus already approved the addendum
in `014`. This is a delta-only re-review — the full M2 surface was
audited in `010-kevlin-review.md` and stands.

## TL;DR

All four fixes match Joel's spec byte-for-byte. Rob shipped exactly
what was locked. The hand-off note for the run-log artifact is
adequate, with one observation below worth recording but not blocking.
No new issues introduced. The `git show 0152134` diff is six files,
+408/-3 lines (mostly the report markdown); the load-bearing code/doc
deltas are tiny and exactly mechanical.

**APPROVED — proceeds to PLAN re-entry v3.**

---

## 1. Fix-by-fix verification (byte-level)

### Fix A (Kevlin v1 §10 Fix A) — `Mount.HostPath` doc-comment

Joel locked the wording at `013-joel-tech-plan-addendum-v2.md` §2.2.
Rob's diff at `internal/registry/types.go` lines 60-63:

```go
	// HostPath is the mount source. For bind mounts it is an absolute host
	// path starting with "/"; for named volumes it is the volume name. The
	// TOML key is historically named host_path. Use Mount.IsNamed() to
	// distinguish at runtime.
```

Verbatim against Joel §2.2's locked block. Three lines, comment-only,
field shape and TOML tag preserved. **Match.**

### Fix B (Kevlin v1 §10 Fix B) — `_docs/usage.md` line 3 tense slip

Joel locked the wording at `013-joel-tech-plan-addendum-v2.md` §3.2.
Rob's diff at `_docs/usage.md` line 3:

```
Operator-facing reference for the Decloud CLI. For host setup, see [`install.md`](./install.md).
```

"M1 " removed; rest of the line preserved (including the `install.md`
link). Verbatim against Joel §3.2. **Match.**

### Fix C (Kevlin v1 §10 Fix C) — `_ai/m1x-backlog.md` item 6 rewrite

Joel locked the wording at `013-joel-tech-plan-addendum-v2.md` §4.2,
which adapted my v1 suggestion with two deltas (image is `nginx:alpine`
post-fix, plus the explanatory sentence on the alpine→nginx rationale).
Rob's diff at `_ai/m1x-backlog.md` line 59 (`**M2 delivery:** ...`)
matches Joel §4.2 verbatim, including the trailing sentence:

> The `nginx:alpine` choice (rather than alpine) is deliberate: nginx
> idles in the foreground via `nginx -g daemon off;`, so the container
> stays alive long enough for `docker exec`; alpine's default `/bin/sh`
> CMD exits under `docker run -d` (Linus's catch in
> `011-linus-impl-review.md` §5, fix in EXECUTION v2).

**Match.** The rewrite now reflects shipped reality (driver-direct,
`nginx:alpine`, no Caddy, no orchestrator), and the audit-trail
sentence ensures future-Don sees the why-not-alpine rationale at the
same time they see the test description.

### Linus integration-test fix — `nginx:alpine` swap

Joel locked the wording at `013-joel-tech-plan-addendum-v2.md` §1.4.
Rob's diff at `internal/integration/mount_test.go` line 23:

```go
	mountTestImage       = "nginx:alpine"
```

One-line constant change. No other line in `mount_test.go` touched
(verified via `git show 0152134 -- internal/integration/mount_test.go`).
**Match.**

The semantic correctness of the swap (Joel §1.3: `nginx:alpine`'s
default CMD is `nginx -g daemon off;`, idles foreground indefinitely,
so `docker exec` runs against a live container) was verified by Joel
and ratified by Linus (`014` §1). I have no daemon to spot-check, but
the plan-stage analysis is sound.

### Linus Observation A — item 11 future-author note

Joel locked the wording at `013-joel-tech-plan-addendum-v2.md` §4.3.
Rob's diff at `_ai/m1x-backlog.md` line 113 appends a new paragraph
after item 11's `**Originator:**` line, before the `---` separator.
Verbatim against Joel §4.3 (read end-to-end against the file). The
paragraph names the gap (no `Cmd []string` on `RunRequest`/
`RunOptions`), names the workaround (`nginx:alpine`), and explicitly
says "consolidated `RunOptions`" so future-author lands the field
on the right post-consolidation struct. **Match.**

---

## 2. Does Rob's `nginx:alpine` change match Joel's spec?

Yes, exactly. Joel §1.4 specified the one-line diff with surrounding
context lines for unambiguous targeting:

```go
mountTestImage       = "alpine:3.19"   // before
mountTestImage       = "nginx:alpine"  // after
```

`git diff` confirms exactly this single-line change in
`internal/integration/mount_test.go` at the constant declaration
inside the `const ()` block (line 23 by current numbering). No
collateral changes to the test body, the cleanup discipline, the
`Network: "decloud"` setup, or anything else. **Spec-faithful.**

The semantic claim — that the container will stay alive for `docker
exec` to run against — is sound: nginx as PID 1 with `daemon off;`
foregrounds the master process indefinitely under `docker run -d`.
The actual proof of correctness is the run-log gate; Rob can't
produce it on Mac (no Docker), and that's the right place to draw
the verification line.

---

## 3. Does the m1x-item-11 future-author note land correctly?

Yes. Three properties to check:

1. **Located at the right place.** The note appends to item 11
   ("Consolidate `Driver.Run` and `Driver.RunWithOptions`"), not
   item 6 (the integration-test one). Reading
   `_ai/m1x-backlog.md` lines 103-113 confirms: the originator line
   is at line 111, the new paragraph is at line 113, and the `---`
   separator is at line 115. The note will be read by whoever picks
   up item 11, exactly when it matters.

2. **Names the right struct.** The note says "the unified
   `RunOptions` should grow `Cmd []string`" (not "add to
   `RunRequest`"). Item 11's "Fix shape" paragraph (line 109) commits
   to retiring `RunRequest` and routing everything through
   `RunWithOptions`, so the note correctly points future-author at
   the post-consolidation struct rather than the deprecated one.
   This is the subtle but load-bearing detail Joel called out at
   §4.3 — and it's correct in the shipped diff.

3. **Cites the source.** The note ends with
   `_tasks/2026-04-28-m2-server-side-mounts/011-linus-impl-review.md`
   §"Observation A". Anyone reading the backlog can trace back to
   the original observation without context-loss.

**Lands correctly.**

---

## 4. New issues introduced by v2?

None. Going through the diff systematically:

- **Code change is one constant.** No new control flow, no new error
  paths, no new types, no new test surface. The `nginx:alpine`
  string is a self-contained literal. There is no way for this
  change to introduce a regression in production code (the constant
  lives in a build-tagged test file).
- **`types.go` change is comment-only.** Comments don't compile to
  anything; `gofmt` formats them; no behaviour delta is possible.
- **`_docs/usage.md` change is two characters of prose.** No
  operator-facing claim was altered (the document still describes
  the Decloud CLI; only the "M1" qualifier was dropped).
- **`_ai/m1x-backlog.md` changes are future-Don notes.** No
  production surface, no test surface.

Sanity gates Rob ran (`015 §"Sanity-check results"`):

- `go build ./...` — clean.
- `go build -tags integration ./...` — clean. **Important:** this
  is the gate that catches a typo'd image constant; empty output
  here means `mount_test.go` still compiles after the rename.
- `go vet ./...` — clean.
- `go test ./...` — all packages green (integration is build-tagged
  out, correctly).
- `gofmt -l .` — empty.

I have no reason to doubt these results — re-running them on this
box would be ceremonial, since the changes are mechanical and the
gates are deterministic. The only verification still pending is
the real-Docker run-log, which is intentionally hand-off'd.

**No new issues.**

---

## 5. The run-log hand-off note — is it adequate?

Reading
`_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md`
end-to-end. Verdict: **adequate, with one observation.**

What the hand-off does well:

- States the artifact is missing and why (Mac, no Docker).
- Names the exact command to run, with `tee` redirection to the
  gating artifact path. Maintainer copy-pastes, runs, gets the file
  in the right place automatically.
- Specifies prerequisites (Docker daemon accessible without sudo,
  Docker Hub reachability, `decloud` network creatable).
- Names the PASS line to grep for AND the FAIL line that must NOT
  appear. Binary, unambiguous gate.
- Says explicitly: "squash-merge into `main` MUST NOT happen until
  this run-log lands and shows PASS." Three sign-offs cited
  (Don §1, Joel §5.1, Linus §2). The squash-merge is an Andy/HR-step
  action per the workflow, so this gate is enforceable as long as
  the agentic team reads the hand-off note before triggering
  finalization.
- Says explicitly that if the first run shows FAIL, the plan
  re-opens — not "retry blindly." This is the right discipline.

The one observation worth recording (NOT a blocker):

The hand-off relies on **agentic-team discipline reading the
hand-off note before triggering FINALIZATION step 3 (squash-merge)**.
There is no programmatic enforcement: no GitHub Actions workflow
that fails the merge if `integration-test-run-log.txt` is absent,
no pre-merge hook, no branch-protection rule keying on the file's
existence. If a future-Don skims and triggers squash-merge without
reading this hand-off, the gate is bypassed silently.

A more robust gate would be either:

1. **A GitHub Actions check** that verifies the file's existence
   AND greps for the PASS line on the branch, set as a required
   status check on the merge PR. But: this requires CI infra Decloud
   doesn't yet have, and standing it up for one closeout gate is
   over-engineering. m1x-backlog territory at best.
2. **A pre-FINALIZATION hard-stop in the agent workflow** — e.g.,
   Andy's instructions read "before squash-merge, verify
   `integration-test-run-log.txt` exists in the task dir AND
   contains `--- PASS: TestIntegration_MountBindRoundTrip`". This
   is a soft enforcement (relies on Andy reading the rule), but
   it routes through a single chokepoint instead of relying on
   every agent reading every hand-off.

For M2 specifically: the workflow already names PLAN re-entry v3
(Don/Joel/Linus closeout vote) BEFORE FINALIZATION. If the closeout
vote requires the run-log as a precondition, the gate is
agent-enforceable without CI. Don's `012` §3 implies this; Joel's
`013` §5 makes it explicit ("No PASS log → no closeout"); Linus's
`014` §2 ratifies. So the chain is:

1. v2 commit lands. ✓ (already done.)
2. Maintainer runs the integration test on Linux + commits the
   run-log artifact.
3. Don/Joel/Linus PLAN re-entry v3 — vote DONE only if the run-log
   PASSes.
4. Ward → Andy → squash-merge.

Step 3 is the human-in-the-loop gate. As long as Don/Joel/Linus
re-entry v3 reads this hand-off before voting DONE, the run-log
requirement is enforceable. **Acceptable for this task.**

Could we go further (per the question)? Yes — a pre-merge GitHub
Actions check would be more bullet-proof. But it's
disproportionate-effort for one closeout. **Recommendation: leave
as-is for M2; if the gate ever bites a future closeout, file an
m1x-backlog item for "automated closeout gates."**

---

## 6. Decision

All four fixes are mechanically correct against Joel's locked spec.
The `nginx:alpine` swap matches the addendum verbatim. The future-
author note for m1x-item-11 lands on the right item with the right
struct name. No new issues are introduced (the changes are too
small to introduce any). The run-log hand-off is adequate given the
PLAN re-entry v3 gate ahead of FINALIZATION.

**APPROVED — proceeds to PLAN re-entry v3.**

The one non-blocking observation (CI-enforced gate vs. agent-
enforced gate) is recorded above for future reference, not for
action.

---

## Files I read for this re-review

- `_tasks/2026-04-28-m2-server-side-mounts/010-kevlin-review.md` (my v1, for fix-A/B/C wording)
- `_tasks/2026-04-28-m2-server-side-mounts/013-joel-tech-plan-addendum-v2.md` (the locked spec)
- `_tasks/2026-04-28-m2-server-side-mounts/015-rob-impl-v2.md` (Rob's report)
- `_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md` (the hand-off)
- `git show 0152134` (the commit, full diff)
- `internal/integration/mount_test.go` (post-fix)
- `internal/registry/types.go` (post-fix)
- `_docs/usage.md` (post-fix, line 3)
- `_ai/m1x-backlog.md` (post-fix, items 6 and 11)
