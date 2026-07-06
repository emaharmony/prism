package main

import (
	"path/filepath"
	"time"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/provider/codexcli"
	"github.com/emaharmony/prism/internal/tool"
)

func imageToolsConfigFromPrismConfig(cfg *orchestrator.Config, workspaceRoot string, writeRoots []string) tool.ImageToolsConfig {
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	out := tool.ImageToolsConfig{
		ReferencesDir:  filepath.Join(workspaceRoot, "references"),
		WorkspaceRoot:  workspaceRoot,
		AllowedPaths:   append([]string(nil), writeRoots...),
		CodexWorkspace: workspaceRoot,
	}
	if cfg == nil {
		return out
	}
	out.CodexExecutable = cfg.Codex.Executable
	out.CodexModel = cfg.Codex.Model
	out.CodexProfile = cfg.Codex.Profile
	out.CodexWorkspace = codexcli.ResolveWorkspace(cfg.Codex.Workspace, cfg.Prism.Workspace, workspaceRoot)
	if cfg.Codex.TimeoutMinutes > 0 {
		out.CodexTimeout = time.Duration(cfg.Codex.TimeoutMinutes) * time.Minute
	}
	return out
}
