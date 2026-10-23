# mcpshell plan

How this plan works: see `~/CLAUDE.md` § Planning — current state + active work only; finished → `done.md`, deferred → `icebox.md`.

## Active work

- ◐ dun sets `MCPSHELL_TMP_OVERFLOW_DIR=<state>/overflow/<session-id>` when it spawns mcpshell (change lives in the dun repo).
  **next:** find dun's spawn point + session ID; add the env; test argv/env.
  **risks:** shell restarts mid-session must keep the same dir; Docker mode path translation; overflow dirs need pruning with sessions.
  **blocking decisions:** none yet.
