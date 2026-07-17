# False greens: three tools that report success while answering a different question

All three bit on one task (caddy-http-compression). Same shape every time: **the tool returned green,
the green was real, and it was not an answer to the question being asked.** Test the output, not the
feeling.

## `gofmt -l` exits 0 whether or not it lists files

`gofmt -l` **always exits 0** on success — "success" means *it ran*, not *nothing was listed*. So:

```bash
gofmt -l internal/ && echo clean      # ← FALSE GREEN: prints "clean" while listing unformatted files
```

This is a false green in **any** CI-style `&&` chain. Test its output for emptiness, not its exit code:

```bash
test -z "$(gofmt -l internal/)" && echo clean
```

Rob hit it (`007` §9) and caught it only because he *printed the list*. Worth knowing why it bit: he
deleted a one-line comment from the `deploy.Request` literal in `internal/cli/deploy_service.go`, which
**merged the literal back into a single gofmt alignment group and re-aligned every field in it**. A
comment deletion is not always the one-line change it looks like.

## `go test` returning `ok (cached)` on mutated source is not evidence

Mutation testing means editing production source and re-running. Go's test cache keys on inputs it
tracks, and Kent's first mutant run reported `ok (cached)` — **the obvious way to manufacture both a
false GREEN and a false CAUGHT.** Always:

```bash
go clean -testcache && go test ./...      # or: go test -count=1 ./...
```

Kent caught his own (`011` §1) and re-ran; the result held. Linus re-ran the whole suite as `9/9
uncached` rather than trusting the report. Linus's note: *"A cached result on mutated source is not
evidence. That detail is worth more than the finding."*

## With parallel agents on one branch, staging is not a lock

`git add` then a later `git commit` is a **race**: `git commit` commits the whole index, so a concurrent
agent's staged files get swallowed into your commit. **This bit twice on this task** — once sweeping
Kent's staged test file into a docs commit (his own `git commit` then reported *"nothing to commit,
working tree clean"* — a green meaning *someone else committed your work*), and once around Raymond's
`_ai/` edits.

Rules:

1. **Commit with an explicit pathspec**, always: `git commit -m "..." -- internal/deploy/service_test.go _tasks/.../011-kent.md`
   — the pathspec commits exactly those paths *regardless of what else is in the index*, which is the
   whole protection. **Wrinkle: a pathspec cannot name an untracked file** (`did not match any file(s)
   known to git`). New files need `git add <paths>` first — add them **narrowly, never `git add -A`/`.`**,
   which is the very sweep that causes this — then commit with the full pathspec as usual.
2. **Verify what actually landed** — `git log -S '<a string from your own change>'` — rather than
   trusting that a green `git commit` committed *your* work.
3. **Resolve at the source, don't rewrite shared history to chase it.** The Kent incident was fixed by
   amending the offending commit to drop his files, restoring them to his staging area; the final
   history is correct and the log shows no trace. Nobody rewrote history anyone else had built on.

## Originator

`_tasks/2026-07-17-caddy-http-compression/{007-rob-implementation.md §9, 009-linus-code-review.md §7
+ §8.5, 011-kent-mutation-resolution.md §1 + §5.1}`. Companion: `_ai/absence-claims-need-a-search.md`
(the reasoning half — the searches that were never run at all). Prior art on the same theme:
`_ai/compile-clean-not-run-clean.md` (`go build -tags integration` is not a PASS).
