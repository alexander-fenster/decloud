# Raymond's docs report: usage.md + AI notes brought in line with Rob's diff

I updated three files to reflect the three behavior-affecting changes Kent
and Rob shipped. No new docs files were created. Every claim below is
cited against the production diff, the existing tests, or the existing
prose so Kevlin can spot a hallucination quickly.

---

## 1. `_docs/usage.md` — three table rows under §2

The table at §2 (`decloud deploy service`) is the operator-facing source
of truth for the flag contract. Joel's tech plan §4 specified exactly
which rows changed; I executed that.

### 1a. `--port` row — Required column flipped to `yes`

**Before:**

```
| `--port` | int | `0` | only if `--host` is set | Container's listen port. |
```

**After:**

```
| `--port` | int | — | yes | Container's listen port. Required because every M1 service is HTTP and the readiness probe targets this port; missing or `0` fails fast with exit 2 (`--port is required`). Worker/job workloads without an HTTP listener are M5. |
```

**Citations:**

- Required column flipped: `internal/cli/deploy_service.go:73-75` —
  `if f.Port == 0 { return fmt.Errorf("--port is required: %w", errUsage) }`.
  The check is now unconditional, not gated on `--host`.
- Default `—` (em dash) instead of `0`: matches the presentation of the
  other "yes-required" row in the same table (`--name` row, line 57)
  where the flag has no useful default to a user. The Cobra-level default
  is still `0` (`deploy_service.go:55`) but presenting `0` next to
  "Required: yes" would be misleading — `0` is not a valid value, it is
  just the zero-value `IntVar` settles on when the flag is unset.
- Error wording `--port is required`: matches Rob's diff at
  `internal/cli/deploy_service.go:74` literally. Rob picked the shorter
  form (Linus's bikeshed suggestion in `04-linus-review.md` §"One thing
  to sanity-check") over Joel's longer parenthetical version; both
  authors signed off on either. The doc shows what users actually see.
- Exit code 2: `internal/cli/exit_codes.go:35-46` — `errUsage` maps to
  `ExitUsageError` = 2, locked in by
  `TestDeployService_NoPortReturnsExitUsageError` and
  `TestDeployService_PortZeroExplicitReturnsExitUsageError` in
  `internal/cli/deploy_service_test.go`.
- "Worker/job workloads ... M5" sentence: Don's policy rationale in
  `02-plan.md` §"Finding 3" — workers with no HTTP listener are a
  separate command (`decloud deploy job`) in M5, not a `--port=0` mode
  of `deploy service`. Linus reaffirmed in `04-linus-review.md`
  §"Finding 3 — Reject port=0: CORRECT POLICY, NOT A CORNER" with a
  cross-reference to `_ai/decisions/m1-scope.md:32` and `README.md:215`.

### 1b. `--dockerfile` row — Notes tightened

**Before:**

```
| `--dockerfile` | string | `Dockerfile` | no | Path to the Dockerfile, relative to `<source-dir>`. |
```

**After:**

```
| `--dockerfile` | string | `Dockerfile` | no | Path to the Dockerfile. Relative paths resolve under `<source-dir>` regardless of the cwd you invoke `decloud` from. Absolute paths are used as-is. |
```

**Citations:**

- "Relative paths resolve under `<source-dir>`": after Rob's six-line
  diff at `internal/cli/deploy_service.go:80-86`, a relative
  `--dockerfile` is `filepath.Join`ed with `abs` (the resolved absolute
  source dir from `filepath.Abs(sourceDir)` at `:76-79`). Locked in by
  `TestDeployService_DefaultDockerfileIsJoinedWithSourceDir` and
  `TestDeployService_RelativeDockerfileIsJoinedWithSourceDir` in
  `internal/cli/deploy_service_test.go`.
- "Regardless of the cwd you invoke `decloud` from": this is the
  user-visible outcome of the bug fix. The original review finding was
  exactly that "from outside `./myservice`" the resolution was wrong;
  the regression is locked in by
  `TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved`,
  which `os.Chdir`s into a parent and invokes with `./svc`. The doc
  promises operators what the code now delivers.
- "Absolute paths are used as-is": after Rob's diff,
  `if !filepath.IsAbs(dockerfile)` skips the `filepath.Join` for
  absolute paths. Locked in by `TestDeployService_AbsoluteDockerfileIsPreserved`
  in `internal/cli/deploy_service_test.go`.

### 1c. `--config-root` row — Logs sentence appended

**Before:**

```
| `--config-root` | string | `$DECLOUD_ROOT` or `/opt/declouding` | no | Root directory of the Declouding tree. Persistent flag, applies to every subcommand. |
```

**After:**

```
| `--config-root` | string | `$DECLOUD_ROOT` or `/opt/declouding` | no | Root directory of the Declouding tree. Persistent flag, applies to every subcommand. Logs are written to `<config-root>/logs/decloud.log` (the flag controls log placement as well as registry/Caddy paths). |
```

**Citations:**

- Logs path `<config-root>/logs/decloud.log`:
  `internal/logging/logging.go:30,36` — `logsDir := filepath.Join(root, "logs")`,
  `logPath := filepath.Join(logsDir, "decloud.log")`. After Rob's diff,
  `root` is the `rc.ConfigRoot` value passed in from
  `internal/cli/root.go:23` (`return logging.Init(rc.ConfigRoot)`).
- "The flag controls log placement as well as registry/Caddy paths":
  The original finding (`01-user-request.md` §2) was that
  `--config-root` controlled registry/Caddy paths but not log placement.
  After the fix, both are governed by the same value. The end-to-end
  regression is locked in by `TestRoot_ConfigRootFlagControlsLogPlacement`
  in `internal/cli/root_test.go`.

### What I did NOT change in `_docs/usage.md`

- The `§1 Quick start` example (`--port 8080`) was already consistent
  with the new "port required" rule. No edit needed.
- `§3 Exit codes` already lists exit 2 = `ExitUsageError` for "missing
  or unknown flag, missing arguments, internal usage misuse" — the new
  `--port` rejection fits cleanly under that bucket. No new exit code,
  no row change.
- `§5 End-to-end example` already passes `--port 8080`. No edit needed.
- Section §1 prose framing (env.sh discovery, "Declouding does not keep
  an image archive", etc.) is unrelated to these three findings.

---

## 2. `_ai/cobra-init-pattern.md` — signature update

This file was the canonical decision record for the
`PersistentPreRunE` + `logging.Init` pattern. After Rob's signature
change it became stale in two ways:

1. The recipe code block showed `logging.Init()` — wrong arity now.
2. It did not document the explicit-input contract (no env-fallback
   inside `Init`).

**Changes (two edits):**

### 2a. Recipe code block + new paragraph

**Before:**

```
Filesystem init lives in `internal/logging/logging.go:Init()` and is
called by `internal/cli/root.go:22-24`:

    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        return logging.Init()
    }
```

**After:**

```
Filesystem init lives in `internal/logging/logging.go:Init(root string) error`
and is called by `internal/cli/root.go:22-24` with the resolved
`--config-root` value:

    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        return logging.Init(rc.ConfigRoot)
    }

Pass the resolved root explicitly — do NOT have `logging.Init` re-read
`DECLOUD_ROOT`. Cobra fills `rc.ConfigRoot` from the persistent
`--config-root` flag (whose default is `config.RootFromEnv()`) before
`PersistentPreRunE` runs, so the value flag-wins-over-env at the Cobra
layer. `logging.Init("")` falls back to `config.DefaultRoot` (mirrors
`config.NewPaths` semantics).
```

**Citations:**

- Signature `Init(root string) error`: `internal/logging/logging.go:22`.
- Caller passes `rc.ConfigRoot`: `internal/cli/root.go:23`.
- `--config-root` default is `config.RootFromEnv()`:
  `internal/cli/root.go:26`.
- Empty-string → `config.DefaultRoot` mirroring `NewPaths`:
  `internal/logging/logging.go:27-29` matches
  `internal/config/paths.go:24-27`. Joel cited this equivalence in
  `03-tech-plan.md` §0 and §"Why empty-string fallback".
- "Do NOT have `Init` re-read `DECLOUD_ROOT`": locked in by
  `TestInit_UsesPassedRootNotEnv` in `internal/logging/logging_test.go`,
  which sets `DECLOUD_ROOT` to a path that must not be touched and
  asserts the log lands under the passed-in root only.

### 2b. Fallback list updated

**Before** (numbered list at "Mandatory: graceful EACCES/ENOENT fallback"):

```
Pattern at `internal/logging/logging.go:21-43`:

1. Env-var short-circuit FIRST...
2. `MkdirAll` failure → ...
3. `OpenFile` failure → ...
4. `Init()` returns nil ...
```

**After:**

```
Pattern at `internal/logging/logging.go:22-46`:

1. Env-var short-circuit FIRST...
2. Empty-string root → fall back to `config.DefaultRoot` (mirrors `config.NewPaths`).
3. `MkdirAll` failure → ...
4. `OpenFile` failure → ...
5. `Init` returns nil ...

The warning text is loadbearing: ... Do not collapse to `Init(string) {}` —
keep the error return for future use.
```

**Citations:**

- Line range `22-46` is the new function body span (`func Init(root string) error {`
  starts at `:22`, closing `}` is `:46`).
- The new step 2 maps to `internal/logging/logging.go:27-29`.
- Step ordering matches Rob's diff (env short-circuit → empty-string
  fallback → MkdirAll → OpenFile).
- The phrase `Init(string) {}` (instead of `Init() {}`) reflects the
  current arity; the contract that the function returns `error` for
  future I/O surfacing is unchanged.

---

## 3. `_ai/m1x-backlog.md` — line numbers + scope note for the
`logging.Init` warning entry

Backlog item #5 ("Logging warning leaks to test stderr") cited the
literal line numbers of the two `fmt.Fprintf` warning calls. After
Rob's diff those lines moved (from `:29`,`:36` to `:32`,`:39`), and
the noise profile of the warning changed: tests that pass
`--config-root <t.TempDir()>` no longer trip it because mkdir succeeds
under the temp dir. The backlog entry would otherwise be misleading
for future-Don.

**Before:**

```
**Where:** `internal/logging/logging.go:29` and `:36` —
`decloud: log dir unavailable, using stderr only: ...` fires once per
CLI test that doesn't set `DECLOUD_LOG_TO_STDERR_ONLY=1`. Cosmetic; no
test asserts stderr cleanliness, no test fails.
```

**After:**

```
**Where:** `internal/logging/logging.go:32` and `:39` —
`decloud: log dir unavailable, using stderr only: ...` fires once per
CLI test that doesn't set `DECLOUD_LOG_TO_STDERR_ONLY=1` (and doesn't
pass a writable `--config-root`). Mostly cosmetic; no test asserts
stderr cleanliness, no test fails. After the `Init(root string)` change
in `_tasks/2026-04-26-fix-deploy-service-review-findings/`, tests that
pass `--config-root <t.TempDir()>` no longer trip the warning, so the
noise is reduced but not gone.
```

**Citations:**

- New line numbers verified directly against
  `internal/logging/logging.go` (`:32` is the "log dir unavailable"
  Fprintf inside the `MkdirAll` failure branch; `:39` is the "log file
  unavailable" Fprintf inside the `OpenFile` failure branch).
- "Tests that pass `--config-root <t.TempDir()>` no longer trip the
  warning": Rob noted this cosmetic improvement explicitly in
  `06-rob-impl.md` §5 note 4.

---

## 4. AI files I considered and intentionally did NOT touch

- `_ai/MEMORY.md` — index file. The `cobra-init-pattern.md` entry's
  one-liner ("`PersistentPreRunE` for filesystem-touching init +
  EACCES/ENOENT graceful fallback; what saves `decloud --help` from
  exit 70 on a fresh box") is still accurate. The signature change is
  documented inside `cobra-init-pattern.md` itself; the index
  one-liner doesn't need to mention it.
- `_ai/decisions/m1-scope.md` — strategic milestone scope. Talks
  about M1 = `decloud deploy service` but does not get into the flag
  contract or logging mechanism. Untouched.
- `_ai/decisions/m1-test-strategy.md` — already mentions
  `DECLOUD_LOG_TO_STDERR_ONLY=1` short-circuit and the
  fallback-warning behavior at §5. The wording is still accurate;
  the env var still does what it used to do; the warning still
  fires when no writable root is reachable. Untouched.
- `_ai/decisions/secrets-split.md`, `_ai/decisions/schema-versioning.md`
  — orthogonal to these findings. Untouched.
- `_ai/error-wrap-discipline.md`, `_ai/optional-input-two-layer.md`,
  `_ai/gomock-inorder-sequencing.md`, `_ai/envcap-portable-bash.md`,
  `_ai/container-naming.md` — orthogonal to these findings. Untouched.
- `README.md` — unchanged by this task; the user-facing reference is
  `_docs/usage.md` per CLAUDE.md docs convention.
- `_docs/install.md` — unrelated to deploy-service flag contracts.

---

## 5. Files touched (final manifest)

| File | Type of change |
|---|---|
| `_docs/usage.md` | Three table rows updated under §2 (Required column on `--port`, Notes on `--dockerfile` and `--config-root`). No structural changes; no new sections. |
| `_ai/cobra-init-pattern.md` | Recipe block updated to new signature; added an explicit-input-contract paragraph; renumbered fallback list to 5 steps; line range updated. |
| `_ai/m1x-backlog.md` | Item #5 line numbers updated (`:29`/`:36` → `:32`/`:39`); scope note added that `--config-root` now reduces (but doesn't eliminate) the warning. |

No files created. No files removed. No files renamed.

---

## 6. Hand-off

Kevlin and Linus can now review in parallel:

- Kevlin: low-level review of the production diffs (Rob's three files)
  AND the doc claims above. The "no hallucinations" bar applies; every
  claim cites its source above.
- Linus: high-level review confirming the contract is now coherent
  end-to-end across CLI, `_docs/usage.md`, and the `_ai` decision
  records.

If either reviewer finds a discrepancy between docs and code, escalate
back to PLAN per the workflow in `CLAUDE.md`.

— Raymond
