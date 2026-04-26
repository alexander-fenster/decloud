# Container naming: M1 vs M4 divergence

Two different naming conventions across milestones. The rename is intentional and is an explicit M4 deliverable, not accidental drift.

- **M1 (recreate strategy):** `decloud-<name>` — no deploy-id suffix. Old container is stopped/removed before new one starts, so name collisions can't happen.
- **M4 (blue/green via Caddy admin API):** `decloud-<name>-<deploy-id>` — old and new containers run simultaneously during the upstream swap, so unique names per deploy are mandatory.

## M4 owns the migration

When M4 lands, its tech plan must include **one-time recreation of all M1-era containers under the new naming convention** as a tracked deliverable. Not a TODO comment in code, not "next time someone redeploys" — an explicit migration step at M4 ship time. Don elevated this from "flag in code" to a tracked deliverable in plan-v2 §9 specifically because comments-in-code rot.

## Anything that depends on container name

If you write code in M1–M3 that hard-codes `decloud-<name>` (Caddy `reverse_proxy` directive, stop/remove logic, status lookup), that code MUST be touched in M4. Use a single helper (e.g. `ids.ContainerName(serviceName, deployID)`) so M4 only changes one function body, not every call site.
