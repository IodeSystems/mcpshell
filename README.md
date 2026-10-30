# mcpshell

[![Release](https://img.shields.io/github/v/release/IodeSystems/mcpshell?label=release)](https://github.com/IodeSystems/mcpshell/releases/latest)

A sandboxed JS-syntax scripting language that gives an LLM **one `eval` tool
instead of twenty**. It earns that one tool on three things a pile of discrete
tools can't offer at once:

- **Reliability on LLM-hard tasks** — exact letter counting, arithmetic, string
  and data wrangling: right by construction and faster than reasoning it out.
- **~96% less tool-schema context** — one `eval` + a compact reference (or a
  deferred `help()`) replaces one tool schema per capability, and stays flat as
  you add capabilities (KV-cache friendly).
- **Safe untrusted execution** — no imports, `eval`, `process`, prototype chain,
  or ambient I/O, plus step/time limits, so model-authored code can't reach or
  hang the host.

Measured in [`benchmarks/`](benchmarks/README.md): a 96% cut in tool-schema
context; on DeepSeek with a 102-tool server, 38% fewer processed tokens and 43%
less time than the same tools given directly (but 51 turns to 29); every host
escape blocked. On small self-contained problems a fast model is quicker
without the tool.

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

Pin a release with `@v0.7.1` in place of `@latest`. Requires Go 1.26+ — the
`go` directive auto-fetches the toolchain for anyone on Go 1.21 or newer. No
Java is needed; the generated parser is committed.

From a checkout, `make install` builds and installs to `~/.local/bin`
(override with `make install PREFIX=/usr/local`).

## Run

```
./bin/mcpshell '[1,2,3] |> map(x => x * 10)'   # evaluate code
echo 'range(5) |> map(n => n * n)' | ./bin/mcpshell
./bin/mcpshell                                 # interactive REPL
./bin/mcpshell --prompt                        # print the LLM system prompt
./bin/mcpshell mcp --web --files-dir ./data    # run as an MCP server over stdio
```

As an MCP server, mcpshell exposes exactly ONE tool, `eval` — the claim at the
top of this file, made literal. The reference is reachable from inside the
language (`help()`, `prompt()`), which is where it belongs: a separate `help`
tool would be a second schema on every turn to describe a thing `eval` can
already answer. It can
also compose upstream MCP servers as namespaced commands
(`--connect`/`--mcp`); an upstream that is itself mcpshell is skipped to avoid
a recursive `eval` loop.

Upstreams are stdio commands or remote streamable-HTTP servers
(`--connect 'ns=https://…'`, or `url` in the `--mcp` JSON). For hosted servers
with short-lived OAuth tokens, `auth_cmd` is a shell command that prints a
bearer token; it is cached, re-run on a 401, and re-run 60s before a JWT's
`exp`. The re-run on a 401 has `MCPSHELL_AUTH_RETRY=1` and
`MCPSHELL_AUTH_REJECTED=<the rejected token>` in its environment, so a script
that caches tokens knows to mint a new one:

```json
{"mcpServers": {"el": {"url": "https://api.us.elevenlabs.io/v1/mcp",
                       "auth_cmd": "my-token-script elevenlabs",
                       "headers": {"X-Extra": "1"}}}}
```

`help("<words>")` searches command names and descriptions; `commands(…)` takes
the same argument and returns `[{name, call, category, signature, description}]`
for filtering in code. `await` is accepted and does nothing: tool calls return
their values directly.

Code an agent writes from JS habit runs, and the eval result says when it may
not do what was meant: lines starting `warning: line N:` follow the result (or
the error). `let all = []` replaces the builtin `all` for the rest of that
scope, with a warning. A command given more arguments than its signature lists
ignores the extras, as JS does, with a warning: `floor(2.7, 9)`.
`round(n, digits?)` rounds to decimal places, and a half rounds up as in JS
`Math.round` (`2.5` → `3`).

Once the client sends `notifications/initialized`, mcpshell reports each
connected upstream in a `notifications/message` with logger
`mcpshell/hosted` and data `{"servers": [{"ns", "tools", "schemaBytes",
"helpBytes"}]}`. `schemaBytes` is the JSON size of the upstream's tools/list
entries (what a direct client would send the model on every request), and
`helpBytes` is the size of `help("<ns>")`. The stderr `connected` line
ends with the same two numbers.

A namespace with more than 20 commands is one line in `help()` and the prompt
(`el: 160 commands — help("el")`). An upstream that large is grouped by tool
name prefix — `help("el")` lists `agents (117)`, `creative (42)`, … and
`help("el.agents")` lists that group. To set the groups yourself, add
`"categories": {"voice": ["tts_*", "voices_*"]}` to a server entry; tools that
match no glob go to `other`.

Tool calls are checked against each tool's input schema before they go
upstream. Missing required arguments are listed in one error. A
misspelled argument name gets a did-you-mean. An unknown name that
resembles no declared one passes as a hidden option, unless the schema
sets `additionalProperties: false`. Arguments are converted to the
declared type when nothing is lost (`"10"` → `10`, `42` → `"42"`,
`"true"` → `true`, `"a"` → `["a"]`, a JSON string → array or object);
anything else fails. For tools whose schema is wrong, add
`"unchecked": ["glob", …]` to the server entry (`["*"]` for every tool)
to send their arguments as given. `ns.raw("tool", {args})` also skips
every check for a single call.

## Numbers & cost

Numbers are **exact and arbitrary-precision** — no silent float64 rounding.
Integer arithmetic auto-promotes past 2^53, decimals and rationals stay exact,
and only transcendental ops (`sqrt`/`sin`/`log`/non-integer powers) are
floating-point (the precision boundary). A tiered `int64 → big.Int → big.Rat`
representation keeps the common integer path allocation-free (~1.3× the float
baseline on hot loops).

```
2 ** 1000            // 302 exact digits
0.1 + 0.2 == 0.3     // true
1/3 + 1/3 + 1/3      // 1
```

Every eval is bounded (step, call-depth, wall-clock, and output limits) so
untrusted code can't hang the host. `profile(() => …)` returns
`{result, steps, lines}` to see which lines cost the most, and a tripped step
limit names the hot lines instead of just a number.

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

### Large output

An eval result over `--max-output` (default 16000 bytes) is truncated, and the
full value is kept as `$r1`, `$r2`, … (the last 5) for the next evals. The
note after the shown part gives the value's inferred TypeScript type, so the
next eval can query it without running the tools again:

```
... OUTPUT TRUNCATED at 16000 of 114330 bytes. The full value is $r1, kept for the next evals (the last 5 are kept):
  $r1: Array<{amount: number, customer_id: number, due: string, id: number, status: "open" | "overdue" | "paid"}>, length 1500
  Query it instead of re-running: $r1 |> limit(5)   $r1 |> filter(x => …) |> len()
```

Set `MCPSHELL_TMP_OVERFLOW_DIR` and the full result is also saved to a new file
there; the truncated response names it. Arrays are saved as JSON Lines, so
`read(path, start, lines)` and `grep(path, {match})` page through them —
both accept the file's absolute path. Scope the directory per session (e.g.
`…/overflow/<session-id>`); mcpshell only makes file names unique.

To save a tool's output without parsing it, use `ns.raw("tool", {args})`:
`write("alerts.json", ns.raw("list_alerts", {}))`.

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

The three claims above are measured in **[`benchmarks/`](benchmarks/README.md)**
(context cost, sandbox properties, and a with/without-tool showcase per suite).
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
make vet      # go vet, without the generated parser/ (ANTLR's dead `goto` trips it)
make generate # regenerate parser/ after editing grammar/ (needs Java + ANTLR 4.13.2)
```

Requires Go 1.26+. Parser regeneration requires Java 11+; `make generate`
downloads the ANTLR 4.13.2 jar to `.cache/`, checks its SHA-256, and, if Java
is missing, prints how to install Eclipse Temurin 21 (formerly AdoptOpenJDK)
for your OS.
