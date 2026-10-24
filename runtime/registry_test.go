package runtime_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
)

func searchRegistry() *runtime.CommandRegistry {
	r := runtime.NewCommandRegistry()
	for _, c := range []struct{ name, desc string }{
		{"observability-list_datasources", "List datasources."},
		{"observability-check_datasources_health", "Check datasource health."},
		{"observability-query_prometheus", "Run a PromQL query."},
		{"alerting-create_contact_point", "Create a contact point. Long text that mentions query routing."},
		{"knowledge-search", "Search the knowledge base."},
	} {
		r.Register(&runtime.CommandDef{Namespace: "server", Name: c.name, Description: c.desc})
	}
	return r
}

func TestHelpSearchSplitsWords(t *testing.T) {
	out := searchRegistry().Help("observability-query_datasource")
	for _, want := range []string{"list_datasources", "check_datasources_health", "query_prometheus"} {
		if !strings.Contains(out, want) {
			t.Errorf("search should find %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "knowledge-search") {
		t.Errorf("search should not list knowledge-search:\n%s", out)
	}
}

func TestHelpSearchRanksNameAboveDescription(t *testing.T) {
	out := searchRegistry().Help("query")
	q, a := strings.Index(out, "query_prometheus"), strings.Index(out, "create_contact_point")
	if q < 0 || a < 0 || q > a {
		t.Errorf("a name match must rank above a description match:\n%s", out)
	}
	if strings.Contains(out, "Long text") {
		t.Errorf("results show one short line, not the full description:\n%s", out)
	}
}
