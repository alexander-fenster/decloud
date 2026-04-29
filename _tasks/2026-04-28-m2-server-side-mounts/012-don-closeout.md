# 012 — Don's PLAN re-entry closeout (M2 server-side mounts)

PLAN re-entry after Kent (RED) → Rob (GREEN) → Raymond (docs) → Kevlin + Linus
(parallel review). Both reviewers approved with minor fixes.

This file decides: another EXECUTION pass, or close out with backlog deferrals?

## TL;DR

**Another EXECUTION pass IS needed.** The integration-test bug Linus found
(`011-linus-impl-review.md` Q5) is a real correctness defect, not a doc tidy.
Fix is small (one-line image swap, Linus Option A) but it must be ACTUALLY
RUN against real Docker before close-out — that's a verification gate, not
something that can be folded into a "doc tidy" commit alongside the three
Kevlin fixes.

The three Kevlin doc fixes (`010-kevlin-review.md` Fix A/B/C) are trivial and
will be bundled with the integration-test fix in the same EXECUTION pass.

Workflow:
1. Joel writes a small addendum (`013-joel-tech-plan-addendum-v2.md`) locking
   the integration-test fix shape (Linus Option A: `nginx:alpine`) plus
   Kevlin's three doc fixes.
2. Rob applies all four fixes in one commit. Then RUNS
   `DECLOUD_INTEGRATION=1 go test -tags integration
   ./internal/integration/...` against real Docker on the maintainer's box
   and commits the PASS log to the task directory.
3. Kevlin and Linus re-review the delta in parallel
   (`015-kevlin-review-v2.md`, `016-linus-impl-review-v2.md`).
4. PLAN re-entry v2 (this file's successor) — Don/Joel/Linus closeout vote.
5. If v2 closes clean, proceed to FINALIZATION (Ward → Andy → squash-merge).

## 1. Linus's REQUIRED fix — integration-test alpine-no-Cmd bug

### Bug verified

I traced it. Confirmed:

- `internal/dockerdrv/driver.go:31-39` — `RunRequest` has fields
  `Name/Image/Network/Env/Restart/Port/Volumes`. **No `Cmd` field.**
- `internal/dockerdrv/driver.go:77-86` — `RunOptions` (used by
  `RunWithOptions`) likewise has no `Cmd` field.
- `internal/dockerdrv/cli_driver.go:65` — `Run` ends with
  `args = append(args, req.Image)`. No way to inject a Cmd suffix.
  `RunWithOptions` (line 241) is identical: `args = append(args, opts.Image)`.
- `internal/integration/mount_test.go:69-77` — calls
  `driver.Run(ctx, RunRequest{Image: "alpine:3.19", ...})` with no Cmd
  override. The shipped argv is therefore
  `docker run -d --name decloud-mounttest --network decloud --restart no
  -v <tmp>:/data:ro --label decloud.service=mounttest alpine:3.19`.
- `alpine:3.19`'s default CMD is `/bin/sh`. With `-d` (detached, no `-i`),
  `/bin/sh` reads from a closed stdin and exits status 0 immediately.
- The subsequent `driver.Exec` against an exited container will fail with
  "Container ... is not running" from the daemon.

Linus's diagnosis is correct: this test cannot have been executed on real
Docker. Kent reported `go build -tags integration ./...` clean and Rob
reported the same — that's compilation, not execution. **Nobody actually ran
`DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/...`
end-to-end before signing off.** That's an EXECUTION-pass deficiency.

### Why this matters (Don's call, brutal version)

The whole bundling argument we made at plan stage said: ship an integration
test in M2 BECAUSE we want automated verification of the mount feature
against real Docker. The unit tests already lock the `Driver.Run` argv
shape via `volumeFlagsFromArgs`. The integration test's *only* job is "does
real Docker accept this argv and surface the file inside the container?"
A test that exits before `docker exec` runs answers neither question.

Shipping this would be exactly the moral failing my "every bug is a moral
failing" line is about — Kent and Rob confused compile-clean for run-clean.
"Go build green" is not "feature works." That's the Netscape 4.0 mistake
(don't ship what you didn't run). Not on my watch.

### Decision: Linus Option A (`nginx:alpine`)

I evaluated all four:

- **Option A (image swap to `nginx:alpine`)**. One-line change in the test.
  Image idles on a foreground nginx process; `docker exec cat /data/marker.txt`
  works against the running container. Production code untouched. m1x-item-11
  stays clean.
  - Cost: ~22MB pull on CI vs ~7MB for alpine. Noise on a CI runner that
    already pulls Go toolchains.
  - Pro: zero production change, smallest blast radius, exercises the
    correct surface (`Driver.Run` + `Driver.Exec`).
- **Option B (add `Cmd []string` to `RunRequest` + plumb in `cli_driver.go:Run`)**.
  Matches my plan §8 wording exactly (`/bin/sh -c 'cat ... ; sleep 60'`).
  - Cost: production-code change for a test-only need. Every existing
    `Driver.EXPECT().Run(RunRequest{...})` mock site stays the same (`Cmd`
    zero-valued is fine), but the field becomes part of the driver contract
    forever. m1x-item-11 (Run/RunWithOptions consolidation) is the right
    vehicle for designing `Cmd` properly across both run paths — fixing it
    here in M2 pre-empts that work and risks doing it wrong twice.
  - Reject for M2.
- **Option C (test bypasses `Driver.Run`, shells out raw)**. Anti-pattern.
  The integration test exists to exercise `Driver.Run`'s argv construction
  on real Docker. Bypassing the driver answers a different question. Reject.
- **Option D (revert the integration test, file as new m1x-item-12)**.
  Negates the bundling argument we made at plan stage. The whole reason
  `internal/integration/mount_test.go` shipped in M2 (rather than being
  deferred wholesale to the original m1x-item-6) was that the mount-only
  failure mode is independently debuggable from the curl-through-Caddy
  failure mode. Reverting now means we shipped M2 with no real-Docker
  mount verification at all. Reject.

**Choose A.** Smallest fix, zero production touch, actually verifies M2 on
real Docker, doesn't pre-empt m1x-item-11.

### What Joel's addendum must lock

1. `internal/integration/mount_test.go:23` — change
   `mountTestImage = "alpine:3.19"` → `mountTestImage = "nginx:alpine"`.
   Update the file-top comment if any (none currently).
2. **Verification gate**: Rob (or whoever picks up the EXECUTION pass) must
   run `DECLOUD_INTEGRATION=1 go test -tags integration -v
   ./internal/integration/... 2>&1 | tee
   _tasks/2026-04-28-m2-server-side-mounts/014-rob-impl-fix.md` against real
   Docker on the maintainer's box. The PASS log goes in the task report. No
   PASS log → no closeout. (Linus said this verbatim in `011-linus-impl-review.md`
   §5, last paragraph: "the test must be ACTUALLY RUN against real Docker
   before M2 closeout, not just compiled." I agree.)
3. **Note for m1x-item-11**: append a one-line "Future-author note" to
   item 11 in `_ai/m1x-backlog.md` saying that the consolidated `RunOptions`
   should grow `Cmd []string` so any future integration test (or one-shot
   job at M5+) doesn't need to pick a specific image-with-idle-CMD. This is
   Linus's Observation A. Doc-only addition. Folds into Rob's same commit.

### Concern: arguing this is "small enough to fold in"

You could argue: the integration test is build-tagged and opt-in
(`DECLOUD_INTEGRATION=1`). The shipped surface (`--mount` flag + loader +
runtime) is unit-test verified end-to-end. Therefore the test bug doesn't
affect shipped binary correctness, only "did we exercise it on real Docker."
Couldn't we punt this to a backlog item and close out M2 today?

**No.** Three reasons:

1. The bundling argument we used to ship the integration test in M2
   *explicitly was* "we want real-Docker verification before closeout." If
   we punt, that argument retroactively becomes a lie. Either the
   integration test verifies M2 (in which case the bug is a closeout
   blocker) or the integration test is just decoration (in which case it
   shouldn't have been bundled). Pick one.
2. The fix is one line. The cost of another EXECUTION pass is small (Joel
   addendum + Rob commit + two reviews + PLAN v2). The cost of "ship now,
   fix in m1x-item-12" is that we ship M2 having lied about verification,
   and the m1x-item-12 backlog entry will sit there for an unknown number
   of milestones. The asymmetry favours fixing now.
3. "Compile-clean ≠ run-clean" is the kind of discipline I instituted
   *exactly* to prevent another Netscape 4.0. If I let it slide here, I'm
   teaching the team that "compile-clean is good enough when it's late and
   the reviewer is being picky." That's how shit ships.

So: another EXECUTION pass.

## 2. Kevlin's three optional fixes — fold into the same EXECUTION pass

All three are trivial doc-only edits. Bundle them with Rob's
integration-test fix to keep the EXECUTION pass to one commit (modulo the
real-Docker run-log commit). Specifics:

### Fix A — doc-comment on `Mount.HostPath` in `internal/registry/types.go`

Wording per Kevlin §10 Fix A. Three-line block comment promoting the
named-volume-aliasing convention (currently only documented on
`Mount.IsNamed()`) to first sight when reading the struct definition.
Matches Joel §3.1 / Linus v1 Issue 3 implication. Zero behaviour change.
**Fold in.**

### Fix B — drop "M1" from `_docs/usage.md:3`

Current: `Operator-facing reference for the Decloud M1 CLI.`
Edit:    `Operator-facing reference for the Decloud CLI.`

M2 has shipped a real new flag end-to-end, so the "M1 CLI" framing is
stale. Trivial. **Fold in.**

### Fix C — rewrite `_ai/m1x-backlog.md` item 6 "M2 delivery" paragraph

Currently claims the integration test "Brings up `decloud caddy up`, builds
a tiny test image, deploys with `--mount=<tmpdir>:/data:ro`" — but the
shipped test does none of those (Joel §4.8 revised approach was
driver-direct). Kevlin's suggested rewrite at §10 Fix C is correct: the
test is `internal/integration/mount_test.go` driver-direct, pulls
`nginx:alpine` (post-fix), calls `Driver.Run` directly with `Volumes`,
asserts `docker exec cat /data/marker.txt`. Cleanup via `t.Cleanup` with
idempotent `docker rm -f`. Does NOT exercise the deploy orchestrator
(split to item 10 per Joel decision 8).

Joel's addendum should lock the new wording — including the `nginx:alpine`
update from the integration-test fix, so the backlog text matches the
shipped reality after Rob's commit. **Fold in.**

## 3. Decision and rollout

**EXECUTION pass v2 is required.** Workflow:

1. **Joel**: write `013-joel-tech-plan-addendum-v2.md` locking the four
   fixes (integration-test image swap, Mount.HostPath doc comment,
   usage.md:3 tense slip, m1x-backlog item 6 rewrite). Commit on the
   branch.
2. **Linus**: review Joel's addendum-v2. If approved, proceed.
   (`004-linus-plan-review.md` and `006-linus-plan-review-v2.md` shape;
   probably file `014-linus-plan-review-v3.md`.)
3. **Rob**: apply all four fixes in one commit. Then RUN
   `DECLOUD_INTEGRATION=1 go test -tags integration -v
   ./internal/integration/... 2>&1` on real Docker, capture the output,
   commit the PASS log as a task report
   (`015-rob-impl-fix.md` or similar — bureau-numbered).
4. **Kevlin + Linus**: parallel re-review of the delta. They focus only on
   the four-fix delta, not the entire M2 surface. (Files
   `016-kevlin-review-v2.md`, `017-linus-impl-review-v2.md`.)
5. **PLAN re-entry v3**: Don/Joel/Linus closeout vote on `018-don-closeout-v2.md`.
6. If v3 closes clean: FINALIZATION (Ward → Andy → squash-merge).

## 4. Don's vote

Closeout vote on M2 v1 (this file): **NOT DONE — return to EXECUTION for
the four-fix pass.**

Reasoning:
- Integration-test bug is a real verification-mechanism defect; bundling
  argument requires fixing it before close.
- Kevlin's three doc fixes are trivial; folding them in costs nothing.
- One commit + one verification log + two reviews + one PLAN re-entry is
  cheap insurance. Punting any of it to backlog dilutes the
  "don't-ship-shit" discipline.

I expect Joel to agree (he wrote the bundling argument; he understands the
asymmetry). I expect Linus to agree (he wrote the same recommendation in
§5 of `011-linus-impl-review.md`).

If either disagrees, they should record their objection here and we'll
reconcile before Joel writes the addendum.

## 5. Joel's vote

(To be filled in by Joel.)

## 6. Linus's vote

(To be filled in by Linus.)

## 7. What's deferred (post-M2 backlog work)

Even after EXECUTION v2 closes, these stay on `_ai/m1x-backlog.md`:

- **Item 9** — reloader stderr `%q` quoting revisit (orthogonal to mounts;
  was correctly split out at plan stage).
- **Item 10** — curl-through-Caddy integration test (the
  ingress-verification half of the original item 6; correctly split out
  at plan stage).
- **Item 11** — `Driver.Run` / `Driver.RunWithOptions` consolidation. After
  this closeout, item 11 grows the future-author note that the
  consolidated `RunOptions` should carry `Cmd []string` (Linus
  Observation A).

No new backlog items are created by this closeout. The integration-test
fix is being absorbed into M2's EXECUTION v2, not deferred.

## 8. Files referenced

Reviewed end-to-end for this closeout:

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/010-kevlin-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/011-linus-impl-review.md`
- `/Users/fenster/dev/decloud/internal/integration/mount_test.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/driver.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver.go`
- `/Users/fenster/dev/decloud/internal/registry/types.go`
- `/Users/fenster/dev/decloud/_docs/usage.md` (lines 1-10 for the tense slip)
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md` (lines 1-117 for item 6
  drift verification + items 9/10/11 context)

Spot-checked (not full re-reads — Kevlin and Linus already audited them):

- `/Users/fenster/dev/decloud/internal/registry/mount.go`
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes.go`
