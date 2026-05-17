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
		{"root type", `root().type`, "root"},
		{"addNode props", `addNode(root(), "person", {name: "Alice", age: 30}).name`, "Alice"},
		{"addNode type", `addNode(root(), "person", {name: "A"}).type`, "person"},
		{"out count", `addNode(root(), "person", {}); addNode(root(), "person", {}); root() |> out("person") |> len()`, "2"},
		{"node by id", `let a = addNode(root(), "person", {name: "Zoe"}); node(a.id).name`, "Zoe"},
		{"nodes by type", `addNode(root(), "x", {}); addNode(root(), "y", {}); nodes("x") |> len()`, "1"},
		{"multi-hop traversal",
			`let alice = addNode(root(), "person", {name: "Alice"}); ` +
				`let acme = addNode(root(), "company", {name: "Acme"}); ` +
				`link(alice, acme, "worksAt"); ` +
				`node(alice.id) |> out("worksAt") |> map(c => c.name)`, `["Acme"]`},
		{"inbound traversal",
			`let alice = addNode(root(), "person", {name: "Alice"}); ` +
				`let acme = addNode(root(), "company", {name: "Acme"}); ` +
				`link(alice, acme, "worksAt"); ` +
				`node(acme.id) |> inbound("worksAt") |> map(n => n.name)`, `["Alice"]`},
		{"both traversal",
			`let a = addNode(root(), "p", {name: "A"}); ` +
				`let b = addNode(root(), "p", {name: "B"}); ` +
				`link(a, b, "knows"); node(a.id) |> both("knows") |> len()`, "1"},
		{"outE props",
			`let a = addNode(root(), "p", {name: "A"}); ` +
				`let b = addNode(root(), "p", {name: "B"}); ` +
				`link(a, b, "knows", {since: 2020}); ` +
				`node(a.id) |> outE("knows") |> map(e => e.since)`, "[2020]"},
		{"setProps node",
			`let a = addNode(root(), "p", {name: "A", age: 30}); setProps(a, {age: 31}); node(a.id).age`, "31"},
		{"removeNode drops edges",
			`let a = addNode(root(), "p", {}); removeNode(a); root() |> out("p") |> len()`, "0"},
		{"unlink",
			`let a = addNode(root(), "p", {}); let b = addNode(root(), "p", {}); ` +
				`let e = link(a, b, "knows"); unlink(e); node(a.id) |> outE() |> len()`, "0"},
		{"degree count (README example)",
			`let a = addNode(root(), "person", {name: "A"}); ` +
				`let b = addNode(root(), "person", {name: "B"}); ` +
				`link(a, b, "knows"); link(a, b, "likes"); ` +
				`nodes("person") |> map(p => node(p.id) |> outE() |> len())`, "[2, 0]"},
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
		{"unknown node", `node("nope")`, "not found"},
		{"remove root", `removeNode(root())`, "cannot remove root"},
		{"link missing source", `link("nope", root(), "x")`, "source node"},
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
