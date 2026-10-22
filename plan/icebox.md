# Icebox — deferred, opt-in

- Pre-validate MCP call args against the tool's input schema; on an unknown key, suggest the snake/camel variant before the call goes upstream (`mcp/toolkit.go` `argsToJSONMap`). Behaviour change at the MCP boundary — needs user opt-in.
- Keep the full value of an over-limit eval result (e.g. `_last`) so a follow-up eval can slice it instead of re-running (`mcp/server.go` truncation). Behaviour change — needs user opt-in.
- Multi-line eval reporting wrong line:col — not reproduced (multi-line test reported the correct line). Needs the failing input.
