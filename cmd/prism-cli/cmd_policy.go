package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/emaharmony/prism/internal/policy"
)

func newPolicyEvaluator(policyDir string) *policy.Evaluator {
	reg := policy.NewRegistry()
	reg.LoadFromDir(policyDir) //nolint:errcheck // best-effort loading
	eval := policy.NewEvaluator(reg)
	return eval
}

func executePolicyList() {
	reg := policy.NewRegistry()
	count, err := reg.LoadFromDir("policies")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	rules := reg.Rules()

	fmt.Println("\U0001F4CB Prism V8 Policy Rules")
	fmt.Println("═══════════════════════════════════════════")
	if len(rules) == 0 {
		fmt.Println("  (no policies loaded)")
	}
	for _, rule := range rules {
		decisionIcon := "\u2705"
		switch rule.Decision {
		case policy.DecisionDenied:
			decisionIcon = "\u274C"
		case policy.DecisionRequiresApproval:
			decisionIcon = "\u26A0\uFE0F"
		}
		fmt.Printf("  %s %-35s %s\n", decisionIcon, rule.ID, rule.Decision)
		fmt.Printf("     %s\n", rule.Description)
	}
	fmt.Println("═══════════════════════════════════════════")
	if count > 0 {
		fmt.Printf("  %d rules loaded\n", count)
	}
}

func executePolicyEvaluate(inputFile, policyDir string) {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to read input file: %v\n", err)
		os.Exit(1)
	}

	var req policy.PolicyRequest
	if err := json.Unmarshal(data, &req); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid JSON input: %v\n", err)
		os.Exit(1)
	}

	evaluator := newPolicyEvaluator(policyDir)

	fmt.Printf("\U0001F50D Policy Evaluation\n")
	fmt.Printf("  Action:   %s\n", req.Action)
	fmt.Printf("  Resource: %s/%s\n", req.Resource.Type, req.Resource.Name)
	fmt.Printf("  Subject:  %s/%s\n", req.Subject.Type, req.Subject.ID)
	if req.Context.Mode != "" {
		fmt.Printf("  Mode:     %s\n", req.Context.Mode)
	}
	fmt.Println()

	decision := evaluator.Evaluate(req)

	switch decision.Decision {
	case policy.DecisionAllowed:
		fmt.Println("  \u2705 ALLOWED")
	case policy.DecisionDenied:
		fmt.Println("  \u274C DENIED")
	case policy.DecisionRequiresApproval:
		fmt.Println("  \u26A0\uFE0F  REQUIRES APPROVAL")
	default:
		fmt.Printf("  \u2753 %s\n", decision.Decision)
	}
	fmt.Println()
	fmt.Printf("  Rule:    %s\n", decision.RuleID)
	fmt.Printf("  Reason:  %s\n", decision.Reason)
	if decision.Severity != "" {
		fmt.Printf("  Severity: %s\n", decision.Severity)
	}
	fmt.Println()

	// Write artifact
	artifactDir := filepath.Join("runs", "policy")
	if err := policy.WritePolicyArtifact(artifactDir, decision); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write policy artifact: %v\n", err)
	} else {
		fmt.Printf("  Artifact: %s/%s.json\n", artifactDir, decision.EvaluationID)
	}

	if decision.Decision == policy.DecisionDenied {
		os.Exit(1)
	}
}

