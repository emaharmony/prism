# Prism — Event-Native AI Agent Platform

Prism is a Go/Python event-native AI agent framework. Instead of hiding agent work inside prompt chains, Prism turns each meaningful step of an AI workflow into canonical events that can be observed, replayed, audited, and extended through hooks. V1 proved the task/event lifecycle. V2 adds real single-agent LLM execution through a provider interface while preserving durable event logs, correlation IDs, and run artifacts.

## V2 Status

V2 focuses on real single-agent LLM execution inside the existing V1 event lifecycle.

**Included in V2:**
- Single-agent LLM execution
- Provider interface (abstract, swappable)
- Mock provider for deterministic tests
- Ollama provider for local model execution
- Prompt builder with deterministic `prompt.md` artifacts
- Optional Remembrance context injection (graceful or strict)
- LLM lifecycle events (`prism.llm.requested`, `prism.llm.completed`, `prism.llm.failed`)
- Context injection events (`prism.context.requested`, `prism.context.injected`, `prism.context.failed`)
- Run artifacts: `events.jsonl`, `summary.json`, `prompt.md`, `output.md`
- `--dry-run-prompt` mode for prompt inspection without LLM calls
- Provider/model metadata in `summary.json`
- Parent chain integrity across success and failure paths

**Not included yet:**
- Multi-agent orchestration
- Tool execution and approval gates
- Dashboard
- Discord/Telegram channel workflows
- OpenClaw migration
- Cron scheduler
- State manager / CQRS
- ACP/A2A gateway
- Full autonomous memory intelligence (Remembrance provides context injection only)

## Architecture Overview

```
prism-cli
  ↓
Run Orchestrator
  ↓
Event Bus: NATS JetStream
  ↓
Optional Remembrance Context Hook
  ↓
Prompt Builder
  ↓
Provider Interface
  ├── Mock Provider
  └── Ollama Provider
  ↓
Run Artifacts
  ├── events.jsonl
  ├── summary.json
  ├── prompt.md
  └── output.md
```

- **Go** owns runtime, events, orchestration, providers, CLI, and artifacts.
- **Python** owns Remembrance memory, vector search, embeddings, and future memory intelligence.

## Requirements

- **Go 1.22+** (repo uses Go 1.26.2)
- **Ollama** installed and running for real model execution (`ollama serve`)
- **Optional:** Python 3.11+ for Remembrance development
- **Optional:** Remembrance service for memory-enabled runs

## Quick Start

### 1. Clone & Build

```bash
git clone https://github.com/emaharmony/prism.git
cd prism

go build -o prism ./cmd/prism-cli/
go build -o prism-bus ./cmd/prism-bus/
go build -o prism-agent ./cmd/prism-agent/
```

Windows:

```powershell
go build -o prism.exe ./cmd/prism-cli/
go build -o prism-bus.exe ./cmd/prism-bus/
go build -o prism-agent.exe ./cmd/prism-agent/
```

### 2. Run Tests

```bash
go test ./...
```

55+ tests across 5 packages:
- `internal/agent` — placeholder agent and delay
- `internal/event` — IDs, event creation, JSON round-trip, security
- `internal/prompt` — prompt builder, context injection, output writing
- `internal/provider` — mock provider, failing mock, Ollama provider (httptest)
- `internal/run` — V1 lifecycle, V2 lifecycle, memory, correlation, parent chains, LLM failure

### 3. Start the Bus

```bash
./prism-bus
```

Starts an embedded NATS server with JetStream on `localhost:4222`. Data stored in `./prism-data/`.

### 4. Run with Mock Provider

```bash
./prism run \
  --task "Explain the Prism event lifecycle in 5 bullets" \
  --project prism \
  --agent lumi \
  --provider mock
```

### 5. Run with Ollama

Make sure Ollama is running with a model pulled:

```bash
ollama serve
ollama pull qwen2.5:7b
```

Then:

```bash
./prism run \
  --task "Explain the Prism event lifecycle in 5 bullets" \
  --project prism \
  --agent lumi \
  --provider ollama \
  --model qwen2.5:7b
```

### 6. Inspect Artifacts

```bash
# Find the latest run
ls runs/

# Read the durable event audit trail
cat runs/<run_id>/events.jsonl

# Run metadata, status, provider/model, duration
cat runs/<run_id>/summary.json

# The exact prompt sent to the provider
cat runs/<run_id>/prompt.md

# The model-generated output
cat runs/<run_id>/output.md
```

## Running with Ollama

Prism communicates with Ollama via the `/api/generate` endpoint (non-streaming). Configure the base URL if Ollama is not on the default:

```bash
./prism run \
  --task "Summarize this concept" \
  --provider ollama \
  --model qwen2.5:7b \
  --ollama-url http://192.168.1.100:11434 \
  --timeout 120s
```

If Ollama is unreachable or the model is not found, Prism emits `prism.llm.failed` → `prism.agent.failed` → `prism.task.failed` and exits with a non-zero code. The error is captured in `summary.json` under the `llm_error` field.

Common provider errors:

```text
ollama: HTTP 404: {"error":"model 'xyz' not found"}  → model not pulled
ollama: Post "http://localhost:11434/api/generate": ... connection refused → ollama not running
context deadline exceeded → request exceeded --timeout duration
```

## Remembrance Context Injection

Remembrance is **optional** in V2. Prism runs fine without it.

Memory modes:

- **Default:** memory disabled, no context request
- **`--memory-enabled`:** request context, continue if unavailable (graceful fallback)
- **`--require-memory`:** fail the task if context cannot be retrieved

```bash
# Graceful: inject context if available, continue anyway
./prism run --task "Analyze the codebase" \
  --memory-enabled --memory-url http://localhost:18790

# Strict: fail the task if memory is unavailable
./prism run --task "Critical analysis" \
  --memory-enabled --require-memory
```

Event behavior by mode:

```text
Memory available (--memory-enabled):
  prism.context.requested → prism.context.injected → prism.llm.requested

Memory unavailable, graceful (--memory-enabled):
  prism.context.requested → prism.context.failed → prism.llm.requested

Memory unavailable, strict (--require-memory):
  prism.context.requested → prism.context.failed → prism.task.failed
```

## Event Lifecycle

### V2 Success (with memory)

```text
prism.task.created
prism.task.started
prism.memory.context_requested    ← V1 compat
prism.context.requested          ← V2
prism.memory.context_built       ← V1 compat
prism.context.injected           ← V2
prism.agent.started
prism.llm.requested
prism.llm.completed
prism.agent.completed
prism.output.written
prism.task.completed
```

### V2 Success (without memory)

```text
prism.task.created
prism.task.started
prism.agent.started
prism.llm.requested
prism.llm.completed
prism.agent.completed
prism.output.written
prism.task.completed
```

### V2 LLM Failure

```text
prism.task.created
prism.task.started
prism.agent.started
prism.llm.requested
prism.llm.failed
prism.agent.failed
prism.task.failed
```

### V2 Dry-Run

```text
prism.task.created
prism.task.started
prism.agent.started
prism.agent.completed      ← no LLM events
prism.task.completed
```

Every event carries a `parent_id` linking it to its direct causal predecessor, forming a traceable chain from `task.created` to `task.completed/failed`.

## Run Artifacts

Each run creates a directory under `runs/`:

```text
runs/<run_id>/
  events.jsonl     ← durable event audit trail (one JSON object per line)
  summary.json     ← run metadata: status, provider, model, duration, artifact paths
  prompt.md        ← exact prompt sent to the provider
  output.md        ← model-generated output
```

- `events.jsonl` — every event in the lifecycle, with IDs, types, timestamps, payloads, and parent chains
- `summary.json` — includes V2 fields: `provider`, `model`, `prompt_path`, `output_path`, `memory_status`, `llm_latency_ms`, `llm_error`
- `prompt.md` — assembled from task, project, agent identity, and optional Remembrance context
- `output.md` — the raw text returned by the provider

## Providers

V2 introduces a provider interface so Prism is not locked to one model backend.

**Current providers:**

| Provider | Flag | Description |
|----------|------|-------------|
| `mock` | `--provider mock` | Deterministic test provider. Returns a mock response. Default. |
| `ollama` | `--provider ollama` | Local Ollama provider. POSTs to `/api/generate`. |

**Future providers** (not implemented): OpenAI, Anthropic, Gemini, GLM, Kimi, custom HTTP.

The provider interface is a single method:

```go
type Provider interface {
    Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)
}
```

Adding a new provider requires implementing `Generate()` and registering it in `cmd/prism-cli/main.go`.

## Prompt Builder

The prompt builder assembles a deterministic `prompt.md` artifact from:

- Agent identity (name)
- Project name
- User task description
- Optional Remembrance context (injected between task and rules)
- Behavior rules (follow the task, use context, don't invent, be concise)
- Output expectations

The `--dry-run-prompt` flag builds `prompt.md` and `events.jsonl` but skips the LLM call. Useful for inspecting what would be sent without spending tokens or requiring a running model.

```bash
./prism run --task "Test prompt assembly" --project prism --agent lumi --dry-run-prompt
```

## Testing

```bash
# Run all tests
go test ./...

# Run V2 lifecycle tests specifically
go test ./internal/run/... -v -run "TestV2"

# Run provider tests
go test ./internal/provider/... -v

# Run prompt builder tests
go test ./internal/prompt/... -v
```

V2 test coverage includes:
- Mock provider success and failure
- LLM lifecycle event ordering and parent chains
- Prompt and output artifact creation
- Memory-enabled graceful fallback
- `--require-memory` strict failure
- Ollama provider: success, timeout, HTTP error, model error, connection refused, token fallback
- Dry-run prompt mode

Ollama integration is tested with `httptest` mocks — a running Ollama instance is **not** required for CI.

## Health Checks

```bash
# Check NATS bus status
./prism health

# Check if Ollama is running
curl http://localhost:11434/api/tags

# Check if Remembrance is running
curl http://localhost:18790/health
```

## Development Commands

```bash
# Build all binaries
go build -o prism       ./cmd/prism-cli/
go build -o prism-bus   ./cmd/prism-bus/
go build -o prism-agent ./cmd/prism-agent/

# Run all tests
go test ./...

# Run a mock provider test
./prism run --task "Test run" --project prism --agent lumi --provider mock

# Dry-run (no LLM call)
./prism run --task "Test prompt" --project prism --agent lumi --dry-run-prompt

# Run with verbose event output
./prism run --task "Debug lifecycle" --project prism --agent lumi --provider mock
```

Windows equivalents:

```powershell
go build -o prism.exe       ./cmd/prism-cli/
go build -o prism-bus.exe   ./cmd/prism-bus/
go build -o prism-agent.exe ./cmd/prism-agent/

.\prism.exe run --task "Test run" --project prism --agent lumi --provider mock
```

## Configuration

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--task` | *(required)* | Task description |
| `--project` | `prism` | Project name |
| `--agent` | `lumi` | Agent name |
| `--bus-url` | `nats://localhost:4222` | NATS connection URL |
| `--provider` | `mock` | LLM provider: `mock` or `ollama` |
| `--model` | `mock-model` | Model name for the provider |
| `--temperature` | `0.2` | LLM sampling temperature |
| `--max-tokens` | `2048` | Max output tokens |
| `--timeout` | `60s` | LLM request timeout |
| `--dry-run-prompt` | `false` | Build prompt.md but skip LLM call |
| `--ollama-url` | `http://localhost:11434` | Ollama API base URL |
| `--memory-enabled` | `false` | Enable Remembrance context hook |
| `--require-memory` | `false` | Fail task if Remembrance is unavailable |
| `--memory-url` | `http://localhost:18790` | Remembrance API URL |
| `--run-dir` | `./runs` | Run artifact output directory |

### Environment

| Variable | Default | Description |
|----------|---------|-------------|
| `PRISM_BUS_URL` | `nats://127.0.0.1:4222` | NATS connection URL |
| (Ollama URL configured via `--ollama-url` flag) | | |
| (Remembrance URL configured via `--memory-url` flag) | | |

## Troubleshooting

### `package cmd/prism-cli is not in std`

Use `./` when running local Go packages:

```bash
# Wrong
go run cmd/prism-cli
# Correct
go run ./cmd/prism-cli
# Or build first
go build -o prism ./cmd/prism-cli/
```

### Ollama connection failed

```bash
# Make sure Ollama is running
ollama serve
ollama list

# If using a remote Ollama, specify the URL
./prism run --task "Test" --provider ollama --ollama-url http://192.168.1.100:11434
```

### Remembrance unavailable

With `--memory-enabled`, Prism continues with a `prism.context.failed` event and proceeds to the LLM call. With `--require-memory`, Prism fails with `prism.task.failed` before reaching the LLM.

### Model not found

```bash
# Pull the model first
ollama pull qwen2.5:7b
```

## Roadmap

### V1 — Complete ✅
- Embedded NATS JetStream event bus
- Canonical event schema with ULID IDs
- Correlation IDs and parent chains
- Task lifecycle: created → started → completed/failed
- Deterministic placeholder agent
- Optional Remembrance context hook with graceful fallback
- Event log persistence (`events.jsonl` + `summary.json`)
- Python SDK for event consumption

### V2 — Current
- Real single-agent LLM execution
- Provider interface (mock + Ollama)
- Prompt builder with deterministic `prompt.md` artifacts
- LLM lifecycle events (`llm.requested/completed/failed`)
- Context injection events (`context.requested/injected/failed`)
- Output artifact (`output.md`)
- Provider/model metadata in summaries
- `--dry-run-prompt` mode
- Strict `--require-memory` failure mode

### V3 — Planned
- Tool registry and execution
- Permission hooks
- Approval events
- Safer file/shell operations

### V4 — Planned
- Multi-agent orchestration
- Agent handoff events
- Planner/developer/reviewer workflows

### V5 — Planned
- Dashboard
- Channel workflows (Discord, Telegram)
- OpenClaw migration
- Full autonomous memory intelligence

## Design Philosophy

1. **The framework controls the lifecycle; the model only generates output.** Prism decides when tasks start, when context is injected, when the LLM is called, and when outputs are written. The model is a function call, not a driver.

2. **Every important action should become an observable event.** If something meaningful happened — a task started, an LLM was called, context was injected, an output was written — it should be in the event log with a parent chain back to its cause.

3. **Memory and context must be explicit, traceable, and auditable.** Remembrance context injection is optional and its success or failure is recorded as events. There are no hidden prompt injections.

## Project Structure

```
prism/
├── cmd/
│   ├── prism-cli/        # CLI entrypoint (build as 'prism')
│   ├── prism-bus/        # Embedded NATS JetStream server
│   └── prism-agent/      # Agent runtime (subscribes, processes, publishes)
├── internal/
│   ├── event/            # Canonical event schema (V1 + V2 types)
│   ├── run/              # Run orchestrator (V1 + V2 lifecycle)
│   ├── prompt/           # Prompt builder (prompt.md assembly)
│   ├── provider/         # Provider interface, MockProvider, OllamaProvider
│   ├── agent/            # Placeholder agent (V1 compat)
│   └── remembrance/      # HTTP client for memory context hook
├── sdk/
│   └── prism/            # Python SDK (PrismClient, Event, tools)
├── remembrance/           # Python memory system (LanceDB, embeddings)
├── runs/                  # Run outputs (gitignored, created at runtime)
├── docs/
│   └── DESIGN.md         # Architecture design document
└── go.mod                 # Go 1.26.2, nats.go, ulid/v2
```

## V2 Event Reference

### V1 Events (preserved)

| Event | Subject | Description |
|-------|---------|-------------|
| Task Created | `prism.task.created` | New task enters the system |
| Task Started | `prism.task.started` | Processing begins |
| Task Completed | `prism.task.completed` | Task finished successfully |
| Task Failed | `prism.task.failed` | Task failed |
| Memory Context Requested | `prism.memory.context_requested` | Asking Remembrance for context |
| Memory Context Built | `prism.memory.context_built` | Context retrieved successfully |
| Memory Context Failed | `prism.memory.context_failed` | Context retrieval failed |
| Agent Started | `prism.agent.started` | Agent begins processing |
| Agent Output | `prism.agent.output` | Agent produces result |
| Agent Completed | `prism.agent.completed` | Agent finished |
| Agent Failed | `prism.agent.failed` | Agent failed |
| Tool Called | `prism.tool.called` | Tool invocation *(not yet implemented)* |
| Tool Result | `prism.tool.result` | Tool returned result *(not yet implemented)* |
| Tool Failed | `prism.tool.failed` | Tool invocation failed *(not yet implemented)* |
| System Health | `prism.system.health` | Health check event |

### V2 Events (new)

| Event | Subject | Description |
|-------|---------|-------------|
| LLM Requested | `prism.llm.requested` | LLM call initiated |
| LLM Completed | `prism.llm.completed` | LLM call succeeded |
| LLM Failed | `prism.llm.failed` | LLM call failed |
| Context Requested | `prism.context.requested` | V2 context request |
| Context Injected | `prism.context.injected` | V2 context injected into prompt |
| Context Failed | `prism.context.failed` | V2 context injection failed |
| Output Written | `prism.output.written` | Output artifact written to disk |

## Event Schema

Every event follows this canonical structure:

```json
{
  "id": "evt_01KRC7AQXH2W66WNXM07",
  "type": "prism.task.created",
  "source": "prism-cli",
  "timestamp": "2026-05-11T15:13:25.123456789Z",
  "correlation_id": "corr_01KRC7AQXBM1Y5ZY0QEBG56JTC",
  "parent_id": "evt_01KRC7...",
  "payload": { "task": "...", "project": "prism" },
  "metadata": {
    "run_id": "run_01KRC7...",
    "session_id": "sess_01KRC7...",
    "project": "prism",
    "agent": "lumi"
  }
}
```

- **ID format:** `evt_<ulid>`, `corr_<ulid>`, `run_<ulid>`, `sess_<ulid>` — sortable, unique, time-encoded
- **Parent chain:** Every event links to its direct causal predecessor via `parent_id`
- **Correlation:** All events in a run share the same `correlation_id`
- **Timestamps:** RFC3339Nano (UTC)

## License

Private — Emmanuel Harmony