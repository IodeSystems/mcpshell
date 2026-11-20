package toolkit_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// rootsFixture is a workspace, a read-only root and a read-write root, each
// holding a.txt, plus a directory outside every root.
type rootsFixture struct {
	ws, ro, rw, outside string
	roots               *toolkit.FileRoots
	sh                  *runtime.Shell
}

func newRootsFixture(t *testing.T, readOnly bool) *rootsFixture {
	t.Helper()
	base := t.TempDir()
	f := &rootsFixture{
		ws: filepath.Join(base, "ws"), ro: filepath.Join(base, "notes"),
		rw: filepath.Join(base, "scratch"), outside: filepath.Join(base, "secret"),
	}
	for _, d := range []string{f.ws, f.ro, f.rw, f.outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "a.txt"), []byte(filepath.Base(d)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	f.roots, err = toolkit.NewFileRoots(f.ws, readOnly, toolkit.FileRoot{Path: f.ro}, toolkit.FileRoot{Path: f.rw, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	f.sh = runtime.NewShell()
	toolkit.InstallCore(f.sh)
	f.sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	toolkit.InstallFileRoots(f.sh, f.roots)
	return f
}

func (f *rootsFixture) eval(t *testing.T, src string) (string, error) {
	t.Helper()
	v, err := f.sh.Eval(src)
	if err != nil {
		return "", err
	}
	return v.Display(), nil
}

func (f *rootsFixture) ok(t *testing.T, src, want string) {
	t.Helper()
	if got, err := f.eval(t, src); err != nil || got != want {
		t.Errorf("%s = %q, %v; want %q", src, got, err, want)
	}
}

func (f *rootsFixture) fails(t *testing.T, src string, wants ...string) {
	t.Helper()
	_, err := f.eval(t, src)
	if err == nil {
		t.Errorf("%s: no error", src)
		return
	}
	for _, w := range wants {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("%s: error lacks %q:\n%v", src, w, err)
		}
	}
}

func TestFileRootsReadWrite(t *testing.T) {
	f := newRootsFixture(t, false)
	q := func(p string) string { return `"` + p + `"` }

	// Relative paths are under the workspace, as before.
	f.ok(t, `Files.readLines("a.txt")`, `["ws"]`)
	f.ok(t, `Files.write("b.txt", "x"); Files.readLines(`+q(filepath.Join(f.ws, "b.txt"))+`)`, `["x"]`)
	// Absolute paths in any root read; ls there lists absolute paths.
	f.ok(t, `Files.readLines(`+q(filepath.Join(f.ro, "a.txt"))+`)`, `["notes"]`)
	f.ok(t, `Files.ls(`+q(f.ro)+`) |> map(x => x.path)`, `["`+filepath.Join(f.ro, "a.txt")+`"]`)
	// A write root takes writes; the read-only root does not, and says so.
	f.ok(t, `Files.write(`+q(filepath.Join(f.rw, "n.txt"))+`, "y"); Files.readLines(`+q(filepath.Join(f.rw, "n.txt"))+`)`, `["y"]`)
	f.fails(t, `Files.write(`+q(filepath.Join(f.ro, "n.txt"))+`, "y")`,
		"is in a read-only root", "Read-only root: "+f.ro+" (read-only)",
		"Writable roots: "+f.ws+" (read-write, workspace), "+f.rw+" (read-write)")
	f.fails(t, `Files.mv(`+q(filepath.Join(f.ro, "a.txt"))+`, "moved.txt")`, "is in a read-only root")
	if _, err := os.Stat(filepath.Join(f.ro, "n.txt")); err == nil {
		t.Error("write reached the read-only root")
	}
	// Outside every root: denied, with the roots listed.
	allowed := "Allowed roots: " + f.ws + " (read-write, workspace), " + f.ro + " (read-only), " + f.rw + " (read-write)"
	f.fails(t, `Files.read(`+q(filepath.Join(f.outside, "a.txt"))+`)`,
		"Access denied: '"+filepath.Join(f.outside, "a.txt")+"' resolves outside the allowed roots",
		"Resolved: "+filepath.Join(f.outside, "a.txt"), allowed, "Paths outside these need access granted by the host.")
	// ".." cannot escape: the check is on the cleaned path.
	f.fails(t, `Files.read("../secret/a.txt")`, "Access denied", "Resolved: "+filepath.Join(f.outside, "a.txt"))
	f.fails(t, `Files.read(`+q(f.ro+"/../secret/a.txt")+`)`, "Access denied")
	f.fails(t, `Files.write("../secret/b.txt", "x")`, "Access denied", "outside the allowed roots")
	// A relative path that lands in another root is in that root.
	f.ok(t, `Files.readLines("../notes/a.txt")`, `["notes"]`)
	// Not found keeps its Resolved to: line.
	f.fails(t, `Files.read(`+q(filepath.Join(f.ro, "nope.txt"))+`)`, "file not found", "Resolved to: "+filepath.Join(f.ro, "nope.txt"))
}

func TestFileRootsHome(t *testing.T) {
	f := newRootsFixture(t, false)
	t.Setenv("HOME", filepath.Dir(f.ro))
	f.ok(t, `Files.readLines("~/notes/a.txt")`, `["notes"]`)
	f.fails(t, `Files.write("~/notes/b.txt", "x")`, "is in a read-only root")
	f.fails(t, `Files.read("~/secret/a.txt")`, "Access denied: '~/secret/a.txt' resolves outside the allowed roots",
		"Resolved: "+filepath.Join(f.outside, "a.txt"))
}

// Client roots replace as a set, are read-only unless granted write, and
// apply to the next operation.
func TestFileRootsClient(t *testing.T) {
	f := newRootsFixture(t, false)
	secret := filepath.Join(f.outside, "a.txt")
	q := `"` + secret + `"`
	f.fails(t, `Files.read(`+q+`)`, "Access denied")

	f.roots.SetClient([]toolkit.FileRoot{{Path: f.outside}})
	f.ok(t, `Files.readLines(`+q+`)`, `["secret"]`)
	f.fails(t, `Files.write(`+q+`, "x")`, "Read-only root: "+f.outside+" (read-only)")

	f.roots.SetClient([]toolkit.FileRoot{{Path: f.outside, Write: true}})
	f.ok(t, `Files.write(`+q+`, "w"); Files.readLines(`+q+`)`, `["w"]`)

	f.roots.SetClient(nil)
	f.fails(t, `Files.read(`+q+`)`, "Access denied")
	// Operator roots are not the client's to remove.
	f.ok(t, `Files.readLines("`+filepath.Join(f.ro, "a.txt")+`")`, `["notes"]`)
	f.roots.SetClient([]toolkit.FileRoot{{Path: "relative/dir"}}) // not absolute: dropped
	if n := len(f.roots.All()); n != 3 {
		t.Errorf("roots after a relative client root = %d, want 3", n)
	}
}

// --files-read-only makes every root read-only, a client write grant too.
func TestFileRootsReadOnly(t *testing.T) {
	f := newRootsFixture(t, true)
	f.roots.SetClient([]toolkit.FileRoot{{Path: f.outside, Write: true}})
	for _, r := range f.roots.All() {
		if r.Write {
			t.Errorf("root %s is writable under read-only", r.Path)
		}
	}
	f.fails(t, `Files.write("x.txt", "x")`) // not registered
	f.ok(t, `Files.readLines("`+filepath.Join(f.outside, "a.txt")+`")`, `["secret"]`)
}

// Db.loadCsv and load(path) use the same roots as Files.read.
func TestFileRootsDbAndLoad(t *testing.T) {
	f := newRootsFixture(t, false)
	if err := os.WriteFile(filepath.Join(f.outside, "t.csv"), []byte("a,b\n1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.ro, "lib.mcpshell"), []byte("export let v = 7"), 0o644); err != nil {
		t.Fatal(err)
	}
	csv := `"` + filepath.Join(f.outside, "t.csv") + `"`
	f.fails(t, `Db.loadCsv("t", `+csv+`)`, "Access denied", "Allowed roots: ")
	f.roots.SetClient([]toolkit.FileRoot{{Path: f.outside}})
	f.ok(t, `Db.loadCsv("t", `+csv+`).rows`, "1")
	f.ok(t, `load("`+filepath.Join(f.ro, "lib.mcpshell")+`"); v`, "7")
	f.fails(t, `load("../x.mcpshell")`, "Access denied")
}

// A client update while evals read files: run with -race.
func TestFileRootsClientRace(t *testing.T) {
	f := newRootsFixture(t, false)
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
			}
			f.roots.SetClient([]toolkit.FileRoot{{Path: f.outside, Write: i%2 == 0}})
		}
	})
	for range 50 {
		_, _ = f.sh.Eval(`all([() => Files.exists("` + filepath.Join(f.outside, "a.txt") + `"), () => Files.exists("a.txt")])`)
	}
	close(done)
	wg.Wait()
}
