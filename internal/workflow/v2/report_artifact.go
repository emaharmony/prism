package v2

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteReportArtifact persists the workflow's final report as a durable Markdown
// file at <baseDir>/<runID>/REPORT.md, alongside the validation artifacts. This
// gives users a permanent proof-of-work summary (phase outcomes, verification,
// delegations, tasks) independent of any Discord/terminal message that scrolls
// away. It returns the path written.
//
// It is an output helper — it does not touch the engine's run loop or gates — so a
// write failure is non-fatal to the caller (the report is still returned in-band).
func WriteReportArtifact(state *WorkflowState, baseDir string) (string, error) {
	if state == nil {
		return "", fmt.Errorf("write report: nil state")
	}
	runID := state.RunID
	if runID == "" {
		runID = "gated-loop"
	}
	dir := filepath.Join(baseDir, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("write report: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "REPORT.md")
	content := FormatFinalReport(state)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write report: %s: %w", path, err)
	}
	return path, nil
}
