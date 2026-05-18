package main

import (
	"context"
	"fmt"
	"os"
	"github.com/emaharmony/prism/internal/approval"
	"github.com/emaharmony/prism/internal/mutation"
)

func executeApprovalList(runID, runsDir string) {
	store := approval.NewStore(runsDir)

	var approvals []*approval.Approval
	var err error

	if runID != "" {
		approvals, err = store.List(runID)
	} else {
		// List all approvals across all runs
		entries, readErr := os.ReadDir(runsDir)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to read runs directory: %v\n", readErr)
			os.Exit(1)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				runApprovals, listErr := store.List(entry.Name())
				if listErr == nil {
					approvals = append(approvals, runApprovals...)
				}
			}
		}
		err = nil
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(approvals) == 0 {
		fmt.Println("No approvals found.")
		return
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Prism V4 Approvals")
	fmt.Println("═══════════════════════════════════════════")
	for _, a := range approvals {
		fmt.Printf("  %s\n", a.ApprovalID)
		fmt.Printf("    Status:       %s\n", a.Status)
		fmt.Printf("    Type:         %s\n", a.MutationType)
		fmt.Printf("    Target:       %s\n", a.TargetPath)
		fmt.Printf("    Requested by: %s\n", a.RequestedBy)
		fmt.Printf("    Run:          %s\n", a.RunID)
		if a.Status == approval.StatusApproved {
			fmt.Printf("    Approved by:  %s at %v\n", a.ApprovedBy, a.ApprovedAt)
		} else if a.Status == approval.StatusDenied {
			fmt.Printf("    Denied by:    %s\n", a.DeniedBy)
			if a.DenialReason != "" {
				fmt.Printf("    Reason:       %s\n", a.DenialReason)
			}
		}
		fmt.Println()
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeApprovalShow(approvalID, runID, runsDir string) {
	if runID == "" {
		fmt.Fprintln(os.Stderr, "Error: --run flag is required for show")
		os.Exit(1)
	}
	if approvalID == "" || approvalID == "show" {
		fmt.Fprintln(os.Stderr, "Error: approval ID required")
		os.Exit(1)
	}

	store := approval.NewStore(runsDir)

	a, err := store.Load(runID, approvalID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Approval: %s\n", a.ApprovalID)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Status:       %s\n", a.Status)
	fmt.Printf("  Type:         %s\n", a.MutationType)
	fmt.Printf("  Target:       %s\n", a.TargetPath)
	fmt.Printf("  Run:          %s\n", a.RunID)
	fmt.Printf("  Correlation:  %s\n", a.CorrelationID)
	fmt.Printf("  Requested by: %s\n", a.RequestedBy)
	fmt.Printf("  Project:      %s\n", a.Project)
	fmt.Printf("  Policy:       %s (%s)\n", a.Policy.Decision, a.Policy.Reason)
	fmt.Printf("  Created:      %v\n", a.CreatedAt)
	if a.Preview != "" {
		fmt.Printf("  Preview:\n%s\n", a.Preview)
	}
	if a.Status == approval.StatusApproved {
		fmt.Printf("  Approved by:  %s at %v\n", a.ApprovedBy, a.ApprovedAt)
	}
	if a.Status == approval.StatusDenied {
		fmt.Printf("  Denied by:    %s at %v\n", a.DeniedBy, a.DeniedAt)
		if a.DenialReason != "" {
			fmt.Printf("  Reason:       %s\n", a.DenialReason)
		}
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeApprovalApprove(approvalID, approvedBy, runID, workspace, runsDir string) {
	if approvedBy == "" {
		fmt.Fprintln(os.Stderr, "Error: --by flag is required")
		os.Exit(1)
	}
	if runID == "" {
		fmt.Fprintln(os.Stderr, "Error: --run flag is required")
		os.Exit(1)
	}

	store := approval.NewStore(runsDir)
	executor := mutation.NewExecutor(workspace, store)

	// We'll emit events via print
	executor.SetEmitter(func(eventType, source string, payload map[string]any) {
		fmt.Printf("  💎 [%s] ", eventType)
		if id, ok := payload["approval_id"].(string); ok {
			fmt.Printf("approval=%s ", id)
		}
		if tp, ok := payload["target_path"].(string); ok {
			fmt.Printf("target=%s", tp)
		}
		fmt.Println()
	})

	result, err := executor.ApplyWithRun(context.Background(), runID, approvalID, approvedBy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	if result.Success {
		fmt.Println("  ✅ Mutation Applied Successfully")
	} else {
		fmt.Println("  ❌ Mutation Failed")
	}
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Approval ID: %s\n", result.ApprovalID)
	fmt.Printf("  Target Path: %s\n", result.TargetPath)
	if result.Message != "" {
		fmt.Printf("  Message:     %s\n", result.Message)
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeApprovalDeny(approvalID, deniedBy, reason, runID, runsDir string) {
	if deniedBy == "" {
		fmt.Fprintln(os.Stderr, "Error: --by flag is required")
		os.Exit(1)
	}
	if runID == "" {
		fmt.Fprintln(os.Stderr, "Error: --run flag is required")
		os.Exit(1)
	}

	store := approval.NewStore(runsDir)
	executor := mutation.NewExecutor(".", store)

	executor.SetEmitter(func(eventType, source string, payload map[string]any) {
		fmt.Printf("  💎 [%s] ", eventType)
		if id, ok := payload["approval_id"].(string); ok {
			fmt.Printf("approval=%s", id)
		}
		fmt.Println()
	})

	err := executor.DenyApproval(runID, approvalID, deniedBy, reason)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  ❌ Approval Denied")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Approval ID: %s\n", approvalID)
	fmt.Printf("  Denied By:   %s\n", deniedBy)
	if reason != "" {
		fmt.Printf("  Reason:      %s\n", reason)
	}
	fmt.Println("  (No files were written)")
	fmt.Println("═══════════════════════════════════════════")
}


// ── V7: Workflow CLI Functions ──────────────────────────────────────────────────

