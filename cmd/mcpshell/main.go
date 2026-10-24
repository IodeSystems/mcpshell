// Command mcpshell is a small CLI for the mcpshell interpreter: evaluate code from
// an argument, a file, or stdin, or run an interactive REPL.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

const usage = `mcpshell — sandboxed JS-subset scripting language

Usage:
  mcpshell '<code>'      evaluate code and print the result
  mcpshell -f <file>     evaluate a file
  echo '<code>' | mcpshell   evaluate piped stdin
  mcpshell               start an interactive REPL (no args, a TTY)
  mcpshell --prompt      print the LLM system prompt
  mcpshell mcp [flags]   run as an MCP server over stdio
  mcpshell -h            show this help
  mcpshell --version     print the version

For direct evaluation and the REPL, the core, math, web, file, and graph
toolkits are all installed; file operations are confined to the current
working directory. In mcp mode only core, math, and graph load by default —
web, file, sql, and browser are opt-in via the flags below.

mcp flags:
  --files-dir <dir>    enable the file toolkit rooted at <dir>
  --files-read-only    file toolkit is read-only
  --web                enable the web toolkit (Web.*, Html.*)
  --browser            enable the browser toolkit (Browser.*, needs a Chrome binary)
  --sql <spec>         add a SQL database: 'ns=dsn' or 'dsn' (SQLite path or postgres:// URL)
  --sql-writable       SQL databases are read-write (default: read-only)
  --max-output <n>     cap eval output bytes (default 16000)
  --connect <spec>     compose an upstream MCP server: 'ns=cmd args', 'cmd args',
                       'ns=https://…', or 'https://…' (streamable HTTP)
  --mcp <spec>         upstream MCP config: JSON file, inline JSON, or 'name:cmd args'
                       (JSON entries take 'command'/'args'/'env', or 'url'/'headers'/
                       'auth_cmd' — a shell command printing a bearer token; any entry
                       takes 'categories': {"name": ["glob_*", …]} to group its tools)

env:
  MCPSHELL_TMP_OVERFLOW_DIR  save eval output over --max-output to a new file in
                       this dir; the truncated response names the file, and
                       read()/grep() accept its absolute path. Scope it per
                       session — mcpshell only makes file names unique.

  --connect/--mcp are repeatable. An upstream server that identifies as mcpshell
  is skipped — composing mcpshell with itself would let eval recurse infinitely.`

// newShell builds a shell with every zero-config toolkit installed.
func newShell() *runtime.Shell {
	sh := runtime.NewShell()
	toolkit.InstallCore(sh)
	toolkit.InstallMath(sh)
	toolkit.InstallWeb(sh)
	toolkit.InstallGraph(sh)
	if wd, err := os.Getwd(); err == nil {
		toolkit.InstallFile(sh, wd, false)
	}
	return sh
}

// version is the module version for a go install @version build, else the
// VCS revision, else "dev".
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 12 {
			return "dev-" + s.Value[:12]
		}
	}
	return "dev"
}

func main() {
	mcp.Version = version()
	args := os.Args[1:]

	switch {
	case len(args) > 0 && (args[0] == "-h" || args[0] == "--help"):
		fmt.Println(usage)
		return
	case len(args) > 0 && (args[0] == "--version" || args[0] == "-v"):
		fmt.Println("mcpshell " + version())
		return
	case len(args) > 0 && args[0] == "mcp":
		runMCP(args[1:])
		return
	case len(args) > 0 && args[0] == "--prompt":
		fmt.Println(newShell().ToPrompt(false))
		return
	case len(args) == 2 && args[0] == "-f":
		src, err := os.ReadFile(args[1])
		if err != nil {
			fail("cannot read %s: %v", args[1], err)
		}
		evalAndPrint(newShell(), string(src))
		return
	case len(args) > 0:
		evalAndPrint(newShell(), strings.Join(args, " "))
		return
	}

	// No args: REPL if interactive, otherwise evaluate piped stdin.
	if info, err := os.Stdin.Stat(); err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
		repl()
		return
	}
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail("cannot read stdin: %v", err)
	}
	evalAndPrint(newShell(), string(src))
}

// evalAndPrint evaluates src, printing the result or exiting on error.
func evalAndPrint(sh *runtime.Shell, src string) {
	v, err := sh.Eval(src)
	if err != nil {
		fail("%v", err)
	}
	if _, isNull := v.(*runtime.NullVal); !isNull {
		fmt.Println(v.Display())
	}
}

// repl runs a line-based read-eval-print loop. Use `export` to persist state.
func repl() {
	fmt.Fprintln(os.Stderr, "mcpshell REPL — Ctrl-D to exit. Use `export` to persist values across lines.")
	sh := newShell()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Fprint(os.Stderr, "mcpshell> ")
		if !scanner.Scan() {
			fmt.Fprintln(os.Stderr)
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		v, err := sh.Eval(line)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		if _, isNull := v.(*runtime.NullVal); !isNull {
			fmt.Println(v.Display())
		}
	}
}

// stringSlice collects a repeatable string flag.
type stringSlice []string

func (s *stringSlice) String() string     { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error { *s = append(*s, v); return nil }

// runMCP serves the MCP protocol over stdio, with toolkits selected by flags.
func runMCP(args []string) {
	fs := flag.NewFlagSet("mcpshell mcp", flag.ExitOnError)
	filesDir := fs.String("files-dir", "", "enable the file toolkit rooted at this directory")
	filesReadOnly := fs.Bool("files-read-only", false, "file toolkit is read-only")
	web := fs.Bool("web", false, "enable the web toolkit (Web.*, Html.*)")
	browser := fs.Bool("browser", false, "enable the browser toolkit (Browser.*, needs Chrome)")
	maxOutput := fs.Int("max-output", 16000, "cap eval output bytes")
	sqlWritable := fs.Bool("sql-writable", false, "SQL databases are read-write (default: read-only)")
	var connect, mcpConfigs, sqlSpecs stringSlice
	fs.Var(&connect, "connect", "upstream MCP server: 'ns=cmd args' or 'cmd args' (repeatable)")
	fs.Var(&mcpConfigs, "mcp", "MCP config: JSON file, inline JSON, or 'name:cmd args' (repeatable)")
	fs.Var(&sqlSpecs, "sql", "SQL database: 'ns=dsn' or 'dsn' (SQLite path or postgres:// URL, repeatable)")
	if err := fs.Parse(args); err != nil {
		fail("%v", err)
	}

	sh := runtime.NewShell()
	toolkit.InstallCore(sh)
	toolkit.InstallMath(sh)
	toolkit.InstallGraph(sh)
	if *web {
		toolkit.InstallWeb(sh)
		fmt.Fprintln(os.Stderr, "mcpshell mcp: web toolkit enabled")
	}
	overflowDir := os.Getenv("MCPSHELL_TMP_OVERFLOW_DIR")
	if overflowDir != "" {
		if err := os.MkdirAll(overflowDir, 0o700); err != nil {
			fail("MCPSHELL_TMP_OVERFLOW_DIR: %v", err)
		}
		fmt.Fprintf(os.Stderr, "mcpshell mcp: over-limit output is saved to %s\n", overflowDir)
	}
	if *filesDir != "" {
		var readRoots []string
		if overflowDir != "" {
			readRoots = append(readRoots, overflowDir)
		}
		toolkit.InstallFile(sh, *filesDir, *filesReadOnly, readRoots...)
		mode := "read-write"
		if *filesReadOnly {
			mode = "read-only"
		}
		fmt.Fprintf(os.Stderr, "mcpshell mcp: file toolkit enabled (%s: %s)\n", mode, *filesDir)
	} else if overflowDir != "" {
		// Without --files-dir, a read-only file toolkit on the overflow dir is
		// what lets read()/grep() page through saved output.
		toolkit.InstallFile(sh, overflowDir, true)
	}

	var closers []io.Closer
	for _, spec := range sqlSpecs {
		ns, dsn := parseSQLSpec(spec)
		closer, err := toolkit.InstallSQL(sh, ns, dsn, !*sqlWritable)
		if err != nil {
			fail("%v", err)
		}
		closers = append(closers, closer)
		mode := "read-only"
		if *sqlWritable {
			mode = "read-write"
		}
		fmt.Fprintf(os.Stderr, "mcpshell mcp: SQL database %q enabled (%s)\n", ns, mode)
	}
	if *browser {
		closers = append(closers, toolkit.InstallBrowser(sh, true))
		fmt.Fprintln(os.Stderr, "mcpshell mcp: browser toolkit enabled (Browser.*)")
	}

	var servers []mcp.NamedServer
	for _, spec := range connect {
		servers = append(servers, parseConnectSpec(spec))
	}
	for _, spec := range mcpConfigs {
		servers = append(servers, parseMCPConfig(spec)...)
	}
	if len(servers) > 0 {
		closers = append(closers, mcp.InstallClients(sh, servers)...)
	}

	fmt.Fprintln(os.Stderr, "mcpshell mcp: serving on stdio")
	err := mcp.NewServer(sh, *maxOutput).WithOverflowDir(overflowDir).RunStdio(os.Stdin, os.Stdout)
	for _, c := range closers {
		_ = c.Close()
	}
	if err != nil {
		fail("mcp server: %v", err)
	}
}

// parseConnectSpec parses a --connect value: 'namespace=command args' or, with
// no explicit namespace, 'command args' (the namespace is derived from the
// command's base name).
func parseConnectSpec(spec string) mcp.NamedServer {
	spec = strings.TrimSpace(spec)
	if isURL(spec) {
		return mcp.NamedServer{Namespace: urlNamespace(spec), Config: mcp.ServerConfig{URL: spec}}
	}
	if before, after, found := strings.Cut(spec, "="); found && !strings.ContainsAny(before, " \t") {
		return mcp.NamedServer{Namespace: before, Config: serverFromSpec(after)}
	}
	parts := strings.Fields(spec)
	name := "server"
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if i := strings.LastIndexByte(last, '/'); i >= 0 {
			last = last[i+1:]
		}
		if i := strings.LastIndexByte(last, '.'); i >= 0 {
			last = last[:i]
		}
		name = sanitizeIdent(last)
	}
	return mcp.NamedServer{Namespace: name, Config: mcp.ServerConfig{Command: parts}}
}

// parseMCPConfig parses a --mcp value: a 'name:command args' shorthand, inline
// JSON, or a path to a JSON file in the standard {"mcpServers": {...}} format.
func parseMCPConfig(arg string) []mcp.NamedServer {
	trimmed := strings.TrimSpace(arg)
	if !strings.HasPrefix(trimmed, "{") && !fileExists(arg) && strings.Contains(arg, ":") {
		before, after, _ := strings.Cut(arg, ":")
		name := strings.TrimSpace(before)
		command := strings.Fields(strings.TrimSpace(after))
		if name != "" && len(command) > 0 {
			cfg := serverFromSpec(after)
			cfg.Label = name
			return []mcp.NamedServer{{Namespace: name, Config: cfg}}
		}
	}

	jsonText := arg
	if !strings.HasPrefix(trimmed, "{") {
		data, err := os.ReadFile(arg)
		if err != nil {
			fail("MCP config not found: %q (expected a JSON file, inline JSON, or name:command)", arg)
		}
		jsonText = string(data)
	}

	var root struct {
		McpServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
			AuthCmd string            `json:"auth_cmd"`
			// category → tool-name globs, for both transports
			Categories map[string][]string `json:"categories"`
			// tool-name globs whose arguments are sent unchecked and unconverted
			Unchecked []string `json:"unchecked"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(jsonText), &root); err != nil {
		fail("invalid JSON in MCP config: %v", err)
	}
	if root.McpServers == nil {
		fail("expected an 'mcpServers' key in the MCP config")
	}
	names := make([]string, 0, len(root.McpServers))
	for name := range root.McpServers {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []mcp.NamedServer
	for _, name := range names {
		srv := root.McpServers[name]
		for cat, globs := range srv.Categories {
			for _, g := range globs {
				if _, err := path.Match(g, ""); err != nil {
					fail("MCP server %q category %q: bad glob %q", name, cat, g)
				}
			}
		}
		for _, g := range srv.Unchecked {
			if _, err := path.Match(g, ""); err != nil {
				fail("MCP server %q unchecked: bad glob %q", name, g)
			}
		}
		switch {
		case srv.Command != "" && srv.URL != "":
			fail("MCP server %q has both 'command' and 'url' — pick one", name)
		case srv.URL != "":
			out = append(out, mcp.NamedServer{
				Namespace: name,
				Config: mcp.ServerConfig{URL: srv.URL, Headers: srv.Headers, AuthCmd: srv.AuthCmd,
					Categories: srv.Categories, Unchecked: srv.Unchecked, Label: name},
			})
			continue
		case srv.Command == "":
			fail("MCP server %q needs a 'command' or a 'url' field", name)
		}
		out = append(out, mcp.NamedServer{
			Namespace: name,
			Config: mcp.ServerConfig{
				Command:    append([]string{srv.Command}, srv.Args...),
				Env:        srv.Env,
				Categories: srv.Categories,
				Unchecked:  srv.Unchecked,
				Label:      name,
			},
		})
	}
	return out
}

// serverFromSpec turns the command part of a --connect/--mcp shorthand into a
// config: a URL connects over HTTP, anything else is a command line.
func serverFromSpec(spec string) mcp.ServerConfig {
	spec = strings.TrimSpace(spec)
	if isURL(spec) {
		return mcp.ServerConfig{URL: spec}
	}
	return mcp.ServerConfig{Command: strings.Fields(spec)}
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// urlNamespace derives a namespace from a URL's host: the label before the
// TLD ("api.us.elevenlabs.io" → "elevenlabs"); "server" for an IP address.
func urlNamespace(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || net.ParseIP(u.Hostname()) != nil {
		return "server"
	}
	labels := strings.Split(u.Hostname(), ".")
	if len(labels) >= 2 {
		return sanitizeIdent(labels[len(labels)-2])
	}
	return sanitizeIdent(labels[0])
}

func sanitizeIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "server"
	}
	return b.String()
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// parseSQLSpec parses a --sql value: 'namespace=dsn', or just 'dsn' with the
// namespace defaulting to "db".
func parseSQLSpec(spec string) (namespace, dsn string) {
	spec = strings.TrimSpace(spec)
	if before, after, found := strings.Cut(spec, "="); found && !strings.ContainsAny(before, " \t/:.\\") {
		return before, after
	}
	return "db", spec
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "mcpshell: "+format+"\n", a...)
	os.Exit(1)
}
