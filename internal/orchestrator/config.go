// Package orchestrator provides the persistent daemon that runs Prism as a
// live service. Config holds the prism.yaml configuration.
package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

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

	// Agents defines the agents Prism should register.
	// Each agent gets its own event namespace based on its ID.
	Agents []AgentConfig `yaml:"agents"`

	// Channels defines messaging channels (Discord, Telegram, etc.).
	Channels []ChannelConfig `yaml:"channels"`

	// Actions defines event-triggered actions (webhook-style).
	Actions []ActionConfig `yaml:"actions"`

	// Sessions configures session management.
	Sessions SessionConfig `yaml:"sessions"`

	// Remembrance configures the memory service.
	Remembrance RemembranceConfig `yaml:"remembrance"`
}

// PrismConfig holds top-level service settings.
type PrismConfig struct {
	// NATSURL is the NATS server URL. Empty means embedded.
	NATSURL string `yaml:"nats_url"`

	// DataDir is where SQLite databases and run artifacts are stored.
	DataDir string `yaml:"data_dir"`

	// Workspace is the root directory for context injection (SOUL.md, AGENTS.md, etc.).
	// Defaults to $HOME/.openclaw/workspace if empty.
	Workspace string `yaml:"workspace"`

	// ContextTokenBudget is the max tokens for workspace context injection.
	// Default: 4000. Higher = more context but less room for conversation.
	ContextTokenBudget int `yaml:"context_token_budget"`

	// Port is the health check server port. Default 8321.
	Port int `yaml:"port"`

	// LogLevel sets verbosity: debug, info, warn, error.
	LogLevel string `yaml:"log_level"`
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

	// Primary marks this agent as the default for unaddressed messages.
	Primary bool `yaml:"primary"`

	// Capabilities lists what this agent can do.
	Capabilities []string `yaml:"capabilities"`

	// Subscriptions lists NATS subjects this agent subscribes to
	// for receiving delegated tasks and results.
	// e.g., "mango.task.created" — Mango receives tasks from Lumi.
	Subscriptions []string `yaml:"subscriptions"`
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
}

// RemembranceConfig configures the memory service connection.
type RemembranceConfig struct {
	// Enabled controls whether Remembrance integration is active.
	Enabled bool `yaml:"enabled"`

	// URL is the Remembrance service URL.
	URL string `yaml:"url"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Prism: PrismConfig{
			NATSURL:  "",
		Port:     8321,
			DataDir:  filepath.Join(os.Getenv("HOME"), ".prism", "data"),
			LogLevel: "info",
		ContextTokenBudget: 4000,
		},
		Sessions: SessionConfig{
			IdleTimeoutMinutes: 30,
			DailyResetHour:     4,
			MaxContextMessages: 100,
			CompactionStrategy: "truncate",
		},
		Remembrance: RemembranceConfig{
			Enabled: false,
			URL:     "http://localhost:18790",
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

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Resolve auto-generated IDs and validate in a single pass
	if err := cfg.ResolveAndValidate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// agentIDPattern enforces alphanumeric + hyphens, no dots.
// This matches agent.Agent's naming rule and prevents ambiguity in event namespaces.
var agentIDPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func isValidAgentID(id string) bool {
	return agentIDPattern.MatchString(id)
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