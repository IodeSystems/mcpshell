package toolkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// FileRoot is one directory the file toolkit may reach.
type FileRoot struct {
	Path  string // absolute and clean
	Write bool   // writes are allowed under Path
	Label string // shown after the mode in errors ("workspace"); may be empty
}

// describe is the root as an error lists it: "/a (read-write, workspace)".
func (r FileRoot) describe() string {
	mode := "read-only"
	if r.Write {
		mode = "read-write"
	}
	if r.Label != "" {
		mode += ", " + r.Label
	}
	return r.Path + " (" + mode + ")"
}

// contains reports whether the clean absolute path p is r.Path or under it.
func (r FileRoot) contains(p string) bool {
	rel, err := filepath.Rel(r.Path, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// FileRoots is the set of directories the file toolkit reaches: the
// workspace (relative paths resolve against it), the operator's extra roots,
// and the roots the MCP client grants. The client set changes at runtime
// (SetClient); every file operation reads the set current at that moment.
type FileRoots struct {
	workspace string
	readOnly  bool
	fixed     []FileRoot // workspace first, then the operator's roots
	client    atomic.Pointer[[]FileRoot]
}

// NewFileRoots builds the root set. workspace and every extra root must be
// existing directories. With readOnly every root is read-only, the client's
// included.
func NewFileRoots(workspace string, readOnly bool, extra ...FileRoot) (*FileRoots, error) {
	r := &FileRoots{readOnly: readOnly}
	ws, err := rootDir(workspace)
	if err != nil {
		return nil, err
	}
	r.workspace = ws
	r.fixed = append(r.fixed, FileRoot{Path: ws, Write: !readOnly, Label: "workspace"})
	for _, e := range extra {
		p, err := rootDir(e.Path)
		if err != nil {
			return nil, err
		}
		r.fixed = append(r.fixed, FileRoot{Path: p, Write: e.Write && !readOnly, Label: e.Label})
	}
	return r, nil
}

// rootDir is the absolute clean form of an existing directory.
func rootDir(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", errors.New("file root " + p + ": " + err.Error())
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return "", errors.New("file root does not exist or is not a directory: " + abs)
	}
	return abs, nil
}

// Workspace is the root relative paths resolve against.
func (r *FileRoots) Workspace() string { return r.workspace }

// ReadOnly reports whether every root is read-only (--files-read-only).
func (r *FileRoots) ReadOnly() bool { return r.readOnly }

// SetClient replaces the client's roots as one set. Paths must be absolute;
// others are dropped. Under ReadOnly a client root never allows writes.
func (r *FileRoots) SetClient(roots []FileRoot) {
	set := make([]FileRoot, 0, len(roots))
	for _, c := range roots {
		if !filepath.IsAbs(c.Path) {
			continue
		}
		set = append(set, FileRoot{Path: filepath.Clean(c.Path), Write: c.Write && !r.readOnly, Label: c.Label})
	}
	r.client.Store(&set)
}

// All is the current set: workspace, operator roots, then client roots.
func (r *FileRoots) All() []FileRoot {
	out := append([]FileRoot(nil), r.fixed...)
	if c := r.client.Load(); c != nil {
		out = append(out, *c...)
	}
	return out
}

// describeRoots lists roots for an error: "/a (read-write, workspace), /b (read-only)".
func describeRoots(roots []FileRoot) string {
	if len(roots) == 0 {
		return "none"
	}
	parts := make([]string, len(roots))
	for i, r := range roots {
		parts[i] = r.describe()
	}
	return strings.Join(parts, ", ")
}
