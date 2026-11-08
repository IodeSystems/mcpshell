"""Reference values for graphalg tests: networkx 3.x and scipy.

Run: venv/bin/python -I gen_refs.py <out_dir>    (pip install networkx scipy)

Random graphs with integer weights, so every reference value is exact.
Each case keeps the arc list in input order; the Go test rebuilds the same
graph from it.
"""
import json
import random
import sys

import networkx as nx
import numpy as np
from scipy.optimize import linear_sum_assignment

out = sys.argv[1]
rng = random.Random(11)


def rand_arcs(n, m, allow_loops=False):
    """m distinct ordered pairs (no duplicates)."""
    seen = set()
    arcs = []
    tries = 0
    while len(arcs) < m and tries < 50 * m:
        tries += 1
        a, b = rng.randrange(n), rng.randrange(n)
        if a == b and not allow_loops:
            continue
        if (a, b) in seen:
            continue
        seen.add((a, b))
        arcs.append([a, b])
    return arcs


def sets(groups):
    return sorted(sorted(g) for g in groups)


cases = {"components": [], "flow": [], "mincost": [], "matching": [], "assignment": [],
         "mst": [], "shortest": [], "dag": []}

for _ in range(60):
    n = rng.randrange(1, 40)
    m = rng.randrange(0, 2 * n + 1)
    arcs = rand_arcs(n, m) if n > 1 else []
    g = nx.DiGraph()
    g.add_nodes_from(range(n))
    g.add_edges_from(map(tuple, arcs))
    cases["components"].append({
        "n": n, "arcs": arcs,
        "weak": sets(nx.weakly_connected_components(g)),
        "strong": sets(nx.strongly_connected_components(g)),
        "dag": nx.is_directed_acyclic_graph(g),
        "undirectedCycle": len(nx.cycle_basis(g.to_undirected())) > 0,
    })

for _ in range(60):
    n = rng.randrange(2, 30)
    arcs = rand_arcs(n, rng.randrange(1, 4 * n))
    caps = [rng.randrange(0, 20) for _ in arcs]
    costs = [rng.randrange(-5, 15) for _ in arcs]
    s, t = 0, n - 1
    g = nx.DiGraph()
    g.add_nodes_from(range(n))
    for (a, b), c, w in zip(arcs, caps, costs):
        g.add_edge(a, b, capacity=c, weight=w)
    value = nx.maximum_flow_value(g, s, t)
    cases["flow"].append({"n": n, "arcs": arcs, "cap": caps, "s": s, "t": t, "value": value})
    # Min cost flow with supplies: ship a random amount from s to t, plus
    # random transshipment demands. networkx demand = -supply. Negative costs
    # with finite capacities are allowed by network simplex.
    supply = [0] * n
    amt = rng.randrange(0, value + 3)
    supply[s] += amt
    supply[t] -= amt
    for v in range(n):
        g.nodes[v]["demand"] = -supply[v]
    try:
        cost = nx.min_cost_flow_cost(g)
        feasible = True
    except nx.NetworkXUnfeasible:
        cost, feasible = None, False
    cases["mincost"].append({"n": n, "arcs": arcs, "cap": caps, "cost": costs,
                             "supply": supply, "feasible": feasible, "total": cost})

for _ in range(60):
    nl, nr = rng.randrange(1, 15), rng.randrange(1, 15)
    edges = []
    for l in range(nl):
        for r in range(nr):
            if rng.random() < 0.3:
                edges.append([l, r])
    g = nx.Graph()
    left = [("L", l) for l in range(nl)]
    g.add_nodes_from(left)
    g.add_nodes_from(("R", r) for r in range(nr))
    g.add_edges_from((("L", l), ("R", r)) for l, r in edges)
    mm = nx.bipartite.hopcroft_karp_matching(g, top_nodes=left)
    size = len(mm) // 2
    costs = [rng.randrange(-10, 30) for _ in edges]
    # Min-cost maximum-cardinality matching via min-cost flow of `size` units.
    fg = nx.DiGraph()
    fg.add_node("S", demand=-size)
    fg.add_node("T", demand=size)
    for l in range(nl):
        fg.add_edge("S", ("L", l), capacity=1, weight=0)
    for r in range(nr):
        fg.add_edge(("R", r), "T", capacity=1, weight=0)
    for (l, r), c in zip(edges, costs):
        fg.add_edge(("L", l), ("R", r), capacity=1, weight=c)
    mc = nx.min_cost_flow_cost(fg)
    cases["matching"].append({"nl": nl, "nr": nr, "edges": edges, "cost": costs,
                              "size": size, "minCost": mc})

for _ in range(40):
    nl, nr = rng.randrange(1, 12), rng.randrange(1, 12)
    c = np.array([[rng.randrange(0, 100) for _ in range(nr)] for _ in range(nl)])
    ri, ci = linear_sum_assignment(c)
    cases["assignment"].append({"cost": c.tolist(), "minCost": int(c[ri, ci].sum()),
                                "size": len(ri)})

for _ in range(60):
    n = rng.randrange(1, 40)
    pairs = set()
    edges = []
    for a, b in rand_arcs(n, rng.randrange(0, 3 * n + 1)) if n > 1 else []:
        if (b, a) in pairs:
            continue
        pairs.add((a, b))
        edges.append([a, b])
    w = [rng.randrange(-20, 50) for _ in edges]
    g = nx.Graph()
    g.add_nodes_from(range(n))
    for (a, b), x in zip(edges, w):
        g.add_edge(a, b, weight=x)
    t = nx.minimum_spanning_tree(g)
    tm = nx.maximum_spanning_tree(g)
    cases["mst"].append({"n": n, "edges": edges, "w": w,
                         "weight": t.size(weight="weight"), "maxWeight": tm.size(weight="weight"),
                         "trees": nx.number_connected_components(g)})

for _ in range(60):
    n = rng.randrange(2, 40)
    arcs = rand_arcs(n, rng.randrange(1, 4 * n))
    negative = rng.random() < 0.4
    w = [rng.randrange(-8 if negative else 0, 30) for _ in arcs]
    g = nx.DiGraph()
    g.add_nodes_from(range(n))
    for (a, b), x in zip(arcs, w):
        g.add_edge(a, b, weight=x)
    rec = {"n": n, "arcs": arcs, "w": w, "src": 0}
    if negative and nx.negative_edge_cycle(g):
        # Only a cycle reachable from 0 is an error for Bellman-Ford from 0.
        reach = nx.descendants(g, 0) | {0}
        rec["negCycle"] = nx.negative_edge_cycle(g.subgraph(reach).copy())
    if not rec.get("negCycle"):
        d = nx.single_source_bellman_ford_path_length(g, 0)
        rec["dist"] = {str(k): v for k, v in d.items()}
    cases["shortest"].append(rec)

for _ in range(60):
    n = rng.randrange(1, 40)
    perm = list(range(n))
    rng.shuffle(perm)
    rank = {v: i for i, v in enumerate(perm)}
    arcs = [[a, b] for a, b in rand_arcs(n, rng.randrange(0, 3 * n + 1)) if rank[a] < rank[b]] if n > 1 else []
    lag = [rng.randrange(0, 5) for _ in arcs]
    dur = [rng.randrange(0, 10) for _ in range(n)]
    # Longest path with node durations: expand each task into start/end nodes.
    g = nx.DiGraph()
    for v in range(n):
        g.add_edge(("s", v), ("e", v), weight=dur[v])
    for (a, b), x in zip(arcs, lag):
        g.add_edge(("e", a), ("s", b), weight=x)
    makespan = nx.dag_longest_path_length(g, weight="weight")
    # Edge-weight longest path from node 0.
    h = nx.DiGraph()
    h.add_nodes_from(range(n))
    for (a, b), x in zip(arcs, lag):
        h.add_edge(a, b, weight=x)
    order = list(nx.topological_sort(h))
    dist = {0: 0}
    for u in order:
        if u in dist:
            for _, v, d in h.out_edges(u, data=True):
                if v not in dist or dist[u] + d["weight"] > dist[v]:
                    dist[v] = dist[u] + d["weight"]
    cases["dag"].append({"n": n, "arcs": arcs, "lag": lag, "dur": dur, "makespan": makespan,
                         "longestFrom0": {str(k): v for k, v in dist.items()}})

with open(out + "/refs.json", "w") as f:
    json.dump(cases, f, separators=(",", ":"))
print("networkx", nx.__version__)
