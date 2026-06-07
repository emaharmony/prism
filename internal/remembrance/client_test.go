package remembrance

import (
	"testing"
	"time"
)

func TestNewClientUsesDefaultTimeout(t *testing.T) {
	client := NewClient("http://localhost:8788")
	if client.HTTPClient.Timeout != DefaultTimeout {
		t.Errorf("NewClient timeout = %v, want %v", client.HTTPClient.Timeout, DefaultTimeout)
	}
}

func TestNewClientWithTimeout(t *testing.T) {
	client := NewClientWithTimeout("http://localhost:8788/", 90*time.Second)
	if client.BaseURL != "http://localhost:8788" {
		t.Errorf("BaseURL = %q, want %q", client.BaseURL, "http://localhost:8788")
	}
	if client.HTTPClient.Timeout != 90*time.Second {
		t.Errorf("NewClientWithTimeout timeout = %v, want 90s", client.HTTPClient.Timeout)
	}
}
