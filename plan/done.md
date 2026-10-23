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

## Tool → file fidelity (2026-10-22)
- MCP tool output and eval `vars` decode via the parseJson parser (exact numbers, key order); args go upstream as `json.Number` (`b80d81c`).
- `ns.raw("tool", {args})` returns unparsed text for byte-exact `write(path, …)`.
- Known: `JSON.stringify` of parsed values keeps every number exact but re-formats them (`1e300` → 301 digits, trailing decimal zeros dropped). Use `ns.raw` when the file must match the tool byte for byte.

## MCP argument-name check (2026-10-23)
- Calls are checked against the tool's input schema before going upstream (`mcp/toolkit.go` `checkArgs`).
- `additionalProperties: false` → any undeclared name fails. Open schema → only a likely mistake fails (same name in another case style, or one edit away when both ≥ 5 chars); other undeclared names pass as hidden options (e.g. a `confirm` nonce for two-phase tools).
- `ns.raw` skips the check — the escape for a hidden option that looks like a typo.

## Overflow output to file (2026-10-23)
- `MCPSHELL_TMP_OVERFLOW_DIR`: eval output over `--max-output` is also saved to `eval-*.txt` there; the truncated response names it (`aaff09b`). Arrays → JSON Lines, objects → one key per line.
- File toolkit read-only commands accept absolute paths inside the overflow dir; without `--files-dir` a read-only toolkit rooted there is installed.
- Session scoping is the launcher's job: dun `d25099f` passes `<sessions>/<root>/overflow/<session-id>` and removes it in `ForgetSession`.
