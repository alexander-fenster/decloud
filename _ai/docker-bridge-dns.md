# Docker bridge DNS only resolves for members of the bridge

Anything that needs to reach a `decloud-<x>` short name MUST be a container attached to the `decloud` user-defined bridge. Host processes — and containers on `--network host` or in a different user-defined bridge — fall through to the host resolver, which will happily return the host's own AAAA (or any wildcard) and dial a port nothing is bound to.

This is the class-of-bug that bit M1.0: Caddy ran on the host as a systemd unit, the generator emitted `reverse_proxy decloud-<svc>:<port>`, and the resolver returned the host's public IPv6. See `_ai/decisions/caddy-runs-in-container.md` for the full architecture record.

## Concrete checks before adding a new component

When designing a component that talks to service containers by name, ask in this order:

1. **Where does its DNS resolution path go?** If it's not on the `decloud` bridge, it cannot resolve `decloud-<x>`. No exceptions — `127.0.0.11` only answers from inside the container's netns; `--network host` shares the host's resolver; sidecar/per-service-Caddy multiplies state without fixing the path; `extra_hosts`/`/etc/hosts` injection races container restarts.

2. **Does an existing component on the host bypass this by going via bridge IP?** The M1 readiness probe does — `Driver.ContainerIP` resolves the bridge IP from `docker inspect` and dials it directly (`internal/dockerdrv/cli_driver.go:170`-ish, on `.NetworkSettings.Networks.decloud.IPAddress`). That's a workaround for one caller; it doesn't propagate to anything else.

3. **If the new caller can't go via `ContainerIP`, it must run as a container on the `decloud` bridge.** That decision drags in volume-mount, bind-mount-source, and cross-namespace-`exec` questions; budget for them up front.

## The lesson that wasn't written down at M1 design time

The M1 tech plan (`_tasks/2026-04-26-m1-implementation/03-tech-plan.md` §9.4) explicitly identified the embedded-DNS gap for the readiness probe and patched it with `Driver.ContainerIP`. That correction stayed inline in the tech plan. It never became a Decision in `_ai/decisions/`, so when Caddy was placed on the host, nobody asked the same question for it.

**When a tech plan corrects an architectural assumption mid-stream, promote the correction to `_ai/decisions/`** — not as the diff against the old plan, but as a standalone "X is true; here are the implications" record. Decisions in `_ai/decisions/` are the thing future reviewers audit new code against; corrections buried in a tech plan are not. Originator: Linus, `_tasks/2026-04-27-caddy-container-connection-refused/004-linus-review.md` §1.2.
