# mcpshell

[![Release](https://img.shields.io/github/v/release/IodeSystems/mcpshell?label=release)](https://github.com/IodeSystems/mcpshell/releases/latest)

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

## Toolkits

Toolkits register namespaced commands into the shell. `core`, `math`, and
`graph` are always on; the rest are opt-in via `mcpshell mcp` flags so the
LLM only sees the capabilities you grant.

| Toolkit | Enable with | Commands |
|---------|-------------|----------|
| **core** | always on | strings, arrays, objects, JSON, regex, pipes, control flow |
| **math** | always on | arithmetic, rounding, trig, `min`/`max`/`sum`, etc. |
| **graph** | always on | in-memory node/edge graph: `addNode`, `link`, `nodes`, `setProps`, … |
| **web** | `--web` | `Web.fetch`, `Web.fetchText`, `Web.search`, `Web.clearCache`; `Html.select`, `Html.links`, `Html.text`, `Html.table` |
| **file** | `--files-dir DIR` | sandboxed reads/writes rooted at `DIR` (add `--files-read-only`) |
| **sql** | `--sql 'ns=DSN'` | Postgres + SQLite: `ns.query`, `ns.tables`, `ns.columns`, `ns.schema`, `ns.execute` |
| **browser** | `--browser` | headless-Chrome automation (chromedp): `Browser.open`, `click`, `type`, `text`, `html`, `select`, `wait`, `screenshot`, `eval` |

### SQL

Attach one or more databases; each becomes a namespace. The DSN is a SQLite
path or a `postgres://` URL. Queries are **read-only by default** (only
`select`/`with`/`show`/… are allowed) and capped at 500 rows; pass
`--sql-writable` to permit writes and DDL via `ns.execute`.

```
mcpshell mcp --sql 'app=postgres://user:pass@localhost/app' \
             --sql 'cache=./local.sqlite'
# then, in eval:  app.query("select id, email from users where id = $1", [42])
#                 app.schema()   ·   cache.tables("order")
```

### Browser

`--browser` drives a real headless Chrome through chromedp — navigate, click,
fill inputs, extract text/HTML, run in-page JS, and screenshot. Requires a
Chrome/Chromium install on the host.

```
mcpshell mcp --browser
# Browser.open("https://example.com")  ·  Browser.text("h1")
# Browser.select("table tr")  ·  Browser.screenshot("shot.png")
```

`bin/mcpshell` is a launcher: it builds a per-arch binary (`bin/mcpshell-<os>-<arch>`)
on demand — when missing, when any source is newer, or when `BUILD=true` is set —
then execs it. The same launcher works across machines and architectures.

## Benchmark

mcpshell isn't a faster calculator (hand an LLM `bc` for that). It earns one
sandboxed `eval` tool on three things a pile of discrete tools can't offer at
once — see **[`benchmarks/`](benchmarks/README.md)** for the full showcase:

1. **LLM-hard reliability** — the "how many r's in strawberry" class: exact by
   construction and ~2.5× faster than reasoning it out, with the gap widening as
   inputs grow.
2. **Context savings** — one `eval` vs. one tool per capability is **−96%**
   context per request (341 vs 8366 prompt tokens for 112 tools), flat as you add
   capabilities, KV-cache friendly. Reproduce with `./bin/bench context`.
3. **Safe untrusted execution** — no imports, `eval`, `process`, prototype chain,
   or ambient I/O, plus step/time limits, so model-authored code can't reach or
   hang the host (`toolkit/sandbox_test.go`).

`bin/bench` runs the challenge suite against an OpenAI-compatible endpoint and
can run the same problems *without* the tool to measure the difference (scored on
correctness, turns, processed/cached tokens, time):

```
./bin/bench                                  # full suite
./bin/bench --only llm_hard_                  # one category
./bin/bench --no-tool                        # reasoning-only baseline (no tool)
./bin/bench compare with_dir without_dir     # side-by-side comparison doc
./bin/bench context                          # tool-context cost measurement
```

Results are written as markdown + a machine-readable `results.json`. Every
deterministic teaser also has a verified single-eval reference solution
(`bench/references.go`, checked by `go test ./bench/` with no LLM). `bin/bench`
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
