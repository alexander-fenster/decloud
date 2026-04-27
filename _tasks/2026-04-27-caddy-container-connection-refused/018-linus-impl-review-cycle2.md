# 018 — Linus Cycle-2 Implementation Review

Author: Linus Torvalds (high-level review agent)
Date: 2026-04-27
Status: High-level review of cycle-2 EXECUTION (Rob 016 + Raymond 017) against
the cycle-2 PLAN I already approved (014). Audited against the source on disk.

## 0. Reading log

In order, in full:

1. `_tasks/.../012-don-final-review.md` — Don's 5-item cycle-2 list.
2. `_tasks/.../013-joel-tech-plan-cycle2.md` — the cycle-2 plan I approved.
3. `_tasks/.../014-linus-review-cycle2.md` — my cycle-2 plan approval.
4. `_tasks/.../016-rob-implementation-cycle2.md` — Rob's impl report.
5. `_tasks/.../017-raymond-docs-cycle2.md` — Raymond's docs report.

Cross-checked against source:

- `internal/caddy/manager.go` (the new actionable branch + helper).
- `internal/caddy/manager_test.go` lines 179-208 (`TestManager_UpPortsBoundActionableError`).
- `internal/cli/caddy_up.go` (full file — `Long` text live).
- `internal/cli/caddy_down.go` (full file — `Long` text live).
- `internal/cli/exit_codes.go` (`errors.Is(err, caddy.ErrCaddyUp)` mapping
  on lines 58-59 — still works).
- `_docs/install.md` lines 168-200 (both reworded troubleshooting blocks).
- `_ai/m1x-backlog.md` (file end, item 6 appended in correct entry style).

I also ran:

- `go vet ./...` — clean.
- `gofmt -l .` — empty.
- `go test ./internal/caddy/... ./internal/cli/... -count=1` — green.
- A standalone harness that wraps the new actionable error and asserts both
  `errors.Is(err, ErrCaddyUp)` and the rendered `err.Error()`. Result:

  ```
  ErrorIs ErrCaddyUp: true
  Rendered: caddy: up failed: ports 80/443 already in use; if you ran the
   M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy'
   or 'apt-get remove -y caddy' to make the change persistent
  ```

  No double-prefix (`caddy: up failed: caddy: up failed:`), no leaked
  `docker run:` chain on the actionable branch. Sentinel preserved through
  `%w`.

- `grep -F` of the literal recovery-naming substring against `_docs/install.md`
  AND `internal/caddy/manager.go` — both files match byte-for-byte.

The reports are honest. No fabricated line numbers, no imagined behavior.

---

## 1. Substring detection placement — manager, not driver

**Verdict: correctly placed.**

`isPortsBoundErr` lives at `internal/caddy/manager.go:147-157`, package-private,
co-located with its single caller. `cli_driver.go` and `cli_driver_test.go`
were not touched. This is exactly what I called for in `014` §2 and what Joel
specified in `013` §1.2.

The driver's wrap shape (`docker run: %w; stderr=%q` at
`cli_driver.go:239`) feeds the substring through into the chain that
`isPortsBoundErr(err.Error())` inspects. No driver surface change. The Caddy
semantics ("ports 80/443 already in use", "M1.0 install") stay where they
belong — in the layer that knows it's running Caddy.

The helper's brittleness comment is accurate and self-documenting. If a
future Docker rewords the stderr, `TestManager_UpPortsBoundActionableError`
fails loudly. That's the right trade.

## 2. Error wrap shape — `errors.Is(ErrCaddyUp)` and double-prefix audit

**Verdict: correct and clean.**

The actionable branch:

```go
return fmt.Errorf("%w: ports 80/443 already in use; ...", ErrCaddyUp)
```

This is a SINGLE `%w` wrap of `ErrCaddyUp`. The driver-side error is
deliberately dropped on this branch — Rob calls this out explicitly in
`016` and Kent's `NotContains(": docker run: docker run:")` assertion locks
the choice. Three checks I cared about:

1. **`errors.Is(err, caddy.ErrCaddyUp)` still returns true** → verified via
   the standalone harness in §0. Exit-code mapper (`exit_codes.go:58-59`)
   continues to map this to `ExitRunFail` (40). No regression.
2. **No double prefix.** Rendered output is `caddy: up failed: ports
   80/443 already in use; ...` — clean. The `ErrCaddyUp.Error()` ("caddy:
   up failed") interpolates exactly once via `%w`.
3. **No leaked driver chain.** The driver's `docker run:` and `; stderr=%q`
   noise is suppressed on the actionable branch. The actionable text already
   names the recovery commands; the driver chain would be redundant noise.
   Operator-friendly choice.

The fall-through path (`fmt.Errorf("%w: docker run: %w", ErrCaddyUp, err)`
at line 99) is unchanged — non-port-conflict failures still preserve the
inner driver error in the chain. `TestManager_UpRunFailsWithoutRollback`
continues to pass with its `errors.New("port allocation failed")` sentinel
(which contains neither canonical Docker substring), so the negative case
is genuinely covered, addressing my cycle-2 plan-review caveat in `014` §1.

**One observation, NOT a blocker:** the error string is 207 characters in
one line. I noted in `014` §3 that this is acceptable but a touch verbose,
and that Don's call was to ship verbatim. Don shipped verbatim. Fine. If
M2 wants to revisit, the text "to make the change persistent" is the
natural trim target — but that's a future-Don decision, not a cycle-2
defect.

## 3. `Long` help text — operator value delivered

**Verdict: text matches the spec verbatim; renders correctly; volume-retention
warning is appropriately prominent.**

I read both rendered help outputs in Rob's report §"Rendered `--help` output"
and traced them back to the literal strings in `caddy_up.go:14-28` and
`caddy_down.go:14-27`. Three things that matter:

### 3.1 Volume retention on `caddy up --help`

Paragraph 3 reads:

> "These named volumes survive container removal — running 'decloud caddy
> down' stops and removes the container but does NOT remove the volumes.
> Wipe them manually with 'docker volume rm' only if you intend to discard
> ACME state."

This is the right shape: name the data, name the consequence of not knowing
the data persists, point at the discard-only escape hatch. Honest about
the `down`-doesn't-wipe semantics without inviting "go ahead, wipe them"
energy.

### 3.2 Volume retention + LE rate-limit warning on `caddy down --help`

Paragraph 3 reads:

> "...Wipe the volumes manually with 'docker volume rm decloud_caddy_data
> decloud_caddy_config' only if you intend to discard ACME state — that
> forces fresh Let's Encrypt issuance and risks tripping LE rate limits on
> hosts with many domains."

The LE rate-limit consequence is named. This is what I called for in `014`
§4 — the LE warning is the move that earns this paragraph its rent.
Without it, "you can wipe the volumes" reads as permission. With it, the
operator pauses. Good prose-engineering.

### 3.3 Ingress severity on `caddy down --help`

Paragraph 2 reads:

> "Stopping Caddy interrupts ingress for ALL services routed by this
> Decloud host. Live traffic will fail until 'decloud caddy up' is run
> again."

The capital `ALL` carries the severity. Operators need to feel that. Don't
soften it.

### 3.4 Cobra rendering — no surprises

Rob's pasted `--help` output shows clean line breaks from the raw backtick
literals. No mid-sentence wrap, no awkward indentation. The smoke test I
asked for in `014` §4 was done; the result is in `016` §"Rendered `--help`
output". Approved.

## 4. Doc accuracy — install.md:173 and :190

**Verdict: both troubleshooting blocks are now honest. The byte-for-byte
match for the ports block is verified.**

### 4.1 Ports block (install.md:170-186)

The displayed example string at line 175:

```
caddy: up failed: ports 80/443 already in use; if you ran the M1.0 install,
run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get
remove -y caddy' to make the change persistent
```

I `grep -F`'d the literal substring against `manager.go` and `install.md`.
Match. The doc shows what the operator will actually see, character for
character. The CLAUDE.md hallucination-check discipline is satisfied for
this block.

### 4.2 IPv6 block (install.md:188-196)

The new prose:

> "`decloud caddy up` fails with stderr containing `listen tcp [::]:80:
> socket: address family not supported by protocol`. The raw `docker run`
> stderr is surfaced as-is; it typically reads similar to: ..."

This is the right framing. It honestly tells the operator:

1. What substring to grep for (the IPv6-specific signal).
2. That the surrounding output is variable Docker daemon prose, not a
   wrapped Decloud chain.

The fenced example uses ellipses to indicate variable surrounding text
without committing to a precise daemon-version-specific line. This is
exactly what the user's EXECUTION 3.3 instructions called for and is more
honest than my own cycle-2 plan-review blessing of Joel's §2.1 prose,
which had committed to a specific wrapped chain. Raymond's deviation here
is an improvement, not a defect.

The recovery prose (re-enable IPv6, M1 has no opt-out flag) is unchanged
and remains correct.

**Both doc-fab violations Kevlin caught in cycle-1 are now closed.**

## 5. `_ai/m1x-backlog.md` item #6

**Verdict: appended in the right place, in the right voice.**

Item #6 (lines 55-63) sits between item #5 and `## Maintenance note` —
correctly placed. Format matches items #1-#5 exactly: four bold-prefixed
sections (Where / Why deferred / Fix shape / Originator), each carrying
substantive detail. The "deferred from the caddy-container-connection-refused
task per `_ai/decisions/m1-test-strategy.md`" phrase appears verbatim in
the **Why deferred** section per the user's EXECUTION 3.3 wording.

The cross-references to `_ai/decisions/m1-test-strategy.md` and
`_ai/decisions/caddy-runs-in-container.md` are correct — both files exist
and the citations align with their content.

Heading: "Docker-compose-based smoke integration test for M1 deploy + Caddy
ingress." Slight semantic stretch — Joel's §5.1 imagined a Go integration
test build-tagged with `//go:build integration`, not a docker-compose
harness, and the **Fix shape** paragraph still describes the Go-test shape.
"Docker-compose-based" in the heading is the user's wording from the
EXECUTION 3.3 instructions, and Raymond preserved it. Future-Don picking
this up will have to reconcile "compose-based" (heading) vs
"`integration_test.go` build-tagged" (fix shape). Not a cycle-2 blocker —
it's M2 backlog material that M2 will scope properly.

Acceptance criterion #15 from v2 §11 flips from MISS to HIT.

## 6. Migration story — end-to-end coherence check

**Verdict: coherent.**

I traced the operator journey from M1.0 → M1.1 once more:

1. Operator on M1.0 has `apt`-installed Caddy holding ports 80/443.
2. Upgrades to M1.1, runs `decloud caddy up`.
3. Docker tries to bind 80/443, kernel returns `EADDRINUSE`,
   `bind:` shows up in stderr with `address already in use` substring.
4. `Manager.Up`'s new branch (`manager.go:96-98`) catches it, returns
   `caddy: up failed: ports 80/443 already in use; ... systemctl mask caddy
   ... apt-get remove -y caddy ...` — which is the exact string operators
   will see, and exactly the string `_docs/install.md:175` shows them.
5. Operator runs one of the recovery commands, retries `decloud caddy up`,
   succeeds.

The error → doc → recovery loop is closed and self-consistent. No fabricated
intermediate steps, no wishful "this should work" prose. The migration
recipe in `_docs/install.md` (which I verified in cycle-1) leads with the
volume-copy step before the recovery commands, so an operator with M1.0
state actually preserves it.

Caddy logs grep prose at `install.md:200` (the `dial tcp [<public IPv6>]:`
diagnostic for the ORIGINAL bug) is unchanged and remains correct — the
architectural fix (Caddy in-container on `decloud` network) means that
error path is no longer reachable on a healthy host.

## 7. Anything cycle-2 made WORSE than cycle-1?

**Verdict: no.**

Concrete checks:

- **Test surface.** Cycle 2 added one test (two sub-tests) and modified zero.
  Net delta: positive (more coverage of the failure-mode path) with no
  fragility added.
- **Code surface.** Cycle 2 added 11 lines of Go (1 import, 4-line branch,
  6-line helper-with-comment) and ~30 lines of Cobra `Long` strings. No
  functions deleted, no signatures changed. Cycle-1 contracts intact.
- **Error API.** Sentinel preservation verified. Exit-code mapping verified.
  Existing tests (`TestManager_UpRunFailsWithoutRollback` et al) green
  unchanged.
- **Docs surface.** Both edited blocks shorter and more honest than the
  cycle-1 versions. `_ai/m1x-backlog.md` grows by one entry that matches
  the existing format. No prose regression.
- **Cycle-1 architecture decisions still hold.** Caddy-in-container on the
  `decloud` bridge. Driver primitives (`ImagePull` / `RunWithOptions` /
  `Exec`). Reload via `docker exec`. None of cycle-2 perturbed those.

The cycle is genuinely small, surgical, scoped — exactly what Don asked
for in `012` and what Joel planned in `013`.

## 8. Verdict

**APPROVED.**

Cycle 2 closes every item Don listed in `012`:

- Item #1 (port-conflict substring detection) — implemented at the right
  layer, correctly sentinel-wrapped, no double-prefix, locked by a test
  with both positive substrings and a branch-choice negative assertion.
- Item #2 (`install.md:189` IPv6 reword) — fabricated example replaced with
  honest "stderr containing X" framing.
- Item #3 (`install.md:173` ports reword) — byte-for-byte match against the
  emitted error literal.
- Item #4 (`Long` help text) — both commands carry the spec'd prose; the
  volume-retention and LE-rate-limit warnings are present and prominent.
- Item #5 (`_ai/m1x-backlog.md` item #6) — appended in correct place and
  voice.

Two non-blocking observations carried forward (already named above):

1. The 207-character actionable error string is verbose; if M2 revisits
   wording, "to make the change persistent" is the trim target.
2. The m1x-backlog #6 heading says "Docker-compose-based" while the fix
   shape describes a Go `integration_test.go`. Reconcile when picking the
   item up.

Neither blocks sign-off.

After cycle 2, v2 §11 acceptance criteria 1-7, 12-18 are HIT; criterion
15 is HIT; criteria 8-11 and 19 remain operator-gated as designed. Don can
flip the task to FULLY DONE pending the operator-side §7 manual
verification on the user's host.

The plan-vs-ship gap is now closed. Ship it.

— Linus
