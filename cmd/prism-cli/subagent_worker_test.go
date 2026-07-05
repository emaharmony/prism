package main

import (
	"testing"

	"github.com/emaharmony/prism/internal/orchestrator"
)

func TestSubAgentResolver_MapsAgents(t *testing.T) {
	cfg := &orchestrator.Config{
		Agents: []orchestrator.AgentConfig{
			{ID: "scout", Provider: "ollama", Model: "qwen3.5:9b", Capabilities: []string{"search", "report"}},
			{ID: "atlas", Provider: "ollama", Model: "deepseek-v4-pro:cloud", Capabilities: []string{"code"}},
		},
	}
	r := newSubAgentResolver(cfg)

	rt, ok := r.Resolve("scout")
	if !ok {
		t.Fatal("scout should resolve")
	}
	if rt.Model != "qwen3.5:9b" || rt.Provider != "ollama" {
		t.Errorf("wrong runtime: %+v", rt)
	}
	if len(rt.Capabilities) != 2 {
		t.Errorf("capabilities not copied: %+v", rt.Capabilities)
	}
	// Mutating the resolved copy must not affect config (defensive copy).
	rt.Capabilities[0] = "MUT"
	if cfg.Agents[0].Capabilities[0] == "MUT" {
		t.Error("resolver aliased the config slice")
	}

	if _, ok := r.Resolve("ghost"); ok {
		t.Error("unknown agent should not resolve")
	}
}
