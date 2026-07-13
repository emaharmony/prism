package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromOpenClaw_Ollama(t *testing.T) {
	cfg := OpenClawConfig{
		Models: OpenClawModelsConfig{
			Providers: map[string]OpenClawProviderConfig{
				"ollama": {
					API:     "ollama",
					APIKey:  "ollama-local",
					BaseURL: "http://localhost:11434",
					Models: []OpenClawModelConfig{
						{
							ID:            "llama3",
							Name:          "llama3",
							ContextWindow: 8192,
							Input:         []string{"text"},
							Reasoning:     false,
							Cost: OpenClawCostConfig{
								Input:  0,
								Output: 0,
							},
						},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "openclaw.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	registry, err := LoadFromOpenClaw(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !registry.HasModel("llama3") {
		t.Error("expected llama3 model to be registered")
	}

	info, err := registry.ModelInfo("llama3")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.ProviderName != "ollama" {
		t.Errorf("expected provider ollama, got %s", info.ProviderName)
	}
	if info.ContextWindow != 8192 {
		t.Errorf("expected context window 8192, got %d", info.ContextWindow)
	}
}

func TestLoadFromOpenClaw_OpenAI(t *testing.T) {
	cfg := OpenClawConfig{
		Models: OpenClawModelsConfig{
			Providers: map[string]OpenClawProviderConfig{
				"openai": {
					API:    "openai",
					APIKey: "sk-test-123",
					Models: []OpenClawModelConfig{
						{
							ID:            "gpt-4o",
							Name:          "gpt-4o",
							ContextWindow: 128000,
							Input:         []string{"text", "image"},
							Reasoning:     true,
							Cost: OpenClawCostConfig{
								Input:  0.0025,
								Output: 0.01,
							},
						},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "openclaw.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	registry, err := LoadFromOpenClaw(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !registry.HasModel("gpt-4o") {
		t.Error("expected gpt-4o model to be registered")
	}

	info, _ := registry.ModelInfo("gpt-4o")
	if info.Cost.InputPer1K != 0.0025 {
		t.Errorf("expected input cost 0.0025, got %f", info.Cost.InputPer1K)
	}
	if info.Cost.OutputPer1K != 0.01 {
		t.Errorf("expected output cost 0.01, got %f", info.Cost.OutputPer1K)
	}
}

func TestLoadFromOpenClaw_MultipleProviders(t *testing.T) {
	cfg := OpenClawConfig{
		Models: OpenClawModelsConfig{
			Providers: map[string]OpenClawProviderConfig{
				"ollama": {
					API:     "ollama",
					BaseURL: "http://localhost:11434",
					Models: []OpenClawModelConfig{
						{ID: "llama3", Name: "llama3", ContextWindow: 8192, Cost: OpenClawCostConfig{}},
					},
				},
				"openai": {
					API:    "openai",
					APIKey: "sk-test",
					Models: []OpenClawModelConfig{
						{ID: "gpt-4o", Name: "gpt-4o", ContextWindow: 128000, Cost: OpenClawCostConfig{Input: 0.0025}},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "openclaw.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	registry, err := LoadFromOpenClaw(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	models := registry.ListModels()
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}

	ollamaModels := registry.ListModelsByProvider("ollama")
	if len(ollamaModels) != 1 || ollamaModels[0] != "llama3" {
		t.Errorf("expected [llama3], got %v", ollamaModels)
	}
}

func TestLoadFromOpenClaw_MissingFile(t *testing.T) {
	_, err := LoadFromOpenClaw("/nonexistent/path/openclaw.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadFromOpenClaw_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "openclaw.json")
	os.WriteFile(cfgPath, []byte("{invalid json}"), 0644)

	_, err := LoadFromOpenClaw(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadFromOpenClaw_SkipsInvalidProvider(t *testing.T) {
	cfg := OpenClawConfig{
		Models: OpenClawModelsConfig{
			Providers: map[string]OpenClawProviderConfig{
				"openai": {
					API:    "openai",
					APIKey: "", // Missing API key — should be skipped
					Models: []OpenClawModelConfig{
						{ID: "gpt-4o", Name: "gpt-4o", ContextWindow: 128000, Cost: OpenClawCostConfig{}},
					},
				},
				"ollama": {
					API:     "ollama",
					BaseURL: "http://localhost:11434",
					Models: []OpenClawModelConfig{
						{ID: "llama3", Name: "llama3", ContextWindow: 8192, Cost: OpenClawCostConfig{}},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "openclaw.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	registry, err := LoadFromOpenClaw(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// gpt-4o should NOT be registered (missing API key)
	if registry.HasModel("gpt-4o") {
		t.Error("expected gpt-4o to be skipped due to missing API key")
	}

	// llama3 should be registered (ollama doesn't need an API key)
	if !registry.HasModel("llama3") {
		t.Error("expected llama3 to be registered")
	}
}

func TestFindOpenClawConfig_ExplicitPath(t *testing.T) {
	path := FindOpenClawConfig("/custom/path/openclaw.json")
	if path != "/custom/path/openclaw.json" {
		t.Errorf("expected explicit path, got %s", path)
	}
}

func TestFindOpenClawConfig_EnvVar(t *testing.T) {
	os.Setenv("OPENCLAW_CONFIG", "/env/path/openclaw.json")
	defer os.Unsetenv("OPENCLAW_CONFIG")

	path := FindOpenClawConfig("")
	if path != "/env/path/openclaw.json" {
		t.Errorf("expected env path, got %s", path)
	}
}

func TestFindOpenClawConfig_Default(t *testing.T) {
	os.Unsetenv("OPENCLAW_CONFIG")

	path := FindOpenClawConfig("")
	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, ".openclaw", "openclaw.json")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func TestLoadFromOpenClaw_WithBaseURL(t *testing.T) {
	cfg := OpenClawConfig{
		Models: OpenClawModelsConfig{
			Providers: map[string]OpenClawProviderConfig{
				"openai": {
					API:     "openai",
					APIKey:  "sk-test",
					BaseURL: "https://api.custom-openai.com/v1",
					Models: []OpenClawModelConfig{
						{ID: "custom-gpt", Name: "custom-gpt", ContextWindow: 32768, Cost: OpenClawCostConfig{}},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "openclaw.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	registry, err := LoadFromOpenClaw(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !registry.HasModel("custom-gpt") {
		t.Error("expected custom-gpt model to be registered")
	}
}

func TestLoadFromRealOpenClawConfig(t *testing.T) {
	cfgPath := FindOpenClawConfig("")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Skip("No OpenClaw config found, skipping real config test")
	}

	registry, err := LoadFromOpenClaw(cfgPath)
	if err != nil {
		t.Fatalf("expected no error loading real config, got %v", err)
	}

	models := registry.ListModels()
	if len(models) == 0 {
		t.Error("expected at least one model from real OpenClaw config")
	}

	t.Logf("Loaded %d models from OpenClaw config: %v", len(models), models)
}

func TestLoadFromOpenClaw_CostMetadata(t *testing.T) {
	cfg := OpenClawConfig{
		Models: OpenClawModelsConfig{
			Providers: map[string]OpenClawProviderConfig{
				"openai": {
					API:    "openai",
					APIKey: "sk-test",
					Models: []OpenClawModelConfig{
						{
							ID:            "gpt-4o",
							Name:          "GPT-4o",
							ContextWindow: 128000,
							Input:         []string{"text", "image"},
							Reasoning:     true,
							Cost: OpenClawCostConfig{
								Input:      0.0025,
								Output:     0.01,
								CacheRead:  0.00125,
								CacheWrite: 0.005,
							},
						},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "openclaw.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	registry, err := LoadFromOpenClaw(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	info, _ := registry.ModelInfo("gpt-4o")
	if info.Name != "GPT-4o" {
		t.Errorf("expected name 'GPT-4o', got %s", info.Name)
	}
	if !info.Reasoning {
		t.Error("expected reasoning to be true")
	}
	if len(info.InputTypes) != 2 {
		t.Errorf("expected 2 input types, got %d", len(info.InputTypes))
	}
	if info.Cost.CacheRead != 0.00125 {
		t.Errorf("expected cache read cost 0.00125, got %f", info.Cost.CacheRead)
	}
	if info.Cost.CacheWrite != 0.005 {
		t.Errorf("expected cache write cost 0.005, got %f", info.Cost.CacheWrite)
	}
}

func TestLoadFromOpenClaw_EmptyProviders(t *testing.T) {
	cfg := OpenClawConfig{
		Models: OpenClawModelsConfig{
			Mode:      "merge",
			Providers: map[string]OpenClawProviderConfig{},
		},
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "openclaw.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	registry, err := LoadFromOpenClaw(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	models := registry.ListModels()
	if len(models) != 0 {
		t.Errorf("expected 0 models for empty config, got %d", len(models))
	}
}

func TestLoadFromOpenClaw_UnknownProviderAPI(t *testing.T) {
	cfg := OpenClawConfig{
		Models: OpenClawModelsConfig{
			Providers: map[string]OpenClawProviderConfig{
				"unknown": {
					API:    "nonexistent",
					APIKey: "test",
					Models: []OpenClawModelConfig{
						{ID: "test-model", Name: "test", ContextWindow: 4096, Cost: OpenClawCostConfig{}},
					},
				},
				"ollama": {
					API:     "ollama",
					BaseURL: "http://localhost:11434",
					Models: []OpenClawModelConfig{
						{ID: "llama3", Name: "llama3", ContextWindow: 8192, Cost: OpenClawCostConfig{}},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "openclaw.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0644)

	registry, err := LoadFromOpenClaw(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Unknown provider should be skipped, known provider should work
	if registry.HasModel("test-model") {
		t.Error("expected test-model to be skipped for unknown provider API")
	}
	if !registry.HasModel("llama3") {
		t.Error("expected llama3 to be registered")
	}
}
