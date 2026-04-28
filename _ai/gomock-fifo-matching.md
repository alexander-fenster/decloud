# gomock matches expectations FIFO, not LIFO

When a test harness installs a permissive `AnyTimes()` default for a mock method and a specific test then registers a more-specific expectation for the same method, the specific expectation does NOT automatically win. `go.uber.org/mock` matches expectations in FIFO insertion order, so the harness default — registered FIRST — claims every matching call.

## The trap

The plan in `_tasks/2026-04-28-deploy-cleanup-on-interrupt/03-tech-plan.md` (and the gomock godoc note about `WithOverridableExpectations`) suggested LIFO precedence: a later, specific expectation overrides an earlier, broader one. Kent verified empirically against `go.uber.org/mock@v0.4.0/gomock/callset.go:96-112` — the iteration is FIFO. The harness default that was supposed to be overridable wins instead.

Symptom in this codebase: `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` registers `Inspect("decloud-foo") → "running"` to exercise the hasPrev stop-failure path. With a harness default of `Inspect(Any, Any) → "absent"`.AnyTimes() registered first, the test sees `state == "absent"`, skips the hasPrev branch entirely, and fails for the wrong reason.

## The fix: explicit opt-out per non-default test

The harness exposes a `harnessOption` that suppresses the default for tests that need a non-`absent` Inspect on the request path. From `internal/deploy/service_test.go:106-113`:

```go
// withoutInspectAbsentDefault opts the harness out of installing the default
// Inspect → absent AnyTimes expectation. Required by tests that need a
// non-absent Inspect response on the request path (gomock matches
// expectations in FIFO insertion order, so the harness default would
// otherwise win against a test-supplied expectation).
func withoutInspectAbsentDefault() harnessOption {
    return func(c *harnessConfig) { c.skipInspectAbsentDefault = true }
}
```

Tests that exercise the §3.5 defensive-orphan branch with a non-absent state — and the existing `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` that needs Inspect on a different path — pass `newDeployerHarness(t, withoutInspectAbsentDefault())`. The default still applies to the majority of tests that just need "no orphan, please skip the §3.5 branch."

## Why an explicit opt-out beats matcher gymnastics

Alternative shapes considered and rejected:

- **Use `gomock.Not(specificName)` in the default** — couples the harness to specific test inputs. Every new test that needs a non-absent Inspect would also need to update the harness default's matcher exclusion list.
- **Stack expectations and rely on godoc-claimed precedence** — the godoc claim is wrong (or interpreted differently than implementation behaviour). Tests pass or fail based on subtle insertion-order details that aren't visible at the test site.
- **Skip the harness default entirely; have every test explicitly register Inspect** — re-introduces the 11-edit churn the AnyTimes default was meant to eliminate.

Opt-out is one bit of state, one option, applied at the test site that needs it. Reads at the call: "this test does NOT want the absent-by-default behaviour."

## When to apply

Whenever a test harness wants to install a permissive default for a mock method to reduce churn, AND at least one test needs a different response from the same method:

1. Install the default in the harness constructor.
2. Expose an option to suppress the default.
3. Tests that need a different response declare the suppression at the call site, then register their own expectation.

Don't rely on gomock's expectation-matching order to do precedence for you.

## Locked in by

- The opt-out itself: `withoutInspectAbsentDefault()` and `harnessConfig.skipInspectAbsentDefault` (`service_test.go:100-113`).
- The inline comment at the harness default site (`service_test.go:145-155`) cites `06-linus-review-v2.md` and `08-kent-tests.md` so future-Kent doesn't re-discover the FIFO surprise the hard way.
- The single existing test that needed the opt-out: `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` (`service_test.go:298`).
- Every new orphan-cleanup test in this task (lines 391, 419, 443, 499, 525, 590) opts out, since they all want a non-absent Inspect.

## Cross-reference

`gomock-inorder-sequencing.md` covers the orthogonal "use `gomock.InOrder` to pin orchestrator step ordering" pattern. The two combine: the harness default suppresses Inspect from the InOrder spec when a test doesn't care about it, and `gomock.InOrder` locks the order when it does.
