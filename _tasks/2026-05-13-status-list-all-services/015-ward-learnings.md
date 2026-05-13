# Ward's learnings — `decloud status` lists every registered service

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`.
STEP 4.1 (FINALIZATION). Predecessor reports read end-to-end:
`002-don-plan.md`, `03-tech-plan.md`, `04-linus-review.md`,
`005-kent-tests.md`, `006-rob-implementation.md`, `007-raymond-docs.md`,
`009-linus-review-exec.md`, `010-kevlin-review.md`, `011-don-round2.md`,
`012-raymond-padding-fix.md`, `013-don-final.md`, `014-joel-final.md`.

Also re-read `_ai/` to avoid duplication; verified the candidates below
are not covered by existing files.

## Files created in `_ai/`

Five new files, all new topics. No existing files updated (the closest
adjacent files — `two-sentinels-for-two-failure-modes.md`,
`cli-flag-surface-coherence.md`, `cross-ref-content-not-line-number.md` —
cover related but distinct patterns; cross-references added in the
"Cross-link" / "Originator" sections of the new files).

### 1. `_ai/two-readers-of-one-registry.md`

Headline learning: when one on-disk source has two consumers with
different operator intents, expose two methods with different drop
policies — not one method with a `strict bool` flag. The status surface
needs every name (broken or not); the Caddyfile surface needs every
service that loads cleanly (broken ones must NOT crash routing). `List`
and `ListNames` divide the contract; the silent-skip `continue` in
`List` carries a load-bearing comment naming the Caddyfile dependency.
Locked by a three-test triangle (skip-on-`List`, no-skip-on-`ListNames`,
agree-when-clean).

### 2. `_ai/presentation-string-in-domain-struct.md`

Headline learning: `ErrorDetail string` beats `error` when the field is
printed verbatim and never matched. One-way demotion via `err.Error()`
is the point — no chain traversal, no accidental sentinel re-routing,
trivial serialisation. The load-bearing doc comment ("NOT rendered in
stdout; CLI prints to stderr") is the only thing preventing future
sixth-column drift. The three-condition test (one consumer, no
categorisation, struct already presentation-shaped) tells the next
maintainer when to apply.

### 3. `_ai/tabular-output-contract-tests.md`

Headline learning: tests for tabwriter-using commands assert on header
field presence, body-row presence by `(name, state)` via
`strings.Fields`, and row ordering — NOT on tabwriter's byte output.
Column widths shift with the widest cell; byte-equal assertions are
change-detector tests. The carve-out: stderr-detail substring assertions
ARE allowed because the five-column shape of stdout IS a contract
surface. Helpers (`headerFields`, `assertRowPresent`, etc.) encode the
contract one level below the test bodies.

### 4. `_ai/cobra-arg-count-widening.md`

Headline learning: widening `ExactArgs(N)` to `MaximumNArgs(N)` is
three changes (`Use:` bracket convention, `Args:`, `RunE` dispatch on
`len(args)`) and zero exit-code matcher updates. The `"accepts"`
substring in `isCobraUsageError` matches both Cobra error strings, so
`>N` args still route to `ExitUsageError` for free. The regression lock
is a forward-looking test (passes today, would fail under a future
over-wide `MaximumNArgs(5)` slip).

### 5. `_ai/doc-examples-verified-not-typed.md`

Headline learning: monospaced example blocks in `_docs/` purporting to
be CLI stdout are a byte-level contract surface. Hand-typing to
"approximate" tabwriter output is the drift class that publishes docs
that lie about the binary's behaviour. Generate the bytes (one-off Go
file with the same writer config), diff before commit, leave a
comment if the generator is non-trivial. This is the surface-4
companion to `_ai/tabular-output-contract-tests.md` (which carves
column widths OUT of the test layer; the carve-out is correct, but it
means the test suite cannot guard the docs).

## Considered but NOT preserved

- **Mock regeneration scope discipline.** Joel's tech plan §4 predicted
  three mock-file diffs (two non-empty, one empty) and Rob's
  implementation matched. Useful tactical advice for the next person
  adding an interface method, but the "stop if anything else moved"
  safety check is implicit in any disciplined `git status` review and
  doesn't generalise into a reusable pattern — it's a Joel-style
  habit, not a `_ai/` line. Not preserved.

- **Two-pass stdout-then-stderr write ordering.** Joel's `runStatusAll`
  flushes the tabwriter first, then iterates the same slice to emit
  stderr lines for rows with non-empty `ErrorDetail`. Deterministic
  for the *process*; kernel-level FD scheduling can still interleave
  when an operator pipes `2>&1` to the same file. Linus called this
  out as Risk B (`04-linus-review.md`); the polish-vs-correctness
  trade-off is per-call-site, not generalisable. Not preserved.

- **Double `loading service:` prefix on stderr.** `StatusAll` populates
  `ErrorDetail` with `err.Error()`, which for the per-service-Load-
  failure path already contains the `loading service:` wrap from
  `Status`. The stderr line then prefixes again with `status: <name>:`,
  producing `status: foo: loading service: registry: schema_version...`.
  Linus's exec review called this Observation A and explicitly tagged
  it polish-not-a-bug. A one-line `strings.TrimPrefix` solves it if an
  operator complains. Not preserved — it's a known-and-deferred polish
  item, not a learning.

- **`fs.ErrNotExist` returning `(nil, nil)` from `ListNames`.** Matches
  `List`'s existing behaviour. The "fresh install host works without
  bootstrap" rationale generalises into a broader pattern about
  optional-state directory reads, but it's already implicit in
  `_ai/optional-input-two-layer.md` (leaf returns no-op for empty
  input). Adding a separate file for "missing dir == empty result"
  would be redundant. Not preserved.

## Files not touched

- No existing `_ai/*.md` file edited. The closest neighbours
  (`two-sentinels-for-two-failure-modes.md`, `cli-flag-surface-coherence.md`,
  `optional-input-two-layer.md`, `cross-ref-content-not-line-number.md`)
  cover related but distinct patterns; the new files link to them in
  their cross-reference sections rather than merging in.
- No `_ai/decisions/*.md` file edited. The status surface is a CLI
  feature, not an M-series scope decision.

## Quality bar

Each new file has:

- A one-line headline rule (the title).
- A live-example code pointer with file:line refs verified against
  HEAD.
- A "when to apply" / "when NOT to apply" pair.
- An "anti-pattern" section.
- An "Originator" section with task-path + section references for
  future archaeology.
- Cross-links to adjacent `_ai/` files where the pattern boundary is
  fuzzy.

Per the cite-discipline from `_ai/cross-ref-content-not-line-number.md`,
code-side cites use file:line (Go file structure is stable) and
prose-side cites use section names (line numbers in Markdown rot).

— Ward
