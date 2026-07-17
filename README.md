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

`bin/bench` is a separate side tool — it runs the LLM challenge suite (general
mcpshell tasks, Project Euler, and composition problems) against an
OpenAI-compatible endpoint. Copy `env.local.example` to `env.local`
(gitignored) and set `MCPSHELL_LLM_URL` / `MCPSHELL_LLM_MODEL`, then:

```
./bin/bench                       # full suite
./bin/bench --only euler_         # one category (substring match on teaser name)
./bin/bench --compact             # compact system prompt
./bin/bench --no-tool             # reasoning-only baseline (no mcpshell tool)
./bin/bench --label bonsai        # name used in output paths/reports
```

Results are written as markdown (+ a machine-readable `results.json`) to
`benchmarks/results[-compact|-notool]/<model>/`, or wherever `--out` points.
`bin/bench` auto-builds the same way `bin/mcpshell` does.

### With vs. without the tool

`--no-tool` offers the model no `mcpshell` tool, so it must answer from its own
reasoning — a baseline for measuring what the tool buys. Teasers that need data
only the tool can reach (the SQL ones) are marked tool-only and skipped in this
mode. `bench compare` turns two runs into a side-by-side doc:

```
./bin/bench --only euler_ --label bonsai --out benchmarks/euler/with
./bin/bench --only euler_ --no-tool --label bonsai --out benchmarks/euler/without
./bin/bench compare --label bonsai benchmarks/euler/with benchmarks/euler/without
```

Every result carries a **tool-runtime vs. model-time** split (`tool ms` /
`model ms`), which separates time spent inside the interpreter from time spent
on model round-trips — e.g. it shows the heavy Euler timeouts are interpreter
runtime, not multi-turn thinking. Sample runs live under `benchmarks/euler/` and
`benchmarks/compose/` (see each `comparison.md`).

### Reference solutions

Every deterministic teaser (Project Euler + composition) has a canonical
single-eval mcpshell solution in `bench/references.go`, verified against the
teaser's expected answer with no LLM (`go test ./bench/`). Heavy solutions run
only with `MCPSHELL_BENCH_HEAVY=1`; a coverage test fails if any such teaser
lacks a reference. The composition problems query a seeded in-memory SQLite
fixture (`bench/fixture.go`), attached as the `shop` namespace.

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
