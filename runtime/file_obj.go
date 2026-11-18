package runtime

import (
	"path"
	"strings"
	"sync"
)

// File is the value Files.ls() and Files.glob() return: an object with path, name, base,
// dir, ext, type and size that acts as its path string where a string is
// expected (templates, +, String(), == with a string, JSON, object keys,
// string methods, and the path argument of every file command). Go code gets
// the path with one type check (AsPath), without calling toString:
// docs/design.md "Objects".

// fileFields are the own properties of a File, in display and key order.
// They are read-only: the Go-side path (ObjectVal.file) must not drift from
// the path field.
var fileFields = []string{"path", "name", "base", "dir", "ext", "type", "size"}

func isFileField(k string) bool {
	for _, f := range fileFields {
		if f == k {
			return true
		}
	}
	return false
}

// AsPath returns the path of a File value. A plain string is not a File: the
// callers check *StringVal first, as they did before.
func AsPath(v Value) (string, bool) {
	if o, ok := v.(*ObjectVal); ok && o.file != nil {
		return o.file.V, true
	}
	return "", false
}

// asPathStr is AsPath giving the shared *StringVal, so a conversion does not
// allocate a new string value.
func asPathStr(v Value) (*StringVal, bool) {
	if o, ok := v.(*ObjectVal); ok && o.file != nil {
		return o.file, true
	}
	return nil, false
}

// fileClass makes the File class: a native class (like Error) whose
// prototype has toString and valueOf giving the path. new File(path) and
// File(path) make one with type and size null; the file commands check the
// path when they use it.
func fileClass() *FuncVal {
	cls := &FuncVal{Name: "File", kind: kindClass, Params: []string{"path"}}
	cls.Body = &NativeBody{Fn: func([]Value) Value { panic(Runtime("File is constructed with new File(path)")) }}
	cls.Class = &classInfo{native: initFile}
	proto := NewObject()
	proto.defineHidden("constructor", cls)
	pathOf := &NativeBody{
		Fn: func([]Value) Value { return Str("") },
		This: func(this Value, _ []Value) Value {
			if s, ok := asPathStr(this); ok {
				return s
			}
			return Str("[object File]")
		},
	}
	proto.defineHidden("toString", &FuncVal{Name: "toString", Body: pathOf})
	proto.defineHidden("valueOf", &FuncVal{Name: "valueOf", Body: pathOf})
	cls.props().defineHidden("prototype", proto)
	return cls
}

// initFile is the File constructor: new File(path), path a string or a File.
func initFile(_ *Visitor, this *ObjectVal, args []Value) {
	var p string
	if len(args) > 0 {
		switch x := args[0].(type) {
		case *StringVal:
			p = x.V
		default:
			if s, ok := AsPath(x); ok {
				p = s
			} else {
				panic(TypeMismatch("new File(path)", "string", x, `new File("data/x.json")`))
			}
		}
	} else {
		panic(Runtime(`new File(path) needs a path: new File("data/x.json")`))
	}
	setFileFields(this, p, Null, Null)
}

// setFileFields fills a File's fields. path, name and the Go-side path share
// one string value.
func setFileFields(o *ObjectVal, p string, typ, size Value) {
	ps := Str(p)
	o.file = ps
	base := path.Base(p)
	if p == "" {
		base = ""
	}
	vals := []Value{ps, ps, Str(base), Str(path.Dir(p)), Str(nodeExt(base)), typ, size}
	o.keys = append(o.keys, fileFields...)
	for i, k := range fileFields {
		o.m[k] = vals[i]
	}
}

// nodeExt is Node's path.extname of a base name: from the last dot, or ""
// when the only dot is the first character (".bashrc").
func nodeExt(base string) string {
	i := strings.LastIndexByte(base, '.')
	if i <= 0 {
		return ""
	}
	return base[i:]
}

// NewFile makes a File value for a path relative to the files root
// (slash-separated). dir selects type "dir"; size is in bytes.
func (t *Shell) NewFile(p string, dir bool, size int64) *ObjectVal {
	o := NewObjectWithProto(t.fileProto)
	typ := "file"
	if dir {
		typ = "dir"
	}
	setFileFields(o, p, Str(typ), Num(float64(size)))
	return o
}

// fileInspect is how a File prints: File("data/x.json"), and File("src",
// dir) for a directory, so an ls() listing still tells the two apart.
func fileInspect(o *ObjectVal) string {
	if t, ok := o.m["type"].(*StringVal); ok && t.V == "dir" {
		return "File(" + o.file.Inspect() + ", dir)"
	}
	return "File(" + o.file.Inspect() + ")"
}

// readOnlyFileField is the error for writing or deleting a File's field.
func readOnlyFileField(k string) *ShellError {
	return Runtime("File." + k + " is read-only: a File keeps the path it was made with; make another with new File(path)")
}

// methodNamespaces holds the namespaces ObjectVal.methodNs indexes; index 0
// is none. It grows by one per toolkit that marks its objects.
var (
	methodNsMu       sync.RWMutex
	methodNamespaces = []string{""}
)

// SetMethodNamespace makes ns's commands methods of o: o.cmd(args) calls
// ns.cmd(o, args) when o has no own member cmd. A toolkit marks the objects
// it makes (graph nodes take Graph.*), so a method call does not need a bare
// global command.
func (o *ObjectVal) SetMethodNamespace(ns string) {
	methodNsMu.RLock()
	for i, n := range methodNamespaces {
		if n == ns {
			methodNsMu.RUnlock()
			o.methodNs = uint8(i)
			return
		}
	}
	methodNsMu.RUnlock()
	methodNsMu.Lock()
	defer methodNsMu.Unlock()
	for i, n := range methodNamespaces {
		if n == ns {
			o.methodNs = uint8(i)
			return
		}
	}
	if len(methodNamespaces) > 255 {
		panic("SetMethodNamespace: too many namespaces")
	}
	methodNamespaces = append(methodNamespaces, ns)
	o.methodNs = uint8(len(methodNamespaces) - 1)
}

func (o *ObjectVal) methodNamespace() string {
	if o.methodNs == 0 {
		return ""
	}
	methodNsMu.RLock()
	defer methodNsMu.RUnlock()
	return methodNamespaces[o.methodNs]
}

// FilesNamespace is the namespace whose commands are a File's methods.
const FilesNamespace = "Files"
