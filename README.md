# Prism — Event-Native AI Agent Platform

[![Go 1.26+](https://img.shields.io/badge/go-1.26%2B-blue)](https://go.dev/)
[![Tests: 669 passing](https://img.shields.io/badge/tests-669%20passing-brightgreen)]()
[![License: All Rights Reserved](https://img.shields.io/badge/license-all%20rights%20reserved-red)](./LICENSE)

> **License Notice:** Prism is source-available under an all-rights-reserved license. You may view the repository, but use, modification, distribution, or incorporation requires written permission. See [LICENSE](./LICENSE) for details.

Prism is a Go event-native AI agent framework. Instead of hiding agent work inside prompt chains, Prism turns each meaningful step into canonical events that can be observed, replayed, audited, and extended.

**The framework controls the lifecycle. The model generates outputs inside that lifecycle.**

---

## Architecture

```
prism-cli run --workflow analyze --prompt "..."
  │
  ▼
Run Orchestrator
  │
  ├─► Event Bus (NATS JetStream embedded)
  │     └─► Adapters (Discord, Echo, Refract Track, Remembrance)
  │     └─► State Projections (RunStatus, AgentActivity, ToolHistory, Approval)
  │
  ├─► Stage Pipeline
  │     ├─ Connection → LLM → Tool → Approval → Validation → Review
  │     └─ WAL crash recovery at each stage
  │
  ├─► Policy Engine (deterministic allow/deny/require_approval)
  │
  ├─► Provider Chain (Mock → Ollama → OpenAI → Anthropic → Gemini)
  │     └─► SSE streaming with 50ms batching
  │
  └─► Artifacts
        ├─ events.jsonl    (canonical event log)
        ├─ summary.json   (run metadata)
        ├─ prompt.md      (assembled prompt)
        ├─ output.md      (LLM output)
        └─ validation/    (test results)
            └─ review.md  (deterministic review)
```

### Core Packages

| Package | Purpose |
|---------|---------|
| `bus/` | Embedded NATS JetStream event bus |
| `event/` | Event types + SQLite WAL store |
| `provider/` | 5 LLM providers: mock, ollama, openai, anthropic, gemini |
| `provider/` (transport) | Shared HTTP connection pooling |
| `agent/` | Agent registry + lifecycle events |
| `workflow/` | Multi-step workflow runner with conditions |
| `approval/` | Human-in-the-loop approval store |
| `policy/` | Deterministic rule engine (allow/deny/require_approval) |
| `mutation/` | Gated file mutations with audit trail |
| `validation/` | Allowlisted command profiles + executor |
| `review/` | Deterministic review artifact generation |
| `projection/` | CQRS state projections (4 built-in) |
| `vector/` | HNSW approximate nearest neighbor search |
| `adapter/` | 4 built-in adapters (echo, discord, refract-track, remembrance) |
| `stage/` | 7 pipeline stages with WAL recovery |
| `dashboard/` | Web UI for runs, events, approvals |
| `retry/` | Exponential backoff with jitter |
| `safety/` | Path validation, symlink protection |

### Key Design Decisions

- **Events are the source of truth** — every meaningful action becomes a canonical event
- **Deterministic policy** — no LLM-based approval, rules decide allow/deny/require_approval
- **Single binary** — no external DB required (SQLite + embedded NATS)
- **SSE streaming** — 50ms batching, spec-compliant, all 5 providers
- **HNSW vector search** — O(log n) approximate nearest neighbor, pure Go, no external DB
- **Shared transport** — connection pooling across all LLM providers (100 idle, 20 per host)
- **WAL crash recovery** — every stage checkpointed, idempotent replay

---

## Version History

| Version | What | PR |
|---------|------|----|
| V1 | Foundation: CLI, events, integration tests | #1 |
| V2 | Real LLM execution: provider interface, Ollama, prompt builder | — |
| V3 | Controlled tool execution: registry, policy, built-ins | — |
| V4 | Approval-gated mutations: propose, approve, apply | — |
| V5 | Validation + review pipeline: allowlisted commands, artifacts | #8 |
| V6 | Gate system + trading domain → moved to [AI-Hedge-Prism](https://github.com/emaharmony/ai-hedge-prism) | — |
| V7 | Workflow runtime: compose capabilities as named workflows | #13 |
| V8 | Core policy engine: declarative rules, evaluator, events | #14 |
| V9 | Adapter contract system: manifest, registry, lifecycle | #15 |
| V10 | State projections: CQRS query layer | #16 |
| V11 | Dashboard: web UI for runs, events, approvals | #17 |
| V12 | Architectural refactor: CLI split, safety consolidation | #18 |
| V13 | Multi-agent orchestration: agents collaborate through events | #19 |
| V14a-e | Pipeline stages, crash recovery, providers, Refract Track, Discord, SQLite | #20-25 |
| V15 | Vector search with pluggable embeddings | #26 |
| V16 | *(proposed — intelligence arc)* | — |
| V17 | Performance: HNSW index, connection pooling, event indexes | #30 |

See [docs/](./docs/) for detailed design documents for each version.

---

## Requirements

- **Go 1.26+**
- **Ollama** running locally for real model execution (`ollama serve`)
- **Optional:** Python 3.10+ for Remembrance development
- **Optional:** Remembrance service for memory-enabled runs

---

## Quick Start

```bash
# Clone & build
git clone https://github.com/emaharmony/prism.git
cd prism
go build -o prism ./cmd/prism-cli/
go build -o prism-bus ./cmd/prism-bus/
go build -o prism-agent ./cmd/prism-agent/

# Run tests
go test ./...

# Start the bus
./prism-bus

# Run a workflow
./prism run --workflow analyze --prompt "Analyze this codebase" --provider ollama --model llama3

# Run with tools
./prism run --workflow analyze --prompt "..." --tools write_file,read_file --allow-tools

# With approval gate
./prism run --workflow analyze --prompt "..." --require-approval

# Check status
./prawm status

# View runs
./prism dashboard
```

---

## Usage Examples

### Basic LLM Call

```bash
./prism run --prompt "What is event-driven architecture?" --provider ollama
```

### With Tool Execution

```bash
./prism run --prompt "Read main.go and summarize it" \
  --tools read_file \
  --allow-tools \
  --provider ollama
```

### With Approval Gate

```bash
# Propose a mutation, require human approval
./prism run --prompt "Fix the bug in handler.go" \
  --tools write_file \
  --require-approval \
  --provider ollama

# Review and approve
./prism approval list
./prism approval approve <approval-id>
```

### With Validation

```bash
# After approval, validate with go test
./prism approval approve <id> --validate go-test
```

### Multi-Agent

```yaml
# agents.yaml
agents:
  analyst:
    provider: ollama
    model: llama3
    role: "Analyze the input"
  reviewer:
    provider: ollama
    model: llama3
    role: "Review the analysis"
```

```bash
./prism run --workflow review --agents agents.yaml --prompt "..."
```

### With Remembrance (Memory)

```bash
# Start Remembrance alongside Prism
remembrance-server &

# Run with memory context
./prism run --prompt "What did we decide about the architecture?" \
  --remembrance http://localhost:8788
```

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENAI_API_KEY` | — | OpenAI provider key |
| `ANTHROPIC_API_KEY` | — | Anthropic provider key |
| `GEMINI_API_KEY` | — | Gemini provider key |
| `NATS_URL` | `nats://127.0.0.1:4222` | NATS server URL |
| `PRISM_DATA_DIR` | `./runs` | Run output directory |
| `REMEMBRANCE_URL` | `http://localhost:8788` | Remembrance service URL |

### CLI Flags

```
--provider     LLM provider (mock, ollama, openai, anthropic, gemini)
--model        Model name (e.g., llama3, gpt-4o, claude-sonnet-4-20250514)
--temperature  Sampling temperature (0.0-1.0)
--timeout      Request timeout (seconds)
--dry-run      Print prompt without calling LLM
--tools        Comma-separated tool names to enable
--allow-tools  Allow tool execution without confirmation
--require-approval  Require human approval for mutations
--workflow     Named workflow to execute
--agents      Agent configuration file
--remembrance Remembrance service URL for memory context
```

---

## Dependencies

**Direct (6):**
- `github.com/mattn/go-sqlite3` — SQLite driver
- `github.com/gofrs/flock` — File locking
- `github.com/nats-io/nats-server/v2` — Embedded NATS
- `github.com/nats-io/nats.go` — NATS client
- `github.com/oklog/ulid` — Unique IDs
- `gopkg.in/yaml.v3` — YAML parsing

**Zero SDKs. Zero frameworks. Zero external databases.**

---

## Testing

```bash
# All tests (620 tests, 35 packages)
go test ./...

# Specific package
go test ./internal/vector/... -v

# With race detector
go test ./internal/vector/... -race

# Benchmarks
go test ./internal/vector/... -bench=.
```

---

## Project Status

Prism is experimental, local-first, source-available AI infrastructure. Not production-ready. See [LICENSE](./LICENSE) for details.

Current focus: local workflows, event observability, approval gates, validation, and safe orchestration patterns.

---

## License

All rights reserved. See [LICENSE](./LICENSE) for details.