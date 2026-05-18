package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/emaharmony/prism/internal/tool"
)

func executeToolList() {
	registry := tool.NewRegistry()
	tool.RegisterBuiltinsV4(registry, ".", 1024*1024)

	names := registry.List()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Prism V4 Built-in Tools")
	fmt.Println("═══════════════════════════════════════════")
	for _, name := range names {
		t, err := registry.Resolve(name)
		if err != nil {
			fmt.Printf("  %-20s (error: %v)\n", name, err)
			continue
		}
		fmt.Printf("  %-20s %s\n", name, t.Description())
		schema := t.Schema()
		if len(schema.Input) > 0 {
			fmt.Println("    Input:")
			for paramName, spec := range schema.Input {
				req := ""
				if spec.Required {
					req = " (required)"
				}
				fmt.Printf("      %s: %s%s — %s\n", paramName, spec.Type, req, spec.Description)
			}
		}
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeToolRun(toolName, inputJSON, project, workspace string, maxFileSize int64) {
	// Parse input JSON
	var input map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid JSON input: %v\n", err)
		os.Exit(1)
	}

	// Set up registry and policy
	registry := tool.NewRegistry()
	tool.RegisterBuiltinsV4(registry, workspace, maxFileSize)
	policyConfig := tool.PolicyConfig{
		WorkspaceRoot: workspace,
		MaxFileSize:   maxFileSize,
	}
	executor := tool.NewExecutor(registry, policyConfig)

	fmt.Printf("Running tool %q with input: %s\n", toolName, inputJSON)

	result, err := executor.ExecuteWithPolicy(context.Background(), toolName, "prism-cli", project, "tool-cli-run", input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing tool: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	if result.Success {
		fmt.Println("  ✅ Tool Execution Succeeded")
	} else {
		fmt.Println("  ❌ Tool Execution Failed")
	}
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Tool:    %s\n", toolName)
	fmt.Printf("  Project: %s\n", project)
	if result.Error != "" {
		fmt.Printf("  Error:   %s\n", result.Error)
	}
	if len(result.Output) > 0 {
		fmt.Println("  Output:")
		outputData, _ := json.MarshalIndent(result.Output, "    ", "  ")
		fmt.Printf("    %s\n", string(outputData))
	}
	fmt.Println("═══════════════════════════════════════════")
}

