// Package mcp exposes a mcpshell shell as a Model Context Protocol server over
// stdio. The protocol is line-delimited JSON-RPC 2.0; this implementation is
// self-contained (no SDK dependency).
//
// # Embedding
//
// A Go program can run mcpshell in process and give eval its own Go
// functions:
//
//   - [runtime.NewShell] makes a shell; [toolkit.InstallCore] (and the other
//     toolkit Install and Loadable functions) add the built-in commands.
//     sh.Limits().Configure sets the operator limits ("steps=…,timeout=…").
//   - [InstallToolkit] registers a [Toolkit]: a namespace of [Function]s, each
//     a [FuncHandler] with a JSON Schema for its one object argument.
//     Arguments are checked and converted as an upstream tool's are, a
//     returned error is thrown in the script, and the handler's context ends
//     at the eval's time limit.
//   - sh.Eval runs code once; sh.EvalExported runs it as one eval of a
//     session (top-level declarations persist). The value's Display method is
//     its text.
//   - [NewServer] serves the shell as an MCP server over stdio
//     ([Server.RunStdio]). [Server.AddToolkit] installs a toolkit and lists
//     its signatures in the eval tool description.
//
// The embedding program is the operator: what it registers needs no flag.
// The API is pre-1.0; names may change in a minor release.
package mcp
