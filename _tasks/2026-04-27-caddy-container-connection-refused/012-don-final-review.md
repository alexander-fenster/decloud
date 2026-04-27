# 012 — Don's Final Review (post-execution PLAN-loop)

Author: Don Melton (planning agent)
Date: 2026-04-27
Status: PLAN loop after EXECUTION. Verdict at the end. **ANOTHER EXECUTION CYCLE NEEDED** — small, surgical, scoped.

## 0. How to read this

I read everything: 001 user request, 005 v2 plan, 006 v2 tech plan, 008
Kent, 009 Rob, 010 Raymond, 011 Kevlin, 011 Linus. I traced the bug-fix
code path end-to-end against the actual files on disk
(`internal/caddy/manager.go`, `reloader.go`, `internal/cli/caddy_up.go`,
`caddy_down.go`, `internal/deploy/service.go:300-325`,
`internal/dockerdrv/cli_driver.go:200-285`,
`_docs/install.md:160-200`). I did not take agent reports on faith — I
verified each claim against the source. Kevlin and Linus were correct on
every observation. The implementation matches v2 plan with high fidelity.

What follows is the brutally-honest assessment the team asked for.

---

## 1. Does the implementation actually fix the user's bug?

**Yes.** Walked the code path:

1. Operator runs `decloud caddy up`
   → `internal/cli/caddy_up.go:16` → `caddyManagerFactory(...)` →
   `buildProductionCaddyManager` (`internal/cli/deploy_service.go:155`)
   constructs `cliManager` with the production `dockerdrv.Driver`.
2. `cliManager.Up` (`internal/caddy/manager.go:64`):
   `NetworkEnsure("decloud")` → `WriteStubIfMissing` →
   `Inspect("decloud-caddy")` → on `absent` → `ImagePull("caddy:2")` →
   `RunWithOptions(runOpts())`. `runOpts()` (line 120-141) hardcodes
   `Network: NetworkName` (= `"decloud"`), so `decloud-caddy` joins the
   bridge. Six dual-stack `PortMap` entries on lines 127-134.
3. `RunWithOptions` (`internal/dockerdrv/cli_driver.go:203`) emits exactly
   the argv Linus walked in `011-linus-impl-review.md` §1, locked by
   `TestCLIDriver_RunWithOptionsCaddyShape`.
4. Operator runs `decloud deploy service ...`
   → `serviceDeployer.Deploy` (`internal/deploy/service.go:127`) joins the
   service container to `decloud` via `Driver.Run` with `--network decloud`.
5. `regenerateAndReload` (line 302) writes Caddyfile, then
   `Reloader.Validate` (`internal/caddy/reloader.go:46`) runs
   `docker exec decloud-caddy caddy validate --config
   /etc/caddy/Caddyfile.tmp` via `Driver.Exec`. The validator runs INSIDE
   the Caddy container — so name resolution goes through the container's
   resolver, which is Docker's embedded DNS at `127.0.0.11`. That DNS
   serves only A records on the IPv4-only `decloud` bridge.
6. After atomic rename, `Reloader.Reload` runs `caddy reload`. Live Caddy
   re-resolves `decloud-durak-live` → `172.18.0.x`. Connection succeeds.

**The IPv6 fallthrough that triggered the original bug is no longer
reachable.** The only path Caddy's resolver can take is the embedded DNS,
which cannot return the host's GUA. Bug fixed at the architectural layer.

## 2. Acceptance criteria from v2 §11 — explicit hit/miss

| # | Criterion | Status |
|---|---|---|
| 1 | `caddy up` brings up `decloud-caddy` with dual-stack 6-port shape, bind mount, two named volumes; idempotent | **HIT** (manager.go:120-141; tests lock the shape) |
| 2 | `caddy down` stops/removes, retains volumes; idempotent | **HIT** (manager.go:101-110) |
| 3 | `caddy reload` `docker exec`s into `decloud-caddy` | **HIT** (reloader.go:54-73) |
| 4 | No flags, no Viper, no TOML; `caddy:2` hardcoded | **HIT** (manager.go:20; caddy_up.go:14) |
| 5 | `Manager.Up` does NOT poll admin API and does NOT roll back | **HIT** (manager.go Up; locked by `TestManager_UpRunFailsWithoutRollback` Times(0)) |
| 6 | `cmdFactory` test seam deleted | **HIT** (reloader.go has only `driver` and `hostCaddyDir` fields) |
| 7 | Deploy-failure error text gives one-command recovery | **HIT** (service.go:316,322 — verbatim Joel §4.9) |
| 8 | End-to-end deploy succeeds on user's host | **PENDING** (operator must run §7 verification) |
| 9 | Caddy logs show no `dial tcp [2a03:...]` errors | **PENDING** (operator) |
| 10 | `docker network inspect decloud` shows both containers | **PENDING** (operator) |
| 11 | `ss -tlnp` shows both `0.0.0.0` AND `::` listeners | **PENDING** (operator) |
| 12 | `_docs/install.md` rewritten (§3, §5, §61-62 deleted, migration recipe leads with volume-copy, mask/remove named) | **MOSTLY HIT** — structure correct, migration recipe correct; **two doc-fab error renderings (Kevlin's nits) violate the no-hallucination contract** — see §3 below |
| 13 | `_docs/usage.md` §1, §6, §7 updated | **HIT** (verified by Kevlin's audit) |
| 14 | `_ai/decisions/caddy-runs-in-container.md` exists | **HIT** (Kevlin and Linus both verified) |
| 15 | `_ai/m1x-backlog.md` integration-test backlog item appended | **MISS** — Raymond explicitly punted this to Ward (010-raymond-docs.md §"Files NOT touched"). Per the v2 plan it was Raymond's. Either we fix attribution or Ward picks it up in step 4; either way it is not yet on disk. |
| 16 | Driver interface gains exactly three methods | **HIT** (driver.go: `ImagePull`, `RunWithOptions`, `Exec`) |
| 17 | `exit_codes.go` gains zero constants; `ErrCaddyUp`/`Down` map to 40 | **HIT** (single new `case` line) |
| 18 | `go test ./...` green; vet/gofmt clean; `go generate` no diff | **HIT** (verified by Kevlin and Linus) |
| 19 | Operator runs §7 manual verification on the actual host and reports green | **PENDING** (the user is the gate) |

Two MISS-class items worth attention: criterion #12 (the Kevlin doc-fab
nits) and criterion #15 (the m1x-backlog item Raymond did not write).
Criteria 8-11 and 19 are operator-side — they're the proof the bug is
gone in production, and they're the user's job, not ours.

## 3. Kevlin's two doc-fab nits — DECISION

The two error renderings at `_docs/install.md:173` and `:189` show
operator-facing error text the implementation does NOT emit. The actual
wrapped error from `Manager.Up` on line 95 of manager.go is:

```
caddy: up failed: docker run: docker run: <inner err>; stderr="...address already in use..."
```

The doc shows:

```
caddy up: ports 80/443 already in use            (line 173 — fabricated)
caddy up: docker run: listen tcp [::]:80: ...    (line 189 — fabricated; missing `caddy: up failed:` prefix)
```

These are exactly the kind of doc hallucinations our hallucination-check
exists to catch. Kevlin caught them. We don't get to ship them.

**The choice the request asked me to make:**

- **Option A (fix the docs):** rewrite the displayed examples to show the
  actual wrapped string operators will see, and tell them what substring
  to look for. Cheap, accurate, ships now.
- **Option B (fix the implementation):** add the substring-detection
  branches Joel §1.5 specced (`address already in use` /
  `port is already allocated` → "ports 80/443 already in use" prefix).
  Better operator UX. Joel actually specified it; Rob skipped it.

**My decision: Option B for the ports case, Option A for the IPv6 case.**

Reasons:

1. Joel §1.5 row 1 explicitly specced the ports substring-detection. It
   was in the plan; it didn't get implemented. That's a "shipping
   almost-done" we don't accept. Implementing it is ~10 lines in
   `manager.go::Up` plus one test in `manager_test.go`. The cost is
   trivial and the operator UX is meaningfully better — the operator
   stuck behind a leftover host-Caddy gets a clean message pointing at
   the recovery commands instead of a stderr-stew.

2. Joel §1.5 row 4 (the IPv6 line) explicitly says "raw stderr passes
   through" — there is NO custom-text branch to implement for IPv6. So
   the doc example at line 189 is purely a presentation lie. The fix is
   to reword the doc to say something honest like:

   > If `decloud caddy up` fails with stderr containing
   > `listen tcp [::]:80: socket: address family not supported by
   > protocol`, the kernel has IPv6 disabled.

   That's accurate to what the operator actually sees.

3. Doing both fixes in the same execution cycle costs us one Kent +
   Rob + Raymond pass. Cheap.

**This is the "fix obvious issues while the code is fresh" rule from my
playbook.** We're 2-3 hours of work from sealing this off cleanly. Don't
backlog something we can finish today.

## 4. Linus's two nits — DECISION

### 4a. `Long` help text on `caddy up`/`caddy down` (Linus 9.1)

Joel §1.6 specified ~4-paragraph `Long` text for `caddy up --help`
(dual-stack note, image, volume names) and a 2-line `Long` for
`caddy down --help` (volume-retention warning, the
`docker volume rm` instruction). Rob shipped `Short:` only on both.
That's a spec-code mismatch on text that has real operator value — the
volume-retention warning is the kind of thing that prevents 3 AM
"oh-shit-I-just-deleted-my-ACME-state" tickets.

**Verified:**

- `internal/cli/caddy_up.go:13` — `Short:` only.
- `internal/cli/caddy_down.go:13` — `Short:` only.

**My decision: FIX in this cycle.** Linus recommended Option A (paste
the spec verbatim). It's ~10 lines across two files, no test changes
needed (Cobra renders `Long` independently). The plan was specific;
"almost done" is not done. Same logic as §3 above: cheap to finish, real
operator value, no scope expansion.

### 4b. Reloader stderr forwarding via `%q` quoting (Linus 9.2)

`reloader.go:72` wraps with `fmt.Errorf("caddy %s: %w; stderr=%q", ...)`.
For multi-line Caddyfile validation errors this mangles newlines into
`\n` and the operator gets a single jumbled line instead of a clean
validator report.

**My decision: DEFER to M2.** Linus himself recommended Option C (defer).
The current behaviour is no worse than M1.0 (host `caddy validate` had
the same property). The bug we set out to fix is fixed. The plan didn't
ask for diagnosability improvement here. Ward should capture this as
backlog material under "improve diagnosability of caddy validate
failures." M2's logging-experience pass picks it up.

## 5. Anything else I noticed

### 5.1 `_ai/m1x-backlog.md` integration-test entry — MISS

Per v2 §11 criterion #15 (and Joel §8.5), the integration-test backlog
item was Raymond's deliverable. Raymond's report (010-raymond-docs.md
§"Files NOT touched") explicitly says: *"`_ai/m1x-backlog.md` — the
integration-test backlog entry the plan mentioned (Don §10 criterion
#15) appears to be Ward's deliverable in step 4. I did not add it."*

Raymond was wrong about the attribution but right that the file isn't
written. Either:

- We fix it now (Raymond appends the line in this cycle).
- We let Ward pick it up in step 4 (knowledge capture) on the explicit
  understanding that the v2 acceptance criterion is only fully satisfied
  after Ward's pass.

**My decision: FIX in this cycle.** The line is two sentences; Raymond
adds it as part of the same docs pass that handles §3 and §4a. We don't
leave acceptance criteria to be picked up by a different agent in a
later phase if we can close them out cleanly here. Ward focuses on
synthesised learnings, not bookkeeping.

### 5.2 No further surprises

I checked everything Kevlin and Linus checked plus a few items they
implicitly trusted:

- **No new `"decloud"` literal sneaked in.** New code (`manager.go`,
  `reloader.go`'s caller in `deploy_service.go`) uses
  `caddy.NetworkName`. The four pre-existing literals
  (`internal/deploy/service.go:131,190,289,238`,
  `internal/dockerdrv/cli_driver.go:170`) remain — Joel §5 explicitly
  scoped that cleanup as M1.x backlog. Not in scope for this task.
- **`go generate` produces no diff.** Mocks committed in sync (Kevlin
  and Linus both verified).
- **The wrap-text duplication on service.go:316,322** is below
  rule-of-three and is fine; Joel §4.9 documented the choice. Don't
  refactor.
- **`translatePath` runs through `filepath.ToSlash`** — Rob's
  Linux/macOS-no-op, Windows-correctness extension. Cheap insurance.
  Approved.

Nothing else to flag.

---

## 6. Verdict

**ANOTHER EXECUTION CYCLE NEEDED** — small, surgical, scoped to four
items.

### Changes for Joel to plan and Kent/Rob/Raymond to execute

1. **Implement `address already in use` / `port is already allocated`
   substring detection in `Manager.Up`** (Joel v2 §1.5 row 1 — already
   specced; never implemented).
   - Owner: Rob (impl), Kent (test for the new branch).
   - Files: `internal/caddy/manager.go::Up` (around line 95 where
     `RunWithOptions` error is wrapped), `internal/caddy/manager_test.go`.
   - Behaviour: when `Driver.RunWithOptions` returns an error whose
     stderr (or wrapped chain) contains either substring, return
     `fmt.Errorf("%w: ports 80/443 already in use; if you ran the M1.0
     install, run 'systemctl disable --now caddy && systemctl mask
     caddy' or 'apt-get remove -y caddy' to make the change persistent",
     ErrCaddyUp)`. Substring miss → fall through to today's generic
     wrap. Test: `TestManager_UpPortsBoundActionableError`.
   - Joel decides whether the substring lives in `manager.go` or in a
     small helper next to `isNotRunningStderr` in `reloader.go` (the
     symmetry argument is mild, the helper is one-shot for now).

2. **Fix `_docs/install.md:189` (IPv6 listener doc fab).** No
   implementation change — Joel §1.5 row 4 is "raw stderr passes
   through". Reword the displayed example to be honest about what the
   operator sees.
   - Owner: Raymond.
   - File: `_docs/install.md` lines 186-192.
   - Reword to: example stderr line `listen tcp [::]:80: socket:
     address family not supported by protocol` shown as "stderr will
     contain" rather than "the error will be" — make it clear it's a
     substring of the wrapped error, not the literal output.

3. **Fix `_docs/install.md:173` displayed text to match the new
   substring-detection output** (after change #1).
   - Owner: Raymond.
   - File: `_docs/install.md` lines 170-184.
   - The displayed example becomes accurate once #1 is implemented; the
     prefix in the doc will match the wrapped error text. Verify the
     prefix matches Joel's specced wording (`caddy up: ports 80/443
     already in use; ...`) byte-for-byte after #1 lands.

4. **Add `Long` help text to `caddy up` and `caddy down`** (Joel §1.6 —
   already specced; never implemented).
   - Owner: Rob (impl). No new tests (Cobra renders `Long`
     independently of `Short`; the existing `TestCaddyUp_NoFlags` /
     `TestCaddyDown_NoFlags` regression guards still pass).
   - Files: `internal/cli/caddy_up.go:13`, `internal/cli/caddy_down.go:13`.
   - Paste the text from Joel v2 §1.6 verbatim. The volume-retention
     warning on `caddy down` is the operator-value bit; do not soften it.

5. **Append the integration-test backlog item to `_ai/m1x-backlog.md`**
   (v2 §11 criterion #15 — Joel §8.5).
   - Owner: Raymond.
   - File: `_ai/m1x-backlog.md`.
   - One bullet, citing this task and `_ai/decisions/m1-test-strategy.md`,
     scoped to "real-Docker integration test for first happy-path
     deploy" per Joel §8.5.

### Items NOT in this cycle (deferred to Ward / M2)

- Reloader stderr `%q` quoting (Linus 9.2). Ward captures as backlog;
  M2 picks it up.
- Existing `"decloud"` literal cleanup (Joel §5). M1.x backlog;
  intentional.
- Wrap-text duplication on service.go:316,322. Below rule-of-three;
  intentional.
- Stdout shape inconsistency between cold-start (`caddy up:` prefix) and
  warm-path (`caddy already running` etc.). Cosmetic; Linus held it.

### Why not FULLY DONE today

Three of the five items above (#1, #4, #5) are explicit acceptance
criteria the v2 plan defined and the execution didn't satisfy. The two
doc-fab items (#2, #3) are direct violations of CLAUDE.md's
hallucination-check discipline. None of them are correctness defects on
the bug-fix path itself — the user's bug is fixed at the
architectural layer — but each is a "say-do gap" between the plan and
the ship. We close those before we sign off, because that's the
shipping standard. The cycle is small: one Joel pass to plan, one Kent
pass for one new test, one Rob pass for ~30 lines of impl, one Raymond
pass for two doc edits and one backlog line. Linus can re-review with
minimal surface area.

After this next cycle, criteria 1-7, 12-14, 16-18 are HIT;
criterion 15 is HIT; criteria 8-11 and 19 remain operator-gated as
designed. That's done done.

— Don
