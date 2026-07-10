# V20 — Live Orchestrator Design

**Date:** 2026-05-20
**Status:** Complete (merged to main)
**PR:** #31

---

## Thesis

Prism transitions from a batch CLI tool to a persistent live service. Instead of running `prism run` for individual tasks, `prism serve` starts a daemon that connects to Discord, maintains sessions, and responds to conversations in real time.

---

## What Changed

### M1.1: Persistent Daemon (`prism serve`)
- Single `prism serve` subcommand — not a separate binary
- Graceful shutdown on SIGINT/SIGTERM
- Health check endpoint on configured port
- Orchestrator lifecycle management

### M1.2: Per-Agent Event Namespaces
- Dynamic agent namespaces: `<agent-id>.agent.output`, `<agent-id>.agent.started`, etc.
- Auto-generated agent IDs from config (e.g., `lumi`, `mango`)
- `internal/agentns/` package for namespace generation

### M1.3: Session Manager
- SQLite-backed session store
- Conversation continuity across messages
- Daily reset at configurable hour (default: 4 AM)
- Idle timeout with configurable minutes (default: 30)
- Compaction strategy: truncate in Phase 1

### M1.4: Agent Router
- Route messages to the correct agent based on config
- `primary: true` agent as fallback
- `internal/router/` package

### M1.5-M1.6: Discord Adapter
- Inbound: Discord messages → `*.channel.received` events
- Outbound: `*.channel.sent` → Discord messages
- Bot token from `prism.yaml`
- Message splitting for Discord's 2000-char limit
- Threading/reply support

### M1.7: Registered Actions
- `internal/action/registry.go` — event-triggered actions
- Wildcard matching: `*.tool.completed`, `**.failed`
- Configuration in `prism.yaml` actions section

### M1.8: End-to-End Test
- Config → agents → session → router → action flow verified
- Discord bot adapter instantiation tested
- Session compaction tested

---

## Key Decisions

- **`prism serve` as CLI subcommand**: Single binary, consistent with Docker model
- **`discordgo` for Discord**: Chosen for safety and reliability
- **`prism.yaml` config**: Separate from OpenClaw entirely
- **Session compaction**: Truncate in Phase 1, summarize with Remembrance in Phase 2
- **Primary agent**: `primary: true` in config, first agent as fallback
- **Per-user debounce**: 3-second window per user to prevent duplicate messages

---

## New Packages

| Package | Purpose |
|---------|---------|
| `internal/orchestrator/` | Config loading, validation, agent ID resolution |
| `internal/session/` | SQLite session manager with daily/idle reset |
| `internal/router/` | Agent routing based on config |
| `internal/debounce/` | Per-user message deduplication |
| `internal/agentns/` | Dynamic agent event namespace generation |
| `internal/action/` | Event-triggered action registry |

---

## Configuration

```yaml
prism:
  nats_url: ""
  data_dir: ".prism/data"
  port: 8321

agents:
  - id: lumi
    role: "lead"
    provider: "ollama"
    model: "glm-5.1:cloud"
    primary: true
    context: [soul, agents, user]

channels:
  - type: "discord"
    token: "<bot-token>"
    channels: []

sessions:
  max_context_messages: 100
  idle_timeout_minutes: 30
  compaction_strategy: "truncate"
  daily_reset_hour: 4

actions: []
```