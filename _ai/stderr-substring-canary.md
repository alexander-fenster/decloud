# Stderr-substring detection: brittle by design, locked by a canary test

When code branches on the contents of a third-party tool's stderr — Docker, systemd, the kernel via Docker — the match is fundamentally brittle. A tool upgrade can reword the message and the branch silently falls through. The mitigation is not to widen the regex; it's to lock the canonical strings with a test that fails loudly if the wording shifts.

Live example: `internal/caddy/manager.go::isPortsBoundErr` matches `address already in use` (kernel `bind(2)` failure surfaced through Docker) and `port is already allocated` (Docker allocator failure). Both have been canonical Docker stderr since 20.10. The helper's doc-comment names the brittleness and points at the test.

The canary: `TestManager_UpPortsBoundActionableError` (two sub-tests, one per substring) drives `MockDriver.RunWithOptions` to return an error containing each substring and asserts the actionable branch fires. If a future Docker reworded `address already in use` to `address in use`, the test fails loudly at `go test` time — the branch goes silent in production but the canary catches it before ship.

## Pattern

1. **Match canonical strings only.** Do NOT pre-emptively widen to phrases the upstream tool doesn't actually emit ("fragility-by-imagination"). Match what's there today.
2. **Co-locate detection with its single caller.** Don't push it down into the driver; the driver is the generic primitive. The caller knows the semantic ("ports 80/443 already in use" — a Caddy-specific message) and owns the substring check. Symmetric to `internal/caddy/reloader.go::isNotRunningStderr`.
3. **Comment the brittleness explicitly.** The helper's comment names the docker-version assumption and points at the canary test by name.
4. **Lock with a test driving each substring as an independent sub-case.** Using `t.Run` per substring means a regression that fixes one but not the other still fails visibly. Add a negative `assert.NotContains` to lock the branch choice, not just the rendered text — without it, accidental fall-through that happens to include the actionable text would pass.
5. **Drop the inner driver-wrap on the actionable branch.** A single `%w` wrap of the sentinel keeps `errors.Is` working and avoids `caddy: up failed: docker run: docker run:` double-prefix noise. The actionable text already names the recovery; the driver chain is redundant here. Locked by `assert.NotContains(err.Error(), ": docker run: docker run:")`.

## Out of scope for this pattern

- **Case-folding the match.** Docker stderr for these phrases is locale-stable (kernel `errno` text via `strerror` plus C-locale Docker output). Folding costs an allocation per check; not worth it for a single-operator English-only tool.
- **Localisation.** If Docker emits translated stderr under non-`C` locale, this pattern breaks — and so will a thousand other things first. Not a concern in M1.

## Originators

- Pattern named in Joel's `_tasks/2026-04-27-caddy-container-connection-refused/013-joel-tech-plan-cycle2.md` §1.2-1.5.
- Brittleness flagged for review by Linus in `007-linus-review-v2.md` non-blocking nit #2.
- Locked in by Kent's `015-kent-tests-cycle2.md` and Rob's `016-rob-implementation-cycle2.md`.
- Verified in `018-linus-impl-review-cycle2.md` §1-2.
