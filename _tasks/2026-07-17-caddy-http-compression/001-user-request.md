# User Request

> discuss if it's safe to enable HTTP compression globally in Caddy config or if it's better to have it as a setting per host; implement the change

## Interpretation

Two parts:

1. **Discussion / decision**: Is it safe to turn on HTTP compression (Caddy's `encode` directive —
   gzip/zstd) globally for every site Decloud generates in its Caddy configuration, or should
   compression be an opt-in/opt-out setting configured per host (per service) instead?
   Considerations expected to matter: BREACH/CRIME-style attacks over TLS, already-compressed
   payloads (images, video, archives), streaming/SSE and long-lived responses, proxied backends
   that already compress, CPU cost, and `Content-Length` / range-request interactions.

2. **Implementation**: Once the decision is made, implement it in the Caddy configuration
   generation code, with tests and docs.

## Constraints

- Follow the workflow in CLAUDE.md: plan (Don → Joel → Linus), execute (Kent → Rob → Raymond →
  Kevlin/Linus), re-plan, finalize (Ward → Andy → squash-merge).
- No Docker on this dev machine; integration tests run on a separate Linux host.
