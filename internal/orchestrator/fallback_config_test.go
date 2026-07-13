package orchestrator

import "testing"

func TestAgentFallbacksParseAndValidate(t *testing.T) {
	cfg, err := LoadConfigFromBytes([]byte(`
agents:
  - id: chisel
    role: asset-maker
    provider: codex
    model: codex
    fallbacks:
      - provider: ollama
        model: qwen3.5:9b
`))
	if err != nil {
		t.Fatalf("LoadConfigFromBytes: %v", err)
	}
	if got := cfg.Agents[0].Fallbacks; len(got) != 1 || got[0].Model != "qwen3.5:9b" {
		t.Fatalf("unexpected fallbacks: %+v", got)
	}
}

func TestAgentFallbacksRejectDuplicateTarget(t *testing.T) {
	cfg, err := LoadConfigFromBytes([]byte(`
agents:
  - id: chisel
    role: asset-maker
    provider: codex
    model: codex
    fallbacks:
      - provider: codex
        model: codex
`))
	if err == nil || cfg != nil {
		t.Fatalf("expected duplicate fallback validation error, got cfg=%+v err=%v", cfg, err)
	}
}
