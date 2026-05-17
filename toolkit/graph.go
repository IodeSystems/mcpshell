package toolkit

import (
	"fmt"
	"sync"

	. "github.com/iodesystems/mcpshell/runtime"
)

// --- in-memory graph store ---------------------------------------------------

type graphNode struct {
	id, typ string
	props   *ObjectVal
}

type graphEdge struct {
	id, from, to string
	typ          string // "" = untyped
	props        *ObjectVal
}

// graphStore is an in-memory directed property graph. It is not internally
// synchronized; graphToolkit serializes access with its own mutex.
type graphStore struct {
	nodeMap   map[string]*graphNode
	edgeMap   map[string]*graphEdge
	nodeOrder []string
	edgeOrder []string
	out       map[string][]string // nodeID → outgoing edge IDs
	in        map[string][]string // nodeID → incoming edge IDs
	nodeSeq   int
	edgeSeq   int
	rootID    string
}

func newGraphStore() *graphStore {
	gs := &graphStore{
		nodeMap: map[string]*graphNode{},
		edgeMap: map[string]*graphEdge{},
		out:     map[string][]string{},
		in:      map[string][]string{},
	}
	root := gs.addNode("root", nil)
	gs.rootID = root.id
	return gs
}

func (gs *graphStore) addNode(typ string, props *ObjectVal) *graphNode {
	gs.nodeSeq++
	id := fmt.Sprintf("n%d", gs.nodeSeq)
	n := &graphNode{id: id, typ: typ, props: copyProps(props)}
	gs.nodeMap[id] = n
	gs.nodeOrder = append(gs.nodeOrder, id)
	gs.out[id] = nil
	gs.in[id] = nil
	return n
}

func (gs *graphStore) addEdge(from, to, typ string, props *ObjectVal) *graphEdge {
	gs.edgeSeq++
	id := fmt.Sprintf("e%d", gs.edgeSeq)
	e := &graphEdge{id: id, from: from, to: to, typ: typ, props: copyProps(props)}
	gs.edgeMap[id] = e
	gs.edgeOrder = append(gs.edgeOrder, id)
	gs.out[from] = append(gs.out[from], id)
	gs.in[to] = append(gs.in[to], id)
	return e
}

func (gs *graphStore) getNode(id string) *graphNode { return gs.nodeMap[id] }
func (gs *graphStore) getEdge(id string) *graphEdge { return gs.edgeMap[id] }

func (gs *graphStore) removeEdge(id string) {
	e := gs.edgeMap[id]
	if e == nil {
		return
	}
	delete(gs.edgeMap, id)
	gs.edgeOrder = removeString(gs.edgeOrder, id)
	gs.out[e.from] = removeString(gs.out[e.from], id)
	gs.in[e.to] = removeString(gs.in[e.to], id)
}

func (gs *graphStore) removeNode(id string) {
	if gs.nodeMap[id] == nil {
		return
	}
	// Remove every edge touching this node (collect first — removeEdge mutates).
	touching := map[string]bool{}
	for _, eid := range gs.out[id] {
		touching[eid] = true
	}
	for _, eid := range gs.in[id] {
		touching[eid] = true
	}
	for eid := range touching {
		gs.removeEdge(eid)
	}
	delete(gs.nodeMap, id)
	delete(gs.out, id)
	delete(gs.in, id)
	gs.nodeOrder = removeString(gs.nodeOrder, id)
}

func (gs *graphStore) outgoing(nodeID string, edgeType *string) []*graphEdge {
	return gs.adjacent(gs.out[nodeID], edgeType)
}

func (gs *graphStore) incoming(nodeID string, edgeType *string) []*graphEdge {
	return gs.adjacent(gs.in[nodeID], edgeType)
}

func (gs *graphStore) adjacent(edgeIDs []string, edgeType *string) []*graphEdge {
	var out []*graphEdge
	for _, eid := range edgeIDs {
		e := gs.edgeMap[eid]
		if e == nil {
			continue
		}
		if edgeType == nil || e.typ == *edgeType {
			out = append(out, e)
		}
	}
	return out
}

func (gs *graphStore) nodes(typ *string) []*graphNode {
	var out []*graphNode
	for _, id := range gs.nodeOrder {
		n := gs.nodeMap[id]
		if typ == nil || n.typ == *typ {
			out = append(out, n)
		}
	}
	return out
}

func (gs *graphStore) updateNode(id string, props *ObjectVal) {
	n := gs.nodeMap[id]
	mergeProps(n.props, props)
}

func (gs *graphStore) updateEdge(id string, props *ObjectVal) {
	e := gs.edgeMap[id]
	mergeProps(e.props, props)
}

func copyProps(src *ObjectVal) *ObjectVal {
	dst := NewObject()
	mergeProps(dst, src)
	return dst
}

func mergeProps(dst, src *ObjectVal) {
	if dst == nil || src == nil {
		return
	}
	for _, k := range src.Keys() {
		v, _ := src.Get(k)
		dst.Set(k, v)
	}
}

func removeString(s []string, target string) []string {
	for i, v := range s {
		if v == target {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

// --- toolkit -----------------------------------------------------------------

type graphToolkit struct {
	mu             sync.Mutex
	store          *graphStore
	traversalCount int
	maxTraversals  int
}

func (gt *graphToolkit) countTraversal(n int) {
	gt.traversalCount += n
	if gt.traversalCount > gt.maxTraversals {
		panic(Runtime(fmt.Sprintf(
			"Graph traversal limit exceeded (%d steps)\n\n"+
				"  Use a more specific edge type filter, or increase the limit.", gt.maxTraversals)))
	}
}

// InstallGraph registers the graph toolkit — an in-memory directed property
// graph with node/edge CRUD and pipe-compatible traversal steps.
func InstallGraph(sh *Shell) *Shell {
	gt := &graphToolkit{store: newGraphStore(), maxTraversals: 10_000}

	sh.RegisterGuide("graph", graphGuide)

	reg := func(name, sig, desc string, examples []string, fn NativeFn) {
		sh.Register(&CommandDef{Name: name, Signature: sig, Description: desc, Examples: examples,
			Fn: func(args []Value) Value {
				gt.mu.Lock()
				defer gt.mu.Unlock()
				return fn(args)
			}})
	}

	// --- query start points ---

	reg("root", "", "returns the graph root node",
		[]string{"root()", `root() |> out("person")`},
		func(_ []Value) Value {
			gt.traversalCount = 0
			return nodeToObj(gt.store.getNode(gt.store.rootID))
		})

	reg("node", "id: string", "gets a node by id",
		[]string{`node("n1")`},
		func(args []Value) Value {
			gt.traversalCount = 0
			id := extractGraphID(arg(args, 0), "node")
			n := gt.store.getNode(id)
			if n == nil {
				panic(Runtime("node: not found '" + id + "'"))
			}
			return nodeToObj(n)
		})

	reg("nodes", "type?: string", "gets all nodes, optionally filtered by type",
		[]string{`nodes("person")`, `nodes() |> filter(n => n.age > 25)`},
		func(args []Value) Value {
			gt.traversalCount = 0
			out := make([]Value, 0)
			for _, n := range gt.store.nodes(optStringPtr(args, 0)) {
				out = append(out, nodeToObj(n))
			}
			return &ArrayVal{Elements: out}
		})

	// --- CRUD ---

	reg("addNode", "parent: node|string, type: string, props?: object",
		"adds a node connected from parent. Edge type defaults to node type",
		[]string{`addNode(root(), "person", {name: "Alice"})`},
		func(args []Value) Value {
			parentID := extractGraphID(arg(args, 0), "addNode parent")
			typ, ok := arg(args, 1).(*StringVal)
			if !ok {
				panic(WrongArguments("addNode", "parent, type: string, props?: object", args,
					`addNode(root(), "person", {name: "Alice"})`))
			}
			props := extractGraphProps(arg(args, 2))
			if gt.store.getNode(parentID) == nil {
				panic(Runtime("addNode: parent node '" + parentID + "' not found"))
			}
			n := gt.store.addNode(typ.V, props)
			gt.store.addEdge(parentID, n.id, typ.V, nil)
			return nodeToObj(n)
		})

	reg("link", "from: node|string, to: node|string, type?: string, props?: object",
		"creates an edge between two nodes",
		[]string{`link(alice, bob, "knows")`, `link(alice, project, "owns", {since: 2024})`},
		func(args []Value) Value {
			fromID := extractGraphID(arg(args, 0), "link from")
			toID := extractGraphID(arg(args, 1), "link to")
			typ := optString(args, 2, "")
			props := extractGraphProps(arg(args, 3))
			if gt.store.getNode(fromID) == nil {
				panic(Runtime("link: source node '" + fromID + "' not found"))
			}
			if gt.store.getNode(toID) == nil {
				panic(Runtime("link: target node '" + toID + "' not found"))
			}
			return edgeToObj(gt.store.addEdge(fromID, toID, typ, props))
		})

	reg("unlink", "edge: edge|string", "removes an edge by id",
		[]string{`unlink(edgeId)`, `unlink("e1")`},
		func(args []Value) Value {
			id := extractGraphID(arg(args, 0), "unlink")
			if gt.store.getEdge(id) == nil {
				panic(Runtime("unlink: edge '" + id + "' not found"))
			}
			gt.store.removeEdge(id)
			return Null
		})

	reg("removeNode", "node: node|string", "removes a node and all its edges",
		[]string{`removeNode(nodeId)`, `removeNode("n3")`},
		func(args []Value) Value {
			id := extractGraphID(arg(args, 0), "removeNode")
			if id == gt.store.rootID {
				panic(Runtime("removeNode: cannot remove root node"))
			}
			if gt.store.getNode(id) == nil {
				panic(Runtime("removeNode: node '" + id + "' not found"))
			}
			gt.store.removeNode(id)
			return Null
		})

	reg("setProps", "target: node|edge|string, props: object",
		"merges properties into a node or edge",
		[]string{`setProps(alice, {age: 31})`, `setProps("e1", {weight: 0.5})`},
		func(args []Value) Value {
			id := extractGraphID(arg(args, 0), "setProps target")
			props, ok := arg(args, 1).(*ObjectVal)
			if !ok {
				panic(WrongArguments("setProps", "target, props: object", args,
					`setProps(node, {key: value})`))
			}
			if gt.store.getNode(id) != nil {
				gt.store.updateNode(id, props)
				return nodeToObj(gt.store.getNode(id))
			}
			if gt.store.getEdge(id) != nil {
				gt.store.updateEdge(id, props)
				return edgeToObj(gt.store.getEdge(id))
			}
			panic(Runtime("setProps: no node or edge with id '" + id + "'"))
		})

	// --- traversal steps (pipe-compatible) ---

	traversal := func(name, sig, desc string, examples []string, step func(nodeID string, edgeType *string) []Value) {
		reg(name, sig, desc, examples, func(args []Value) Value {
			edgeType := optStringPtr(args, 1)
			out := make([]Value, 0)
			for _, obj := range toNodeList(arg(args, 0), name) {
				out = append(out, step(graphObjID(obj), edgeType)...)
			}
			return &ArrayVal{Elements: out}
		})
	}

	traversal("out", "input: node|array, type?: string", "follows outgoing edges, returns target nodes",
		[]string{`root() |> out("person")`, `node |> out()`},
		func(nodeID string, edgeType *string) []Value {
			edges := gt.store.outgoing(nodeID, edgeType)
			gt.countTraversal(len(edges))
			var out []Value
			for _, e := range edges {
				if t := gt.store.getNode(e.to); t != nil {
					out = append(out, nodeToObj(t))
				}
			}
			return out
		})

	traversal("inbound", "input: node|array, type?: string", "follows incoming edges, returns source nodes",
		[]string{`node |> inbound("knows")`},
		func(nodeID string, edgeType *string) []Value {
			edges := gt.store.incoming(nodeID, edgeType)
			gt.countTraversal(len(edges))
			var out []Value
			for _, e := range edges {
				if s := gt.store.getNode(e.from); s != nil {
					out = append(out, nodeToObj(s))
				}
			}
			return out
		})

	reg("both", "input: node|array, type?: string", "follows edges in both directions",
		[]string{`node |> both("knows")`},
		func(args []Value) Value {
			edgeType := optStringPtr(args, 1)
			seen := map[string]bool{}
			out := make([]Value, 0)
			for _, obj := range toNodeList(arg(args, 0), "both") {
				nodeID := graphObjID(obj)
				outE := gt.store.outgoing(nodeID, edgeType)
				inE := gt.store.incoming(nodeID, edgeType)
				gt.countTraversal(len(outE) + len(inE))
				for _, e := range outE {
					if !seen[e.to] {
						seen[e.to] = true
						if t := gt.store.getNode(e.to); t != nil {
							out = append(out, nodeToObj(t))
						}
					}
				}
				for _, e := range inE {
					if !seen[e.from] {
						seen[e.from] = true
						if s := gt.store.getNode(e.from); s != nil {
							out = append(out, nodeToObj(s))
						}
					}
				}
			}
			return &ArrayVal{Elements: out}
		})

	traversal("outE", "input: node|array, type?: string", "gets outgoing edges as objects",
		[]string{`node |> outE("knows")`, `root() |> outE()`},
		func(nodeID string, edgeType *string) []Value {
			edges := gt.store.outgoing(nodeID, edgeType)
			gt.countTraversal(len(edges))
			out := make([]Value, len(edges))
			for i, e := range edges {
				out[i] = edgeToObj(e)
			}
			return out
		})

	traversal("inE", "input: node|array, type?: string", "gets incoming edges as objects",
		[]string{`node |> inE("knows")`},
		func(nodeID string, edgeType *string) []Value {
			edges := gt.store.incoming(nodeID, edgeType)
			gt.countTraversal(len(edges))
			out := make([]Value, len(edges))
			for i, e := range edges {
				out[i] = edgeToObj(e)
			}
			return out
		})

	return sh
}

// --- conversions -------------------------------------------------------------

func nodeToObj(n *graphNode) *ObjectVal {
	o := NewObject()
	o.Set("id", Str(n.id))
	o.Set("type", Str(n.typ))
	mergeProps(o, n.props)
	return o
}

func edgeToObj(e *graphEdge) *ObjectVal {
	o := NewObject()
	o.Set("id", Str(e.id))
	o.Set("from", Str(e.from))
	o.Set("to", Str(e.to))
	if e.typ != "" {
		o.Set("type", Str(e.typ))
	} else {
		o.Set("type", Null)
	}
	mergeProps(o, e.props)
	return o
}

func extractGraphID(v Value, ctx string) string {
	switch x := v.(type) {
	case *StringVal:
		return x.V
	case *ObjectVal:
		if id, ok := x.Get("id"); ok {
			if s, ok := id.(*StringVal); ok {
				return s.V
			}
		}
		panic(Runtime(ctx + ": object has no 'id' field"))
	default:
		panic(Runtime(ctx + ": expected node/edge object or id string, got " + v.TypeName()))
	}
}

func extractGraphProps(v Value) *ObjectVal {
	switch x := v.(type) {
	case *ObjectVal:
		return x
	case *NullVal, nil:
		return nil
	default:
		panic(Runtime("Expected object for properties, got " + v.TypeName()))
	}
}

func graphObjID(o *ObjectVal) string {
	if id, ok := o.Get("id"); ok {
		if s, ok := id.(*StringVal); ok {
			return s.V
		}
	}
	panic(Runtime("Graph traversal: object missing 'id' field"))
}

func toNodeList(input Value, step string) []*ObjectVal {
	switch x := input.(type) {
	case *ObjectVal:
		return []*ObjectVal{x}
	case *ArrayVal:
		out := make([]*ObjectVal, len(x.Elements))
		for i, el := range x.Elements {
			o, ok := el.(*ObjectVal)
			if !ok {
				panic(TypeMismatch(step, "node objects", el, ""))
			}
			out[i] = o
		}
		return out
	default:
		panic(TypeMismatch(step, "node or array of nodes", input, ""))
	}
}

// optStringPtr returns args[i] as a *string, or nil when absent/non-string.
func optStringPtr(args []Value, i int) *string {
	if i < len(args) {
		if s, ok := args[i].(*StringVal); ok {
			return &s.V
		}
	}
	return nil
}

const graphGuide = `Graph Toolkit — nodes, edges, and traversal

All nodes connect to root. Traversal steps (out, inbound, both) work as pipes:
they accept a single node or an array and return an array, so filter(), map(),
reduce(), sort() all compose naturally.

TYPICAL: Build a graph
  let alice = addNode(root(), "person", {name: "Alice", age: 30})
  let acme  = addNode(root(), "company", {name: "Acme"})
  link(alice, acme, "worksAt")

TYPICAL: Traverse and query
  root() |> out("person")                            // all people
  root() |> out("person") |> filter(n => n.age > 28)  // filtered
  root() |> out("person") |> out("worksAt") |> map(c => c.name)

  // Reverse: who works at Acme?
  nodes("company") |> filter(c => c.name == "Acme") |> inbound("worksAt") |> map(n => n.name)

TYPICAL: Edge inspection
  node(alice.id) |> outE("knows")
  node(alice.id) |> outE() |> filter(e => e.type == "worksAt")

TYPICAL: CRUD
  setProps(alice, {age: 31})   // update node properties
  removeNode(bob)              // removes node + all edges
  unlink("e3")                 // remove single edge

ADVANCED: Degree count
  nodes("person") |> map(p => {name: p.name, connections: (node(p.id) |> outE() |> len())})`
