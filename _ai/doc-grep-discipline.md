# Doc-fab: example error strings must `grep -F` against the source

When `_docs/*.md` shows the operator an error string in a fenced block — "you will see this" — the displayed bytes must be producible by code on disk. Cycle 1 of the caddy-container task shipped two fabrications:

- `_docs/install.md:173` displayed `caddy up: ports 80/443 already in use` while the code actually emitted `caddy: up failed: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or ...`. The `caddy up:` prefix never existed; `ErrCaddyUp.Error()` was `caddy: up failed`. Operators grepping logs for the documented prefix would find nothing.
- `_docs/install.md:189` displayed `caddy up: docker run: listen tcp [::]:80: socket: address family not supported by protocol` while the real wrap was `caddy: up failed: docker run: docker run: exit status N; stderr="..."` (yes, the doubled `docker run:` is real — manager wrap plus driver wrap).

Both were caught in `_tasks/2026-04-27-caddy-container-connection-refused/011-kevlin-review.md` and re-checked in cycle-2 review (`018-linus-impl-review-cycle2.md` §4).

## Discipline

For any error string a doc displays as something the operator will see literally:

```
grep -F "<the displayed substring>" $(find internal -name '*.go') $(find _docs -name '*.md')
```

Both files must match. If the source doesn't contain the literal, either:

1. The doc is fabricated — fix the doc to match the code, OR
2. The error wrap is wrong — fix the code to match the documented contract.

Never paper over the gap with prose like "you will see something similar to."

## When fabrication is unavoidable

If the surrounding bytes are genuinely variable (Docker daemon prefixes, kernel-version-dependent stderr, log timestamps), don't fabricate a precise example. Show the diagnostic substring in inline code, then frame the surrounding output as variable:

> "fails with stderr containing `<the diagnostic substring>`. The raw `docker run` stderr is surfaced as-is; it typically reads similar to: `docker: Error response from daemon: ...<substring>...`"

Use ellipses inside the fenced block to signal "this part varies." Do not commit to bytes the code does not emit. Pattern lives in `_docs/install.md` §IPv6-listener block (post-cycle-2).

## Why this is a recurring class

Error wrap chains compose: the manager wraps with one prefix, the driver wraps with another, the OS contributes its own. Each layer adds bytes. Doc writers tend to render a "clean-looking" version that drops one of the layers. That version is not what the operator sees.

The bug class generalizes: any time a doc shows a literal the operator will compare against terminal output, the displayed bytes are a contract with the code. Treat them as such.

## Slog messages quoted in operator runbooks are also a contract

The same discipline applies when `_docs/*.md` quotes a slog message that operators are told to grep their audit log for. The deploy-cleanup-on-interrupt task changed two slog phrases in iter2 (`"removed orphan ..."` → `"removing orphan ..."`, and `"cleanup failed; please remove X manually"` → `"cleanup failed; manual removal may be required"`). Both production sites shipped clean; the doc had to chase. Joel pre-flagged the first quote drift (`_docs/usage.md:237`) in `03-tech-plan.md` §13.8; Linus caught the second (`usage.md:235`) on iter2 re-review (`20-linus-impl-review-iter2.md`). Two passes, two drifts, same shape.

Recipe: when changing a slog `Warn`/`Error`/`Info` message string, immediately `grep -F "<old phrase>" _docs/` and update every hit in the same diff. Don't trust the production-side change alone.

## Originator

`_tasks/2026-04-27-caddy-container-connection-refused/011-kevlin-review.md` (cycle-1 catch), `017-raymond-docs-cycle2.md` (cycle-2 fix), `018-linus-impl-review-cycle2.md` §4 (post-fix verification with the literal `grep -F` recipe above). Slog-message extension: `_tasks/2026-04-28-deploy-cleanup-on-interrupt/03-tech-plan.md` §13.8 + `20-linus-impl-review-iter2.md`.
