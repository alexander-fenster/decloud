# Joel — final M1 sign-off (implementation planner)

## VERDICT: **M1 SHIPS.** Implementation matches tech plans.

I wrote `06-tech-plan-v2.md` and the iter2 delta `14-fixup-tech-plan.md`. My job here is to attest that the SHIPPED CODE matches what those two specs said. It does. Don approved DONE-criteria (`021-don-final-signoff.md`); Linus approved architecture (`20-linus-rereview.md`); Kevlin approved low-level (`019-kevlin-rereview.md`); now I close the loop on planner-fidelity.

---

## Spec-to-code verification

| Item | Spec | Code | Match |
|---|---|---|---|
| Two-file secrets split | `06-tech-plan-v2.md` §7 / prior §4.5–§4.7 | `internal/registry/store.go:119` Save (config 0o644 first, secrets 0o600 second, `ErrPartialWrite` on secrets-fail), `:152` Delete (secrets-first then dir then config), `:168` `DeleteOrphanConfig`, `:51` Load enforces 0o700 dir + 0o600 file via `ErrPermissionMode` | YES |
| `Reloader.Validate` + validate-before-rename | §8 / §9.2 step 8b | `internal/caddy/reloader.go:14` interface; `internal/deploy/service.go:302` `regenerateAndReload` does Generate→tmp→Validate→Rename→Reload with `os.Remove(tmp)` on validate failure | YES |
| `Driver.ContainerIP`, no `OneShotProbe`, host-side probe | §9, §13.6 | `internal/dockerdrv/driver.go:52` interface (no `OneShotProbe`, has `Start`+`ContainerIP`); `internal/dockerdrv/cli_driver.go` parses inspect format string; `internal/deploy/readiness.go` host-side `httpProbe` per-tick re-resolution; `grep OneShotProbe` returns zero | YES |
| One struct, two interfaces, separate `lifecycle.go`, shared `regenerateAndReload` | §15.4–§15.6 | `internal/deploy/lifecycle.go` (137 lines, 7 receiver methods on `*serviceDeployer`); `internal/deploy/service.go:302` `regenerateAndReload` shared by Deploy step 8 + Unregister step 5 + CaddyReload | YES |
| All 7 lifecycle methods | §9.6 | Unregister/Stop/Start/Restart/Status/Logs/CaddyReload — all present at `lifecycle.go:15,37,48,82,89,118,135`; step ordering and `errCaddyReload`/`errRun`/`ErrNotFound` sentinels match spec | YES |
| `mounts` rejection | §10 | `internal/registry/store.go:68` `len(cfg.Run.Mounts) > 0` after `DisallowUnknownFields`; `internal/cli/deploy_service.go:67` CLI mirror; both wrap `ErrMountsNotSupported` → exit 10. Empty array `mounts = []` accepted | YES |
| Mockgen layout `<pkg>/mocks/` + `internal/cli/mocks/` exception | §15 / §5.1 | Per-package `mocks/` dirs present for registry/envcap/caddy/dockerdrv; `internal/cli/mocks/` holds `mock_deployer.go` + `mock_lifecycle.go` + the rationale `doc.go` (15 lines, exact text from spec) | YES |
| Iter2 Item 1 logging fallback + PersistentPreRunE | `14-fixup-tech-plan.md` Item 1 | `internal/logging/logging.go:22` env-var short-circuit, `:28-32` MkdirAll fallback, `:34-39` OpenFile fallback, both with one-line stderr warning; `internal/cli/root.go:22-24` `PersistentPreRunE → logging.Init()`; `cmd/decloud/main.go` no longer imports logging | YES |
| Iter2 Item 2 env.sh optional + auto-discovery | Item 2 | `internal/envcap/capture.go:47` `Capture("")` returns `(nil, nil)` defensively; `internal/deploy/service.go:139` orchestrator skips Capture on empty; `internal/cli/deploy_service.go:103` `resolveEnvFile()` precedence (explicit-flag-must-exist or stat `<src>/env.sh`); explicit-missing → `ErrEnvScriptMissing` → exit 10 via `internal/cli/exit_codes.go` mapping | YES |
| Iter2 Item 3 NetworkEnsure as Deploy step 0 | Item 3 | `internal/deploy/service.go:131-135` calls `NetworkEnsure` BEFORE envcap (line 137); failure wraps as `ErrRun`; `gomock.InOrder` test pins ordering | YES |
| Iter2 Item 4 `NewHTTPProbeForTest` rename | Item 4 | `grep NewHTTPProbeForTest` returns zero across `internal/` and `cmd/`; `NewHTTPProbe` is the public seam | YES |
| Iter2 Item 5 readiness switch + `ErrNoBridgeIP` | Item 5 | `internal/deploy/readiness.go:49-60` 3-branch switch (`ipErr != nil` / empty IP → `ErrNoBridgeIP` / probe); silent-empty-IP branch is now loud | YES |
| Iter2 Item 6 21 sites `%w: %v` → `%w: %w` | Item 6 | `grep '%w: %v'` across `internal/` + `cmd/` returns zero; `TestDeploy_BuildErrorPreservesInnerSentinel` proves the chain works | YES |
| Status format string | §15.8 / §8.3 | `internal/cli/status.go:25-26` formats `%s state=%s container=%s deploy=%s deployed_at=%s` with RFC3339 timestamp — byte-for-byte the spec | YES |
| `_ai/decisions/m1-test-strategy.md` | Item 9 / §3 deliverables | 53 lines, 5 sections, every claim cross-referenced to code (verified against `dockerdrv/cli_driver.go` cmdFactory, `envcap/capture.go` real-bash, deploy step-0 NetworkEnsure). Reflects actual test architecture, not aspirational | YES |

`go test ./...` passes 9 packages this session. `grep '%w: %v'`, `grep OneShotProbe`, `grep NewHTTPProbeForTest` all empty. Receipt items 7–9 from Rob's report (`017-rob-fixup-impl.md`) verified clean.

---

## Drift between plan and code

**None of substance.** Two minor items I want on the record:

1. **Status `LastDeployedAt`** — `06-tech-plan-v2.md` §15.1 flagged this as "extension to prior tech-plan §4.2 type definition; flag for Linus." Linus accepted in `07-linus-review-v2.md`. The field exists on `ServiceConfig` and Status reads it. Not drift; flagged item resolved.

2. **Status field-naming** — spec said `deploy=<deploy-id>`. Code says `deploy=<LastDeployID>` which IS the deploy ID parsed from the ImageRef tag. Same field. No drift.

Rob's own honesty note (`017-rob-fixup-impl.md` §"Deviations") cites a cosmetic `c` variable name in service.go's auto-discovery branch — matches my snippet verbatim. Acceptable.

---

## Final word

Two iterations, every blocker closed, every deferred item documented. Joel's iter2 plan is the cleanest delta-spec we produced this whole task; Rob's implementation matched it; Kevlin verified low-level; Linus verified architecture; Don verified DONE criteria. The four-way agreement (Don/Joel/Linus + Kevlin) is real, not ceremonial.

M1 ships. Ward writes the M1.x backlog file. Hand it to the user.

End of Joel's final sign-off.
