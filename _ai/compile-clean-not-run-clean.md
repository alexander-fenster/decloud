# Compile-clean is not run-clean: opt-in tests need a real run, but they don't gate merge

Two paired rules, both surfaced by the M2 integration-test arc:

1. **A test that requires external services (real Docker, real DB, real network) needs an actual `PASS` from a real run, not a `go build -tags integration ./...` log.** Compilation proves the test type-checks, not that it works.
2. **An opt-in test (build-tagged + env-var-gated) is not the right gate for squash-merge** when the user-visible surface is independently unit-tested. The first real-Docker deploy doubles as the smoke check.

The two rules look contradictory; they're not. Rule 1 is about how you justify a "PASS" claim. Rule 2 is about what artefact pre-merge enforcement requires. Together: don't claim a test passed without running it, but also don't hold a feature ship hostage to a maintainer-only run-log when the unit tests already lock the user-visible behaviour.

## How rule 1 bit M2

Kent and Rob both reported `go build -tags integration ./...` clean as verification for `internal/integration/mount_test.go` v1. The test pulled `alpine:3.19` and called `Driver.Run` then `Driver.Exec` to read a marker file. **Nobody ran it against real Docker.** Linus caught it on impl review by reading the code: `alpine:3.19`'s default CMD is `/bin/sh`, which under `docker run -d` (no `-i`/`-t`, closed stdin) exits with status 0 immediately. `docker exec` against an exited container fails. The test could not have passed end-to-end. v2 swapped the image to `nginx:alpine` (idles in foreground via `nginx -g daemon off;`) — see `_ai/m1x-backlog.md` items 6 + 11.

Trace: `_tasks/2026-04-28-m2-server-side-mounts/{007-kent-tests.md, 008-rob-impl.md, 011-linus-impl-review.md §5}`.

## How rule 2 bit M2 (and was eventually right)

After v2 image swap, the closeout v1 (`012-don-closeout.md §1`, `013-joel-tech-plan-addendum-v2.md §5.1`, `014-linus-addendum-v2-review.md §2`) all required a maintainer-produced `integration-test-run-log.txt` showing `--- PASS:` on a Linux host before squash-merge. The agents could not produce it (Mac dev box, no Docker). The workflow stalled.

Linus reversed in `017-linus-review-v2.md §2`. Don tie-broke and dropped the gate in `018-don-closeout-vote.md §1`. Reasoning that flipped them:

- The shipped binary's user-visible mount surface was unit-test verified end-to-end (CLI parse → loader → driver argv shape, byte-for-byte locked by `service_test.go`'s three new `Mounts` tests + `volumeFlagsFromArgs` matcher).
- The integration test is opt-in (`//go:build integration` + `DECLOUD_INTEGRATION=1`). It is excluded from default `go test ./...`. **An opt-in test should not gate squash-merge.**
- The v1 bug class (alpine-no-Cmd) was already discharged by code-read review across three reviewers; the gate's residual value was defending against unspecified hypothetical regressions.
- Workflow cost (maintainer-only blocker) was disproportionate to defended scope.

## When rule 2 does NOT apply (the gate IS load-bearing)

The carve-out: when the feature's user-visible surface CANNOT be unit-tested without running on real infrastructure. Examples that would re-arm the gate:

- A Docker daemon negotiation feature (e.g., a new flag whose argv interpretation is daemon-version-specific).
- A Caddy reload semantic that depends on Caddy's actual response.
- A bash-portability fix where the bug only reproduces under bash 3.2 vs bash 5.

If the user-visible behaviour can only be observed end-to-end, the integration test is the user-visible surface and rule 2 does not apply — keep the gate.

## What "actually ran" looks like

For an integration test, the gate artefact is a tee'd run-log:

```
DECLOUD_INTEGRATION=1 go test -tags integration -v ./internal/integration/... 2>&1 | tee <task-dir>/integration-test-run-log.txt
```

Must contain `--- PASS: TestIntegration_<name>` and `PASS\nok` lines. Must NOT contain `--- FAIL:`. The hand-off note shape from M2 is the right template: `_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md` enumerates the command, the PASS line to grep, and the explicit "do NOT commit as PASS if FAIL appears" rule.

## The discipline failure to avoid

Don's framing in `018-don-closeout-vote.md §1` is the load-bearing one — paraphrased: "Netscape 4.0 shipped a user-visible feature that didn't work because the team confused 'compiles' with 'tested.' We are not at that failure mode here: we tested the user-visible surface extensively. The integration test is one *additional* layer of verification on top of that, not the *only* layer."

Translate: the discipline is "ship what you ran *for the user-visible surface*." Unit tests against gomocks count as "ran" if the argv shape is locked byte-for-byte. Integration tests against real infrastructure are an additional layer; their absence is acceptable iff the user-visible layer is independently verified.

## Originator

`_tasks/2026-04-28-m2-server-side-mounts/{007-kent-tests.md, 008-rob-impl.md, 011-linus-impl-review.md §5, 012-don-closeout.md, 013-joel-tech-plan-addendum-v2.md, 014-linus-addendum-v2-review.md, 017-linus-review-v2.md §2, 018-don-closeout-vote.md §1, integration-test-run-log-handoff.md}`.
