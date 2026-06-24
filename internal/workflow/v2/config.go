package v2

import (
	"os"
	"encoding/json"
	"path/filepath"
)

// WorkflowConfig is the YAML/JSON configuration for a Natural Gates workflow.
type WorkflowConfig struct {
	Name              string            `json:"name"`
	Version           int               `json:"version"`
	Description       string            `json:"description"`
	Global            GlobalConfig      `json:"global"`
	Phases            []PhaseConfig     `json:"phases"`
	ConfidenceDomains []string          `json:"confidence_domains"`
	Agents            []AgentConfig     `json:"agents"`
	FastPath          *FastPathConfig   `json:"fast_path,omitempty"`
}

type GlobalConfig struct {
	MaxTotalIterations   int    `json:"max_total_iterations"`
	MaxTotalTime         string `json:"max_total_time"`
	StatePersistenceDir  string `json:"state_persistence_dir"`
	EventEmission        bool   `json:"event_emission"`
}

type PhaseConfig struct {
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	Description   string         `json:"description"`
	MaxIterations int            `json:"max_iterations"`
	AllowedTools  []string       `json:"allowed_tools"`
	Gate          GateConfig     `json:"gate"`
	Fallback      FallbackConfig `json:"fallback"`
}

type GateConfig struct {
	Type       string                 `json:"type"`
	Threshold  float64                `json:"threshold"`
	Weights    map[string]float64     `json:"weights,omitempty"`
	Domains    []string               `json:"domains,omitempty"`
	Approvers  []string               `json:"approvers,omitempty"`
	Mode       string                 `json:"mode,omitempty"`
	RequiredReviewers []string        `json:"required_reviewers,omitempty"`
	MaxWarn    int                    `json:"max_warn,omitempty"`
}

type FallbackConfig struct {
	OnMaxIterations   string `json:"on_max_iterations"`
	Blocks            bool   `json:"blocks"`
}

type AgentConfig struct {
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	Availability string   `json:"availability"`
}

type FastPathConfig struct {
	Enabled      bool   `json:"enabled"`
	RiskLevels   []string `json:"risk_levels"` // which risk levels get fast path
	SkipPhases   []string `json:"skip_phases"` // phases to skip
}

// GetPhase returns the phase config for a given phase name.
func (c *WorkflowConfig) GetPhase(name string) *PhaseConfig {
	for i, p := range c.Phases {
		if p.Name == name {
			return &c.Phases[i]
		}
	}
	return nil
}

// LoadConfig loads a workflow config from a JSON file.
func LoadConfig(path string) (*WorkflowConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config WorkflowConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// LoadConfigFromDir loads all workflow configs from a directory.
func LoadConfigFromDir(dir string) (map[string]*WorkflowConfig, error) {
	configs := make(map[string]*WorkflowConfig)
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		ext := filepath.Ext(file.Name())
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, file.Name())
		config, err := LoadConfig(path)
		if err != nil {
			continue // skip invalid configs
		}
		configs[config.Name] = config
	}
	return configs, nil
}

// NewPhaseFromConfig creates a Phase instance from config.
func NewPhaseFromConfig(cfg PhaseConfig) Phase {
	switch cfg.Type {
	case "probe":
		return &ProbePhase{AllowedToolsList: cfg.AllowedTools, MaxIter: cfg.MaxIterations}
	case "research":
		return &ResearchPhase{AllowedToolsList: cfg.AllowedTools, MaxIter: cfg.MaxIterations}
	case "plan":
		return &PlanPhase{AllowedToolsList: cfg.AllowedTools, MaxIter: cfg.MaxIterations}
	case "feedback_pre":
		return &FeedbackPrePhase{AllowedToolsList: cfg.AllowedTools, MaxIter: cfg.MaxIterations}
	case "execution":
		return &ExecutionPhase{AllowedToolsList: cfg.AllowedTools, MaxIter: cfg.MaxIterations}
	case "feedback_post":
		return &FeedbackPostPhase{AllowedToolsList: cfg.AllowedTools, MaxIter: cfg.MaxIterations}
	case "report":
		return &ReportPhase{AllowedToolsList: cfg.AllowedTools, MaxIter: cfg.MaxIterations}
	default:
		// Unknown phase type — default to a generic execution phase
		return &ExecutionPhase{AllowedToolsList: cfg.AllowedTools, MaxIter: cfg.MaxIterations}
	}
}

// NewGateFromConfig creates a Gate instance from config.
func NewGateFromConfig(cfg GateConfig) Gate {
	switch cfg.Type {
	case "assumption_threshold":
		weights := cfg.Weights
		if weights == nil {
			weights = map[string]float64{
				"blocker": 4.0,
				"high":    2.0,
				"medium":  1.0,
				"low":     0.5,
			}
		}
		return &AssumptionThresholdGate{Threshold: cfg.Threshold, Weights: weights}
	case "confidence_threshold":
		return &ConfidenceThresholdGate{Threshold: cfg.Threshold, Domains: cfg.Domains}
	case "plan_completeness":
		weights := cfg.Weights
		if weights == nil {
			weights = map[string]float64{
				"tasks_identified":     0.3,
				"resources_assigned":   0.3,
				"dependencies_ordered": 0.2,
				"success_criteria":     0.1,
				"risk_mitigation":      0.1,
			}
		}
		return &PlanCompletenessGate{Threshold: cfg.Threshold, Weights: weights}
	case "approval":
		return &ApprovalGate{RequiredApprovers: cfg.Approvers, Mode: cfg.Mode}
	case "task_completion":
		mode := cfg.Mode
		if mode == "" {
			mode = "all_tasks"
		}
		return &TaskCompletionGate{Mode: mode}
	case "review_pass":
		maxWarn := cfg.MaxWarn
		if maxWarn == 0 {
			maxWarn = 2
		}
		return &ReviewPassGate{RequiredReviewers: cfg.RequiredReviewers, MaxWarn: maxWarn}
	case "report_completeness":
		sections := []string{"change_summary", "proof_of_work", "impact", "next_steps", "learnings"}
		return &ReportCompletenessGate{RequiredSections: sections}
	default:
		return &TaskCompletionGate{Mode: "all_tasks"}
	}
}

// DefaultConfig returns the simplified 3-phase Natural Gates workflow configuration.
// PLAN → EXECUTE → REPORT. Proven V36 enforcement merged into EXECUTE phase.
func DefaultConfig() *WorkflowConfig {
	return &WorkflowConfig{
		Name:    "natural-gates-project-work",
		Version: 2,
		Description: "3-phase autonomous workflow: PLAN → EXECUTE → REPORT",
		Global: GlobalConfig{
			MaxTotalIterations:  40,
			MaxTotalTime:        "30m",
			StatePersistenceDir: "runs/natural-gates",
			EventEmission:       true,
		},
		ConfidenceDomains: []string{}, // not used in 3-phase mode
		Phases: []PhaseConfig{
			{
				Name: "PLAN", Type: "plan", Description: "Read project state, identify task, create plan",
				MaxIterations: 5,
				AllowedTools: []string{"read_file", "list_dir", "search_files", "project_overview", "git_status", "git_log", "git_branch_list"},
				Gate: GateConfig{Type: "plan_completeness", Threshold: 0.5,
					Weights: map[string]float64{"tasks_identified": 0.5, "resources_assigned": 0.5}},
				Fallback: FallbackConfig{OnMaxIterations: "proceed_with_partial_plan", Blocks: false},
			},
			{
				Name: "EXECUTE", Type: "execution", Description: "Write code with branch protection, read budget, commit-push enforcement",
				MaxIterations: 25,
				AllowedTools: []string{"read_file", "write_file", "list_dir", "search_files", "git_status", "git_log", "git_diff", "git_add", "git_commit", "git_push", "git_branch_list", "project_overview"},
				Gate: GateConfig{Type: "task_completion", Mode: "partial_allowed"},
				Fallback: FallbackConfig{OnMaxIterations: "proceed_with_partial_completion", Blocks: false},
			},
			{
				Name: "REPORT", Type: "report", Description: "Final report with proof of work",
				MaxIterations: 3,
				AllowedTools: []string{"read_file", "git_log", "git_status"},
				Gate: GateConfig{Type: "report_completeness"},
				Fallback: FallbackConfig{OnMaxIterations: "auto_generate", Blocks: false},
			},
		},
		Agents: []AgentConfig{
			{Name: "prism", Role: "implementation", Capabilities: []string{"write_code", "git_operations", "file_operations", "code_review"}, Provider: "ollama", Model: "glm-5.2:cloud", Availability: "online"},
			{Name: "mango", Role: "implementation-review", Capabilities: []string{"write_code", "data_structuring", "complex_computation", "code_review", "test_writing"}, Provider: "openclaw-subagent", Model: "deepseek-v4-pro:cloud", Availability: "online"},
			{Name: "lumi", Role: "architect-reviewer", Capabilities: []string{"architecture_design", "code_review", "creative_direction", "plan_approval", "agent_orchestration"}, Provider: "openclaw", Model: "deepseek-v4-pro:cloud", Availability: "online"},
		},
		FastPath: &FastPathConfig{Enabled: false}, // all tasks go through full 3-phase
	}
}