# Icebox — deferred, opt-in

- Keep the full value of an over-limit eval result (e.g. `_last`) so a follow-up eval can slice it instead of re-running (`mcp/server.go` truncation). Partly covered by `write(path, ns.raw(…))` then `read`/`grep`. Behaviour change — needs user opt-in.
- Multi-line eval reporting wrong line:col — not reproduced (multi-line test reported the correct line). Needs the failing input.
