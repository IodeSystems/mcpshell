package toolkit_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func graphAlgoShell(t *testing.T) *runtime.Shell {
	t.Helper()
	sh := runtime.NewShell()
	toolkit.InstallCore(sh)
	toolkit.InstallMath(sh)
	sh.AddLoadable(toolkit.GraphLoadable())
	toolkit.AddSolver(sh)
	okEval(t, sh, `load("graph")`)
	return sh
}

func TestGraphAlgoLoad(t *testing.T) {
	sh := runtime.NewShell()
	toolkit.InstallCore(sh)
	sh.AddLoadable(toolkit.GraphLoadable())
	mustContain(t, "before load", failEval(t, sh, `Graph.topo({edges: []})`), `load("graph")`)
	first := okEval(t, sh, `load("graph")`)
	mustContain(t, "first load", first, "Graph.path(", "Graph.reach(", "Graph.components(", "Graph.cycle(",
		"Graph.topo(", "Graph.cpm(", "Graph.flow(", "Graph.match(", "Graph.mst(", "edges(", "TWO-WAY", "Solver.lp")
	// Errors end with the usage of the command.
	mustContain(t, "usage", failEval(t, sh, `Graph.topo({edges: [{from: "a", to: "b"}], wieght: 1})`),
		`unknown key "wieght"`, "Usage: Graph.topo(")
	// help("graph") is the guide.
	mustContain(t, "help", okEval(t, sh, `help("graph")`), "TYPICAL: Project schedule", "GRAPH TO Solver.lp")
}

func TestGraphAlgoResults(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"path exact", `Graph.path({edges: [{from: "a", to: "b", weight: 0.1}, {from: "b", to: "c", weight: 0.2}], from: "a", to: "c"}).weight`, "0.3"},
		{"path two-way", `Graph.path({edges: [{from: "a", to: "b", dist: 4}], from: "b", to: "a"}).dist`, "4"},
		{"path directed none", `Graph.path({edges: [{from: "a", to: "b", dist: 4}], from: "b", to: "a", directed: true}).found`, "false"},
		{"path hops", `Graph.path({edges: [{from: "a", to: "b"}, {from: "b", to: "c"}, {from: "a", to: "c"}], from: "a", to: "c", weight: 1}).dist`, "1"},
		{"path all", `Graph.path({edges: [{from: "a", to: "b", weight: 2}, {from: "b", to: "c", weight: 3}], from: "a"}) |> map(r => r.to + r.weight)`, `["b2", "c5"]`},
		{"path negative", `Graph.path({edges: [{from: "a", to: "b", weight: 4}, {from: "b", to: "c", weight: -3}, {from: "a", to: "c", weight: 2}], from: "a", to: "c", directed: true}) |> (r => [r.weight, r.method])`, `[1, "bellman-ford"]`},
		{"path longest", `Graph.path({edges: [{from: "a", to: "b", weight: 4}, {from: "b", to: "c", weight: 3}, {from: "a", to: "c", weight: 2}], from: "a", to: "c", directed: true, longest: true}).path`, `["a", "b", "c"]`},
		{"reach dfs", `Graph.reach({edges: [{from: "a", to: "b"}, {from: "b", to: "c"}, {from: "a", to: "d"}], from: "a", order: "dfs"}) |> map(r => r.id)`, `["a", "b", "c", "d"]`},
		{"reach depth", `Graph.reach({edges: [{from: "a", to: "b"}, {from: "b", to: "c"}], from: "a", maxDepth: 1}) |> len()`, "2"},
		{"reach reverse", `Graph.reach({edges: [{from: "a", to: "b"}, {from: "c", to: "b"}], from: "b", directed: true, reverse: true}) |> map(r => r.id)`, `["b", "a", "c"]`},
		{"components", `Graph.components({edges: [{from: "a", to: "b"}, {from: "c", to: "d"}, {from: "d", to: "e"}], nodes: [{id: "z"}]}) |> (r => [r.count, r.components])`, `[3, [["c", "d", "e"], ["a", "b"], ["z"]]]`},
		{"scc", `Graph.components({edges: [{from: "a", to: "b"}, {from: "b", to: "a"}, {from: "b", to: "c"}], directed: true}).components`, `[["a", "b"], ["c"]]`},
		{"cycle directed", `Graph.cycle({edges: [{from: "a", to: "b"}, {from: "b", to: "c"}, {from: "c", to: "a"}], directed: true}).cycle`, `["a", "b", "c", "a"]`},
		{"cycle pair is one edge", `Graph.cycle({edges: [{from: "a", to: "b"}, {from: "b", to: "a"}]}).found`, "false"},
		{"cycle undirected", `Graph.cycle({edges: [{from: "a", to: "b"}, {from: "b", to: "c"}, {from: "a", to: "c"}]}).cycle |> len()`, "4"},
		{"topo", `Graph.topo({edges: [{from: "b", to: "c"}, {from: "a", to: "b"}]})`, `["a", "b", "c"]`},
		{"cpm", `Graph.cpm({nodes: [{id: "A", duration: 3}, {id: "B", duration: 2, after: "A"}, {id: "C", duration: 5, after: ["A"]}, {id: "D", duration: 4, after: ["B", "C"]}, {id: "E", duration: 1, after: ["C"]}, {id: "F", duration: 1, after: ["D", "E"]}]}) |> (r => [r.makespan, r.critical, r.tasks |> filter(t => !t.critical) |> map(t => t.id + t.slack)])`,
			`[13, ["A", "C", "D", "F"], ["B3", "E3"]]`},
		{"cpm edges lag", `Graph.cpm({nodes: [{id: "a", duration: 2}, {id: "b", duration: 1}], edges: [{from: "a", to: "b", lag: 0.5}]}).makespan`, "3.5"},
		{"cpm duration constant", `Graph.cpm({edges: [{from: "a", to: "b"}, {from: "b", to: "c"}], duration: 1}).makespan`, "3"},
		{"flow max", `Graph.flow({edges: [{from: "s", to: "a", cap: 3}, {from: "s", to: "b", cap: 2}, {from: "a", to: "b", cap: 1}, {from: "a", to: "t", cap: 2}, {from: "b", to: "t", cap: 3}], from: "s", to: "t", directed: true}) |> (r => [r.status, r.value, r.cut |> map(e => e.to)])`,
			`["optimal", 5, ["a", "b"]]`},
		{"flow min cost", `Graph.flow({edges: [{from: "s", to: "a", cap: 3, cost: 1}, {from: "s", to: "b", cap: 2, cost: 4}, {from: "a", to: "b", cap: 1, cost: 1}, {from: "a", to: "t", cap: 2, cost: 5}, {from: "b", to: "t", cap: 3, cost: 1}], from: "s", to: "t", directed: true}) |> (r => [r.value, r.cost])`, "[5, 25]"},
		{"flow amount", `Graph.flow({edges: [{from: "s", to: "t", cap: 3, cost: 2}, {from: "s", to: "t", cap: 3, cost: 1}], from: "s", to: "t", amount: 4, directed: true}).cost`, "5"},
		{"flow amount infeasible", `Graph.flow({edges: [{from: "s", to: "t", cap: 3}], from: "s", to: "t", amount: 4, directed: true}) |> (r => [r.status, r.value])`, `["infeasible", 3]`},
		{"flow transport rational", `Graph.flow({nodes: [{id: "w1", supply: 30}, {id: "w2", supply: 20}, {id: "c1", supply: -25}, {id: "c2", supply: -25}], edges: [{from: "w1", to: "c1", cost: 2.5}, {from: "w1", to: "c2", cost: 4}, {from: "w2", to: "c1", cost: 3}, {from: "w2", to: "c2", cost: 1/3}], directed: true}).cost`,
			`89.1666666666666666666666666666666667`},
		{"flow two-way", `Graph.flow({edges: [{from: "a", to: "s", cap: 2}, {from: "a", to: "t", cap: 5}], from: "s", to: "t"}) |> (r => [r.value, r.flows |> map(e => e.flow)])`, "[2, [-2, 2]]"},
		{"flow unbounded", `Graph.flow({edges: [{from: "s", to: "t", cap: null}], from: "s", to: "t", directed: true}).status`, `unbounded`},
		{"match size", `Graph.match({edges: [{from: "a", to: "x"}, {from: "a", to: "y"}, {from: "b", to: "x"}]}).size`, "2"},
		{"match cost", `Graph.match({edges: [{from: "ann", to: "t1", cost: 3}, {from: "ann", to: "t2", cost: 1}, {from: "bob", to: "t1", cost: 2}]}) |> (r => [r.size, r.cost])`, "[2, 3]"},
		{"match maximize", `Graph.match({edges: [{from: "a", to: "x", cost: 5}, {from: "a", to: "y", cost: 1}, {from: "b", to: "x", cost: 4}, {from: "b", to: "y", cost: 3}], maximize: true}).cost`, "8"},
		{"mst", `Graph.mst({edges: [{from: "a", to: "b", weight: 1}, {from: "b", to: "c", weight: 2}, {from: "a", to: "c", weight: 3}]}) |> (r => [r.weight, r.trees])`, "[3, 1]"},
		{"mst max", `Graph.mst({edges: [{from: "a", to: "b", km: 1}, {from: "b", to: "c", km: 2}, {from: "a", to: "c", km: 3}], weight: "km", maximize: true}).km`, "5"},
		{"store input", `let a = addNode(root(), "task", {duration: 2}); let b = addNode(root(), "task", {duration: 3}); link(a, b, "before"); ` +
			`Graph.cpm({nodes: nodes("task"), edges: edges("before")}).makespan`, "5"},
		{"store components", `let a = addNode(root(), "p", {}); let b = addNode(root(), "p", {}); addNode(root(), "p", {}); link(a, b, "knows"); ` +
			`Graph.components({nodes: nodes("p"), edges: edges("knows")}).count`, "2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := okEval(t, graphAlgoShell(t), c.src); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestGraphAlgoErrors(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{"topo cycle names edges", `Graph.topo({edges: [{from: "a", to: "b"}, {from: "b", to: "c"}, {from: "c", to: "a"}]})`,
			[]string{`"a" -> "b" -> "c" -> "a"`, "edges[0], edges[1], edges[2]"}},
		{"topo undirected", `Graph.topo({edges: [{from: "a", to: "b"}], directed: false})`, []string{"directed edges only"}},
		{"cpm cycle", `Graph.cpm({nodes: [{id: "a", duration: 1, after: "b"}, {id: "b", duration: 1, after: "a"}]})`, []string{"cycle", `"a" -> "b" -> "a"`}},
		{"cpm missing duration", `Graph.cpm({nodes: [{id: "a", duration: 1}], edges: [{from: "a", to: "b"}]})`, []string{`task "b" is in edges but has no row in nodes`}},
		{"cpm unknown after", `Graph.cpm({nodes: [{id: "a", duration: 1, after: "zz"}]})`, []string{`nodes[0].after[0]: "zz" is not a task`}},
		{"path needs weight", `Graph.path({edges: [{from: "a", to: "b"}], from: "a", to: "b"})`, []string{"weight: 1 to count hops"}},
		{"path weight typo", `Graph.path({edges: [{from: "a", to: "b", wieght: 2}], from: "a", to: "b"})`, []string{`did you mean "weight"`}},
		{"path negative two-way", `Graph.path({edges: [{from: "a", to: "b", weight: -1}], from: "a", to: "b"})`, []string{"edges[0]", "directed: true"}},
		{"path negative cycle", `Graph.path({edges: [{from: "a", to: "b", weight: -1}, {from: "b", to: "a", weight: -1}], from: "a", directed: true})`, []string{"negative cycle"}},
		{"path unknown node", `Graph.path({edges: [{from: "a", to: "b", weight: 1}], from: "a", to: "q"})`, []string{`to "q" is not a node`}},
		{"longest two-way", `Graph.path({edges: [{from: "a", to: "b", weight: 1}], from: "a", longest: true})`, []string{"directed: true"}},
		{"flow float", `Graph.flow({edges: [{from: "s", to: "t", cap: Math.sqrt(2)}], from: "s", to: "t", directed: true})`, []string{"edges[0] cap is not an exact number"}},
		{"flow supplies", `Graph.flow({nodes: [{id: "a", supply: 3}, {id: "b", supply: -2}], edges: [{from: "a", to: "b"}], directed: true})`, []string{"supplies sum to 3 and demands to 2", `supply: -1`}},
		{"flow cap typo", `Graph.flow({edges: [{from: "s", to: "t", capcity: 3}], from: "s", to: "t"})`, []string{`did you mean "capacity"`}},
		{"flow no mode", `Graph.flow({edges: [{from: "s", to: "t", cap: 3}]})`, []string{"needs from and to"}},
		{"flow negative cap", `Graph.flow({edges: [{from: "s", to: "t", cap: -3}], from: "s", to: "t"})`, []string{"edges[0] cap must be >= 0"}},
		{"match not bipartite", `Graph.match({edges: [{from: "a", to: "b"}, {from: "b", to: "c"}]})`, []string{`"b" is both a from (left) and a to (right)`}},
		{"mst directed", `Graph.mst({edges: [{from: "a", to: "b", weight: 1}], directed: true})`, []string{"undirected edges only"}},
		{"reach reverse two-way", `Graph.reach({edges: [{from: "a", to: "b"}], from: "a", reverse: true})`, []string{"directed: true"}},
		{"edge row", `Graph.components({edges: [{from: "a"}]})`, []string{"edges[0] needs from and to"}},
		{"duplicate node", `Graph.components({nodes: [{id: "a"}, {id: "a"}]})`, []string{`nodes[1]: duplicate id "a"`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mustContain(t, c.name, failEval(t, graphAlgoShell(t), c.src), c.want...)
		})
	}
}

func TestGraphAlgoNotes(t *testing.T) {
	sh := graphAlgoShell(t)
	okEval(t, sh, `Graph.mst({edges: [{from: "a", to: "b", weight: 1}, {from: "c", to: "d", weight: 1}]})`)
	mustContain(t, "forest note", sh.Diagnostics().Format(), "spanning forest")
	okEval(t, sh, `Graph.flow({edges: [{from: "s", to: "t", cap: 3}], from: "s", to: "t", amount: 4, directed: true})`)
	mustContain(t, "infeasible note", sh.Diagnostics().Format(), "only 3 of 4")
}

// Min-cost flow through the toolkit (rational caps and costs, scaling)
// equals the exact LP optimum of the same network from Solver.lp.
func TestGraphFlowMatchesSolverLP(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for trial := range 25 {
		sh := graphAlgoShell(t)
		okEval(t, sh, `load("solver")`)
		n := 3 + rng.Intn(6)
		var edges []string
		seen := map[[2]int]bool{}
		for range 3 * n {
			a, b := rng.Intn(n), rng.Intn(n)
			if a == b || seen[[2]int{a, b}] {
				continue
			}
			seen[[2]int{a, b}] = true
			edges = append(edges, fmt.Sprintf(`{from: "n%d", to: "n%d", cap: %d/%d, cost: %d/%d}`,
				a, b, 1+rng.Intn(20), 1+rng.Intn(3), rng.Intn(15), 1+rng.Intn(4)))
		}
		src := fmt.Sprintf(`let edges = [%s]; let nodes = [{id: "n0"}, {id: "n%d"}];
			let f = Graph.flow({edges, nodes, from: "n0", to: "n%d", directed: true});
			let id = e => e.from + ">" + e.to;
			let ids = edges |> flatMap(e => [e.from, e.to]) |> unique();
			// The max-flow value from the LP: maximize ret. Then the cost at that value.
			let mx = Solver.lp({sense: "max",
				vars: [...edges |> map(e => ({id: id(e), obj: 0, hi: e.cap})), {id: "ret", obj: 1}],
				coefs: [...edges |> flatMap(e => [{con: e.from, var: id(e), coef: 1}, {con: e.to, var: id(e), coef: -1}]),
					{con: "n%d", var: "ret", coef: 1}, {con: "n0", var: "ret", coef: -1}],
				rhs: ids |> map(v => ({con: v, eq: 0}))});
			let mc = Solver.lp({sense: "min",
				vars: [...edges |> map(e => ({id: id(e), obj: e.cost, hi: e.cap})), {id: "ret", obj: 0, lo: mx.objective, hi: mx.objective}],
				coefs: [...edges |> flatMap(e => [{con: e.from, var: id(e), coef: 1}, {con: e.to, var: id(e), coef: -1}]),
					{con: "n%d", var: "ret", coef: 1}, {con: "n0", var: "ret", coef: -1}],
				rhs: ids |> map(v => ({con: v, eq: 0}))});
			[f.value == mx.objective, f.cost == mc.objective, f.value, mx.objective, f.cost, mc.objective]`,
			strings.Join(edges, ", "), n-1, n-1, n-1, n-1)
		got := okEval(t, sh, src)
		if !strings.HasPrefix(got, "[true, true,") {
			t.Errorf("trial %d: flow vs LP %s", trial, got)
		}
	}
}
