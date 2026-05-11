# Prism — Event-Native AI Agent Platform

Prism is an event-native AI agent platform where every action, decision, and state change flows as a canonical event through NATS JetStream. V1 proves the core theory: **a task enters, becomes an event, flows through the bus, triggers handlers, produces results, and leaves a durable audit trail.**

## V1 Status: Stabilized ✅

The V1 lifecycle is proven end-to-end:
- ✅ `prism run` CLI — full task lifecycle with event persistence
- ✅ Canonical event schema with ULID IDs, correlation IDs, and parent chains
- ✅ NATS JetStream bus with durable storage
- ✅ Deterministic placeholder agent (real LLM calls are V2)
- ✅ Optional Remembrance context hook (graceful fallback)
- ✅ Event log persistence (`runs/<run_id>/events.jsonl` + `summary.json`)
- ✅ 22 passing integration tests

## Quick Start

### Prerequisites
- Go 1.22+
- NATS Server (embedded mode included in `prism-bus`)

### Build
```bash
go build -o prism-cli ./cmd/prism-cli/
go build -o prism-bus ./cmd/prism-bus/
go build -o prism-agent ./cmd/prism-agent/
```

### Run V1 Lifecycle

1. Start the bus:
```bash
./prism-bus
```

2. In another terminal, run a task:
```bash
./prism-cli run --task "Test V1 event lifecycle" --project prism --agent lumi
```

3. Check the results:
```bash
ls runs/
cat runs/<run_id>/events.jsonl
cat runs/<run_id>/summary.json
```

### With Memory Context (Optional)
```bash
# Requires Remembrance running at http://localhost:18790
./prism-cli run --task "Analyze the codebase" --memory-enabled --memory-url http://localhost:18790

# Fail if memory is unavailable
./prism-cli run --task "Critical analysis" --memory-enabled --require-memory
```

### Health Check
```bash
./prism-cli health
```

### Start an Agent
```bash
./prism-agent -name lumi -subs "prism.task.>,prism.agent.>"
```

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│  prism-cli   │────▶│  NATS        │────▶│ prism-agent │
│  (run cmd)   │     │  JetStream   │     │ (handlers)  │
└─────────────┘     │  (bus)       │     └─────────────┘
                    │              │     ┌─────────────┐
                    │              │────▶│Remembrance  │
                    │              │     │(memory hook) │
                    └──────────────┘     └─────────────┘
                           │
                    ┌──────┴──────┐
                    │ events.jsonl│
                    │ summary.json│
                    │ (per run)   │
                    └─────────────┘
```

## V1 Event Types

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
| Tool Called | `prism.tool.called` | Tool invocation |
| Tool Result | `prism.tool.result` | Tool returned result |
| Tool Failed | `prism.tool.failed` | Tool invocation failed |
| System Health | `prism.system.health` | Health check event |

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

## Project Structure

```
prism/
├── cmd/
│   ├── prism-cli/       # CLI entrypoint (prism run, prism health)
│   ├── prism-bus/        # Embedded NATS JetStream server
│   └── prism-agent/      # Agent runtime (subscribes, processes, publishes)
├── internal/
│   ├── event/            # Canonical event schema (shared package)
│   ├── run/              # V1 lifecycle orchestrator
│   ├── agent/            # Placeholder agent (deterministic)
│   └── remembrance/      # HTTP client for memory context hook
├── sdk/
│   └── prism/            # Python SDK (PrismClient, Event, tools)
├── remembrance/          # FastAPI memory layer (LanceDB + SQLite + Ollama)
├── runs/                 # Run outputs (events.jsonl, summary.json)
├── docs/
│   └── DESIGN.md         # Architecture design document
└── go.mod
```

## Running Tests

```bash
go test ./internal/... -v
```

## V2 Roadmap

- Real LLM agent (replace placeholder with Ollama API calls)
- Multi-agent orchestration (spawn agents based on task type)
- Web dashboard for event visualization
- Discord/Telegram channel adapters via Python SDK
- ACP/A2A protocol support
- Memory consolidation pipeline
- OpenClaw integration

## License

Private — Emmanuel Harmony