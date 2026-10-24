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

## Agent-feedback fixes, round 2 (2026-10-23)
Field feedback from the same 112-tool upstream session. Verified each claim first.
- `await x` is the identity (grammar `AWAIT unaryExpr`, `da016f1`); `async` functions still rejected.
- Caught `e` stays the message string; `e.message` returns it; an unknown member on a string now fails instead of returning null (cause of the agent's "ERR null").
- Upstream errors name the call, the arguments sent, and the text, or `(no message)`. Missing required args are listed all at once before the call; `ns.raw` skips that check.
- `ns.help(…)` error points at `help("ns")`.
- `help(word)` splits the query into words, ranks name hits above description hits, one short line per result, max 25.
- `commands(search?)` returns `[{name, call, category, signature, description}]`.
- `--version`; MCP serverInfo/clientInfo carry the build version (was 0.1.0).
- Not changed: a trailing `help()` being the result (last expression is the output — by design); "no truncation signal inside the language" (values are not cut inside; the eval response, web, file and sql all say when they cut).

## MCP argument types (2026-10-24)
- Declared argument types are checked before the call (`mcp/argtypes.go`). Lossless conversion: number/bool → string, numeric string → number/integer, "true"/"false" → boolean, JSON string → array/object, single value → `[value]`. Anything else fails. Properties without a plain `type` (anyOf/oneOf/$ref) and `null` values are not checked.
- `"unchecked": [globs]` per `--mcp` server entry turns off the name, required and type checks for matching tools (user ask: tools whose schemas are wrong).
- Assumption: array items are not converted (`"a"` → `["a"]` even if items are numbers).

## Release v0.5.0 (2026-10-24)
- Tagged and published: https://github.com/IodeSystems/mcpshell/releases/tag/v0.5.0. Covers everything since v0.4.1. Minor bump: unknown string members now fail; upstream args checked before the call.
- Checks: tests, `make vet`, `govulncheck` clean (Go 1.27.3); `go install …@v0.5.0` from the proxy prints `mcpshell v0.5.0`; proxy `@latest` = v0.5.0.
- Follow-up outside this repo: rebuild dun's Docker image (it copies its own mcpshell binary).
