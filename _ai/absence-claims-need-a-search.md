# A presence claim is confirmed by an observation; an absence claim requires a search

Linus's synthesis from the caddy-http-compression task, and the only thing on it that generalizes
beyond compression:

> **A presence claim is confirmed by an observation. An absence claim requires a search — and the
> search must range over the space the claim quantifies, not over the artifact in front of you.**

Four errors survived that task. **Every one was absence-shaped. Not one false positive survived.**
Four agents, four different artifact types, one bug:

| Claim | Whose | The unsearched absence |
| --- | --- | --- |
| "`init()` is **only** called from `Write`" | Linus's, inherited by Joel | *no other call sites* — read one function, never grepped for `init()` (there were two) |
| "these states must not warn" | Kent's test table | *no un-tabled state* — enumerated the **code's branches**, not the **property's states** |
| "all **three terms** are pinned" | Kevlin's mutation report | *no unpinned term* — counted his **rows**, not the **operators**; he'd mutated two terms plus a whole statement |
| "the record is complete" | Don's + Joel's doc spec | *nothing missing from the list* — nobody asked what **wasn't** on it; CRIME fell out of a ten-item list |

The verification culture on that task worked perfectly on everything that was *there*: every
affirmative claim anyone made got caught, by source reads, mutation, grep, and probe. **It had no
mechanism at all for what wasn't.**

## The recipe, per claim shape

Each of these is **one command**. Each was skipped by someone who was, in that same document,
unusually rigorous:

- *"`init()` is only called from `Write`"* → search the **call sites** (`grep -n "init()"`); don't read one.
- *"all three terms are pinned"* → enumerate the **operators**; don't count your rows.
- *"the record is complete"* → grep the **originating request's own nouns** and look at the zeroes;
  don't check the items on your own list.
- *"these states must not warn"* → enumerate the **property's** states; don't enumerate the code's branches.

**That's the point: rigor about what is present generates the *feeling* of having verified, and the
feeling is what stops the search.** The two errors that survived longest were Don's and Linus's — the
two who spent the most words telling everyone else that being told is not evidence. Not ironic; the
mechanism. **Authority is the thing that stops the search, and it stops it hardest in the person who
has it.**

### Fifth data point: this file's own author, ten minutes later

Recorded because it's the best evidence the rule generalizes. Writing the entry you are reading, Ward
checked whether `_ai/MEMORY.md` indexed the task's new files, found **four** unindexed, fixed them, and
wrote *"the index is now complete"* — pasting the verification loop without running it. Running it over
the full domain turned up **six more**. Same shape, one file away from the transcription:

- *"the index is complete"* is an absence claim — **no unindexed file exists**.
- Confirmed by observation of the artifact in front of him: the four files he happened to touch.
- The search had to range over every file in `_ai/`. One command. `_ai/MEMORY.md` had **eleven** zeroes.
- **Fixing four generated the feeling of having audited the index, and the feeling stopped the search.**

It caught its own author on the one library claim the librarian is supposed to be authoritative
about — which is, per Linus above, exactly why it caught him. Trace:
`_tasks/2026-07-17-caddy-http-compression/012-ward-knowledge.md` §3.1.

**Standing check for this library** (an index is an absence-hiding structure by construction — it
presents its own contents as the domain):

```sh
for f in _ai/*.md _ai/decisions/*.md; do b=$(basename "$f"); [ "$b" = MEMORY.md ] && continue;
  grep -qF "$b" _ai/MEMORY.md || echo "MISSING $f"; done
```

Run it when adding a file. And read a file before you index it — an index line derived from a filename
is a presence claim about something nobody opened.

## Numbered lists are absence-hiding structures

Joel's sentence, kept verbatim because it's the sharpest thing the task produced:

> **"A numbered list is where completeness goes to die — it reads as rigor, so reviewers check the
> items in it and never ask what isn't."**

A list presents its own contents as the domain. Reviewers audit the items; nobody audits the domain.
Linus endorsed Joel's ten-item doc spec **item by item** in `005` — which is exactly how it defeated
him. CRIME was named in the user's request, singled out in review as *"correct and worth making"*, and
then silently failed to make the trip into the spec. Raymond shipped precisely what both planners
specified. They specified wrong.

Corollary, Linus's: **praising a point in review is not requiring it in a spec.** If a spec enumerates,
someone must diff the enumeration against the source request.

## Coverage check against the originating request

Don's rule, and the cheapest form of the search above. **A permanent record needs a coverage check
against the originating request, not only an accuracy check against the code.** Take the user's own
nouns, grep the durable artifact for each, look at the zeroes:

```
for n in BREACH CRIME already-compressed streaming CPU Content-Length range Vary zstd; do
  printf '%s %s\n' "$n" "$(grep -ric "$n" _ai/ _docs/ | awk -F: '{s+=$2} END {print s+0}')"
done
```

Zero zeroes = closed. That is exactly how both gaps were found, and it took one command.

Two traps in aiming it, both live on this task:

1. **Aim it at `001`, not at the framing you were handed.** Don reconstructed "the user's nouns" from
   the coordinator's summary and attributed *"gzip vs zstd ordering"* to a request that never
   contained it (`grep -c` → 0). **He skipped his own rule on the very input he was writing it about.**
2. **Omission-shaped gaps produce no error to notice**, so they need a checklist. Raymond's `008` §B3
   is the honest post-mortem: his §6 verification was thorough about everything he *wrote* — `grep -F`
   on every literal, source-reading every claim, an empirical TOML probe — and had **no step that
   walked the request asking "is each thing it asked for answered?"** Every check was artifact-inward.
   `grep -ri crime _ai/` is two seconds and would have caught it before any review.

## Test tables: enumerate the property, not the condition

Kent's diagnosis, which beats the finding it explains:

> **"I tabled the cases the implementation suggested, not the cases the property required."**

His table's negative rows covered the two cases nobody would plausibly break, while the mutation a real
reviewer would actually propose ("why is this condition three terms? simplify it") walked straight
through. The missing row was `ordinary_redeploy_never_disabled` — **the most common state in the entire
product**, absent because it is uninteresting from the *condition's* point of view and the only
interesting one from the *operator's*.

The lesson is not "add more rows." Negative cases derived from an `if`'s shape test the `if`; negative
cases derived from the property test the product. **A negative row that names a property in its failure
message and never exercises the state that property protects is decoration.** Live table:
`internal/deploy/service_test.go:TestDeploy_WarnsWhenCompressionReEnabledOnRedeploy`.

## Originator

`_tasks/2026-07-17-caddy-http-compression/{002-don-plan.md §10.2-§10.3, 003-joel-tech-plan.md §13.6,
008-raymond-docs.md §B3, 009-linus-code-review.md §8.3-§8.4, 011-kent-mutation-resolution.md §4}`.
Companion: `_ai/false-greens.md` (the tooling half — the searches that ran but answered a different
question).
