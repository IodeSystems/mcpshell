# mcpshell plan

How this plan works: see `~/CLAUDE.md` § Planning — current state + active work only; finished → `done.md`, deferred → `icebox.md`.

## Active work — agent-feedback fixes (2026-10-22)

Source: field feedback from an agent driving a 112-tool upstream (`vay`) through mcpshell.
Claims were verified against the binary before scoping; non-reproducible or upstream-owned items are in `icebox.md`.

- ✅ 1. Calling a missing member names it and suggests near keys (substring + edit distance, `-`≈`_`).
- ◻ 2. Non-identifier tool names display as `ns["a-b"]` in help/guides (DisplayName + MCP guide).
- ◻ 3. Regex literal methods: `/x/.test(s)`, `.exec(s)`.
- ◻ 4. `let <builtin> = …` error says it shadows a builtin, with a line number.
- ◻ 5. `allSettled(...)` — per-thunk `{ok, value}` / `{ok: false, error}`.
- ◻ 6. MCP namespace guide mentions `help("<term>")` keyword search and full per-category lists.

**next:** item 2.
**risks:** item 1 suggestions only cover the visitor's postfix path (not `delete` receivers); item 4 may touch scope rules that tests pin.
**blocking decisions:** none open — user accepted items 1, 2A, 3, 4, 5 + guide wording.
**optional extensions:** see `icebox.md`.
**assumptions:** `keys(ns)` is the right pointer for large objects in item 1 errors (works for namespaces).
