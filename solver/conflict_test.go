package solver

import (
	"context"
	"errors"
	"math/rand"
	"testing"
)

// subModel keeps only the conflict's members of m.
func subModel(m *Model, c *Conflict, skip int) *Model {
	sub := &Model{Sense: m.Sense}
	for _, v := range m.Vars {
		sub.Vars = append(sub.Vars, Var{Name: v.Name, NoLo: true, Integer: v.Integer})
	}
	k := 0
	next := func() bool { k++; return k-1 != skip }
	for _, i := range c.Cons {
		if next() {
			sub.Cons = append(sub.Cons, m.Cons[i])
		}
	}
	for _, j := range c.Lo {
		if next() {
			sub.Vars[j].NoLo, sub.Vars[j].Lo = false, m.Vars[j].Lo
		}
	}
	for _, j := range c.Hi {
		if next() {
			sub.Vars[j].Hi = m.Vars[j].Hi
		}
	}
	return sub
}

// Demand 10 from two plants of capacity 4 and 5: the demand row and both
// capacity rows conflict; the cost row and the unrelated budget do not.
func TestExplainInfeasible(t *testing.T) {
	m := &Model{Objective: ts("a", 1, "b", 1), Vars: vars("a", "b", "c"),
		Cons: []Constraint{
			con(LE, 100, "c", 1),
			con(GE, 10, "a", 1, "b", 1),
			con(LE, 4, "a", 1),
			con(LE, 5, "b", 1),
		}}
	c, err := ExplainInfeasible(context.Background(), m, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Irreducible || len(c.Cons) != 3 || c.Cons[0] != 1 || c.Cons[1] != 2 || c.Cons[2] != 3 || len(c.Hi) != 0 {
		t.Fatalf("got %+v", c)
	}
	// The default lower bounds are not needed: a + b >= 10 with a <= 4, b <= 5
	// fails even for negative a, b.
	if len(c.Lo) != 0 {
		t.Fatalf("lower bounds in conflict: %+v", c)
	}
}

func TestExplainFeasibleIsInvalid(t *testing.T) {
	m := &Model{Objective: ts("x", 1), Vars: vars("x")}
	if _, err := ExplainInfeasible(context.Background(), m, Limits{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

// 2x = 1 has a real solution but no integer one.
func TestExplainIntegerOnly(t *testing.T) {
	m := &Model{Vars: []Var{{Name: "x", Integer: true}}, Cons: []Constraint{con(EQ, 1, "x", 2)}}
	c, err := ExplainInfeasible(context.Background(), m, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Integer || len(c.Cons) != 1 {
		t.Fatalf("got %+v", c)
	}
}

// Random infeasible models: the conflict is infeasible, and dropping any one
// member makes it feasible.
func TestExplainRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	ctx := context.Background()
	n := 0
	for i := 0; i < 600 && n < 80; i++ {
		m := randModel(rng, i%3 == 0)
		r, err := Solve(ctx, m, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != Infeasible {
			continue
		}
		n++
		c, err := ExplainInfeasible(ctx, m, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if !c.Irreducible {
			t.Fatalf("case %d: not irreducible", i)
		}
		size := len(c.Cons) + len(c.Lo) + len(c.Hi)
		for skip := -1; skip < size; skip++ {
			rr, err := Solve(ctx, subModel(m, c, skip), Limits{})
			if err != nil {
				t.Fatal(err)
			}
			if (skip < 0) != (rr.Status == Infeasible) {
				t.Fatalf("case %d: conflict %+v, dropping member %d gives %v", i, c, skip, rr.Status)
			}
		}
	}
	if n < 40 {
		t.Fatalf("only %d infeasible cases", n)
	}
}
