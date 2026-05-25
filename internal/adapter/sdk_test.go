package adapter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockLifecycleAdapter is a test implementation of LifecycleAdapter.
type mockLifecycleAdapter struct {
	name    string
	version string
	config  map[string]any
	started bool
	stopped bool
}

func (m *mockLifecycleAdapter) Init(config map[string]any) error {
	m.config = config
	return nil
}

func (m *mockLifecycleAdapter) Start(ctx context.Context) error {
	m.started = true
	<-ctx.Done()
	return nil
}

func (m *mockLifecycleAdapter) Stop() error {
	m.stopped = true
	return nil
}

func (m *mockLifecycleAdapter) Name() string               { return m.name }
func (m *mockLifecycleAdapter) Version() string            { return m.version }
func (m *mockLifecycleAdapter) Capabilities() []Capability  { return nil }
func (m *mockLifecycleAdapter) Execute(ctx context.Context, action string, input map[string]any) (*Result, error) {
	return &Result{Output: map[string]any{"status": "ok"}}, nil
}
func (m *mockLifecycleAdapter) Health(ctx context.Context) (*HealthResult, error) {
	return &HealthResult{Ready: true}, nil
}

func TestLifecycleAdapter_Interface(t *testing.T) {
	var _ LifecycleAdapter = &mockLifecycleAdapter{}
	a := &mockLifecycleAdapter{name: "test", version: "1.0.0"}

	if err := a.Init(map[string]any{"key": "value"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if a.config["key"] != "value" {
		t.Errorf("expected config key=value, got %v", a.config["key"])
	}
}

func TestLifecycleAdapter_StartStop(t *testing.T) {
	a := &mockLifecycleAdapter{name: "test", version: "1.0.0"}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if err := a.Start(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !a.started {
		t.Error("expected adapter to be started")
	}

	if err := a.Stop(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !a.stopped {
		t.Error("expected adapter to be stopped")
	}
}

func TestSDKManifest_Load(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	content := `name: my-adapter
version: 1.0.0
description: A test adapter
author: test
type: chat
events:
  - subject: my-adapter.message.received
    direction: in
    description: Incoming message from chat
  - subject: my-adapter.message.sent
    direction: out
    description: Outgoing message to chat
config:
  - name: token
    type: string
    required: true
    description: Bot token
  - name: channel_id
    type: string
    required: false
    description: Default channel ID
dependencies:
  - discord
`
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	m, err := LoadSDKManifest(manifestPath)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	if m.Name != "my-adapter" {
		t.Errorf("expected name my-adapter, got %s", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", m.Version)
	}
	if m.Type != "chat" {
		t.Errorf("expected type chat, got %s", m.Type)
	}
	if len(m.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(m.Events))
	}
	if len(m.Config) != 2 {
		t.Errorf("expected 2 config fields, got %d", len(m.Config))
	}
	if len(m.Dependencies) != 1 {
		t.Errorf("expected 1 dependency, got %d", len(m.Dependencies))
	}
}

func TestSDKManifest_MissingName(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	content := `version: 1.0.0
type: chat
`
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	_, err := LoadSDKManifest(manifestPath)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestSDKManifest_MissingVersion(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	content := `name: my-adapter
type: chat
`
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	_, err := LoadSDKManifest(manifestPath)
	if err == nil {
		t.Error("expected error for missing version")
	}
}

func TestSDKManifest_ValidateConfig(t *testing.T) {
	m := &SDKManifest{
		Name:    "test",
		Version: "1.0.0",
		Type:    "chat",
		Config: []ConfigField{
			{Name: "token", Type: "string", Required: true, Description: "Bot token"},
			{Name: "channel", Type: "string", Required: false, Description: "Channel ID"},
		},
	}

	// Missing required field
	err := m.ValidateConfig(map[string]any{"channel": "123"})
	if err == nil {
		t.Error("expected error for missing required field")
	}

	// All required fields present
	err = m.ValidateConfig(map[string]any{"token": "abc123"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// All fields present
	err = m.ValidateConfig(map[string]any{"token": "abc123", "channel": "123"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSDKManifest_FileNotFound(t *testing.T) {
	_, err := LoadSDKManifest("/nonexistent/manifest.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestEventDescriptor(t *testing.T) {
	ed := EventDescriptor{
		Subject:     "adapter.discord.message.received",
		Direction:   "in",
		Description: "Incoming message from Discord",
	}
	if ed.Subject != "adapter.discord.message.received" {
		t.Errorf("unexpected subject: %s", ed.Subject)
	}
	if ed.Direction != "in" {
		t.Errorf("unexpected direction: %s", ed.Direction)
	}
}

func TestHealthStatus(t *testing.T) {
	hs := HealthStatus{
		Name:      "test-adapter",
		Healthy:   true,
		Message:   "all good",
		StartedAt: time.Now(),
	}
	if hs.Name != "test-adapter" {
		t.Errorf("unexpected name: %s", hs.Name)
	}
	if !hs.Healthy {
		t.Error("expected healthy=true")
	}
}