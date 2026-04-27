# 013 — Joel's Tech Plan, Cycle 2

Author: Joel Spolsky (planning agent)
Date: 2026-04-27
Status: Cycle-2 expansion of `012-don-final-review.md`'s 5-item list. Builds on `006-joel-tech-plan-v2.md` (still the source of truth). Awaiting Linus re-review.

## 0. How to read this document

Don's `012-don-final-review.md` enumerated five surgical items to close out before sign-off. This plan binds each to a file path, a function signature, and a test. Nothing in `006-joel-tech-plan-v2.md` is being revised — the v2 plan stands; cycle-2 fills the "say-do gaps" between v2 plan and v2 ship.

Items in scope (Don's list):

1. Implement port-conflict substring detection in `Manager.Up` (Joel v2 §1.5 row 1).
2. Reword `_docs/install.md:189` (IPv6 example — doc fab).
3. Reword `_docs/install.md:173` (ports example) AFTER item #1 lands.
4. Add `Long` help text to `decloud caddy up` and `decloud caddy down` (Joel v2 §1.6).
5. Append the integration-test backlog item to `_ai/m1x-backlog.md` (v2 acceptance criterion #15).

Out of scope (Don's list, explicit):
- Reloader stderr `%q` quoting (deferred to M2).
- `"decloud"` literal cleanup (M1.x backlog).
- Wrap-text duplication in `service.go:316,322` (below rule-of-three).
- Stdout-prefix cosmetic inconsistency (held).

There is one open question for Linus, listed in §6.

---

## 1. Item #1 — Port-conflict substring detection in `Manager.Up`

### 1.1 The exact substrings

The detection branch fires when `Driver.RunWithOptions` returns an error AND the error chain (post-wrap) contains EITHER of these case-sensitive substrings:

- `address already in use` — emitted by the kernel via Docker when `bind(2)` fails. Format from Docker 20.10+ on Linux:
  ```
  docker: Error response from daemon: driver failed programming external connectivity on endpoint decloud-caddy (...): Error starting userland proxy: listen tcp4 0.0.0.0:80: bind: address already in use.
  ```
- `port is already allocated` — Docker's allocator-side variant when a different docker-managed container holds the port:
  ```
  docker: Error response from daemon: driver failed programming external connectivity on endpoint decloud-caddy (...): Bind for 0.0.0.0:80 failed: port is already allocated.
  ```

Both substrings are case-sensitive. Both have been the canonical Docker error text since at least 20.10 (the M1 minimum docker version per `_docs/install.md` §1) and remain unchanged through the latest Docker CE at the time of writing. Substring miss → fall through to today's generic wrap. We accept the brittleness — see §1.4.

**Why these two substrings and not the `bind:` prefix:** the `bind:` token appears ONLY in the syscall-level variant; the allocator variant has no `bind:` at all. Matching on `bind:` would miss the second case. Two contains-checks is cheaper than a regex and reads cleanly.

### 1.2 Where the detection lives — driver vs. manager

**Decision: in `internal/caddy/manager.go::Up`, NOT in `internal/dockerdrv/cli_driver.go::RunWithOptions`.**

Rationale:

1. The detection composes a CADDY-SPECIFIC error message ("ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl mask caddy' ..."). The driver does not know which container's ports are which — `RunWithOptions` is a generic primitive that runs ANY container. Pushing port-conflict text into the driver leaks Caddy semantics into a Caddy-agnostic layer and would force every other caller of `RunWithOptions` (today: none; M2+: maybe service deploys with published ports) to either inherit Caddy-specific text or override.
2. The driver already wraps with `; stderr=%q` so the substring is preserved into the error chain — `fmt.Errorf("docker run: %w; stderr=%q", err, stderr.String())` (`internal/dockerdrv/cli_driver.go:239`). The manager can `strings.Contains(err.Error(), substring)` against that chain without any driver change.
3. The reloader has `isNotRunningStderr` as a manager-layer helper (`internal/caddy/reloader.go:89`) — a precedent for "stderr-substring detection is the caller's job, not the driver's." Symmetric placement.
4. Keeps `RunWithOptions` byte-identical from this cycle — no driver test changes, no `cli_driver_test.go` churn.

**Trade-off accepted:** The detection inspects the wrapped `err.Error()` string rather than a structured stderr field. We don't have a structured field today — `Driver.RunWithOptions` returns the error chain only. Adding one would expand the driver surface for one caller. Substring-on-error-string is the cheap right answer here.

### 1.3 Exact code shape — `internal/caddy/manager.go::Up`

Current code at line 94-96:

```go
if _, err := m.cfg.Driver.RunWithOptions(ctx, m.runOpts()); err != nil {
    return fmt.Errorf("%w: docker run: %w", ErrCaddyUp, err)
}
```

Updated code (replace lines 94-96):

```go
if _, err := m.cfg.Driver.RunWithOptions(ctx, m.runOpts()); err != nil {
    if isPortsBoundErr(err) {
        return fmt.Errorf("%w: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent", ErrCaddyUp)
    }
    return fmt.Errorf("%w: docker run: %w", ErrCaddyUp, err)
}
```

Add the helper near the bottom of `manager.go` (after `runOpts`):

```go
// isPortsBoundErr matches the two stderr substrings docker emits when a
// publish-target port is already bound on the host: kernel bind(2) failure
// ("address already in use") and docker allocator failure ("port is already
// allocated"). Brittle to docker error-text wording; locked by
// TestManager_UpPortsBoundActionableError so a docker upgrade that reworded
// the message would fail loudly.
func isPortsBoundErr(err error) bool {
    s := err.Error()
    return strings.Contains(s, "address already in use") ||
        strings.Contains(s, "port is already allocated")
}
```

This adds a single import: `"strings"`.

### 1.4 Brittleness, named honestly

Named for Linus (`007-linus-review-v2.md` non-blocking nit #2). The substring approach is brittle along three axes:

- **Docker error-text wording.** A future Docker version that reworded `address already in use` (e.g., to `address in use`) would silently fall through to the generic wrap. The test in §1.5 locks the current shape so the failure is loud at test time, not in production.
- **Case sensitivity.** Docker's stderr is consistently lowercase for these two phrases; we don't lowercase before matching. If a future variant capitalised any letter in either phrase (unlikely — these strings come from the kernel `errno` text via `strerror`, which is locale-stable), the match fails. Trade-off: case-folding costs an allocation per check; we don't fold.
- **Localisation.** Docker on a non-`C` locale could emit translated stderr. We accept this — Decloud is an English-only single-operator tool in M1, the install doc assumes `LANG=C`/`en_US.UTF-8`, and locale-sensitive stderr would surface a thousand other issues before hitting this branch.

**The right balance per Linus:** match the canonical strings, lock with a test, document the brittleness in the helper comment. Don't pre-emptively widen the match to phrases Docker doesn't emit — that's fragility-by-imagination. This is what the helper comment above already says.

### 1.5 New test — `internal/caddy/manager_test.go::TestManager_UpPortsBoundActionableError`

Add to `internal/caddy/manager_test.go`. Use the existing harness (`newManagerHarness`, `absentInspect`).

```go
func TestManager_UpPortsBoundActionableError(t *testing.T) {
    cases := []struct {
        name      string
        stderrSub string
    }{
        {"kernel bind", "Error starting userland proxy: listen tcp4 0.0.0.0:80: bind: address already in use"},
        {"docker allocator", "Bind for 0.0.0.0:80 failed: port is already allocated"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            h := newManagerHarness(t)
            // Simulate the wrapped error shape from cli_driver.go:239.
            runErr := fmt.Errorf("docker run: exit status 125; stderr=%q", tc.stderrSub)
            gomock.InOrder(
                h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil),
                h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(absentInspect(), nil),
                h.driver.EXPECT().ImagePull(gomock.Any(), caddy.DefaultImage).Return(nil),
                h.driver.EXPECT().RunWithOptions(gomock.Any(), gomock.Any()).Return("", runErr),
            )
            err := h.mgr.Up(context.Background())
            require.Error(t, err)
            assert.ErrorIs(t, err, caddy.ErrCaddyUp)
            assert.Contains(t, err.Error(), "ports 80/443 already in use")
            assert.Contains(t, err.Error(), "systemctl mask caddy")
            assert.Contains(t, err.Error(), "apt-get remove -y caddy")
            // Negative: the generic 'docker run:' prefix from the fallthrough
            // path must NOT be present — we took the actionable branch.
            assert.NotContains(t, err.Error(), ": docker run: docker run:")
        })
    }
}
```

**Why two sub-tests:** locks both substring matches independently. A regression that fixes one but not the other should fail visibly.

**Why the `NotContains` assertion:** if a future refactor accidentally fell through both branches (returning the generic wrap with the actionable text appended), the contains-checks alone would pass. The negative assertion locks the branch choice, not just the message content.

**Test file imports:** `fmt` already imported; no new imports needed.

### 1.6 Stdout shape — unchanged

The actionable error is on `error` return, not stdout. `Up` still emits `caddy up: pulled caddy:2` on the success path before the `RunWithOptions` call. On the actionable-error path it emits the pull line, then returns the error — operator sees the pull line on stdout AND the actionable error on stderr (via Cobra's default error rendering).

This matches today's behaviour for any other `Up` failure post-pull. No change needed to the existing stdout-shape tests.

---

## 2. Item #2 — `_docs/install.md:189` IPv6 doc rewording

The current text (lines 186-192):

```markdown
### IPv6 listener fails to bind

```
caddy up: docker run: listen tcp [::]:80: socket: address family not supported by protocol
```

The kernel has IPv6 disabled (`net.ipv6.conf.all.disable_ipv6=1`). Re-enable IPv6, or accept that this host cannot serve IPv6 clients. M1 does not have a flag to opt out of dual-stack publishing.
```

The displayed string is fabricated. The actual operator-visible error chain is the wrapped form from `Manager.Up` line 95:

```
caddy: up failed: docker run: docker run: exit status N; stderr="...listen tcp [::]:80: socket: address family not supported by protocol..."
```

(Note the doubled `docker run:` is real — once from the manager wrap, once from the driver wrap. We do not refactor that in this cycle; Don §5.2 held the wrap shape.)

### 2.1 Replacement prose

Replace lines 186-192 with:

```markdown
### IPv6 listener fails to bind

`decloud caddy up` fails with stderr containing:

```
listen tcp [::]:80: socket: address family not supported by protocol
```

The kernel has IPv6 disabled (`net.ipv6.conf.all.disable_ipv6=1`). Re-enable IPv6, or accept that this host cannot serve IPv6 clients. M1 does not have a flag to opt out of dual-stack publishing.
```

**Key changes:**
1. Frame the displayed string as "stderr containing" rather than as the literal error rendering — this is honest about substring-match semantics.
2. Drop the fake `caddy up:` prefix entirely. The wrapped chain has `caddy: up failed:` (the `ErrCaddyUp` text), then `docker run:` from the manager, then `docker run:` from the driver, then `; stderr=...`. Showing the substring inside stderr is the truthful, future-proof presentation.
3. Keep the recovery instructions byte-identical — they remain correct.

**Why "stderr containing" and not the full wrapped chain:** showing the full chain is technically accurate but operator-hostile. Operators grep for the IPv6 substring; that's the actionable signal. The presentation matches what they'd actually do (`decloud caddy up 2>&1 | grep IPv6`).

---

## 3. Item #3 — `_docs/install.md:173` ports doc rewording (AFTER item #1)

Sequencing matters: this edit depends on the EXACT string emitted by the new substring branch (§1.3). Rob lands #1 before Raymond touches #3.

The current text (lines 170-184):

```markdown
### Ports 80/443 already bound

```
caddy up: ports 80/443 already in use
```

Something else is listening on the public ports. Almost always a leftover host Caddy from the M1.0 install. Make the disable persistent:

```sh
systemctl disable --now caddy && systemctl mask caddy
# OR
apt-get remove -y caddy
```

`systemctl disable --now caddy` alone is **not** durable — package upgrades re-enable the unit. Use `mask` or fully remove the package.
```

After §1.3 lands, the EXACT operator-visible error becomes (single `%w` wrap of `ErrCaddyUp` via `fmt.Errorf` — `ErrCaddyUp.Error()` is `caddy: up failed`):

```
caddy: up failed: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent
```

### 3.1 Replacement prose

Replace lines 170-184 with:

```markdown
### Ports 80/443 already bound

`decloud caddy up` fails with:

```
caddy: up failed: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent
```

Something else is listening on the public ports. Almost always a leftover host Caddy from the M1.0 install. The error already names the recovery commands; the persistent-disable step is mandatory because:

```sh
systemctl disable --now caddy && systemctl mask caddy
# OR
apt-get remove -y caddy
```

`systemctl disable --now caddy` alone is **not** durable — package upgrades re-enable the unit. Use `mask` or fully remove the package.
```

**Verification step for Raymond before sign-off:** copy the EXACT error string from §1.3's `fmt.Errorf` literal into the markdown fenced block. Byte-for-byte. The recovery shell block underneath repeats the commands intentionally — operators who skim past the error text still see them in a copy-pasteable code block.

**If Rob's implementation in §1.3 deviates from the spec'd literal:** Raymond updates the doc to match the implementation, NOT the other way around (per CLAUDE.md hallucination-check discipline).

---

## 4. Item #4 — `Long` help text on `caddy up` and `caddy down`

Per Joel v2 §1.6, with the volume-retention warning emphasised per Don §4a / Linus 9.1. Specified verbatim so Rob doesn't drift.

### 4.1 `internal/cli/caddy_up.go` — add `Long`

Replace the existing command literal (lines 10-23) with:

```go
func newCaddyUpCmd(rc *rootContext) *cobra.Command {
    return &cobra.Command{
        Use:   "up",
        Short: "Run the decloud-caddy container on the decloud network",
        Long: `Run the decloud-caddy container on the decloud network.

Ensures the decloud Docker network exists, writes the Caddyfile stub if
missing, and starts (or runs) the decloud-caddy container with dual-stack
publishing on 80/tcp, 443/tcp, and 443/udp (both 0.0.0.0 and [::]).

The container uses image caddy:2 and named volumes decloud_caddy_data (ACME
state and issued certs) and decloud_caddy_config (runtime config). These
named volumes survive container removal — running 'decloud caddy down'
stops and removes the container but does NOT remove the volumes. Wipe them
manually with 'docker volume rm' only if you intend to discard ACME state.

Idempotent: if the container is already running, this command logs
'caddy already running' and exits 0. If the container exists but is
stopped, it is started in place.`,
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            mgr, err := caddyManagerFactory(config.NewPaths(rc.ConfigRoot))
            if err != nil {
                return fmt.Errorf("building caddy manager: %w", err)
            }
            return mgr.Up(cmd.Context())
        },
    }
}
```

### 4.2 `internal/cli/caddy_down.go` — add `Long`

Replace the existing command literal (lines 10-23) with:

```go
func newCaddyDownCmd(rc *rootContext) *cobra.Command {
    return &cobra.Command{
        Use:   "down",
        Short: "Stop and remove the decloud-caddy container (volumes preserved)",
        Long: `Stop and remove the decloud-caddy container.

Stopping Caddy interrupts ingress for ALL services routed by this Decloud
host. Live traffic will fail until 'decloud caddy up' is run again.

The named volumes decloud_caddy_data (ACME state, issued certs) and
decloud_caddy_config (runtime config) are NOT removed. Re-running
'decloud caddy up' brings Caddy back with the same certificates and
runtime state. Wipe the volumes manually with 'docker volume rm
decloud_caddy_data decloud_caddy_config' only if you intend to discard
ACME state — that forces fresh Let's Encrypt issuance and risks tripping
LE rate limits on hosts with many domains.

Idempotent: if the container is already absent, this command exits 0.`,
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            mgr, err := caddyManagerFactory(config.NewPaths(rc.ConfigRoot))
            if err != nil {
                return fmt.Errorf("building caddy manager: %w", err)
            }
            return mgr.Down(cmd.Context())
        },
    }
}
```

### 4.3 No new tests

Cobra renders `Long` independently of `Short`. The existing `TestCaddyUp_NoFlags` / `TestCaddyDown_NoFlags` regression guards still pass without modification — they assert against the unknown-flag rejection path, which is independent of help text. `gofmt` and `go vet` validate the literal strings.

**No behaviour change.** No new exit codes, no new errors, no flag surface change. Operators running `decloud caddy up --help` or `decloud caddy down --help` get the longer prose; everything else is unchanged.

**Why warn about LE rate limits on `down --help`:** the volume-removal trap is the operationally-painful 3 AM scenario. Naming LE rate limits explicitly makes the warning land. Per Don §4a "do not soften it." This text does not soften it.

---

## 5. Item #5 — `_ai/m1x-backlog.md` integration-test entry

Append a new item #6 (the file currently has items 1-5). The wording per Joel v2 §8.5, expanded so it matches the file's existing entry style (where, why deferred, fix shape, originator). Specified verbatim so Raymond doesn't drift.

### 5.1 Exact bullet wording

Insert after item 5 (current end of file is line 53), before the `## Maintenance note` section. New text:

```markdown
## 6. Real-Docker integration test for first happy-path deploy

**Where:** No file yet. Likely lives at `internal/integration/` (new package) or `_test/integration/`. Test invokes `decloud caddy up`, `decloud deploy service` against a real Docker daemon (CI runner with Docker-in-Docker, or a tagged opt-in test that requires `DECLOUD_INTEGRATION=1`), asserts a real HTTP request through Caddy reaches a real upstream container.

**Why deferred:** Per `_ai/decisions/m1-test-strategy.md`, M1 is unit-tests-only against the gomock'd `Driver`. The bridge-DNS resolution path is locked architecturally by the new `decloud-caddy`-on-`decloud`-network design (`_ai/decisions/caddy-runs-in-container.md`); the only thing a real-Docker test catches that unit tests miss is "is our argv actually accepted by docker?", and the §6.3 argv-shape tests in `internal/dockerdrv/cli_driver_test.go` lock that argv byte-for-byte. Don confirmed this scoping in `_tasks/2026-04-27-caddy-container-connection-refused/002-don-plan.md` §6.4; Linus confirmed in `004-linus-review.md` §7.

**Fix shape:** New `integration_test.go` build-tagged with `//go:build integration`, requires `DECLOUD_INTEGRATION=1` to run, brings up Caddy, deploys a one-line nginx service, curls through Caddy, asserts 200 OK with nginx body. Tear down both containers and the network on completion. Cleanup must be idempotent (test failures must not leave dangling containers). M2 material; M2 is also the milestone where reloader stderr `%q` quoting (Linus 9.2) gets revisited, so the integration test naturally covers that improvement too.

**Originator:** Joel §8.5 of `_tasks/2026-04-27-caddy-container-connection-refused/006-joel-tech-plan-v2.md`. Acknowledged by Don in `012-don-final-review.md` §5.1.
```

**Why this length:** matches the existing items' style (each ~10-12 lines, with where / why deferred / fix shape / originator). A two-sentence stub would be inconsistent with the file's voice and would lose the "what does this look like when picked up" guidance that future-Don needs.

---

## 6. Open question for Linus

**One open question — for §1.2 placement:**

Should `isPortsBoundErr` live next to `isNotRunningStderr` in `internal/caddy/reloader.go` (as a "stderr-substring detection" toolkit) or in `internal/caddy/manager.go` next to its single caller? I chose `manager.go` because:
- The helper is one-shot (Caddy ports specifically; reloader's helper is exec-not-running specifically).
- Cross-file co-location of unrelated helpers solely on the basis of "they both use `strings.Contains`" feels like grouping by mechanism rather than by purpose.
- The package-private helper in `manager.go` is invisible to other callers; if a third stderr-substring helper appears, that's the trigger to extract a `stderr_match.go` module.

Linus called this out as a non-blocking-but-mild nit before. If he has a strong preference for a `stderr_match.go` extraction now (so we have one home for these helpers when item #2 comes back as M2 work), I'll accept the placement change. Otherwise: helpers stay co-located with their callers.

No other open questions. The two doc rewordings are mechanical; the `Long` text is verbatim from §4; the backlog item is verbatim from §5.

---

## 7. Test inventory delta from v2

Cycle 2 adds exactly ONE test:

| File | Test | Locks |
|---|---|---|
| `internal/caddy/manager_test.go` | `TestManager_UpPortsBoundActionableError` | The two substring matches and the actionable error text per §1.5 |

No tests dropped. No tests modified. All cycle-1 tests continue to pass.

`internal/dockerdrv/cli_driver_test.go` is NOT touched in this cycle (the detection lives in the manager layer, per §1.2).

`internal/cli/caddy_up_test.go` and `caddy_down_test.go` are NOT touched (Cobra renders `Long` independently — no contract change).

---

## 8. Phase ordering for cycle 2

Per Don §"Changes for Joel to plan and Kent/Rob/Raymond to execute":

**Phase 1 — Test (Kent):**
- Add `TestManager_UpPortsBoundActionableError` to `internal/caddy/manager_test.go` per §1.5. Test fails (helper does not exist yet).

**Phase 2 — Impl (Rob), parallel-safe with Phase 4:**
- Add `isPortsBoundErr` helper to `internal/caddy/manager.go` per §1.3.
- Edit `Manager.Up`'s `RunWithOptions` failure branch per §1.3.
- `go test ./internal/caddy/... -count=1` green.

**Phase 3 — Impl (Rob), parallel-safe with Phase 2:**
- Add `Long` strings to `internal/cli/caddy_up.go` and `internal/cli/caddy_down.go` per §4.1 / §4.2.
- `go test ./internal/cli/... -count=1` green (no new tests; existing ones must still pass).

**Phase 4 — Docs (Raymond), depends on Phase 2 for §3 only:**
- Reword `_docs/install.md:186-192` per §2.1.
- Reword `_docs/install.md:170-184` per §3.1, copying the EXACT error string from Rob's Phase 2 implementation. **Verify byte-for-byte against the literal in `manager.go::Up`.**
- Append the new item #6 to `_ai/m1x-backlog.md` per §5.1.

**Phase 5 — Verification gate:**
- `gofmt -l .` empty.
- `go vet ./...` empty.
- `go generate ./...` then `git status --porcelain` empty (no mock-shape changes expected; no `//go:generate` directive added).
- `go test ./... -count=1` green.

**Phase 6 — Review:**
- Kevlin re-checks the doc text against the new error string (same hallucination-check discipline as cycle 1).
- Linus re-checks the placement of `isPortsBoundErr` and the `Long` text.
- Don's PLAN-loop runs to verify acceptance criteria #12 and #15 of v2 §11 now flip from MOSTLY HIT/MISS to HIT.

**Dependencies:**
- Phase 4's §3 edit depends on Phase 2 (the EXACT error literal lands in code first).
- Phase 4's §2 edit and §5 backlog edit are independent of Phase 2 — Raymond can start them in parallel with Rob's work.
- Phase 3 is independent of Phase 2.
- Phase 5 depends on Phases 1-4.

Estimated effort: Kent ~30 minutes for one test, Rob ~30 minutes for the helper + edit + the two `Long` strings, Raymond ~20 minutes for three doc edits. Total cycle 2 wall time: ~2 hours of agent work.

---

## 9. Acceptance criteria for cycle 2

After this cycle lands:

| # | Criterion | Verification |
|---|---|---|
| C2-1 | `Manager.Up` returns the actionable port-conflict text when stderr contains `address already in use` | `TestManager_UpPortsBoundActionableError` sub-test "kernel bind" |
| C2-2 | Same, for `port is already allocated` | Same test, sub-test "docker allocator" |
| C2-3 | Generic wrap path still fires for all other `RunWithOptions` failures | Existing `TestManager_UpRunFailsWithoutRollback` passes unchanged (asserts `errors.Is(err, ErrCaddyUp)` against a sentinel that does not contain either substring) |
| C2-4 | `_docs/install.md:189` example string is the substring inside stderr, not a fabricated wrapped chain | Manual diff vs. §2.1 |
| C2-5 | `_docs/install.md:173` example matches the EXACT string emitted by the new branch | `grep -F` the literal from manager.go into install.md returns a match |
| C2-6 | `decloud caddy up --help` includes the volume-retention text and dual-stack note | Manual `--help` invocation |
| C2-7 | `decloud caddy down --help` includes the ingress-interruption warning AND the LE rate-limit warning | Manual `--help` invocation |
| C2-8 | `_ai/m1x-backlog.md` item #6 exists and matches §5.1 verbatim | `grep -c "^## 6\\. Real-Docker"` returns 1 |
| C2-9 | `gofmt -l .` empty | Phase 5 |
| C2-10 | `go test ./... -count=1` green | Phase 5 |
| C2-11 | `go generate ./...` produces no diff | Phase 5 |

After this cycle, v2 §11 acceptance criteria #12 and #15 flip from MOSTLY HIT / MISS to HIT. Criteria 8-11 and 19 remain operator-gated as designed.

---

## 10. Risk matrix

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Docker rewords stderr in a future version, breaks substring match | Medium (5+ year horizon) | Low (falls through to generic wrap; recovery still achievable, just less ergonomic) | Test locks current shape; failure is loud |
| Operator runs into a port-conflict that emits a non-canonical stderr (Docker on Windows? Podman?) | Low (M1 supports Linux-only docker-ce per install.md §1) | Low (same fall-through) | Out of scope for M1 |
| Raymond copies the §3.1 error string and misses a character | Medium (manual transcription) | Medium (doc-fab regression) | Phase 4 ordering: Rob lands the literal first, Raymond copies from source; Kevlin re-checks |
| Rob types the `Long` text and introduces a typo that operators see | Low | Low (cosmetic) | gofmt validates the literal compiles; manual `--help` smoke test in Phase 5 |
| `Long` text drifts from `_docs/install.md`'s migration recipe | Low | Low (two sources of truth on the same warning is fine; the docs are the canonical operator surface) | Linus re-review |

No high-impact risks. The brittleness in §1.4 is the highest-impact item and it's medium-likelihood / low-impact, with a test-as-canary mitigation.

---

## 11. Simplification opportunities (none taken)

For completeness — items considered and rejected:

1. **Could detection live in the driver?** Considered in §1.2. Rejected: leaks Caddy semantics into a Caddy-agnostic primitive.
2. **Could detection use a regex?** Considered. Rejected: two `strings.Contains` calls are 8 lines clearer than a regex compile + match, and this code path is hot in failure mode but cold in production.
3. **Could the actionable error be a sentinel (`ErrPortsBound`)?** Considered. Rejected: no caller cares about the typed distinction; `errors.Is(err, ErrCaddyUp)` is what the exit-code mapper checks. Adding a sentinel for a single error-text branch is over-engineering.
4. **Could the `Long` text be loaded from an embedded `.txt` file via `//go:embed`?** Considered. Rejected: two strings, one in each command file, both small. Embedding adds a build-time indirection for no readability win.
5. **Could the doc rewordings use a shared "what error you actually see" template?** Considered. Rejected: two examples is below rule-of-three and the IPv6 case has no actionable rewrite path while the ports case does — different shapes.

Rejecting these is correct. Don't take simplifications that obscure intent.

---

## 12. Coordination with the v1 plan

`006-joel-tech-plan-v2.md` is the canonical source of truth for everything not in this cycle 2 plan. Where v2 and cycle 2 conflict (they don't, but for clarity):

- v2 §1.5 row 1 specified the substring detection and its message text. Cycle 2 §1 implements that spec. The message text in cycle 2 §1.3 matches v2 §1.5 row 1 byte-for-byte.
- v2 §1.6 specified the `Long` help text in summary form. Cycle 2 §4 expands to verbatim text per Don's "specify exact prose so Rob doesn't drift" directive.
- v2 §8.5 specified the backlog item one-liner. Cycle 2 §5 expands to the file's standard entry style.

Nothing in v2 is overridden. v2's acceptance criteria stand; cycle 2 is the cycle that closes the items v2 specified-but-execution-missed.

---

— Joel
