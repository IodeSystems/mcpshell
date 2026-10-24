# mcpshell plan

How this plan works: see `~/CLAUDE.md` § Planning — current state + active work only; finished → `done.md`, deferred → `icebox.md`.

## Active work

None. Last finished: agent-feedback round 2 (see `done.md`). Opt-in follow-ups in `icebox.md`.

## Conventions

- Vet with `make vet`, not `go vet ./...`. It skips the generated `parser/`, where ANTLR's dead `goto errorExit` after each rule's `return` is 69 known, non-actionable "unreachable code" reports (same at v0.4.1).
