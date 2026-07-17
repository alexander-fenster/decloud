# 006 — Kent's Tests: HTTP compression in the generated Caddyfile

> Status: EXECUTION step 1. Tests written per `003-joel-tech-plan.md` rev 2 §6 (tests 1-14), after the
> §7 step-1 field stubs. **Rob starts at §5 of Joel's plan.** Nothing here implements behavior.
>
> **Verification discipline: byte-asserted; pending operator `caddy validate`.** No Docker on this
> box, so every generator assertion is a string assertion on the emitted body — a proxy, not proof.
> Production validates on every deploy/reload at `service.go:413`. I am not claiming validation.

## 1. What landed

### 1.1 Field stubs (Joel §7 step 1 — required for the tests to compile)

These are declarations only. **No behavior reads them yet; that's Rob's §5.**

| File | Change |
| --- | --- |
| `internal/registry/types.go:19-30` | `DisableCompression bool \`toml:"disable_compression"\`` + the long *why* comment from Joel §5.1, placed after `Strategy`, above `Readiness`. |
| `internal/caddy/generator.go:16-22` | `GeneratorInput.DisableCompression bool`. |
| `internal/deploy/service.go:62-65` | `Request.DisableCompression bool` + the one-line CLI-name-asymmetry comment Joel §5.4 asked for. |

`schema_version` untouched (still `1`). `mocks/mock_generator.go` untouched — the `Generator`
interface signature did not change. No loader validator added (a bool has no invalid value).

### 1.2 Tests

| # | Test | File |
| --- | --- | --- |
| 1 | `TestGenerator_EmitsCompressionByDefault` | `internal/caddy/generator_test.go` |
| 2 | `TestGenerator_CompressionPrecedesReverseProxy` | " |
| 3 | `TestGenerator_DisableCompressionOmitsEncode` | " |
| 4 | `TestGenerator_MixedCompressionSettings` | " |
| 5/7 | `TestGenerator_MultiHostnameServiceEncodesEveryBlock` | " |
| 6 | `TestGenerator_EmptyInputProducesHeaderAndGlobalBlock` (extended, no new func) | " |
| 8 | `TestGenerator_EncodeIsNotInGlobalOptionsBlock` | " |
| 9 | `TestStore_RoundTripsDisableCompression` | `internal/registry/store_test.go` |
| 10 | `TestStore_LoadDefaultsDisableCompressionToFalse` | " |
| 11 | `TestDeployService_NoCompressionFlagSetsRequest` | `internal/cli/deploy_service_test.go` |
| 12 | `TestDeployService_BuildsExpectedRequest` (extended, no new func) | " |
| 13 | `TestDeploy_PersistsDisableCompression` | `internal/deploy/service_test.go` |
| 14 | `TestDeploy_WarnsWhenCompressionReEnabledOnRedeploy` (3 rows) | " |

Per Joel §6.1: **`makeService` keeps its variadic signature.** The two generator tests that need the
flag mutate the returned struct (`svc.Config.DisableCompression = true`). No new helper.

## 2. Current state — RED, in the expected way

`go build ./...` clean, `go vet` clean, `gofmt -l internal/` clean. **Only the new tests fail; zero
regressions** in any other package.

```
FAIL internal/caddy   TestGenerator_{EmitsCompressionByDefault,CompressionPrecedesReverseProxy,
                                    MixedCompressionSettings,MultiHostnameServiceEncodesEveryBlock}
FAIL internal/cli     TestDeployService_NoCompressionFlagSetsRequest
FAIL internal/deploy  TestDeploy_PersistsDisableCompression
                      TestDeploy_WarnsWhenCompressionReEnabledOnRedeploy/reset_without_flag
ok   internal/registry
```

Representative failures — each names the missing implementation, not a test bug:

- generator: `"...foo.example.com {\n    reverse_proxy decloud-foo:8080\n}\n" does not contain "\n    encode zstd gzip\n"`
- cli: `unknown flag: --no-compression`
- deploy 13: `saved.Config.DisableCompression: Should be true`
- deploy 14: the captured JSON log `does not contain "--no-compression"`

### 2.1 Three things that are green *now* and must stay green — read this before you touch them

1. **`internal/registry` is fully green already.** Tests 9 and 10 pass off the field stub, because for
   the store layer the field + TOML tag *is* the implementation. They are contract locks, not TDD red.
   Test 10 is the one that fails the day someone "fixes" the polarity to `enable_compression`.
2. **Tests 3 and 8 pass vacuously today** (the generator emits no `encode` at all, so "no encode in
   the opted-out block" and "no encode in the global block" are trivially true). They only acquire
   teeth once you emit the directive. **If either one goes red after your change, you have emitted
   `encode` in the wrong place** — 3 means the flag isn't suppressing it, 8 means it landed in the
   global options block (which would fail `caddy validate` on the operator's host, exit 60).
3. **Test 14's two negative rows pass today** (nothing warns). Same deal — they exist to catch a
   warning that fires when it shouldn't. `first_deploy_has_no_previous_config` is the nil-`prev`
   guard: if you drop `hasPrev` from the condition, that row **panics**, it doesn't just fail.

## 3. Implementation notes for Rob

Everything you need is Joel §5. Additions from writing the tests:

### 3.1 The generator (§5.2c) — the `if` goes in the INNER loop

```go
for _, host := range in.Hostnames {
    fmt.Fprintf(&buf, "%s {\n", host)
    if !in.DisableCompression {
        fmt.Fprintln(&buf, "    encode zstd gzip")
    }
    fmt.Fprintf(&buf, "    reverse_proxy %s:%d\n", in.ContainerName, in.Port)
    fmt.Fprintln(&buf, "}")
}
```

Hoist it to the outer loop and `TestGenerator_MultiHostnameServiceEncodesEveryBlock` fails with
`expected: 2, actual: 1`. Four spaces, not a tab. Exact string `encode zstd gzip`.

Don't forget `normalize` (§5.2b) — carry `svc.Config.DisableCompression` into the input.
`TestGenerator_MixedCompressionSettings` is the one that catches a flag smeared onto the wrong service
by `normalize`'s `sort.Slice`.

### 3.2 The warning (§5.3.1) — assertion contract

Test 14 asserts **only the two semantic tokens**, per Joel §5.3.1 and the
`_ai/cli-flag-surface-coherence.md` carve-out:

- `--no-compression` — the operator's fix
- `disable_compression` — what they see in the TOML

**Polish the prose around those two tokens freely; do not drop either token.** I deliberately did not
assert the sentence — that would be a change-detector.

Condition, all three terms, `hasPrev` first:

```go
hasPrev && prev.Config.DisableCompression && !req.DisableCompression
```

Call site: immediately before `svc := &registry.Service{` at `service.go:317`.

### 3.3 Test 14's log capture — a live constraint on your code

`captureDeployLogs` swaps `slog.SetDefault` to a `bytes.Buffer` JSON handler and restores it in
`t.Cleanup`. This works **only because `service.go:157` derives `logger` from the default logger**
(`slog.With(...)`). If you replace that with an injected logger or a package-level handler, test 14
goes dark — tell Kent, don't quietly re-point the test.

Empirical note: the capture confirms Joel's `logging.go:43-44` premise from the other side — `Warn`
clears the `LevelInfo` filter and lands in the handler's writer. Option C's premise holds.

`slog.SetDefault` is process-global: **no `t.Parallel()`** in that test (and none in `internal/cli`
either, per `deployerFactory`).

### 3.4 New shared helper in `internal/deploy/service_test.go`

`expectHappyPathDeploy(h, prev)` installs the standard happy-path expectations around a caller-supplied
`Load` result; `prev == nil` makes it a first deploy (`ErrNotFound`, no stop/remove). It exists because
test 14's three rows differ *only* in the `Load` result and the request flag — inlining it three times
would have buried the one line that varies. `Save` is left to the caller: test 13 needs `DoAndReturn`
to capture, test 14 only needs `Return(nil)`.

Per Joel §6.5 I did **not** retrofit capture into the other ten `Save` expectations. One test closes
the seam; the rest would be churn for no signal.

## 4. What I did not test, and why

- **Help text** — change-detector, banned by CLAUDE.md. §5.4's four-surface coherence is review
  discipline (Kevlin), not test enforcement. There is no runtime check and no error message for this
  flag, so only `--help` and `_docs/usage.md` exist as surfaces, and they must agree.
- **That Caddy actually compresses anything.** We generate text; we don't run Caddy.
- **`caddy validate`.** Impossible here — no Docker. Operator's Linux host. See the header.

## 5. Note for Linus/Kevlin at review

The §11.1 tripwire counts **production** lines. My diff adds ~14 non-test lines, of which ~11 are the
`DisableCompression` doc comment Joel §5.1 specified verbatim; the actual field/stub declarations are
3 lines and are step-1 stubs, not implementation. Rob's diff should land the remaining ~8.
