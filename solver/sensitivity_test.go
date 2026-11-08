package solver

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"
)

func sens(t *testing.T, m *Model) (*Result, *Sensitivity) {
	t.Helper()
	r, s, err := SolveSensitivity(context.Background(), m, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return r, s
}

func ratStr(r *big.Rat) string {
	if r == nil {
		return "inf"
	}
	return r.RatString()
}

// The guide's product mix: max 3c + 5t; wood c + 2t <= 14; ratio 3c - t >= 0;
// c <= 6. Optimum c=6, t=4. Wood is worth 5/2 per unit while its rhs stays in
// [6, 42]; the chair cap is worth 1/2; ratio has slack, so 0.
func TestSensitivityProductMix(t *testing.T) {
	m := &Model{Sense: Maximize, Objective: ts("c", 3, "t", 5), Vars: vars("c", "t"),
		Cons: []Constraint{con(LE, 14, "c", 1, "t", 2), con(GE, 0, "c", 3, "t", -1), con(LE, 6, "c", 1)}}
	r, s := sens(t, m)
	wantObj(t, r, "38")
	got := ""
	for i := range m.Cons {
		got += fmt.Sprintf("%s[%s,%s] ", ratStr(s.ShadowPrice[i]), ratStr(s.RHSLo[i]), ratStr(s.RHSHi[i]))
	}
	if want := "5/2[6,42] 0[inf,14] 1/2[2,14] "; got != want {
		t.Errorf("constraints: got %s, want %s", got, want)
	}
	got = ""
	for j := range m.Vars {
		got += fmt.Sprintf("%s[%s,%s] ", ratStr(s.ReducedCost[j]), ratStr(s.ObjLo[j]), ratStr(s.ObjHi[j]))
	}
	// c stays optimal for c's coefficient >= 5/2 (below, the wood/ratio corner
	// wins); t for t's coefficient in [0, 6].
	if want := "0[5/2,inf] 0[0,6] "; got != want {
		t.Errorf("vars: got %s, want %s", got, want)
	}
}

func TestSensitivityRejectsInteger(t *testing.T) {
	m := &Model{Objective: ts("x", 1), Vars: []Var{{Name: "x", Integer: true}}}
	if _, _, err := SolveSensitivity(context.Background(), m, Limits{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

// randSensModel is randModel with some free and upper-bounded-only variables,
// so every variable substitution of the simplex is exercised.
func randSensModel(rng *rand.Rand) *Model {
	m := randModel(rng, false)
	for k := range m.Objective {
		m.Objective[k].Coef = Rat(int64(rng.Intn(21)-10), int64(1+rng.Intn(3)))
	}
	for j := range m.Vars {
		switch rng.Intn(6) {
		case 0:
			m.Vars[j].Lo, m.Vars[j].NoLo = nil, true // x <= hi
		case 1:
			m.Vars[j].Lo, m.Vars[j].NoLo, m.Vars[j].Hi = nil, true, nil // free
			// Keep the LP bounded: box the free variable with a constraint pair.
			name := m.Vars[j].Name
			m.Cons = append(m.Cons, con(LE, 5, name, 1), con(GE, -5, name, 1))
		}
	}
	return m
}

func objAt(m *Model, x []*big.Rat) *big.Rat {
	s := new(big.Rat)
	for _, term := range m.Objective {
		for j, v := range m.Vars {
			if v.Name == term.Var {
				s.Add(s, new(big.Rat).Mul(term.Coef, x[j]))
			}
		}
	}
	return s
}

// Exact checks on random LPs:
//   - strong duality: objective = sum shadow_i*rhs_i + sum reducedCost_j*x_j;
//   - moving one rhs inside its range changes the objective by shadow*delta;
//   - moving one objective coefficient inside its range keeps x optimal;
//   - moving past a finite range end usually breaks the prediction (counted:
//     degenerate vertices can hold it, so this is a coverage check).
func TestSensitivityRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	ctx := context.Background()
	var optimal, rhsChecks, objChecks, rhsTight, rhsEnds, objTight, objEnds int
	for c := range 400 {
		m := randSensModel(rng)
		r, s, err := SolveSensitivity(ctx, m, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != Optimal {
			continue
		}
		optimal++
		checkFeasible(t, m, r)

		dual := new(big.Rat)
		for i, cn := range m.Cons {
			dual.Add(dual, new(big.Rat).Mul(s.ShadowPrice[i], cn.RHS))
		}
		for j := range m.Vars {
			dual.Add(dual, new(big.Rat).Mul(s.ReducedCost[j], r.Values[j]))
		}
		if dual.Cmp(r.Objective) != 0 {
			t.Fatalf("case %d: strong duality: %s != objective %s\n%+v", c, dual.RatString(), r.Objective.RatString(), m)
		}

		resolve := func(mm *Model) *Result {
			rr, err := Solve(ctx, mm, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			return rr
		}
		for i := range m.Cons {
			b := m.Cons[i].RHS
			for _, end := range []*big.Rat{s.RHSLo[i], s.RHSHi[i]} {
				var nb *big.Rat
				if end == nil {
					nb = new(big.Rat).Add(b, Int(1))
					if s.RHSLo[i] == end {
						nb = new(big.Rat).Sub(b, Int(1))
					}
				} else if end.Cmp(b) == 0 {
					continue
				} else {
					nb = new(big.Rat).Add(b, end)
					nb.Quo(nb, Int(2)) // midpoint
				}
				mm := *m
				mm.Cons = append([]Constraint(nil), m.Cons...)
				mm.Cons[i].RHS = nb
				rr := resolve(&mm)
				want := new(big.Rat).Mul(s.ShadowPrice[i], new(big.Rat).Sub(nb, b))
				want.Add(want, r.Objective)
				if rr.Status != Optimal || rr.Objective.Cmp(want) != 0 {
					t.Fatalf("case %d: con %d rhs %s -> %s (range [%s,%s], shadow %s): got %v %v, want %s\n%+v",
						c, i, b.RatString(), nb.RatString(), ratStr(s.RHSLo[i]), ratStr(s.RHSHi[i]),
						s.ShadowPrice[i].RatString(), rr.Status, rr.Objective, want.RatString(), m)
				}
				rhsChecks++
				if end != nil { // just past the end
					rhsEnds++
					past := new(big.Rat).Sub(end, b)
					past.Mul(past, Rat(11, 10)).Add(past, b)
					mm.Cons[i].RHS = past
					rr := resolve(&mm)
					want := new(big.Rat).Mul(s.ShadowPrice[i], new(big.Rat).Sub(past, b))
					want.Add(want, r.Objective)
					if rr.Status != Optimal || rr.Objective.Cmp(want) != 0 {
						rhsTight++
					}
				}
			}
		}
		for j := range m.Vars {
			var cj *big.Rat = new(big.Rat)
			k := -1
			for kk, term := range m.Objective {
				if term.Var == m.Vars[j].Name {
					cj, k = term.Coef, kk
				}
			}
			if k < 0 {
				continue
			}
			for _, end := range []*big.Rat{s.ObjLo[j], s.ObjHi[j]} {
				var nc *big.Rat
				if end == nil {
					nc = new(big.Rat).Add(cj, Int(3))
					if s.ObjLo[j] == end {
						nc = new(big.Rat).Sub(cj, Int(3))
					}
				} else if end.Cmp(cj) == 0 {
					continue
				} else {
					nc = new(big.Rat).Add(cj, end)
					nc.Quo(nc, Int(2))
				}
				mm := *m
				mm.Objective = append([]Term(nil), m.Objective...)
				mm.Objective[k].Coef = nc
				rr := resolve(&mm)
				if want := objAt(&mm, r.Values); rr.Status != Optimal || rr.Objective.Cmp(want) != 0 {
					t.Fatalf("case %d: var %d coef %s -> %s (range [%s,%s]): got %v %v, old x gives %s\n%+v",
						c, j, cj.RatString(), nc.RatString(), ratStr(s.ObjLo[j]), ratStr(s.ObjHi[j]),
						rr.Status, rr.Objective, want.RatString(), m)
				}
				objChecks++
				if end != nil {
					objEnds++
					past := new(big.Rat).Sub(end, cj)
					past.Mul(past, Rat(11, 10)).Add(past, cj)
					mm.Objective[k].Coef = past
					rr := resolve(&mm)
					if rr.Status != Optimal || rr.Objective.Cmp(objAt(&mm, r.Values)) != 0 {
						objTight++
					}
				}
			}
		}
	}
	t.Logf("optimal=%d rhs checks=%d (past end broke %d/%d) obj checks=%d (past end broke %d/%d)",
		optimal, rhsChecks, rhsTight, rhsEnds, objChecks, objTight, objEnds)
	if optimal < 100 || rhsEnds < 50 || objEnds < 50 {
		t.Fatalf("poor coverage")
	}
	// A range that is too narrow passes the inside checks; most past-the-end
	// moves must change the answer or the ranges are not tight.
	if rhsTight*10 < rhsEnds*6 || objTight*10 < objEnds*6 {
		t.Fatalf("ranges look loose: rhs %d/%d, obj %d/%d", rhsTight, rhsEnds, objTight, objEnds)
	}
}

// Two copies of one equality: either rhs alone cannot move, so both ranges
// are a single point.
func TestSensitivityRedundantRows(t *testing.T) {
	m := &Model{Objective: ts("x", 1, "y", 2), Vars: vars("x", "y"),
		Cons: []Constraint{con(EQ, 2, "x", 1, "y", 1), con(EQ, 4, "x", 2, "y", 2)}}
	r, s := sens(t, m)
	wantObj(t, r, "2")
	for i := range m.Cons {
		if s.RHSLo[i].Cmp(m.Cons[i].RHS) != 0 || s.RHSHi[i].Cmp(m.Cons[i].RHS) != 0 {
			t.Errorf("con %d range [%s,%s], want a point", i, ratStr(s.RHSLo[i]), ratStr(s.RHSHi[i]))
		}
	}
	dual := new(big.Rat)
	for i, c := range m.Cons {
		dual.Add(dual, new(big.Rat).Mul(s.ShadowPrice[i], c.RHS))
	}
	for j := range m.Vars {
		dual.Add(dual, new(big.Rat).Mul(s.ReducedCost[j], r.Values[j]))
	}
	if dual.Cmp(r.Objective) != 0 {
		t.Errorf("strong duality: %s", dual.RatString())
	}
}

// Reported, not asserted: sensitivity on the dense 100x100 LP of TestPerfDenseLP.
func TestPerfSensitivity(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	rng := rand.New(rand.NewSource(1))
	const n, mc = 100, 100
	m := &Model{Sense: Maximize}
	for j := range n {
		nm := fmt.Sprintf("x%d", j)
		m.Vars = append(m.Vars, Var{Name: nm})
		m.Objective = append(m.Objective, Term{nm, Int(int64(1 + rng.Intn(9)))})
	}
	for range mc {
		c := Constraint{Op: LE, RHS: Int(int64(100 + rng.Intn(900)))}
		for j := range n {
			c.Terms = append(c.Terms, Term{fmt.Sprintf("x%d", j), Int(int64(1 + rng.Intn(9)))})
		}
		m.Cons = append(m.Cons, c)
	}
	start := time.Now()
	_, err := Solve(context.Background(), m, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	plain := time.Since(start)
	start = time.Now()
	_, s := sens(t, m)
	t.Logf("PERF dense LP %dx%d: solve %v, solve+sensitivity %v (%d ranges)", n, mc, plain, time.Since(start), len(s.ObjLo))
}
