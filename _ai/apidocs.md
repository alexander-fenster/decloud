# Doc-writing notes (Raymond)

- `_docs/` in this project is plain Markdown (`install.md`, `usage.md`) — NOT a Next.js/JSX app. No build step; do not run `next build`.
- Operator-facing port/protocol facts live in `_docs/install.md` (§3 firewall, `caddy up` steps) and `_docs/usage.md` (§ caddy up bullet ~196, network model §6 ~320). README.md also lists host ports (~61 feature line, ~124 prerequisites).
- Architecture/"why" rationale lives in `_ai/decisions/*.md`, deliberately NOT in `_docs/` (see caddy-runs-in-container.md footer). When a Decision's premise is field-disproven, amend with a dated subsection — never delete the original reasoning.
- HTTP/3 is disabled (`servers { protocols h1 h2 }` in the generated Caddyfile). UDP/443 is still published by `manager.go` runOpts() but inert. There is NO user-facing protocol config flag (hardcoded in `internal/caddy/generator.go`); `caddy.protocols` is only an M3 forward-looking idea.
- No Docker on the dev box → never write "validated" for Caddyfile changes; say "byte-asserted; pending operator `caddy validate`."
