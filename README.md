# Prism — Event-Native AI Agent Platform

[![Go 1.26+](https://img.shields.io/badge/go-1.26%2B-blue)](https://go.dev/)
[![Tests: 988 passing](https://img.shields.io/badge/tests-988%20passing-brightgreen)]()
[![Packages: 54](https://img.shields.io/badge/packages-54-green)]()
[![Version: v0.26.0](https://img.shields.io/badge/version-v0.26.0-purple)]()
[![License: All Rights Reserved](https://img.shields.io/badge/license-all%20rights%20reserved-red)](./LICENSE)

> **License Notice:** Prism is source-available under an all-rights-reserved license. You may view the repository, but use, modification, distribution, or incorporation requires written permission. See [LICENSE](./LICENSE) for details.

Prism is a Go event-native AI agent platform that runs as a persistent service. Agents communicate through a NATS event bus, maintain conversation sessions, remember context across conversations, and can be monitored through a web dashboard.

**The framework controls the lifecycle. The model generates outputs inside that lifecycle.**

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                        Prism Runtime                             │
│                                                                  │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐  ┌───────────────┐    │
│  │ Session  │  │  Agent   │  │   Task    │  │   Approval    │    │
│  │ Manager  │  │  Router  │  │  Tracker  │  │    Manager    │    │
│  └────┬─────┘  └────┬─────┘  └─────┬─────┘  └──────┬───────┘    │
│       │              │             │                │            │
│  ┌────▼──────────────▼─────────────▼────────────────▼──────────┐  │
│  │                 Event Bus (NATS JetStream)                 │  │
│  │                                                            │  │
│  │  Namespaces: lumi.*  mango.*  remembrance.*  prism.*       │  │
│  └──────────┬──────────────┬───────────────┬──────────────────┘  │
│             │              │               │                     │
│  ┌──────────▼──┐  ┌───────▼──────┐  ┌─────▼──────────┐         │
│  │  Discord    │  │ Remembrance  │  │   Dashboard     │         │
│  │  Bot        │  │  (Python)    │  │   + API         │         │
│  │             │  │  Memory +    │  │   + Workflow     │         │
│  │  Receive →  │  │  Dream Cycle │  │   Editor         │         │
│  │  Respond    │  │  Graph +     │  │                  │         │
│  │             │  │  Search     │  │  6-tab UI        │         │
│  └─────────────┘  └─────────────┘  └──────────────────┘         │
└──────────────────────────────────────────────────────────────────┘
```

### Core Packages

| Package | Purpose |
|---------|---------|
| `bus/` | Embedded NATS JetStream event bus |
| `event/` | Event types + SQLite WAL store |
| `provider/` | 5 LLM providers: mock, ollama, openai, anthropic, gemini |
| `agent/` | Agent registry + lifecycle events |
| `session/` | SQLite session manager with daily/idle reset |
| `router/` | Agent routing based on config |
| `debounce/` | Per-user message deduplication |
| `stage/` | 10 pipeline stages with WAL recovery |
| `orchestrator/` | Config loading, validation, agent lifecycle |
| `remembrance/` | Go HTTP client for Remembrance memory layer |
| `delegation/` | Task delegation engine + capabilities + approval gates |
| `approval/` | Human-in-the-loop approval store |
| `task/` | SQLite task store for delegation tracking |
| `context/` | Workspace context injection (SOUL.md, AGENTS.md, etc.) |
| `vector/` | HNSW approximate nearest neighbor search |
| `adapter/` | 4 built-in adapters + SDK for custom adapters |
| `api/` | HTTP REST API (13 endpoints) + SSE event stream |
| `dashboard/` | 6-tab web UI with workflow editor |
| `editor/` | Visual workflow editor state model |
| `workflow/` | SVG diagram generators (5 types) |
| `bridge/` | Multi-Prism communication via NATS |
| `policy/` | Deterministic rule engine (allow/deny/require_approval) |
| `validation/` | Allowlisted command profiles + executor |
| `review/` | Deterministic review artifact generation |
| `projection/` | CQRS state projections (4 built-in) |
| `mutation/` | Gated file mutations with audit trail |
| `retry/` | Exponential backoff with jitter |
| `safety/` | Path validation, symlink protection |
| `cost/` | Token usage tracking + pricing |
| `sse/` | Server-Sent Events decoder |

---

## Quick Start

### Prerequisites

- **Go 1.26+**
- **Ollama** running locally or cloud access (`ollama serve`)
- **Python 3.10+** (optional, for Remembrance memory layer)

### Install

```bash
git clone https://github.com/emaharmony/prism.git
cd prism
go build -o prism ./cmd/prism-cli/
```

### Run Tests

```bash
go test ./...
```

### Start Prism

```bash
# 1. Create a config (or copy the example)
cp prism.yaml.example prism.yaml

# 2. Start the persistent daemon
./prism serve

# 3. Open the dashboard
open http://localhost:8322
```

### Start with Memory (Remembrance)

```bash
# Terminal 1: Start Remembrance
pip install -e ".[nats]"  # in the memory-mcp-server repo
python -m rememberance_mcp.serve --port 8788 --nats nats://localhost:4222

# Terminal 2: Start Prism
./prism serve
```

In `prism.yaml`, enable Remembrance:
```yaml
remembrance:
  enabled: true
  url: "http://localhost:8788"
```

### One-Shot CLI Mode

```bash
# Single LLM call (no daemon needed)
./prism run --prompt "Explain event-driven architecture" --provider ollama

# With tool execution
./prism run --prompt "Read main.go and summarize it" --tools read_file --allow-tools

# With approval gate
./prism run --prompt "Fix the bug" --tools write_file --require-approval
```

---

## Use Cases

### 1. Persistent AI Assistant with Discord
Run Prism as a daemon connected to Discord. Agents maintain sessions, remember context across conversations, and respond in real time with streaming.

```yaml
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
```

### 2. Multi-Agent Delegation
Lumi plans and delegates tasks to Mango. Tasks flow through the event bus with full lifecycle tracking and approval gates.

```yaml
agents:
  - id: lumi
    role: "lead"
    capabilities: [plan, delegate, review, approve]
  - id: mango
    role: "developer"
    capabilities: [code, test, delegate]
```

### 3. Memory-Aware Conversations
Remembrance captures agent output, extracts entities, builds a knowledge graph, and injects relevant context into future conversations. The dream cycle maintains the knowledge base automatically.

### 4. Visual Workflow Design
The dashboard includes a drag-and-drop SVG editor for designing agent topologies. Edit nodes, draw edges, and write back to `prism.yaml`.

### 5. Platform Integration
The HTTP API (13 REST endpoints + SSE event stream) allows external tools to monitor agents, approve requests, and query the knowledge graph.

### 6. Approval-Gated Operations
Write files, run commands, or make changes that require human approval through Discord reactions or the dashboard.

---

## CLI Reference

### Daemon Mode

```bash
prism serve [--config prism.yaml] [--port 8321]
```

### One-Shot Mode

```bash
prism run --prompt "..." --provider ollama --model llama3
prism run --workflow analyze --prompt "..."
prism run --tools write_file,read_file --allow-tools --prompt "..."
prism run --require-approval --prompt "..."
```

### Management

```bash
prism status                    # System status
prism health                    # Check NATS bus
prism dashboard                 # Open web dashboard

# Agents
prism agent list                # List registered agents
prism agent show <id>           # Agent details

# Approvals
prism approval list             # Pending approvals
prism approval show <id>        # Approval details
prism approval approve <id>     # Approve a mutation
prism approval deny <id>        # Deny a mutation

# Tools
prism tool list                  # Available tools
prism tool run <name>            # Execute a tool

# Validation
prism validation list            # Validation profiles
prism validation run <profile>   # Run validation

# Policy
prism policy list                # Policy rules
prism policy evaluate            # Evaluate a request

# Other
prism adapter list               # Registered adapters
prism adapter health <name>      # Adapter health check
prism projection list            # State projections
prism workflow list              # Registered workflows
prism context build              # Build workspace context
prism cost                       # Token usage tracking
```

---

## Configuration

### prism.yaml

```yaml
prism:
  nats_url: ""                    # empty = embedded NATS
  data_dir: ".prism/data"
  port: 8321                      # health check port
  log_level: "info"
  context_token_budget: 4000      # max tokens for workspace injection

agents:
  - id: lumi
    role: "lead"
    provider: "ollama"
    model: "glm-5.1:cloud"
    primary: true
    context: [soul, agents, user]  # which workspace files to inject
    capabilities: [plan, delegate, review, approve]
    subscriptions: []               # NATS subjects to listen on

channels:
  - type: "discord"
    token: "<bot-token>"
    channels: []                   # empty = all channels

sessions:
  max_context_messages: 100
  idle_timeout_minutes: 30
  compaction_strategy: "truncate"
  daily_reset_hour: 4

actions: []                         # event-triggered actions

remembrance:
  enabled: false
  url: "http://localhost:8788"
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENAI_API_KEY` | — | OpenAI provider key |
| `ANTHROPIC_API_KEY` | — | Anthropic provider key |
| `GEMINI_API_KEY` | — | Gemini provider key |
| `NATS_URL` | `nats://127.0.0.1:4222` | NATS server URL (empty = embedded) |
| `PRISM_DATA_DIR` | `./runs` | Run output directory |
| `REMEMBRANCE_HOME` | `~/.remembrance` | Remembrance data directory |

---

## Key Design Decisions

- **Events are the source of truth** — every meaningful action becomes a canonical event
- **Per-agent namespaces** — `lumi.agent.output`, `mango.task.created`, `remembrance.dream.triggered`
- **Deterministic policy** — no LLM-based approval, rules decide allow/deny/require_approval
- **Single binary** — no external DB required (SQLite + embedded NATS)
- **SSE streaming** — 50ms batching, spec-compliant, all 5 providers
- **HNSW vector search** — O(log n) approximate nearest neighbor, pure Go
- **Shared transport** — connection pooling across all LLM providers
- **WAL crash recovery** — every stage checkpointed, idempotent replay
- **Remembrance is separate** — Python service, HTTP + NATS, not embedded
- **Dream cycle** — nightly 3AM + event-triggered after 10 PERSIST captures

---

## Version History

| Version | What | PR |
|---------|------|----|
| V1 | Foundation: CLI, events, integration tests | #1 |
| V2 | Real LLM execution: provider interface, Ollama | — |
| V3 | Controlled tool execution: registry, policy, built-ins | — |
| V4 | Approval-gated mutations: propose, approve, apply | — |
| V5 | Validation + review pipeline | #8 |
| V6 | Gate system → moved to AI-Hedge-Prism | — |
| V7 | Workflow runtime: compose capabilities as named workflows | #13 |
| V8 | Core policy engine: declarative rules, evaluator | #14 |
| V9 | Adapter contract system: manifest, registry, lifecycle | #15 |
| V10 | State projections: CQRS query layer | #16 |
| V11 | Dashboard: web UI for runs, events, approvals | #17 |
| V12 | Architectural refactor: CLI split, safety consolidation | #18 |
| V13 | Multi-agent orchestration through events | #19 |
| V14a-e | Pipeline stages, crash recovery, providers, Discord, SQLite | #20-25 |
| V15 | Vector search with pluggable embeddings | #26 |
| V16 | Intelligence arc: context injection, cost tracking | — |
| V17 | Performance: HNSW index, connection pooling, event indexes | #30 |
| V18 | OpenClaw config transfer | — |
| V19 | Smart context injection | — |
| V20 | Live orchestrator: `prism serve`, Discord bot, sessions | #31 |
| V21 | Full conversation pipeline with streaming | #32 |
| V22 | Multi-agent delegation, capabilities, approval gates | #34 |
| V23 | Platform: HTTP API, dashboard v2, bridge, adapter SDK | — |
| V24 | Visual representations: 5 SVG diagram types | — |
| V25 | Visual workflow editor: drag-and-drop SVG | — |
| V26 | Remembrance integration: memory, dream cycle, context caching | #35 |

See [docs/](./docs/) for detailed design documents for each version.

---

## Dependencies

**Direct (6):**
- `modernc.org/sqlite` — pure-Go SQLite driver
- `github.com/gofrs/flock` — File locking
- `github.com/nats-io/nats-server/v2` — Embedded NATS
- `github.com/nats-io/nats.go` — NATS client
- `github.com/oklog/ulid` — Unique IDs
- `gopkg.in/yaml.v3` — YAML parsing

**Zero SDKs. Zero frameworks. Zero external databases.**

---

## Testing

```bash
# All tests (988 tests, 54 packages)
go test ./...

# Specific package
go test ./internal/stage/... -v

# With race detector
go test ./internal/stage/... -race

# Benchmarks
go test ./internal/vector/... -bench=.
```

---

## Project Status

Prism is a source-available AI agent platform at v0.26.0. The core runtime is stable with persistent daemon mode, Discord integration, multi-agent orchestration, memory, and a web dashboard.

**Current focus:** Streaming responses, cron scheduling, OpenClaw migration tool.

See [ROADMAP.md](./docs/ROADMAP.md) for phase milestones and [TASKS.md](./docs/TASKS.md) for granular task status.

---

## License

All rights reserved. See [LICENSE](./LICENSE) for details.
