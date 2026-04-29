# 005 — Joel's tech-plan addendum: Linus's three revisions

This is an append-only addendum to `003-joel-tech-plan.md`. The original tech plan stands; this file deltas it for the three items Linus called out in `004-linus-plan-review.md` (Issues 1, 5, 10). Kent and Rob: read both files. Where the addendum contradicts the original, the addendum wins.

Anchors:
- Linus's review: `_tasks/2026-04-28-m2-server-side-mounts/004-linus-plan-review.md`
- Original tech plan: `_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md`

---

## Issue 1 — Dual-sentinel chain in `parseMountFlags`

### Decision: Option B (strip the wrap).

Linus leaned B; I concur. Option A (a comment in `exit_codes.go`) leaves the case-ordering footgun live; it works only as long as nobody alphabetises or groups the cases in a future cleanup. Comments lie eventually. Option C (parameterise `ValidateMounts` with a wrapping sentinel) is API noise for a single sentinel. Option B is structurally honest: each surface produces exactly one sentinel.

### Which sentinel does the CLI's duplicate-target case carry: `errUsage` or `ErrInvalidMount`?

**`errUsage`. Exit 2.**

The CLI is a *parsing context*. `--mount /a:/b --mount /c:/b` is the operator typing two flags whose interaction is invalid — the same shape as `--port=0`, `--strategy=blue_green-from-CLI` (if the CLI ever rejected that — it does not today, by design, see the M1 split), or `--readiness-timeout=-5s`. The grammar of *the command line* is wrong. That is exactly what `errUsage` (exit 2) means.

The same TOML-on-disk shape (two mounts pointing at one target inside one `services/foo.toml`) is a *world-state error* the loader catches: a hand-edited TOML the registry refuses to accept. That is `ErrInvalidMount` (exit 10).

This is the same split the M1 plan locked between CLI `--strategy=blue_green` (no CLI rejection in M1; goes to loader, which returns `ErrInvalidStrategy` exit 10) and CLI `--port=0` (rejected at CLI parse time as `errUsage`, exit 2). Decision 5 of the original tech plan already states the rule: **CLI parse failure → `errUsage` (exit 2); loader failure → `ErrInvalidMount` (exit 10).** The current `parseMountFlags` implementation accidentally violates this rule for the duplicate-target sub-case by routing through `ValidateMounts` (which wraps with `ErrInvalidMount`); Linus's Issue 1 is real and the fix restores the rule.

### Implementation: factor the cross-mount duplicate check into its own helper

The original §3.2 builds duplicate detection inside `ValidateMounts`. To strip the wrap cleanly, separate the per-mount check from the cross-mount check.

#### Updated `internal/registry/mount.go` (delta vs §3.2 of the original tech plan)

Replace the `ValidateMounts` body in §3.2 with this structure. The signature of `ValidateMounts` does NOT change (it still wraps with `ErrInvalidMount`); we add an unexported helper for the cross-mount check that returns a bare error, which both `ValidateMounts` and `parseMountFlags` consume.

```go
// ValidateMounts validates each entry and additionally rejects duplicate
// container_paths within the slice. Loader callers receive errors wrapped
// with ErrInvalidMount; CLI callers should NOT use this entry point — use
// the per-mount ValidateMount helper plus checkDuplicateTargets directly so
// the CLI wraps with errUsage instead.
func ValidateMounts(mounts []Mount, name, path string) error {
    for i, m := range mounts {
        if err := ValidateMount(m); err != nil {
            return fmt.Errorf("%w: service %q mount[%d] in %s: %w", ErrInvalidMount, name, i, path, err)
        }
    }
    if i, j, ok := findDuplicateTarget(mounts); ok {
        return fmt.Errorf("%w: service %q mount[%d] in %s: duplicate container_path %q (also at mount[%d])",
            ErrInvalidMount, name, j, path, mounts[j].ContainerPath, i)
    }
    return nil
}

// findDuplicateTarget reports the first duplicate container_path in mounts.
// Returns (firstIndex, secondIndex, true) when a duplicate exists, or
// (0, 0, false) when all targets are unique. The bare-error shape lets CLI
// callers wrap with errUsage without re-routing through ErrInvalidMount.
func findDuplicateTarget(mounts []Mount) (firstIdx, dupIdx int, found bool) {
    seen := make(map[string]int, len(mounts))
    for i, m := range mounts {
        if first, ok := seen[m.ContainerPath]; ok {
            return first, i, true
        }
        seen[m.ContainerPath] = i
    }
    return 0, 0, false
}
```

#### Updated `internal/cli/deploy_service.go` `parseMountFlags` helper (delta vs §3.5(d) of the original tech plan)

```go
// parseMountFlags converts repeatable --mount string values into validated
// []registry.Mount entries. CLI failures wrap errUsage (exit 2); the loader's
// equivalent path wraps ErrInvalidMount (exit 10) — they are distinct error
// chains by design (see _tasks/2026-04-28-m2-server-side-mounts/005-joel-
// tech-plan-addendum.md, Issue 1). Do not call registry.ValidateMounts from
// here; its ErrInvalidMount wrap would land this in exit 10 by accident of
// case ordering in exit_codes.go.
func parseMountFlags(raw []string) ([]registry.Mount, error) {
    if len(raw) == 0 {
        return nil, nil
    }
    out := make([]registry.Mount, 0, len(raw))
    for _, s := range raw {
        m, err := registry.ParseMountString(s)
        if err != nil {
            return nil, fmt.Errorf("--mount %q: %s: %w", s, err.Error(), errUsage)
        }
        out = append(out, m)
    }
    if first, dup, ok := registry.FindDuplicateTarget(out); ok {
        return nil, fmt.Errorf("--mount %q: duplicate container_path (also at --mount[%d]): %w",
            out[dup].ContainerPath, first, errUsage)
    }
    return out, nil
}
```

Note the function-naming choice: `findDuplicateTarget` is unexported in §3.2's local writeup, but the CLI lives in a different package (`internal/cli`), so we must export it. Rename to `FindDuplicateTarget` with a doc comment marking it package-API surface for the CLI's use. The `ValidateMounts` body uses the same helper internally.

#### Net effect on the error chain

CLI duplicate-target case:
```
"--mount \"/x\": duplicate container_path (also at --mount[0]): usage error"
```
- `errors.Is(err, errUsage)` → true → exit 2. ✓
- `errors.Is(err, registry.ErrInvalidMount)` → **false** (no longer in the chain). ✓

Loader duplicate-target case (unchanged):
```
"registry: invalid mount: service \"foo\" mount[1] in /opt/decloud/config/services/foo.toml: duplicate container_path \"/x\" (also at mount[0])"
```
- `errors.Is(err, registry.ErrInvalidMount)` → true → exit 10. ✓
- `errors.Is(err, errUsage)` → false. ✓

The case-ordering footgun in `exit_codes.go` is dead. Reorder the cases however you want; both routes land on the right code.

#### Test-surface delta (Kent's contract)

Add to §4.3 of the original tech plan, in the `TestDeployService_MountFlagInvalidReturnsExitUsageError` table-driven test — the `duplicate_target` subtest:

- **NEW assertion:** `assert.False(t, errors.Is(err, registry.ErrInvalidMount), "CLI duplicate-target must NOT carry ErrInvalidMount in its chain")`. This locks Issue 1 against future regressions; if anyone reintroduces the dual wrap, this test fails.
- Existing assertions (`errors.Is(err, errUsage)`, `ExitCodeFor(err) == ExitUsageError`) remain.

Add to §4.1 of the original tech plan, in `TestValidateMounts_DuplicateContainerPath`:
- **NEW companion test** `TestRegistry_FindDuplicateTarget`: table-driven over (none, two-different, two-same, three-with-collision-at-2-and-0). Covers the new exported helper directly without the wrap.

#### File-diff delta (Rob's contract)

Update §3 of the original tech plan, file `internal/registry/mount.go`:
- The `ValidateMounts` body and a new exported `FindDuplicateTarget` function. Body shown above.

Update §3 of the original tech plan, file `internal/cli/deploy_service.go`:
- The `parseMountFlags` helper body. Body shown above. The `--mount: <inner-err>: errUsage` re-wrap form (which dragged `ErrInvalidMount` through) is **deleted**.

§5 (deletion list) of the original tech plan is unaffected — no new sentinels added or removed; only the wrap chain at one call site changes.

§6 (atomic-commit file list) of the original tech plan is unaffected — same files; the diffs inside `mount.go` and `deploy_service.go` shift to match the addendum.

---

## Issue 5 — No-stat operator UX paragraph in `_docs/usage.md`

### Decision: Option B (write the paragraph now).

Linus leaned B; I concur. Option A leaves the first-deploy-with-typo'd-bind-source operator staring at `mkdir /missing: read-only file system` with no path back to "you typo'd the host path." One paragraph in `_docs/usage.md` saves the Google search.

### Exact paragraph wording (locked; do not paraphrase)

```
Bind-mount source paths are not pre-checked. If you pass `--mount /missing-path:/data` and `/missing-path` does not exist on the host, the deploy fails at the `docker run` step with a Docker daemon error referencing the path (typical text: `error while creating mount source path '/missing-path': mkdir ...`), exit 40. To verify before deploying:

```sh
ls -ld /path/to/source
```

Decloud deliberately does not stat the source at parse or load time. Bind sources can legitimately appear after deploy time — for example an automounted disk that is not yet mounted at `decloud start` after a host reboot — and a stat-check would punish that recoverable state. The trade-off is one bad first-deploy error message in exchange for a `decloud start` that survives a reboot ordering race.
```

### Insertion point (Raymond, copy this verbatim)

Insert the paragraph as a new paragraph in `_docs/usage.md`, **between line 73 and line 74** (after the closing `|` of the flag table that ends with the `--config-root` row, and before the line that begins `The \`env.sh\` model.`). The paragraph belongs immediately after the flag table because the `--mount` row at line 71 is the natural anchor for the operator's eyes and the surrounding prose at line 74 is on a different topic (`env.sh` capture model).

The exact location after Raymond's M2 sweep flips line 71 (the row currently saying "Rejected with exit 10 in M1. Persistent volumes are M2.") to its M2 wording — the paragraph still lands between the table and the `env.sh` model paragraph.

### Test-surface delta (Kent's contract)

None. This is doc-only; the `cli-flag-surface-coherence.md` carve-out at line 42 calls out the four surfaces, and `_docs/usage.md` is one of them, but no test asserts on usage.md text directly. Per Don §7 / Issue 7 of Linus's review, no semantic-token survives in M2 to lock prose against.

### File-diff delta (Rob's contract)

None. This is Raymond's deliverable. Folded into the §11 (consolidated docs sweep) of the original tech plan; Raymond's deliverable list gains one line:

> `_docs/usage.md` (between lines 73 and 74): insert the verbatim no-stat paragraph from `005-joel-tech-plan-addendum.md` Issue 5.

---

## Issue 10 — Strategy-block papercut at `store.go:73`

### Decision: Option A (fix both blocks in the fix-while-fresh sweep).

Linus leaned A; I concur. Three characters (`cfg.Name` → `name`) at one site, same papercut as the mount block, same package, same diff-hunk neighbourhood. The whole point of fix-while-fresh is to close adjacent loose ends while the file is open; leaving block 2 with `cfg.Name` after fixing block 1 is the worst kind of half-fix — it means the next diff in this file mentions "we already fixed this once but only in one spot."

### Exact diff for `internal/registry/store.go`

Verified line numbers against the current file (read `internal/registry/store.go:64-76` for context). The strategy-rejection block sits at lines 72-75. After M2's mount-block rewrite at lines 68-71 (now `ValidateMounts(cfg.Run.Mounts, name, cfgPath)`), the strategy block is unchanged structurally; we change the one identifier:

```go
// CURRENT (lines 72-75):
if cfg.Strategy != "" && cfg.Strategy != "recreate" {
    return nil, fmt.Errorf("%w: service %q declares strategy=%q in %s; only \"recreate\" is supported in M1",
        ErrInvalidStrategy, cfg.Name, cfg.Strategy, cfgPath)
}

// NEW (lines 72-75 after M2 rewrite of mount block above):
if cfg.Strategy != "" && cfg.Strategy != "recreate" {
    return nil, fmt.Errorf("%w: service %q declares strategy=%q in %s; only \"recreate\" is supported in M1",
        ErrInvalidStrategy, name, cfg.Strategy, cfgPath)
}
```

The only change: `cfg.Name` → `name`. The `cfg.Name = name` assignment at line 76 (post-block) is unchanged; it still happens for the rest of the function's downstream uses.

**Why this is correct.** The `name` parameter (`func (s *fsStore) Load(ctx context.Context, name string)`) is sourced from the filename — always populated. `cfg.Name` is sourced from the TOML body's `name = "..."` field — empty if the TOML omits the field (and the strict-mode TOML decoder does not enforce it as required). At lines 68 and 73, the function has not yet copied `name` into `cfg.Name` (that happens at line 76), so `cfg.Name` here is whatever the TOML literally said, which can be `""`. Using `name` is the correct operator-debug context — it always names the actual service whose file is being loaded.

### Bundling note (Rob: do NOT split this)

This fix is part of the same fix-while-fresh sweep as the mount-block rename in §3.4. Both edits touch `internal/registry/store.go` in the same function (`Load`), within five lines of each other. Bundle them in **one diff hunk**, in **one commit** — Rob's GREEN production commit per §6 of the original tech plan. Do not split the strategy-block rename into a separate commit even though it is technically unrelated to the mount-block rewrite; the whole point of fix-while-fresh is bundled adjacency.

The original §3.4 diff in the tech plan mentions "fix-while-fresh papercut, in scope per §7" for the mount block. This addendum extends that scope to the strategy block on the same line of reasoning.

### Test-surface delta (Kent's contract)

No new test. The strategy-rejection test that Linus references (`TestStore_LoadRejectsBlueGreenStrategy` or similar — Kent please grep `TestStore_Load` to find the exact name) currently uses a TOML fixture that includes `name = "..."`, so the test has not noticed the papercut. Two options:

- **Option α:** add a subtest with a TOML fixture that omits `name = "..."` and asserts `err.Error()` contains `service "foo"` (where `foo` is the filename). This locks the fix.
- **Option β:** leave the test alone; the existing fixture passes either way; the rename has zero behaviour change at the test surface.

**Locked: Option β.** This is a fix-while-fresh papercut, not a feature; no behaviour change ever surfaced through tests, and adding a new subtest re-creates change-detector character (the new subtest only asserts on a string substring). The rename is correct for the reason stated above; the existing test continues to pass; the next operator who writes a TOML without `name = ` gets the right error in production.

If Kent disagrees and wants to lock it, Option α is acceptable — but as an additive subtest, not a replacement, and the assertion must be substring-only on `service %q` shape, not byte-for-byte.

### File-diff delta (Rob's contract)

Update §3.4 of the original tech plan: the §3.4 diff currently rewrites only the mount block (lines 68-71). The addendum adds: while in this file, change `cfg.Name` → `name` at line 73 inside the strategy block as well. One token, one line, same hunk.

§5 (deletion list) is unaffected.
§6 (atomic-commit file list) is unaffected — `internal/registry/store.go` is already in the list.
§7 (fix-while-fresh sweep) gains a sub-bullet:

> Strategy-block (`store.go:73`): `cfg.Name` → `name` rename, bundled in the same hunk as the mount-block rewrite. Same reasoning as the mount-block fix-while-fresh: the TOML body's `name` field is not enforced as required, so `cfg.Name` can be empty before line 76's assignment; using the function parameter is the correct operator-debug context.

---

## Confirmation

These three revisions are folded into Kent's test surface and Rob's implementation surface as listed in the per-issue "Test-surface delta" and "File-diff delta" subsections above. Kent can start.
