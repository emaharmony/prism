package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/scheduler"
)

// This file implements the dashboard config + scheduler editors. Writes are
// surgical (orchestrator.SetYAMLPath) so prism.yaml keeps its comments and
// untouched sections, and validated (orchestrator.ValidateAndWrite) before they
// land. Changes apply on the next `prism serve` restart.

const defaultSchedulerEvent = "prism.task.scheduled"

// --- GET /api/v1/config ------------------------------------------------------

type settingsView struct {
	InstanceID     string `json:"instance_id"`
	Workspace      string `json:"workspace"`
	OllamaURL      string `json:"ollama_url"`
	Port           int    `json:"port"`
	LogLevel       string `json:"log_level"`
	WorkflowConfig string `json:"workflow_config"`

	SchedulerEnabled bool `json:"scheduler_enabled"`

	AutopatchEnabled     bool     `json:"autopatch_enabled"`
	AutopatchMode        string   `json:"autopatch_mode"`
	AutopatchMaxAttempts int      `json:"autopatch_max_attempts"`
	AutopatchWorkerOrder []string `json:"autopatch_worker_order"`
	AutopatchLocalAgent  string   `json:"autopatch_local_agent"`
	AutopatchBaseBranch  string   `json:"autopatch_base_branch"`

	RemembranceEnabled bool   `json:"remembrance_enabled"`
	RemembranceURL     string `json:"remembrance_url"`
	RemembranceTimeout int    `json:"remembrance_timeout_seconds"`
}

type jobView struct {
	Name     string         `json:"name"`
	Schedule string         `json:"schedule"`
	Event    string         `json:"event"`
	Action   string         `json:"action"`
	Enabled  bool           `json:"enabled"`
	Payload  map[string]any `json:"payload,omitempty"`
}

type configResponse struct {
	Settings           settingsView `json:"settings"`
	SchedulerEnabled   bool         `json:"scheduler_enabled"`
	Jobs               []jobView    `json:"jobs"`
	ConfigPath         string       `json:"config_path"`
	WorkflowConfigPath string       `json:"workflow_config_path"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.configPath == "" {
		writeJSONError(w, "config editing not available (no config path)", http.StatusBadRequest)
		return
	}
	cfg, err := orchestrator.LoadConfig(s.configPath)
	if err != nil {
		writeJSONError(w, "load config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	resp := configResponse{
		Settings: settingsView{
			InstanceID:           cfg.Prism.InstanceID,
			Workspace:            cfg.Prism.Workspace,
			OllamaURL:            cfg.Prism.OllamaURL,
			Port:                 cfg.Prism.Port,
			LogLevel:             cfg.Prism.LogLevel,
			WorkflowConfig:       cfg.Prism.WorkflowConfig,
			SchedulerEnabled:     cfg.Prism.Scheduler.Enabled,
			AutopatchEnabled:     cfg.Autopatch.Enabled,
			AutopatchMode:        cfg.Autopatch.Mode,
			AutopatchMaxAttempts: cfg.Autopatch.MaxAttempts,
			AutopatchWorkerOrder: cfg.Autopatch.WorkerOrder,
			AutopatchLocalAgent:  cfg.Autopatch.LocalAgent,
			AutopatchBaseBranch:  cfg.Autopatch.BaseBranch,
			RemembranceEnabled:   cfg.Remembrance.Enabled,
			RemembranceURL:       cfg.Remembrance.URL,
			RemembranceTimeout:   cfg.Remembrance.TimeoutSeconds,
		},
		SchedulerEnabled:   cfg.Prism.Scheduler.Enabled,
		ConfigPath:         s.configPath,
		WorkflowConfigPath: s.workflowConfigPath,
	}
	for _, j := range cfg.Prism.Scheduler.Jobs {
		action, _ := j.Payload["action"].(string)
		resp.Jobs = append(resp.Jobs, jobView{
			Name: j.Name, Schedule: j.Schedule, Event: j.Event,
			Action: action, Enabled: j.Enabled, Payload: j.Payload,
		})
	}
	writeJSON(w, resp)
}

// --- PUT /api/v1/config/settings ---------------------------------------------

// settingsPatch is a partial update: only non-nil fields are written, so the UI
// can send just what changed and every other key in prism.yaml is untouched.
type settingsPatch struct {
	InstanceID     *string `json:"instance_id"`
	Workspace      *string `json:"workspace"`
	OllamaURL      *string `json:"ollama_url"`
	Port           *int    `json:"port"`
	LogLevel       *string `json:"log_level"`
	WorkflowConfig *string `json:"workflow_config"`

	SchedulerEnabled *bool `json:"scheduler_enabled"`

	AutopatchEnabled     *bool    `json:"autopatch_enabled"`
	AutopatchMode        *string  `json:"autopatch_mode"`
	AutopatchMaxAttempts *int     `json:"autopatch_max_attempts"`
	AutopatchWorkerOrder []string `json:"autopatch_worker_order"`
	AutopatchLocalAgent  *string  `json:"autopatch_local_agent"`
	AutopatchBaseBranch  *string  `json:"autopatch_base_branch"`

	RemembranceEnabled *bool   `json:"remembrance_enabled"`
	RemembranceURL     *string `json:"remembrance_url"`
	RemembranceTimeout *int    `json:"remembrance_timeout_seconds"`
}

func (s *Server) handleConfigSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.configPath == "" {
		writeJSONError(w, "config editing not available (no config path)", http.StatusBadRequest)
		return
	}
	var p settingsPatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&p); err != nil {
		writeJSONError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Light validation of enum-ish fields before touching the file.
	if p.AutopatchMode != nil && *p.AutopatchMode != "propose" && *p.AutopatchMode != "pr" {
		writeJSONError(w, `autopatch_mode must be "propose" or "pr"`, http.StatusBadRequest)
		return
	}

	src, err := os.ReadFile(s.configPath)
	if err != nil {
		writeJSONError(w, "read config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Each edit is one surgical SetYAMLPath, threading the bytes through.
	edits := []struct {
		path []string
		val  any
		set  bool
	}{
		{[]string{"prism", "instance_id"}, deref(p.InstanceID), p.InstanceID != nil},
		{[]string{"prism", "workspace"}, deref(p.Workspace), p.Workspace != nil},
		{[]string{"prism", "ollama_url"}, deref(p.OllamaURL), p.OllamaURL != nil},
		{[]string{"prism", "port"}, derefInt(p.Port), p.Port != nil},
		{[]string{"prism", "log_level"}, deref(p.LogLevel), p.LogLevel != nil},
		{[]string{"prism", "workflow_config"}, deref(p.WorkflowConfig), p.WorkflowConfig != nil},
		{[]string{"prism", "scheduler", "enabled"}, derefBool(p.SchedulerEnabled), p.SchedulerEnabled != nil},
		{[]string{"autopatch", "enabled"}, derefBool(p.AutopatchEnabled), p.AutopatchEnabled != nil},
		{[]string{"autopatch", "mode"}, deref(p.AutopatchMode), p.AutopatchMode != nil},
		{[]string{"autopatch", "max_attempts"}, derefInt(p.AutopatchMaxAttempts), p.AutopatchMaxAttempts != nil},
		{[]string{"autopatch", "worker_order"}, p.AutopatchWorkerOrder, p.AutopatchWorkerOrder != nil},
		{[]string{"autopatch", "local_agent"}, deref(p.AutopatchLocalAgent), p.AutopatchLocalAgent != nil},
		{[]string{"autopatch", "base_branch"}, deref(p.AutopatchBaseBranch), p.AutopatchBaseBranch != nil},
		{[]string{"remembrance", "enabled"}, derefBool(p.RemembranceEnabled), p.RemembranceEnabled != nil},
		{[]string{"remembrance", "url"}, deref(p.RemembranceURL), p.RemembranceURL != nil},
		{[]string{"remembrance", "timeout_seconds"}, derefInt(p.RemembranceTimeout), p.RemembranceTimeout != nil},
	}
	for _, e := range edits {
		if !e.set {
			continue
		}
		src, err = orchestrator.SetYAMLPath(src, e.path, e.val)
		if err != nil {
			writeJSONError(w, fmt.Sprintf("edit %s: %v", strings.Join(e.path, "."), err), http.StatusInternalServerError)
			return
		}
	}

	if err := orchestrator.ValidateAndWrite(s.configPath, src); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"status": "saved", "path": s.configPath, "restart_required": true})
}

// --- PUT /api/v1/config/scheduler --------------------------------------------

type schedulerJobPatch struct {
	Name     string         `json:"name"`
	Schedule string         `json:"schedule"`
	Event    string         `json:"event"`
	Action   string         `json:"action"`
	Enabled  bool           `json:"enabled"`
	Payload  map[string]any `json:"payload"`
}

type schedulerPatch struct {
	Enabled bool                `json:"enabled"`
	Jobs    []schedulerJobPatch `json:"jobs"`
}

// schedulerYAML mirrors the on-disk prism.scheduler shape for marshalling.
type schedulerYAML struct {
	Enabled bool           `yaml:"enabled"`
	Jobs    []schedulerJob `yaml:"jobs"`
}

type schedulerJob struct {
	Name     string         `yaml:"name"`
	Schedule string         `yaml:"schedule"`
	Event    string         `yaml:"event"`
	Payload  map[string]any `yaml:"payload,omitempty"`
	Enabled  bool           `yaml:"enabled"`
}

func (s *Server) handleConfigScheduler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.configPath == "" {
		writeJSONError(w, "config editing not available (no config path)", http.StatusBadRequest)
		return
	}
	var p schedulerPatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&p); err != nil {
		writeJSONError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	seen := map[string]bool{}
	out := schedulerYAML{Enabled: p.Enabled}
	for i, j := range p.Jobs {
		name := strings.TrimSpace(j.Name)
		if name == "" {
			writeJSONError(w, fmt.Sprintf("job %d: name is required", i+1), http.StatusBadRequest)
			return
		}
		if seen[name] {
			writeJSONError(w, fmt.Sprintf("duplicate job name %q", name), http.StatusBadRequest)
			return
		}
		seen[name] = true
		if _, err := scheduler.ParseCron(j.Schedule); err != nil {
			writeJSONError(w, fmt.Sprintf("job %q: invalid schedule %q: %v", name, j.Schedule, err), http.StatusBadRequest)
			return
		}
		event := strings.TrimSpace(j.Event)
		if event == "" {
			event = defaultSchedulerEvent
		}
		// Merge action into payload so presets and advanced custom payloads
		// coexist: the action dropdown sets payload.action.
		payload := map[string]any{}
		for k, v := range j.Payload {
			payload[k] = v
		}
		if action := strings.TrimSpace(j.Action); action != "" {
			payload["action"] = action
		}
		if len(payload) == 0 {
			payload = nil
		}
		out.Jobs = append(out.Jobs, schedulerJob{
			Name: name, Schedule: j.Schedule, Event: event,
			Payload: payload, Enabled: j.Enabled,
		})
	}

	src, err := os.ReadFile(s.configPath)
	if err != nil {
		writeJSONError(w, "read config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	src, err = orchestrator.SetYAMLPath(src, []string{"prism", "scheduler"}, out)
	if err != nil {
		writeJSONError(w, "edit scheduler: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := orchestrator.ValidateAndWrite(s.configPath, src); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"status": "saved", "path": s.configPath, "jobs": len(out.Jobs), "restart_required": true})
}

// --- POST /api/v1/config/cron/validate ---------------------------------------

func (s *Server) handleCronValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Schedule string `json:"schedule"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSONError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := scheduler.ParseCron(req.Schedule); err != nil {
		writeJSON(w, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"valid": true})
}

// --- GET /api/v1/config/actions ----------------------------------------------

func (s *Server) handleSchedulerActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	actions := append([]SchedulerAction(nil), s.schedulerActions...)
	sort.Slice(actions, func(i, j int) bool { return actions[i].Key < actions[j].Key })
	if actions == nil {
		actions = []SchedulerAction{}
	}
	writeJSON(w, map[string]any{"actions": actions})
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}
