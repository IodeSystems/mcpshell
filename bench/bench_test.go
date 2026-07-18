package bench_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/bench"
	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.local")
	content := "# a comment\n\nMCPSHELL_TEST_URL = https://example.com \nMCPSHELL_TEST_MODEL=\"mpt\"\nMCPSHELL_TEST_PRESET=fromfile\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// A variable already in the environment must win over the file.
	t.Setenv("MCPSHELL_TEST_PRESET", "fromenv")
	t.Cleanup(func() {
		os.Unsetenv("MCPSHELL_TEST_URL")
		os.Unsetenv("MCPSHELL_TEST_MODEL")
	})

	if err := bench.LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if got := os.Getenv("MCPSHELL_TEST_URL"); got != "https://example.com" {
		t.Errorf("MCPSHELL_TEST_URL = %q", got)
	}
	if got := os.Getenv("MCPSHELL_TEST_MODEL"); got != "mpt" {
		t.Errorf("MCPSHELL_TEST_MODEL = %q (quotes should be stripped)", got)
	}
	if got := os.Getenv("MCPSHELL_TEST_PRESET"); got != "fromenv" {
		t.Errorf("MCPSHELL_TEST_PRESET = %q, want fromenv (env wins over file)", got)
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	if err := bench.LoadEnvFile(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Errorf("missing env file should not error, got %v", err)
	}
}

// fakeLLM serves a minimal OpenAI-compatible API: it returns a mcpshell tool call
// the first time, then a final answer once it sees a tool result.
func fakeLLM(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			io.WriteString(w, `{"data":[{"id":"mpt-test-7b"},{"id":"llama-3"}]}`)
			return
		}
		var req struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		last := ""
		if n := len(req.Messages); n > 0 {
			last = req.Messages[n-1].Role
		}
		if last == "tool" {
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"final answer"}}]}`)
		} else {
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","tool_calls":`+
				`[{"id":"c1","type":"function","function":{"name":"mcpshell","arguments":"{\"code\":\"2 + 3\"}"}}]}}]}`)
		}
	}))
}

func TestResolveModel(t *testing.T) {
	srv := fakeLLM(t)
	defer srv.Close()
	llm := bench.NewLLM(srv.URL, "")

	got, err := llm.ResolveModel(context.Background(), "mpt")
	if err != nil {
		t.Fatalf("ResolveModel: %v", err)
	}
	if got != "mpt-test-7b" {
		t.Errorf("ResolveModel(\"mpt\") = %q, want mpt-test-7b", got)
	}
	if _, err := llm.ResolveModel(context.Background(), "nonexistent"); err == nil {
		t.Errorf("expected error for an unknown model")
	}
}

func TestRunAgent(t *testing.T) {
	srv := fakeLLM(t)
	defer srv.Close()
	llm := bench.NewLLM(srv.URL, "")

	var ranCode string
	answer, attempts, stats, err := llm.RunAgent(context.Background(), "mpt-test-7b",
		"system", "compute 2+3",
		func(code string) string {
			ranCode = code
			return "5"
		}, 10)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if stats.Turns != 2 {
		t.Errorf("stats.Turns = %d, want 2 (one tool-call turn + one final)", stats.Turns)
	}
	if answer != "final answer" {
		t.Errorf("answer = %q, want %q", answer, "final answer")
	}
	if ranCode != "2 + 3" {
		t.Errorf("tool ran code %q, want %q", ranCode, "2 + 3")
	}
	if len(attempts) != 1 || attempts[0].Result != "5" {
		t.Errorf("attempts = %+v, want one attempt with result 5", attempts)
	}
}

func TestRunWritesResults(t *testing.T) {
	srv := fakeLLM(t)
	defer srv.Close()
	llm := bench.NewLLM(srv.URL, "")
	outDir := t.TempDir()

	factory := func() *runtime.Shell {
		return toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell()))
	}
	err := bench.Run(llm, "mpt-test-7b", factory, bench.Options{
		SystemPrompt: "system",
		OutputDir:    outDir,
		TimeoutSec:   10,
		MaxIters:     5,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "README.md")); err != nil {
		t.Errorf("index README.md not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "mpt-test-7b", "factorial.md")); err != nil {
		t.Errorf("per-teaser result not written: %v", err)
	}
}

func TestSuiteShape(t *testing.T) {
	if len(bench.Suite) != 52 {
		t.Errorf("suite has %d teasers, want 52", len(bench.Suite))
	}
	seen := map[string]bool{}
	for _, teaser := range bench.Suite {
		if teaser.Name == "" || teaser.Prompt == "" || teaser.Validate == nil {
			t.Errorf("teaser %q is incomplete", teaser.Name)
		}
		if seen[teaser.Name] {
			t.Errorf("duplicate teaser name %q", teaser.Name)
		}
		seen[teaser.Name] = true
	}
	// Spot-check a validator.
	fizz := bench.Suite[1]
	if !fizz.Validate("[1, 2, Fizz, 4, Buzz, FizzBuzz]") {
		t.Errorf("fizzbuzz validator should accept a valid answer")
	}
	if fizz.Validate("nope") {
		t.Errorf("fizzbuzz validator should reject a wrong answer")
	}
}
