package provider

import (
	"context"
	"fmt"
	"testing"
)

func TestChainProvider_SingleProvider_Success(t *testing.T) {
	success := &testProvider{
		response: GenerateResponse{Text: "hello", Provider: "mock"},
	}
	chain := NewChainProvider(success)
	resp, err := chain.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "hello" {
		t.Errorf("Text = %q, want hello", resp.Text)
	}
}

func TestChainProvider_FallbackToNext(t *testing.T) {
	failing := &testProvider{
		err: fmt.Errorf("503 service unavailable"),
	}
	success := &testProvider{
		response: GenerateResponse{Text: "fallback response", Provider: "backup"},
	}
	chain := NewChainProvider(failing, success)
	chain.AllowPaid = true

	resp, err := chain.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "fallback response" {
		t.Errorf("Text = %q, want fallback response", resp.Text)
	}
}

func TestChainProvider_PaidProviderBlocked(t *testing.T) {
	paid := &testProvider{
		response: GenerateResponse{Text: "paid response"},
		tier:     TierPaid,
	}
	chain := NewChainProvider(paid)
	chain.AllowPaid = false

	_, err := chain.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error when all providers are blocked")
	}
}

func TestChainProvider_PaidProviderAllowed(t *testing.T) {
	paid := &testProvider{
		response: GenerateResponse{Text: "paid response"},
		tier:     TierPaid,
	}
	chain := NewChainProvider(paid)
	chain.AllowPaid = true

	resp, err := chain.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "paid response" {
		t.Errorf("Text = %q, want paid response", resp.Text)
	}
}

func TestChainProvider_NonRetryableStops(t *testing.T) {
	failing := &testProvider{
		err: fmt.Errorf("forbidden"),
	}
	success := &testProvider{
		response: GenerateResponse{Text: "should not reach"},
	}
	chain := NewChainProvider(failing, success)
	chain.AllowPaid = true

	_, err := chain.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error for non-retryable failure")
	}
}

func TestChainProvider_AllProvidersFail(t *testing.T) {
	f1 := &testProvider{err: fmt.Errorf("503 unavailable")}
	f2 := &testProvider{err: fmt.Errorf("429 rate limited")}
	chain := NewChainProvider(f1, f2)
	chain.AllowPaid = true

	_, err := chain.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestChainProvider_EmptyChain(t *testing.T) {
	chain := NewChainProvider()
	chain.AllowPaid = true

	_, err := chain.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error for empty chain")
	}
}

func TestChainProvider_Tier(t *testing.T) {
	free := &testProvider{tier: TierFree}
	paid := &testProvider{tier: TierPaid}

	chain1 := NewChainProvider(free)
	if chain1.Tier() != TierFree {
		t.Error("chain with only free provider should be TierFree")
	}

	chain2 := NewChainProvider(free, paid)
	if chain2.Tier() != TierPaid {
		t.Error("chain with paid provider should be TierPaid")
	}

	chain3 := NewChainProvider()
	if chain3.Tier() != TierFree {
		t.Error("empty chain should be TierFree")
	}
}

func TestChainProvider_MixedPaidFree(t *testing.T) {
	free := &testProvider{
		response: GenerateResponse{Text: "free response"},
		tier:     TierFree,
	}
	paid := &testProvider{
		response: GenerateResponse{Text: "paid response"},
		tier:     TierPaid,
	}

	// When free fails, should NOT fall back to paid if AllowPaid=false
	chain := NewChainProvider(free, paid)
	chain.AllowPaid = false

	// But free succeeds, so it should return free response
	resp, err := chain.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "free response" {
		t.Errorf("Text = %q, want free response", resp.Text)
	}
}

func TestStartLatencyMeasurer(t *testing.T) {
	mock := &testProvider{
		response: GenerateResponse{Text: "test"},
	}
	measurer := NewStartLatencyMeasurer(mock, "mock")
	resp, err := measurer.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "test" {
		t.Errorf("Text = %q, want test", resp.Text)
	}
	if measurer.Name() != "mock" {
		t.Errorf("Name() = %q, want mock", measurer.Name())
	}
}