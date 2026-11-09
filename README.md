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

Pin a release with `@v0.11.1` in place of `@latest`. Requires Go 1.26+ — the
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
for filtering in code. `load("name")` installs a loadable toolkit (see Toolkits). `await` is accepted and does nothing: tool calls return
their values directly.

Code an agent writes from JS habit runs, and the eval result says when it may
not do what was meant: one block of typed lines follows the result (or the
error): `warning: line N: …`, `note: …` (a toolkit's notice), `profile: …`,
`limits: steps 850000/1000000 (85%)`. Top-level `let`, `const` and `function`
last for the session (the MCP server, the REPL), as in the Node REPL; there a
builtin or command name (`let all = []`) is an error that suggests another name.
Inside a function or block it replaces the builtin for the rest of that scope,
with a warning. A command given more arguments than its signature lists
is an error with its signature and an example: `floor(2.7, 9)`. JS arguments
that mean something (`parseInt(s, 16)`, `split(",", 2)`, `indexOf(x, from)`)
work; callbacks' `thisArg` is ignored, as in JS.
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
untrusted code can't hang the host. The eval tool takes `profile: true` (per-line
step costs) and `limits: {steps, timeout, callDepth, outputBytes, memory}` (this eval
only, up to the operator maximums in `--max-limits`); the CLI takes
`--profile` and `--limits steps=N,timeout=60s,memory=2GB`. `memory` is bytes of Go heap (default 1 GB, maximum 2 GB unless
`--max-limits memory=4GB`); over it the error lists what holds memory and how to
free it (`unload("db")`, `forget("$r2")`, `forget("rows")`, or `clear: true` for everything), and an eval
that ends at 80% or more says so in a `note:` line. A tripped limit says which
argument raises it.

## Classes and collections

The JS object model works as in Node: `class` with `extends`, `super`, `static` members and blocks, `get`/`set`, `#private` fields and methods, class fields; `this` in methods (arrows keep the outer one); constructor functions with `F.prototype.m = …`, `Object.create`, `Object.getPrototypeOf`/`setPrototypeOf`, `call`/`apply`/`bind`; `instanceof`; `Map`, `Set` (with `union`, `intersection`, `difference`, …), `WeakMap`, `WeakSet`, `Map.groupBy`; `Error`, `TypeError`, `RangeError` and the rest, which classes extend. `throw` keeps any value, so `catch (e)` gets the `Error` object back. Map and Set keys compare as in JS: by value for strings and numbers, by identity for objects.

```js
class NotFound extends Error { constructor(what) { super(what + " not found"); this.name = "NotFound" } }
class Inventory {
  #items = new Map()
  add(sku, n) { this.#items.set(sku, (this.#items.get(sku) ?? 0) + n); return this }
  get total() { let t = 0; for (const [, n] of this.#items) t += n; return t }
  take(sku) { if (!this.#items.has(sku)) throw new NotFound(sku); return this.#items.get(sku) }
}
```

Two differences from JS stay: `==`/`===` compare plain objects and instances by structure (as they always have here), and an error mcpshell raises itself (a type mismatch, a failed command) is its message string in `catch`, where JS would give a `TypeError`. A Map or Set's `keys()`, `values()` and `entries()` return arrays, not iterators. Not supported: `Symbol`, `Proxy`, `Reflect`, generators, `extends Array`/`Map`/`Set` (keep one in a field), `Object.create` with property descriptors, `#x in obj`.

## Dates, number formats, tables

`Date` follows JS for what models write by habit: `new Date()`, `Date.now()`, `new Date("2026-11-07")`, `new Date(y, m, d, …)`, `Date.UTC`, the `get…`/`set…`/`getUTC…`/`setUTC…` methods, `toISOString`, comparison and `d2 - d1` in ms. Local time is **UTC**, because the sandbox has no ambient time zone; `mcpshell mcp --tz Europe/Berlin` (or `MCPSHELL_TZ`) sets another. `Date.now()` reads the host clock. `Dates.*` adds what JS lacks: `add` (months clamp to the month end), `diff`, `startOf`/`endOf`, `isoWeek`, `businessDaysBetween`/`addBusinessDays` (with holidays), `range`, `format`/`parse` with `yyyy-MM-dd` patterns. Each takes a Date, an ISO or SQL date string, or ms, and gives back the same kind.

`n.toLocaleString("en-US", opts)` and `new Intl.NumberFormat("en-US", opts).format(n)` format from the exact decimal: `(0.1 + 0.2)` with two digits is `"0.30"`. Supported: grouping, fraction and significant digits, `currency`, `percent`, `compact`, `signDisplay`. Another locale or an unsupported option is an error that lists what works. `toMarkdown(rows, {columns, header})` turns an array of objects (or of arrays) into a GitHub table. `help("dates")` and `help("format")` have the details.

## Toolkits

Toolkits register commands into the shell. `core` and `math` are always on.
In `mcpshell mcp`, `graph`, `db`, `solver`, `cp`, `route`, `geo` and `num` are loadable by the agent; `web`, `file`, and
`browser` need an operator flag, and the agent then installs them with
`load("name")`. The LLM only sees the capabilities you grant. One-shot and
REPL modes install core, math, web, graph, and file (rooted at the current
directory) at start.

| Toolkit | Enable with | Commands |
|---------|-------------|----------|
| **core** | always on | strings, arrays, objects, JSON, regex, pipes, control flow; `Date` and `Dates.*`, `toLocaleString` and `Intl.NumberFormat`, `toMarkdown` |
| **math** | always on | arithmetic, rounding, trig, `min`/`max`/`sum`, etc. |
| **graph** | `load("graph")` | in-memory node/edge graph: `addNode`, `link`, `nodes`, `edges`, `setProps`, …; algorithms on edge rows (two-way unless `directed: true`): `Graph.path` (Dijkstra, Bellman-Ford, DAG longest), `reach`, `components` (connected, strong), `cycle`, `topo`, `cpm` (critical path, slack), `flow` (max flow/min cut, min-cost flow, supplies), `match` (bipartite, assignment), `mst`; exact on exact inputs |
| **web** | `--web`, then `load("web")` | `Web.fetch`, `Web.fetchText`, `Web.search`, `Web.clearCache`; `Html.select`, `Html.links`, `Html.text`, `Html.table` |
| **file** | `--files-dir DIR`, then `load("files")` | sandboxed reads/writes rooted at `DIR` (add `--files-read-only`) |
| **sql** | `--sql 'ns=DSN'` | Postgres + SQLite: `ns.query`, `ns.tables`, `ns.columns`, `ns.schema`, `ns.execute` |
| **db** | `load("db")`; `--db-max-mb N` | in-memory SQLite scratch database: `Db.load`, `loadCsv`, `query`, `exec`, `tables`, `schema`, `drop`, `reset`, `parseCsv`, `toCsv`; exact `sum`/`avg`, `median`, `quantile`, `stdev`, `corr`, `mode`, window functions; R-tree spatial index (`CREATE VIRTUAL TABLE ... USING rtree`, the only virtual table allowed) |
| **solver** | `load("solver")` | exact LP/MILP (no floats): `Solver.lp({maximize, subjectTo, ...})` returns status, objective, values, slack; takes row arrays from queries; `sensitivity: true` adds shadow prices, reduced costs and ranges; `explain: true` names the conflicting constraints of an infeasible model |
| **cp** | `load("cp")` | constraint programming over integers: `Cp.solve({vars, intervals, constraints, minimize})` with linear rows, allDifferent, element, table, count, max/min, noOverlap, cumulative, before, circuit; schedules out as rows; `status` is `optimal` only when proven |
| **route** | `load("route")` | routing: `Route.matrix` (haversine, euclidean, manhattan, or shortest paths over edges), `Route.path`/`paths` (Dijkstra, A*, k shortest), `Route.tsp` (exact up to 16 stops, else heuristic with a lower bound and gap), `Route.vrp` (capacity, time windows, maxDist, maxTime); `status` is `optimal` only when proven |
| **geo** | `load("geo")` | spatial on rows `{lat, lon}` / `{x, y}` and GeoJSON (`[lon, lat]`): `Geo.distance` (haversine), `bearing`, `destination`, `bbox` (radius boxes for a `Db` R-tree), `nearest` (k), `length`, `area` (WGS84 ellipsoid), `centroid`, `contains`/`intersects`/`within` (no polygon overlay), `hull`, `circle`, `toGeoJSON`/`fromGeoJSON`, `kmeans` (seeded, reports `converged`; a local optimum) and `dbscan`; swapped or out-of-range coordinates are errors |
| **num** | `load("num")` | float64 numerics: `Num.regress(rows, {y, x})` (OLS with standard errors, t/p, CIs, R², F), `Num.fit` (Levenberg-Marquardt with your model `(x, p) => y`, or built-in Gaussian/Lorentzian/pseudo-Voigt peaks with a baseline; params, standard errors, covariance), `Num.minimize` (Nelder-Mead, bounds), `Num.root` (Brent), `Num.integrate` (adaptive Gauss-Kronrod, infinite ends; trapezoid on samples), `Num.interp` (linear, natural cubic), `Num.solve`/`inv`/`det`/`eig` (symmetric)/`lstsq`/`matmul`, `Num.pdf`/`cdf`/`sf`/`ppf` for 11 distributions, `Num.ttest`, `Num.ci`, `Num.rng(seed)`; `status` says when a method did not converge |
| **browser** | `--browser`, then `load("browser")` | headless-Chrome automation (chromedp): `Browser.open`, `click`, `type`, `text`, `html`, `select`, `wait`, `screenshot`, `eval` |

Operator flags are the gate for toolkits that reach outside the process (files, network, databases, a browser): a script cannot enable them. A flag permits a toolkit; it is not installed until `load("name")`. Without the flag the toolkit is not advertised, and `load("web")` fails naming `--web`. Loadable toolkits are listed on one line in the prompt and in the `eval` tool description (`Available (load first): load("name") — summary`). `load("name")` installs one for the rest of the session and returns its commands, `unload("name")` removes it. Calling a command before its load gives a "call load first" error. `help("name")` shows the guide before and after the load. `mcp --no-load a,b` hides and denies the listed loadables. `load("path/to/file.mcpshell")` evaluates a file as a module (only its `export`ed names become globals); it installs the file toolkit first if `--files-dir` permitted it, and fails naming `--files-dir` if not. `mcp --prompt` prints the system prompt for a flag set.

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
both accept the file's absolute path. A read-only file toolkit on that
directory is installed at start, so paging needs no `load("files")`. Scope the directory per session (e.g.
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
toolkit/   Built-in command toolkits — core, math, web, file, graph, sql, db, solver, cp, route, geo, num, browser
solver/    Exact LP/MILP solver (math/big.Rat), no runtime dependency
routing/   Distances, shortest paths, TSP, VRP (float64), no runtime dependency
graphalg/  Graph algorithms: components, SCC, topo, CPM, flows, matching, MST; no runtime dependency
geo/       Spatial geometry and clustering (float64), no runtime dependency
numeric/   Float64 linear algebra, fitting, optimization, distributions, RNG; no runtime dependency
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
