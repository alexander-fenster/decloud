# Raymond's doc updates — `decloud status` lists every registered service

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`.
Predecessor reports: `002-don-plan.md`, `03-tech-plan.md`, `04-linus-review.md`,
`005-kent-tests.md`, `006-rob-implementation.md`.

This is the STEP 3.3 commit. Rob's implementation (commit `d86973a`) is
the source of truth for every claim in the docs; I cross-checked each
field-level statement against `internal/cli/status.go`,
`internal/deploy/lifecycle.go`, and `internal/registry/store.go` as
merged on this branch.

## 1. Files touched

Two files. Both are user-facing markdown — no Next.js compile step in
this repo (the `_docs/` Next.js framing in the Raymond agent
description does not apply to `decloud`; `_docs/` here is plain
markdown).

- `_docs/usage.md` — three sections touched (§4 bullet, §4.1 "Status
  format" expansion, §5 end-to-end example expansion).
- `README.md` — one bullet touched (line 104 area).

`git diff --stat` confirms scope:

```
 README.md      |  2 +-
 _docs/usage.md | 57 ++++++++++++++++++++++++++++++++++++++++++++++++++++++---
 2 files changed, 55 insertions(+), 4 deletions(-)
```

No other files in `_docs/`, `_ai/`, or elsewhere needed updating —
see §3 below for the search I did.

## 2. Sections changed and why

### 2.1 `_docs/usage.md` §4 lifecycle bullet (line 194 before, line 194 after)

Old:

```
- `decloud status <name>` — runtime state plus registry view. Output is one line.
```

New (one bullet, dual-form synopsis with a forward-link to §4.1):

```
- `decloud status [name]` — runtime state plus registry view. With a service name, prints one line for that service. Without an argument, prints one row per registered service as an aligned table on stdout. Passing more than one positional argument is a usage error (exit 2). See [§4.1](#status-format) for both output shapes.
```

Rationale: the bullet list in §4 is a one-line-per-command index;
operators scanning for "what does status do" need both forms named
inline, not buried below. Detail goes to §4.1.

### 2.2 `_docs/usage.md` §4.1 "Status format" — restructured

Before: one flat list documenting the single-line format and four
state values.

After: three subsections under the existing `### Status format`
heading. I kept §4.1's H3 heading anchor (`#status-format`) intact so
existing cross-links — including my new bullet's `[§4.1](#status-format)`
— do not break.

- `#### Single-service form` — verbatim move of the prior format line
  and the four state values, plus one sentence locking in
  bit-for-bit backward compatibility ("existing scripts that parse
  this line keep working"). This is the Linus Risk A regression
  lock surfaced in operator-facing text.
- `#### Multi-service form (no argument)` — new. Documents:
  - The five-column header in the exact tabwriter order
    (`NAME STATE CONTAINER DEPLOY DEPLOYED_AT`).
  - Sort by NAME, byte order, justified against the service-name
    regex.
  - That tabwriter renders space-padded columns (no embedded tabs)
    so `awk` / `cut -d' '` work — this is the grep-friendliness
    contract Joel called out at tech plan §10.
  - The `-` placeholder convention for empty cells (preserves
    column count for scripts that read by index).
  - One example output block showing two healthy services + one
    `error` row. The example uses the exact column header from
    `runStatusAll` and plausible-but-illustrative deploy-ID values
    matching the `YYYYMMDD-HHMMSS-XXXXXX` shape produced by
    `internal/ids`. I did NOT hand-compute tabwriter padding pixel-
    by-pixel; the example uses spacing consistent with what
    `tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)` produces for those
    row widths. Operators reading the docs care about *which*
    columns appear and in what order, not whether the example has
    eleven spaces vs twelve between two cells.
  - Empty-registry behaviour: header line alone, no sentinel
    sentence (the operator-script-friendliness rationale is one
    sentence inline).
- `##### Per-row error policy` (H5, nested under multi-service form) —
  new. Documents:
  - `STATE=error` rows render `-` in every other data column.
  - The listing still exits 0 — per-service failures do not poison
    the rest.
  - The stderr companion format `status: <name>: <wrapped error text>`
    matches the literal `fmt.Fprintf(errw, "status: %s: %s\n", ...)`
    in `internal/cli/status.go:64`.
  - `2>/dev/null` suppresses the diagnostics without touching the
    table — an operator hint, not a code claim.
  - Concurrent-deploy race policy: services that vanish between
    `ListNames` and `Load` are dropped, not synthesised. This
    matches `lifecycle.go:145` (`errors.Is(err, registry.ErrNotFound)`
    + `continue`).
  - Host-level failure (registry dir unreadable) → no stdout, exit
    70. Matches `TestStatus_NoArgs_StatusAllErrorIsReturnedAndMapsToExitInternal`
    in `internal/cli/lifecycle_commands_test.go:175-186`.
  - `>1 args` → exit 2. Matches `TestStatus_TooManyArgsReturnsUsageError`
    at line 209-216.
- `#### State values` — consolidated subsection covering BOTH forms.
  The five values are listed in the order an operator usually sees
  them in the wild: `running`, `stopped`, `absent`, `config-only`,
  `error`. Each value carries one sentence of meaning. `error` is
  explicitly flagged "multi-service form only" — the single-service
  path either returns a valid `Status` or returns an error to the
  caller; it never emits a synthesised `error` row.

The five-value enum is the precise contract surface Joel locked in
at tech plan §0.1 and §10, and Kevlin was tagged to hallucination-
check at tech plan §12. I cross-read it against `lifecycle.go`:

- `running` — passthrough from `Driver.Inspect` (`lifecycle.go:108`).
- `stopped` — rewrite of `exited` (`lifecycle.go:107`).
- `absent` — passthrough from `Driver.Inspect` (`lifecycle.go:108`).
- `config-only` — synthesised in `Status` when `Load` returns
  `ErrSecretsMissing` (`lifecycle.go:95`).
- `error` — synthesised in `StatusAll` only (`lifecycle.go:150`).

Five values, exactly. No drift.

### 2.3 `_docs/usage.md` §5 end-to-end example (line 274-279 before)

Added a second `decloud status` invocation after the single-service
form, showing the no-arg table form with two illustrative rows
(`myservice` running, `other` stopped). This makes the new form
visible to operators who copy-paste from §5 without reading the
flag reference, which Joel flagged as optional at tech plan §8.3
but I included because the docs example was the only place to see
the table layout in context.

The intro sentence above the original `decloud status myservice`
example changed from "Inspect:" to "Inspect one service:" so the
two examples are clearly parallel ("one service" vs "every
registered service"). One word added.

### 2.4 `README.md` line 104

Old:

```
- `decloud status <name>` — runtime + registry state on one line.
```

New:

```
- `decloud status [name]` — runtime + registry state. With a name, one line for that service; without, an aligned table with one row per registered service.
```

Rationale: README is terse; one bullet, dual-form synopsis, no
output-shape detail (that lives in `_docs/usage.md`).

## 3. What I checked but did NOT change

### 3.1 `_ai/` documentation

`grep -n -i 'decloud status\|status command\|status <name>\|status \[name\]'`
across all eight `_ai/*.md` files that mention "status" found zero
references to the CLI surface. The `_ai/` files that match the grep
for the word "status" use it in unrelated senses ("HTTP 200 status",
"task status", "registry state status"). No `_ai/` doc needed
updating.

This is the right outcome: per `_ai/cli-flag-surface-coherence.md`,
user-facing surfaces are documented in `_docs/usage.md`, and `_ai/`
captures *learnings* (which is Ward's STEP 4.1 job, not mine).

### 3.2 `_docs/install.md`

Spot-checked: no references to `decloud status`. Nothing to do.

### 3.3 Existing docs that are unrelated to this task

While reading §2 of `_docs/usage.md` for tone calibration, I noticed
the bullet for `--config-root` at the bottom of the `deploy service`
flag table says "Logs are written to `<config-root>/logs/decloud.log`
(the flag controls log placement as well as registry/Caddy paths)."
This claim is unrelated to the `status` listing surface; I have NOT
verified it against the current code and left it untouched. Flagging
it here per the task brief ("don't fix unrelated bugs — just flag
them") in case it surfaces during PLAN round 2.

I made no other unscoped edits.

## 4. Verification done

- `git diff --stat` — only the two expected files changed.
- Full `git diff` walked line-by-line. Every claim in the diff was
  cross-referenced against the merged code:
  - `Use: "status [name]"` ↔ `status.go:17`.
  - Column header `NAME\tSTATE\tCONTAINER\tDEPLOY\tDEPLOYED_AT` ↔
    `status.go:49`.
  - Sort order ↔ `store.go:205` `sort.Strings(names)`.
  - `-` placeholder ↔ `dashIfEmpty` + `rfc3339OrDash` in `status.go:70-82`.
  - Stderr format `status: %s: %s\n` ↔ `status.go:64`.
  - Per-row error policy (exit 0, stderr companion) ↔
    `lifecycle.go:148-153` + CLI test `TestStatus_NoArgs_RowErrorDetailRoutesToStderrButNotStdout`.
  - Concurrent-deploy race drop ↔ `lifecycle.go:145-147`.
  - Host-level failure → exit 70 ↔
    `TestStatus_NoArgs_StatusAllErrorIsReturnedAndMapsToExitInternal`.
  - `>1 args` → exit 2 ↔ `TestStatus_TooManyArgsReturnsUsageError`.
  - Five state values ↔ five sites in `lifecycle.go`.
- Anchor link `[§4.1](#status-format)` resolves: §4.1 keeps its
  `### Status format` H3 heading (anchor unchanged), only the body
  below it was expanded into nested H4/H5 subsections.

## 5. What I want Kevlin to hallucination-check extra-carefully

Per the Raymond agent description ("Apply the same accuracy
discipline to your report that you apply to your artifacts"), the
two areas most prone to drift between docs and code are:

1. **The five-value `STATE` enum.** Listed in three places in the
   diff: the §4 bullet (implicit via "exit 2"), the multi-service
   example output (`stopped`, `running`, `error`), and the
   `#### State values` subsection (all five spelled out). If the
   code ever adds a sixth synthesised state or renames one, all
   three places need to change. The contract is exactly:
   `running`, `stopped`, `absent`, `config-only`, `error`.
2. **The literal stderr prefix `status: `.** I quoted the exact
   `fmt.Fprintf` format string from `status.go:64`. If a future
   refactor changes that prefix (Joel §6.4 says it can evolve), the
   docs need to follow. Kevlin: please re-grep
   `fmt.Fprintf(errw,` in `internal/cli/status.go` and confirm the
   literal `"status: %s: %s\n"` is still there.
3. **The example output block in §4.1's multi-service form.** I
   used illustrative deploy IDs (`20260426-102001-aa11bb`,
   `20260426-093214-7f3a9c`) that match the `YYYYMMDD-HHMMSS-XXXXXX`
   shape produced by `internal/ids`. I did NOT machine-generate
   the example by running `decloud status` against a fake registry
   — the spacing in the example is hand-typed to approximate
   tabwriter's two-space padding. If Kevlin wants byte-precise
   alignment, the right fix is a follow-up that captures the
   output of one of Kent's tests as a fixture and inlines it. I do
   not think that level of precision adds operator value, but I
   want it on the record.

Nothing in this diff invented field names. The doc treats
`STATE`, `CONTAINER`, `DEPLOY`, `DEPLOYED_AT` as column labels (UI
strings), not as Go field names on the `Status` struct, so the
"NEVER hallucinate field names" rule from the Raymond agent
description applies trivially: the column labels are the operator-
facing identifiers, not the Go-side `ContainerName` /
`LastDeployID` / `LastDeployedAt` field names.

## 6. What's next

Per CLAUDE.md workflow STEP 3.4: Kevlin and Linus run parallel
review of all execution-step output (Kent + Rob + Raymond). Kevlin
should pay particular attention to the three items in §5 above.

— Raymond
