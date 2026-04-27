# 018 — Kevlin's Cycle-2 Low-Level Review

Author: Kevlin Henney (low-level reviewer)
Date: 2026-04-27
Status: APPROVED. No revisions required.

## Reading log

1. `013-joel-tech-plan-cycle2.md` — full plan including §1.3 code shape, §1.5 test contract, §2.1 IPv6 reword, §3.1 ports reword, §4.1/§4.2 Long text verbatim, §5.1 backlog item.
2. `015-kent-tests-cycle2.md` — sub-test shape, harness reuse, failure log proving the test bites today.
3. `016-rob-implementation-cycle2.md` — implementation report with verification logs.
4. `017-raymond-docs-cycle2.md` — three doc edits with byte-for-byte verification.
5. `internal/caddy/manager.go` (current) — full file.
6. `internal/caddy/manager_test.go` (current) — full file, especially lines 179-208 (new test) and 160-177 (existing `UpRunFailsWithoutRollback`).
7. `internal/cli/caddy_up.go` and `internal/cli/caddy_down.go` (current).
8. `_docs/install.md` lines 168-200.
9. `_ai/m1x-backlog.md` (full file).

## 1. Code correctness — `internal/caddy/manager.go`

`isPortsBoundErr` (lines 153-157):

- Matches both substrings as separated `||` clauses on `err.Error()`. Substrings are byte-identical to the canonical Docker stderr text Joel quoted in §1.1.
- Helper comment names the trade-off honestly (brittleness, locked by test) — exactly what Kevlin asks for when a comment exists: explain WHY (the brittleness contract), not WHAT (the substring match).
- Helper is package-private and co-located with its single caller — placement matches Joel §6's open question / Linus's mild preference, consistent with the `isNotRunningStderr` precedent in `reloader.go`.

`Up` (lines 95-100):

- Actionable branch wraps `ErrCaddyUp` with `%w` only. Rendered chain is `caddy: up failed: ports 80/443 already in use; ...` — NO `docker run:` prefix at all, NO double prefix. Matches the test's `NotContains(": docker run: docker run:")` assertion.
- Generic fall-through path unchanged (`%w: docker run: %w` against `ErrCaddyUp` and the inner err) — `TestManager_UpRunFailsWithoutRollback`'s sentinel `errors.New("port allocation failed")` does NOT contain either canonical substring (the word "port" is shared but `"port is already allocated"` is not a substring of `"port allocation failed"`), so the existing test still locks the generic-wrap branch.
- `errors.Is(err, ErrCaddyUp)` holds on both branches — verified by the new test's line 201 assertion plus all pre-existing `Is` assertions still passing.
- `strings` import added cleanly. No dead code.

Verdict: clean. No double-prefix, no sentinel breakage, no leakage of Caddy semantics into the driver.

## 2. Test quality — `internal/caddy/manager_test.go::TestManager_UpPortsBoundActionableError`

Lines 179-208:

- Table-driven with two sub-tests covering both substrings independently — locks them as separate regression points per Joel §1.5 "two sub-tests so a regression that fixes one but not the other should fail visibly."
- The `runErr` shape uses `fmt.Errorf("docker run: exit status 125; stderr=%q", tc.stderrSub)` — this mirrors `cli_driver.go:239`'s wrap shape, which means the substring match in `Up` is exercised against the exact error chain shape it sees in production. Not a change-detector — it tests the contract (substring inside wrapped err.Error()).
- All five assertions are load-bearing: sentinel preserved + two recovery commands + actionable symptom + branch-choice negative. The `NotContains(": docker run: docker run:")` lock is the standout — without it, a future refactor that re-introduces inner-err wrap on the actionable branch would silently pass the four positive assertions.
- Harness reuse is correct (`newManagerHarness`, `absentInspect`, fresh-install precondition chain). No new helper introduced (correctly — single-use error literal is below rule-of-three).
- Kent matched `h.manager` (the actual harness field) over Joel's spec'd `h.mgr` — right call, no other deviations.

No `t.Log`/`t.Error`/`if`-branching anti-patterns. No skipped tests. No commented-out assertions. Test is self-documenting through its sub-test names ("kernel bind", "docker allocator").

Verdict: clean. This test pulls its weight.

## 3. Format / lint / build

- `gofmt -l .` → empty.
- `go vet ./...` → empty.
- `go test ./... -count=1` → all packages green (`internal/caddy 0.017s`, `internal/cli 0.023s`, `internal/deploy 12.069s`, etc.).
- No mock regen needed (no interface change).

## 4. Doc hallucinations — install.md

CRITICAL discipline check. Three substrings to verify byte-for-byte:

**Item 3 (ports-bound, line 175):** `grep -F` of the full literal:

```
ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent
```

Matches both `internal/caddy/manager.go:97` AND `_docs/install.md:175` byte-for-byte. The full rendered chain `caddy: up failed: ports 80/443 already in use; ...` in install.md correctly composes `ErrCaddyUp.Error()` (`"caddy: up failed"`) + `%w` separator (`": "`) + the literal. Verified via direct grep.

**Item 2 (IPv6, line 190):** `grep -F "listen tcp [::]:80: socket: address family not supported by protocol"` → match. The doc framing ("stderr containing", "raw `docker run` stderr is surfaced as-is", ellipses around the daemon prefix) is honest about substring-match semantics and does not commit to a precise wrapped chain. The deviation from Joel §2.1 is documented in Raymond's report §"Why this shape (deviation from Joel §2.1 prose)" and tracks the user's direct EXECUTION 3.3 instruction. No fabrication remains.

**Recovery shell block (lines 180-184):** `systemctl disable --now caddy && systemctl mask caddy` and `apt-get remove -y caddy` — both substrings are byte-identical to the strings inside the actionable error. No drift.

No hallucinations. No `caddy up:` legacy fake-prefix anywhere in the troubleshooting section. The `caddy: up failed:` prefix matches `ErrCaddyUp.Error()` exactly.

## 5. `Long` help text rendering

Rendered `go run ./cmd/decloud caddy up --help` matches Joel §4.1 verbatim:
- Three paragraphs, each as specified.
- Dual-stack publishing note present (80/tcp, 443/tcp, 443/udp on both 0.0.0.0 and [::]).
- Named-volume retention warning present with both volume names spelled correctly.
- Idempotency contract present (already running → exits 0; exited → started in place).

Rendered `go run ./cmd/decloud caddy down --help` matches Joel §4.2 verbatim:
- Ingress-interruption warning present ("interrupts ingress for ALL services routed by this Decloud host").
- Named-volume retention named correctly (`decloud_caddy_data`, `decloud_caddy_config`).
- LE rate-limit warning present ("forces fresh Let's Encrypt issuance and risks tripping LE rate limits on hosts with many domains") — Don §4a / Linus 9.1 compliance.
- Idempotency contract present.

Both `Args: cobra.NoArgs` retained. Both `RunE` bodies unchanged. No drift in flag surface, exit codes, or factory wiring. `gofmt` validates the literals compile.

## 6. Backlog format — `_ai/m1x-backlog.md`

Item #6 (lines 55-63) inserted between item 5 and the `## Maintenance note` section. Format check against items 1-5:

- Heading: `## 6. Docker-compose-based smoke integration test for M1 deploy + Caddy ingress` — matches the `## N. <title>` pattern.
- Four bold-prefixed sections: **Where:** / **Why deferred:** / **Fix shape:** / **Originator:** — all five existing items use exactly this structure.
- Length ~10 lines per section, matching items 1-5's voice.
- Originator naming convention (path + agent + section reference) consistent with item 1's "Linus, `20-linus-rereview.md`" style.

`grep -c "^## 6\\. " _ai/m1x-backlog.md` returns 1.

Note: Raymond's heading wording ("Docker-compose-based smoke integration test...") differs from Joel §5.1's spec'd ("Real-Docker integration test for first happy-path deploy"). This was an explicit user-instruction deviation per Raymond's §"Heading wording" note, and the bullet's content matches Joel §5.1 in substance. Acceptable.

## 7. Acceptance criteria coverage (Joel §9 cycle-2)

| # | Criterion | Status |
|---|-----------|--------|
| C2-1 | kernel-bind substring → actionable | PASS (sub-test green) |
| C2-2 | allocator substring → actionable | PASS (sub-test green) |
| C2-3 | generic wrap path still fires for non-port errors | PASS (`UpRunFailsWithoutRollback` green; sentinel does not contain either substring) |
| C2-4 | IPv6 example reframed as substring inside stderr | PASS |
| C2-5 | ports example matches code byte-for-byte | PASS (grep -F confirms) |
| C2-6 | `caddy up --help` includes volume-retention + dual-stack | PASS (rendered) |
| C2-7 | `caddy down --help` includes ingress + LE rate-limit warnings | PASS (rendered) |
| C2-8 | `m1x-backlog.md` item #6 exists | PASS (grep -c == 1) |
| C2-9 | `gofmt -l .` empty | PASS |
| C2-10 | `go test ./... -count=1` green | PASS |
| C2-11 | `go generate ./...` produces no diff | not re-run; no `//go:generate` was added or modified, no interface change |

C2-11 not separately re-verified by Kevlin since no interface or directive changed in this cycle, but Rob's §"Verification" log covered it.

## 8. Disease vs. symptom check (Kevlin discipline)

- **Architecture:** detection lives in the manager, not the driver — domain semantics stay where they belong. Joel §1.2 reasoning is sound.
- **Duplication:** none introduced. The substring detection is a single-use helper; the Long text is two strings, one per command, below rule-of-three for extraction.
- **Existing helpers:** `newManagerHarness`, `absentInspect`, `expectedCaddyRunOptions` reused. No reinvention.
- **File location:** new test added to existing `manager_test.go`, in the obvious neighbourhood of `TestManager_UpRunFailsWithoutRollback`. Correct.
- **Comments:** the helper's doc comment explains WHY (brittleness, locked by test), not WHAT. The actionable error message is itself the documentation — operators read the error and know what to do.

## 9. Verdict

**APPROVED** — cycle-2 ready to leave EXECUTION.

Code is honest about its brittleness, names it in a comment, and locks it with a test. The doc edits track the code byte-for-byte. The Long text matches Joel's spec verbatim. The backlog item respects the file's voice. Nothing to revise; on to PLAN-loop with Don, Joel, and Linus.

— Kevlin
