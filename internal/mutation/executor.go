// Package mutation implements Prism V4's approval-gated mutation execution.
// After an approval is granted, the mutation executor applies the approved
// file change, with comprehensive safety checks before writing any file.
package mutation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emaharmony/prism/internal/approval"
	"github.com/emaharmony/prism/internal/safety"
)

// MaxContentSize is the maximum allowed content size (1MB).
const MaxContentSize = 1024 * 1024

// EmitFunc is a callback for emitting events.
type EmitFunc func(eventType, source string, payload map[string]any)

// Executor applies approved file mutations with safety checks.
type Executor struct {
	workspaceRoot string
	allowedPaths  []string
	approvalStore *approval.Store
	emit          EmitFunc
}

// NewExecutor creates a new mutation executor.
func NewExecutor(workspaceRoot string, approvalStore *approval.Store, allowedPaths ...string) *Executor {
	return &Executor{
		workspaceRoot: workspaceRoot,
		allowedPaths:  allowedPaths,
		approvalStore: approvalStore,
	}
}

// SetEmitter sets the event emission callback.
func (e *Executor) SetEmitter(emit EmitFunc) {
	e.emit = emit
}

// MutationResult captures the result of applying a mutation.
type MutationResult struct {
	Success    bool   `json:"success"`
	ApprovalID string `json:"approval_id"`
	TargetPath string `json:"target_path"`
	Message    string `json:"message,omitempty"`
}

// Apply executes the mutation for a granted approval.
// It performs comprehensive safety checks before any file write.
func (e *Executor) Apply(ctx context.Context, approvalID string, approvedBy string) (*MutationResult, error) {
	// We can't load without runID yet — we'll scan for it or the caller will
	// provide it. For now we need the runID to look up the approval.
	// Since the CLI provides the runID context, let's accept it differently.
	// Actually, the approval ID is a ULID, we should have the runID too.
	// Let's require the runID to be derivable or passed in.
	// For now, return an error requiring runID context.
	return nil, fmt.Errorf("runID required — use ApplyWithRun")
}

// ApplyWithRun executes the mutation for a granted approval with explicit runID.
func (e *Executor) ApplyWithRun(ctx context.Context, runID, approvalID, approvedBy string) (*MutationResult, error) {
	// 1. Load the approval
	a, err := e.approvalStore.Load(runID, approvalID)
	if err != nil {
		e.emitEvent("prism.mutation.failed", map[string]any{
			"approval_id": approvalID,
			"run_id":      runID,
			"error":       "approval not found: " + err.Error(),
		})
		return &MutationResult{Success: false, ApprovalID: approvalID, Message: "approval not found"}, nil
	}

	// 2. Status check: must be pending (we'll approve first) or already approved
	if a.Status != approval.StatusPending && a.Status != approval.StatusApproved {
		e.emitEvent("prism.mutation.failed", map[string]any{
			"approval_id":     approvalID,
			"run_id":          runID,
			"mutation_type":   a.MutationType,
			"target_path":     a.TargetPath,
			"correlation_id":  a.CorrelationID,
			"approval_status": a.Status,
			"error":           fmt.Sprintf("approval status is %q, not pending or approved", a.Status),
		})
		return &MutationResult{Success: false, ApprovalID: approvalID, TargetPath: a.TargetPath, Message: fmt.Sprintf("approval status is %q", a.Status)}, nil
	}

	// 3. Safety checks
	safetyErr := e.validateSafety(a)
	if safetyErr != nil {
		e.emitEvent("prism.mutation.failed", map[string]any{
			"approval_id":    approvalID,
			"run_id":         runID,
			"mutation_type":  a.MutationType,
			"target_path":    a.TargetPath,
			"correlation_id": a.CorrelationID,
			"error":          safetyErr.Error(),
		})
		return &MutationResult{Success: false, ApprovalID: approvalID, TargetPath: a.TargetPath, Message: safetyErr.Error()}, nil
	}

	// 4. Approve if still pending
	if a.Status == approval.StatusPending {
		if err := a.Approve(approvedBy); err != nil {
			e.emitEvent("prism.mutation.failed", map[string]any{
				"approval_id": approvalID,
				"error":       "failed to approve: " + err.Error(),
			})
			return &MutationResult{Success: false, ApprovalID: approvalID, Message: "failed to approve"}, nil
		}
		// Persist the updated approval
		if err := e.approvalStore.Save(a); err != nil {
			return nil, fmt.Errorf("failed to save updated approval: %w", err)
		}
	}

	// 5. Emit approval.granted
	e.emitEvent("prism.approval.granted", map[string]any{
		"approval_id":     approvalID,
		"run_id":          runID,
		"mutation_type":   a.MutationType,
		"target_path":     a.TargetPath,
		"correlation_id":  a.CorrelationID,
		"approved_by":     approvedBy,
		"policy_decision": a.Policy.Decision,
		"policy_reason":   a.Policy.Reason,
	})

	// 6. Emit mutation.validated
	e.emitEvent("prism.mutation.validated", map[string]any{
		"approval_id":    approvalID,
		"run_id":         runID,
		"mutation_type":  a.MutationType,
		"target_path":    a.TargetPath,
		"correlation_id": a.CorrelationID,
	})

	// 7. Apply the mutation
	absPath, resolveErr := e.resolveTargetPath(a.TargetPath)
	if resolveErr != nil {
		e.emitEvent("prism.mutation.failed", map[string]any{
			"approval_id":    approvalID,
			"mutation_type":  a.MutationType,
			"target_path":    a.TargetPath,
			"correlation_id": a.CorrelationID,
			"error":          resolveErr.Error(),
		})
		return &MutationResult{Success: false, ApprovalID: approvalID, TargetPath: a.TargetPath, Message: resolveErr.Error()}, nil
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(absPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		e.emitEvent("prism.mutation.failed", map[string]any{
			"approval_id":    approvalID,
			"mutation_type":  a.MutationType,
			"target_path":    a.TargetPath,
			"correlation_id": a.CorrelationID,
			"error":          "failed to create parent directory: " + err.Error(),
		})
		return &MutationResult{Success: false, ApprovalID: approvalID, TargetPath: a.TargetPath, Message: "failed to create parent directory"}, nil
	}

	if err := os.WriteFile(absPath, []byte(a.Content), 0644); err != nil {
		e.emitEvent("prism.mutation.failed", map[string]any{
			"approval_id":    approvalID,
			"mutation_type":  a.MutationType,
			"target_path":    a.TargetPath,
			"correlation_id": a.CorrelationID,
			"error":          "file write failed: " + err.Error(),
		})
		return &MutationResult{Success: false, ApprovalID: approvalID, TargetPath: a.TargetPath, Message: "file write failed: " + err.Error()}, nil
	}

	// 8. Emit mutation.applied
	e.emitEvent("prism.mutation.applied", map[string]any{
		"approval_id":    approvalID,
		"run_id":         runID,
		"mutation_type":  a.MutationType,
		"target_path":    a.TargetPath,
		"correlation_id": a.CorrelationID,
		"content_size":   len(a.Content),
		"applied_by":     approvedBy,
	})

	return &MutationResult{
		Success:    true,
		ApprovalID: approvalID,
		TargetPath: a.TargetPath,
		Message:    "mutation applied successfully",
	}, nil
}

// validateSafety performs all required safety checks before any file write.
func (e *Executor) validateSafety(a *approval.Approval) error {
	// Check: target path must not be empty
	if a.TargetPath == "" {
		return fmt.Errorf("target path is empty")
	}

	// Check: no path traversal
	if strings.Contains(a.TargetPath, "..") {
		return fmt.Errorf("path traversal with '..' is blocked: %q", a.TargetPath)
	}

	resolvedTargetPath, resolveErr := e.resolveTargetPath(a.TargetPath)
	if resolveErr != nil {
		return resolveErr
	}
	if len(a.Content) > MaxContentSize {
		return fmt.Errorf("content size %d bytes exceeds maximum %d bytes", len(a.Content), MaxContentSize)
	}
	if info, statErr := os.Lstat(resolvedTargetPath); statErr == nil {
		if info.IsDir() {
			return fmt.Errorf("target path %q is a directory, not a file", a.TargetPath)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("target path %q is a symlink, not a regular file", a.TargetPath)
		}
	}
	return nil

	// Check: no absolute paths
	if safety.IsAbsolutePath(a.TargetPath) {
		return fmt.Errorf("absolute paths are not allowed: %q", a.TargetPath)
	}

	// Check: path stays within workspace root (with symlink resolution)
	// This is defense-in-depth against path traversal attacks. We resolve ALL symlinks
	// in both the root and the target path before comparing, so an attacker can't
	// create a symlink inside the workspace that points outside it.
	// Why EvalSymlinks instead of just checking string prefixes? Because
	// `workspace/safe_link → /etc/passwd` would pass a string prefix check but
	// EvalSymlinks resolves it to /etc/passwd, which is clearly outside the root.
	absRoot, err := filepath.Abs(e.workspaceRoot)
	if err != nil {
		return fmt.Errorf("invalid workspace root: %w", err)
	}

	// Resolve symlinks in workspace root
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		resolvedRoot = absRoot // fallback if root doesn't resolve
	}
	resolvedRoot = filepath.Clean(resolvedRoot)

	// Resolve the target path — for write targets that may not exist yet,
	// resolve the parent directory and join with the filename
	absPath := filepath.Clean(filepath.Join(absRoot, a.TargetPath))
	parentDir := filepath.Dir(absPath)
	resolvedParent, parentErr := filepath.EvalSymlinks(parentDir)
	if parentErr != nil {
		// Parent doesn't exist yet — verify the raw path is within root
		// (this is safe because we've already blocked '..' and absolute paths)
		rel, relErr := filepath.Rel(absRoot, absPath)
		if relErr != nil || strings.HasPrefix(rel, "..") || rel == ".." {
			return fmt.Errorf("path is outside workspace root: %q", a.TargetPath)
		}
	} else {
		// Parent exists — resolve and verify symlink safety
		resolvedAbsPath := filepath.Join(resolvedParent, filepath.Base(absPath))
		resolvedAbsPath = filepath.Clean(resolvedAbsPath)
		rel, relErr := filepath.Rel(resolvedRoot, resolvedAbsPath)
		if relErr != nil || strings.HasPrefix(rel, "..") || rel == ".." {
			return fmt.Errorf("path is outside workspace root (symlink escape blocked): %q", a.TargetPath)
		}
	}

	// Check: content is text, size under limit
	if len(a.Content) > MaxContentSize {
		return fmt.Errorf("content size %d bytes exceeds maximum %d bytes", len(a.Content), MaxContentSize)
	}

	// Check: if the target exists, it must be a file (not a directory)
	// Use Lstat to check without following final symlink
	if info, err := os.Lstat(absPath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("target path %q is a directory, not a file", a.TargetPath)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("target path %q is a symlink, not a regular file", a.TargetPath)
		}
	}

	return nil
}

func (e *Executor) resolveTargetPath(targetPath string) (string, error) {
	roots := append([]string{e.workspaceRoot}, e.allowedPaths...)
	absPath, err := safety.ResolveAndContainMulti(roots, targetPath)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absPath), nil
}

// DenyApproval marks an approval as denied without applying the mutation.
func (e *Executor) DenyApproval(runID, approvalID, deniedBy, reason string) error {
	a, err := e.approvalStore.Load(runID, approvalID)
	if err != nil {
		return fmt.Errorf("approval not found: %w", err)
	}

	if err := a.Deny(deniedBy, reason); err != nil {
		return err
	}

	if err := e.approvalStore.Save(a); err != nil {
		return fmt.Errorf("failed to save denied approval: %w", err)
	}

	e.emitEvent("prism.approval.denied", map[string]any{
		"approval_id":     approvalID,
		"run_id":          runID,
		"mutation_type":   a.MutationType,
		"target_path":     a.TargetPath,
		"correlation_id":  a.CorrelationID,
		"denied_by":       deniedBy,
		"denial_reason":   reason,
		"policy_decision": a.Policy.Decision,
	})

	return nil
}

// emitEvent calls the configured emitter if it exists.
func (e *Executor) emitEvent(eventType string, payload map[string]any) {
	if e.emit != nil {
		e.emit(eventType, "prism-mutation-executor", payload)
	}
}
