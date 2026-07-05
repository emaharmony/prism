package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFullFrontmatter(t *testing.T) {
	content := `---
name: pdf-processing
description: Extract text from PDFs. Use when working with PDF files.
allowed-tools: [read_file, run_validation]
---
# PDF processing

Run the extractor:

` + "```bash\npython extract.py\n```\n"
	s, err := Parse([]byte(content), "/skills/pdf-processing", "claude")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Name != "pdf-processing" {
		t.Fatalf("name = %q", s.Name)
	}
	if !strings.Contains(s.Description, "Extract text") {
		t.Fatalf("description = %q", s.Description)
	}
	if len(s.AllowedTools) != 2 || s.AllowedTools[0] != "read_file" {
		t.Fatalf("allowed-tools = %v", s.AllowedTools)
	}
	if !strings.Contains(s.Body, "# PDF processing") || !strings.Contains(s.Body, "python extract.py") {
		t.Fatalf("body missing content:\n%s", s.Body)
	}
	if s.Source != "claude" {
		t.Fatalf("source = %q", s.Source)
	}
}

func TestParseNameFallsBackToDir(t *testing.T) {
	content := "---\ndescription: no name here\n---\nbody"
	s, err := Parse([]byte(content), "/skills/my-skill", "x")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Name != "my-skill" {
		t.Fatalf("expected dir-name fallback, got %q", s.Name)
	}
}

func TestParseNoFrontmatterIsAllBody(t *testing.T) {
	content := "# Just instructions\n\nDo the thing."
	s, err := Parse([]byte(content), "/skills/raw", "x")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Name != "raw" {
		t.Fatalf("name = %q", s.Name)
	}
	if !strings.Contains(s.Body, "Just instructions") {
		t.Fatalf("body should be the whole content: %q", s.Body)
	}
}

func TestParseCRLFAndUnterminatedFrontmatter(t *testing.T) {
	// CRLF frontmatter parses normally.
	crlf := "---\r\nname: x\r\ndescription: d\r\n---\r\nbody line\r\n"
	s, err := Parse([]byte(crlf), "/s/x", "c")
	if err != nil || s.Name != "x" || !strings.Contains(s.Body, "body line") {
		t.Fatalf("CRLF parse failed: %+v err=%v", s, err)
	}
	// Unterminated frontmatter → treated as all body, name from dir.
	bad := "---\nname: y\nno closing fence"
	s2, err := Parse([]byte(bad), "/s/dirname", "c")
	if err != nil {
		t.Fatalf("unterminated should not error: %v", err)
	}
	if s2.Name != "dirname" {
		t.Fatalf("expected dir fallback for unterminated frontmatter, got %q", s2.Name)
	}
}

func TestPromptRendersInstructions(t *testing.T) {
	s := &Skill{Name: "x", Description: "does x", Body: "step 1\nstep 2", AllowedTools: []string{"read_file"}, Dir: "/s/x"}
	p := s.Prompt()
	for _, want := range []string{"# Skill: x", "does x", "read_file", "/s/x", "step 1"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q:\n%s", want, p)
		}
	}
}

// --- registry + loader ---

func writeSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SkillFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryRegisterResolveListDedup(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Skill{Name: "a", Description: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&Skill{Name: "a", Description: "dup"}); err == nil {
		t.Fatal("duplicate registration should be rejected (first wins)")
	}
	if err := r.Register(&Skill{Name: ""}); err == nil {
		t.Fatal("nameless skill should be rejected")
	}
	_ = r.Register(&Skill{Name: "b", Description: "second"})
	if r.Len() != 2 {
		t.Fatalf("len = %d", r.Len())
	}
	got, err := r.Resolve("a")
	if err != nil || got.Description != "first" {
		t.Fatalf("resolve a: %+v err=%v", got, err)
	}
	if _, err := r.Resolve("missing"); err == nil {
		t.Fatal("missing skill should error")
	}
	list := r.List()
	if len(list) != 2 || list[0].Name != "a" || list[1].Name != "b" {
		t.Fatalf("list not sorted: %+v", list)
	}
}

func TestLoadDir(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "alpha", "---\nname: alpha\ndescription: A\n---\nbody A")
	writeSkill(t, root, "beta", "---\nname: beta\ndescription: B\n---\nbody B")
	// a non-skill subdir (no SKILL.md) is skipped
	_ = os.MkdirAll(filepath.Join(root, "not-a-skill"), 0o755)

	r := NewRegistry()
	n, err := r.LoadDir(root, "claude")
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if n != 2 || r.Len() != 2 {
		t.Fatalf("expected 2 skills, got n=%d len=%d", n, r.Len())
	}
	a, _ := r.Resolve("alpha")
	if a.Source != "claude" || !strings.Contains(a.Body, "body A") {
		t.Fatalf("alpha wrong: %+v", a)
	}
}

func TestLoadDirMissingRootIsNoError(t *testing.T) {
	r := NewRegistry()
	n, err := r.LoadDir(filepath.Join(t.TempDir(), "nope"), "x")
	if err != nil || n != 0 {
		t.Fatalf("missing root should be (0,nil), got n=%d err=%v", n, err)
	}
}

func TestLoadDefaultPrecedence(t *testing.T) {
	root := t.TempDir()
	// Same skill name in .claude/skills and skills/ — claude (loaded first) wins.
	writeSkill(t, filepath.Join(root, ".claude", "skills"), "shared", "---\nname: shared\ndescription: from-claude\n---\nx")
	writeSkill(t, filepath.Join(root, "skills"), "shared", "---\nname: shared\ndescription: from-workspace\n---\nx")
	writeSkill(t, filepath.Join(root, "skills"), "only-ws", "---\nname: only-ws\ndescription: w\n---\ny")

	r := NewRegistry()
	total, _ := r.LoadDefault(root)
	if total != 2 { // shared (claude) + only-ws; workspace "shared" rejected as dup
		t.Fatalf("expected 2 loaded, got %d", total)
	}
	shared, _ := r.Resolve("shared")
	if shared.Description != "from-claude" || shared.Source != "claude" {
		t.Fatalf("claude skill should win precedence, got %+v", shared)
	}
}

func TestPromptSuffix(t *testing.T) {
	out := PromptSuffix([]Info{
		{Name: "pdf", Description: "process pdfs"},
		{Name: "fastapi", Description: "fastapi best practices"},
	})
	for _, want := range []string{"Available skills", "use_skill", "pdf: process pdfs", "fastapi: fastapi best practices"} {
		if !strings.Contains(out, want) {
			t.Fatalf("suffix missing %q:\n%s", want, out)
		}
	}
	if PromptSuffix(nil) != "" {
		t.Fatal("no skills should yield empty suffix")
	}
}
