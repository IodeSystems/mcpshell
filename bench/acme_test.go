package bench

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func TestAcmeTools(t *testing.T) {
	d := NewAcme()
	tools := d.Tools()
	if len(tools) != 102 {
		t.Fatalf("got %d tools, want 102", len(tools))
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		if seen[tool.Name] {
			t.Errorf("duplicate tool %s", tool.Name)
		}
		seen[tool.Name] = true
		if !json.Valid(tool.InputSchema) {
			t.Errorf("%s: invalid schema", tool.Name)
		}
	}
	if len(d.OpenAITools()) != 102 {
		t.Fatal("OpenAITools must list every tool")
	}
}

func TestAcmeCall(t *testing.T) {
	d := NewAcme()
	cases := []struct {
		tool, args, want string
		isError          bool
	}{
		{"get_customer", `{"id":1}`, `"id":1`, false},
		{"get_customer", `{"id":999}`, "not found", true},
		{"list_invoices", `{"status":"paid","page_size":5}`, `"page_size":5`, false},
		{"list_invoices", `{"page_size":100}`, "page_size 1-25", true},
		{"list_invoices", `{"colour":"red"}`, "unknown filter", true},
		{"search_customers", `{"query":"co 17"}`, `"id":17`, false},
		{"create_team", `{}`, "name is required", true},
		{"nope", `{}`, "unknown tool", true},
	}
	for _, c := range cases {
		text, isError := d.Call(c.tool, []byte(c.args))
		if isError != c.isError || !strings.Contains(text, c.want) {
			t.Errorf("%s(%s) = %q, isError %v; want %q, isError %v", c.tool, c.args, text, isError, c.want, c.isError)
		}
	}
}

// TestAcmeTasksHaveNoTies keeps every expected answer unique, so a correct
// model answer cannot fail and a wrong one cannot pass by a tie.
func TestAcmeTasksHaveNoTies(t *testing.T) {
	for _, task := range acmeTasks(NewAcme()) {
		if task.Tie {
			t.Errorf("%s: expected answer %v is not unique", task.Name, task.Expect)
		}
	}
}

func TestWhole(t *testing.T) {
	v := whole("12", "Ada")
	for s, want := range map[string]bool{"Ada=12": true, "Ada=123": false, "Adams=12": false, "12 (Ada)": true} {
		if v(s) != want {
			t.Errorf("whole(12, Ada)(%q) = %v", s, !want)
		}
	}
}

// acmeReferences solve each hosted task in one eval through mcpshell hosting
// the Acme MCP server, the path the mcpshell arm takes.
var acmeReferences = map[string]string{
	"hosted_lookup_email": `acme.search_customers({query: name}) |> filter(c => c.name == name) |> at(0) |> (c => c.email)`,
	"hosted_region_paid_invoices": `let cs = pages(acme.list_customers, {}); let byId = {}; for (const c of cs) byId[str(c.id)] = c;
		pages(acme.list_invoices, {status: "paid"}) |> groupBy(i => byId[str(i.customer_id)].region) |> entries
		|> map(e => [e[0], e[1] |> map(i => i.amount) |> sum()]) |> sort((a, b) => b[1] - a[1]) |> at(0) |> (e => e[0] + "=" + str(e[1]))`,
	"hosted_gold_open_tickets": `let gold = pages(acme.list_customers, {tier: "gold"}) |> map(c => c.id);
		pages(acme.list_tickets, {status: "open"}) |> filter(t => gold.includes(t.customer_id)) |> len()`,
	"hosted_top_product_delivered": `let del = pages(acme.list_orders, {status: "delivered"}) |> map(o => o.id);
		let units = pages(acme.list_order_items, {}) |> filter(i => del.includes(i.order_id)) |> groupBy(i => str(i.product_id)) |> entries
		|> map(e => [e[0], e[1] |> map(i => i.qty) |> sum()]) |> sort((a, b) => b[1] - a[1]);
		acme.get_product({id: num(units[0][0])}).name`,
	"hosted_platform_open_tasks": `let team = acme.search_teams({query: "Platform"}) |> at(0);
		let projects = pages(acme.list_projects, {team_id: team.id}) |> map(p => p.id);
		let counts = pages(acme.list_tasks, {status: "open"}) |> filter(t => projects.includes(t.project_id)) |> countBy(t => str(t.assignee_id)) |> entries
		|> sort((a, b) => b[1] - a[1]);
		acme.get_employee({id: num(counts[0][0])}).name`,
	"hosted_customer_overdue_total": `let c = acme.search_customers({query: name}) |> filter(c => c.name == name) |> at(0);
		pages(acme.list_invoices, {status: "overdue", customer_id: c.id}) |> map(i => i.amount) |> sum()`,
	"hosted_west_avg_order": `let west = pages(acme.list_customers, {region: "West"}) |> map(c => c.id);
		let os = pages(acme.list_orders, {}) |> filter(o => o.status != "cancelled" && west.includes(o.customer_id));
		((os |> map(o => o.total) |> sum()) / len(os)).toFixed(2)`,
	"hosted_vendor_approved_po": `pages(acme.list_purchase_orders, {status: "approved"}) |> groupBy(p => str(p.vendor_id)) |> entries
		|> map(e => [e[0], e[1] |> map(p => p.amount) |> sum()]) |> sort((a, b) => b[1] - a[1]) |> at(0)
		|> (e => acme.get_vendor({id: num(e[0])}).name)`,
}

// pagePrelude defines pages(tool, filters): every row of a paginated list tool.
const pagePrelude = `function pages(tool, filters) {
	let out = []; let page = 1;
	while (true) {
		let r = tool({...filters, page: page, page_size: 25});
		out = [...out, ...r.items];
		if (!r.has_more) return out;
		page = page + 1;
	}
}
`

func TestHostedReferenceSolutions(t *testing.T) {
	d := NewAcme()
	srv := ServeAcme(d)
	defer srv.Close()

	tasks := acmeTasks(d)
	for _, ts := range HostedSuite(d) {
		t.Run(ts.Name, func(t *testing.T) {
			code, ok := acmeReferences[ts.Name]
			if !ok {
				t.Fatal("no reference solution")
			}
			prelude := pagePrelude
			for _, task := range tasks {
				if task.Name == ts.Name && strings.Contains(task.Prompt, "named ") {
					name := task.Prompt[strings.Index(task.Prompt, `"`)+1:]
					prelude += "let name = " + `"` + name[:strings.Index(name, `"`)] + `"` + ";\n"
				}
			}
			sh := runtime.NewShell()
			toolkit.InstallCore(sh)
			toolkit.InstallMath(sh)
			closers := mcp.InstallClients(sh, []mcp.NamedServer{{Namespace: "acme", Config: mcp.ServerConfig{URL: srv.URL}}})
			if len(closers) != 1 {
				t.Fatal("acme upstream did not connect")
			}
			defer closers[0].Close()
			v, err := sh.Eval(prelude + code)
			if err != nil {
				t.Fatalf("eval error: %v", err)
			}
			if got := v.Display(); !ts.Validate(got) {
				t.Fatalf("answer %q fails the validator", got)
			}
		})
	}
}
