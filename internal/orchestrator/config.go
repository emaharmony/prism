// Package orchestrator provides the persistent daemon that runs Prism as a
// live service. Config holds the prism.yaml configuration.
package orchestrator

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/emaharmony/prism/internal/agent"
	"gopkg.in/yaml.v3"
)

// Config represents the full Prism configuration loaded from prism.yaml.
//
// The config defines agents, channels (Discord, Telegram, etc.),
// registered actions, and service settings. It is the single source of
// truth for how Prism runs — no OpenClaw dependency.
//
// Agent IDs become event namespace prefixes. If no ID is provided,
// the system auto-generates: prism1, prism2, prism3, etc.
type Config struct {
	// Prism holds top-level service settings.
	Prism PrismConfig `yaml:"prism"`

	// API configures HTTP API authentication and CORS.
	API APIServerConfig `yaml:"api"`

	// Bridge configures signed cross-Prism protocol subjects.
	Bridge BridgeConfig `yaml:"bridge"`

	// Codex configures subscription-backed Codex CLI task delegation.
	Codex CodexConfig `yaml:"codex"`

	// ClaudeCode configures the Claude Code CLI sub-agent reviewer that can
	// fulfill gated-loop FEEDBACK_PRE/FEEDBACK_POST gates automatically.
	ClaudeCode ClaudeCodeConfig `yaml:"claude_code"`

	// Autopatch configures diagnose-and-propose patch tasks.
	Autopatch AutopatchConfig `yaml:"autopatch"`

	// FactoryMonitor configures local Roblox Factory status notifications.
	FactoryMonitor FactoryMonitorConfig `yaml:"factory_monitor"`

	// Agents defines the agents Prism should register.
	// Each agent gets its own event namespace based on its ID.
	Agents []AgentConfig `yaml:"agents"`

	// Projects defines the assignable projects the gated loop can work on.
	// Replaces hardcoded repo paths so projects are dynamic/assignable.
	Projects []ProjectConfig `yaml:"projects"`

	// Channels defines messaging channels (Discord, Telegram, etc.).
	Channels []ChannelConfig `yaml:"channels"`

	// Actions defines event-triggered actions (webhook-style).
	Actions []ActionConfig `yaml:"actions"`

	// Sessions configures session management.
	Sessions SessionConfig `yaml:"sessions"`

	// Users maps external channel identities to stable owner identities.
	Users []UserConfig `yaml:"users"`

	// Remembrance configures the memory service.
	Remembrance RemembranceConfig `yaml:"remembrance"`

	// ChannelRoles maps Discord channel IDs to role names that determine
	// which state action applies. Role names must match state_actions keys.
	ChannelRoles []ChannelRole `yaml:"channel_roles"`

	// MCPServers declares external Model Context Protocol tool servers whose tools
	// are registered into the policy-gated tool registry at serve startup.
	MCPServers []MCPServerConfig `yaml:"mcp_servers"`

	// MCPAutoApprove opts in to UNATTENDED execution of external MCP tools (skips
	// the approval gate for mcp_<server>_<tool>). Default false: MCP tools require
	// approval. Separate from any mutation auto-approve — remote tools are
	// higher-trust-risk, so enabling autonomous MCP execution is an explicit choice.
	MCPAutoApprove bool `yaml:"mcp_auto_approve"`
}

// MCPServerConfig declares one external MCP (Model Context Protocol) tool server.
// Its tools are registered as mcp_<name>_<tool> and run through the same policy
// engine as built-in tools.
type MCPServerConfig struct {
	Name    string   `yaml:"name"`    // logical server name (namespaces its tools)
	Command string   `yaml:"command"` // executable to spawn, e.g. "npx"
	Args    []string `yaml:"args"`    // arguments, e.g. ["-y","@modelcontextprotocol/server-filesystem","/repo"]
	Env     []string `yaml:"env"`     // extra KEY=VALUE environment entries
	Enabled bool     `yaml:"enabled"` // skip when false
}

// PrismConfig holds top-level service settings.
type PrismConfig struct {
	// InstanceID identifies this Prism process in cross-Prism messages.
	InstanceID string `yaml:"instance_id"`

	// NATSURL is the NATS server URL. Empty means embedded.
	NATSURL string `yaml:"nats_url"`

	// DataDir is where SQLite databases and run artifacts are stored.
	DataDir string `yaml:"data_dir"`

	// Workspace is the root directory for context injection (SOUL.md, AGENTS.md, etc.).
	// Defaults to $HOME/.openclaw/workspace if empty.
	Workspace string `yaml:"workspace"`

	// OllamaURL is the base URL for local Ollama-compatible agents.
	// Default: http://localhost:11434.
	OllamaURL string `yaml:"ollama_url"`

	// ContextTokenBudget is the max tokens for workspace context injection.
	// Default: 4000. Higher = more context but less room for conversation.
	ContextTokenBudget int `yaml:"context_token_budget"`

	// LLMTimeoutSeconds is the serve-mode timeout for each live LLM call.
	// Default: 1200 seconds (20 minutes). Local inference can be slow.
	LLMTimeoutSeconds int `yaml:"llm_timeout_seconds"`

	// Port is the health check server port. Default 8321.
	Port int `yaml:"port"`

	// BindHost is the network interface the HTTP API, health, and dashboard
	// servers bind to. Default "127.0.0.1" (loopback only). Setting a
	// non-loopback host (e.g. "0.0.0.0") exposes Prism on the network and
	// requires api.auth_token (or api.auth_token_env) to be set — Validate
	// rejects a non-loopback bind without a token.
	BindHost string `yaml:"bind_host"`

	// LogLevel sets verbosity: debug, info, warn, error.
	LogLevel string `yaml:"log_level"`

	// AllowedPaths is a list of additional directory roots the agent can access
	// beyond the workspace root. Paths are absolute or relative to CWD.
	// The workspace root is always implicitly allowed.
	// Example: ["/Users/ema/projects/repos", "/tmp/prism-data"]
	AllowedPaths []string `yaml:"allowed_paths"`

	// ReadRoots grants recursive read/search/list access beyond the workspace root.
	// When empty, AllowedPaths is used for backward compatibility.
	ReadRoots []string `yaml:"read_roots"`

	// WriteRoots grants recursive approval-gated mutation access beyond the workspace root.
	// When empty, AllowedPaths is used for backward compatibility.
	WriteRoots []string `yaml:"write_roots"`

	// Scheduler configures cron-style scheduled tasks that fire NATS events.
	// V32: Event-driven wake replaces heartbeat babysitting.
	Scheduler SchedulerConfig `yaml:"scheduler"`

	// WorkflowConfig is the path to a gated-loop workflow definition (YAML or JSON).
	// When set and loadable, it overrides the built-in 7-phase DefaultConfig.
	// See examples/workflows/gated-loop.yaml.
	WorkflowConfig string `yaml:"workflow_config"`
}

// APIServerConfig configures HTTP API authentication and CORS.
type APIServerConfig struct {
	// AuthToken is a static bearer token required on state-changing endpoints
	// (POST/PUT/DELETE/PATCH) and the SSE event stream. Empty disables auth,
	// which is only permitted when the server binds to loopback.
	AuthToken string `yaml:"auth_token"`

	// AuthTokenEnv names an environment variable holding the bearer token.
	// When set and non-empty it takes priority over AuthToken so the secret
	// can stay out of tracked config.
	AuthTokenEnv string `yaml:"auth_token_env"`

	// AllowedOrigins is the CORS origin allowlist for browser-based editors.
	// Empty means no cross-origin requests are permitted (same-origin only).
	// Use "*" only for trusted local development.
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// ResolveAuthToken returns the effective API bearer token, preferring the
// environment variable named by AuthTokenEnv over the inline AuthToken.
func (a APIServerConfig) ResolveAuthToken() string {
	if a.AuthTokenEnv != "" {
		if v := os.Getenv(a.AuthTokenEnv); v != "" {
			return v
		}
	}
	return a.AuthToken
}

// IsLoopbackHost reports whether host refers to the loopback interface.
// An empty host is treated as loopback because BindAddr defaults to 127.0.0.1.
func IsLoopbackHost(host string) bool {
	switch strings.TrimSpace(host) {
	case "", "127.0.0.1", "localhost", "::1", "[::1]":
		return true
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// BindAddr returns the host:port listen address for a server, defaulting the
// host to loopback when prism.bind_host is unset.
func (c *Config) BindAddr(port int) string {
	host := c.Prism.BindHost
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// BridgeConfig configures signed cross-Prism NATS subjects.
type BridgeConfig struct {
	// Enabled controls whether the cross-Prism protocol listener starts.
	Enabled bool `yaml:"enabled"`

	// Mode documents the topology. "shared_nats" means both Prisms use the same broker.
	Mode string `yaml:"mode"`

	// AllowedSubjects is the explicit protocol subject allowlist.
	AllowedSubjects []string `yaml:"allowed_subjects"`

	// SecretEnv names the environment variable containing the shared HMAC secret.
	SecretEnv string `yaml:"secret_env"`

	// Secret is an optional local fallback HMAC secret. Prefer SecretEnv for
	// shared or production environments so the secret stays out of tracked config.
	Secret string `yaml:"secret"`

	// LeaderInstance is the Prism instance currently allowed to coordinate a
	// cross-Prism thread. It is configurable so leadership can move between
	// environments without changing code.
	LeaderInstance string `yaml:"leader_instance"`

	// ConfidenceThreshold is the receiver-rated threshold required before a
	// task is accepted without clarification.
	ConfidenceThreshold float64 `yaml:"confidence_threshold"`

	// MaxClarificationRounds caps back-and-forth clarification turns before the
	// task needs human input.
	MaxClarificationRounds int `yaml:"max_clarification_rounds"`

	// TargetProfiles define addressable cross-Prism destinations for commands.
	TargetProfiles []BridgeTargetProfile `yaml:"target_profiles"`

	// Factory configures optional Roblox Factory task handoff for task_request messages.
	Factory FactoryBridgeConfig `yaml:"factory"`
}

// BridgeTargetProfile maps a human command target to a Prism instance and adapter.
type BridgeTargetProfile struct {
	Name         string   `yaml:"name"`
	InstanceID   string   `yaml:"instance_id"`
	Adapter      string   `yaml:"adapter"`
	Capabilities []string `yaml:"capabilities"`
}

// CodexConfig configures local Codex CLI task execution.
type CodexConfig struct {
	Enabled        bool     `yaml:"enabled"`
	Executable     string   `yaml:"executable"`
	Model          string   `yaml:"model"`
	Profile        string   `yaml:"profile"`
	Workspace      string   `yaml:"workspace"`
	Sandbox        string   `yaml:"sandbox"`
	ApprovalPolicy string   `yaml:"approval_policy"`
	TimeoutMinutes int      `yaml:"timeout_minutes"`
	MaxConcurrency int      `yaml:"max_concurrency"`
	CaptureDiff    bool     `yaml:"capture_diff"`
	ExtraArgs      []string `yaml:"extra_args"`
}

// ClaudeCodeConfig configures the Claude Code CLI sub-agent reviewer. When
// enabled, a reviewer service watches for paused gated-loop feedback gates and,
// if the gate requires the configured reviewer name, runs `claude -p` to produce
// an approve / changes_requested verdict automatically.
type ClaudeCodeConfig struct {
	Enabled        bool     `yaml:"enabled"`
	Executable     string   `yaml:"executable"`      // CLI binary (default: claude / claude.cmd)
	Model          string   `yaml:"model"`           // optional --model override
	ReviewerName   string   `yaml:"reviewer_name"`   // gate approver/reviewer name this fulfills (default: claude)
	TimeoutMinutes int      `yaml:"timeout_minutes"` // per-review timeout (default: 10)
	AllowedTools   string   `yaml:"allowed_tools"`   // --allowedTools whitelist (read-only review tools)
	ExtraArgs      []string `yaml:"extra_args"`      // additional CLI args
}

// AutopatchConfig configures the bug diagnosis and patch proposal loop.
type AutopatchConfig struct {
	Enabled              bool     `yaml:"enabled"`
	Mode                 string   `yaml:"mode"` // "propose" (patch artifact) or "pr" (open a pull request)
	RequireCleanWorktree *bool    `yaml:"require_clean_worktree"`
	MaxAttempts          int      `yaml:"max_attempts"`
	ValidationProfiles   []string `yaml:"validation_profiles"`
	WorkerOrder          []string `yaml:"worker_order"`
	LocalAgent           string   `yaml:"local_agent"`
	WorktreeRoot         string   `yaml:"worktree_root"`
	BaseBranch           string   `yaml:"base_branch"` // PR base branch in "pr" mode (empty → repo default)
}

// FactoryMonitorConfig configures local Factory queue status notifications.
type FactoryMonitorConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Root              string `yaml:"root"`
	NotifyChannelID   string `yaml:"notify_channel_id"`
	PollSeconds       int    `yaml:"poll_seconds"`
	StuckAfterMinutes int    `yaml:"stuck_after_minutes"`
}

// FactoryBridgeConfig configures report/validation-only handoff to Roblox Factory.
type FactoryBridgeConfig struct {
	Enabled            bool   `yaml:"enabled"`
	Root               string `yaml:"root"`
	Project            string `yaml:"project"`
	ProjectPath        string `yaml:"project_path"`
	ApprovalMode       string `yaml:"approval_mode"`
	RunCodex           bool   `yaml:"run_codex"`
	VisionReview       string `yaml:"vision_review"`
	PlaytestMode       string `yaml:"playtest_mode"`
	EnableUIGeneration bool   `yaml:"enable_ui_generation"`
	UIGenerationDryRun bool   `yaml:"ui_generation_dry_run"`
}

// AgentConfig defines a single agent in prism.yaml.
//
// ID becomes the event namespace prefix. If omitted, auto-generated
// as prism1, prism2, etc. No hardcoded names except "prism" for system.
type AgentConfig struct {
	// ID is the agent's unique identifier and event namespace prefix.
	// If omitted, auto-generated as prism1, prism2, etc.
	ID string `yaml:"id"`

	// Role describes what this agent does: lead, coder, researcher, etc.
	Role string `yaml:"role"`

	// Provider is the LLM provider: ollama, openai, anthropic, gemini.
	Provider string `yaml:"provider"`

	// Model is the model identifier: glm-5.1:cloud, gpt-4o, etc.
	Model string `yaml:"model"`

	// Context lists which context sources to inject: soul, agents, user, etc.
	Context []string `yaml:"context"`

	// ConversationPostfix is appended to the system prompt to shape conversation behavior.
	// It primes the model to stay present, ask follow-ups, and avoid premature closures.
	// Example: "Stay curious. Ask follow-up questions. Don't wrap up unless the topic is genuinely resolved."
	ConversationPostfix string `yaml:"conversation_postfix"`

	// Primary marks this agent as the default for unaddressed messages.
	Primary bool `yaml:"primary"`

	// Capabilities lists what this agent can do.
	Capabilities []string `yaml:"capabilities"`

	// Subscriptions lists NATS subjects this agent subscribes to
	// for receiving delegated tasks and results.
	// e.g., "mango.task.created" — Mango receives tasks from Lumi.
	Subscriptions []string `yaml:"subscriptions"`

	// ListenToAgents lists bot user IDs that this agent should respond to.
	// By default, Prism ignores messages from other bots. Adding a bot ID here
	// tells Prism to treat messages from that bot as agent-to-agent communication.
	// The message is processed with a modified prompt that frames it as peer input.
	ListenToAgents []string `yaml:"listen_to_agents"`

	// StateActions maps context names (channel roles, "agent", etc.) to prompt
	// instructions that are injected when the agent is in that state.
	// Key examples: "manager-room", "build-room", "fun", "agent".
	// The "inject" field is appended to the system prompt after conversation_postfix
	// but before tool instructions.
	StateActions map[string]StateAction `yaml:"state_actions"`
}

// ProjectConfig describes an assignable project the gated loop can work on.
// It replaces hardcoded repo paths and channel IDs so projects are dynamic.
type ProjectConfig struct {
	// ID is the project's unique identifier (e.g. "bassbook").
	ID string `yaml:"id"`

	// RepoPath is the absolute path to the project's git repository.
	RepoPath string `yaml:"repo_path"`

	// StateFile is an optional project state file the agent reads for task
	// assignment (e.g. "PROJECT_STATE.md"). Relative to RepoPath if not absolute.
	StateFile string `yaml:"state_file"`

	// DefaultBranch is the protected branch agents must not write to directly.
	// Defaults to "main" when empty.
	DefaultBranch string `yaml:"default_branch"`

	// Channel is the messaging channel ID where results/feedback are posted.
	Channel string `yaml:"channel"`

	// WorkflowConfig optionally overrides the global workflow definition for
	// this project (path to a gated-loop YAML/JSON file).
	WorkflowConfig string `yaml:"workflow_config"`

	// Orchestrator optionally names the agent ID whose model drives this
	// project's gated loop. Empty falls back to the primary agent. Use this to
	// point a project at a Claude Code (subscription) brain, e.g. an agent with
	// provider "claude_code", without changing the global default.
	Orchestrator string `yaml:"orchestrator"`

	// Default marks this project as the one used when no project is specified.
	Default bool `yaml:"default"`
}

// FindProject returns the project with the given ID, or nil if not found.
func (c *Config) FindProject(id string) *ProjectConfig {
	for i := range c.Projects {
		if c.Projects[i].ID == id {
			return &c.Projects[i]
		}
	}
	return nil
}

// DefaultProject returns the project marked default, or the first project, or nil.
func (c *Config) DefaultProject() *ProjectConfig {
	for i := range c.Projects {
		if c.Projects[i].Default {
			return &c.Projects[i]
		}
	}
	if len(c.Projects) > 0 {
		return &c.Projects[0]
	}
	return nil
}

// StateAction defines behavior modifiers for a specific context state.
type StateAction struct {
	// Inject is the text to append to the system prompt when this state is active.
	// It is inserted after conversation_postfix and before tool instructions.
	Inject string `yaml:"inject"`
}

// SchedulerConfig configures cron-style scheduled tasks.
// V32: Event-driven wake replaces heartbeat babysitting.
type SchedulerConfig struct {
	// Enabled controls whether the scheduler runs. Default: false.
	Enabled bool `yaml:"enabled"`

	// Jobs defines the scheduled tasks.
	Jobs []SchedulerJobConfig `yaml:"jobs"`
}

// SchedulerJobConfig defines a single scheduled task.
type SchedulerJobConfig struct {
	// Name is a human-readable identifier (e.g., "daily-review").
	Name string `yaml:"name"`

	// Schedule is a cron expression (minute hour dayOfMonth month dayOfWeek).
	// Example: "0 3 * * *" for daily at 3:00 AM.
	Schedule string `yaml:"schedule"`

	// Event is the NATS subject to publish when the job fires.
	// Example: "prism.task.scheduled"
	Event string `yaml:"event"`

	// Payload is the JSON payload for the event.
	Payload map[string]any `yaml:"payload"`

	// Enabled controls whether this job is active. Default: true.
	Enabled bool `yaml:"enabled"`
}

// ChannelRole maps a Discord channel ID to a role name that determines
// which state action (if any) applies when the agent is in that channel.
// V33: Includes tool filtering, personality, and structured channel context.
// Project focus is NOT hardcoded — the agent switches context dynamically
// based on conversation. The channel provides vibes, not project scope.
type ChannelRole struct {
	// ID is the Discord channel ID.
	ID string `yaml:"id"`

	// Role is the state action key to activate (e.g., "manager-room", "fun").
	Role string `yaml:"role"`

	// Tools controls which tools are available in this channel.
	// "all" = all tools, "read-only" = only read tools, "none" = no tools.
	// When empty, defaults to "all".
	Tools string `yaml:"tools,omitempty"`

	// Personality controls the agent's communication style in this channel.
	// "direct" = make decisions, push back, no menus
	// "terse" = structured data, concise, no pleasantries
	// "bubbly" = exaggerated personality, playful, enthusiastic
	// "social" = warm, conversational, present
	// When empty, uses the agent's conversation_postfix.
	Personality string `yaml:"personality,omitempty"`

	// Context is structured channel context that replaces state_actions.inject.
	// It provides rich context about where the agent is, who it's talking to,
	// and what's expected — but NOT which project (that's dynamic).
	// When empty, falls back to state_actions.inject.
	Context string `yaml:"context,omitempty"`

	// TaggedOnly means the agent only responds when directly mentioned (@Lumi).
	// When true, messages that don't mention the bot are skipped entirely.
	TaggedOnly bool `yaml:"tagged_only,omitempty"`
}

// ChannelConfig defines a messaging channel connection.
type ChannelConfig struct {
	// Type is the channel type: discord, telegram, webchat.
	Type string `yaml:"type"`

	// Token is the bot authentication token.
	Token string `yaml:"token"`

	// Channels is a list of channel/room IDs to listen on.
	Channels []string `yaml:"channels"`
}

// UserConfig maps channel-specific user IDs to one durable Prism owner ID.
type UserConfig struct {
	ID          string              `yaml:"id"`
	DisplayName string              `yaml:"display_name"`
	Default     bool                `yaml:"default"`
	Aliases     map[string][]string `yaml:"aliases"`
}

// ResolveOwnerID maps an external channel user ID to a stable owner ID.
// If no alias matches, the configured default owner is used. If no default is
// configured, Prism falls back to the external ID to avoid cross-user leakage.
func (c *Config) ResolveOwnerID(channelType, externalID string) string {
	if c == nil {
		return externalID
	}
	for _, u := range c.Users {
		if userHasAlias(u, channelType, externalID) {
			return u.ID
		}
	}
	for _, u := range c.Users {
		if u.Default && u.ID != "" {
			return u.ID
		}
	}
	return externalID
}

// OwnerAliases returns all known local and external IDs that may appear in old
// session rows for the same owner.
func (c *Config) OwnerAliases(ownerID string) []string {
	seen := map[string]bool{}
	var aliases []string
	add := func(v string) {
		if v != "" && !seen[v] {
			seen[v] = true
			aliases = append(aliases, v)
		}
	}
	add(ownerID)
	if c == nil {
		return aliases
	}
	for _, u := range c.Users {
		if u.ID != ownerID {
			continue
		}
		for _, values := range u.Aliases {
			for _, v := range values {
				add(v)
			}
		}
	}
	return aliases
}

func userHasAlias(u UserConfig, channelType, externalID string) bool {
	if u.ID == externalID {
		return true
	}
	for _, v := range u.Aliases[channelType] {
		if v == externalID {
			return true
		}
	}
	return false
}

// ResolveChannelRole returns the state action key for a given channel ID.
// Returns empty string if no role is configured for the channel.
func (c *Config) ResolveChannelRole(channelID string) string {
	cr := c.ResolveChannelRoleConfig(channelID)
	if cr != nil {
		return cr.Role
	}
	return ""
}

// ResolveChannelRoleConfig returns the full ChannelRole config for a given channel ID.
// Returns nil if no role is configured for the channel.
func (c *Config) ResolveChannelRoleConfig(channelID string) *ChannelRole {
	for i := range c.ChannelRoles {
		if c.ChannelRoles[i].ID == channelID {
			return &c.ChannelRoles[i]
		}
	}
	return nil
}

// ResolveStateAction returns the StateAction for a given key.
// It searches all agents (primary first) for a matching state action.
func (c *Config) ResolveStateAction(agentID, key string) *StateAction {
	for _, a := range c.Agents {
		if a.ID == agentID {
			if sa, ok := a.StateActions[key]; ok {
				return &sa
			}
		}
	}
	return nil
}

// ActionConfig defines an event-triggered action.
type ActionConfig struct {
	// Trigger is the event type pattern to match.
	// Supports wildcards: "*.tool.completed", "lumi.agent.output"
	Trigger string `yaml:"trigger"`

	// Action is the action to execute when the trigger fires.
	// Format: "<namespace>.<action>" e.g., "remembrance.gate.extract"
	Action string `yaml:"action"`

	// Enabled controls whether this action is active.
	Enabled bool `yaml:"enabled"`
}

// SessionConfig controls session management behavior.
type SessionConfig struct {
	// IdleTimeoutMinutes is how long before an idle session resets.
	IdleTimeoutMinutes int `yaml:"idle_timeout_minutes"`

	// DailyResetHour is the hour (0-23) at which sessions reset daily.
	DailyResetHour int `yaml:"daily_reset_hour"`

	// MaxContextMessages is the maximum messages in a session before truncation.
	MaxContextMessages int `yaml:"max_context_messages"`

	// CompactionStrategy controls how sessions are compacted.
	// "truncate" removes oldest messages (V20). "summarize" uses Remembrance (V21).
	CompactionStrategy string `yaml:"compaction_strategy"`

	// Persistence enables durable resume of prior sessions from SQLite.
	Persistence bool `yaml:"persistence"`

	// ResumeAfterIdle reuses the latest session even after IdleTimeoutMinutes.
	ResumeAfterIdle bool `yaml:"resume_after_idle"`

	// KeepArchivedMessages preserves compacted messages in SQLite instead of deleting them.
	KeepArchivedMessages bool `yaml:"keep_archived_messages"`

	// ContinuityScope controls session reuse. "owner_agent" keeps one live
	// conversation per owner and agent across channels. "channel_user" preserves
	// the historical channel+user behavior.
	ContinuityScope string `yaml:"continuity_scope"`

	// RecallWindowMode controls recent local memory bounds. "calendar_week"
	// recalls from the current week start. "rolling_days" uses ShortTermWindowDays.
	RecallWindowMode string `yaml:"recall_window_mode"`

	// RecallTimezone is the IANA timezone for calendar week recall. "Local"
	// uses the machine local timezone.
	RecallTimezone string `yaml:"recall_timezone"`

	// ShortTermWindowDays controls local recent-memory lookup across sessions.
	ShortTermWindowDays int `yaml:"short_term_window_days"`

	// VerbatimRecentMessages caps exact recent local memory injected from other
	// sessions. Current session history remains controlled by MaxContextMessages.
	VerbatimRecentMessages int `yaml:"verbatim_recent_messages"`
}

// RemembranceConfig configures the memory service connection.
type RemembranceConfig struct {
	// Enabled controls whether Remembrance integration is active.
	Enabled bool `yaml:"enabled"`

	// URL is the Remembrance service URL.
	URL string `yaml:"url"`

	// TimeoutSeconds is the HTTP timeout for Remembrance requests.
	// Default: 60 seconds. Capture may run synchronous extraction before returning.
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Prism: PrismConfig{
			InstanceID:         "prism",
			NATSURL:            "",
			Port:               8321,
			BindHost:           "127.0.0.1",
			DataDir:            filepath.Join(os.Getenv("HOME"), ".prism", "data"),
			OllamaURL:          "http://localhost:11434",
			LogLevel:           "info",
			ContextTokenBudget: 4000,
			LLMTimeoutSeconds:  1200,
		},
		Sessions: SessionConfig{
			IdleTimeoutMinutes:     30,
			DailyResetHour:         4,
			MaxContextMessages:     100,
			CompactionStrategy:     "summarize",
			Persistence:            true,
			ResumeAfterIdle:        true,
			KeepArchivedMessages:   true,
			ContinuityScope:        "owner_agent",
			RecallWindowMode:       "calendar_week",
			RecallTimezone:         "Local",
			ShortTermWindowDays:    7,
			VerbatimRecentMessages: 40,
		},
		Remembrance: RemembranceConfig{
			Enabled:        false,
			URL:            "http://localhost:18790",
			TimeoutSeconds: 60,
		},
		Bridge: BridgeConfig{
			Enabled: false,
			Mode:    "shared_nats",
			AllowedSubjects: []string{
				"prism.cross.context_sync",
				"prism.cross.task_request",
				"prism.cross.status_request",
				"prism.cross.validation_request",
				"prism.cross.task_response",
				"prism.cross.task_accept",
				"prism.cross.task_reject",
				"prism.cross.clarification",
				"prism.cross.task_progress",
				"prism.cross.task_result",
				"prism.cross.task_cancel",
			},
			LeaderInstance:         "lumi-ceo",
			ConfidenceThreshold:    0.75,
			MaxClarificationRounds: 1,
			TargetProfiles: []BridgeTargetProfile{
				{
					Name:       "generic",
					InstanceID: "astraea-manager",
					Adapter:    "generic",
					Capabilities: []string{
						"plan",
						"review",
						"report",
					},
				},
				{
					Name:       "factory",
					InstanceID: "astraea-manager",
					Adapter:    "factory",
					Capabilities: []string{
						"roblox_factory",
						"validation",
						"report",
					},
				},
				{
					Name:       "codex",
					InstanceID: "astraea-manager",
					Adapter:    "codex",
					Capabilities: []string{
						"code",
						"test",
						"review",
						"report",
					},
				},
			},
			SecretEnv: "PRISM_BRIDGE_SECRET",
			Factory: FactoryBridgeConfig{
				Enabled:            false,
				Root:               `D:\_projects_\roblox-factory`,
				Project:            "eggventura",
				ProjectPath:        `D:\Projects\Roblox\eggventura`,
				ApprovalMode:       "report_only",
				RunCodex:           false,
				VisionReview:       "none",
				PlaytestMode:       "none",
				UIGenerationDryRun: true,
			},
		},
		Codex: CodexConfig{
			Enabled:        false,
			Executable:     "",
			Model:          "",
			Profile:        "",
			Workspace:      "",
			Sandbox:        "workspace-write",
			ApprovalPolicy: "on-request",
			TimeoutMinutes: 30,
			MaxConcurrency: 1,
			CaptureDiff:    true,
		},
		Autopatch: AutopatchConfig{
			Enabled:              false,
			Mode:                 "propose",
			RequireCleanWorktree: boolPtr(true),
			MaxAttempts:          2,
			ValidationProfiles:   []string{"go_test_all"},
			WorkerOrder:          []string{"codex", "local_agent"},
			LocalAgent:           "forge",
			WorktreeRoot:         filepath.Join(".prism", "worktrees"),
		},
		FactoryMonitor: FactoryMonitorConfig{
			Enabled:           false,
			Root:              `D:\_projects_\roblox-factory`,
			PollSeconds:       30,
			StuckAfterMinutes: 30,
		},
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	// Validate agents
	seenIDs := make(map[string]bool)
	autoGenCounter := 0

	for i, a := range c.Agents {
		id := a.ID
		if id == "" {
			autoGenCounter++
			id = fmt.Sprintf("prism%d", autoGenCounter)
		}

		// Agent ID must be alphanumeric + hyphens (same rule as agent.Agent.Validate)
		if !isValidAgentID(id) {
			return fmt.Errorf("config: agent[%d] id %q must be alphanumeric + hyphens only", i, id)
		}

		if seenIDs[id] {
			return fmt.Errorf("config: duplicate agent id %q", id)
		}
		seenIDs[id] = true

		if a.Role == "" {
			return fmt.Errorf("config: agent[%d] %q missing role", i, id)
		}

		if a.Model == "" {
			return fmt.Errorf("config: agent[%d] %q missing model", i, id)
		}
	}

	// Validate channels
	for i, ch := range c.Channels {
		if ch.Type == "" {
			return fmt.Errorf("config: channel[%d] missing type", i)
		}
		if ch.Token == "" {
			return fmt.Errorf("config: channel[%d] missing token", i)
		}
	}

	// Validate actions
	for i, act := range c.Actions {
		if act.Trigger == "" {
			return fmt.Errorf("config: action[%d] missing trigger", i)
		}
		if act.Action == "" {
			return fmt.Errorf("config: action[%d] missing action", i)
		}
	}

	// Validate sessions
	if c.Sessions.MaxContextMessages < 1 {
		return fmt.Errorf("config: max_context_messages must be >= 1")
	}
	if c.Sessions.CompactionStrategy != "truncate" && c.Sessions.CompactionStrategy != "summarize" {
		return fmt.Errorf("config: compaction_strategy must be 'truncate' or 'summarize'")
	}
	if c.Sessions.ContinuityScope == "" {
		c.Sessions.ContinuityScope = "owner_agent"
	}
	if c.Sessions.ContinuityScope != "owner_agent" && c.Sessions.ContinuityScope != "channel_user" {
		return fmt.Errorf("config: continuity_scope must be 'owner_agent' or 'channel_user'")
	}
	if c.Sessions.RecallWindowMode == "" {
		c.Sessions.RecallWindowMode = "calendar_week"
	}
	if c.Sessions.RecallWindowMode != "calendar_week" && c.Sessions.RecallWindowMode != "rolling_days" {
		return fmt.Errorf("config: recall_window_mode must be 'calendar_week' or 'rolling_days'")
	}
	if c.Sessions.RecallTimezone == "" {
		c.Sessions.RecallTimezone = "Local"
	}
	if c.Sessions.RecallTimezone != "Local" {
		if _, err := time.LoadLocation(c.Sessions.RecallTimezone); err != nil {
			return fmt.Errorf("config: recall_timezone %q is invalid: %w", c.Sessions.RecallTimezone, err)
		}
	}
	if c.Sessions.ShortTermWindowDays < 0 {
		return fmt.Errorf("config: short_term_window_days must be >= 0")
	}
	if c.Sessions.VerbatimRecentMessages < 0 {
		return fmt.Errorf("config: verbatim_recent_messages must be >= 0")
	}
	if c.Prism.LLMTimeoutSeconds < 0 {
		return fmt.Errorf("config: llm_timeout_seconds must be >= 0")
	}
	if c.Remembrance.TimeoutSeconds < 0 {
		return fmt.Errorf("config: remembrance.timeout_seconds must be >= 0")
	}
	if c.Codex.Enabled {
		if c.Codex.Workspace == "" {
			c.Codex.Workspace = c.Prism.Workspace
		}
		if c.Codex.Workspace == "" {
			c.Codex.Workspace = "."
		}
		if c.Codex.Sandbox == "" {
			c.Codex.Sandbox = "workspace-write"
		}
		if c.Codex.ApprovalPolicy == "" {
			c.Codex.ApprovalPolicy = "on-request"
		}
		if c.Codex.TimeoutMinutes == 0 {
			c.Codex.TimeoutMinutes = 30
		}
		if c.Codex.MaxConcurrency == 0 {
			c.Codex.MaxConcurrency = 1
		}
		if c.Codex.TimeoutMinutes < 1 {
			return fmt.Errorf("config: codex.timeout_minutes must be >= 1")
		}
		if c.Codex.MaxConcurrency < 1 {
			return fmt.Errorf("config: codex.max_concurrency must be >= 1")
		}
		if c.Codex.Sandbox != "read-only" && c.Codex.Sandbox != "workspace-write" && c.Codex.Sandbox != "danger-full-access" {
			return fmt.Errorf("config: codex.sandbox must be 'read-only', 'workspace-write', or 'danger-full-access'")
		}
		if c.Codex.ApprovalPolicy != "untrusted" && c.Codex.ApprovalPolicy != "on-request" && c.Codex.ApprovalPolicy != "never" {
			return fmt.Errorf("config: codex.approval_policy must be 'untrusted', 'on-request', or 'never'")
		}
	}
	if c.Autopatch.Mode == "" {
		c.Autopatch.Mode = "propose"
	}
	if c.Autopatch.RequireCleanWorktree == nil {
		c.Autopatch.RequireCleanWorktree = boolPtr(true)
	}
	if c.Autopatch.MaxAttempts == 0 {
		c.Autopatch.MaxAttempts = 2
	}
	if len(c.Autopatch.ValidationProfiles) == 0 {
		c.Autopatch.ValidationProfiles = []string{"go_test_all"}
	}
	if len(c.Autopatch.WorkerOrder) == 0 {
		c.Autopatch.WorkerOrder = []string{"codex", "local_agent"}
	}
	if c.Autopatch.WorktreeRoot == "" {
		c.Autopatch.WorktreeRoot = filepath.Join(".prism", "worktrees")
	}
	if c.Autopatch.Enabled {
		if c.Autopatch.Mode != "propose" {
			return fmt.Errorf("config: autopatch.mode must be 'propose'")
		}
		if c.Autopatch.MaxAttempts < 1 {
			return fmt.Errorf("config: autopatch.max_attempts must be >= 1")
		}
		for _, worker := range c.Autopatch.WorkerOrder {
			if worker != "codex" && worker != "local_agent" {
				return fmt.Errorf("config: autopatch.worker_order contains unknown worker %q", worker)
			}
		}
	}
	if c.FactoryMonitor.PollSeconds == 0 {
		c.FactoryMonitor.PollSeconds = 30
	}
	if c.FactoryMonitor.StuckAfterMinutes == 0 {
		c.FactoryMonitor.StuckAfterMinutes = 30
	}
	if c.FactoryMonitor.Enabled {
		if c.FactoryMonitor.Root == "" {
			return fmt.Errorf("config: factory_monitor.root is required when factory monitor is enabled")
		}
		if c.FactoryMonitor.NotifyChannelID == "" {
			return fmt.Errorf("config: factory_monitor.notify_channel_id is required when factory monitor is enabled")
		}
		if c.FactoryMonitor.PollSeconds < 1 {
			return fmt.Errorf("config: factory_monitor.poll_seconds must be >= 1")
		}
		if c.FactoryMonitor.StuckAfterMinutes < 1 {
			return fmt.Errorf("config: factory_monitor.stuck_after_minutes must be >= 1")
		}
	}
	if c.Prism.InstanceID != "" && !isValidAgentID(c.Prism.InstanceID) {
		return fmt.Errorf("config: prism.instance_id %q must be alphanumeric + hyphens only", c.Prism.InstanceID)
	}
	if c.Bridge.Enabled {
		if c.Prism.InstanceID == "" {
			return fmt.Errorf("config: prism.instance_id is required when bridge is enabled")
		}
		if c.Bridge.SecretEnv == "" && c.Bridge.Secret == "" {
			return fmt.Errorf("config: bridge.secret_env or bridge.secret is required when bridge is enabled")
		}
		if c.Bridge.Mode == "" {
			c.Bridge.Mode = "shared_nats"
		}
		if c.Bridge.Mode != "shared_nats" {
			return fmt.Errorf("config: bridge.mode must be 'shared_nats'")
		}
		if c.Bridge.ConfidenceThreshold < 0 || c.Bridge.ConfidenceThreshold > 1 {
			return fmt.Errorf("config: bridge.confidence_threshold must be between 0 and 1")
		}
		if c.Bridge.MaxClarificationRounds < 0 {
			return fmt.Errorf("config: bridge.max_clarification_rounds must be >= 0")
		}
		for i, profile := range c.Bridge.TargetProfiles {
			if profile.Name == "" {
				return fmt.Errorf("config: bridge.target_profiles[%d].name is required", i)
			}
			if profile.InstanceID == "" {
				return fmt.Errorf("config: bridge.target_profiles[%d].instance_id is required", i)
			}
			if profile.Adapter == "" {
				return fmt.Errorf("config: bridge.target_profiles[%d].adapter is required", i)
			}
		}
		if c.Bridge.Factory.Enabled {
			if c.Bridge.Factory.Root == "" {
				return fmt.Errorf("config: bridge.factory.root is required when factory bridge is enabled")
			}
			if c.Bridge.Factory.Project == "" {
				return fmt.Errorf("config: bridge.factory.project is required when factory bridge is enabled")
			}
			if c.Bridge.Factory.ApprovalMode == "" {
				c.Bridge.Factory.ApprovalMode = "report_only"
			}
			if c.Bridge.Factory.ApprovalMode != "report_only" && c.Bridge.Factory.ApprovalMode != "implementation" {
				return fmt.Errorf("config: bridge.factory.approval_mode must be 'report_only' or 'implementation'")
			}
			if c.Bridge.Factory.RunCodex && c.Bridge.Factory.ApprovalMode != "implementation" {
				return fmt.Errorf("config: bridge.factory.run_codex=true requires approval_mode='implementation'")
			}
		}
	}

	// Network exposure: binding to a non-loopback interface without a bearer
	// token would expose unauthenticated state-changing endpoints (approvals,
	// editor save) to the network. Fail closed.
	if !IsLoopbackHost(c.Prism.BindHost) && c.API.ResolveAuthToken() == "" {
		return fmt.Errorf("config: prism.bind_host %q is not loopback; set api.auth_token or api.auth_token_env to expose the API on the network", c.Prism.BindHost)
	}

	return nil
}

// ResolveAndValidate resolves auto-generated agent IDs and validates the config.
// This is a single-pass operation — resolve first, then validate — to avoid
// the fragile two-pass approach where IDs might differ between passes.
func (c *Config) ResolveAndValidate() error {
	// Resolve auto-generated IDs first
	autoGenCounter := 0
	for i := range c.Agents {
		if c.Agents[i].ID == "" {
			autoGenCounter++
			c.Agents[i].ID = fmt.Sprintf("prism%d", autoGenCounter)
		}
	}

	// Now validate with resolved IDs
	return c.Validate()
}

// PrimaryAgent returns the agent marked as primary, or the first agent if none
// is explicitly marked. Returns nil if no agents are configured.
func (c *Config) PrimaryAgent() *AgentConfig {
	for i, a := range c.Agents {
		if a.Primary {
			return &c.Agents[i]
		}
	}
	// Fallback: first agent
	if len(c.Agents) > 0 {
		return &c.Agents[0]
	}
	return nil
}

// LoadConfig reads a prism.yaml file and returns the parsed Config.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return LoadConfigFromBytes(data)
}

// LoadConfigFromBytes parses config from raw bytes (for testing).
func LoadConfigFromBytes(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.ResolveEnv()

	// Resolve auto-generated IDs and validate in a single pass
	if err := cfg.ResolveAndValidate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ResolveEnv expands ${VAR} references for secret-bearing config fields.
func (c *Config) ResolveEnv() {
	for i := range c.Channels {
		c.Channels[i].Token = os.ExpandEnv(c.Channels[i].Token)
	}
	c.Bridge.Secret = os.ExpandEnv(c.Bridge.Secret)
	c.Codex.Executable = os.ExpandEnv(c.Codex.Executable)
	c.Codex.Workspace = os.ExpandEnv(c.Codex.Workspace)
	c.FactoryMonitor.Root = os.ExpandEnv(c.FactoryMonitor.Root)
	expandList(c.Prism.AllowedPaths)
	expandList(c.Prism.ReadRoots)
	expandList(c.Prism.WriteRoots)
}

// EffectiveReadRoots returns configured recursive read roots. New read_roots
// takes precedence; allowed_paths remains the legacy alias.
func (c *Config) EffectiveReadRoots() []string {
	if c == nil {
		return nil
	}
	if len(c.Prism.ReadRoots) > 0 {
		return append([]string(nil), c.Prism.ReadRoots...)
	}
	return append([]string(nil), c.Prism.AllowedPaths...)
}

// EffectiveWriteRoots returns configured recursive approval-gated write roots.
// New write_roots takes precedence; allowed_paths remains the legacy alias.
func (c *Config) EffectiveWriteRoots() []string {
	if c == nil {
		return nil
	}
	if len(c.Prism.WriteRoots) > 0 {
		return append([]string(nil), c.Prism.WriteRoots...)
	}
	return append([]string(nil), c.Prism.AllowedPaths...)
}

func expandList(values []string) {
	for i := range values {
		values[i] = os.ExpandEnv(values[i])
	}
}

// agentIDPattern enforces alphanumeric + hyphens, no dots.
// This matches agent.Agent's naming rule and prevents ambiguity in event namespaces.
var agentIDPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func isValidAgentID(id string) bool {
	return agentIDPattern.MatchString(id)
}

func boolPtr(v bool) *bool {
	return &v
}

// RegisterAgents adds all configured agents to the given registry.
// Auto-generated IDs (prism1, prism2, etc.) are already resolved.
func (c *Config) RegisterAgents(registry *agent.Registry) error {
	for _, agentCfg := range c.Agents {
		a := &agent.Agent{
			Name:         agentCfg.ID,
			Role:         agentCfg.Role,
			Version:      "1.0.0",
			ProviderName: agentCfg.Provider,
			Model:        agentCfg.Model,
		}
		// If no capabilities are specified, add a default based on role
		if len(agentCfg.Capabilities) == 0 {
			a.Capabilities = []agent.AgentCapability{
				{Action: agentCfg.Role, Description: agentCfg.Role},
			}
		} else {
			for _, cap := range agentCfg.Capabilities {
				a.Capabilities = append(a.Capabilities, agent.AgentCapability{
					Action:      cap,
					Description: cap,
				})
			}
		}
		if err := registry.Register(a); err != nil {
			return fmt.Errorf("register agent %q: %w", agentCfg.ID, err)
		}
	}
	return nil
}
