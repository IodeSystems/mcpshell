# mcpshell

A sandboxed JS-syntax scripting language that gives LLMs safe computation
through a single `eval` tool.

**Status: complete.** Parser, runtime, interpreter, all toolkits
(core, math, web, file, graph, SQL, browser), the MCP server, and the LLM
benchmark harness are implemented and tested — 750+ tests.

## Install

```
go install github.com/iodesystems/mcpshell/cmd/mcpshell@latest
```

This drops a `mcpshell` binary in your `$GOBIN`. Or run it without installing:

```
go run github.com/iodesystems/mcpshell/cmd/mcpshell@latest '[1,2,3] |> map(x => x * 10)'
```

Pin a release with `@v0.1.0` in place of `@latest`. Requires Go 1.26+ — the
`go` directive auto-fetches the toolchain for anyone on Go 1.21 or newer. No
Java is needed; the generated parser is committed.

## Run

```
./bin/mcpshell '[1,2,3] |> map(x => x * 10)'   # evaluate code
echo 'range(5) |> map(n => n * n)' | ./bin/mcpshell
./bin/mcpshell                                 # interactive REPL
./bin/mcpshell --prompt                        # print the LLM system prompt
./bin/mcpshell mcp --web --files-dir ./data    # run as an MCP server over stdio
```

As an MCP server, mcpshell exposes `eval`, `help`, and `prompt` tools. It can
also compose upstream MCP servers as namespaced commands
(`--connect`/`--mcp`); an upstream that is itself mcpshell is skipped to avoid
a recursive `eval` loop.

`bin/mcpshell` is a launcher: it builds a per-arch binary (`bin/mcpshell-<os>-<arch>`)
on demand — when missing, when any source is newer, or when `BUILD=true` is set —
then execs it. The same launcher works across machines and architectures.

## Benchmark

`bin/bench` is a separate side tool — it runs the 33-challenge LLM suite against
an OpenAI-compatible endpoint. Copy `env.local.example` to `env.local`
(gitignored) and set `MCPSHELL_LLM_URL` / `MCPSHELL_LLM_MODEL`, then:

```
./bin/bench                    # full suite
./bin/bench --only factorial   # one teaser (smoke test)
./bin/bench --compact          # compact system prompt
```

Results are written as markdown to `benchmarks/results/<model>/`. `bin/bench`
auto-builds the same way `bin/mcpshell` does.

## Layout

```
grammar/   ANTLR4 grammar (.g4)
parser/    Generated Go parser + hand-written lexer base (committed)
runtime/   Value types, environment, interpreter, the Shell facade
toolkit/   Built-in command toolkits — core, math, web, file, graph, sql, browser
mcp/       MCP server + upstream-MCP client/composition
bench/     LLM benchmark harness (OpenAI-compatible agent loop)
cmd/       Binaries: cmd/mcpshell (the CLI), cmd/bench (the benchmark)
bin/       Auto-building launchers — bin/mcpshell, bin/bench
```

## Build

```
make build    # go build ./...
make test     # go test ./...
make generate # regenerate parser/ after editing grammar/ (needs Java + ANTLR 4.13.2)
```

Requires Go 1.26+. Parser regeneration requires a JDK and the ANTLR 4.13.2 tool
jar with its dependencies (resolved from the local Maven repo — see `Makefile`).
