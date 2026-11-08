package mcp_test

import (
	"strings"
	"testing"
)

func TestLimitsArgMemory(t *testing.T) {
	texts, errs := evalCalls(t, coreShell(),
		codeArgs("limits().maxMemory", `"limits":{"memory":"512MB"}`),
		codeArgs("limits().maxMemory", `"limits":{"memory":1073741824}`),
		codeArgs("limits().maxMemory", ""),
		codeArgs("1", `"limits":{"memory":"64GB"}`),
		codeArgs("1", `"limits":{"memory":"lots"}`),
		codeArgs("1", `"limits":{"memory":-5}`),
	)
	if errs[0] || texts[0] != "536870912" {
		t.Errorf("size string: %q", texts[0])
	}
	if errs[1] || texts[1] != "1073741824" {
		t.Errorf("bytes: %q", texts[1])
	}
	if errs[2] || texts[2] != "1073741824" {
		t.Errorf("the next eval must have the default: %q", texts[2])
	}
	if !errs[3] || !strings.Contains(texts[3], "exceeds the operator maximum (2 GB)") || !strings.Contains(texts[3], "--max-limits memory") {
		t.Errorf("over the cap: %q", texts[3])
	}
	for i := 4; i < 6; i++ {
		if !errs[i] || !strings.Contains(texts[i], "memory") {
			t.Errorf("bad memory %d: %q", i, texts[i])
		}
	}
}

func TestMemoryCapFromOperator(t *testing.T) {
	sh := coreShell()
	if err := sh.Limits().ParseCaps("memory=4GB"); err != nil {
		t.Fatal(err)
	}
	texts, errs := evalCalls(t, sh, codeArgs("limits().maxMemory", `"limits":{"memory":"3GB"}`))
	if errs[0] || texts[0] != "3221225472" {
		t.Errorf("a raised cap lets the eval argument through: %q", texts[0])
	}
}

func TestMemoryErrorNamesEvalArgument(t *testing.T) {
	texts, errs := evalCalls(t, coreShell(),
		codeArgs(`"x".repeat(100000000).length`, `"limits":{"memory":"64MB"}`))
	if !errs[0] || !strings.Contains(texts[0], "memory limit exceeded") ||
		!strings.Contains(texts[0], `limits: {memory: "`) || !strings.Contains(texts[0], "operator maximum: 2 GB") {
		t.Errorf("the error must name the eval argument and the cap: %q", texts[0])
	}
}
