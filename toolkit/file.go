package toolkit

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	. "github.com/iodesystems/mcpshell/runtime"
)

// fileCfg holds the FileToolkit configuration.
type fileCfg struct {
	root         string
	readOnly     bool
	maxReadLines int
}

// confine resolves path against the root and rejects anything escaping it.
func (c fileCfg) confine(path string) string {
	resolved := filepath.Clean(filepath.Join(c.root, path))
	rel, err := filepath.Rel(c.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		panic(Runtime("Access denied: '" + path + "' resolves outside the allowed directory\n\n" +
			"  Root: " + c.root + "\n  Resolved: " + resolved + "\n\n" +
			"  All file operations are confined to the root directory."))
	}
	return resolved
}

// InstallFile registers the file toolkit, confining all operations to root.
// When readOnly is true, the mutating commands are not registered.
func InstallFile(sh *Shell, root string, readOnly bool) *Shell {
	abs, err := filepath.Abs(root)
	if err != nil {
		panic("FileToolkit: cannot resolve root: " + err.Error())
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		panic("FileToolkit root does not exist or is not a directory: " + abs)
	}
	cfg := fileCfg{root: abs, readOnly: readOnly, maxReadLines: 200}

	reg := func(name, sig, desc string, examples []string, fn NativeFn) {
		sh.Register(&CommandDef{Name: name, Signature: sig, Description: desc, Examples: examples, Fn: fn})
	}

	mode := "read, write, search, and edit files"
	if readOnly {
		mode = "read and search files (read-only mode)"
	}
	sh.RegisterGuide("files", "File Toolkit — "+mode+`

TYPICAL: Read and inspect files
  read("config.json")             // full file with line numbers
  read("big.log", 100, 20)        // 20 lines starting at line 100
  readLines("data.csv") |> len()  // count lines
  exists("output.txt")
  ls("src") |> filter(f => f.type == "file") |> map(f => f.name)

TYPICAL: Search files and content
  glob("src/**/*.go")             // find files by pattern
  glob("**/*.go", {grep: "TODO"}) // → [{file, num, line}]
  grep("app.go", {match: "TODO", context: 2})

TYPICAL: Write and edit files
  write("output.txt", "hello world")
  append("log.txt", "new entry\n")
  edit("app.go", "const x = 1", "const x = 42")`)

	reg("glob", "pattern: string, opts?: {grep?: string, depth?: number, dirs?: boolean, hidden?: boolean}",
		"finds files matching glob pattern. Skips hidden and build dirs by default. "+
			"{grep: pattern} searches content returning {file, num, line} hits",
		[]string{`glob("src/**/*.go")`, `glob("**/*.go", {grep: "TODO"})`},
		func(args []Value) Value {
			pattern := requireStringArg("glob", args, 0)
			opts, _ := argOpt(args, 1).(*ObjectVal)
			grepPat, hasGrep := optObjStr(opts, "grep")
			maxDepth := optObjInt(opts, "depth")
			includeDirs := optObjBool(opts, "dirs")
			includeHidden := optObjBool(opts, "hidden")

			re := globToRegex(pattern)
			matched := cfg.walk(maxDepth, includeHidden, includeDirs, func(rel string, isDir bool) bool {
				return re.MatchString(rel)
			})

			if hasGrep {
				grepRe := compileRegex(grepPat)
				var hits []Value
				for _, rel := range matched {
					info, err := os.Stat(filepath.Join(cfg.root, rel))
					if err != nil || info.IsDir() {
						continue
					}
					for i, line := range cfg.readLinesOrEmpty(rel) {
						if regexMatch(grepRe, line) {
							o := NewObject()
							o.Set("file", Str(rel))
							o.Set("num", Num(float64(i+1)))
							o.Set("line", Str(line))
							hits = append(hits, o)
						}
					}
				}
				return &ArrayVal{Elements: hits}
			}
			return strArr(matched)
		})

	reg("read", "path: string, start?: number, lines?: number",
		fmt.Sprintf("reads file with line numbers (1: content). Optional start line and count for partial reads. "+
			"Large files truncated to %d lines", cfg.maxReadLines),
		[]string{`read("config.json")`, `read("big.log", 100, 20)`},
		func(args []Value) Value {
			path := requireStringArg("read", args, 0)
			start := optInt(args, 1)
			count := optInt(args, 2)
			resolved := cfg.confine(path)
			if !pathExists(resolved) {
				panic(Runtime("read: file not found '" + path + "'\n\n  Resolved to: " + resolved))
			}
			lines := readLinesOf(resolved)
			total := len(lines)
			width := len(strconv.Itoa(total))
			if width == 0 {
				width = 1
			}
			if start != nil {
				startIdx := max(*start-1, 0)
				n := total - startIdx
				if count != nil {
					n = *count
				}
				return Str(numberLines(takeSlice(lines, startIdx, n), startIdx+1, width))
			}
			if total > cfg.maxReadLines {
				return Str(numberLines(lines[:cfg.maxReadLines], 1, width) +
					fmt.Sprintf("\n\n... truncated (showing %d of %d lines). Use read(%q, %d) to continue.",
						cfg.maxReadLines, total, path, cfg.maxReadLines+1))
			}
			return Str(numberLines(lines, 1, width))
		})

	reg("readLines", "path: string", "reads file as array of lines",
		[]string{`readLines("data.txt")`},
		func(args []Value) Value {
			path := requireStringArg("readLines", args, 0)
			resolved := cfg.confine(path)
			if !pathExists(resolved) {
				panic(Runtime("readLines: file not found '" + path + "'\n\n  Resolved to: " + resolved))
			}
			return strArr(readLinesOf(resolved))
		})

	reg("exists", "path: string", "checks if file or directory exists",
		[]string{`exists("config.json")`},
		func(args []Value) Value {
			return Bln(pathExists(cfg.confine(requireStringArg("exists", args, 0))))
		})

	reg("ls", "path?: string", "lists directory contents as array of {name, type, size}",
		[]string{`ls()`, `ls("src")`},
		func(args []Value) Value {
			path := optString(args, 0, "")
			resolved := cfg.confine(path)
			info, err := os.Stat(resolved)
			if err != nil || !info.IsDir() {
				panic(Runtime("ls: not a directory '" + path + "'\n\n  Resolved to: " + resolved))
			}
			entries, err := os.ReadDir(resolved)
			if err != nil {
				panic(Runtime("ls: " + err.Error()))
			}
			out := make([]Value, 0, len(entries))
			for _, e := range entries {
				full := filepath.Join(resolved, e.Name())
				rel, _ := filepath.Rel(cfg.root, full)
				o := NewObject()
				o.Set("name", Str(filepath.ToSlash(rel)))
				size := 0.0
				if e.IsDir() {
					o.Set("type", Str("dir"))
				} else {
					o.Set("type", Str("file"))
					if fi, err := e.Info(); err == nil {
						size = float64(fi.Size())
					}
				}
				o.Set("size", Num(size))
				out = append(out, o)
			}
			return &ArrayVal{Elements: out}
		})

	reg("tree", "path?: string, opts?: {depth?: number, files?: boolean}",
		"shows directory tree. Default: dirs with file counts, depth 4. Skips hidden/build dirs",
		[]string{`tree()`, `tree("src", {depth: 6, files: true})`},
		func(args []Value) Value {
			path := optString(args, 0, "")
			opts, _ := argOpt(args, 1).(*ObjectVal)
			maxDepth := 4
			if d := optObjInt(opts, "depth"); d != nil {
				maxDepth = *d
			}
			showFiles := optObjBool(opts, "files")
			resolved := cfg.confine(path)
			info, err := os.Stat(resolved)
			if err != nil || !info.IsDir() {
				panic(Runtime("tree: not a directory '" + path + "'\n\n  Resolved to: " + resolved))
			}
			rootName := "."
			if path != "" {
				rootName = path
			}
			return Str(renderTree(resolved, rootName, maxDepth, showFiles))
		})

	reg("grep", "path: string, opts: {match: string, context?: number, mode?: string, limit?: number}",
		"searches file for pattern. mode: \"count\" → count, \"files\" → boolean, else [{line, num}]",
		[]string{`grep("app.go", {match: "TODO"})`, `grep("app.go", "import")`},
		func(args []Value) Value {
			path := requireStringArg("grep", args, 0)
			var match, mode string
			contextLines := 0
			limit := -1
			switch o := argOpt(args, 1).(type) {
			case *ObjectVal:
				m, ok := optObjStr(o, "match")
				if !ok {
					panic(WrongArguments("grep", "path, {match: string, context?: number, mode?: string, limit?: number}", args,
						`grep("file.txt", {match: "pattern"})`))
				}
				match = m
				if c := optObjInt(o, "context"); c != nil {
					contextLines = *c
				}
				if md, ok := optObjStr(o, "mode"); ok {
					mode = md
				} else {
					mode = "content"
				}
				if l := optObjInt(o, "limit"); l != nil {
					limit = *l
				}
			case *StringVal:
				match, mode = o.V, "content"
			default:
				panic(WrongArguments("grep", "path, {match: string}", args, `grep("file.txt", {match: "pattern"})`))
			}

			resolved := cfg.confine(path)
			if !pathExists(resolved) {
				panic(Runtime("grep: file not found '" + path + "'\n\n  Resolved to: " + resolved))
			}
			lines := readLinesOf(resolved)
			re := compileRegex(match)

			switch mode {
			case "count":
				c := 0
				for _, l := range lines {
					if regexMatch(re, l) {
						c++
					}
				}
				return Num(float64(c))
			case "files":
				return Bln(slices.ContainsFunc(lines, func(l string) bool { return regexMatch(re, l) }))
			default:
				var results []Value
				for idx, line := range lines {
					if limit >= 0 && len(results) >= limit {
						break
					}
					if !regexMatch(re, line) {
						continue
					}
					entry := NewObject()
					entry.Set("line", Str(line))
					entry.Set("num", Num(float64(idx+1)))
					if contextLines > 0 {
						lo := max(0, idx-contextLines)
						hi := min(len(lines)-1, idx+contextLines)
						var ctx []Value
						for i := lo; i <= hi; i++ {
							co := NewObject()
							co.Set("num", Num(float64(i+1)))
							co.Set("line", Str(lines[i]))
							co.Set("match", Bln(i == idx))
							ctx = append(ctx, co)
						}
						entry.Set("context", &ArrayVal{Elements: ctx})
					}
					results = append(results, entry)
				}
				return &ArrayVal{Elements: results}
			}
		})

	if readOnly {
		return sh
	}

	reg("write", "path: string, content: string", "writes content to file, creates parent directories",
		[]string{`write("out.txt", "hello")`},
		func(args []Value) Value {
			path := requireStringArg("write", args, 0)
			content := requireStringArg("write", args, 1)
			resolved := cfg.confine(path)
			mustMkdirParent(resolved)
			if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
				panic(Runtime("write: " + err.Error()))
			}
			return Null
		})

	reg("append", "path: string, content: string", "appends content to file",
		[]string{`append("log.txt", "entry\n")`},
		func(args []Value) Value {
			path := requireStringArg("append", args, 0)
			content := requireStringArg("append", args, 1)
			resolved := cfg.confine(path)
			mustMkdirParent(resolved)
			f, err := os.OpenFile(resolved, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				panic(Runtime("append: " + err.Error()))
			}
			defer f.Close()
			if _, err := f.WriteString(content); err != nil {
				panic(Runtime("append: " + err.Error()))
			}
			return Null
		})

	reg("edit", "path: string, old: string, new: string, opts?: {all?: boolean}",
		"replaces exact string match in file. Fails if not found, or ambiguous unless all=true",
		[]string{`edit("app.go", "const x = 1", "const x = 42")`},
		func(args []Value) Value {
			path := requireStringArg("edit", args, 0)
			oldStr := requireStringArg("edit", args, 1)
			newStr := requireStringArg("edit", args, 2)
			all := optObjBool(asObject(argOpt(args, 3)), "all")
			resolved := cfg.confine(path)
			if !pathExists(resolved) {
				panic(Runtime("edit: file not found '" + path + "'\n\n  Resolved to: " + resolved))
			}
			data, err := os.ReadFile(resolved)
			if err != nil {
				panic(Runtime("edit: " + err.Error()))
			}
			content := string(data)
			count := strings.Count(content, oldStr)
			switch {
			case count == 0:
				panic(Runtime("edit: old string not found in '" + path + "'\n\n" +
					"  Searched for:\n    " + strings.ReplaceAll(oldStr, "\n", "\n    ") + "\n\n" +
					"  Hint: the old string must match exactly, including whitespace and indentation"))
			case count > 1 && !all:
				panic(Runtime(fmt.Sprintf("edit: old string appears %d times in '%s'\n\n"+
					"  Provide more surrounding context to make the match unique, or use {all: true}", count, path)))
			}
			var updated string
			if all {
				updated = strings.ReplaceAll(content, oldStr, newStr)
			} else {
				updated = strings.Replace(content, oldStr, newStr, 1)
			}
			if err := os.WriteFile(resolved, []byte(updated), 0o644); err != nil {
				panic(Runtime("edit: " + err.Error()))
			}
			return Null
		})

	reg("deletePath", "path: string", "deletes a file or empty directory",
		[]string{`deletePath("temp.txt")`},
		func(args []Value) Value {
			path := requireStringArg("deletePath", args, 0)
			resolved := cfg.confine(path)
			if !pathExists(resolved) {
				panic(Runtime("deletePath: file not found '" + path + "'\n\n  Resolved to: " + resolved))
			}
			if err := os.Remove(resolved); err != nil {
				panic(Runtime("deletePath: " + err.Error()))
			}
			return Null
		})

	reg("mv", "from: string, to: string", "moves/renames a file or directory",
		[]string{`mv("old.txt", "new.txt")`},
		func(args []Value) Value {
			from := requireStringArg("mv", args, 0)
			to := requireStringArg("mv", args, 1)
			resolvedFrom := cfg.confine(from)
			resolvedTo := cfg.confine(to)
			if !pathExists(resolvedFrom) {
				panic(Runtime("mv: source not found '" + from + "'\n\n  Resolved to: " + resolvedFrom))
			}
			mustMkdirParent(resolvedTo)
			if err := os.Rename(resolvedFrom, resolvedTo); err != nil {
				panic(Runtime("mv: " + err.Error()))
			}
			return Null
		})

	reg("mkdir", "path: string", "creates directory and any parent directories",
		[]string{`mkdir("output/reports")`},
		func(args []Value) Value {
			resolved := cfg.confine(requireStringArg("mkdir", args, 0))
			if err := os.MkdirAll(resolved, 0o755); err != nil {
				panic(Runtime("mkdir: " + err.Error()))
			}
			return Null
		})

	reg("load", "path: string", "loads and evaluates a .mcpshell file, making its definitions available",
		[]string{`load("lib/helpers.mcpshell")`},
		func(args []Value) Value {
			path := requireStringArg("load", args, 0)
			resolved := cfg.confine(path)
			if !pathExists(resolved) {
				panic(Runtime("load: file not found '" + path + "'\n\n  Resolved to: " + resolved))
			}
			data, err := os.ReadFile(resolved)
			if err != nil {
				panic(Runtime("load: " + err.Error()))
			}
			v, evalErr := sh.Eval(string(data))
			if evalErr != nil {
				panic(evalErr)
			}
			return v
		})

	return sh
}

// --- file helpers ------------------------------------------------------------

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func asObject(v Value) *ObjectVal {
	o, _ := v.(*ObjectVal)
	return o
}

func mustMkdirParent(path string) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			panic(Runtime("mkdir parent: " + err.Error()))
		}
	}
}

// readLinesOf reads a file as lines, dropping a single trailing empty line.
func readLinesOf(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(Runtime("read: " + err.Error()))
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func (c fileCfg) readLinesOrEmpty(rel string) []string {
	data, err := os.ReadFile(filepath.Join(c.root, rel))
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func numberLines(lines []string, startNum, width int) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		num := strconv.Itoa(startNum + i)
		b.WriteString(strings.Repeat(" ", width-len(num)))
		b.WriteString(num)
		b.WriteString(": ")
		b.WriteString(line)
	}
	return b.String()
}

func takeSlice(s []string, start, n int) []string {
	if start < 0 {
		start = 0
	}
	if start > len(s) {
		start = len(s)
	}
	end := start + n
	if n < 0 || end > len(s) {
		end = len(s)
	}
	if end < start {
		end = start
	}
	return s[start:end]
}

// globToRegex converts a glob pattern (with `**`) into an anchored regexp over
// slash-separated relative paths.
func globToRegex(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteByte('^')
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch c := runes[i]; c {
		case '*':
			if i+1 < len(runes) && runes[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.', '(', ')', '+', '|', '^', '$', '{', '}', '\\', '[', ']':
			b.WriteByte('\\')
			b.WriteRune(c)
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('$')
	return regexp.MustCompile(b.String())
}

// walk traverses the root, returning slash-relative paths accepted by match.
func (c fileCfg) walk(maxDepth *int, includeHidden, includeDirs bool, match func(rel string, isDir bool) bool) []string {
	var out []string
	_ = filepath.WalkDir(c.root, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == c.root {
			return nil
		}
		rel, relErr := filepath.Rel(c.root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")

		hiddenOrBuild := parts[0] == "build" ||
			slices.ContainsFunc(parts, func(p string) bool { return strings.HasPrefix(p, ".") })
		if !includeHidden && hiddenOrBuild {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if maxDepth != nil && len(parts) > *maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && !includeDirs {
			return nil
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return nil
		}
		if match(rel, d.IsDir()) {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// renderTree builds the textual directory tree.
func renderTree(root, rootName string, maxDepth int, showFiles bool) string {
	filteredChildren := func(dir string) []os.DirEntry {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var kept []os.DirEntry
		for _, e := range entries {
			n := e.Name()
			if !strings.HasPrefix(n, ".") && n != "build" {
				kept = append(kept, e)
			}
		}
		return kept
	}
	lineCount := func(file string) int {
		data, err := os.ReadFile(file)
		if err != nil || len(data) == 0 {
			return 0
		}
		n := strings.Count(string(data), "\n")
		if !strings.HasSuffix(string(data), "\n") {
			n++
		}
		return n
	}

	var b strings.Builder
	var walk func(dir, prefix string, depth int)
	walk = func(dir, prefix string, depth int) {
		if depth > maxDepth {
			b.WriteString(prefix + "...\n")
			return
		}
		children := filteredChildren(dir)
		var dirs, files []os.DirEntry
		for _, ch := range children {
			if ch.IsDir() {
				dirs = append(dirs, ch)
			} else {
				files = append(files, ch)
			}
		}

		// Collapse single-child directory chains.
		if len(dirs) == 1 && len(files) == 0 {
			chain := []string{dirs[0].Name()}
			current := filepath.Join(dir, dirs[0].Name())
			for {
				gc := filteredChildren(current)
				var subDirs, subFiles []os.DirEntry
				for _, ch := range gc {
					if ch.IsDir() {
						subDirs = append(subDirs, ch)
					} else {
						subFiles = append(subFiles, ch)
					}
				}
				if len(subDirs) == 1 && len(subFiles) == 0 {
					chain = append(chain, subDirs[0].Name())
					current = filepath.Join(current, subDirs[0].Name())
				} else {
					break
				}
			}
			b.WriteString(prefix + "└── " + strings.Join(chain, "/") + "/\n")
			walk(current, prefix+"    ", depth+1)
			return
		}

		if showFiles {
			items := append(append([]os.DirEntry(nil), dirs...), files...)
			for i, ch := range items {
				isLast := i == len(items)-1
				connector := "├── "
				if isLast {
					connector = "└── "
				}
				if ch.IsDir() {
					b.WriteString(prefix + connector + ch.Name() + "/\n")
					next := prefix + "│   "
					if isLast {
						next = prefix + "    "
					}
					walk(filepath.Join(dir, ch.Name()), next, depth+1)
				} else {
					fmt.Fprintf(&b, "%s%s%s (%d lines)\n", prefix, connector, ch.Name(),
						lineCount(filepath.Join(dir, ch.Name())))
				}
			}
		} else {
			totalItems := len(dirs)
			if len(files) > 0 {
				totalItems++
			}
			idx := 0
			for _, d := range dirs {
				idx++
				isLast := idx == totalItems
				connector := "├── "
				if isLast {
					connector = "└── "
				}
				b.WriteString(prefix + connector + d.Name() + "/\n")
				next := prefix + "│   "
				if isLast {
					next = prefix + "    "
				}
				walk(filepath.Join(dir, d.Name()), next, depth+1)
			}
			if len(files) > 0 {
				idx++
				connector := "├── "
				if idx == totalItems {
					connector = "└── "
				}
				byExt := map[string][]string{}
				var extOrder []string
				for _, f := range files {
					ext := ""
					if dot := strings.LastIndexByte(f.Name(), '.'); dot >= 0 {
						ext = f.Name()[dot+1:]
					}
					if _, seen := byExt[ext]; !seen {
						extOrder = append(extOrder, ext)
					}
					byExt[ext] = append(byExt[ext], filepath.Join(dir, f.Name()))
				}
				var summaries []string
				for _, ext := range extOrder {
					list := byExt[ext]
					if ext == "" {
						summaries = append(summaries, fmt.Sprintf("%d files", len(list)))
					} else {
						total := 0
						for _, f := range list {
							total += lineCount(f)
						}
						summaries = append(summaries, fmt.Sprintf("%d .%s (%dL)", len(list), ext, total))
					}
				}
				b.WriteString(prefix + connector + "[" + strings.Join(summaries, ", ") + "]\n")
			}
		}
	}

	b.WriteString(rootName + "/\n")
	walk(root, "", 1)
	return strings.TrimRight(b.String(), "\n ")
}
