package bench

import (
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

// acmeTask is one hosted-suite question with its expected answer computed
// from the dataset. Tie is set when the answer is not unique (a test keeps
// every task free of ties). A count too small to tell from a guess is
// marked Tie too.
type acmeTask struct {
	Name, Prompt, Hint string
	Expect             []string
	Tie                bool
}

// top returns the key with the largest value and whether another key ties it.
func top(m map[string]int) (string, int, bool) {
	best, bestV, tie := "", 0, false
	for _, k := range sortedKeys(m) {
		switch v := m[k]; {
		case best == "" || v > bestV:
			best, bestV, tie = k, v, false
		case v == bestV:
			tie = true
		}
	}
	return best, bestV, tie
}

func acmeTasks(d *AcmeData) []acmeTask {
	customers := d.Table("customers")
	cust := func(id any) map[string]any { return customers[id.(int)-1] }

	var tasks []acmeTask

	// 1. A single lookup: the control. Both arms need one search.
	c := customers[16]
	tasks = append(tasks, acmeTask{Name: "hosted_lookup_email",
		Prompt: fmt.Sprintf("What is the billing email of the customer named %q?", c["name"]),
		Hint:   "Return ONLY the email address.", Expect: []string{c["email"].(string)}})

	// 2. Paid invoice total per region: invoices joined to customers.
	paid := map[string]int{}
	for _, inv := range d.Table("invoices") {
		if inv["status"] == "paid" {
			paid[cust(inv["customer_id"])["region"].(string)] += inv["amount"].(int)
		}
	}
	reg, total, tie := top(paid)
	tasks = append(tasks, acmeTask{Name: "hosted_region_paid_invoices",
		Prompt: "Which customer region has the highest total amount of paid invoices, and what is that total?",
		Hint:   "Return ONLY region=total, e.g. North=1234.", Expect: []string{reg, fmt.Sprint(total)}, Tie: tie})

	// 3. Count across a join, filtered on each side.
	n := 0
	for _, t := range d.Table("tickets") {
		if t["status"] == "open" && cust(t["customer_id"])["tier"] == "gold" {
			n++
		}
	}
	tasks = append(tasks, acmeTask{Name: "hosted_gold_open_tickets",
		Prompt: "How many support tickets with status open belong to gold-tier customers?",
		Hint:   "Return ONLY the count.", Expect: []string{fmt.Sprint(n)}, Tie: n < 3})

	// 4. Units per product over delivered orders: three tables.
	orders := d.Table("orders")
	products := d.Table("products")
	units := map[string]int{}
	for _, it := range d.Table("order_items") {
		if orders[it["order_id"].(int)-1]["status"] == "delivered" {
			units[products[it["product_id"].(int)-1]["name"].(string)] += it["qty"].(int)
		}
	}
	prod, _, tie := top(units)
	tasks = append(tasks, acmeTask{Name: "hosted_top_product_delivered",
		Prompt: "Across all orders with status delivered, which product sold the most units?",
		Hint:   "Return ONLY the product name.", Expect: []string{prod}, Tie: tie})

	// 5. Open tasks per employee on one team's projects: four tables.
	teamID := 0
	for _, t := range d.Table("teams") {
		if t["name"] == "Platform" {
			teamID = t["id"].(int)
		}
	}
	projects := d.Table("projects")
	employees := d.Table("employees")
	open := map[string]int{}
	for _, t := range d.Table("tasks") {
		if t["status"] == "open" && projects[t["project_id"].(int)-1]["team_id"] == teamID {
			open[employees[t["assignee_id"].(int)-1]["name"].(string)]++
		}
	}
	emp, _, tie := top(open)
	tasks = append(tasks, acmeTask{Name: "hosted_platform_open_tasks",
		Prompt: "On projects owned by the Platform team, which employee is assigned the most tasks with status open?",
		Hint:   "Return ONLY the employee's name.", Expect: []string{emp}, Tie: tie})

	// 6. Sum for one customer found by name: search, then a filtered list.
	overdue := map[int]int{}
	count := map[int]int{}
	for _, inv := range d.Table("invoices") {
		if inv["status"] == "overdue" {
			overdue[inv["customer_id"].(int)] += inv["amount"].(int)
			count[inv["customer_id"].(int)]++
		}
	}
	pickID := 0
	for id := 1; id <= len(customers); id++ {
		if count[id] >= 2 && (pickID == 0 || count[id] > count[pickID]) {
			pickID = id
		}
	}
	tasks = append(tasks, acmeTask{Name: "hosted_customer_overdue_total",
		Prompt: fmt.Sprintf("What is the total amount of overdue invoices for the customer named %q?", customers[pickID-1]["name"]),
		Hint:   "Return ONLY the number.", Expect: []string{fmt.Sprint(overdue[pickID])}})

	// 7. An average over a join, to two decimals.
	sum, cnt := 0, 0
	for _, o := range orders {
		if o["status"] != "cancelled" && cust(o["customer_id"])["region"] == "West" {
			sum += o["total"].(int)
			cnt++
		}
	}
	avg := new(big.Rat).SetFrac64(int64(sum), int64(cnt))
	// A third decimal of exactly 5 would leave the rounding ambiguous.
	thousandths := new(big.Rat).Mul(avg, big.NewRat(1000, 1))
	half := thousandths.IsInt() && new(big.Int).Mod(thousandths.Num(), big.NewInt(10)).Int64() == 5
	tasks = append(tasks, acmeTask{Name: "hosted_west_avg_order",
		Prompt: "What is the average order total over orders that are not cancelled, for customers in the West region?",
		Hint:   "Return ONLY the number rounded to 2 decimal places.", Expect: []string{avg.FloatString(2)}, Tie: half})

	// 8. Largest approved purchase-order total per vendor.
	vendors := d.Table("vendors")
	po := map[string]int{}
	for _, p := range d.Table("purchase_orders") {
		if p["status"] == "approved" {
			po[vendors[p["vendor_id"].(int)-1]["name"].(string)] += p["amount"].(int)
		}
	}
	ven, _, tie := top(po)
	tasks = append(tasks, acmeTask{Name: "hosted_vendor_approved_po",
		Prompt: "Which vendor has the largest total amount of approved purchase orders?",
		Hint:   "Return ONLY the vendor name.", Expect: []string{ven}, Tie: tie})

	return tasks
}

// HostedSuite is the hosted-upstream suite: questions about Acme's data that
// need several of its 102 tools. It runs in two arms over the same server —
// the tools as discrete tools, or hosted in mcpshell as the acme namespace.
func HostedSuite(d *AcmeData) []Teaser {
	var out []Teaser
	for _, t := range acmeTasks(d) {
		out = append(out, Teaser{
			Name:       t.Name,
			Prompt:     "You have access to Acme's company tools. " + t.Prompt,
			FormatHint: t.Hint,
			Validate:   whole(t.Expect...),
			TimeoutSec: 300,
		})
	}
	return out
}

// whole is like all, but each value must stand alone: "12" does not match in
// "123", and "Ada" does not match in "Adams".
func whole(subs ...string) func(string) bool {
	alnum := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	return func(s string) bool {
		for _, sub := range subs {
			found := false
			for i := 0; !found; {
				j := strings.Index(s[i:], sub)
				if j < 0 {
					break
				}
				start, end := i+j, i+j+len(sub)
				before, after := ' ', ' '
				if start > 0 {
					before = []rune(s[:start])[len([]rune(s[:start]))-1]
				}
				if end < len(s) {
					after = []rune(s[end:])[0]
				}
				found = !alnum(before) && !alnum(after)
				i = start + 1
			}
			if !found {
				return false
			}
		}
		return true
	}
}
