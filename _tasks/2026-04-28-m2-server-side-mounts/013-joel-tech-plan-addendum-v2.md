# 013 — Joel's tech-plan addendum v2 (M2 server-side mounts)

PLAN re-entry v2 after Kevlin (`010-kevlin-review.md`) and Linus
(`011-linus-impl-review.md`) reviewed the shipped code. Don's closeout
(`012-don-closeout.md`) decided EXECUTION v2 is needed: one commit covering
Linus's required integration-test fix plus Kevlin's three optional doc fixes,
followed by a real-Docker run-log verification gate.

This addendum locks the EXACT shape of the four-fix delta so Rob can apply
it mechanically and Linus/Kevlin can re-review against a stable target.

## TL;DR

Four fixes, one commit, plus one verification log committed separately:

1. **Integration-test image swap** — `mountTestImage = "alpine:3.19"` →
   `"nginx:alpine"`. One line in `internal/integration/mount_test.go:23`.
   No other test surface changes.
2. **`Mount.HostPath` doc-comment** — three-line block comment in
   `internal/registry/types.go` above the struct field, promoting the
   named-volume-aliasing convention to first sight.
3. **`_docs/usage.md:3` tense slip** — drop "M1" from the file's intro line.
4. **`_ai/m1x-backlog.md` item 6 "M2 delivery" paragraph** — rewrite to match
   shipped reality (driver-direct test, `nginx:alpine` post-fix, no Caddy,
   no orchestrator). Plus: append a one-line "Future-author note" to item 11
   recording Linus Observation A — consolidated `RunOptions` should grow
   `Cmd []string`.

Then: real-Docker run-log committed as a separate task report. No PASS
log → no closeout. Don's gate, my full agreement.

I vote **NOT DONE pending Rob's v2 commit + run-log; APPROVE the path
forward in this addendum.**

---

## 1. Integration-test fix (Don picked Linus Option A)

### 1.1 Confirming Don's analysis

Don's reasoning in `012-don-closeout.md` §1 is correct on every point I can
verify by reading the codebase:

- **`RunRequest` has no `Cmd` field.** Verified at
  `internal/dockerdrv/driver.go:31-39`. The struct carries
  `Name/Image/Network/Env/Restart/Port/Volumes` and nothing else. There is
  no test-only escape hatch like `Args []string` either. **Confirmed.**
- **`RunOptions` (used by `RunWithOptions`) also has no `Cmd` field.**
  Verified at `internal/dockerdrv/driver.go:77-86`. It carries
  `Image/Name/Network/Restart/Volumes/Ports/Labels`. Same gap. **Confirmed.**
- **`cli_driver.go:Run` ends with `args = append(args, req.Image)`** at line
  65. There is no Cmd suffix appended. `RunWithOptions` is identical at
  line 241 with `args = append(args, opts.Image)`. **Confirmed.** No way to
  inject a Cmd via the Driver surface short of adding a field.
- **`alpine:3.19` default CMD is `/bin/sh`.** Don't have a Docker daemon on
  hand to `docker inspect`, but the official Alpine Dockerfile lineage
  carries `CMD ["/bin/sh"]` and has done so since Alpine first shipped.
  With `docker run -d` (detached, no `-i -t`), `/bin/sh` reads from a
  closed stdin and exits immediately with status 0. **Confirmed by Linus's
  diagnosis at `011-linus-impl-review.md` §5 and Don's closeout §1.**

The shipped argv is `docker run -d --name decloud-mounttest --network
decloud --restart no -v <tmp>:/data:ro --label decloud.service=mounttest
alpine:3.19` (verified by reading `cli_driver.go:Run` plus
`mount_test.go:69-77`). The container exits before `driver.Exec` runs.
The test cannot have been executed against real Docker — Kent and Rob
both reported `go build -tags integration` clean, which is compilation,
not execution. Linus called this out at §5; Don ratified it at §1.

The bundling argument we made at plan stage (Joel §4.8 / Don §8) was
"ship integration test in M2 BECAUSE we want automated real-Docker
verification of the mount feature." A test that exits before `docker
exec` runs answers neither of the two questions the test exists to ask
("does Docker accept this argv shape?" and "does the bind mount actually
surface the file at the target?"). It's a verification-mechanism failure.

### 1.2 Why Option A (`nginx:alpine`) is correct

Linus laid out four options at §5; Don picked A. I agree on the merits:

- **Option A — `nginx:alpine`.** One-line test fix. Production code
  untouched. m1x-item-11 stays clean. Cost: ~22MB image pull on CI
  vs ~7MB alpine. On a CI runner that already pulls multi-GB Go
  toolchains, that's noise.
- **Option B — add `Cmd []string` to `RunRequest`.** Matches my plan §8
  wording exactly (`/bin/sh -c 'cat ... ; sleep 60'`). But: it bakes a
  test-only need into the production driver contract, and m1x-item-11
  (Driver.Run / RunWithOptions consolidation) is the right vehicle for
  designing `Cmd` properly across both run paths. Pre-empting that work
  here risks doing it wrong twice. Reject for M2.
- **Option C — test bypasses Driver.Run, shells out raw.** Anti-pattern.
  The integration test exists to exercise `Driver.Run`'s argv
  construction on real Docker. Bypassing the driver answers a different
  question. Reject.
- **Option D — revert the integration test, file as new m1x-item-12.**
  Negates the bundling argument. We shipped the integration test in M2
  precisely to verify M2 against real Docker; reverting means we shipped
  M2 with no real-Docker mount verification at all. Reject.

**Choose A.** Smallest fix, zero production touch, actually verifies
M2 on real Docker, doesn't pre-empt m1x-item-11.

### 1.3 `nginx:alpine`'s default CMD — confirmed long-running

Verified via the upstream nginx official image documentation: the default
CMD is `nginx -g daemon off;`. The `daemon off;` directive is mandatory
specifically so nginx stays in the foreground as PID 1 and Docker can
track the process. With `docker run -d nginx:alpine`, the container runs
the nginx master process in the foreground, indefinitely, until
explicitly stopped. **Exactly the idle-but-alive container shape we
need.**

This is documented at the nginx Docker Hub page: "If you add a custom CMD
in the Dockerfile, be sure to include `-g daemon off;` in the CMD in
order for nginx to stay in the foreground, so that Docker can track the
process properly (otherwise your container will stop immediately after
starting)." The default image already has this directive baked in.

### 1.4 The exact one-line diff

File: `/Users/fenster/dev/decloud/internal/integration/mount_test.go`.

Line 23, current:

```go
	mountTestImage       = "alpine:3.19"
```

Line 23, after fix:

```go
	mountTestImage       = "nginx:alpine"
```

No other line in the file changes. The test's setup (write marker file
on host, `t.Cleanup`, defensive pre-Run `removeContainerIdempotent`,
`ImagePull`, `Run` with `Volumes`, `Exec` with `cat /data/marker.txt`,
`assert.Equal` on `mountTestMarkerBytes`) all works identically against
nginx:alpine. The container is alive when `Exec` runs; `cat
/data/marker.txt` reads the marker file from the bind mount the same
way; nginx ignores `/data` (it serves from `/usr/share/nginx/html` by
default). No interference.

### 1.5 Gotcha checks

Don asked me to verify two specific gotchas. I did:

**Gotcha 1 — Cleanup discipline with nginx's signal handling.** nginx
(any flavour, including Alpine) handles SIGTERM by initiating a graceful
shutdown: stop accepting new connections, finish in-flight requests,
exit. By default the timeout is bounded (worker process shutdown timeout,
30s default). `docker rm -f` sends SIGKILL after a 10-second SIGTERM
grace period (Docker default; configurable via `--time` to `docker stop`,
but `docker rm -f` uses 10s). Two questions:

1. **Does the test's `removeContainerIdempotent` complete in reasonable
   time?** It uses `exec.CommandContext(ctx, "docker", "rm", "-f", name)`
   with a 30-second cleanup ctx (`mount_test.go:36-37`). Worst case
   nginx ignores SIGTERM for 10 seconds and gets SIGKILLed; well within
   the 30-second budget. **No issue.**

2. **Does nginx's signal handling delay test teardown enough to matter?**
   The test has no in-flight HTTP traffic, so SIGTERM exits cleanly
   within ~1 second. The 10-second SIGKILL grace period is the worst
   case and almost never hit. **No issue.**

The test cleanup discipline (cleanup ctx derived from
`context.Background()` with 30s timeout, registered before `Run` call)
is correct per `_ai/cleanup-context-discipline.md`. Kevlin verified this
in §7 of the review. nginx:alpine doesn't change anything here.

**Gotcha 2 — nginx starts an HTTP server on `:80`, requires Docker
network.** True: nginx listens on `:80` inside the container by default.
But:

- The test sets `Network: "decloud"` (`mount_test.go:72`), which is the
  same network used by all decloud services.
- The test calls `ensureNetwork(t, driver)` (`mount_test.go:59`) which
  does `driver.NetworkEnsure(ctx, "decloud")` first.
- The test does NOT publish the port to the host (no `Port` field in the
  RunRequest, and `cli_driver.go:Run` only emits `-p` when `req.Port > 0`
  — verified at lines 53-56). So nginx listens on `:80` inside the
  container's network namespace only. No host-port collision. No risk
  of "port 80 already in use" from a host running another HTTP server.
- nginx serving its default index.html on `:80` is irrelevant to the
  test, which uses `docker exec` (which doesn't go through the network)
  to read `/data/marker.txt`.

**No issue.** The existing test setup (network ensure, no port publish,
exec-based assertion) is sufficient for nginx:alpine.

**Bonus gotcha — image pull time.** `nginx:alpine` is ~22MB compressed.
`ImagePull` is given a 2-minute ctx (`mount_test.go:63-64`). Even on a
slow CI runner, 22MB pulls in well under 30 seconds. **No issue.**

### 1.6 Why this isn't in `_ai/cleanup-context-discipline.md`'s scope

The cleanup-context discipline is about ensuring teardown runs on a
non-cancellable ctx — that's preserved here unchanged. The image swap
doesn't touch the cleanup chain at all; it changes which container is
running long enough for `Exec` to succeed. Kevlin explicitly verified
the cleanup discipline is held at §7 of the review. The fix doesn't
disturb that.

---

## 2. Kevlin Fix A — `Mount.HostPath` doc-comment

### 2.1 Where

File: `/Users/fenster/dev/decloud/internal/registry/types.go`.

Current state at lines 59-63:

```go
type Mount struct {
	HostPath      string `toml:"host_path"`
	ContainerPath string `toml:"container_path"`
	ReadOnly      bool   `toml:"read_only"`
}
```

The struct has no doc-comment on any field. The named-volume-aliasing
convention (HostPath doubles as a Docker named-volume name when it
doesn't start with `/`) is documented on `Mount.IsNamed()` in
`mount.go`, but a casual reader of `types.go` doesn't see it.

### 2.2 The exact replacement (per Kevlin §10 Fix A)

```go
type Mount struct {
	// HostPath is the mount source. For bind mounts it is an absolute host
	// path starting with "/"; for named volumes it is the volume name. The
	// TOML key is historically named host_path. Use Mount.IsNamed() to
	// distinguish at runtime.
	HostPath      string `toml:"host_path"`
	ContainerPath string `toml:"container_path"`
	ReadOnly      bool   `toml:"read_only"`
}
```

Three lines of comment above `HostPath`, no other changes. The `toml:"host_path"`
tag is preserved verbatim. Field types unchanged. No behaviour delta.

### 2.3 Why the wording is right

- "mount source" — pairs with the existing language in `mount.go`'s
  `ValidateMount` doc-comment ("source"); operator-readable.
- "absolute host path starting with `/`" — names the discriminator that
  `IsNamed` uses (`!strings.HasPrefix(m.HostPath, "/")`).
- "for named volumes it is the volume name" — explicitly calls out the
  aliasing.
- "historically named host_path" — explains why we didn't rename to
  `Source` despite the aliasing being a permanent quirk (Joel §1
  Decision 3 Option B: derive instead of rename, to preserve schema
  stability).
- "Use Mount.IsNamed() to distinguish at runtime" — points readers at
  the helper that already exists.

No tests change. The comment is operator/developer-facing. `gofmt` will
preserve it verbatim. **Behaviour-preserving.**

---

## 3. Kevlin Fix B — `_docs/usage.md` line 3 tense slip

### 3.1 Where

File: `/Users/fenster/dev/decloud/_docs/usage.md`.

Current line 3 (verbatim from the file):

```
Operator-facing reference for the Decloud M1 CLI. For host setup, see [`install.md`](./install.md).
```

### 3.2 The exact replacement (per Kevlin §10 Fix B)

```
Operator-facing reference for the Decloud CLI. For host setup, see [`install.md`](./install.md).
```

The single change is dropping "M1 " from "Decloud M1 CLI" → "Decloud
CLI". Everything else on the line (including the `install.md` link) is
preserved.

### 3.3 Why now

M2 has shipped a real new flag end-to-end (not a refactor or a doc-only
update). The "M1 CLI" framing is a tense slip — the document now
documents M1 + M2 features, and the labelling on line 3 is stale.
Kevlin flagged it at §10 Fix B; Don ratified at §2 Fix B. **Trivial
prose fix; no tests change.**

---

## 4. Kevlin Fix C — `_ai/m1x-backlog.md` item 6 rewrite + Linus
   Observation A folded into item 11

### 4.1 Item 6 "M2 delivery" paragraph — current text

File: `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`, lines 55-61.

Current item 6 body (the "M2 delivery" paragraph at line 59):

```
**M2 delivery:** `internal/integration/mount_test.go` with `//go:build integration` tag, gated on `DECLOUD_INTEGRATION=1`. Brings up `decloud caddy up`, builds a tiny test image, deploys with `--mount=<tmpdir>:/data:ro`, asserts `docker exec` reads the marker file, and tears down through `t.Cleanup` with idempotent `docker rm -f`. Mount-only — no curl-through-Caddy step (split per Joel decision 8 of the M2 tech plan).
```

This is wrong against the shipped reality. The actual test (`internal/integration/mount_test.go`):
- Does NOT bring up `decloud caddy up`.
- Does NOT build a test image — pulls a public image directly.
- Does NOT call the deploy orchestrator — calls `driver.Run` directly.
- DOES use `docker exec` to read the marker file. ✓
- DOES use `t.Cleanup` with idempotent `docker rm -f`. ✓

Kevlin called the drift at §4 + §10 Fix C. Don ratified at §2 Fix C.

### 4.2 The exact replacement (per Kevlin §10 Fix C, with `nginx:alpine` update)

Replace the `**M2 delivery:**` paragraph (line 59, single paragraph)
with:

```
**M2 delivery:** `internal/integration/mount_test.go` with `//go:build integration` tag, gated on `DECLOUD_INTEGRATION=1`. Pulls `nginx:alpine` via the real `dockerdrv.CLIDriver`, calls `driver.Run` directly with a `Volumes: [...]` shape carrying one bind ro mount, and asserts `docker exec cat /data/marker.txt` returns the marker bytes. Cleanup via `t.Cleanup` with idempotent `docker rm -f decloud-mounttest`. Does NOT exercise the deploy orchestrator (build, readiness, Caddyfile generation, reload) — those are split to item 10 (curl-through-Caddy ingress test) per Joel decision 8 of the M2 tech plan. The `nginx:alpine` choice (rather than alpine) is deliberate: nginx idles in the foreground via `nginx -g daemon off;`, so the container stays alive long enough for `docker exec`; alpine's default `/bin/sh` CMD exits under `docker run -d` (Linus's catch in `011-linus-impl-review.md` §5, fix in EXECUTION v2).
```

The two important deltas vs Kevlin's suggested wording:

1. Image is `nginx:alpine` (post-fix), not `alpine:3.19`.
2. One-sentence note explaining why `nginx:alpine` was chosen (idle
   foreground process), with a pointer to the review-and-fix audit
   trail. This matters for future-Don: when item 6 is finally deleted
   per the maintenance note, this paragraph is the only on-tree record
   that the alpine→nginx swap was deliberate.

The strikethrough heading at line 55 (`## 6. ~~Docker-compose-based
smoke integration test for M1 deploy + Caddy ingress~~`) and the status
sentence at line 57 (`**Status:** PARTIALLY DONE at M2. ...`) and the
originator paragraph at line 61 (`**Originator:** Joel §8.5 of ...`)
are all unchanged.

**No tests change.** This is the m1x-backlog future-Don note, not an
operator-facing surface.

### 4.3 Linus Observation A folded into item 11

File: `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`, item 11 currently
ends at line 111 with the "Originator" line:

```
**Originator:** Don §5 Option α of `_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`; Joel Decision 4 of `003-joel-tech-plan.md`.
```

After this Originator line, append a new paragraph (one blank line then
the new paragraph, before the `---` separator at line 113):

```
**Future-author note (Linus Observation A, recorded at M2 closeout):** When picking up this consolidation, the unified `RunOptions` should grow `Cmd []string` so future integration tests (or one-shot job/migration runners at M5+) don't need to pick a specific image with an idle CMD. The M2 integration test exposed this gap: `alpine:3.19` exits under `docker run -d` because its default CMD is `/bin/sh` reading closed stdin; M2 worked around this by switching the test to `nginx:alpine` (which idles in the foreground). Adding `Cmd []string` to the consolidated `RunOptions` removes that constraint and aligns the run path with `ExecOptions.Cmd`. Source: `_tasks/2026-04-28-m2-server-side-mounts/011-linus-impl-review.md` §"Observation A".
```

The paragraph names the gap, names the workaround, names where to look
for full context, and explicitly says "consolidated `RunOptions`" so
the future-author lands the field in the right struct (not on the
deprecated `RunRequest`).

**No tests change.** Item 11 is future-author work; the note just
records the field that work should add.

---

## 5. Verification gate — what Rob must do

Per Don §1 final paragraph and Linus §5 last paragraph, the integration
test must be **actually run** against real Docker on the maintainer's
box before M2 closeout. Compile-clean is not run-clean. The PASS log
goes in the task directory.

### 5.1 Steps

1. **Apply the four fixes in one commit.** Branch:
   `feat/m2-server-side-mounts`. Commit message shape: `fix(m2):
   integration test image swap + Kevlin's three doc fixes`. Co-Author
   line per CLAUDE.md.
2. **Run the integration test on a real Linux host with Docker
   installed:**

   ```
   DECLOUD_INTEGRATION=1 go test -tags integration -v ./internal/integration/... 2>&1 | tee _tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log.txt
   ```

   The command must run from the repo root. The host needs:
   - Docker daemon running and accessible to the current user (`docker
     ps` succeeds without sudo).
   - Network reachability to Docker Hub for the `nginx:alpine` pull.
   - The `decloud` Docker network either pre-existing or createable
     (the test calls `NetworkEnsure` which creates it idempotently).

3. **Output file:**
   `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log.txt`.

4. **Verification:** the log MUST contain a PASS line for
   `TestIntegration_MountBindRoundTrip` (Kent named it that — verified
   at `mount_test.go:53`). Look for:

   ```
   --- PASS: TestIntegration_MountBindRoundTrip (...s)
   PASS
   ok  	github.com/alexander-fenster/decloud/internal/integration	...
   ```

   And specifically NOT:

   ```
   --- FAIL: TestIntegration_MountBindRoundTrip ...
   ```

5. **If it fails: do NOT commit the log.** Diagnose, fix, re-run. The
   log committed must be a PASS log. If the failure is `nginx:alpine`
   pull error (network), that's not a closeout blocker — re-run on a
   host with network. If the failure is `docker exec cat` returning
   empty/wrong bytes, that's a real bug — report and revisit the plan.
6. **Commit the log as a separate commit on the branch.** Commit
   message: `chore(m2): integration test PASS log against real Docker`.
   Co-Author line.

### 5.2 What the log proves

Three things, all of which are M2 closeout requirements:

1. **`docker run -v` argv shape works.** The driver's `Run` method
   constructs the right argv to make Docker accept the bind mount.
2. **The bind mount actually surfaces the host file at the container
   target.** Marker file written on host is visible inside the
   container at `/data/marker.txt`.
3. **The cleanup discipline holds.** `t.Cleanup` runs even on test
   failure; the container is removed; the test is repeatable.

Without the PASS log, the bundling argument we made at plan stage is
retroactively a lie. WITH the PASS log, M2 ships with verified
real-Docker mount support. Hard gate.

---

## 6. Test-surface deltas

**None.** All four fixes are behaviour-preserving:

- Image-name change: the test file changes by one constant. No new test,
  no removed test, no test assertion changes. The same `Exec` call
  works because `nginx:alpine` runs nginx as PID 1 and `docker exec`
  doesn't care what's running — it just spawns `cat /data/marker.txt`
  in a new process namespace inside the existing container. The bind
  mount at `/data` is the same shape regardless of the image. The
  marker bytes assertion (`assert.Equal(t, mountTestMarkerBytes,
  strings.TrimSpace(stdout.String()))`) doesn't depend on the image.
- `Mount.HostPath` doc-comment: comment text. No behaviour. No test.
- `_docs/usage.md` line 3: prose. No behaviour. No test.
- `_ai/m1x-backlog.md` items 6 + 11: future-Don notes. No production
  surface, no test.

**The existing `TestIntegration_MountBindRoundTrip` is the test that
proves the fix works.** It moves from "compiles but exits before exec"
to "passes against real Docker." That's not a new assertion; that's
the assertion that was supposed to work all along finally working.

If anyone proposes a new test as part of v2, push back: the existing
test is the load-bearing one, and adding more would dilute. The
verification gate is "the existing test passes against real Docker,"
not "we wrote more tests."

---

## 7. Workflow (re-confirming Don §3)

Don's rollout in `012-don-closeout.md` §3 is:

1. **Joel addendum-v2** (this file) — locks the four-fix delta. Commit
   with `docs(m2): Joel's addendum v2 + closeout vote`.
2. **Linus reviews this addendum.** If approved, proceed. File something
   like `014-linus-plan-review-v3.md`.
3. **Rob applies the four fixes in one commit.** Then runs the
   integration test against real Docker, captures PASS log, commits log
   as separate commit. Reports as `015-rob-impl-fix.md` (or the next
   bureau number).
4. **Kevlin and Linus parallel re-review of the delta.** Files
   `016-kevlin-review-v2.md` and `017-linus-impl-review-v2.md`
   (bureau-numbered when written). Scope: only the four-fix delta and
   the run-log. NOT a full re-audit of the M2 surface — that's already
   done and approved-with-fixes.
5. **PLAN re-entry v3 — Don/Joel/Linus closeout vote.** File
   `018-don-closeout-v2.md`. If all three vote DONE, proceed to
   FINALIZATION (Ward → Andy → squash-merge into main).

**I confirm this workflow.** No additional steps needed.

The numbering in this file (`013` for me, `014` for Linus, `015` for
Rob's fix, `016/017` for the parallel reviews, `018` for Don's v2
closeout) assumes Bureau MCP assigns them in the same order. If Bureau
hands out different numbers because of parallel writes, the file names
shift but the workflow shape doesn't. The numbering above is
descriptive, not prescriptive.

---

## 8. Files to be touched in EXECUTION v2

Production code (one file):
- `/Users/fenster/dev/decloud/internal/integration/mount_test.go` — line
  23 image-constant change.

Doc files (three files):
- `/Users/fenster/dev/decloud/internal/registry/types.go` — three-line
  doc-comment above `Mount.HostPath` (lines 59-63 region).
- `/Users/fenster/dev/decloud/_docs/usage.md` — line 3 tense slip.
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md` — item 6 "M2
  delivery" paragraph rewrite, item 11 "Future-author note" appended.

New file (one):
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log.txt`
  — committed as a separate commit AFTER the four-fix commit.

That's four files in commit one, one new file in commit two. Linus and
Kevlin re-review the delta of those four files plus eyeball the
`integration-test-run-log.txt` for a PASS line.

---

## 9. Joel's closeout vote

**Joel votes: NOT DONE pending Rob's v2 commit + run-log; APPROVE the
path forward in this addendum.**

Reasoning, in the order they matter:

1. **The integration test bug is a real verification-mechanism defect.**
   The bundling argument I made at plan stage (Joel §4.8) was that
   shipping the integration test in M2 was the right call BECAUSE it
   would automatically verify the mount feature against real Docker.
   The shipped test cannot do that — `alpine:3.19` exits before
   `docker exec` runs. If we close out without fixing it, the bundling
   argument retroactively becomes a lie. Don wrote this almost
   verbatim in §1 of his closeout; I agree fully. **Cannot close.**
2. **The fix is one line.** Image swap. Plus three trivial doc fixes
   bundled in. One commit, one verification log. The cost of EXECUTION
   v2 (this addendum + Linus review + Rob commit + two parallel reviews
   + Don/Joel/Linus closeout v2) is small — maybe an hour of agentic
   wallclock, plus the verification gate which is wallclock-bounded by
   "Rob runs `go test` against real Docker." **Asymmetry favours
   fixing now.**
3. **The "compile-clean ≠ run-clean" discipline matters more than the
   one-test slip.** This is the discipline Don instituted explicitly to
   prevent another Netscape 4.0 ship-without-running fiasco. If we let
   it slide here ("we shipped a real feature, the unit tests cover it,
   the integration test is just decoration"), we teach the team that
   integration-test breakage is OK to defer when the feature itself
   works. That's exactly the wrong lesson. The integration test is
   not decoration; it's the verification mechanism for the M2 feature
   against real Docker. **Hold the line.**
4. **The path forward is clean.** The four-fix delta is small,
   well-specified (this addendum), behaviour-preserving (no test
   surface changes), and gateable (the run-log is binary
   PASS-or-no-closeout). Linus and Kevlin re-review against a stable
   target. PLAN re-entry v3 is a quick closeout vote, not another
   round of architectural debate. **Approve the path.**

The conditional vote — "DONE *if* Rob ships the v2 commit cleanly and
the run-log shows PASS" — is implicit. I'll cast my unconditional vote
at PLAN re-entry v3 once the run-log is in the task directory.

If Linus's review of this addendum surfaces something I missed (he
caught the alpine-no-Cmd bug; he might catch something else), I'll
revisit. Otherwise this addendum is the locking document for EXECUTION
v2.

---

## 10. Sources

The nginx:alpine default-CMD verification draws on:

- [nginx Official Docker Image Documentation](https://hub.docker.com/_/nginx)
- [How to Use the NGINX Docker Official Image (Docker blog)](https://www.docker.com/blog/how-to-use-the-official-nginx-docker-image/)

Both confirm the default CMD is `nginx -g daemon off;`, which keeps
the master process foreground as PID 1 and the container alive
indefinitely under `docker run -d`.

---

## 11. Files referenced

Files I read end-to-end for this addendum:

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/010-kevlin-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/011-linus-impl-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/012-don-closeout.md`
- `/Users/fenster/dev/decloud/internal/integration/mount_test.go`
- `/Users/fenster/dev/decloud/internal/registry/types.go`
- `/Users/fenster/dev/decloud/_docs/usage.md` (line 3 region)
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`

Spot-checked (already audited by Linus and Kevlin in the v1 reviews):

- `/Users/fenster/dev/decloud/internal/dockerdrv/driver.go` (RunRequest /
  RunOptions field absence verified)
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver.go` (Run /
  RunWithOptions argv shape verified — no Cmd suffix)
