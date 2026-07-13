package bus

import (
	"testing"
)

func TestStartEmbeddedBus(t *testing.T) {
	url, cleanup, err := StartEmbeddedBus(0) // Use port 0 for auto-assignment
	if err != nil {
		t.Fatalf("StartEmbeddedBus() error = %v", err)
	}
	defer cleanup()

	if url == "" {
		t.Error("expected non-empty URL")
	}

	// Try to connect to the embedded bus
	nc, err := ConnectToBus(url)
	if err != nil {
		t.Fatalf("ConnectToBus() error = %v", err)
	}
	defer nc.Close()

	if !nc.IsConnected() {
		t.Error("expected connected to embedded bus")
	}
}

func TestStartEmbeddedBusSpecificPort(t *testing.T) {
	// Use port 0 to avoid conflicts
	url, cleanup, err := StartEmbeddedBus(0)
	if err != nil {
		t.Fatalf("StartEmbeddedBus() error = %v", err)
	}
	defer cleanup()

	if url == "" {
		t.Error("expected non-empty URL")
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		input    string
		default_ int
		expected int
	}{
		{"4222", 4222, 4222},
		{"8080", 4222, 8080},
		{"", 4222, 4222},
		{"invalid", 4222, 4222},
		{"0", 4222, 0},
	}

	for _, tt := range tests {
		got := ParsePort(tt.input, tt.default_)
		if got != tt.expected {
			t.Errorf("ParsePort(%q, %d) = %d, want %d", tt.input, tt.default_, got, tt.expected)
		}
	}
}

func TestGetAvailablePort(t *testing.T) {
	port, err := getAvailablePort()
	if err != nil {
		t.Fatalf("getAvailablePort() error = %v", err)
	}
	if port <= 0 {
		t.Errorf("expected positive port, got %d", port)
	}
}
