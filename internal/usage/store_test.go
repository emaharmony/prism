package usage

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecordAndTotals(t *testing.T) {
	s := newTestStore(t)
	base := time.Now().UnixMilli()

	rows := []Event{
		{TS: base, Agent: "astraea", Provider: "anthropic", Model: "claude-sonnet-4-20250514", Source: "invoke", PromptTokens: 100, CompletionTokens: 50, CostUSD: 0.1},
		{TS: base + 1000, Agent: "astraea", Provider: "anthropic", Model: "claude-sonnet-4-20250514", Source: "chat", PromptTokens: 200, CompletionTokens: 40, CostUSD: 0.2},
		{TS: base + 2000, Agent: "eddie", Provider: "openai", Model: "gpt-4o", Source: "invoke", PromptTokens: 10, CompletionTokens: 5, CostUSD: 0.01},
	}
	for _, r := range rows {
		if err := s.Record(r); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	tot, err := s.TotalsFor(base-1, base+10_000)
	if err != nil {
		t.Fatalf("TotalsFor: %v", err)
	}
	if tot.Total != 405 { // (100+50)+(200+40)+(10+5)
		t.Errorf("total tokens = %d, want 405", tot.Total)
	}
	if tot.Count != 3 {
		t.Errorf("count = %d, want 3", tot.Count)
	}
	if tot.Prompt != 310 {
		t.Errorf("prompt = %d, want 310", tot.Prompt)
	}
}

func TestTotalTokensDerivedWhenZero(t *testing.T) {
	s := newTestStore(t)
	base := time.Now().UnixMilli()
	// TotalTokens omitted -> derived from prompt+completion.
	if err := s.Record(Event{TS: base, PromptTokens: 7, CompletionTokens: 3}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	tot, err := s.TotalsFor(base-1, base+1)
	if err != nil {
		t.Fatalf("TotalsFor: %v", err)
	}
	if tot.Total != 10 {
		t.Errorf("derived total = %d, want 10", tot.Total)
	}
}

func TestSumByDimensions(t *testing.T) {
	s := newTestStore(t)
	base := time.Now().UnixMilli()
	_ = s.Record(Event{TS: base, Agent: "a", Provider: "anthropic", Model: "m1", PromptTokens: 100, CompletionTokens: 0})
	_ = s.Record(Event{TS: base + 1, Agent: "b", Provider: "anthropic", Model: "m2", PromptTokens: 300, CompletionTokens: 0})

	byProvider, err := s.SumBy("provider", base-1, base+10)
	if err != nil {
		t.Fatalf("SumBy: %v", err)
	}
	if len(byProvider) != 1 || byProvider[0].Key != "anthropic" || byProvider[0].Total != 400 {
		t.Errorf("byProvider = %+v, want single anthropic=400", byProvider)
	}

	byAgent, err := s.SumBy("agent", base-1, base+10)
	if err != nil {
		t.Fatalf("SumBy agent: %v", err)
	}
	// Ordered by total desc -> b (300) before a (100).
	if len(byAgent) != 2 || byAgent[0].Key != "b" {
		t.Errorf("byAgent = %+v, want b first", byAgent)
	}

	if _, err := s.SumBy("bogus", base, base+1); err == nil {
		t.Error("SumBy with invalid dimension should error")
	}
}

func TestSeriesBuckets(t *testing.T) {
	s := newTestStore(t)
	base := int64(1_000_000_000_000) // fixed, bucket-aligned-ish
	bucket := int64(1000)
	// Two events in the same bucket, one in a later bucket.
	_ = s.Record(Event{TS: base + 100, PromptTokens: 10, CompletionTokens: 0})
	_ = s.Record(Event{TS: base + 200, PromptTokens: 20, CompletionTokens: 0})
	_ = s.Record(Event{TS: base + 3100, PromptTokens: 5, CompletionTokens: 0})

	pts, err := s.Series(bucket, base, base+10_000)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("got %d buckets, want 2: %+v", len(pts), pts)
	}
	if pts[0].Total != 30 {
		t.Errorf("first bucket total = %d, want 30", pts[0].Total)
	}
	if pts[1].Total != 5 {
		t.Errorf("second bucket total = %d, want 5", pts[1].Total)
	}
}
