package trajectory

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestStore_CRUD(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()

	store := &Store{db: db}
	if err := store.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	now := time.Now()
	run := TrajectoryRun{
		ID:         "traj-test-001",
		AgentID:    "lumi",
		SessionKey: "discord:channel:123",
		Model:      "glm-5.2:cloud",
		Provider:   "ollama",
		Trigger:    "discord",
		TriggerDetail: "channel:123",
		Status:     StatusCompleted,
		ToolCalls: []ToolCallRecord{
			{ToolName: "search_files", Success: true, DurationMs: 120},
			{ToolName: "read_file", Success: true, DurationMs: 45},
		},
		PromptTokens: 15000,
		OutputTokens: 800,
		DurationMs:   3500,
		StartedAt:    now,
		EndedAt:      now.Add(3500 * time.Millisecond),
		SkillsLoaded: []string{"weather", "github"},
		ConfigSnapshot: map[string]string{"think_level": "high"},
	}

	// Save
	if err := store.Save(run); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Load
	loaded, err := store.Load("traj-test-001")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ID != "traj-test-001" {
		t.Errorf("expected ID 'traj-test-001', got %q", loaded.ID)
	}
	if loaded.AgentID != "lumi" {
		t.Errorf("expected agent 'lumi', got %q", loaded.AgentID)
	}
	if loaded.Model != "glm-5.2:cloud" {
		t.Errorf("expected model, got %q", loaded.Model)
	}
	if len(loaded.ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls, got %d", len(loaded.ToolCalls))
	}
	if loaded.ToolCalls[0].ToolName != "search_files" {
		t.Errorf("expected first tool 'search_files', got %q", loaded.ToolCalls[0].ToolName)
	}
	if len(loaded.SkillsLoaded) != 2 {
		t.Errorf("expected 2 skills, got %d", len(loaded.SkillsLoaded))
	}
	if loaded.ConfigSnapshot["think_level"] != "high" {
		t.Errorf("expected think_level 'high', got %q", loaded.ConfigSnapshot["think_level"])
	}
	if loaded.PromptTokens != 15000 {
		t.Errorf("expected 15000 prompt tokens, got %d", loaded.PromptTokens)
	}
}

func TestStore_List(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()

	store := &Store{db: db}
	store.Init()

	// Insert multiple runs
	for i := 0; i < 5; i++ {
		run := TrajectoryRun{
			ID:        "traj-list-" + string(rune('a'+i)),
			AgentID:   "lumi",
			Model:     "glm-5.2:cloud",
			Trigger:   "discord",
			Status:    StatusCompleted,
			StartedAt: time.Now().Add(time.Duration(i) * time.Minute),
		}
		store.Save(run)
	}

	// List all (most recent first)
	runs, err := store.List(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(runs) != 5 {
		t.Fatalf("expected 5 runs, got %d", len(runs))
	}
	// Most recent should be first (last inserted has latest started_at)
	if runs[0].ID != "traj-list-e" {
		t.Errorf("expected most recent first, got %q", runs[0].ID)
	}

	// List with limit
	runs, _ = store.List(2)
	if len(runs) != 2 {
		t.Errorf("expected 2 runs with limit, got %d", len(runs))
	}
}

func TestStore_ListForAgent(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()

	store := &Store{db: db}
	store.Init()

	store.Save(TrajectoryRun{ID: "a1", AgentID: "lumi", Model: "m1", StartedAt: time.Now()})
	store.Save(TrajectoryRun{ID: "a2", AgentID: "scout", Model: "m2", StartedAt: time.Now()})
	store.Save(TrajectoryRun{ID: "a3", AgentID: "lumi", Model: "m1", StartedAt: time.Now()})

	lumiRuns, _ := store.ListForAgent("lumi", 10)
	if len(lumiRuns) != 2 {
		t.Errorf("expected 2 lumi runs, got %d", len(lumiRuns))
	}

	scoutRuns, _ := store.ListForAgent("scout", 10)
	if len(scoutRuns) != 1 {
		t.Errorf("expected 1 scout run, got %d", len(scoutRuns))
	}
}

func TestStore_ExportJSONL(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()

	store := &Store{db: db}
	store.Init()

	store.Save(TrajectoryRun{ID: "exp1", AgentID: "lumi", Model: "m", StartedAt: time.Now()})
	store.Save(TrajectoryRun{ID: "exp2", AgentID: "lumi", Model: "m", StartedAt: time.Now()})

	jsonl, err := store.ExportJSONL(10)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if jsonl == "" {
		t.Error("expected non-empty JSONL")
	}
	// Should have 2 lines (one per run)
	lines := 0
	for _, c := range jsonl {
		if c == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("expected 2 lines, got %d", lines)
	}
}

func TestHashPrompt(t *testing.T) {
	hash1 := HashPrompt("hello world")
	hash2 := HashPrompt("hello world")
	hash3 := HashPrompt("different prompt")

	if hash1 != hash2 {
		t.Error("same prompt should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different prompts should produce different hashes")
	}
	if len(hash1) != 32 {
		t.Errorf("expected 32 char hash, got %d", len(hash1))
	}
}

func TestStore_UpdateStatus(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()

	store := &Store{db: db}
	store.Init()

	// Save with pending status
	store.Save(TrajectoryRun{
		ID:        "update-001",
		AgentID:   "lumi",
		Status:    "pending",
		StartedAt: time.Now(),
	})

	// Update to completed
	run, _ := store.Load("update-001")
	run.Status = StatusCompleted
	run.EndedAt = time.Now()
	store.Save(*run)

	loaded, _ := store.Load("update-001")
	if loaded.Status != StatusCompleted {
		t.Errorf("expected completed, got %q", loaded.Status)
	}
}