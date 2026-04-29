# READMEs for mid-build projects: dated Project Status before any install instruction

A README for a project that ships some milestones and not others must lead with **honest, dated, prominent project-state framing** before any install steps. Otherwise the reader spends `go install` time on a tool whose front page implied features it doesn't have, and the maintainer pays for that misled-reader debt forever.

## Shape

Section ordering, top to bottom:

1. H1 + two-sentence elevator pitch.
2. **Project status** — dated ("As of April 2026"), declarative ("Decloud is mid-build"), one bullet per shipped milestone (with the `(SHIPPED)` tag), one bullet per planned milestone (with the `(PLANNED)` tag), and a single forward link to the Roadmap section.
3. Quick start.
4. Everything else.

The Quick start cannot come before the Project Status. A reader who skips to Quick start and follows it to a working `decloud --help` then discovers that "Decloud" doesn't include backups, a client binary, or a bootstrap script has been ambushed. Better to lose them at "Project status" than burn their evening.

## Tone discipline

The Project Status section is the part most likely to drift apologetic. The discipline:

- **No "we're sorry."** "Backups are not yet shipped (M6)" is a fact. "We haven't finished backups yet, sorry" is an apology. Cut.
- **No "still missing."** "Still" implies an obligation; the project doesn't owe the reader features it never promised. "Not yet shipped" is the right phrasing — the "yet" preserves forward intent without conceding lateness.
- **No "should eventually."** Either it's on the roadmap with a milestone tag or it isn't.
- **Dated.** "As of April 2026" is a load-bearing token: it tells the reader this is a snapshot they should sanity-check against `git log` if they're reading it months later. Without the date, "is mid-build" becomes uncalibratable.
- **Uniform tags.** Every milestone gets `(SHIPPED)` or `(PLANNED)`. No `(IN PROGRESS)`, no `(DONE)`, no `(NEXT UP)`. Two states keep the table grokkable; more states reward over-classifying.

Live exemplar: `README.md:7-22` at HEAD on `task/readme-and-license`. Linus's read in `007-linus-execution-review.md` §3.3: *"the 'Not yet shipped' lead-in is bare and direct. The status tags `(SHIPPED)` and `(PLANNED)` in the Roadmap are uniform and unromanticized."*

## Don't write a design doc and call it a README

Distinct failure mode that this rewrite cleaned up: the pre-rewrite `README.md` was 278 lines of pre-M1 mid-level design narrative ("Operating Model", "Workload Types", "Deploy Lifecycle", "CLI Shape", etc.). That content is correct and load-bearing — but its home is `_tasks/2026-04-26-readme-implementation-planning/` and `_ai/decisions/`, not the README. The README's job is to orient a stranger, not to host the design.

The Architecture section in a stranger-facing README is one short paragraph (~5 lines) — "Architecture in 60 seconds" is the right size. If it's growing past 12 lines, it's metastasizing into a design doc and the rest belongs elsewhere.

## What the README does NOT need at this maturity

For a one-maintainer pre-1.0 repo:

- No badges, no logo, no shields. Cosmetic theatre.
- No FAQ — there are no users yet beyond the maintainer.
- No code-of-conduct, no PR template — one-maintainer repo. Add when there's a second maintainer.
- No "Why X vs Y" comparison matrix. The elevator pitch's two-sentence form is sufficient.
- No `CHANGELOG.md`. The `_tasks/` directory is the de-facto changelog.

These get cut explicitly in Don's plan §3.2 + §7 (`_tasks/2026-04-29-readme-and-license/02-plan.md`) and reaffirmed by Linus §6.5.

## Originator

`_tasks/2026-04-29-readme-and-license/02-plan.md` §3.1 (section ordering), §6 (status framing draft), §7 (cut list); `008-don-final.md` §6.2 L4 (Don explicitly routed the pattern to Ward).
