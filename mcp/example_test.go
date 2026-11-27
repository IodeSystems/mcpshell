package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// A Go program registers its own functions and evaluates code that calls them.
func ExampleInstallToolkit() {
	stock := map[string]int{"A1": 3, "B2": 5}

	sh := toolkit.InstallCore(runtime.NewShell())
	err := mcp.InstallToolkit(sh, mcp.Toolkit{
		Namespace: "Inventory",
		Summary:   "stock levels",
		Functions: []mcp.Function{{
			Name:        "stock",
			Description: "Units in stock for a SKU.",
			InputSchema: json.RawMessage(`{"type": "object",
				"properties": {"sku": {"type": "string"}}, "required": ["sku"]}`),
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				sku := args["sku"].(string)
				n, ok := stock[sku]
				if !ok {
					return nil, fmt.Errorf("unknown SKU %q", sku)
				}
				return map[string]any{"sku": sku, "units": n}, nil
			},
		}},
	})
	if err != nil {
		panic(err)
	}

	v, err := sh.Eval(`["A1", "B2"] |> map(s => Inventory.stock({sku: s}).units) |> sum()`)
	fmt.Println(v.Display(), err)

	v, err = sh.Eval(`let r = ""; try { Inventory.stock("Z9") } catch (e) { r = e.split("\n")[0] }; r`)
	fmt.Println(v.Display(), err)

	_, err = sh.Eval(`Inventory.stock({})`)
	fmt.Println(err != nil)
	// Output:
	// 8 <nil>
	// Inventory.stock failed: unknown SKU "Z9" <nil>
	// true
}

// The same toolkit served as an MCP server on stdio; AddToolkit also lists
// the functions in the eval tool description.
func ExampleServer_AddToolkit() {
	sh := toolkit.InstallCore(runtime.NewShell())
	if err := sh.Limits().Configure("", "timeout=2m"); err != nil {
		panic(err)
	}
	srv := mcp.NewServer(sh, 0)
	err := srv.AddToolkit(mcp.Toolkit{
		Namespace: "Clock",
		Summary:   "the host's clock",
		Functions: []mcp.Function{{
			Name: "zone",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				return "UTC", nil
			},
		}},
	})
	if err != nil {
		panic(err)
	}
	if err := srv.RunStdio(os.Stdin, os.Stdout); err != nil {
		panic(err)
	}
}
