# Safe untrusted execution

The reason you'd hand an LLM `eval` at all: mcpshell is a **sandboxed
JS-*subset*, not a language runtime.** You can run model-authored code in an
untrusted environment without exposing the host — something you *cannot* safely
do by handing the model a Python, Node, or shell tool, whose stdlibs reach the
filesystem, network, and process table by default.

The guarantee is enforced by the language, not by policy or a blocklist — there
is simply no construct that reaches the host. Verified in
[`toolkit/sandbox_test.go`](../../toolkit/sandbox_test.go):

| Attempt | Python/`bash` exec tool | mcpshell |
|---------|:-----------------------:|:--------:|
| `require("fs")` / `import` | ✅ loads modules | ❌ no module system |
| `eval(...)` / `Function(...)` | ✅ | ❌ no nested/host eval |
| `process`, `process.exit` | ✅ | ❌ no process object |
| `globalThis`, `this.constructor` | ✅ walks to host | ❌ no global / no prototype chain |
| `readFile("/etc/passwd")` | ✅ reads it | ❌ no ambient filesystem |
| `fetch("http://169.254.169.254/")` (SSRF) | ✅ | ❌ no ambient network |
| `exec("id")` / `system(...)` | ✅ spawns | ❌ no process spawn |
| `while (true) {}` | ✅ hangs the host | ❌ step-limit error |
| unbounded recursion | ✅ stack blows | ❌ call-depth error |

Two properties do the work:

- **No escape hatch.** The subset has no imports, no `eval`, no `this`/prototype
  chain, no global object, and no ambient I/O. Model-authored code can only call
  the commands you registered — nothing else exists to call.
- **Capabilities are opt-in and scoped.** A core-only shell can't touch the
  network or disk *by name*. Filesystem access appears only when you
  `InstallFile(root, readOnly)` — rooted at a directory you choose, optionally
  read-only. Network only with `InstallWeb`. SQL only with `InstallSQL(dsn,
  readOnly)`. You grant exactly the surface the task needs.
- **Bounded resources.** Every eval carries a step budget, call-depth cap,
  wall-clock timeout, and output cap, so untrusted code can't hang or exhaust the
  host — a runaway loop returns an error, it doesn't take the process down.

This is the property a raw `python -c` / `bash -c` tool can't offer, and it's
why one sandboxed `eval` is safe to expose where a general interpreter is not.
