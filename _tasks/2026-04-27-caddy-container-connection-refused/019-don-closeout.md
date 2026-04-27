# 019 — Don's Cycle-2 Closeout (final PLAN-loop verdict)

Author: Don Melton (planning agent)
Date: 2026-04-27
Status: **FULLY DONE.** Cycle-2 EXECUTION closed every item from `012-don-final-review.md`. Task advances to FINALIZATION (Ward + Andy).

## 0. How I closed this out

I read everything the prompt required: my own cycle-2 list (012),
Rob's impl (016), Raymond's docs (017), Kevlin's review (018-kevlin),
Linus's review (018-linus). Then I verified each of the five items
against the actual files on disk — `internal/caddy/manager.go`,
`internal/caddy/manager_test.go`, `internal/cli/caddy_up.go`,
`internal/cli/caddy_down.go`, `_docs/install.md` lines 168-200, and
`_ai/m1x-backlog.md`. I re-ran `go test ./... -count=1`: green across
all packages (`internal/caddy 0.016s`, `internal/cli 0.023s`,
`internal/deploy 12.078s`, etc.).

This is the brutally-honest sign-off. No item gets a free pass.

## 1. Item-by-item: hit or miss

### Item 1 — Joel §1.5 ports substring detection — **HIT**

`internal/caddy/manager.go:96-98` carries the new actionable branch.
`isPortsBoundErr` lives at `manager.go:153-157` with both canonical
Docker substrings (`"address already in use"` and
`"port is already allocated"`). Brittleness comment present and
honest. Helper is package-private and co-located with its single
caller — exactly Joel §1.2 / Linus 014 §2.

The actionable branch wraps `ErrCaddyUp` with `%w` only, deliberately
suppressing the inner driver chain to avoid the doubled
`docker run: docker run:` rendering. Kent's
`NotContains(": docker run: docker run:")` assertion at
`manager_test.go:205` locks that branch-choice. Sentinel preserved on
both branches; `errors.Is(err, caddy.ErrCaddyUp)` continues to flow
through `exit_codes.go:58-59` to `ExitRunFail` (40).

`TestManager_UpRunFailsWithoutRollback` still passes with its
`errors.New("port allocation failed")` sentinel — neither canonical
substring matches it, so the generic-wrap branch genuinely fires for
non-port-conflict failures. No test was deleted, none weakened.

### Item 2 — `_docs/install.md:189` IPv6 reword — **HIT**

`_docs/install.md:188-196`. The fabricated cycle-1 example
(`caddy up: docker run: listen tcp [::]:80: ...`) is gone. New
framing: "fails with stderr containing `listen tcp [::]:80: socket:
address family not supported by protocol`. The raw `docker run`
stderr is surfaced as-is; it typically reads similar to: ..." with
ellipses inside the fenced block to signal variable surrounding
daemon prose. This is honest about substring-match semantics. No
hallucination.

Raymond deviated from Joel §2.1's specific prose to follow the
user's EXECUTION 3.3 guidance — that's an improvement, not a defect,
and Linus 018 §4.2 explicitly endorses it.

### Item 3 — `_docs/install.md:173` ports-bound example match — **HIT**

`_docs/install.md:174-176` shows the exact rendered string:

```
caddy: up failed: ports 80/443 already in use; if you ran the M1.0
install, run 'systemctl disable --now caddy && systemctl mask caddy'
or 'apt-get remove -y caddy' to make the change persistent
```

I verified byte-for-byte against `manager.go:97`. The `caddy: up
failed:` prefix composes from `ErrCaddyUp.Error()` + `%w` separator
correctly. `grep -F` of the recovery substring matches both the doc
and the source. CLAUDE.md hallucination-check discipline satisfied.

### Item 4 — `Long` help text for `caddy up`/`caddy down` — **HIT**

`internal/cli/caddy_up.go:14-28` and `internal/cli/caddy_down.go:14-27`.
Both `Long` fields populated with three paragraphs each per Joel
§4.1/§4.2.

`caddy up --help` carries: dual-stack publishing description (80/tcp,
443/tcp, 443/udp on both 0.0.0.0 and [::]); image and named-volume
names; volume-retention warning with "wipe manually only if you intend
to discard ACME state" framing; idempotency contract.

`caddy down --help` carries: ingress-interruption warning with
capital `ALL` for severity; named-volume retention with both volume
names spelled correctly; LE rate-limit warning ("forces fresh Let's
Encrypt issuance and risks tripping LE rate limits") — the operator-
value bit Linus 014 §4 specifically called out; idempotency contract.

`Args: cobra.NoArgs` retained. `RunE` bodies unchanged. No flag
surface drift. No new tests required (Cobra renders `Long`
independently; existing `NoFlags` regression guards still pass).

### Item 5 — `_ai/m1x-backlog.md` integration-test backlog item — **HIT**

`_ai/m1x-backlog.md` item #6 (lines 55-63) inserted between item #5
and the `## Maintenance note` section. Format matches items #1-#5
exactly: four bold-prefixed sections (Where / Why deferred / Fix
shape / Originator). Cross-references to
`_ai/decisions/m1-test-strategy.md` and
`_ai/decisions/caddy-runs-in-container.md` both check out. The
"deferred from the caddy-container-connection-refused task per
`_ai/decisions/m1-test-strategy.md`" phrase appears verbatim in **Why
deferred**. Originator naming convention (path + agent + section)
matches item #1's style.

One small future-Don note (Linus 018 §5 already flagged this): the
heading says "Docker-compose-based" while the **Fix shape** describes
a Go `integration_test.go` build-tagged with `//go:build integration`.
Reconcile when picking the item up in M2. Not a cycle-2 blocker.

## 2. Did any new gap appear?

**No.**

I checked the obvious places a cycle-2 change could have introduced
a new defect:

- **Test surface.** Cycle-2 added one test (two sub-tests) and
  modified zero. All packages pass `-count=1`. No flake risk
  introduced; `Times(0)` rollback assertions on the existing
  `UpRunFailsWithoutRollback` continue to bite.
- **Code surface.** Cycle-2 added 11 lines of Go (1 import, 4-line
  branch, 6-line helper-with-comment) and ~30 lines of Cobra `Long`
  strings. No functions deleted, no signatures changed. Driver
  surface untouched. Cycle-1 contracts intact.
- **Error API.** `errors.Is(err, ErrCaddyUp)` holds on the new branch
  (verified by Linus's standalone harness in 018-linus §0 and by Kent's
  test on `manager_test.go:201`). Exit-code mapping unchanged.
- **Docs surface.** Both reworded blocks shorter and more honest than
  the cycle-1 versions. Backlog file grows by one entry that respects
  the existing voice.
- **No new `"decloud"` literal sneaked in.** New code uses
  `caddy.NetworkName`. The four pre-existing literals (Joel §5
  scoped to M1.x backlog) remain — intentional, not in scope.

The two non-blocking observations Linus carried forward (207-char
error string verbosity; m1x-backlog #6 heading vs fix-shape mismatch)
are both M2 material. Neither is a defect on the bug-fix path.

## 3. Acceptance criteria roll-up (v2 §11)

Now-final state of the 19 criteria:

- **HIT** (operator-independent): 1, 2, 3, 4, 5, 6, 7, 12, 13, 14,
  15, 16, 17, 18 — all closed.
- **PENDING** (operator-gated by design): 8, 9, 10, 11, 19. The user
  runs §7 manual verification on their host. That gate is intentional;
  we don't unilaterally close it.

Every criterion under our control is HIT. The five operator-gated
items remain operator-gated — that was the plan.

## 4. Why FULLY DONE, not cycle-3

The cycle-2 changes are:

- Surgical (~40 lines of Go + 3 doc edits + 1 backlog entry).
- Tested (two new sub-tests, both green; full suite green).
- Reviewed and approved by both Kevlin (018-kevlin) and Linus
  (018-linus). Independent verification, both on the source.
- Self-consistent (the error string the operator sees matches the
  doc the operator reads, byte-for-byte).
- Coherent end-to-end (M1.0→M1.1 migration story walks cleanly:
  bind fails → actionable error names recovery → operator runs
  recovery → retry succeeds; verified by Linus 018 §6).

There is nothing materially broken. The two non-blocking observations
Linus listed are M2 material and should not spawn a cycle-3 — that's
exactly the "try really hard not to spawn cycle-3 unless something is
materially broken" discipline the prompt asked for.

The plan-vs-ship gap is closed. The bug is fixed at the architectural
layer (Caddy in-container on the `decloud` bridge, no IPv6
fallthrough). The shipping standard is met.

## 5. Verdict

**FULLY DONE.** Advance to FINALIZATION:

1. **Ward** captures learnings — three substantive ones I'd flag for
   his attention:
   - "Driver wrap shape feeds substring detection in higher layers"
     pattern (manager.go's `isPortsBoundErr` reading through
     `cli_driver.go`'s `; stderr=%q` wrap). Worth documenting as a
     reusable pattern alongside the `isNotRunningStderr` precedent
     in `reloader.go`.
   - The architectural decision: `_ai/decisions/caddy-runs-in-container.md`
     is the durable record of why we moved Caddy off the host. Ward
     should sanity-check the file is complete and cross-linked from
     wherever new contributors might land.
   - The "doc shows the literal error string" discipline: install.md
     now matches manager.go byte-for-byte. That's a pattern worth
     making explicit for future doc writers — fabricated error
     examples are forbidden; show the real string or describe it as
     a substring inside variable surrounding text.

2. **Andy** considers agent-instruction tuning. The user nudged
   Raymond's IPv6 prose mid-cycle; that's a signal worth Andy
   examining — not necessarily a change, but a consideration.

— Don
