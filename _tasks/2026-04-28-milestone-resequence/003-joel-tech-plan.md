# 003 — Joel's tech plan: milestone resequence

## TL;DR

Don's plan is sound. I expand it here with verbatim before/after substitutions for every edit, resolve his five open questions, and surface one important reversal: **this is no longer a docs-only task.** Three user-visible strings in Go source say "M3" today and must become "M2"; per Don's locked-in contingency clause (his §"Workflow", final paragraph), Kent and Rob get re-added to the workflow.

The new sequence (locked from Don's plan):

```
M1 service deploy MVP                                   [SHIPPED]
M2 server-side --mount + env-file hardening            [was M3a, minus secret files]
M3 host bootstrap + Viper + caddy.image config         [was M2]
M4 zero-downtime blue/green via Caddy admin API        [unchanged]
M5 jobs (systemd timers)                                [unchanged]
M6 backups + image GC                                   [unchanged]
M7 secret files + client binary + operational polish   [absorbs former M3b + secret files]
```

---

## Section A: Resolved open questions

### A.1 Runtime error string grep — code IS affected

I searched `internal/` for any literal `M2`/`M3`/`M3a`/`M3b` token in Go source. Results (verbatim, with file:line):

```
internal/cli/deploy_service.go:61:    cmd.Flags().StringSliceVar(&f.Mounts, "mount", nil, "M1: rejected with ExitConfigError (M3 only)")
internal/cli/deploy_service.go:72:        return fmt.Errorf("--mount is not supported until M3: %w", registry.ErrMountsNotSupported)
internal/registry/store.go:69:        return nil, fmt.Errorf("%w: service %q declares %d mount(s) in %s; mounts are not supported until M3",
internal/ids/ids.go:20:// M1 format: "decloud-<name>". M4 changes to include a deploy-id suffix —
internal/ids/ids.go:21:// route all naming through this helper so the M4 migration touches one
internal/caddy/manager.go:97:        return fmt.Errorf("%w: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent", ErrCaddyUp)
```

Triage:

- **`deploy_service.go:61`** — flag-help text shown by `decloud deploy service --help`. **User-visible. Must change.**
- **`deploy_service.go:72`** — error message returned when `--mount` is passed in M1. **User-visible. Must change.**
- **`store.go:69`** — error message returned when a config TOML hand-edited to include `Mounts` is loaded. **User-visible. Must change.**
- **`ids.go:20-21`** — comment naming M1 and M4. M4 stays M4; no change.
- **`manager.go:97`** — names `M1.0` (the historical pre-Caddy-container install). M1.0 is a fixed point in history; no change.

**Test-side audit.** I grepped `internal/.../*_test.go` for tests asserting on the literal "M3" substring. Result: **zero hits**. Existing tests (`store_test.go:296`, `deploy_service_test.go:81-90`, `exit_codes_test.go:24`) all assert via `errors.Is(err, registry.ErrMountsNotSupported)` and never compare against the wrapped error text. So renaming the wrapped text is **safe under existing tests** — no test breaks.

**However**, Kent should ADD one new assertion per call site that locks the new "M2" string into place. Without it, the next docs-grep audit comparing `_docs/usage.md:71` ("Persistent volumes are M2") to source bytes is back to the pre-M1-iter2 state — same class of bug `_ai/doc-grep-discipline.md` exists to prevent. New tests are described in §C.4 below.

**Workflow consequence:** Per Don's §"Workflow" paragraph 5 ("If during Raymond's pass we discover that a docs change actually requires a code change..., Kent and Rob get re-added to the workflow"), this triggers re-add. The execution order becomes:

1. Kent — write the three new "lock the M2 wording" assertions.
2. Rob — change the three Go strings; verify Kent's tests pass.
3. Raymond — execute the docs edits (this is the bulk of the task).
4. Kevlin + Linus — review in parallel.
5. PLAN re-entry per CLAUDE.md.

### A.2 Wording for `_ai/m1x-backlog.md` item 6

Don listed two candidates:

- (a) "M2-new (mounts)" — explicit, but couples the backlog file to the resequence label.
- (b) "the next post-M1 milestone where we touch real Docker for the first time" — name-agnostic.

I pick (b) — name-agnostic. The backlog file is read months from now; the "M2-new" / "M2-old" disambiguator will have decayed by then. Don leans (b) too. Locked.

Exact substitution in §B.8 below.

### A.3 MEMORY.md placement of the schema-versioning entry

Don's question: where does the schema-versioning index entry's milestone reference live, and does the resequence force a move?

Answer: the entry lives at `_ai/MEMORY.md:9`. Verified verbatim:

> `decisions/schema-versioning.md` — `pelletier/go-toml/v2` strict mode + `schema_version` integer; bump only on semantic breaks, never preemptively; M1/M3 both write version 1.

Don's edit (his §7) just changes `M1/M3` to `M1/M2/M7`. The line stays where it is — this is the "Architecture decisions" section and that's the right home for it. No move required.

### A.4 Add a separate `_ai/decisions/milestone-resequence-2026-04.md`?

**Decide: NO — argue against, override Don.** I take Don's lean here.

Reasons:

1. The architectural rationale already lives in two places: `m1-scope.md`'s appended sentence at line 34 (per Don's §1 edit) points at `_tasks/2026-04-28-milestone-resequence/`, and `_ai/MEMORY.md`'s "Source-of-truth task artefacts" section gets the cross-reference one-liner (Don's §11). The task directory itself contains `001-user-request.md` (the "why"), `002-don-plan.md` (the "what"), and this file (the "how"). That's three independent breadcrumbs.

2. `_ai/decisions/` is for *enduring architectural choices the codebase depends on* (schema shape, container naming, Caddy network model). A milestone resequence is a *roadmap* decision — it changes which milestone delivers a given feature, but says nothing about how that feature is shaped. Milestone resequencing is precisely the kind of thing that happens repeatedly during a project's life. If we add a decision file every time, `_ai/decisions/` becomes a journal of priority changes rather than an architecture record.

3. The cost of being wrong: future-Don finds a stale resequence file and has to figure out whether subsequent resequences invalidate it. The cost of NOT having it: future-Don greps `_tasks/` for "resequence" and finds this directory — three files, all linked. The grep cost is lower than the maintenance cost.

4. Counter-evidence Don considered: "future-Don reading MEMORY.md's decisions list shouldn't have to dig into `_tasks/` to see the rationale." Counter-counter: future-Don reading MEMORY.md sees `m1-scope.md` listed under "Architecture decisions" and follows that link; `m1-scope.md`'s appended sentence (Don's §1 edit) carries the resequence pointer. Two clicks to rationale, no separate file needed.

**Plan as locked: do not create `_ai/decisions/milestone-resequence-2026-04.md`.** If Linus disagrees, this is the only open question I expect pushback on; Don's "lean against" plus my analysis above is my position.

### A.5 Verbosity of `_ai/decisions/m1-test-strategy.md` footnote

The file's M2 references are:

- Line 7: "That smoke-test is M2's first feedback signal, not an M1 deliverable."
- Line 49: "When the maintainer reports a real-system failure, the unit-test gap that allowed it through becomes M2's first priority."

The substantive content the file communicates is "M1 ships unit-tests-only; the manual smoke-test the maintainer runs after M1 is the bridge." That truth survives the resequence: it doesn't matter what the next milestone is *called* — the maintainer's first-after-M1 smoke test is what it is. The new M2 (mounts) actually exercises *more* of Docker than the old M2 (bootstrap, which is mostly `apt install`), so the smoke-test framing is even better-fit now.

**Verbosity decision: terse.** A single-token rename is sufficient. Don's proposed full-paragraph footnote ("per the 2026-04-28 resequence: server-side mounts...") in his §5 first bullet is too verbose for what is fundamentally a label rename. Keep the footnote pointer at `m1-scope.md` (the canonical record), don't duplicate it here.

Exact substitution in §B.5 below.

---

## Section B: Verbatim before/after substitutions

Each item below specifies a single Edit-tool call. **I have verified each `old_string` against the current file**; line numbers in Don's plan have NOT drifted.

Files are listed in **dependency order** (canonical roadmap first, then files that quote it, then user-facing docs, then operator-facing docs, then cross-references). Raymond MUST execute in this order — if a downstream file disagrees with the canonical roadmap mid-task, reviewers see noise.

### B.1 — `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md`

**The canonical roadmap. Single most important file in this task.** Six separate Edit calls. Each `old_string` is verbatim from my read of the file.

#### B.1.1 — Line 8 (bootstrap milestone label)

`old_string`:
```
- **NOT "host bootstrap first"** — bootstrap is `apt install docker caddy && systemctl enable decloud.service`; five lines of substance, exercises none of the design's hard parts. M2.
```

`new_string`:
```
- **NOT "host bootstrap first"** — bootstrap is `apt install docker caddy && systemctl enable decloud.service`; five lines of substance, exercises none of the design's hard parts. M3.
```

#### B.1.2 — Line 13 (client binary milestone label)

`old_string`:
```
- **No client binary** — operator SSHes in. README explicitly says server CLI is "equally usable by a human SSH'd in." Client is M3b.
```

`new_string`:
```
- **No client binary** — operator SSHes in. README explicitly says server CLI is "equally usable by a human SSH'd in." Client is M7.
```

#### B.1.3 — Line 15 (jobs/backups/image-GC/bootstrap labels)

`old_string`:
```
- **No jobs** / **No backups** / **No image GC** / **No bootstrap script** — M5/M6/M6/M2 respectively.
```

`new_string`:
```
- **No jobs** / **No backups** / **No image GC** / **No bootstrap script** — M5/M6/M6/M3 respectively.
```

#### B.1.4 — Line 16 (mount-rejection milestone label)

`old_string`:
```
- **No `--mount`** — flag rejected; loader also rejects non-empty `Mounts` (closes hand-edit loophole). M3a.
```

`new_string`:
```
- **No `--mount`** — flag rejected; loader also rejects non-empty `Mounts` (closes hand-edit loophole). M2.
```

#### B.1.5 — Line 18 (Viper-introduction milestone label)

`old_string`:
```
- **No Viper** — plain Cobra + `os.Getenv("DECLOUD_ROOT")` is three lines. M2 introduces Viper when there's a real `/etc/decloud/config.toml` to read.
```

`new_string`:
```
- **No Viper** — plain Cobra + `os.Getenv("DECLOUD_ROOT")` is three lines. M3 introduces Viper when there's a real `/etc/decloud/config.toml` to read.
```

#### B.1.6 — Line 32 (the canonical sequence) AND line 34 (Linus-approval pointer with appended sentence)

These two lines are best edited as a single Edit call (they're consecutive after the heading, and the new content folds them together):

`old_string`:
```
M1 service deploy MVP → M2 host bootstrap (introduces Viper) → M3a server-side mounts/secret-files/env hardening + M3b client binary → M4 zero-downtime blue/green via Caddy admin API → M5 jobs (systemd timers) → M6 backups + image GC → M7 operational polish (supervisor, deploy locks, etc.).

Don't reopen this sequencing without a concrete reason. Linus approved the bones in `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md`.
```

`new_string`:
```
M1 service deploy MVP → M2 server-side mounts + env-file hardening (`--mount` flag, loader populates `Mounts`) → M3 host bootstrap (introduces Viper, `caddy.image` config knob) → M4 zero-downtime blue/green via Caddy admin API → M5 jobs (systemd timers) → M6 backups + image GC → M7 operational polish (supervisor, deploy locks, secret-files-on-disk, client binary, etc.).

Don't reopen this sequencing without a concrete reason. Linus approved the bones in `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md`. The 2026-04-28 resequence (`_tasks/2026-04-28-milestone-resequence/`) re-ordered M2/M3 and split former M3a/M3b across M2/M7 per maintainer priority; Linus's approval of the original bones still applies to M1's content, which is unchanged.
```

### B.2 — `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md`

Two Edit calls.

#### B.2.1 — Lines 10–11 (the rule)

`old_string`:
```
- M1 writes `schema_version = 1`.
- M3 writes `schema_version = 1`. M3 only populates fields that M1 reserved (`Mounts`, future secret-file declarations under `mounts`); the schema *shape* doesn't change.
```

`new_string`:
```
- M1 writes `schema_version = 1`.
- M2 writes `schema_version = 1`. M2 populates `Mounts`. M7 (secret-files-on-disk) also writes `schema_version = 1` and populates the secret-file substructure under `mounts`. The schema *shape* doesn't change between any of these milestones.
```

#### B.2.2 — Line 16 (the "what reserve fields means" paragraph)

`old_string`:
```
M1 declares the full schema shape — including fields M1 won't populate (`Mounts` always empty in M1). M1's loader rejects non-empty `Mounts` with the same `ErrMountsNotSupported` as the CLI's `--mount` flag (closes the hand-edit loophole). M3 starts populating; no file rewrite, no migration code. An M1-era TOML loads cleanly in an M3 binary because the shape is identical, only the values differ.
```

`new_string`:
```
M1 declares the full schema shape — including fields M1 won't populate (`Mounts` always empty in M1). M1's loader rejects non-empty `Mounts` with the same `ErrMountsNotSupported` as the CLI's `--mount` flag (closes the hand-edit loophole). M2 starts populating `Mounts`; no file rewrite, no migration code. An M1-era TOML loads cleanly in an M2 binary because the shape is identical, only the values differ. M7 extends populating to secret-file declarations on the same shape.
```

**Lines 20 and the rest are unchanged** — the escalation rule is M1-specific and survives.

### B.3 — `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md`

One Edit call.

#### B.3.1 — Line 6 (secret-files-on-disk milestone label)

`old_string`:
```
- `/opt/decloud/secrets/<name>/env.toml` — root:root, mode **0600**, in a directory at mode **0700**. Holds `schema_version` (must match config) + `env` map (the `env.sh` capture). M3 will add `secrets/<name>/files/` for secret file contents.
```

`new_string`:
```
- `/opt/decloud/secrets/<name>/env.toml` — root:root, mode **0600**, in a directory at mode **0700**. Holds `schema_version` (must match config) + `env` map (the `env.sh` capture). M7 will add `secrets/<name>/files/` for secret file contents (originally planned for M3, deferred per maintainer priority — see `_tasks/2026-04-28-milestone-resequence/`).
```

**Lines 24 and 29 are unchanged.** Per Don's analysis (his §3 second and third bullets), `ErrMountsNotSupported (M1)` and the rejected-alternative C reference at line 29 don't depend on whether secret-files lives at M3 or M7. Verified.

### B.4 — `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md`

Two Edit calls.

#### B.4.1 — Line 15 (Viper-introduction milestone)

`old_string`:
```
Image: `caddy:2`, hardcoded as `caddy.DefaultImage`. No flag, no env var, no TOML override in M1 — that comes when M2 introduces Viper and a real config file. Operators who need a pinned tag can `docker pull caddy:2.7.6 && docker tag caddy:2.7.6 caddy:2`.
```

`new_string`:
```
Image: `caddy:2`, hardcoded as `caddy.DefaultImage`. No flag, no env var, no TOML override in M1 — that comes when M3 introduces Viper and a real config file. Operators who need a pinned tag can `docker pull caddy:2.7.6 && docker tag caddy:2.7.6 caddy:2`.
```

#### B.4.2 — Line 58 (forward-looking notes)

`old_string`:
```
- **Pinning the image tag.** When M2 introduces Viper, the obvious config knob is `caddy.image = "caddy:2.7.6"`. The `DefaultImage` constant becomes the fallback.
```

`new_string`:
```
- **Pinning the image tag.** When M3 introduces Viper, the obvious config knob is `caddy.image = "caddy:2.7.6"`. The `DefaultImage` constant becomes the fallback.
```

**Line 53 ("Concurrent deploys (theoretical, M2+).")** is borderline — "M2+" means "M2 onwards", a milestone-range bound rather than a content claim. Under the new sequence, "M2+" still means "the milestone after M1" — M2-new (mounts) is single-operator just like M2-old was, so the concurrency claim survives unchanged. **Leave line 53 as-is.** Flagging this so reviewers know I considered it.

### B.5 — `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md`

Two Edit calls. Per §A.5, I'm choosing the terse rendering.

#### B.5.1 — Line 7 (smoke-test feedback signal)

`old_string`:
```
The maintainer's instruction was explicit: "I will test it on a real system after M1 is done." That moves every `-tags integration` test out of M1 execution scope. Joel's tech-plan §12.2 listed `internal/dockerdrv/integration_test.go`, `internal/caddy/integration_test.go`, and `internal/deploy/integration_test.go`; none of those files exist in the M1 tree. Their replacement is the manual smoke-test the maintainer will run on a real Linux host once the binary lands. That smoke-test is M2's first feedback signal, not an M1 deliverable.
```

`new_string`:
```
The maintainer's instruction was explicit: "I will test it on a real system after M1 is done." That moves every `-tags integration` test out of M1 execution scope. Joel's tech-plan §12.2 listed `internal/dockerdrv/integration_test.go`, `internal/caddy/integration_test.go`, and `internal/deploy/integration_test.go`; none of those files exist in the M1 tree. Their replacement is the manual smoke-test the maintainer will run on a real Linux host once the binary lands. That smoke-test is the next milestone's first feedback signal, not an M1 deliverable.
```

#### B.5.2 — Line 49 (real-system-failure priority)

`old_string`:
```
When the maintainer reports a real-system failure, the unit-test gap that allowed it through becomes M2's first priority. Add the missing test, then add the integration test that would have caught it earlier, then ship the fix.
```

`new_string`:
```
When the maintainer reports a real-system failure, the unit-test gap that allowed it through becomes the next milestone's first priority. Add the missing test, then add the integration test that would have caught it earlier, then ship the fix.
```

**Why "the next milestone's" instead of "M2-new":** the file's substantive claim is "we ship unit-tests-only; the manual smoke-test bridges to real-system reality." That claim is milestone-numbering-agnostic. Using "the next milestone's" makes the file robust to *future* resequences (which will happen — see §A.4 above for why we don't write a decision file every time).

### B.6 — `/Users/fenster/dev/decloud/_ai/container-naming.md`

**Zero edits.**

I read the file end to end (file has 14 lines total). The only M2/M3 reference is line 14:

> If you write code in M1–M3 that hard-codes `decloud-<name>` (Caddy `reverse_proxy` directive, stop/remove logic, status lookup), that code MUST be touched in M4.

This is a milestone-range bound: "any milestone before blue/green lands." Under the new sequence, M2-new (mounts) and M3-new (bootstrap) both still ship the `recreate` strategy with `decloud-<name>` containers — blue/green is M4 in both old and new sequences, so the M1–M3 range still bounds the right set of milestones. **Don's audit-note (his §6) is correct and I confirm it: line 14 needs no edit.**

The "M4 boundary" claim Don asked me to verify (his §"What does NOT change" item 2) holds. Verified by reading lines 1–14: M4 is the rename milestone, M4 stays at M4 in the new sequence, the migration-as-tracked-deliverable obligation is unchanged.

### B.7 — `/Users/fenster/dev/decloud/_ai/MEMORY.md`

Two Edit calls.

#### B.7.1 — Line 9 (schema-versioning index entry)

`old_string`:
```
- `decisions/schema-versioning.md` — `pelletier/go-toml/v2` strict mode + `schema_version` integer; bump only on semantic breaks, never preemptively; M1/M3 both write version 1.
```

`new_string`:
```
- `decisions/schema-versioning.md` — `pelletier/go-toml/v2` strict mode + `schema_version` integer; bump only on semantic breaks, never preemptively; M1/M2/M7 all write version 1 (mounts populate at M2, secret-files at M7).
```

#### B.7.2 — End of "Source-of-truth task artefacts" section (insert new bullet)

The existing bullets end at line 56. Insert a new bullet at the end of the list. Since the previous line is the most recent `_tasks/` entry, the safest unique-text Edit is to append after that last bullet.

`old_string`:
```
- `_tasks/2026-04-27-caddy-container-connection-refused/004-linus-review.md` — enumeration of the seven rejected alternatives (`host.docker.internal`, `--network host`, `--network container:`, sidecar, `/etc/hosts` injection, `--resolvers 127.0.0.11`, host-local `dnsmasq`).
```

`new_string`:
```
- `_tasks/2026-04-27-caddy-container-connection-refused/004-linus-review.md` — enumeration of the seven rejected alternatives (`host.docker.internal`, `--network host`, `--network container:`, sidecar, `/etc/hosts` injection, `--resolvers 127.0.0.11`, host-local `dnsmasq`).
- `_tasks/2026-04-28-milestone-resequence/` — 2026-04-28 maintainer-priority resequence: M2/M3 swap, M3b client deferred to M7, secret-files-on-disk deferred to M7. Doesn't change M1, M4, M5, M6 in content.
```

Don's §11 placed this in "Source-of-truth task artefacts" (correct — it's a `_tasks/` reference). I confirm.

### B.8 — `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`

One Edit call. Per §A.2, I picked the name-agnostic phrasing.

#### B.8.1 — Lines 61 (item 6, "Fix shape" paragraph, end)

`old_string`:
```
**Fix shape:** New `integration_test.go` build-tagged with `//go:build integration`, requires `DECLOUD_INTEGRATION=1` to run, brings up Caddy, deploys a one-line nginx service, curls through Caddy, asserts 200 OK with nginx body. Tear down both containers and the network on completion. Cleanup must be idempotent (test failures must not leave dangling containers). M2 material; M2 is also the milestone where reloader stderr `%q` quoting gets revisited, so the integration test naturally covers that improvement too.
```

`new_string`:
```
**Fix shape:** New `integration_test.go` build-tagged with `//go:build integration`, requires `DECLOUD_INTEGRATION=1` to run, brings up Caddy, deploys a one-line nginx service, curls through Caddy, asserts 200 OK with nginx body. Tear down both containers and the network on completion. Cleanup must be idempotent (test failures must not leave dangling containers). Belongs to the next post-M1 milestone where we touch real Docker for the first time (the new M2 — server-side `--mount` — per the 2026-04-28 resequence); that milestone is also the natural home for revisiting reloader stderr `%q` quoting, so the integration test can cover both improvements.
```

**Reasoning for keeping a brief resequence pointer here:** unlike `m1-test-strategy.md` (where the substantive claim is milestone-agnostic), this backlog entry concretely names a milestone for a deliverable. The reader benefits from knowing which milestone is meant. The phrase "the next post-M1 milestone where we touch real Docker for the first time" carries the architectural justification (Don's argument from his §"What does NOT change" item 5: M2-new actually exercises Docker volume semantics, M2-old didn't) and the parenthetical resequence pointer ties it to today's roadmap.

### B.9 — `/Users/fenster/dev/decloud/_docs/install.md`

**One Edit call. This is the pre-existing-bug fix-while-fresh.**

#### B.9.1 — Line 121 (state/deploys/ population milestone)

I verified the current text by reading the file. Don's quote is exact.

`old_string`:
```
`state/deploys/` is created here but no M1 code populates it. M2 will write source bundles there for backup.
```

`new_string`:
```
`state/deploys/` is created here but no M1 code populates it. M6 will write source bundles there for backup.
```

**My justification (Don asked me to pick "M6" vs "future milestones" and explain in two sentences):** Pick **"M6"**. The original docs were wrong about *which* milestone owns this, not wrong that *some* milestone owns it; backups have always been M6 per `m1-scope.md:32` (verified — old sequence and new sequence both put backups at M6). Naming the specific milestone keeps `install.md` coupled to the canonical roadmap and makes the next docs-grep audit catch any future drift; "future milestones will write..." would lose that signal and re-introduce the same vagueness that let the M2-vs-M6 mismatch persist for the original M1 ship.

### B.10 — `/Users/fenster/dev/decloud/_docs/usage.md`

One Edit call.

#### B.10.1 — Line 71 (--mount table row)

`old_string`:
```
| `--mount` | string (repeatable) | none | no | Rejected with exit 10 in M1. Persistent volumes are M3. |
```

`new_string`:
```
| `--mount` | string (repeatable) | none | no | Rejected with exit 10 in M1. Persistent volumes are M2. |
```

**Coupling note:** This doc says "M2" — operators expect the `--mount` rejection to lift in M2. The Go runtime error string at `internal/cli/deploy_service.go:72` currently says "M3"; **§C below covers the source-side fix that keeps these in sync.** If Raymond runs B.10 without §C, the doc and binary diverge — exactly the doc-fab class `_ai/doc-grep-discipline.md` warns about. Hence the dependency ordering in §D below.

### B.11 — Source code (NEW — Rob's responsibility, NOT Raymond's)

Per §A.1, three user-visible Go strings must change. Rob does this AFTER Kent's tests land.

#### B.11.1 — `/Users/fenster/dev/decloud/internal/cli/deploy_service.go:61` (flag help)

`old_string`:
```
	cmd.Flags().StringSliceVar(&f.Mounts, "mount", nil, "M1: rejected with ExitConfigError (M3 only)")
```

`new_string`:
```
	cmd.Flags().StringSliceVar(&f.Mounts, "mount", nil, "M1: rejected with ExitConfigError (M2 only)")
```

#### B.11.2 — `/Users/fenster/dev/decloud/internal/cli/deploy_service.go:72` (rejection error)

`old_string`:
```
		return fmt.Errorf("--mount is not supported until M3: %w", registry.ErrMountsNotSupported)
```

`new_string`:
```
		return fmt.Errorf("--mount is not supported until M2: %w", registry.ErrMountsNotSupported)
```

#### B.11.3 — `/Users/fenster/dev/decloud/internal/registry/store.go:69` (loader rejection error)

`old_string`:
```
		return nil, fmt.Errorf("%w: service %q declares %d mount(s) in %s; mounts are not supported until M3",
```

`new_string`:
```
		return nil, fmt.Errorf("%w: service %q declares %d mount(s) in %s; mounts are not supported until M2",
```

(Single-line edit; the format-string continuation on line 70 is unchanged.)

---

## Section C: New tests Kent must add

Per §A.1, no existing test breaks, but we are gaining three user-visible strings that we want locked. Without these tests, the next docs-grep audit re-discovers the same drift.

### C.1 — `internal/cli/deploy_service_test.go` (extend existing test)

The existing `TestDeployService_MountFlagReturnsErrMountsNotSupported` (line 81) asserts on the sentinel via `errors.Is`. Extend it with one new substring assertion against the wrapped text. New code, drop-in additive:

```go
// After the existing assert.True(t, errors.Is(err, registry.ErrMountsNotSupported)):
assert.Contains(t, err.Error(), "--mount is not supported until M2",
    "user-facing milestone label must match _docs/usage.md:71 and _ai/decisions/m1-scope.md")
```

The trailing message names the cross-reference so a future drifter sees the contract immediately.

### C.2 — `internal/registry/store_test.go` (extend existing test)

The `assert.ErrorIs(t, err, registry.ErrMountsNotSupported)` at line 296 lives inside a test that already loads a TOML with non-empty `Mounts`. Add one substring assertion immediately after the ErrorIs line:

```go
assert.Contains(t, err.Error(), "mounts are not supported until M2",
    "user-facing milestone label must match _docs/usage.md:71 and _ai/decisions/m1-scope.md")
```

### C.3 — Flag-help text test (NEW test, file `internal/cli/deploy_service_test.go`)

The flag-help text at `deploy_service.go:61` is currently not asserted in any test. The bug class is "the flag help drifts from the doc and from the error string." Add a small test that asserts the cobra command's flag help includes the `M2 only` token:

```go
func TestDeployService_MountFlagHelpReferencesM2(t *testing.T) {
    cmd := newDeployServiceCommand(&rootContext{}) // or whatever the existing constructor is
    flag := cmd.Flags().Lookup("mount")
    require.NotNil(t, flag)
    assert.Contains(t, flag.Usage, "M2 only",
        "flag help must match _docs/usage.md:71 and the runtime rejection error")
}
```

(Rob: if `newDeployServiceCommand` takes a different constructor signature in the existing tests, follow that pattern — the existing `TestDeployService_MountFlagReturnsErrMountsNotSupported` shows you how the command is built in tests today.)

### C.4 — Test naming and `_ai/cli-flag-surface-coherence.md`

`_ai/cli-flag-surface-coherence.md` already documents the four-surface contract (runtime check, error string, `--help` text, `_docs/usage.md`). The three tests above lock three of those four surfaces; the fourth (the doc) is locked by the file change at §B.10. **No new `_ai/` content is needed** — this task exercises the existing pattern, doesn't establish a new one.

If Linus presses on whether we should also add a doc-grep discipline test (i.e., a meta-test that reads `_docs/usage.md` and asserts the literal string appears in source), my answer is: NO, that's a change-detector test in disguise (CLAUDE.md §1.4), and the doc-grep discipline is a *review* discipline, not a test discipline. The four-surface lock is enough.

---

## Section D: Execution order for Raymond, Rob, and Kent

Dependency order (each step depends on the previous; do not reorder):

1. **Kent** writes the three new test additions (§C.1, §C.2, §C.3). These tests will FAIL today because the source still says "M3"; Kent commits them anyway in his test-author report. Kent's report explicitly notes the failures are expected and will pass once Rob lands §B.11.

2. **Rob** executes §B.11.1, §B.11.2, §B.11.3 (three Go-source edits). Runs `go test ./...` and confirms Kent's new tests now pass. Runs `gofmt -l .` (must be empty), `go vet ./...` (must be empty), `go generate ./...` followed by `git status --porcelain` (must show only this task's expected diffs).

3. **Raymond** executes the doc edits in this order:

   a. **§B.1 (`_ai/decisions/m1-scope.md`)** — six Edit calls in sub-order B.1.1 → B.1.6. The canonical roadmap is updated FIRST so every downstream reference is consistent with the source of truth. **All six are single-instance Edits; none need `replace_all`.**

   b. **§B.2 (`_ai/decisions/schema-versioning.md`)** — two Edit calls B.2.1, B.2.2. Single-instance.

   c. **§B.3 (`_ai/decisions/secrets-split.md`)** — one Edit call B.3.1. Single-instance.

   d. **§B.4 (`_ai/decisions/caddy-runs-in-container.md`)** — two Edit calls B.4.1, B.4.2. Single-instance.

   e. **§B.5 (`_ai/decisions/m1-test-strategy.md`)** — two Edit calls B.5.1, B.5.2. Single-instance.

   f. **§B.6 (`_ai/container-naming.md`)** — ZERO edits. Skip.

   g. **§B.7 (`_ai/MEMORY.md`)** — two Edit calls B.7.1, B.7.2. Single-instance.

   h. **§B.8 (`_ai/m1x-backlog.md`)** — one Edit call B.8.1. Single-instance.

   i. **§B.9 (`_docs/install.md`)** — one Edit call B.9.1 (the pre-existing-bug fix-while-fresh). Single-instance.

   j. **§B.10 (`_docs/usage.md`)** — one Edit call B.10.1. Single-instance.

4. **Kevlin and Linus** review in parallel. Per CLAUDE.md, Kevlin reviews docs for hallucinations very carefully — the substring assertions in §C.1, §C.2, §C.3 lock the bytes that `usage.md` and `m1-scope.md` reference, so the doc-grep discipline is mechanically satisfied; Kevlin's job is to spot any *new* claim that drifted in.

5. **PLAN re-entry** per CLAUDE.md: Don/Joel/Linus reconfirm done. Ward extracts learnings (one obvious one: the "fix-while-fresh" rule applies to milestone-reference audits, AND grepping source for milestone labels before declaring "docs-only" is now part of the workflow). Andy considers agent-definition tweaks — probably none, this task surfaced an existing contingency clause in Don's playbook that did its job.

**Replace-all summary:** ZERO of the edits in this plan need `replace_all`. Every `old_string` is unique within its file (verified by reading each file). Raymond uses single-instance Edit calls throughout.

---

## Section E: Cross-reference audit

After every substitution in §B and §B.11 lands, will any doc still reference an old milestone label in a way that contradicts the new sequence?

I grepped `_ai/`, `_docs/`, `README.md`, and Go source comments for `M[1-9][a-z]?` patterns. Here is the complete remaining-after-substitutions audit (every site I could find, including ones Don's plan didn't enumerate):

### E.1 — Sites that DON'T need changes (but I checked them)

| Site | Current text | Why no change |
|---|---|---|
| `internal/ids/ids.go:20-21` | `M1 format`, `M4 changes` | Comment names M1 and M4 only. M4 stays M4 in the new sequence. |
| `internal/caddy/manager.go:97` | `M1.0 install` | M1.0 is a historical version label, not a future milestone. |
| `_docs/install.md:3, 5, 57, 59, 125, 150, 175, 178, 196` | "M1" / "M1.0" references | All historical or current-state references; no milestone-label drift. |
| `_docs/usage.md:3, 5, 55, 65, 69, 110, 137, 201` | "M1" / "M4" / "M5" references | All historical or future-milestone-label references where the milestone number is unchanged. |
| `_docs/usage.md:69` | `blue_green` is rejected with exit 10 (M4) | M4 unchanged. |
| `_docs/usage.md:65` | Worker/job workloads ... are M5 | M5 unchanged. |
| `_ai/docker-bridge-dns.md:5, 13, 17, 19` | "M1" / "M1.0" references | Historical; unchanged. |
| `_ai/stderr-substring-canary.md:20` | "Not a concern in M1" | M1-specific; unchanged. |
| `_ai/cobra-init-pattern.md:27` | "every M1 path" | M1-specific; unchanged. |
| `_ai/decisions/no-magic-zero-modes.md:3, 5, 11` | "M5 milestone" | M5 unchanged. |
| `_ai/decisions/caddy-runs-in-container.md:7, 9, 33, 47, 48, 49, 53, 57` | M1 / M1.0 / M4 / "M2+" | M1, M1.0, M4 unchanged; "M2+" range bound survives (see §B.4 note). |
| `_ai/decisions/m1-test-strategy.md` (other lines) | "M1" references | M1-specific; unchanged. |
| `_ai/container-naming.md` (lines 1, 3, 5, 6, 8, 10, 14) | M1 / M4 references and "M1–M3" range | All milestone-range bounds or M1/M4-specific; survive (see §B.6). |
| `_ai/m1x-backlog.md` (lines 1, 3, 55) | "M1.x backlog" / "M1 deploy + Caddy ingress" | Historical M1 references; unchanged. |
| `_ai/gomock-inorder-sequencing.md:35` | "BlueGreen in M4, Backup in M6" | M4, M6 unchanged. |
| `_ai/MEMORY.md:7, 10, 11, 31, 32, 36, 46, 52, 53` | M1 / M4 / M5 references | All M1/M4/M5-specific; unchanged. |
| `_ai/optional-input-two-layer.md:3` | "landed in M1" | Historical; unchanged. |
| `_ai/doc-grep-discipline.md:5` | "Two M1 doc-fab incidents" | Historical; unchanged. |
| `_ai/decisions/secrets-split.md:24, 29` | `ErrMountsNotSupported (M1)`, "defer the split to M3" rejected alternative | M1 reference correct (loader rejection IS M1-specific); "M3" in rejected-alternative-C refers to deferring the env/config split (a *different* deferral than secret-files-on-disk), so the M3 label there is about a hypothetical Plan-C past, not about the new sequence. Don's analysis (his §3 third bullet) and mine concur: leave both. |
| `README.md` | (no M-milestone references) | Verified by grep: README has no milestone numbering. |

### E.2 — Sites that DO need changes (covered by §B and §B.11)

Every site that needed an edit is in §B or §B.11. I cross-checked: my list matches Don's eleven items (his §1–§11) plus the three new Go-source edits in §B.11 plus zero edits for §B.6 (which Don listed but I confirmed is a no-op).

### E.3 — Conclusion

After all substitutions in §B and §B.11 land, no doc or Go source contradicts the new sequence. The audit is clean.

---

## Section F: "Survives unchanged" sanity checks

Don's plan listed eight things that survive the resequence intact. I verified the two most load-bearing:

### F.1 — Schema-versioning's "shape doesn't change" promise

Read `_ai/decisions/schema-versioning.md` end-to-end (26 lines). The promise structure is:

1. **§Forward compatibility** (lines 3–7): two mechanisms (strict mode + `schema_version` integer). Mechanism-level claim, milestone-agnostic. Survives.

2. **§The rule** (lines 9–12): the bullet I'm editing in §B.2.1 is the only milestone-specific claim. After my edit, the rule reads "M1 writes 1, M2 writes 1 (populates Mounts), M7 writes 1 (populates secret-files); shape doesn't change." Mechanically equivalent to the old "M1 writes 1, M3 writes 1" promise — the *number* of populating milestones grew from 2 to 3, but each populates a subset of M1's reserved shape, which is exactly what the original promise required. Survives.

3. **§What "reserve fields" looks like in practice** (lines 14–16): edited in §B.2.2; same survival argument.

4. **§Escalation rule** (lines 18–20): M1-specific ("during M1 implementation"). M1 is shipped, so the rule has discharged. The wording stays accurate as a historical record. Survives.

5. **§Cross-file mismatch** (lines 22–24): `schema_version` mismatch detection. Mechanism-level, milestone-agnostic. Survives.

6. **Trailing pointer** (line 26): "tech plan v2 §5" — historical pointer, survives.

**Don's claim verified: schema-versioning's shape promise survives the resequence intact.**

### F.2 — Container-naming's "M4 boundary" claim

Read `_ai/container-naming.md` end-to-end (14 lines). Structure:

1. **Lines 1–3**: M1 vs M4 divergence framing. M1 and M4 unchanged in the new sequence. Survives.

2. **Lines 5–6**: M1 = `decloud-<name>`, M4 = `decloud-<name>-<deploy-id>`. M1 and M4 unchanged. Survives.

3. **§M4 owns the migration** (lines 8–10): M4 ship-time deliverable. M4 unchanged. Survives.

4. **§Anything that depends on container name** (lines 12–14): "M1–M3" range bound — verified in §B.6 above that this range still describes the right set (any milestone before blue/green). Survives.

**Don's claim verified: M4 boundary in container-naming holds.**

---

## Section G: Risk register and gotchas

### G.1 — The "docs-only became code-touching" risk

Don's plan said "skip Kent and Rob." My grep changed that. **The blast radius is limited to three Go strings, three new test assertions, and zero behavior change** — but the workflow shape difference matters. If Linus reviews the diff expecting docs-only and sees a Go diff, he correctly objects. **Mitigation:** Don, Joel, and Raymond all flag this prominently; this tech plan §A.1 is the first place it surfaces, so Linus reads it before the diff exists.

### G.2 — The order-of-edits failure mode

If Raymond accidentally runs §B.10 (`usage.md` says "M2") *before* Rob's §B.11.2 lands (binary still says "M3"), the docs and binary diverge. Operators reading the doc and hitting the rejection see different milestone numbers. **Mitigation:** §D's execution order is explicit: Kent's tests first (will fail), Rob's source edits second (tests pass), Raymond's docs third (in dependency order). Don't reorder.

### G.3 — Pre-existing bug fix risks scope expansion

§B.9 fixes the `install.md:121` "M2 will write source bundles" → "M6". Per Don's "fix-while-fresh" rule: same-file, mechanical, on-theme. **Risk:** Linus sees the install.md diff and asks "is there ANOTHER pre-existing bug we should fix while we're here?" **Mitigation:** I grepped `install.md` for any other milestone-mismatch pattern. No other line in `install.md` mentions a milestone in a way that contradicts `m1-scope.md:32`. The §B.9 fix is the only one of its class.

### G.4 — The `m1x-backlog.md` item-6 wording is still M2-coupled

After §B.8, the text says "the new M2 — server-side `--mount` — per the 2026-04-28 resequence". This couples the backlog file to the *current* resequence. If a *future* resequence moves "real Docker" to a different milestone, this entry needs another edit. **Acceptable risk:** the alternative ("never name a milestone") loses readability, and "the next post-M1 real-Docker milestone" remains valid even after a future resequence — it's the parenthetical that decays. Future-Don can update on the next pass.

### G.5 — Schema-versioning escalation rule wording

The §B.2 edits leave the `## Escalation rule` section (line 18 onward) untouched. That section says "If during M1 implementation..." M1 is shipped, so the rule is now historical advice. **Don's plan §2 final bullet noted this and chose to leave it alone — I concur.** Editing it to "If during any milestone implementation..." would generalize correctly but makes a much bigger claim than the original intent. Out of scope for this task.

---

## Section H: What this plan locks in (for Linus)

1. **The new sequence** (per Don, unchanged): M1 / M2 mounts / M3 bootstrap / M4 blue-green / M5 jobs / M6 backups+GC / M7 polish+secrets+client.

2. **Workflow modification:** Kent and Rob ARE included (re-added per Don's contingency). Three new tests, three Go-source edits, zero behavior change.

3. **No new `_ai/decisions/` file** — per §A.4, the task directory plus `m1-scope.md`'s appended sentence plus the `MEMORY.md` cross-reference is sufficient.

4. **Verbosity calls:** terse for `m1-test-strategy.md` (§A.5), brief-pointer for `m1x-backlog.md` (§B.8 reasoning).

5. **Pre-existing bug:** `_docs/install.md:121` "M2 will write source bundles" → "M6 will write source bundles" — fixed in this task per fix-while-fresh, justified in §B.9.

6. **Edit count:** 18 doc edits across 9 files + 3 source edits across 2 files + 3 new test assertions across 2 test files = 24 distinct surgical changes. Every `old_string` is verified against current bytes. Zero `replace_all` calls.

If Linus pushes back on any of (1)–(6), the burden is on Linus to produce a counter-argument that this plan didn't address.

---

## Files relevant to this task (absolute paths)

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/002-don-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/003-joel-tech-plan.md` (this file)
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md`
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md`
- `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md`
- `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md`
- `/Users/fenster/dev/decloud/_ai/container-naming.md`
- `/Users/fenster/dev/decloud/_ai/MEMORY.md`
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`
- `/Users/fenster/dev/decloud/_docs/install.md`
- `/Users/fenster/dev/decloud/_docs/usage.md`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go` (for §B.11)
- `/Users/fenster/dev/decloud/internal/registry/store.go` (for §B.11)
- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go` (for §C.1, §C.3)
- `/Users/fenster/dev/decloud/internal/registry/store_test.go` (for §C.2)
