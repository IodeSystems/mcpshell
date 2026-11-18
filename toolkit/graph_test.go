package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func graphShell() *runtime.Shell {
	return toolkit.InstallGraph(toolkit.InstallCore(runtime.NewShell()))
}

func TestGraphToolkit(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"root type", `Graph.root().type`, "root"},
		{"addNode props", `Graph.addNode(Graph.root(), "person", {name: "Alice", age: 30}).name`, "Alice"},
		{"addNode type", `Graph.addNode(Graph.root(), "person", {name: "A"}).type`, "person"},
		{"out count", `Graph.addNode(Graph.root(), "person", {}); Graph.addNode(Graph.root(), "person", {}); Graph.root() |> Graph.out("person") |> len()`, "2"},
		{"node by id", `let a = Graph.addNode(Graph.root(), "person", {name: "Zoe"}); Graph.node(a.id).name`, "Zoe"},
		{"nodes by type", `Graph.addNode(Graph.root(), "x", {}); Graph.addNode(Graph.root(), "y", {}); Graph.nodes("x") |> len()`, "1"},
		{"multi-hop traversal",
			`let alice = Graph.addNode(Graph.root(), "person", {name: "Alice"}); ` +
				`let acme = Graph.addNode(Graph.root(), "company", {name: "Acme"}); ` +
				`Graph.link(alice, acme, "worksAt"); ` +
				`Graph.node(alice.id) |> Graph.out("worksAt") |> map(c => c.name)`, `["Acme"]`},
		{"inbound traversal",
			`let alice = Graph.addNode(Graph.root(), "person", {name: "Alice"}); ` +
				`let acme = Graph.addNode(Graph.root(), "company", {name: "Acme"}); ` +
				`Graph.link(alice, acme, "worksAt"); ` +
				`Graph.node(acme.id) |> Graph.inbound("worksAt") |> map(n => n.name)`, `["Alice"]`},
		{"both traversal",
			`let a = Graph.addNode(Graph.root(), "p", {name: "A"}); ` +
				`let b = Graph.addNode(Graph.root(), "p", {name: "B"}); ` +
				`Graph.link(a, b, "knows"); Graph.node(a.id) |> Graph.both("knows") |> len()`, "1"},
		{"outE props",
			`let a = Graph.addNode(Graph.root(), "p", {name: "A"}); ` +
				`let b = Graph.addNode(Graph.root(), "p", {name: "B"}); ` +
				`Graph.link(a, b, "knows", {since: 2020}); ` +
				`Graph.node(a.id) |> Graph.outE("knows") |> map(e => e.since)`, "[2020]"},
		{"setProps node",
			`let a = Graph.addNode(Graph.root(), "p", {name: "A", age: 30}); Graph.setProps(a, {age: 31}); Graph.node(a.id).age`, "31"},
		{"removeNode drops edges",
			`let a = Graph.addNode(Graph.root(), "p", {}); Graph.removeNode(a); Graph.root() |> Graph.out("p") |> len()`, "0"},
		{"unlink",
			`let a = Graph.addNode(Graph.root(), "p", {}); let b = Graph.addNode(Graph.root(), "p", {}); ` +
				`let e = Graph.link(a, b, "knows"); Graph.unlink(e); Graph.node(a.id) |> Graph.outE() |> len()`, "0"},
		{"degree count (README example)",
			`let a = Graph.addNode(Graph.root(), "person", {name: "A"}); ` +
				`let b = Graph.addNode(Graph.root(), "person", {name: "B"}); ` +
				`Graph.link(a, b, "knows"); Graph.link(a, b, "likes"); ` +
				`Graph.nodes("person") |> map(p => Graph.node(p.id) |> Graph.outE() |> len())`, "[2, 0]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := graphShell().Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q) errored:\n%v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

func TestGraphErrors(t *testing.T) {
	cases := []struct{ name, src, wantSub string }{
		{"unknown node", `Graph.node("nope")`, "not found"},
		{"remove root", `Graph.removeNode(Graph.root())`, "cannot remove root"},
		{"link missing source", `Graph.link("nope", Graph.root(), "x")`, "source node"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := graphShell().Eval(c.src)
			if err == nil || !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("eval(%q) error = %v, want substring %q", c.src, err, c.wantSub)
			}
		})
	}
}
