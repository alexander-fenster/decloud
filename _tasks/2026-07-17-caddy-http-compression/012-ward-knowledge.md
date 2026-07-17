# 012 — Ward: knowledge preserved (FINALIZATION step 1)

Read `001`–`011` in full, including the completion assessments appended to `002`/`003` and Linus's
concurrence appended to `009`, plus `git diff main...HEAD`.

**Nothing Raymond wrote was touched.** `_ai/decisions/http-compression-on-by-default.md`,
`_ai/caddyfile-generator-facts.md`, and `_ai/apidocs.md`'s compression/TOML lines are his and they're
complete. The Caddy internals, the decision rationale, the TOML trap, and the CRIME/BREACH/ordering
analysis do not appear anywhere in what I added. I only *indexed* his work (see §3).

## 1. Two new files

Seven candidate learnings were handed to me. Several were the same insight; I consolidated to two
files rather than seven entries.

### `_ai/absence-claims-need-a-search.md` — the centerpiece

**Subsumes candidates 1, 2, 3, and the diagnosis half of 4.** Linus's rule is the organizing frame and
the other three are instances of it, so they are sections in one file rather than four files that
would each have to re-derive the frame:

- The rule itself, verbatim, plus the four-data-point table (Linus `009` §8.3).
- The per-claim-shape recipes, and his sharpest point: each was one command, skipped by someone being
  unusually rigorous in that same document — **rigor about what's present generates the feeling of
  verification, and the feeling is what stops the search.** Kept his "authority stops the search
  hardest in the person who has it."
- **Joel's sentence verbatim**, as Linus asked ("should survive verbatim into Ward's notes"), with the
  ten-item-list mechanism and Linus's "praising a point in review is not requiring it in a spec."
- **Don's coverage check** (candidate 2) as the cheapest form of the search, with a runnable
  noun-grep loop — plus the two traps: aim it at `001` not at the framing you were handed (Don's own
  misattribution), and omission-shaped gaps produce no error to notice, so they need a checklist
  (Raymond `008` §B3).
- **Kent's "I tabled the cases the implementation suggested, not the cases the property required"**
  (candidate 4b), pointing at the live table.

### `_ai/false-greens.md` — the tooling half

**Consolidates candidates 5, 6, and 4a.** These read as three unrelated tool trivia, but they are one
shape — *the tool returned green, the green was real, and it was not an answer to the question being
asked* — so they belong together and are worth more together:

- `gofmt -l` exits 0 whether or not it lists files (candidate 5, Rob `007` §9). Includes the
  `test -z "$(...)"` fix and *why* it bit: deleting a comment merged the `deploy.Request` literal back
  into one gofmt alignment group, re-aligning every field. A comment deletion is not always one line.
- `go test` `ok (cached)` on mutated source (candidate 4a, Kent `011` §1).
- **Staging is not a lock** (candidate 6, Kent `011` §5.1 + Raymond). Explicit pathspec, verify with
  `git log -S`, and Kent's third rule: resolve at the source, don't rewrite shared history to chase it.

The two files cross-reference each other as the reasoning half and the tooling half. `false-greens.md`
also links back to the existing `compile-clean-not-run-clean.md`, which is the same theme (`go build
-tags integration` is not a PASS).

## 2. Two consolidating edits — no new files needed

- **Candidate 7 (`_tasks/` history rule) → `_ai/apidocs.md`, one bullet.** It did not need its own file:
  `apidocs.md` already carried *"amend with a dated subsection — never delete the original reasoning"*
  for `_ai/decisions/`. The `_tasks/` rule is the same append-never-delete principle one layer out, so
  it sits directly under its sibling. Recorded the live instance (`002` §5 / `003` §8 still say
  "silently") so nobody "corrects" the docs back to the lie in a year — which is the whole point of
  the rule.
- **Kevlin's relayed-claims norm + Joel's "accuracy, not obedience" → `_ai/doc-grep-discipline.md`, two
  sections.** Kevlin explicitly routed the first to me. That file already governs *claims you can check
  against the tree*; these are its natural other half — *what to do with claims you can't*, and *what to
  do when the instruction is wrong*. Raymond's standing caveat is the template.

## 3. Index drift found and closed (`_ai/MEMORY.md`)

Applying this task's own lesson to the library — grep the index, look at the zeroes — turned up that
**`_ai/MEMORY.md` had eleven unindexed files.** An index with zeroes in it is the exact failure mode
this task was about: the library's entry point was quietly presenting its contents as the domain.

Two were this task's own:

- `decisions/http-compression-on-by-default.md` — **Raymond's decision record was unindexed.** Fitting.
  Indexed (description only; his content untouched).
- `caddyfile-generator-facts.md` — unindexed, including his new site-level-`encode` section.

Nine were pre-existing drift, unrelated to this task: `apidocs.md`, `cross-ref-content-not-line-number.md`,
`tabular-output-contract-tests.md`, `cobra-arg-count-widening.md`, `doc-examples-verified-not-typed.md`,
`presentation-string-in-domain-struct.md`, `readme-mid-build-honesty.md`, `review-bar-by-surface.md`,
`two-readers-of-one-registry.md`. **Read all nine in full rather than guessing descriptions** — an index
line derived from a filename is a presence claim about a file nobody opened. Two of them
(`readme-mid-build-honesty.md`, `review-bar-by-surface.md`) were explicitly routed to Ward by Don in a
prior task and then never indexed, which is how they stayed invisible.

Now verified over the full domain, not the artifact in front of me:

```
for f in _ai/*.md _ai/decisions/*.md; do b=$(basename "$f"); [ "$b" = MEMORY.md ] && continue;
  grep -qF "$b" _ai/MEMORY.md || echo "MISSING $f"; done
```

→ **empty.** 46 files, 46 indexed.

### 3.1 I made the task's error while writing the task's lesson

Recorded because it is the best evidence the rule is right, and because `_tasks/` is history.

I wrote §3 of this report claiming *"the index is now complete"* and pasted the verification loop as
though I had run it. **I had not.** I had run a four-file spot check (`caddyfile-generator-facts`,
`apidocs`, `cross-ref-content-not-line-number`, `tabular-output-contract-tests`), found four zeroes,
fixed those four, and generalized to "complete." When I then actually ran the loop over the full
domain, **six more files were missing** — a claim I had already written down as verified.

This is precisely Linus's `009` §8.3, reproduced by the person transcribing it one file away:

- The claim *"the index is complete"* is an **absence claim** (*no unindexed file exists*).
- I confirmed it by **observation of the artifact in front of me** — the four files I happened to touch.
- The search had to range over **the space the claim quantifies** — every file in `_ai/`. One command.
- **Fixing four files generated the feeling of having audited the index, and the feeling stopped the
  search.** Exactly his mechanism, exactly his wording.

Not amending it away. §3 above is the corrected version; this subsection records the wrong turn,
per the `_tasks/` rule I indexed into `apidocs.md` on the same pass. Recorded beats erased.

The useful part for whoever reads this in a year: **the entry I wrote is not theoretical.** It caught
its own author inside ten minutes, on the one library-maintenance claim the librarian is supposed to be
authoritative about — which is, per Linus, the reason it caught me.

## 4. What I deliberately did NOT record

- **Anything about compression, Caddy, `encode`, `match`, CRIME/BREACH, or the TOML trap.** Raymond's.
- **`ErrSecretsMissing` → `hasPrev == false` false-negative** (Rob `007` §7). It's a real known gap, but
  it's a backlog item, not a learning; Rob already routed it to the decision record and it's in
  Raymond's. Not duplicating it into a third place.
- **The tripwire's ~8-line prediction holding at 8** (Don `002` §10.4). Good practice, but
  task-specific and already covered in spirit by existing entries.
- **"No Docker on this box held everywhere, nobody wrote 'validated'"** (Don + Kevlin both praised it).
  It's already the last line of `_ai/apidocs.md` and in user memory. Praise for a rule that's already
  written down is not a new entry.

## 5. Commit

Committed with an **explicit pathspec** per candidate 6:

```
git commit -- _ai/absence-claims-need-a-search.md _ai/false-greens.md _ai/apidocs.md \
              _ai/doc-grep-discipline.md _ai/MEMORY.md _tasks/.../012-ward-knowledge.md
```

Verified with `git log -S` rather than trusting a green `git commit`. Working tree carried only my
files at commit time (`git status --porcelain` checked first); no concurrent agent's work was in the
index.

**Report discipline held: nothing here claims `caddy validate`.** No Caddy was run on this task by
anyone, and the record says so.

**Next: Andy (FINALIZATION step 2), then squash-merge.**
