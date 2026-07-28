package skill

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Install installs a skill from a git repository into the skills directory.
// The repo must contain a SKILL.md file at the root or in a subdirectory.
//
// The skill is cloned into <skillsDir>/<name>/. If the directory already
// exists, the install fails unless force is true (which pulls the latest
// changes instead of cloning).
//
// skillsDir is typically <workspace>/.claude/skills/ or <workspace>/skills/.
func Install(gitURL, skillsDir string, force bool) (*Skill, error) {
	if gitURL == "" {
		return nil, fmt.Errorf("skill: git URL is required")
	}
	if skillsDir == "" {
		return nil, fmt.Errorf("skill: skills directory is required")
	}

	// Derive skill name from the repo name
	name := deriveSkillName(gitURL)
	if name == "" {
		return nil, fmt.Errorf("skill: could not derive name from URL %s", gitURL)
	}

	targetDir := filepath.Join(skillsDir, name)

	// Check if already installed
	if _, err := os.Stat(targetDir); err == nil {
		if !force {
			return nil, fmt.Errorf("skill %q already installed at %s (use force to update)", name, targetDir)
		}
		// Force: git pull instead of clone
		return updateSkill(targetDir, gitURL)
	}

	// Create skills directory if it doesn't exist
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return nil, fmt.Errorf("skill: create skills dir: %w", err)
	}

	// Clone the repo
	cmd := exec.Command("git", "clone", "--depth", "1", gitURL, targetDir)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("skill: clone %s: %w", gitURL, err)
	}

	// Find SKILL.md in the cloned repo
	skillPath := filepath.Join(targetDir, SkillFile)
	if _, err := os.Stat(skillPath); err != nil {
		// Check subdirectories
		entries, _ := os.ReadDir(targetDir)
		found := false
		for _, e := range entries {
			if e.IsDir() {
				candidate := filepath.Join(targetDir, e.Name(), SkillFile)
				if _, err := os.Stat(candidate); err == nil {
					skillPath = candidate
					targetDir = filepath.Join(targetDir, e.Name())
					found = true
					break
				}
			}
		}
		if !found {
			os.RemoveAll(targetDir)
			return nil, fmt.Errorf("skill: no SKILL.md found in %s", gitURL)
		}
	}

	// Parse the skill
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, fmt.Errorf("skill: read SKILL.md: %w", err)
	}

	s, err := Parse(data, targetDir, "git")
	if err != nil {
		return nil, fmt.Errorf("skill: parse: %w", err)
	}

	// Override name with the derived name if the skill doesn't specify one
	if s.Name == "" || s.Name == filepath.Base(targetDir) {
		s.Name = name
	}

	return s, nil
}

// Uninstall removes a skill from the skills directory.
func Uninstall(name, skillsDir string) error {
	if name == "" {
		return fmt.Errorf("skill: name is required")
	}
	targetDir := filepath.Join(skillsDir, name)
	if _, err := os.Stat(targetDir); err != nil {
		return fmt.Errorf("skill %q not found at %s", name, targetDir)
	}
	return os.RemoveAll(targetDir)
}

// Update pulls the latest changes for an installed skill.
func Update(name, skillsDir string) (*Skill, error) {
	if name == "" {
		return nil, fmt.Errorf("skill: name is required")
	}
	targetDir := filepath.Join(skillsDir, name)
	if _, err := os.Stat(targetDir); err != nil {
		return nil, fmt.Errorf("skill %q not found at %s", name, targetDir)
	}
	return updateSkill(targetDir, "")
}

// updateSkill pulls the latest changes and re-parses the skill.
func updateSkill(targetDir, gitURL string) (*Skill, error) {
	cmd := exec.Command("git", "pull", "--ff-only")
	cmd.Dir = targetDir
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("skill: git pull: %w", err)
	}

	// Re-parse
	skillPath := filepath.Join(targetDir, SkillFile)
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, fmt.Errorf("skill: read SKILL.md: %w", err)
	}

	s, err := Parse(data, targetDir, "git")
	if err != nil {
		return nil, fmt.Errorf("skill: parse: %w", err)
	}

	return s, nil
}

// ListInstalled returns all installed skills in the skills directory.
func ListInstalled(skillsDir string) ([]Info, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("skill: read dir: %w", err)
	}

	var infos []Info
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillPath := filepath.Join(skillsDir, e.Name(), SkillFile)
		data, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}
		s, err := Parse(data, filepath.Join(skillsDir, e.Name()), "installed")
		if err != nil {
			continue
		}
		infos = append(infos, s.Info())
	}
	return infos, nil
}

// deriveSkillName extracts a skill name from a git URL.
// e.g., https://github.com/user/my-skill.git → my-skill
func deriveSkillName(gitURL string) string {
	// Remove .git suffix
	url := strings.TrimSuffix(gitURL, ".git")
	// Get the last path component
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}