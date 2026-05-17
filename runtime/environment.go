package runtime

import (
	"maps"
	"sync"
)

// Environment is a lexical scope. A single mutex (the "GIL") is shared across
// the whole scope graph so compound operations — check-then-write, parent-chain
// walks, read-modify-write — stay atomic while parallel branches run.
type Environment struct {
	parent   *Environment
	gil      *sync.Mutex
	bindings map[string]Value
}

// NewEnvironment creates a scope. A child inherits its parent's GIL; a root
// (parent == nil) gets a fresh one.
func NewEnvironment(parent *Environment) *Environment {
	gil := &sync.Mutex{}
	if parent != nil {
		gil = parent.gil
	}
	return &Environment{parent: parent, gil: gil, bindings: make(map[string]Value)}
}

// Get resolves a name, walking the parent chain. Returns nil if unbound.
func (e *Environment) Get(name string) Value {
	e.gil.Lock()
	defer e.gil.Unlock()
	return e.getLocked(name)
}

func (e *Environment) getLocked(name string) Value {
	if v, ok := e.bindings[name]; ok {
		return v
	}
	if e.parent != nil {
		return e.parent.getLocked(name)
	}
	return nil
}

// Define binds a new name in this scope; panics if already declared here.
func (e *Environment) Define(name string, value Value) {
	e.gil.Lock()
	defer e.gil.Unlock()
	if _, ok := e.bindings[name]; ok {
		panic(Runtime("'" + name + "' is already declared in this scope"))
	}
	e.bindings[name] = value
}

// Set assigns to an existing binding, searching outward; panics if undefined.
func (e *Environment) Set(name string, value Value) {
	e.gil.Lock()
	defer e.gil.Unlock()
	e.setLocked(name, value)
}

func (e *Environment) setLocked(name string, value Value) {
	if _, ok := e.bindings[name]; ok {
		e.bindings[name] = value
		return
	}
	if e.parent != nil {
		e.parent.setLocked(name, value)
		return
	}
	panic(Runtime("'" + name + "' is not defined"))
}

// Mutate performs an atomic read-modify-write at the given path. A single-element
// path targets a binding; a longer path walks into objects and rewrites the leaf
// in place (JS reference semantics). Returns the new value.
func (e *Environment) Mutate(path []string, transform func(Value) Value) Value {
	e.gil.Lock()
	defer e.gil.Unlock()

	if len(path) == 0 {
		panic(Runtime("mutate: empty path"))
	}
	if len(path) == 1 {
		current := e.getLocked(path[0])
		if current == nil {
			current = Null
		}
		newVal := transform(current)
		e.setLocked(path[0], newVal)
		return newVal
	}

	root := e.getLocked(path[0])
	if root == nil {
		panic(Runtime("'" + path[0] + "' is not defined"))
	}
	current := root
	for i := 1; i < len(path)-1; i++ {
		obj, ok := current.(*ObjectVal)
		if !ok {
			panic(Runtime("Cannot access '" + path[i] + "' on " + current.TypeName()))
		}
		v, _ := obj.Get(path[i])
		if v == nil {
			v = Null
		}
		current = v
	}
	leafKey := path[len(path)-1]
	obj, ok := current.(*ObjectVal)
	if !ok {
		panic(Runtime("Cannot access '" + leafKey + "' on " + current.TypeName()))
	}
	leaf, _ := obj.Get(leafKey)
	if leaf == nil {
		leaf = Null
	}
	newLeaf := transform(leaf)
	obj.Set(leafKey, newLeaf)
	return newLeaf
}

// Child creates a nested scope sharing this scope's GIL.
func (e *Environment) Child() *Environment { return NewEnvironment(e) }

// OwnBindings returns a copy of the bindings declared directly in this scope.
func (e *Environment) OwnBindings() map[string]Value {
	e.gil.Lock()
	defer e.gil.Unlock()
	return copyBindings(e.bindings)
}

// AllBindings returns every visible binding, child shadowing parent.
func (e *Environment) AllBindings() map[string]Value {
	e.gil.Lock()
	defer e.gil.Unlock()
	return e.allBindingsLocked()
}

func (e *Environment) allBindingsLocked() map[string]Value {
	var all map[string]Value
	if e.parent != nil {
		all = e.parent.allBindingsLocked()
	} else {
		all = make(map[string]Value)
	}
	maps.Copy(all, e.bindings)
	return all
}

// Snapshot returns a copy of this scope's own bindings.
func (e *Environment) Snapshot() map[string]Value { return e.OwnBindings() }

// Restore replaces this scope's bindings with the given state.
func (e *Environment) Restore(state map[string]Value) {
	e.gil.Lock()
	defer e.gil.Unlock()
	e.bindings = copyBindings(state)
}

// Remove deletes the named bindings from this scope.
func (e *Environment) Remove(names map[string]struct{}) {
	e.gil.Lock()
	defer e.gil.Unlock()
	for name := range names {
		delete(e.bindings, name)
	}
}

// DefineOrSet binds a name in this scope, overwriting any existing binding.
func (e *Environment) DefineOrSet(name string, value Value) {
	e.gil.Lock()
	defer e.gil.Unlock()
	e.bindings[name] = value
}

func copyBindings(src map[string]Value) map[string]Value {
	return maps.Clone(src)
}
