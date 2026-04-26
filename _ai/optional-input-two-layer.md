# Two-layer optional-input pattern

When an input is optional (an env file, a config override, a feature flag), defend the contract at TWO layers: the leaf consumer returns a no-op for the empty case, and the orchestrator guards before calling. This pattern landed in M1 for `env.sh` capture and is the right shape whenever a caller might or might not have the input.

## The two layers

**Layer 1 — leaf consumer defensive return.** `internal/envcap/capture.go:46-49`:

```go
func (b *bashCapturer) Capture(ctx context.Context, scriptPath string) (map[string]string, error) {
    if scriptPath == "" {
        return nil, nil
    }
    ...
}
```

The contract: real path = real work; empty = no work. The leaf does not crash, does not error, does not surface anything weird up the stack.

**Layer 2 — orchestrator guard.** `internal/deploy/service.go:137-149`:

```go
envFile := req.EnvFile
var captured map[string]string
if envFile != "" {
    c, err := d.deps.Capturer.Capture(ctx, envFile)
    if err != nil { ... }
    captured = c
    logger.Info("env captured", ...)
} else {
    logger.Info("env capture skipped: no env script", "step", "envcap")
}
```

Owns the decision of whether to call. Logs the skip explicitly so operators can grep audit trails for "did the env capture happen on that deploy?"

## Why both, not one or the other

- **Leaf-only** = orchestrators end up with hidden no-op calls; logging the "we did nothing" event is awkward to retrofit; mock tests have to expect a `Capture("")` call instead of `Times(0)`. Test like `TestDeploy_NoEnvScript_SkipsCapturerEntirely` would not be expressible.
- **Orchestrator-only** = a future bypass caller (a script, a different orchestrator, a refactor) hits a stat-on-empty-path bug. The leaf becomes a footgun.

Both layers, no coupling. Linus called this out in his iter2 re-review praise (`20-linus-rereview.md` §Praise).

## When to apply

- Optional file paths (config, secrets, env scripts).
- Optional command-line flags whose presence triggers an external call.
- Optional dependency injection (a hook, a metrics sink) where "absent" should be a clean no-op.

Match the test pattern too: write `EXPECT().X().Times(0)` at the orchestrator level, plus one direct test of the leaf's empty-input defensive return.
