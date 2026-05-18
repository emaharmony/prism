package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

const echoManifestYAML = `name: echo
version: "1.0"
description: "Built-in echo adapter for testing and demos"
capabilities:
  - action: echo
    description: "Echo input back as output"
`

func TestLoadManifestFromYAML(t *testing.T) {
	m, err := LoadManifestFromYAML([]byte(echoManifestYAML))
	if err != nil {
		t.Fatalf("LoadManifestFromYAML() error = %v", err)
	}
	if m.Name != "echo" {
		t.Errorf("Name = %q, want %q", m.Name, "echo")
	}
	if m.Version != "1.0" {
		t.Errorf("Version = %q, want %q", m.Version, "1.0")
	}
	if len(m.Capabilities) != 1 {
		t.Fatalf("Capabilities length = %d, want 1", len(m.Capabilities))
	}
	if m.Capabilities[0].Action != "echo" {
		t.Errorf("Capability Action = %q, want %q", m.Capabilities[0].Action, "echo")
	}
}

func TestLoadManifestFromYAML_MissingName(t *testing.T) {
	_, err := LoadManifestFromYAML([]byte("version: \"1.0\"\n"))
	if err == nil {
		t.Error("LoadManifestFromYAML() missing name should return error")
	}
}

func TestLoadManifestFromYAML_MissingVersion(t *testing.T) {
	_, err := LoadManifestFromYAML([]byte("name: echo\n"))
	if err == nil {
		t.Error("LoadManifestFromYAML() missing version should return error")
	}
}

func TestLoadManifestFromYAML_InvalidName(t *testing.T) {
	badYAML := `name: my.adapter
version: "1.0"
`
	_, err := LoadManifestFromYAML([]byte(badYAML))
	if err == nil {
		t.Error("LoadManifestFromYAML() name with dots should return error")
	}
}

func TestLoadManifestFromYAML_InvalidYAML(t *testing.T) {
	_, err := LoadManifestFromYAML([]byte("not: valid: yaml: [[[["))
	if err == nil {
		t.Error("LoadManifestFromYAML() invalid YAML should return error")
	}
}

func TestLoadManifestFromDir(t *testing.T) {
	dir := t.TempDir()

	// Write echo manifest
	echoPath := filepath.Join(dir, "echo.yaml")
	if err := os.WriteFile(echoPath, []byte(echoManifestYAML), 0644); err != nil {
		t.Fatalf("Failed to write test manifest: %v", err)
	}

	// Write another manifest
	tradingYAML := `name: trading
version: "2.0"
description: "Trading adapter"
capabilities:
  - action: evaluate
    description: "Evaluate trading decisions"
`
	tradingPath := filepath.Join(dir, "trading.yaml")
	if err := os.WriteFile(tradingPath, []byte(tradingYAML), 0644); err != nil {
		t.Fatalf("Failed to write test manifest: %v", err)
	}

	manifests, err := LoadManifestFromDir(dir)
	if err != nil {
		t.Fatalf("LoadManifestFromDir() error = %v", err)
	}
	if len(manifests) != 2 {
		t.Errorf("LoadManifestFromDir() count = %d, want 2", len(manifests))
	}

	// Verify we got both manifests by name
	found := map[string]bool{}
	for _, m := range manifests {
		found[m.Name] = true
	}
	if !found["echo"] || !found["trading"] {
		t.Errorf("LoadManifestFromDir() names = %v, want echo and trading", found)
	}
}

func TestLoadManifestFromDir_Empty(t *testing.T) {
	dir := t.TempDir()

	manifests, err := LoadManifestFromDir(dir)
	if err != nil {
		t.Fatalf("LoadManifestFromDir() empty dir error = %v", err)
	}
	if len(manifests) != 0 {
		t.Errorf("LoadManifestFromDir() empty dir count = %d, want 0", len(manifests))
	}
}

func TestLoadManifestFromDir_Nonexistent(t *testing.T) {
	_, err := LoadManifestFromDir("/nonexistent/path")
	if err == nil {
		t.Error("LoadManifestFromDir() nonexistent dir should return error")
	}
}

func TestLoadManifestFromDir_IgnoresNonYAML(t *testing.T) {
	dir := t.TempDir()

	// Write a non-YAML file
	txtPath := filepath.Join(dir, "readme.txt")
	os.WriteFile(txtPath, []byte("not a manifest"), 0644)

	// Write a YAML manifest
	yamlPath := filepath.Join(dir, "echo.yaml")
	os.WriteFile(yamlPath, []byte(echoManifestYAML), 0644)

	manifests, err := LoadManifestFromDir(dir)
	if err != nil {
		t.Fatalf("LoadManifestFromDir() error = %v", err)
	}
	if len(manifests) != 1 {
		t.Errorf("LoadManifestFromDir() count = %d, want 1", len(manifests))
	}
}