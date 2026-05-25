package delegation

import (
	"testing"

	"github.com/emaharmony/prism/internal/orchestrator"
)

func TestCanDelegate_PrimaryAgent(t *testing.T) {
	delegator := &orchestrator.AgentConfig{
		ID:       "lumi",
		Role:     "lead",
		Provider: "ollama",
		Model:    "glm-5.1:cloud",
		Primary:  true,
	}
	target := &orchestrator.AgentConfig{
		ID:   "mango",
		Role:  "coder",
		Model: "deepseek-v4-pro:cloud",
	}

	// Primary agent can always delegate
	err := CanDelegate(delegator, target, "code_implementation")
	if err != nil {
		t.Errorf("primary agent should be able to delegate: %v", err)
	}
}

func TestCanDelegate_WithDelegateCapability(t *testing.T) {
	delegator := &orchestrator.AgentConfig{
		ID:           "lumi",
		Role:         "lead",
		Provider:     "ollama",
		Model:        "glm-5.1:cloud",
		Capabilities: []string{"plan", "delegate", "review"},
	}
	target := &orchestrator.AgentConfig{
		ID:           "mango",
		Role:         "coder",
		Provider:     "ollama",
		Model:        "deepseek-v4-pro:cloud",
		Capabilities: []string{"code", "test", "report"},
	}

	err := CanDelegate(delegator, target, "code_implementation")
	if err != nil {
		t.Errorf("agent with delegate capability should be able to delegate: %v", err)
	}
}

func TestCanDelegate_WithoutDelegateCapability(t *testing.T) {
	delegator := &orchestrator.AgentConfig{
		ID:           "researcher",
		Role:         "researcher",
		Provider:     "ollama",
		Model:        "qwen3-vl:235b-cloud",
		Capabilities: []string{"search", "summarize"},
	}
	target := &orchestrator.AgentConfig{
		ID:           "mango",
		Role:         "coder",
		Provider:     "ollama",
		Model:        "deepseek-v4-pro:cloud",
		Capabilities: []string{"code", "test", "report"},
	}

	err := CanDelegate(delegator, target, "code_implementation")
	if err == nil {
		t.Error("agent without delegate capability should not be able to delegate")
	}
}

func TestCanDelegate_TargetCannotHandleTaskType(t *testing.T) {
	delegator := &orchestrator.AgentConfig{
		ID:           "lumi",
		Role:         "lead",
		Provider:     "ollama",
		Model:        "glm-5.1:cloud",
		Capabilities: []string{"plan", "delegate", "review"},
	}
	target := &orchestrator.AgentConfig{
		ID:           "researcher",
		Role:         "researcher",
		Provider:     "ollama",
		Model:        "qwen3-vl:235b-cloud",
		Capabilities: []string{"search", "summarize"},
	}

	err := CanDelegate(delegator, target, "code_implementation")
	if err == nil {
		t.Error("researcher should not be able to handle code tasks")
	}
}

func TestCanDelegate_GeneralTaskType(t *testing.T) {
	delegator := &orchestrator.AgentConfig{
		ID:           "lumi",
		Role:         "lead",
		Provider:     "ollama",
		Model:        "glm-5.1:cloud",
		Capabilities: []string{"plan", "delegate"},
	}
	target := &orchestrator.AgentConfig{
		ID:           "mango",
		Role:         "coder",
		Provider:     "ollama",
		Model:        "deepseek-v4-pro:cloud",
		Capabilities: []string{"code"},
	}

	// "general" tasks can be handled by any agent
	err := CanDelegate(delegator, target, "general")
	if err != nil {
		t.Errorf("general tasks should be delegatable to any agent: %v", err)
	}
}

func TestCanDelegate_RoleDefaults(t *testing.T) {
	// Agent with no explicit capabilities should use role defaults
	delegator := &orchestrator.AgentConfig{
		ID:       "lumi",
		Role:     "lead",
		Provider: "ollama",
		Model:    "glm-5.1:cloud",
		// No explicit capabilities — should get role defaults
	}

	// "lead" role defaults: plan, delegate, review
	if !hasCapability(delegator, CapDelegate) {
		t.Error("lead role should have delegate capability by default")
	}
	if !hasCapability(delegator, CapPlan) {
		t.Error("lead role should have plan capability by default")
	}
}

func TestCanDelegate_RoleDefaults_Coder(t *testing.T) {
	target := &orchestrator.AgentConfig{
		ID:       "mango",
		Role:     "coder",
		Provider: "ollama",
		Model:    "deepseek-v4-pro:cloud",
		// No explicit capabilities — should get role defaults
	}

	// "coder" role defaults: code, test, report
	if !hasCapability(target, CapCode) {
		t.Error("coder role should have code capability by default")
	}
	if !hasCapability(target, CapTest) {
		t.Error("coder role should have test capability by default")
	}
}

func TestTaskTypeToCapability(t *testing.T) {
	tests := []struct {
		taskType string
		want     string
	}{
		{"code", CapCode},
		{"code_implementation", CapCode},
		{"code_fix", CapCode},
		{"code_refactor", CapCode},
		{"test", CapTest},
		{"test_write", CapTest},
		{"review", CapReview},
		{"code_review", CapReview},
		{"plan", CapPlan},
		{"approve", CapApprove},
		{"research", CapSearch},
		{"report", CapReport},
		{"general", ""},
		{"unknown_type", ""},
	}

	for _, tt := range tests {
		got := taskTypeToCapability(tt.taskType)
		if got != tt.want {
			t.Errorf("taskTypeToCapability(%q) = %q, want %q", tt.taskType, got, tt.want)
		}
	}
}

func TestCanDelegate_UnknownTaskType(t *testing.T) {
	delegator := &orchestrator.AgentConfig{
		ID:           "lumi",
		Role:         "lead",
		Provider:     "ollama",
		Model:        "glm-5.1:cloud",
		Capabilities: []string{"plan", "delegate"},
	}
	target := &orchestrator.AgentConfig{
		ID:           "mango",
		Role:         "coder",
		Provider:     "ollama",
		Model:        "deepseek-v4-pro:cloud",
		Capabilities: []string{"code"},
	}

	// Unknown task types are allowed (permissive default)
	err := CanDelegate(delegator, target, "unknown_task_type")
	if err != nil {
		t.Errorf("unknown task types should be allowed: %v", err)
	}
}