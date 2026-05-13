# Two readers of one data source, two failure semantics — by design

When the same on-disk state has two consumers with different operator intents, expose two methods with different drop policies. Do not try to satisfy both consumers from one method by adding flags, options, or "strict mode" parameters. One method silently skips broken entries, the other surfaces them as errors — the divergence is the contract, not a bug.

This is a different pattern from `two-sentinels-for-two-failure-modes.md` (which is one leaf-guard returning two sentinels for two reasons). Here we have two top-level methods, one underlying source, two failure semantics.

## Live example

`internal/registry/store.go` exposes two readers of the `ServicesDir`:

- `List(ctx) ([]*Service, error)` — readdir + Load each, **silently skip on per-name Load failure** (`store.go:217-219`). Caller: `internal/deploy/service.go:regenerateAndReload` for Caddyfile generation. Intent: "build a working Caddyfile from whatever loads cleanly; a broken service must NOT crash the routing layer."
- `ListNames(ctx) ([]string, error)` — readdir only; returns names regardless of load-ability (`store.go:186-207`). Caller: `internal/deploy/lifecycle.go:StatusAll` for the `decloud status` no-arg surface. Intent: "show every name the operator registered, even the broken ones — that is the whole point of a status command."

`List` is internally implemented as `ListNames` + `Load` loop, so there is one source of truth for "what files count as services," and the silent-skip is exactly one `continue` documented with `// existing silent-skip contract; Caddyfile path depends on it`.

## Why this beats a `strict bool` parameter

- A flag-driven contract surface forces every call site to think about which policy it wants. Two methods makes the *choice* part of the type system: a caller that takes `Store.List` gets the drop policy automatically; a caller that takes `Store.ListNames` gets the don't-drop policy automatically. No flag to forget.
- Mocks gain one method, not one method with conditional behaviour. Test fixtures stay simple.
- The doc comment on each method names the intent in operator-facing language. `List`'s comment names the Caddyfile dependency; `ListNames`' comment names that callers handle per-name errors themselves. Comments-as-rationale survive future refactors.

## Test discipline that locks it

Two named regression tests, both in `internal/registry/store_test.go`:

- `TestStore_List_StillSilentlySkipsLoadErrors` — corrupt one of two services; assert `List` returns one entry and `err == nil`. **Locks the Caddyfile path contract.**
- `TestStore_ListNames_IncludesNamesEvenWhenLoadWouldFail` — same fixture; assert `ListNames` returns both names. **Locks the status path contract.**

Plus a cross-check, `TestStore_ListAndListNamesAgreeWhenAllServicesLoadCleanly` — when nothing is corrupt, the two methods return the same set. Pins the refactor invariant that `List` is `ListNames + Load loop`, not a parallel implementation.

The three-test triangle is the lock: if a future "simplification" rips out one method and adds a flag to the other, at least one of these tests fails.

## When to apply

Any storage abstraction where:

1. Two callers want the same data with different tolerance for partial corruption.
2. A future maintainer might "consolidate" the two readers into one with a flag. (The triangle of tests is what stops them.)
3. The drop policy is part of the operator contract, not an implementation detail — i.e., adding a "show broken services" flag at the CLI layer would be the wrong fix because the CLI does not own the registry.

## Anti-pattern

`Store.List(ctx, includeBroken bool)` — flag-driven, every call site decides anew, comment lives in the parameter doc instead of in the method intent, mocks gain conditional behaviour, the two operator intents become indistinguishable in code review.

## Originator

`_tasks/2026-05-13-status-list-all-services/{03-tech-plan.md §1.1, 04-linus-review.md §6.2, 005-kent-tests.md §6.1}` — Don's plan named the keystone; Joel's tech plan locked the refactor shape with the load-bearing comment; Linus's plan review ratified the "two readers, two semantics" framing explicitly.
