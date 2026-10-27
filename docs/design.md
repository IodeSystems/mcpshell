# Design notes

Why some behaviour is the way it is. Work items are aw4 issues (project `mcpshell`), not here.

## Upstream tool calls

- **Arguments are checked before the call, but hidden options pass.** Some servers accept undeclared options on purpose, such as a `confirm` nonce for a two-phase ask → detail → continue flow. On an open schema, an unknown argument name fails only when it is a likely mistake for a declared one: the same name in another case style, or one edit away when both names are at least 5 characters (`mcp/toolkit.go` `checkArgs`, `likelyMeant`). A schema with `additionalProperties: false` rejects every undeclared name.
- **Missing required arguments are reported all at once** (`checkRequired`), so an agent does not discover them one turn at a time.
- **Types are converted only when nothing is lost** (`mcp/argtypes.go`): number/bool → string, numeric string → number, `"true"`/`"false"` → boolean, JSON string → array/object, single value → `[value]`. `5.5` for an integer fails instead of rounding. The checks skip properties without a plain `type` (anyOf/oneOf/$ref) and `null` values. Array items are not converted.
- **Escapes.** A server entry's `"unchecked": [globs]` covers tools with wrong schemas. `ns.raw("tool", {args})` skips every check for one call and returns unparsed text.
- **help is sized to save turns.** `help("ns")` lists every tool with its signature, grouped by category, up to 16 KB (`helpListAllMax`; the bench's 102-tool Acme server is 10.5 KB against 34 KB of schemas). Above that it shows categories with three names each. `help("ns.tool")` lists each parameter with its type or enum, default and description, and an error from the tool carries the same list. On the DeepSeek hosted run (2026-10-25) discovery cost 3–5 help() turns per task, and a limit stated only in a parameter description caused 6 error turns.
- **Numbers stay exact across the boundary.** Tool output and eval `vars` decode through the same parser as `parseJson`, which keeps exact numbers and key order. Arguments go upstream as `json.Number`. `JSON.stringify` of a parsed value keeps every digit but re-formats it (`1e300` → 301 digits, trailing decimal zeros dropped). Use `ns.raw` when a file must match the tool's output byte for byte.

## Language

- **`await x` is the identity.** Tool calls return their values directly, so there is nothing to wait for. Agents write `await` by habit, and rejecting it sent them looking for a concurrency problem. `async` functions are still unsupported.
- **JS habits run, with a warning.** An agent pays a turn for every refusal, so code that works in JS works here when the meaning is clear, and the eval result carries `warning: line N: …` lines (also after an error) when it may not do what was meant.
  - Declaring a builtin's name (`let all = []`) replaces it for the rest of the scope, with a warning. Refusing it was round-1 behaviour; on the hosted benchmark it cost 5 of 14 error turns. Command names (`sum`, `count`) were always declarable and do not warn: a warning on every common name would be noise.
  - Extra arguments to a command are dropped, as in JS, and warned with the line: the signature's parameter count is the limit (`CommandDef.MaxArgs`); a signature with `...` or no signature is not checked, and one object argument is named arguments. `round(x, 2)` returned an integer with no sign of it before.
  - Named arguments work on every callable: user functions by their parameters, commands by their signature's names, upstream tools by their schema's property names, in calls and pipes (`xs |> limit(n: 2)` — the piped value is the first argument). A null in an upstream call's position is a skipped argument and is not sent. On the second DeepSeek hosted run, `acme.list_customers(region: "West", page_size: 100)` failed 3 times with "no parameter names".
  - Reading an absent object key gives null, as JS gives undefined, and that is how optional fields are checked, so it is not warned in general. It is warned for `.length` on an object (the array is usually inside: `.items.length`) and for a key that resembles an existing one (`o.nmae`). Warnings are kept on `ExecutionLimits`, which every interpreter of one eval shares, so a callback that a command runs (the fn given to `map`) warns too.
  - `floor`, `ceil`, `trunc`, `abs` and `toFixed` are exact for exact values. `toFixed` rounds as `round` does, so `(1.005).toFixed(2)` is `"1.01"` where JS gives `"1.00"` from its float64 value: the README promises exact numbers, and two answers for one rounding would be worse.
  - `round(n, digits?)` rounds a half toward +∞ like JS `Math.round` (it was round-half-even), exactly for exact values: `round(1.005, 2)` is `1.01`.
- **A caught error is its message string.** `"x" + e` keeps working. `e.message` returns the string for JS habit. Any other unknown member on a string raises an error, because silently returning `null` hid real errors ("ERR null").

## Large output

- **A truncated value is kept, not lost.** Without a file, the rest of an over-limit result was gone, and getting it meant running the eval again — every upstream call again. dun's `recap({ref, grep})` reads back a spilled text file; mcpshell holds the structured value, so it keeps it as `$rN` (the last `KeptResults` = 5) and the note gives its inferred TypeScript type (`runtime.InferType`). Querying `$r1 |> filter(…)` is more precise than grepping a text copy. A dropped ref errors with the refs still kept.
- **The type is inferred from the whole value**, not the shown part: array elements merge, keys missing from some objects are optional, a string with ≤ 5 distinct values each seen twice on average is a union of the values seen (values seen, not a schema), and objects whose keys are data (groupBy, countBy) are `Record<…>`. The type is capped at 600 bytes; past it objects list their first keys and "…N more".

- `MCPSHELL_TMP_OVERFLOW_DIR` receives the full over-limit eval output. mcpshell makes file names unique only. Scoping per session is the launcher's job: dun uses `<sessions>/<root>/overflow/<session-id>` and removes it on `ForgetSession`.
- Arrays are saved as JSON Lines and objects one key per line, so line-based `read`/`grep` can page them.

## Build

- `make vet`, not `go vet ./...`. ANTLR ends every rule in the generated `parser/` with a dead `goto errorExit` so the label is always used. vet reports each one as unreachable code (69 reports, the same at v0.4.1). These reports cannot be acted on in a generated file.
- `make release DIST=<dir outside the checkout>`, run from a clean checkout of the tag, so the build stamps the tag as the version. Go stamps a pseudo-version when the tree is dirty. v0.5.0's `.gitignore` has no `/dist/`, so output written into that checkout would make it dirty. Writing outside the checkout avoids that.
- Release builds use `-tags release`. dun classifies a binary from its build info: a `vcs.revision` without the release tag reads as "built from a checkout", so v0.6.0's tarballs showed in `dun doctor` as a checkout at de7d047 although their module version was v0.6.0. `-trimpath` drops `-ldflags` from the build info, so the tag is the only mark that survives. No source file uses the tag.
