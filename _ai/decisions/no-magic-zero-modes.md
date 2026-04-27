# Decision: no magic-zero modes; new shapes get new commands

**Context:** M1's `decloud deploy service` requires `--port` because every service is HTTP and the readiness probe targets that port. Workers/cron/jobs are an M5 milestone with their own lifecycle (no HTTP listener, readiness signal = "container exited 0"). The temptation: treat `--port=0` as "this is a worker, skip readiness." We rejected this.

**Decision:** `--port` is unconditionally required by `deploy service`. Reject `f.Port == 0` at validation with `errUsage` → exit 2 (`internal/cli/deploy_service.go:73-75`). When M5 lands, workers get `decloud deploy job` (or a `--kind=worker` flag) with their own command, validation, and deployer logic — NOT a port=0 mode of `deploy service`.

## Why

- **Magic-zero modes fold unrelated workloads into one command.** The deployer would have to branch on `port == 0` to decide whether to skip readiness, register Caddy routes, write health checks. That's a 200-line if-else tree three milestones from now.
- **"Skip readiness when port=0" is the dangerous variant.** A container that exits 0 immediately would record a "successful deploy" with no probe. Lying to operators about deploy success is the worst possible outcome — pages within a week.
- **Don't build hooks for hypothetical future shapes.** YAGNI applies even when the future is on the roadmap. M5's command shape is decided at M5 time, with M5 context.

## Where the contract is enforced

CLI boundary, not deeper layers. `internal/deploy/service.go:213` and `internal/deploy/readiness.go:55` stay simple — they call `Wait(..., req.Port)` unconditionally. The contract that "every service has an HTTP port" lives in `runDeployService`. Pushing the policy down would split it across layers and force the deployer to know about workload kinds it shouldn't care about.

## Generalization

When a new workload shape appears, ask: *new command, or new mode of an existing command?* The answer is "new command" if the lifecycle, validation, or contract differs in any user-visible way. `deploy service` (always-on HTTP) vs `deploy job` (run-to-completion batch) are two different shapes. They share an underlying container driver and that's where reuse belongs — not at the user-facing CLI layer.

Locked in by `TestDeployService_NoPortReturnsExitUsageError` and `TestDeployService_PortZeroExplicitReturnsExitUsageError` (the latter prevents a future "treat 0 as unset" regression).

## Originator

`_tasks/2026-04-26-fix-deploy-service-review-findings/02-plan.md` §"Finding 3", `04-linus-review.md` §"Finding 3 — Reject port=0: CORRECT POLICY, NOT A CORNER" (cross-referenced README.md:215 and `_ai/decisions/m1-scope.md:32`).
