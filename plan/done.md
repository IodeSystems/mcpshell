# Done

## Agent-feedback fixes (2026-10-22)
Field feedback from an agent driving a 112-tool upstream (`vay`). Verified claims, fixed:
- Calling a missing member names it and suggests near keys (`f800575`).
- Non-identifier tool names shown as `ns["a-b"](…)` in help (`CommandDef.CallName`).
- Regex literal `.test/.exec/.source/.flags` (stateless; `g` ignored).
- `let <builtin>` says it is a builtin (incl. `Math` via `Shell.SetBuiltin`); all let redeclare errors carry a line.
- Member suggestions compare with case and `-`/`_` removed (camel ≡ snake ≡ kebab ≡ UPPER).
- `allSettled(...)` / `Promise.allSettled`.
- MCP namespace guide mentions `help("<word>")` search and `help("ns.<category>")`.
Deferred items → `icebox.md`.
