# env.sh capture: portable bash mechanism

The deploy-time env capture must run on macOS (BSD env, bash 3.2 at `/bin/bash`) AND Linux (GNU env, bash 5+) without modification — the maintainer's dev box is macOS.

## The trap to avoid

GNU `env -0` does NOT exist on BSD env (macOS). BSD env silently treats `-0` as a `name=value` pair and does nothing — classic "looks fine until you parse it" failure. Do not use `env -0` anywhere in the capture pipeline.

## The mechanism (verified working on macOS bash 3.2)

```bash
/usr/bin/env -i \
    PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    HOME=/tmp \
    /bin/bash --noprofile --norc -c '
if [ -n "$1" ]; then
    set -a
    source "$1"
fi
while IFS= read -r __name; do
    printf "%s=%s\0" "$__name" "${!__name}"
done < <(compgen -e)
' _ "$SCRIPT_PATH"
```

Each piece is load-bearing:
- `env -i` — wipes inherited env so operator's SSH session vars (`SSH_AUTH_SOCK`, `LANG`, `LS_COLORS`) don't leak in. Without this, different operators get different captures from the same `env.sh`.
- `PATH=...` + `HOME=/tmp` re-seeded — bash needs PATH for spawned commands and HOME for tilde expansion; we pass `/tmp` to keep operator dotfiles out via tilde paths even when `--norc` is honored.
- `--noprofile --norc` — skip `~/.bash_profile`, `~/.bashrc`, `/etc/bash.bashrc`. Without this, `direnv hook bash` or `source ~/.aws/credentials` from the operator's dotfiles ends up in the container env. Real, dangerous.
- `set -a` — operators write `FOO=bar` without `export` constantly. Without `set -a` those are silently lost.
- `compgen -e` — bash builtin (bash 2+, present on macOS bash 3.2 and Linux bash 5+); enumerates exported variables. No external command, no GNU/BSD divergence.
- `printf '%s=%s\0'` — bash's printf builtin honors `\0`; NUL is the only byte that cannot legally appear in env values, so it's the only safe separator (newlines are valid payload in PEM-style values).
- `${!name}` — bash 2+ indirect expansion. Single source of truth for the value.
- `_ "$SCRIPT_PATH"` — sets `$0=_`, `$1=$SCRIPT_PATH`. Same script gates baseline (no source) and full (source) on `if [ -n "$1" ]` — single source of truth for what gets emitted.

## Baseline-diff strategy

Run the same hermetic command twice — once with no script (baseline), once with the script. Subtract: drop `(k, v)` from full if baseline has `k` with the same `v`. Catches script-defines, script-modifies, script-unsets correctly. Drops bash internals (`PWD`, `SHLVL`, `BASH_VERSION`).

Residual limitation: a script that intentionally sets a variable to its baseline value gets dropped. Almost always a no-op (`export PATH="$PATH"`); document, don't fix.

## Known sharp edges (document, don't engineer around)

- **`set +a` partway through a script** — variables not auto-exported are dropped. Matches operator's mental model ("if I didn't export it, it's not in the container env"). Document.
- **Arrays in env.sh** — `${!FOO}` on an array yields only `${FOO[0]}` on bash 3.2. Arrays don't survive into container env anyway (env vars are scalars). Document, don't engineer around.
- **`readonly` variables** — `set -e` + readonly conflict surfaces as bash exit non-zero (we report it via stderr). Without `set -e`, silent ignore — operator notices on first deploy.
- **`/bin/bash` replaced with `rbash`** — `compgen` is disabled. Vanishingly rare on a controlled production host.

## Tests

Unit tests run on macOS AND Linux in CI from day one — no build-tag skip. The whole point of this design is portability; CI catches anyone re-introducing a GNU dep. See `internal/envcap/capture_test.go` (per tech plan §3.5, §12.1).
