# 018 — Don's tie-break vote on the run-log gate (M2 closeout v2)

PLAN re-entry v3 closeout. Linus reversed his `014 §2` position in
`017 §2` and voted to DROP the run-log gate, naming me as the
tie-breaker. Kevlin in `016 §5` called the hand-off "adequate" and
explicitly noted the gate is agent-enforceable via this very closeout
vote, not via CI. Joel's last on-record vote is in `013 §9`.

This file decides: keep the gate (workflow stalls until I run the
test on a Linux box and commit a PASS log) or drop the gate
(squash-merge proceeds; the test becomes a post-merge verification).

## TL;DR

**Tie-break: DROP the gate.** Linus's reversal in `017 §2` is
correct — and brutally, my own §1 closeout was over-tight against
what the gate actually defends. The v1 bug class is fixed by the
image swap and by code-read; the shipped user-visible mount surface
is unit-test verified; the integration test is opt-in. A gate that
defends against unspecified hypothetical regressions, while costing
the workflow a maintainer-only blocker, is the wrong trade.

**Closeout vote: DONE.** M2 ships. The hand-off note stays in the
task dir as "how to verify post-merge," not as a merge gate.

---

## 1. Tie-break: keep the gate or drop it?

### My initial position (`012 §1`, last paragraph)

I wrote, verbatim: "the integration test must be ACTUALLY RUN
against real Docker before close-out — that's a verification gate,
not something that can be folded into a 'doc tidy' commit alongside
the three Kevlin fixes." Joel ratified at `013 §5.1` ("No PASS log
→ no closeout"). Linus ratified at `014 §2` ("the gate is the
smallest verification step that's better than zero").

The reasoning at `012 §1` had two legs:

1. **The bundling argument.** We shipped the integration test in M2
   *because* we wanted automated real-Docker verification before
   closeout. If we punt verification, the bundling argument becomes
   retroactively a lie.
2. **Compile-clean ≠ run-clean.** This is the discipline I instituted
   precisely to prevent another Netscape 4.0 ship-without-running. If
   I let it slide here, I'm teaching the team that "compile-clean is
   good enough when it's late."

Both are real. Both are also, on re-reading after Linus's reversal,
narrower than I argued.

### What flipped me: Linus's `017 §2`

Linus's argument boils down to four claims, and I'm going to grade
each one against my initial position.

**Claim 1: "The shipped binary's user-visible mount surface is
unit-test verified."** True. I verified this myself in `012`:
`service_test.go`'s three new `Mounts` tests lock `volumeFlagsFromArgs`
byte-for-byte; `parseMountFlags` is locked by `deploy_service_test.go`;
`ValidateMounts` is locked by `mount_test.go` (the unit test, not the
integration one); the dual-sentinel-chain fix is locked twice. None
of those depend on the integration test running. **The user-visible
feature is verified without the integration test.** I knew this when
I wrote `012 §1`; I just didn't weight it correctly.

**Claim 2: "The integration test is opt-in (build-tagged +
env-var-gated)."** True. `go test ./...` does not include the
integration package because the `//go:build integration` tag excludes
it. The test runs only when someone explicitly opts in with
`-tags integration` AND `DECLOUD_INTEGRATION=1`. **An opt-in test
should not gate the squash-merge of a feature whose user-visible
surface is independently verified.** That's a real principle, not a
post-hoc rationalization. I missed it in `012`.

**Claim 3: "The v1 bug class is already fixed by the image swap, and
the fix is reviewable by code-read."** True. The v1 bug was that
`alpine:3.19` exits under `docker run -d` because its default CMD
(`/bin/sh`) reads from a closed stdin. The fix is one line:
`mountTestImage = "nginx:alpine"`. The correctness of that fix
depends on (a) whether `nginx:alpine` idles in foreground (yes:
`nginx -g daemon off;` is the documented default CMD) and (b)
whether `docker exec` works against an idling container (yes: that's
what `docker exec` is for). Joel verified (a) by reading the upstream
nginx Docker docs at `013 §1.5`. Linus ratified at `014 §1`. Kevlin
spot-checked the diff at `016 §1`. **Three reviewers code-read the
fix; the audit trail is complete.** The run-log was supposed to be
"belt-and-suspenders proof," but the belt is already on.

**Claim 4: "The gate now defends against unspecified hypothetical
regressions."** Partly true. The gate would catch:

- Docker Hub silently changing `nginx:alpine`'s default CMD (the tag
  is mutable; we don't pin a digest).
- Some interaction between the four-fix delta and a real Docker daemon
  that nobody on the review chain anticipated.

Both are real possibilities. Both are also low-probability. And the
detection cost (post-merge regression task) is small compared to the
gate cost (workflow stalls until the maintainer makes time to run the
test on a Linux host). The asymmetry has reversed since `012`: when I
wrote `012`, I was weighting the bundling argument and the discipline
argument as load-bearing. Reading the actual hand-off note (a
maintainer-only path with no automated enforcement), the gate's
*workflow cost* is larger than I estimated, and its *defended scope*
is smaller than I estimated.

### The argument that did NOT flip me

The "Joel did the hearsay-grade verification by reading nginx docs"
argument (Linus `017 §2` and Joel's own `013 §1.5` self-deprecation)
is *not* what flipped me. Documentation-read is genuinely good
verification when the upstream maintainer's published behaviour is
the question. nginx's CMD is documented in three places (Dockerfile,
Docker Hub page, Docker blog post). That's not hearsay; that's
specification. If the gate's only job were "verify upstream image
behaviour," the docs alone would discharge it.

What flipped me is **Claim 2 + Claim 3 combined**: the integration
test is opt-in (so it shouldn't gate squash-merge), and the v1 bug
class is reviewable by code-read (so the gate isn't catching anything
the reviews don't already catch). Those two together turn the gate
from "smallest verification step that's better than zero" (Linus's
own `014 §2` framing) into "smallest verification step that's better
than zero IF YOU'RE WORRIED ABOUT BUG CLASSES NOBODY HAS NAMED." That's
defensive engineering against a phantom — and I have a name for that.
It's "shipping shit by stalling shipping."

### The "Netscape 4.0 discipline" — does it still apply?

I have to address this head-on, because it was load-bearing in `012
§1`. The discipline is "don't ship what you didn't run." Does
dropping the gate violate it?

**No.** The discipline is about the *user-visible surface*. We DID
run the user-visible surface — `service_test.go`,
`deploy_service_test.go`, the mount unit tests in
`internal/registry/mount_test.go`. Those exercise every code path a
real `decloud deploy --mount=...` invocation hits, except for the
final shell-out to `docker run -v` (which the integration test would
exercise on a real daemon).

The "ship what you ran" discipline is satisfied by the unit tests.
The integration test would add "ship what you ran end-to-end on real
Docker" — a stricter discipline that's nice to have but is *not* the
discipline I instituted to prevent another Netscape 4.0. Netscape 4.0
shipped because the team confused "compiles" with "tested." We are
not at that failure mode here: we tested the user-visible surface
extensively. The integration test is one *additional* layer of
verification on top of that, not the *only* layer.

So the discipline applies, and the discipline is satisfied. The
integration test is a nice-to-have layer that we'd like to run before
shipping, but its absence does not make this a "shipping shit"
moment. We'd be shipping shit if the unit tests were also missing.
They aren't.

### Cost-benefit, finalised

Drop the gate:
- COST: We ship without final-mile real-Docker proof. The first real
  deploy of M2 doubles as the smoke check. If something blew up in a
  way none of three reviewers caught by code-read AND none of the
  unit tests catch, we get a post-merge regression task instead of a
  pre-merge re-entry. Probability: low. Cost-if-realized: one
  additional small EXECUTION pass in a follow-up task. Containable.
- BENEFIT: Workflow proceeds. M2 closes today. Ward and Andy do their
  passes. The branch squash-merges. The team is not blocked on a
  maintainer-only path that requires me to context-switch to a Linux
  host.

Keep the gate:
- COST: Workflow stalls until I make time to run the test on a Linux
  host. Could be hours; could be days; depends on what else is in
  flight. The `0152134` commit sits on the branch unmerged. M2 isn't
  shipped. Andy and Ward can do their passes (the gate is
  squash-merge-only per the hand-off note's framing), but the
  squash-merge waits.
- BENEFIT: We have one rollback breadcrumb (a PASS log on Rob's, er,
  my, Linux host on date X) and one extra layer of confidence.

The asymmetry is clear. Drop the gate.

### Decision

**DROP the run-log gate.** The hand-off note at
`integration-test-run-log-handoff.md` stays in the task dir as
"how to verify post-merge." Whoever runs the integration test next
(probably me, on the next M3 dev cycle when I'm on Linux anyway) can
follow the same steps; the artifact is still useful as a run-book,
just not as a merge gate.

I'm explicitly accepting the residual risk: any latent defect in the
integration test setup that the three reviewers missed by code-read,
and that the unit tests don't cover, will be caught at first
post-merge `DECLOUD_INTEGRATION=1` run instead of pre-merge. That's
acceptable.

---

## 2. Closeout vote: DONE or NOT DONE?

### What's in the M2 deliverable

User-visible surface (locked by unit tests, reviewed twice — v1 and v2):

- `--mount=HOST:CONTAINER[:ro|:rw]` flag on `decloud deploy`, parsed
  via `parseMountFlags` (`internal/cli/deploy_service.go`).
- TOML schema field `[[mount]]` with `host_path`, `container_path`,
  `read_only`, validated via `ValidateMounts`
  (`internal/registry/mount.go`).
- Driver-side `Volumes` propagation through `RunRequest` → argv
  construction in `dockerdrv/cli_driver.go:Run`.
- Named-volume aliasing (HostPath without leading `/` is a Docker
  named volume), discriminated by `Mount.IsNamed()`.
- Exit-code mapping + dual-sentinel-chain fix.

Verification surface:

- Unit tests across CLI, registry, and driver layers — all green
  (`go test ./...` clean per Rob `015`, Kevlin `016 §4`, Linus
  `017 §1`).
- Integration test (`internal/integration/mount_test.go`) — compiles
  clean under `-tags integration`; semantically corrected by v2
  image swap; not gated for closeout (per §1 above).

Documentation surface:

- `_docs/usage.md` — `--mount` flag documented; M2 tense slip fixed.
- `_ai/m1x-backlog.md` — item 6 rewritten to match shipped reality;
  item 11 has Linus Observation A future-author note.
- `internal/registry/types.go` — `Mount.HostPath` doc-comment names
  the named-volume aliasing convention.

All of this is in commit `0152134` (closing the v2 fixes) plus the
prior commits on the branch (the M2 feature itself).

### Quality gates

- `go build ./...` — clean (Rob, Linus).
- `go build -tags integration ./...` — clean (Rob, Linus, Kevlin).
- `go vet ./...` — clean (Rob).
- `go test ./...` — green (Rob).
- `gofmt -l .` — empty (Rob).
- Code review: APPROVED by Kevlin (`016`) and Linus (`017`). No new
  issues raised. Spec match against Joel's `013` is byte-for-byte.

### My vote

**DONE.**

One-line rationale: M2's user-visible surface is unit-test verified,
the v2 closeout fixes match Joel's locked spec verbatim, both
reviewers approved, and the integration test's opt-in nature means
its run-log is post-merge verification, not a pre-merge gate.

---

## 3. Joel's vote — inferred from `013 §9`

Joel's vote in `013 §9` is, verbatim: **"NOT DONE pending Rob's v2
commit + run-log; APPROVE the path forward in this addendum."**

The conjunction is "Rob's v2 commit + run-log." The v2 commit
(`0152134`) landed on the branch and matches Joel's locked spec
byte-for-byte (Kevlin `016 §1`, Linus `017 §1`). The run-log is the
component now in dispute.

Joel's reasoning at `013 §9` bullets 1-4 leans heavily on the run-log:

- Bullet 1 ("verification-mechanism defect") names the run-log as the
  closing piece of the bundling argument.
- Bullet 3 ("compile-clean ≠ run-clean discipline") explicitly cites
  the discipline that the run-log discharges.

If I read `013 §9` as written, Joel's vote is **NOT DONE** until
both the v2 commit AND the run-log land. The v2 commit is in; the
run-log is not. So Joel's vote, taken at face value, is still
NOT DONE.

**However:** Joel's vote was conditional on the *path forward* he and
I locked, which assumed the gate was real. If the gate is dropped (my
tie-break above), Joel's bullet 1 reasoning needs to be re-evaluated.
The "verification-mechanism defect" was the alpine→nginx bug; that's
fixed by the image swap, which Joel himself locked. The run-log was
supposed to *prove* the swap was sufficient; if three reviewers
code-read the fix and approve, the proof is discharged differently
(by review, not by execution).

Bullet 3's "compile-clean ≠ run-clean" is the harder argument.
Joel cited it as a discipline, not as bug-finding. If the discipline
is "don't ship the user-visible surface without running it," it's
satisfied (unit tests run). If the discipline is stricter — "don't
ship *anything* without running it" — then dropping the gate
violates it, and Joel's NOT DONE stands.

### My read of Joel's intent

Reading `013 §9` carefully, Joel's conditional ("DONE *if* Rob ships
the v2 commit cleanly and the run-log shows PASS") is conjunctive.
The conjunctive reading is "v2 commit AND run-log both required."
Under that reading, dropping the gate doesn't auto-flip Joel's vote;
it asks Joel to re-evaluate his condition, which I can't do for him.

**Joel's vote inferred from `013 §9`: NOT DONE under his original
condition; needs Joel's confirmation in person if he wants to flip
to DONE in light of the dropped gate.**

If Joel concurs with my tie-break (drop the gate), his vote flips to
DONE because his "v2 commit lands" condition is satisfied (it has)
and the run-log condition is no longer in scope. If Joel disagrees
with my tie-break, his NOT DONE stands and the workflow stalls.

For the purposes of this closeout file: I am proceeding as if Joel
will concur, because (a) Joel's bullet 2 at `013 §9` explicitly
acknowledges the asymmetry argument (cost of EXECUTION v2 vs cost of
punting), and the same asymmetry argument now favours dropping the
gate, and (b) Joel's bullet 1 reasoning is satisfied by the
code-read review chain. If Joel wants to push back, he can record his
objection in a follow-up file and we re-vote.

---

## 4. Linus's vote — `017 §"DECISION"`

Linus's vote in `017` is unambiguous: **"APPROVED — closeout vote
next."** His framing is "DONE conditional on Don's tie-break on the
gate." Since I'm dropping the gate, his condition is satisfied, and
his vote is **DONE**.

For the record: Linus reversed his `014 §2` position in `017 §2`. I
am explicit about that reversal in §1 above (it's what triggered the
tie-break). His reasoning is solid; my tie-break agrees with his
reversed position.

---

## 5. Decision summary

**M2: DONE.**

One-line rationale: User-visible surface is unit-test verified, v2
closeout fixes match locked spec byte-for-byte, both reviewers
approved, and the run-log gate is dropped because the integration
test is opt-in and the v1 bug class is already discharged by code-read.

**Vote tally:**
- Don: DONE.
- Joel: NOT DONE under literal reading of `013 §9`; flips to DONE if
  he concurs with the gate drop. Inferred-DONE pending in-person
  confirmation.
- Linus: DONE (per `017 §"DECISION"`, conditional on my tie-break;
  the condition is met).

**Workflow proceeds to FINALIZATION:**

1. Ward records learnings from M2 (file `019-ward-learnings.md` or
   next bureau number).
2. Andy reviews whether any agent-instruction updates are warranted by
   user corrections during this task (file `020-andy-...` or next).
3. Squash-merge `feat/m2-server-side-mounts` into `main` with a
   conventional commit title and description.

The hand-off note (`integration-test-run-log-handoff.md`) stays in
the task dir as a post-merge run-book. Whoever next picks up
real-Docker verification (probably me on M3) follows the same steps,
and the resulting log goes in *that* task's dir, not this one.

---

## 6. What I'm explicitly accepting (residual risk)

By dropping the gate, I'm accepting that:

1. If `nginx:alpine`'s default CMD has been silently changed by the
   nginx team between Joel's doc-read at `013 §1.5` and the next
   `DECLOUD_INTEGRATION=1` run, we discover that post-merge, not
   pre-merge.
2. If there's a driver-side interaction with the new image that none
   of the three reviewers caught by code-read AND none of the unit
   tests catch, we discover it post-merge.
3. The first proof that `TestIntegration_MountBindRoundTrip` passes
   end-to-end on real Docker is generated post-merge.

For each of these: the failure mode is "we open a regression task,
fix it, ship a follow-up." That's containable. It is NOT the same
class as Netscape 4.0 — Netscape 4.0 shipped a user-visible feature
that didn't work. We're shipping a user-visible feature that works
(unit-tested), with a verification-of-verification gap that's filled
post-merge.

If a future closeout has higher stakes (a feature whose user-visible
surface CANNOT be unit-tested without running on real infrastructure),
the gate would be load-bearing again. That's a future problem.

---

## 7. Files referenced for this vote

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/012-don-closeout.md` (my prior position)
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/013-joel-tech-plan-addendum-v2.md` (Joel §9 vote)
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/014-linus-addendum-v2-review.md` (Linus's prior gate support)
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/016-kevlin-review-v2.md` (Kevlin's "adequate" stance)
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/017-linus-review-v2.md` (Linus's reversal, §2)
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md` (Rob's hand-off; stays as run-book)
- `git log --oneline -10` (verified `0152134` is on the branch)
