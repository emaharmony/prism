# Prism — Event-Native AI Agent Platform

Prism is an event-native AI agent platform where every action, decision, and state change flows as a canonical event through NATS JetStream. V1 proves the core theory: **a task enters, becomes an event, flows through the bus, triggers handlers, produces results, and leaves a durable audit trail.**

## V1 Status: Stabilized ✅

The V1 lifecycle is proven end-to-end with 25 passing integration tests:
- ✅ `prism` CLI — full task lifecycle with event persistence
- ✅ Canonical event schema with ULID IDs, correlation IDs, and parent chains
- ✅ NATS JetStream bus with durable storage
- ✅ Deterministic placeholder agent (real LLM calls are V2)
- ✅ Optional Remembrance context hook (graceful fallback)
- ✅ Event log persistence (`runs/<run_id>/events.jsonl` + `summary.json`)
- ✅ Parent chain integrity — events link back via `parent_id` in both in-memory and NATS-published events

## Quick Start (Fresh Clone)

### 1. Prerequisites

- **Go 1.26.2+** — [install](https://go.dev/dl/)
- **Git** — for cloning the repo

### 2. Clone & Build

```bash
git clone https://github.com/emaharmony/prism.git
cd prism

# Build all three binaries
# The CLI binary is named 'prism' for convenience
go build -o prism       ./cmd/prism-cli/
go build -o prism-bus   ./cmd/prism-bus/
go build -o prism-agent ./cmd/prism-agent/
```

### 3. Run Tests

```bash
go test ./internal/... -v
```

You should see 25 tests pass across three packages:
- `internal/agent` — 5 tests (PlaceholderAgent + delay)
- `internal/event` — 12 tests (IDs, event creation, JSON round-trip, security)
- `internal/run` — 8 tests (lifecycle, memory, correlation, NATS pub/sub, parent chains)

### 4. Start the Bus

```bash
./prism-bus
```

This starts an embedded NATS server with JetStream enabled on `localhost:4222`. Data is stored in `./prism-data/`.

You should see:
```
prism: starting event bus...
prism: NATS server running on nats://localhost:4222
prism: stream 'PRISM' created (subjects: prism.>)
prism: publishing V1 test events...
```

### 5. Run a Task

In another terminal:

```bash
./prism run --task "Test V1 event lifecycle" --project prism --agent lumi
```

You'll see events emitted in real time:
```
  💎 [prism.task.created]  id=evt_01KRC7AQXH...
  💎 [prism.task.started]  id=evt_01KRC7AQXH...
  💎 [prism.agent.started] id=evt_01KRC7AQXH...
  💎 [prism.agent.output]  id=evt_01KRC7AQXH...
  💎 [prism.agent.completed] id=evt_01KRC7AQXH...
  💎 [prism.task.completed] id=evt_01KRC7AQXH...
prism: run run_01KRC... completed (6 events, 8ms)
```

### 6. Check the Results

```bash
# Find the latest run (directory is created after your first run)
ls runs/

# Read the event log (one compact JSON object per line)
cat runs/<run_id>/events.jsonl

# Read the human-readable summary
cat runs/<run_id>/summary.json
```

### 7. (Optional) With Memory Context

If you have Remembrance running at `http://localhost:18790`:

```bash
# Enable memory context (graceful fallback if unavailable)
./prism run --task "Analyze the codebase" --project prism --agent lumi \
  --memory-enabled --memory-url http://localhost:18790

# Fail if memory is unavailable
./prism run --task "Critical analysis" --project prism --agent lumi \
  --memory-enabled --require-memory
```

### 8. Health Check

```bash
./prism health
```

Connects to NATS, verifies JetStream, checks the PRISM stream, and shows message count.

### 9. (Optional) Start an Agent

```bash
./prism-agent -name lumi -subs "prism.task.>,prism.agent.>"
```

This subscribes to task and agent events and dispatches them to registered handlers.

## CLI Reference

### `prism run`

```
./prism run --task <description> [options]

Required:
  --task string         Task description

Options:
  --project string     Project name (default: "prism")
  --agent string       Agent name (default: "lumi")
  --bus-url string     NATS URL (default: "nats://localhost:4222")
  --run-dir string     Output directory (default: "./runs")
  --memory-enabled     Enable Remembrance context hook
  --require-memory     Fail if Remembrance is unavailable
  --memory-url string  Remembrance URL (default: "http://localhost:18790")
```

### `prism health`

```
./prism health [--bus-url string]

Connects to NATS and reports bus/stream status.
```

### `prism version`

Prints the current version (`v0.1.0`).

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

**Event flow:** `prism run` → emits `task.created` → `task.started` → (optional) `memory.context_requested/built/failed` → `agent.started` → `agent.output` → `agent.completed` → `task.completed/failed`. Every event has a `correlation_id` linking it to the run, and a `parent_id` linking it to the causal predecessor.

## V1 Event Types

| Event | Subject | Description |
|-------|---------|-------------|
| Task Created | `prism.task.created` | New task enters the system |
| Task Started | `prism.task.started` | Processing begins (parent: task.created) |
| Task Completed | `prism.task.completed` | Task finished successfully |
| Task Failed | `prism.task.failed` | Task failed (parent: task.started) |
| Memory Context Requested | `prism.memory.context_requested` | Asking Remembrance for context |
| Memory Context Built | `prism.memory.context_built` | Context retrieved successfully |
| Memory Context Failed | `prism.memory.context_failed` | Context retrieval failed |
| Agent Started | `prism.agent.started` | Agent begins processing (parent: task.started) |
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

- **ID format:** `evt_<ulid>`, `corr_<ulid>`, `run_<ulid>`, `sess_<ulid>` — sortable, unique, time-encoded
- **Parent chain:** Every event links to its direct causal predecessor via `parent_id`
- **Correlation:** All events in a run share the same `correlation_id`
- **Timestamps:** RFC3339Nano (UTC)

## Project Structure

```
prism/
├── cmd/
│   ├── prism-cli/        # CLI entrypoint (build as 'prism')
│   ├── prism-bus/        # Embedded NATS JetStream server
│   └── prism-agent/      # Agent runtime (subscribes, processes, publishes)
├── internal/
│   ├── event/            # Canonical event schema (shared package)
│   ├── run/              # V1 lifecycle orchestrator (buildEvent/publishEvent)
│   ├── agent/            # Placeholder agent (deterministic, with delay option)
│   └── remembrance/      # HTTP client for memory context hook
├── sdk/
│   └── prism/            # Python SDK (PrismClient, Event, tools)
├── runs/                 # Run outputs (gitignored, created at runtime)
├── docs/
│   └── DESIGN.md         # Architecture design document
└── go.mod                # Go 1.26.2, nats.go, ulid/v2
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