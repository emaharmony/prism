package checksum

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeChecksum(t *testing.T) {
	data := []byte("hello world")
	checksum := ComputeChecksum(data)
	if len(checksum) != 64 {
		t.Errorf("checksum length = %d, want 64 (SHA-256 hex)", len(checksum))
	}
	// SHA-256 of "hello world" is a well-known value
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if checksum != expected {
		t.Errorf("ComputeChecksum = %q, want %q", checksum, expected)
	}
}

func TestComputeChecksumEmpty(t *testing.T) {
	checksum := ComputeChecksum([]byte{})
	// SHA-256 of empty string
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if checksum != expected {
		t.Errorf("ComputeChecksum(empty) = %q, want %q", checksum, expected)
	}
}

func TestComputeChecksumDeterministic(t *testing.T) {
	data := []byte("deterministic test")
	c1 := ComputeChecksum(data)
	c2 := ComputeChecksum(data)
	if c1 != c2 {
		t.Errorf("same data produced different checksums: %q != %q", c1, c2)
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("verify me")
	checksum := ComputeChecksum(data)

	if !VerifyChecksum(data, checksum) {
		t.Error("VerifyChecksum should return true for matching checksum")
	}

	if VerifyChecksum(data, "wrongchecksum") {
		t.Error("VerifyChecksum should return false for mismatched checksum")
	}

	if VerifyChecksum([]byte("different data"), checksum) {
		t.Error("VerifyChecksum should return false for different data")
	}
}

func TestWriteWithChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile.md")
	data := []byte("# Hello World\n\nThis is a test artifact.")

	err := WriteWithChecksum(path, data)
	if err != nil {
		t.Fatalf("WriteWithChecksum() error = %v", err)
	}

	// Verify the data file was written
	readData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(readData) != string(data) {
		t.Errorf("read data doesn't match written data")
	}

	// Verify the sidecar was written
	sidecar := path + ".sha256"
	sidecarData, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("ReadFile(sidecar) error = %v", err)
	}

	expectedChecksum := ComputeChecksum(data)
	// Sidecar format: "checksum  filename\n"
	expectedContent := expectedChecksum + "  testfile.md\n"
	if string(sidecarData) != expectedContent {
		t.Errorf("sidecar content = %q, want %q", string(sidecarData), expectedContent)
	}
}

func TestWriteWithChecksumCreatesParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "dir", "file.md")
	data := []byte("nested content")

	err := WriteWithChecksum(path, data)
	if err != nil {
		t.Fatalf("WriteWithChecksum() error = %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file not created in nested directory")
	}
}

func TestReadWithChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile.md")
	data := []byte("# Test Content")

	// Write with checksum first
	WriteWithChecksum(path, data)

	// Read and verify
	readData, err := ReadWithChecksum(path)
	if err != nil {
		t.Fatalf("ReadWithChecksum() error = %v", err)
	}
	if string(readData) != string(data) {
		t.Errorf("read data doesn't match written data")
	}
}

func TestReadWithChecksumMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile.md")

	// Write file with checksum
	WriteWithChecksum(path, []byte("original content"))

	// Tamper with the file (but not the sidecar)
	if err := os.WriteFile(path, []byte("tampered content"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Read should detect the mismatch
	_, err := ReadWithChecksum(path)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("expected ErrChecksumMismatch, got %v", err)
	}
}

func TestReadWithChecksumNoSidecar(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile.md")
	data := []byte("no sidecar")

	// Write file without checksum
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Read should succeed without verification (no sidecar)
	readData, err := ReadWithChecksum(path)
	if err != nil {
		t.Fatalf("ReadWithChecksum() error = %v", err)
	}
	if string(readData) != string(data) {
		t.Errorf("read data doesn't match written data")
	}
}

func TestReadWithChecksumFileNotFound(t *testing.T) {
	_, err := ReadWithChecksum("/nonexistent/file.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseSidecar(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"standard format", "abc123  testfile.md\n", "abc123"},
		{"no newline", "abc123  testfile.md", "abc123"},
		{"just checksum", "abc123\n", "abc123"},
		{"raw checksum", "abc123", "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSidecar([]byte(tt.input))
			if got != tt.expected {
				t.Errorf("parseSidecar(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roundtrip.md")

	for i, data := range [][]byte{
		[]byte("first write"),
		[]byte("second write with more content"),
		[]byte(""),
		[]byte("unicode: 你好世界 🌍"),
	} {
		err := WriteWithChecksum(path, data)
		if err != nil {
			t.Fatalf("WriteWithChecksum() iteration %d error = %v", i, err)
		}

		readData, err := ReadWithChecksum(path)
		if err != nil {
			t.Fatalf("ReadWithChecksum() iteration %d error = %v", i, err)
		}
		if string(readData) != string(data) {
			t.Errorf("iteration %d: read data doesn't match written data", i)
		}
	}
}