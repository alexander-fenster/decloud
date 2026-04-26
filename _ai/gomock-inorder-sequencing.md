# `gomock.InOrder` for orchestrator-step sequencing

When step ordering is part of the contract (network must exist before container runs; secrets must save before config; validate must precede atomic-rename), pin the order with `gomock.InOrder` — not by inspecting state, not by counting calls.

## Pattern

`internal/deploy/service_test.go:138-150` (`TestDeploy_HappyPathFirstDeploy`):

```go
gomock.InOrder(
    h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil),
    h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil),
    h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound),
    h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img-id", nil),
    h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil),
    h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil),
    h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
    h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
    h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
    h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
    h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
)
```

This is a **contract test, not an implementation test** (Linus's distinction in `20-linus-rereview.md`). A future refactor that moves `NetworkEnsure` later in the orchestrator fails with a "missing call" gomock error rather than a state assertion mismatch. The test reads like the spec.

## Why this beats alternatives

- **State machines / spies** — couple the test to internal recording surfaces. Refactors that rename a step break the test even when the contract holds.
- **`Times(1)` only** — does not detect step-reordering; a wrong-but-happy-path will still pass.
- **Per-step assertions** — readable but `gomock.InOrder` enforces the same property in fewer lines and survives interspersed allowed-but-unordered calls (use `gomock.InAnyOrder` for those).

## When to use

- Multi-step orchestrators where order matters for correctness (M1's `Deploy`, future `BlueGreen` in M4, `Backup` in M6).
- Cross-component sequencing (validate-before-swap, save-before-reload).
- Anything where a debugger trace + comment is your current contract documentation.

Co-located instances in `service_test.go:160, 250, 298, 450` cover redeploy, build-failure rollback, save-failure rollback, and step-7b orphan-config cleanup respectively.
