package mcp_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/toolkit"
)

// The Db database lasts the MCP session: load in one eval, query in later ones.
func TestDbPersistsAcrossEvals(t *testing.T) {
	sh := coreShell()
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	codes := []string{
		`Db.tables()`,
		`load("db")`,
		`Db.load("sales", [{region: "W", amount: 0.1}, {region: "W", amount: 0.2}, {region: "E", amount: 5}], {exact: ["amount"]}).rows`,
		`Db.query("SELECT region, sum(amount) AS total FROM sales GROUP BY region ORDER BY region")`,
		`Db.exec("UPDATE sales SET amount = 1 WHERE region = 'E'").affected`,
		`Db.query("SELECT sum(amount) AS t FROM sales")[0].t`,
	}
	var lines []string
	for i, c := range codes {
		b, _ := json.Marshal(c)
		lines = append(lines, `{"jsonrpc":"2.0","id":`+string(rune('1'+i))+`,"method":"tools/call","params":{"name":"eval","arguments":{"code":`+string(b)+`}}}`)
	}
	var out bytes.Buffer
	if err := mcp.NewServer(sh, 16000).RunStdio(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	resp := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(resp) != len(codes) {
		t.Fatalf("got %d responses", len(resp))
	}
	text := func(i int) string {
		var r struct {
			Result struct {
				IsError bool `json:"isError"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(resp[i]), &r); err != nil {
			t.Fatal(err)
		}
		if i == 0 != r.Result.IsError {
			t.Errorf("eval %d: isError = %v", i, r.Result.IsError)
		}
		return r.Result.Content[0].Text
	}
	if !strings.Contains(text(0), "call load(") {
		t.Errorf("before load: %s", text(0))
	}
	if !strings.Contains(text(1), "Db.query") {
		t.Errorf("load: %s", text(1))
	}
	for i, want := range map[int]string{
		2: "3",
		4: "1",
		5: "1.3",
	} {
		if got := text(i); got != want {
			t.Errorf("eval %d = %q, want %q", i, got, want)
		}
	}
	if got := text(3); !strings.Contains(got, `region: "E"`) || !strings.Contains(got, "total: 0.3") {
		t.Errorf("group by: %s", got)
	}
}
