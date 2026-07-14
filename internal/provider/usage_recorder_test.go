package provider

import (
	"context"
	"testing"
)

// fakeChatProvider is a minimal ChatProvider that returns a fixed response.
type fakeChatProvider struct {
	resp ChatGenerateResponse
	err  error
}

func (f *fakeChatProvider) ChatGenerate(ctx context.Context, req ChatGenerateRequest) (ChatGenerateResponse, error) {
	return f.resp, f.err
}

// Generate satisfies Provider; the registry stores providers as Provider and
// type-asserts to ChatProvider in GetChatProviderForAgent.
func (f *fakeChatProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	return GenerateResponse{Text: f.resp.Content}, f.err
}

type captureRecorder struct{ records []UsageRecord }

func (c *captureRecorder) RecordUsage(u UsageRecord) { c.records = append(c.records, u) }

func TestGetChatProviderForAgent_RecordsUsage(t *testing.T) {
	reg := NewProviderRegistry()
	fake := &fakeChatProvider{resp: ChatGenerateResponse{
		Content: "hi", Provider: "anthropic", Model: "claude-sonnet-4-20250514",
		PromptTokens: 120, OutputTokens: 45,
	}}
	reg.Register("configured-model", fake, ModelInfo{ProviderName: "anthropic"})

	rec := &captureRecorder{}
	reg.SetUsageRecorder(rec)

	cp, err := reg.GetChatProviderForAgent("astraea", "configured-model")
	if err != nil {
		t.Fatalf("GetChatProviderForAgent: %v", err)
	}
	if _, err := cp.ChatGenerate(context.Background(), ChatGenerateRequest{
		RunID: "invoke-abc", Agent: "astraea", Model: "configured-model",
	}); err != nil {
		t.Fatalf("ChatGenerate: %v", err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(rec.records))
	}
	r := rec.records[0]
	// Attribution should use the actual serving provider/model from the response,
	// not the configured "configured-model" id.
	if r.Provider != "anthropic" || r.Model != "claude-sonnet-4-20250514" {
		t.Errorf("attribution = %s/%s, want anthropic/claude-sonnet-4-20250514", r.Provider, r.Model)
	}
	if r.Agent != "astraea" || r.PromptTokens != 120 || r.CompletionTokens != 45 {
		t.Errorf("record = %+v, want astraea 120/45", r)
	}
	if r.RunID != "invoke-abc" {
		t.Errorf("run id = %q, want invoke-abc", r.RunID)
	}
}

func TestGetChatProviderForAgent_NoRecorderNoWrap(t *testing.T) {
	reg := NewProviderRegistry()
	fake := &fakeChatProvider{resp: ChatGenerateResponse{Content: "hi"}}
	reg.Register("m", fake, ModelInfo{})
	cp, err := reg.GetChatProviderForAgent("a", "m")
	if err != nil {
		t.Fatalf("GetChatProviderForAgent: %v", err)
	}
	// With no recorder installed the raw provider is returned unwrapped.
	if _, wrapped := cp.(*recordingChatProvider); wrapped {
		t.Error("provider should not be wrapped when no recorder is set")
	}
}
