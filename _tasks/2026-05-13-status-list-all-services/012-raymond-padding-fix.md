# Raymond's doc padding fix — `decloud status` example blocks

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`.
Predecessor reports read for context: `007-raymond-docs.md`,
`010-kevlin-review.md` (N1), `011-don-round2.md` (verdict ITERATE).

## Scope

EXECUTION lap 2, narrow. Two example output blocks in `_docs/usage.md`
had hand-typed CONTAINER-column padding that did not match what
`tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)` actually emits for the
rows shown. Don verified Kevlin's byte-precise replacements before
nominating them. My job: paste them in and re-verify.

## What I changed

Single file: `_docs/usage.md`. Two blocks, both fenced as ```text```:

### Block 1 — §4.1 "Multi-service form" example (lines 227-230)

Replaced:

```text
NAME        STATE    CONTAINER             DEPLOY                  DEPLOYED_AT
bar         stopped  decloud-bar           20260426-102001-aa11bb  2026-04-26T10:20:01Z
broken-svc  error    -                     -                       -
foo         running  decloud-foo           20260426-093214-7f3a9c  2026-04-26T09:32:14Z
```

with:

```text
NAME        STATE    CONTAINER    DEPLOY                  DEPLOYED_AT
bar         stopped  decloud-bar  20260426-102001-aa11bb  2026-04-26T10:20:01Z
broken-svc  error    -            -                       -
foo         running  decloud-foo  20260426-093214-7f3a9c  2026-04-26T09:32:14Z
```

Drift was on the CONTAINER column: padded to ~22 chars (11 trailing
spaces after `decloud-bar`) where real tabwriter pads to 13 (the
configured `padding=2` after the widest cell, `decloud-bar`).

### Block 2 — §5 "End-to-end example" multi-service listing (lines 285-287)

Replaced:

```text
NAME       STATE    CONTAINER           DEPLOY                  DEPLOYED_AT
myservice  running  decloud-myservice   20260426-093214-7f3a9c  2026-04-26T09:32:14Z
other      stopped  decloud-other       20260425-110000-def456  2026-04-25T11:00:00Z
```

with:

```text
NAME       STATE    CONTAINER          DEPLOY                  DEPLOYED_AT
myservice  running  decloud-myservice  20260426-093214-7f3a9c  2026-04-26T09:32:14Z
other      stopped  decloud-other      20260425-110000-def456  2026-04-25T11:00:00Z
```

Same defect, off-by-one on the CONTAINER column (3 trailing spaces
after `decloud-myservice` instead of 2).

No surrounding prose was touched. The fenced blocks are the only
edits — header rows, sort order, dash-rendering for empty cells,
and the `error`-row five-column shape all survive intact.

## Verification

### 1. Re-rendered the rows through real `text/tabwriter`

Wrote a throwaway Go file at `/tmp/tabwriter_verify.go` that constructs
`tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)` — the exact config
from `internal/cli/status.go:48` — feeds it the same five header
columns and the same body rows (tab-separated), and flushes.
Output:

```text
=== §4.1 ===
NAME        STATE    CONTAINER    DEPLOY                  DEPLOYED_AT
bar         stopped  decloud-bar  20260426-102001-aa11bb  2026-04-26T10:20:01Z
broken-svc  error    -            -                       -
foo         running  decloud-foo  20260426-093214-7f3a9c  2026-04-26T09:32:14Z

=== §5 ===
NAME       STATE    CONTAINER          DEPLOY                  DEPLOYED_AT
myservice  running  decloud-myservice  20260426-093214-7f3a9c  2026-04-26T09:32:14Z
other      stopped  decloud-other      20260425-110000-def456  2026-04-25T11:00:00Z
```

That output matches the new doc blocks byte-for-byte. Scratch file
deleted after verification (`rm /tmp/tabwriter_verify.go`).

### 2. Diff scope check

`git diff --stat` after the edits:

```text
 _docs/usage.md | 14 +++++++-------
 1 file changed, 7 insertions(+), 7 deletions(-)
```

Single file, both hunks confined to the two fenced ```text``` blocks
Don nominated. No code change, no test change, no other doc file
touched, no `README.md` change.

### 3. gofmt / `go test` sanity

No Go files were touched, so there is nothing for `gofmt` to
reformat and nothing for `go test` to regress. The `git diff --stat`
above confirms the change is doc-only.

## What I did not change

- `README.md` — not in scope; Don explicitly excluded.
- Any Go code or test file — none was needed.
- Surrounding prose in §4.1 or §5 — the new blocks remain
  grammatically embedded in the existing lead-ins ("Example output
  for two healthy services plus one with a broken registry entry:"
  and "Or list every registered service in one table:"). No
  one-word lead-in adjustment was required.
- §4.1 structure or new sections — Don forbade restructuring; none
  attempted.

## Status

Doc-padding fix complete. Ready for Kevlin's 30-second re-skim of the
two-block diff per Don's plan in `011-don-round2.md` §"Re-review —
Kevlin only".

— Raymond
