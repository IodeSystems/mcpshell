package mcp_test

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
)

// evalCalls runs each arguments object as an eval call on one server and
// returns the text and isError of each result.
func evalCalls(t *testing.T, sh *runtime.Shell, calls ...string) (texts []string, errs []bool) {
	t.Helper()
	var in []string
	for i, c := range calls {
		in = append(in, `{"jsonrpc":"2.0","id":`+string(rune('1'+i))+`,"method":"tools/call","params":{"name":"eval","arguments":`+c+`}}`)
	}
	var out bytes.Buffer
	if err := mcp.NewServer(sh, 16000).RunStdio(strings.NewReader(strings.Join(in, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		texts = append(texts, toolText(t, m))
		errs = append(errs, m["result"].(map[string]any)["isError"].(bool))
	}
	return texts, errs
}

func codeArgs(code string, extra string) string {
	b, _ := json.Marshal(code)
	if extra != "" {
		extra = "," + extra
	}
	return `{"code":` + string(b) + extra + `}`
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// smallShell has a 1000-step default and a 20000-step maximum, so tests hit
// limits in a few thousand steps.
func smallShell() *runtime.Shell {
	sh := coreShell()
	sh.Limits().SetDefaults(1000, 256, 30_000)
	sh.Limits().SetCaps(20_000, 0, 0, 0)
	return sh
}

func noteShell() *runtime.Shell {
	sh := coreShell()
	sh.Register(&runtime.CommandDef{Name: "noteme", Fn: func([]runtime.Value) runtime.Value {
		sh.Note(`db 212/256 MB — Db.drop(t) or unload("db") frees it`)
		return runtime.Null
	}})
	return sh
}

const loop = `let i = 0; while (i < %N%) { i = i + 1 }; i`

func loopCode(n string) string { return strings.Replace(loop, "%N%", n, 1) }

func TestEvalSchemaHasProfileAndLimits(t *testing.T) {
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"eval","arguments":{"code":"1","profile":true,"limits":{"steps":5000}}}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}` + "\n"
	var out bytes.Buffer
	if err := mcp.NewServer(coreShell(), 16000).RunStdio(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d responses", len(lines))
	}
	for _, want := range []string{`"profile":{"type":"boolean"`, `"limits":{"type":"object"`, `"steps":{"type":"integer"}`, `"outputBytes"`, `"memory":{"type":["integer","string"]`} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("tools/list lacks %s: %s", want, lines[0])
		}
	}
	if lines[0] != strings.Replace(lines[2], `"id":3`, `"id":1`, 1) {
		t.Errorf("tools/list changed after eval calls with profile and limits")
	}
}

func TestDiagnosticsBlockHasWarningNoteAndProfile(t *testing.T) {
	texts, errs := evalCalls(t, noteShell(),
		codeArgs("let o = {name: 1}; o.nmae\nnoteme()\nnoteme()\nlet s = 0\nfor (let i = 0; i < 50; i++) { s += i }\ns", `"profile":true`))
	if errs[0] {
		t.Fatalf("error: %s", texts[0])
	}
	got := texts[0]
	i := strings.Index(got, "\n\nwarning: line 1: ")
	j := strings.Index(got, "\nnote: db 212/256 MB")
	k := strings.Index(got, "\nprofile: ")
	if !strings.HasPrefix(got, "1225") || i < 0 || j < i || k < j {
		t.Fatalf("want result, then one block of warning, note, profile lines:\n%s", got)
	}
	if strings.Count(got, "note:") != 1 {
		t.Errorf("a repeated note must appear once:\n%s", got)
	}
	if strings.Count(got, "\n\n") != 1 {
		t.Errorf("one blank line before the block, none inside:\n%s", got)
	}
	// The loop on line 5 costs the most.
	if !strings.Contains(got, "\nprofile: line 5: ") || !strings.Contains(got, "%)") {
		t.Errorf("profile lines lack the hot line with a percentage:\n%s", got)
	}
	if !regexp.MustCompile(`profile: \d+ steps\n`).MatchString(got + "\n") {
		t.Errorf("profile lacks the step total:\n%s", got)
	}
}

func TestNoBlockWhenNothingToReport(t *testing.T) {
	texts, _ := evalCalls(t, noteShell(), codeArgs("1 + 2", ""))
	if texts[0] != "3" {
		t.Errorf("got %q, want just the value", texts[0])
	}
}

func TestNoteAfterErrorToo(t *testing.T) {
	texts, errs := evalCalls(t, noteShell(), codeArgs("noteme()\nnope()", ""))
	if !errs[0] || !strings.Contains(texts[0], "\n\nnote: db 212/256 MB") {
		t.Errorf("an error result carries the block too: %s", texts[0])
	}
}

func TestLimitsArgRaisesOneEvalOnly(t *testing.T) {
	texts, errs := evalCalls(t, smallShell(),
		codeArgs(loopCode("5000"), `"limits":{"steps":9000}`),
		codeArgs(loopCode("5000"), ""),
		codeArgs(loopCode("5000"), `"limits":{"steps":50000}`), // over the maximum
	)
	if errs[0] || !strings.HasPrefix(texts[0], "5000") {
		t.Fatalf("with limits.steps the loop must run: %s", texts[0])
	}
	if !errs[1] || !strings.Contains(texts[1], "step limit exceeded (1000 steps)") {
		t.Errorf("the raise must not last into the next eval: %s", texts[1])
	}
	if !errs[2] || !strings.Contains(texts[2], "exceeds the operator maximum (20000)") || !strings.Contains(texts[2], "--max-limits steps=N") {
		t.Errorf("over the maximum must be an error naming it: %s", texts[2])
	}
}

func TestLimitsArgCanLower(t *testing.T) {
	texts, errs := evalCalls(t, coreShell(), codeArgs(loopCode("5000"), `"limits":{"steps":1000}`))
	if !errs[0] || !strings.Contains(texts[0], "step limit exceeded (1000 steps)") {
		t.Errorf("a lower limit applies: %s", texts[0])
	}
}

func TestLimitsArgRejectsBadInput(t *testing.T) {
	for _, bad := range []string{`{"steps":"many"}`, `{"stepz":5}`, `{"steps":-5}`, `{"steps":1.5}`} {
		texts, errs := evalCalls(t, coreShell(), codeArgs("1", `"limits":`+bad))
		if !errs[0] || !strings.Contains(texts[0], "limits") {
			t.Errorf("limits %s: want an error, got %q", bad, texts[0])
		}
	}
	texts, errs := evalCalls(t, coreShell(), codeArgs("1", `"limits":{"timeout":"60s"}`))
	if errs[0] {
		t.Errorf("a duration string for timeout: %s", texts[0])
	}
}

func TestLimitHitSuggestsEvalArgument(t *testing.T) {
	texts, errs := evalCalls(t, smallShell(),
		codeArgs(loopCode("1500"), ""),
		codeArgs("let f = n => f(n + 1)\nf(0)", ""),
	)
	if !errs[0] || !strings.Contains(texts[0], "re-run with the eval argument limits: {steps: 5000}") ||
		strings.Contains(texts[0], "extendLimit") || strings.Contains(texts[0], "profile(() =>") ||
		!strings.Contains(texts[0], "eval argument profile: true") {
		t.Errorf("step-limit text: %s", texts[0])
	}
	if !errs[1] || !strings.Contains(texts[1], "limits: {callDepth: 512}") || strings.Contains(texts[1], "extendLimit") {
		t.Errorf("call-depth text: %s", texts[1])
	}
	if !strings.Contains(texts[0], "\n\nlimits: steps 1000/1000 (hit)") {
		t.Errorf("a hit limit gets a limits line: %s", texts[0])
	}
}

func TestNearLimitLine(t *testing.T) {
	texts, errs := evalCalls(t, coreShell(), codeArgs(loopCode("10"), ""))
	if errs[0] || strings.Contains(texts[0], "limits:") {
		t.Errorf("a small eval has no limits line: %s", texts[0])
	}
	// Measure the loop's steps, then size the limit so it uses about 85%.
	texts, errs = evalCalls(t, coreShell(), codeArgs(loopCode("1000"), `"profile":true`))
	var steps int
	for line := range strings.SplitSeq(texts[0], "\n") {
		if n, ok := strings.CutPrefix(line, "profile: "); ok && strings.HasSuffix(n, " steps") {
			steps = mustAtoi(t, strings.TrimSuffix(n, " steps"))
		}
	}
	if errs[0] || steps == 0 {
		t.Fatalf("no step total: %s", texts[0])
	}
	limit := steps * 100 / 85
	arg := `"limits":{"steps":` + strconv.Itoa(limit) + `}`
	texts, errs = evalCalls(t, coreShell(), codeArgs(loopCode("1000"), arg))
	if errs[0] || !strings.Contains(texts[0], "\n\nlimits: steps ") || !strings.Contains(texts[0], "/"+strconv.Itoa(limit)+" (8") {
		t.Errorf("near the limit, the result states used/limit: %s", texts[0])
	}
}

func TestHiddenCommandsStayCallable(t *testing.T) {
	texts, errs := evalCalls(t, coreShell(),
		codeArgs(`help()`, ""),
		codeArgs(`limits().maxSteps`, ""),
		codeArgs(`extendLimit({steps: 2000000}); limits().maxSteps`, ""),
		codeArgs(`profile(() => 1 + 1).result`, ""),
		codeArgs(`profile(() => 1 + 1).steps`, `"profile":true`),
	)
	for _, name := range []string{"profile", "limits", "extendLimit"} {
		if strings.Contains(texts[0], name) {
			t.Errorf("help() lists %s", name)
		}
	}
	if texts[1] != "1000000" || texts[2] != "2000000" || strings.HasPrefix(texts[3], "ERROR") || errs[4] {
		t.Errorf("hidden commands must work: %q %q %q %q", texts[1], texts[2], texts[3], texts[4])
	}
}

// Clients send integers as floats (Python json gives 9000.0 and 1e12).
func TestLimitsArgAcceptsIntegralFloats(t *testing.T) {
	texts, errs := evalCalls(t, smallShell(),
		codeArgs(loopCode("5000"), `"limits":{"steps":9000.0}`),
		codeArgs("1", `"limits":{"steps":1e12}`),
		codeArgs("1", `"limits":{"steps":1e400}`),
		codeArgs("1", `"limits":{"timeout":5000.0}`),
	)
	if errs[0] || !strings.HasPrefix(texts[0], "5000") {
		t.Errorf("steps 9000.0 must apply: %s", texts[0])
	}
	for i := 1; i <= 2; i++ {
		if !errs[i] || !strings.Contains(texts[i], "exceeds the operator maximum") {
			t.Errorf("call %d: a huge value must name the operator maximum: %s", i, texts[i])
		}
	}
	if errs[3] {
		t.Errorf("timeout 5000.0: %s", texts[3])
	}
}
