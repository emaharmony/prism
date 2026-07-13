package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAdapterArtifact(t *testing.T) {
	dir := t.TempDir()

	result := &Result{
		Success: true,
		Output:  map[string]any{"message": "hello"},
	}

	err := WriteAdapterArtifact(dir, "echo", "echo", result)
	if err != nil {
		t.Fatalf("WriteAdapterArtifact() error = %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "adapter", "echo_echo.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read artifact: %v", err)
	}

	// Verify content
	if len(data) == 0 {
		t.Error("Artifact file is empty")
	}

	// Verify the success field is in the output
	if string(data) == "" {
		t.Error("Artifact data is empty")
	}
}

func TestWriteAdapterArtifact_CreatesDir(t *testing.T) {
	dir := t.TempDir()

	result := &Result{Success: true}

	err := WriteAdapterArtifact(dir, "trading", "execute", result)
	if err != nil {
		t.Fatalf("WriteAdapterArtifact() error = %v", err)
	}

	// Verify directory was created
	adapterDir := filepath.Join(dir, "adapter")
	if _, err := os.Stat(adapterDir); os.IsNotExist(err) {
		t.Error("WriteAdapterArtifact() did not create adapter directory")
	}
}
