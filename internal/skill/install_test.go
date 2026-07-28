package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveSkillName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/user/my-skill.git", "my-skill"},
		{"https://github.com/user/my-skill", "my-skill"},
		{"git@github.com:user/awesome-tool.git", "awesome-tool"},
		{"https://gitlab.com/team/cool-skill.git", "cool-skill"},
	}
	for _, tt := range tests {
		got := deriveSkillName(tt.url)
		if got != tt.want {
			t.Errorf("deriveSkillName(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestInstall_AndUninstall(t *testing.T) {
	// Create a fake git repo with a SKILL.md
	repoDir := t.TempDir()
	skillContent := `---
name: test-skill
description: A test skill for unit testing
---
# Test Skill
This is a test skill body.
`
	if err := os.WriteFile(filepath.Join(repoDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}
	// Init git repo
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Since we can't clone a local non-bare repo easily, test the parse logic instead
	skillsDir := t.TempDir()

	// Manually create the skill directory (simulating a clone)
	targetDir := filepath.Join(skillsDir, "test-skill")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test ListInstalled
	infos, err := ListInstalled(skillsDir)
	if err != nil {
		t.Fatalf("ListInstalled error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 installed skill, got %d", len(infos))
	}
	if infos[0].Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", infos[0].Name)
	}
	if infos[0].Description != "A test skill for unit testing" {
		t.Errorf("expected description, got %q", infos[0].Description)
	}

	// Test Uninstall
	if err := Uninstall("test-skill", skillsDir); err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}

	// Verify it's gone
	infos, _ = ListInstalled(skillsDir)
	if len(infos) != 0 {
		t.Errorf("expected 0 skills after uninstall, got %d", len(infos))
	}
}

func TestUninstall_NotFound(t *testing.T) {
	skillsDir := t.TempDir()
	err := Uninstall("nonexistent", skillsDir)
	if err == nil {
		t.Error("expected error for uninstalling non-existent skill")
	}
}

func TestListInstalled_EmptyDir(t *testing.T) {
	skillsDir := t.TempDir()
	infos, err := ListInstalled(skillsDir)
	if err != nil {
		t.Fatalf("ListInstalled error: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected 0 skills in empty dir, got %d", len(infos))
	}
}

func TestListInstalled_NonexistentDir(t *testing.T) {
	infos, err := ListInstalled("/nonexistent/path/skills")
	if err != nil {
		t.Fatalf("ListInstalled error: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected 0 skills for nonexistent dir, got %d", len(infos))
	}
}