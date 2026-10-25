# Design notes

Why some behaviour is the way it is. Work items are aw4 issues (project `mcpshell`), not here.

## Upstream tool calls

- **Arguments are checked before the call, but hidden options pass.** Some servers accept undeclared options on purpose, such as a `confirm` nonce for a two-phase ask → detail → continue flow. On an open schema, an unknown argument name fails only when it is a likely mistake for a declared one: the same name in another case style, or one edit away when both names are at least 5 characters (`mcp/toolkit.go` `checkArgs`, `likelyMeant`). A schema with `additionalProperties: false` rejects every undeclared name.
- **Missing required arguments are reported all at once** (`checkRequired`), so an agent does not discover them one turn at a time.
- **Types are converted only when nothing is lost** (`mcp/argtypes.go`): number/bool → string, numeric string → number, `"true"`/`"false"` → boolean, JSON string → array/object, single value → `[value]`. `5.5` for an integer fails instead of rounding. The checks skip properties without a plain `type` (anyOf/oneOf/$ref) and `null` values. Array items are not converted.
- **Escapes.** A server entry's `"unchecked": [globs]` covers tools with wrong schemas. `ns.raw("tool", {args})` skips every check for one call and returns unparsed text.
- **Numbers stay exact across the boundary.** Tool output and eval `vars` decode through the same parser as `parseJson`, which keeps exact numbers and key order. Arguments go upstream as `json.Number`. `JSON.stringify` of a parsed value keeps every digit but re-formats it (`1e300` → 301 digits, trailing decimal zeros dropped). Use `ns.raw` when a file must match the tool's output byte for byte.

## Language

- **`await x` is the identity.** Tool calls return their values directly, so there is nothing to wait for. Agents write `await` by habit, and rejecting it sent them looking for a concurrency problem. `async` functions are still unsupported.
- **A caught error is its message string.** `"x" + e` keeps working. `e.message` returns the string for JS habit. Any other unknown member on a string raises an error, because silently returning `null` hid real errors ("ERR null").

## Large output

- `MCPSHELL_TMP_OVERFLOW_DIR` receives the full over-limit eval output. mcpshell makes file names unique only. Scoping per session is the launcher's job: dun uses `<sessions>/<root>/overflow/<session-id>` and removes it on `ForgetSession`.
- Arrays are saved as JSON Lines and objects one key per line, so line-based `read`/`grep` can page them.

## Build

- `make vet`, not `go vet ./...`. ANTLR ends every rule in the generated `parser/` with a dead `goto errorExit` so the label is always used. vet reports each one as unreachable code (69 reports, the same at v0.4.1). These reports cannot be acted on in a generated file.
- `make release DIST=<dir outside the checkout>`, run from a clean checkout of the tag, so the build stamps the tag as the version. Go stamps a pseudo-version when the tree is dirty. v0.5.0's `.gitignore` has no `/dist/`, so output written into that checkout would make it dirty. Writing outside the checkout avoids that.
