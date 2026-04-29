# 014 — Linus's review of Joel's addendum v2

Reviewing `013-joel-tech-plan-addendum-v2.md` against Don's closeout
(`012-don-closeout.md`) and my own impl review (`011-linus-impl-review.md`).
Scope: the four-fix delta + the verification gate. NOT a re-audit of M2.

## TL;DR

Joel locked the right shape. Image swap to `nginx:alpine`, three doc fixes
folded in, run-log committed as a separate gating commit. Every concern I'd
raise about this addendum has either already been pre-empted by Joel
(gotcha checks at §1.5) or is small enough to bundle now.

The four questions you asked me — image choice, run-log gate, Kevlin's
three, closeout vote — all come back the same way: **APPROVED.**

---

## 1. Is `nginx:alpine` actually the right image?

### Short answer: yes, but for less obvious reasons than Joel argues.

Joel argues the merit is "nginx idles in foreground via `nginx -g daemon off;`,
22MB pull is noise on a CI runner that already pulls Go toolchains." That's
correct as far as it goes. But the deeper question you raised — "is there
a smaller long-running-by-default image that's more honest about 'we just
need a container that doesn't die'?" — deserves a real answer.

**Candidates I considered:**

| Image | Compressed size | Default CMD | Honest about purpose? |
|---|---|---|---|
| `alpine:3.19` | ~7MB | `/bin/sh` (exits) | yes, but doesn't idle |
| `alpine:3.19` + Cmd override | ~7MB | n/a (we'd inject `sleep infinity`) | yes, but requires Option B (Cmd field) |
| `busybox:latest` | ~4MB | `sh` (exits) | same as alpine |
| `busybox:1.36` with `sleep 3600` | ~4MB | requires Cmd override | same as alpine + override |
| `nginx:alpine` | ~22MB | `nginx -g daemon off;` (idles) | no — implies "we need a web server" |
| `traefik:latest` | ~50MB | runs traefik | no — same problem, larger |
| `registry:2` | ~10MB | runs Docker registry | no — same problem |
| `pause` (k8s sandbox) | ~700KB | sleeps forever | yes, but Google-distributed, less universal |
| `hashicorp/http-echo` | ~5MB | echoes HTTP | no — implies HTTP testing |

**The honest answer:** there is no off-the-shelf "tiny container that idles
and is honest about doing nothing" in the public Docker Hub catalog that
also meets our "lingua-franca, already cached" bar. `pause` images exist
but they're k8s-ecosystem and add a layer of explanation ("why are we
pulling a Kubernetes infrastructure image to test bind mounts?").

So the choice is between:

- **`nginx:alpine` (Joel's pick):** ~22MB, lingua-franca, idles via
  documented CMD. Slightly dishonest signal ("we need a web server"),
  but mitigated by the m1x-backlog item 6 paragraph rewrite (Fix C)
  which explicitly says "the `nginx:alpine` choice is deliberate:
  nginx idles in the foreground..." — i.e. the dishonesty is paid off
  in writing.
- **Option B revival (`alpine:3.19` + `Cmd: []string{"sleep", "infinity"}`):**
  ~7MB, fully honest ("alpine + sleep" is the canonical "container that
  does nothing" recipe), but requires adding `Cmd []string` to
  `RunRequest` which Don and Joel both rejected as M2 scope creep
  (m1x-item-11 territory).

**My take: Joel's call is right.** The 15MB delta is genuinely noise on
any modern CI runner. The "dishonesty" of nginx-as-idler is paid off by
the m1x-backlog item 6 rewrite that explicitly explains the choice
(Fix C, with Joel's added sentence at line 325 of the addendum: "The
`nginx:alpine` choice (rather than alpine) is deliberate: nginx idles in
the foreground via `nginx -g daemon off;`..."). Future-Don will not be
confused; the explanation is on-tree.

**What I would NOT accept** is `nginx:alpine` with no explanatory note in
the backlog. The explanation is what makes the dishonesty acceptable.
Joel folded the explanation into Fix C. Approved.

**Counter-argument I'm rejecting:** "use `nginx:alpine` because it's
already cached on every CI runner that uses Docker." This is folklore.
GitHub Actions runners don't pre-cache `nginx:alpine` any more than they
pre-cache `alpine:3.19`. Both are network pulls on first run. The
"already cached" argument is wrong. The real argument is: 22MB is small
enough to not matter.

**Re: integration test slowdown:** the test runs once per CI invocation
(if it runs at all — gated on `DECLOUD_INTEGRATION=1`). 22MB pull at
typical CI bandwidth (50-100 MB/s on cloud runners) is sub-second. The
test runtime is dominated by `nginx:alpine` startup (~1s for nginx to
start listening) and `docker exec` round-trip (~100ms). Total test
runtime: ~3-5 seconds end-to-end. Image size is not the bottleneck.

**Verdict on Q1: nginx:alpine is the right call. Joel's reasoning is
correct and the m1x-backlog Fix C makes the choice self-documenting.**

---

## 2. Does the run-log committed to task dir prove anything?

### Short answer: yes, but not what you might think.

You raised the right concern: "the run-log itself doesn't prove anything
is reproducible." Correct. A `--- PASS: TestIntegration_MountBindRoundTrip`
line in a text file does not prove that the next person running the test
will see the same result. It proves only that on Rob's machine, on one
specific invocation, the test passed.

**So why is the gate not superstition?**

Three reasons:

1. **It proves the test was actually run, not just compiled.** Before the
   gate, Kent and Rob both reported "go build -tags integration clean" as
   verification. That's compilation. The shipped test had a verification-
   mechanism failure (alpine + no Cmd → exits before exec) that
   compilation cannot catch. The run-log is the artifact that proves
   somebody actually executed the test against real Docker. Without it,
   we have no evidence the test works at all.

2. **It proves the four-fix delta is sufficient.** The whole point of
   the v2 EXECUTION pass is "swap alpine→nginx:alpine and the test
   passes." If the run-log shows PASS, the swap was sufficient. If it
   shows FAIL with some new error, we have a different bug to chase
   before close-out.

3. **It creates a rollback breadcrumb.** If a future regression breaks
   the integration test, the on-tree run-log is evidence that the test
   USED to pass against `nginx:alpine` at this commit. That's diagnostic
   value — "did we ever ship it green?" is answered yes, with a date and
   a commit SHA on the log entry.

**What the run-log does NOT prove (and Joel didn't claim it does):**

- It doesn't prove reproducibility. Different Docker versions, different
  kernel versions, different host filesystems (overlay2 vs btrfs vs ZFS)
  could all produce different results. A single PASS log doesn't bound
  that variability.
- It doesn't prove the test will pass on CI. Rob's machine is Rob's
  machine; CI runners are different. (Decloud has no CI yet, so this is
  moot, but it'll matter when CI lands.)
- It doesn't prove the test will pass tomorrow. Docker Hub might change
  what `nginx:alpine` resolves to (the tag is mutable). The pin is
  `nginx:alpine`, not `nginx:alpine@sha256:...`.

**The gate is the smallest verification step that's better than zero.**
Without it: "we shipped a feature whose integration test was never run
against real Docker." With it: "we shipped a feature whose integration
test was run against real Docker exactly once, on the maintainer's box,
on date X, against image-tag-resolved-to-digest Y." The second is
genuinely better. Not perfect — but the gate is "did we run it" not "is
it reproducible," and "did we run it" is the question we need to answer.

**Counter-arguments I'm rejecting:**

- "The gate is superstition because the log proves nothing." Wrong.
  The log proves the test was run end-to-end at least once. That's the
  delta from the v1 ship state, where the test was confirmed only to
  compile. Compile-clean ≠ run-clean. The log is the run-clean evidence.
- "We should pin to a digest (`nginx:alpine@sha256:...`)." Tempting,
  but introduces a maintenance burden (whose digest? rotated when?) and
  a CI-stability vs. security-update tradeoff that's outside M2 scope.
  Could be a future m1x-backlog item if we ever care. Don't fold in now.
- "The gate should require multiple runs (matrix of Docker versions)."
  Way out of M2 scope. The gate is "did we run it once on a real Docker"
  not "did we prove it works on every Docker version ever shipped." One
  run is enough to verify the four-fix delta.

**Verdict on Q2: the gate is right-sized. It's not superstition; it's
the minimum evidence that the test was actually run, and it's the only
defense against the v1 mistake (ship without running). Anything more
would be over-engineering.**

---

## 3. Do Kevlin's three doc fixes get bundled correctly?

### Short answer: yes, all three folded correctly. One small catch.

Walking through Joel §2/§3/§4 against Kevlin §10:

### Fix A — `Mount.HostPath` doc-comment (Joel §2)

Joel reproduces Kevlin's suggested wording verbatim. Three lines of comment
above `HostPath`, no field shape change, no test surface change. Correctly
folded.

**Catch:** Joel notes at §2.3 last sentence: "Use Mount.IsNamed() to
distinguish at runtime." That's pointing to a method that exists at
`internal/registry/mount.go:19` (per Kevlin §1's verification). Cross-
package readers IDE-jumping to `Mount` see only the field; they have to
chase `IsNamed()` separately. The doc-comment closing the loop is
correct. **Approved.**

### Fix B — `_docs/usage.md:3` tense slip (Joel §3)

`Operator-facing reference for the Decloud M1 CLI.` → `Operator-facing
reference for the Decloud CLI.` (drop "M1 ").

Trivial. Correctly folded. **Approved.**

### Fix C — `_ai/m1x-backlog.md` item 6 "M2 delivery" rewrite (Joel §4)

This is the meaty one. Joel takes Kevlin's suggested rewrite and adds
TWO important deltas (§4.2):

1. Image is `nginx:alpine` (post-fix), not `alpine:3.19` (Kevlin's draft
   was written before the image swap was decided).
2. One-sentence explanation: "The `nginx:alpine` choice (rather than
   alpine) is deliberate: nginx idles in the foreground via `nginx -g
   daemon off;`, so the container stays alive long enough for `docker
   exec`; alpine's default `/bin/sh` CMD exits under `docker run -d`
   (Linus's catch in `011-linus-impl-review.md` §5, fix in EXECUTION v2)."

Both deltas are correct. The second one is doing the load-bearing work I
flagged in §1 above — it makes the otherwise-dishonest signal of
"nginx-as-idler" self-documenting on-tree. Without that sentence, future-
Don reads the backlog and asks "why nginx?" With it, future-Don reads
"because alpine exits under -d, see §5 of 011." That's the right shape.

**Bonus addition by Joel — Linus Observation A folded into item 11.**
Joel goes beyond Kevlin's scope to fold my "future-author note" about
`Cmd []string` on the consolidated `RunOptions` into m1x-item 11. This
is exactly what I asked for in `011-linus-impl-review.md` §"Observation
A" — name the gap now so future-author doesn't have to re-derive it.
The wording at Joel §4.3 is correct: it names the gap, names the
workaround (image swap), names where to look (`011 §"Observation A"`),
and explicitly says "consolidated `RunOptions`" so the future-author
lands it in the right struct (not on the deprecated `RunRequest`).

**Verdict on Q3: all three doc fixes correctly bundled. The Cmd-field
future-author note (Observation A) is a bonus addition that I welcome.**

### Anything missing?

I went looking. Three things to check:

1. **Kent's reports** (`007-kent-tests.md`) — do they need an addendum?
   No. Kent's report is a snapshot of the RED commit; v2 doesn't
   invalidate it, just adds a follow-up image swap. The narrative
   "Kent shipped a test that compiled but never ran on real Docker" is
   already documented in `011-linus-impl-review.md` §5 and ratified in
   `012-don-closeout.md` §1. No need to amend Kent's report.

2. **Rob's report** (`008-rob-impl.md`) — similar question. No amendment
   needed; Rob's report is a snapshot of the GREEN commit. v2's commit
   will get its own report (Joel §7 step 3 names it
   `015-rob-impl-fix.md`). The original report stays as-is.

3. **Raymond's docs** (`009-raymond-docs.md`) — is there any doc surface
   we're missing? I went through the M2 docs in my v1 review §"Files
   reviewed" list. The four files Joel touches in v2 (mount_test.go,
   types.go, usage.md, m1x-backlog.md) cover everything Kevlin flagged.
   Raymond's report doesn't need amendment either.

**Nothing missing.** The four-fix bundle is exhaustive against Kevlin's
review, Don's closeout, and my v1 review. No fifth fix surfaces from
re-reading.

---

## 4. Closeout vote

### What Joel locked, in order:

1. One-line image swap in `internal/integration/mount_test.go:23`.
2. Three-line doc-comment block in `internal/registry/types.go` above
   `Mount.HostPath`.
3. One-line tense slip fix in `_docs/usage.md:3`.
4. Item 6 paragraph rewrite + item 11 future-author note in
   `_ai/m1x-backlog.md`.
5. Verification gate: real-Docker run-log committed as a separate
   commit, with PASS-line check, after the four-fix commit lands.

### What Joel did right:

- Reproduces my Option-A reasoning verbatim and adds his own gotcha
  checks (§1.5: nginx signal handling, port collision, image pull time).
  Each gotcha is anticipated, traced to the relevant code line, and
  closed. This is the level of due-diligence an addendum should hit.
- Names the alternatives I rejected (Options B/C/D) and re-rejects each
  with the same reasoning. Doesn't try to re-litigate my §5 analysis.
- Adds the m1x-item-11 future-author note (my Observation A) without
  being asked to. Self-driven scope expansion in the right direction.
- Locks the verification-gate workflow in §5: exact command, exact
  output file path, exact PASS-line substring to look for, what to do
  if it fails (don't commit a FAIL log; diagnose and re-run).
- Explicitly votes "NOT DONE pending Rob's commit + run-log" but
  "APPROVE the path forward in this addendum" — the right conditional
  shape for an addendum that hasn't been executed yet.

### What I'd push back on (non-blocking):

- Joel §1.3 says "verified via the upstream nginx official image
  documentation." He couldn't actually run `docker inspect` (no Docker
  daemon on his box). The verification is by reading the nginx Docker
  Hub page, which is hearsay-grade evidence. Acceptable for a doc
  addendum, but Rob's run-log is the actual proof. If Rob's first run
  fails because nginx:alpine's CMD has been silently changed by the
  nginx team, we'll know.
- Joel §5.4 says Rob should grep for `--- PASS:
  TestIntegration_MountBindRoundTrip`. That's correct. He should ALSO
  grep for `--- FAIL:` and abort if found, which §5.5 covers. Belt and
  braces. Fine.
- Joel §6 says "no test-surface deltas." Correct in spirit (no new
  test, no removed test, no assertion changes). The `mountTestImage`
  constant changes — that's a constant change, not a test-surface
  change. Pedantically I'd phrase it differently, but the substance is
  right.

None of these are blockers. They're observations about the addendum's
prose, not about the four-fix delta.

### My vote

**APPROVED — Rob can fix and run-log.**

The addendum is precise, the four-fix delta is well-specified, the
verification gate is right-sized, and Joel's pre-emptive gotcha checks
(§1.5) cover the second-order concerns that would otherwise surface in
review. The m1x-item-11 future-author note is a bonus.

Rob can apply the four fixes mechanically against this addendum, run
the integration test against real Docker, commit the PASS log, and we
proceed to PLAN re-entry v3 (Don/Joel/Linus closeout vote). Kevlin and
I will re-review the v2 delta in parallel before that final vote.

If Rob's run-log shows FAIL instead of PASS — for any reason —
we re-open the plan. But I'd bet hard money it shows PASS. The fix is
mechanical, the test is well-formed, and `nginx:alpine` does what Joel
says it does (verified by my own background reading of the nginx
Docker docs).

---

## DECISION: APPROVED — Rob can fix and run-log.

No additional rounds needed at the plan stage. Joel's addendum-v2 locks
the right shape; Rob's commit is mechanical against it; the run-log is
binary PASS/no-closeout.

Files reviewed for this addendum:

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/010-kevlin-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/011-linus-impl-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/012-don-closeout.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/013-joel-tech-plan-addendum-v2.md`
- `/Users/fenster/dev/decloud/internal/integration/mount_test.go` (verified line 23 is the only point of change)
