package context

import (
	"os"
	"path/filepath"
	"strings"
)

// readCorrespondenceDir reads correspondence/*.md files from the workspace's
// correspondence directory. These are inter-agent letters between OpenClaw Lumi
// and Prism Lumi. Files are sorted newest-first. Priority 65 — above memory
// files (40) but below named sources (80+), reflecting that letters are more
// time-sensitive than historical memory but less foundational than identity.
func (b *Builder) readCorrespondenceDir() []ContextFile {
	corrDir := filepath.Join(b.WorkspaceRoot, correspondenceDir)
	entries, err := os.ReadDir(corrDir)
	if err != nil {
		return nil
	}

	var mdFiles []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			mdFiles = append(mdFiles, e)
		}
	}
	if len(mdFiles) == 0 {
		return nil
	}

	// Sort by name descending (newest first — letter files start with dates)
	for i := 1; i < len(mdFiles); i++ {
		for j := i; j > 0 && mdFiles[j].Name() > mdFiles[j-1].Name(); j-- {
			mdFiles[j], mdFiles[j-1] = mdFiles[j-1], mdFiles[j]
		}
	}

	var files []ContextFile
	for _, entry := range mdFiles {
		fullPath := filepath.Join(corrDir, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		content := string(data)
		files = append(files, ContextFile{
			Name:            "correspondence/" + entry.Name(),
			Path:            fullPath,
			Content:         content,
			SizeBytes:       len(data),
			EstimatedTokens: estimateTokens(content),
			Priority:        65,
			Source:          "correspondence",
		})
	}
	return files
}