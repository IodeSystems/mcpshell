package bench

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
)

// Acme is a synthetic company backend for the hosted-upstream benchmark: 17
// entities, each with list/get/search/create/update/delete tools (102 tools),
// the size of a real company MCP server. The data is generated from a fixed
// seed, so every run and every arm sees the same rows. The same tools are
// served two ways: as discrete tools in the prompt, and over MCP to mcpshell,
// which hosts them as the `acme` namespace (see ServeAcme).

// acmeField is one column of an entity.
type acmeField struct {
	Name   string
	Type   string   // JSON schema type: integer, string
	Enum   []string // allowed values, for filters
	Filter bool     // offered as a list_<entity> filter
	Desc   string
}

// acmeEntity is one table and the six tools over it.
type acmeEntity struct {
	Plural   string // customers — tool suffix for list/search
	Singular string // customer — tool suffix for get/create/update/delete
	Desc     string // what one row is
	Search   string // the field search_<plural> matches
	Fields   []acmeField
	Rows     []map[string]any
}

const acmePageMax = 25

var (
	acmeRegions = []string{"North", "South", "East", "West", "Central"}
	acmeFirst   = []string{"Ada", "Ben", "Cara", "Dev", "Eli", "Fay", "Gus", "Hana", "Ivo", "Jun", "Kai", "Lea", "Max", "Nia", "Oto", "Pia", "Quin", "Rui", "Sol", "Tia"}
	acmeLast    = []string{"Abe", "Berg", "Cole", "Diaz", "Engel", "Frost", "Gale", "Hart", "Ito", "Jansen", "Kerr", "Lund", "Moss", "Nash", "Ortiz", "Park"}
	acmeAdj     = []string{"Swift", "Bright", "Solid", "Prime", "Nimble", "Quiet", "Bold", "Clear"}
	acmeNoun    = []string{"Widget", "Gadget", "Bracket", "Sensor", "Valve", "Panel", "Cable", "Router"}
	acmeTeams   = []string{"Platform", "Payments", "Growth", "Support", "Data"}
)

func (e *acmeEntity) field(name string) *acmeField {
	for i := range e.Fields {
		if e.Fields[i].Name == name {
			return &e.Fields[i]
		}
	}
	return nil
}

// AcmeData holds every entity, keyed by plural name.
type AcmeData struct {
	Entities []*acmeEntity
	byPlural map[string]*acmeEntity
}

// Table returns the rows of one entity.
func (d *AcmeData) Table(plural string) []map[string]any { return d.byPlural[plural].Rows }

// NewAcme generates the dataset. The seed is fixed: the hosted suite's
// expected answers are computed from these rows (acme_suite.go), and a test
// checks that no answer is a tie.
func NewAcme() *AcmeData {
	r := rand.New(rand.NewPCG(7, 11))
	pick := func(xs []string) string { return xs[r.IntN(len(xs))] }
	date := func(year int) string {
		return fmt.Sprintf("%d-%02d-%02d", year, 1+r.IntN(12), 1+r.IntN(28))
	}
	person := func(i int) string {
		return acmeFirst[i%len(acmeFirst)] + " " + acmeLast[(i/len(acmeFirst)+i)%len(acmeLast)]
	}
	email := func(name, domain string) string {
		return strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@" + domain
	}
	id := acmeField{Name: "id", Type: "integer", Desc: "unique id"}
	fk := func(name, of string) acmeField {
		return acmeField{Name: name, Type: "integer", Filter: true, Desc: "id of the " + of}
	}
	str := func(name, desc string) acmeField { return acmeField{Name: name, Type: "string", Desc: desc} }
	enum := func(name, desc string, vals ...string) acmeField {
		return acmeField{Name: name, Type: "string", Enum: vals, Filter: true, Desc: desc}
	}
	num := func(name, desc string) acmeField { return acmeField{Name: name, Type: "integer", Desc: desc} }

	d := &AcmeData{byPlural: map[string]*acmeEntity{}}
	add := func(e *acmeEntity, n int, row func(i int) map[string]any) *acmeEntity {
		for i := 1; i <= n; i++ {
			m := row(i)
			m["id"] = i
			e.Rows = append(e.Rows, m)
		}
		d.Entities = append(d.Entities, e)
		d.byPlural[e.Plural] = e
		return e
	}

	add(&acmeEntity{Plural: "customers", Singular: "customer", Desc: "a customer account", Search: "name",
		Fields: []acmeField{id, str("name", "company name"), str("email", "billing email"),
			enum("region", "sales region", acmeRegions...), enum("tier", "support tier", "bronze", "silver", "gold"),
			str("created", "signup date, YYYY-MM-DD")}},
		40, func(i int) map[string]any {
			name := acmeLast[i%len(acmeLast)] + " " + acmeNoun[i%len(acmeNoun)] + " Co " + strconv.Itoa(i)
			return map[string]any{"name": name, "email": email(acmeLast[i%len(acmeLast)]+strconv.Itoa(i), "example.com"),
				"region": pick(acmeRegions), "tier": pick([]string{"bronze", "bronze", "silver", "gold"}), "created": date(2024)}
		})
	add(&acmeEntity{Plural: "contacts", Singular: "contact", Desc: "a person at a customer", Search: "name",
		Fields: []acmeField{id, fk("customer_id", "customer"), str("name", "full name"), str("email", "email"),
			enum("role", "role at the customer", "buyer", "admin", "engineer")}},
		80, func(i int) map[string]any {
			name := person(i + 3)
			return map[string]any{"customer_id": 1 + r.IntN(40), "name": name, "email": email(name, "mail.test"),
				"role": pick([]string{"buyer", "admin", "engineer"})}
		})
	products := add(&acmeEntity{Plural: "products", Singular: "product", Desc: "a product in the catalog", Search: "name",
		Fields: []acmeField{id, str("name", "product name"), enum("category", "catalog category", "hardware", "cabling", "kits"),
			num("price", "list price in whole dollars")}},
		30, func(i int) map[string]any {
			return map[string]any{"name": acmeAdj[i%len(acmeAdj)] + " " + acmeNoun[(i/len(acmeAdj)+i)%len(acmeNoun)] + " " + strconv.Itoa(100+i),
				"category": pick([]string{"hardware", "cabling", "kits"}), "price": 5 + r.IntN(60)}
		})
	orders := add(&acmeEntity{Plural: "orders", Singular: "order", Desc: "a customer order; total is the sum of its order items", Search: "id",
		Fields: []acmeField{id, fk("customer_id", "customer"),
			enum("status", "fulfilment status", "pending", "shipped", "delivered", "cancelled"),
			num("total", "order total in whole dollars"), str("created", "order date, YYYY-MM-DD")}},
		200, func(i int) map[string]any {
			return map[string]any{"customer_id": 1 + r.IntN(40), "status": pick([]string{"pending", "shipped", "delivered", "delivered", "cancelled"}),
				"total": 0, "created": date(2025 + r.IntN(2))}
		})
	add(&acmeEntity{Plural: "order_items", Singular: "order_item", Desc: "one line of an order", Search: "id",
		Fields: []acmeField{id, fk("order_id", "order"), fk("product_id", "product"), num("qty", "units"),
			num("unit_price", "price per unit in whole dollars")}},
		480, func(i int) map[string]any {
			o := 1 + (i-1)%200
			if i > 200 {
				o = 1 + r.IntN(200)
			}
			p := 1 + r.IntN(30)
			qty := 1 + r.IntN(9)
			price := products.Rows[p-1]["price"].(int)
			orders.Rows[o-1]["total"] = orders.Rows[o-1]["total"].(int) + qty*price
			return map[string]any{"order_id": o, "product_id": p, "qty": qty, "unit_price": price}
		})
	add(&acmeEntity{Plural: "invoices", Singular: "invoice", Desc: "a bill sent to a customer", Search: "id",
		Fields: []acmeField{id, fk("customer_id", "customer"), num("amount", "amount in whole dollars"),
			enum("status", "payment status", "paid", "open", "overdue"), str("due", "due date, YYYY-MM-DD")}},
		150, func(i int) map[string]any {
			return map[string]any{"customer_id": 1 + r.IntN(40), "amount": 50 + r.IntN(1950),
				"status": pick([]string{"paid", "paid", "open", "overdue"}), "due": date(2026)}
		})
	add(&acmeEntity{Plural: "payments", Singular: "payment", Desc: "money received against an invoice", Search: "id",
		Fields: []acmeField{id, fk("invoice_id", "invoice"), num("amount", "amount in whole dollars"),
			enum("method", "payment method", "card", "wire", "ach")}},
		90, func(i int) map[string]any {
			return map[string]any{"invoice_id": 1 + r.IntN(150), "amount": 50 + r.IntN(1950), "method": pick([]string{"card", "wire", "ach"})}
		})
	add(&acmeEntity{Plural: "tickets", Singular: "ticket", Desc: "a support ticket", Search: "subject",
		Fields: []acmeField{id, fk("customer_id", "customer"), str("subject", "one-line summary"),
			enum("priority", "urgency", "low", "medium", "high"), enum("status", "ticket state", "open", "pending", "closed")}},
		120, func(i int) map[string]any {
			return map[string]any{"customer_id": 1 + r.IntN(40),
				"subject":  pick([]string{"Login fails", "Invoice question", "Shipment late", "API errors", "Feature request", "Refund"}) + " #" + strconv.Itoa(i),
				"priority": pick([]string{"low", "medium", "high"}), "status": pick([]string{"open", "pending", "closed"})}
		})
	add(&acmeEntity{Plural: "teams", Singular: "team", Desc: "an internal team", Search: "name",
		Fields: []acmeField{id, str("name", "team name")}},
		len(acmeTeams), func(i int) map[string]any { return map[string]any{"name": acmeTeams[i-1]} })
	add(&acmeEntity{Plural: "employees", Singular: "employee", Desc: "an Acme employee", Search: "name",
		Fields: []acmeField{id, str("name", "full name"), str("email", "work email"), fk("team_id", "team")}},
		25, func(i int) map[string]any {
			name := person(i * 7)
			return map[string]any{"name": name, "email": email(name, "acme.test"), "team_id": 1 + r.IntN(len(acmeTeams))}
		})
	add(&acmeEntity{Plural: "projects", Singular: "project", Desc: "an internal project owned by a team", Search: "name",
		Fields: []acmeField{id, str("name", "project name"), fk("team_id", "owning team"),
			enum("status", "project state", "active", "paused", "done")}},
		15, func(i int) map[string]any {
			return map[string]any{"name": "Project " + acmeAdj[i%len(acmeAdj)] + " " + strconv.Itoa(i), "team_id": 1 + r.IntN(len(acmeTeams)),
				"status": pick([]string{"active", "active", "paused", "done"})}
		})
	add(&acmeEntity{Plural: "tasks", Singular: "task", Desc: "a work item in a project", Search: "title",
		Fields: []acmeField{id, fk("project_id", "project"), fk("assignee_id", "assigned employee"), str("title", "task title"),
			enum("status", "task state", "open", "done")}},
		160, func(i int) map[string]any {
			return map[string]any{"project_id": 1 + r.IntN(15), "assignee_id": 1 + r.IntN(25),
				"title":  pick([]string{"Write spec", "Fix bug", "Review PR", "Migrate data", "Add tests"}) + " " + strconv.Itoa(i),
				"status": pick([]string{"open", "done"})}
		})
	add(&acmeEntity{Plural: "vendors", Singular: "vendor", Desc: "a supplier", Search: "name",
		Fields: []acmeField{id, str("name", "vendor name"), enum("country", "country code", "US", "DE", "JP", "BR")}},
		12, func(i int) map[string]any {
			return map[string]any{"name": acmeAdj[(i+3)%len(acmeAdj)] + " Supply " + strconv.Itoa(i), "country": pick([]string{"US", "DE", "JP", "BR"})}
		})
	add(&acmeEntity{Plural: "purchase_orders", Singular: "purchase_order", Desc: "an order Acme places with a vendor", Search: "id",
		Fields: []acmeField{id, fk("vendor_id", "vendor"), num("amount", "amount in whole dollars"),
			enum("status", "approval state", "draft", "approved", "rejected")}},
		70, func(i int) map[string]any {
			return map[string]any{"vendor_id": 1 + r.IntN(12), "amount": 100 + r.IntN(4900), "status": pick([]string{"draft", "approved", "approved", "rejected"})}
		})
	add(&acmeEntity{Plural: "shipments", Singular: "shipment", Desc: "a parcel for an order", Search: "id",
		Fields: []acmeField{id, fk("order_id", "order"), enum("carrier", "carrier", "UPS", "DHL", "FedEx"),
			enum("status", "delivery state", "label", "in_transit", "delivered")}},
		140, func(i int) map[string]any {
			return map[string]any{"order_id": 1 + r.IntN(200), "carrier": pick([]string{"UPS", "DHL", "FedEx"}),
				"status": pick([]string{"label", "in_transit", "delivered"})}
		})
	add(&acmeEntity{Plural: "warehouses", Singular: "warehouse", Desc: "a stock location", Search: "name",
		Fields: []acmeField{id, str("name", "warehouse name"), enum("region", "region", acmeRegions...)}},
		6, func(i int) map[string]any {
			return map[string]any{"name": acmeRegions[(i-1)%len(acmeRegions)] + " DC " + strconv.Itoa(i), "region": acmeRegions[(i-1)%len(acmeRegions)]}
		})
	add(&acmeEntity{Plural: "campaigns", Singular: "campaign", Desc: "a marketing campaign", Search: "name",
		Fields: []acmeField{id, str("name", "campaign name"), enum("channel", "channel", "email", "search", "social"),
			num("budget", "budget in whole dollars")}},
		20, func(i int) map[string]any {
			return map[string]any{"name": "Campaign " + strconv.Itoa(i), "channel": pick([]string{"email", "search", "social"}), "budget": 1000 + r.IntN(9000)}
		})
	return d
}

// --- tools -------------------------------------------------------------------

// AcmeTool is one tool: its MCP listing and the function serving it.
type AcmeTool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	call        func(args map[string]any) (any, error)
}

// Tools returns the 102 tools in a stable order.
func (d *AcmeData) Tools() []AcmeTool {
	var tools []AcmeTool
	for _, e := range d.Entities {
		tools = append(tools, e.tools()...)
	}
	return tools
}

func schemaOf(props map[string]any, required ...string) json.RawMessage {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	b, _ := json.Marshal(s)
	return b
}

func propOf(f acmeField) map[string]any {
	p := map[string]any{"type": f.Type, "description": f.Desc}
	if len(f.Enum) > 0 {
		p["enum"] = f.Enum
	}
	return p
}

func (e *acmeEntity) tools() []AcmeTool {
	idProp := map[string]any{"id": map[string]any{"type": "integer", "description": "the " + e.Singular + " id"}}

	listProps := map[string]any{
		"page":      map[string]any{"type": "integer", "description": "1-based page number (default 1)"},
		"page_size": map[string]any{"type": "integer", "description": fmt.Sprintf("rows per page, 1-%d (default 20)", acmePageMax)},
	}
	var filters []string
	for _, f := range e.Fields {
		if f.Filter {
			listProps[f.Name] = propOf(f)
			filters = append(filters, f.Name)
		}
	}
	listDesc := fmt.Sprintf("List %s (%s). Paginated: returns {items, page, page_size, total, has_more}.", e.Plural, e.Desc)
	if len(filters) > 0 {
		listDesc += " Optional exact-match filters: " + strings.Join(filters, ", ") + "."
	}

	writeProps := map[string]any{}
	var writeRequired []string
	for _, f := range e.Fields {
		if f.Name != "id" {
			writeProps[f.Name] = propOf(f)
			writeRequired = append(writeRequired, f.Name)
		}
	}
	updateProps := map[string]any{"id": idProp["id"]}
	for k, v := range writeProps {
		updateProps[k] = v
	}

	return []AcmeTool{
		{Name: "list_" + e.Plural, Description: listDesc, InputSchema: schemaOf(listProps), call: e.list},
		{Name: "get_" + e.Singular, Description: "Get one " + e.Singular + " by id.", InputSchema: schemaOf(idProp, "id"), call: e.get},
		{Name: "search_" + e.Plural, Description: fmt.Sprintf("Search %s whose %s contains the query (case-insensitive). Returns at most 10.", e.Plural, e.Search),
			InputSchema: schemaOf(map[string]any{"query": map[string]any{"type": "string", "description": "text to find"}}, "query"), call: e.search},
		{Name: "create_" + e.Singular, Description: "Create " + article(e.Singular) + " " + e.Singular + ". Returns the new row.", InputSchema: schemaOf(writeProps, writeRequired...), call: e.create},
		{Name: "update_" + e.Singular, Description: "Update fields of " + article(e.Singular) + " " + e.Singular + ". Returns the updated row.", InputSchema: schemaOf(updateProps, "id"), call: e.update},
		{Name: "delete_" + e.Singular, Description: "Delete a " + e.Singular + " by id.", InputSchema: schemaOf(idProp, "id"), call: e.del},
	}
}

func article(noun string) string {
	if strings.ContainsRune("aeiou", rune(noun[0])) {
		return "an"
	}
	return "a"
}

func intArg(args map[string]any, name string, def int) (int, error) {
	v, ok := args[name]
	if !ok || v == nil {
		return def, nil
	}
	switch x := v.(type) {
	case json.Number:
		n, err := strconv.Atoi(string(x))
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer, got %s", name, x)
		}
		return n, nil
	case string:
		n, err := strconv.Atoi(x)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer, got %q", name, x)
		}
		return n, nil
	}
	return 0, fmt.Errorf("%s must be an integer", name)
}

func (e *acmeEntity) list(args map[string]any) (any, error) {
	page, err := intArg(args, "page", 1)
	if err != nil {
		return nil, err
	}
	size, err := intArg(args, "page_size", 20)
	if err != nil {
		return nil, err
	}
	if page < 1 || size < 1 || size > acmePageMax {
		return nil, fmt.Errorf("page must be >= 1 and page_size 1-%d", acmePageMax)
	}
	var rows []map[string]any
	for _, row := range e.Rows {
		keep := true
		for name, want := range args {
			f := e.field(name)
			if f == nil || !f.Filter || want == nil {
				continue
			}
			if fmt.Sprint(row[name]) != fmt.Sprint(want) {
				keep = false
				break
			}
		}
		if keep {
			rows = append(rows, row)
		}
	}
	for name := range args {
		if name != "page" && name != "page_size" && (e.field(name) == nil || !e.field(name).Filter) {
			return nil, fmt.Errorf("unknown filter %q for list_%s", name, e.Plural)
		}
	}
	start := min((page-1)*size, len(rows))
	end := min(start+size, len(rows))
	items := rows[start:end]
	if items == nil {
		items = []map[string]any{}
	}
	return map[string]any{"items": items, "page": page, "page_size": size, "total": len(rows), "has_more": end < len(rows)}, nil
}

func (e *acmeEntity) row(args map[string]any) (map[string]any, error) {
	id, err := intArg(args, "id", 0)
	if err != nil {
		return nil, err
	}
	if id < 1 || id > len(e.Rows) {
		return nil, fmt.Errorf("%s %d not found", e.Singular, id)
	}
	return e.Rows[id-1], nil
}

func (e *acmeEntity) get(args map[string]any) (any, error) { return e.row(args) }

func (e *acmeEntity) search(args map[string]any) (any, error) {
	q, _ := args["query"].(string)
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil, fmt.Errorf("query is required")
	}
	out := []map[string]any{}
	for _, row := range e.Rows {
		if strings.Contains(strings.ToLower(fmt.Sprint(row[e.Search])), q) {
			out = append(out, row)
			if len(out) == 10 {
				break
			}
		}
	}
	return out, nil
}

// create, update and delete validate and echo but never change the data, so
// the read-only tasks stay deterministic whatever the model does.
func (e *acmeEntity) create(args map[string]any) (any, error) {
	out := map[string]any{"id": len(e.Rows) + 1}
	for _, f := range e.Fields {
		if f.Name == "id" {
			continue
		}
		v, ok := args[f.Name]
		if !ok {
			return nil, fmt.Errorf("%s is required", f.Name)
		}
		out[f.Name] = v
	}
	return out, nil
}

func (e *acmeEntity) update(args map[string]any) (any, error) {
	row, err := e.row(args)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for k, v := range row {
		out[k] = v
	}
	for k, v := range args {
		if k != "id" && e.field(k) != nil {
			out[k] = v
		}
	}
	return out, nil
}

func (e *acmeEntity) del(args map[string]any) (any, error) {
	row, err := e.row(args)
	if err != nil {
		return nil, err
	}
	return map[string]any{"deleted": row["id"]}, nil
}

// Call runs one tool with JSON arguments and returns its JSON text output, or
// an error message with isError set.
func (d *AcmeData) Call(name string, argsJSON []byte) (text string, isError bool) {
	var tool *AcmeTool
	for _, t := range d.Tools() {
		if t.Name == name {
			tool = &t
			break
		}
	}
	if tool == nil {
		return "unknown tool " + strconv.Quote(name), true
	}
	args := map[string]any{}
	if len(strings.TrimSpace(string(argsJSON))) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(argsJSON)))
		dec.UseNumber()
		if err := dec.Decode(&args); err != nil {
			return "arguments must be a JSON object: " + err.Error(), true
		}
	}
	res, err := tool.call(args)
	if err != nil {
		return err.Error(), true
	}
	b, _ := json.Marshal(res)
	return string(b), false
}

// OpenAITools returns the tools as OpenAI function schemas: the discrete arm,
// where every tool sits in the prompt of every request.
func (d *AcmeData) OpenAITools() []json.RawMessage {
	var out []json.RawMessage
	for _, t := range d.Tools() {
		b, _ := json.Marshal(map[string]any{"type": "function", "function": map[string]any{
			"name": t.Name, "description": t.Description, "parameters": t.InputSchema}})
		out = append(out, b)
	}
	return out
}

// sortedKeys returns m's keys in order, for deterministic tie checks.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
