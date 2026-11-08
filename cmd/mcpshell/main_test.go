package main

import (
	"slices"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
)

func TestTakeEvalFlags(t *testing.T) {
	evalOpts = runtime.EvalOptions{}
	rest := takeEvalFlags([]string{"--profile", "--limits", "steps=5000,timeout=60s", "1 + 1"})
	if !slices.Equal(rest, []string{"1 + 1"}) || !evalOpts.Profile {
		t.Fatalf("rest = %v, opts = %+v", rest, evalOpts)
	}
	if *evalOpts.Limits.Steps != 5000 || *evalOpts.Limits.TimeoutMs != 60000 {
		t.Errorf("limits = %+v", evalOpts.Limits)
	}
	evalOpts = runtime.EvalOptions{}
	rest = takeEvalFlags([]string{"--limits=steps=7", "-f", "x.mcpshell"})
	if !slices.Equal(rest, []string{"-f", "x.mcpshell"}) || *evalOpts.Limits.Steps != 7 {
		t.Errorf("rest = %v, limits = %+v", rest, evalOpts.Limits)
	}
	evalOpts = runtime.EvalOptions{}
	if rest = takeEvalFlags([]string{"mcp", "--profile"}); len(rest) != 2 || evalOpts.Profile {
		t.Errorf("flags after the first argument stay: %v", rest)
	}
}
