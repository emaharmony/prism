# Prism — Developer Onboarding

Welcome to Prism. This guide gets you from clone to contributing in ~15 minutes.

---

## What Prism Is

Prism is a Go event-native AI agent platform that runs as a persistent service. Agents communicate through a NATS event bus, maintain conversation sessions, remember context across conversations (via Remembrance), and can be monitored through a web dashboard.

**Three ways to use it:**
1. **Persistent daemon** (`prism serve`) — connects to Discord, maintains sessions, responds in real time
2. **One-shot CLI** (`prism run`) — single LLM calls with tools, workflows, and approval gates
3. **Platform** (HTTP API + SSE) — integrate Prism into your own tools

---

## Quick Start

```bash
# 1. Clone and build
git clone https://github.com/emaharmony/prism.git
cd prism
go build -o prism ./cmd/prism-cli/

# 2. Run the test suite (988 tests, 54 packages)
go test ./...

# 3. Create a config
cp prism.yaml.example prism.yaml

# 4. Start the daemon
./prism serve

# 5. Open the dashboard
open http://localhost:8322
```

### With Memory (Remembrance)

```bash
# Terminal 1: Start Remembrance (Python service)
pip install -e ".[nats]"
python -m rememberance_mcp.serve --port 8788

# Terminal 2: Start Prism
./prism serve
```

Enable in `prism.yaml`:
```yaml
remembrance:
  enabled: true
  url: "http://localhost:8788"
```

### One-Shot CLI

```bash
# Single LLM call
./prism run --prompt "Explain event-driven architecture" --provider ollama

# With tools
./prism run --prompt "Read main.go and summarize it" --tools read_file --allow-tools

# With approval gate (requires human approval before write)
./prism run --prompt "Fix the bug" --tools write_file --require-approval
```

---

## Key Concepts

### Events
Everything in Prism is an event. Agent outputs, tool calls, approval requests, session changes — all become canonical events on the NATS bus. Events are the source of truth.

- **Per-agent namespaces**: `lumi.agent.output`, `mango.task.created`, `remembrance.dream.triggered`
- **WAL persistence**: Every stage checkpoint is written to a write-ahead log before execution
- **Idempotent replay**: If Prism crashes, it replays from the last checkpoint

### Sessions
Conversations are stored in SQLite sessions. Each session has:
- A message history (user + assistant messages)
- An idle timeout (default: 30 minutes)
- A daily reset (default: 4 AM)
- Compaction: truncation when context exceeds budget

### Agents
Agents are defined in `prism.yaml`. Each has:
- An `id` (used for event namespaces: `<id>.agent.output`)
- A `role` (lead, developer, researcher, orchestrator)
- A `provider` + `model` (ollama, openai, anthropic, gemini)
- `capabilities` (what the agent is allowed to do)
- `context` (which workspace files to inject into the prompt)
- `primary: true` marks the default agent

### Remembrance (Memory)
Remembrance is a separate Python service that:
- **Captures** agent output through a gate (DilBERT classifier: SKIP/COLD/ACTIVE/PERSIST)
- **Extracts** entities, relationships, and facts
- **Builds context** for future conversations (hybrid search: FTS5 + vector + graph + RRF)
- **Maintains itself** through a dream cycle (nightly 3AM + event trigger after 10 PERSIST captures)

Two integration paths:
1. **Async capture**: Agent output → NATS event → Remembrance auto-captures
2. **Sync context**: Before LLM call, `BuildContext()` injects relevant memories

### Delegation
Agents can delegate tasks to other agents:
- `[DELEGATE: mango | code]` in LLM output triggers task creation
- Tasks have a lifecycle: created → assigned → in_progress → completed/failed
- Approval gates require human sign-off before execution
- Capabilities enforce what each agent can do

### Approvals
Mutations (file writes, command execution) can require human approval:
- Approval types: file_write, command_exec, config_change, resource_access, agent_delegation
- Approve/deny via Discord reactions or the dashboard
- All approvals are tracked in SQLite

### Dashboard
6-tab web UI at `http://localhost:8322`:
1. **Overview** — system status, agent states, recent events
2. **Runs** — run history with event timelines
3. **Events** — live event stream with filtering
4. **Approvals** — pending/approved/denied mutations
5. **Diagrams** — 5 SVG visualization types
6. **Editor** — drag-and-drop visual workflow editor

---

## Architecture Tour

```
prism serve
    │
    ├── NATS JetStream (embedded or external)
    │   ├── lumi.agent.output
    │   ├── mango.agent.output
    │   ├── prism.tool.completed
    │   └── remembrance.dream.triggered
    │
    ├── Discord Bot
    │   ├── Message received → debounce → session → agent router → LLM → respond
    │   └── Streaming via placeholder message edits (900ms batching)
    │
    ├── Remembrance (Python, separate process)
    │   ├── NATS subscriber: *.agent.output → capture
    │   ├── REST API: /capture, /context/build, /search, /dream
    │   └── Dream cycle: 3AM nightly + after 10 PERSIST
    │
    ├── HTTP API (port 8322)
    │   ├── /api/v1/runs, /agents, /approvals, /tasks, /events
    │   └── /api/v1/events/stream (SSE with 30s heartbeat)
    │
    └── Dashboard (static HTML)
        ├── Tabs: overview, runs, events, approvals, diagrams, editor
        └── Editor: SVG drag-and-drop → write to prism.yaml
```

### Pipeline Stages

When `prism run` executes a workflow:
1. **LLMStage** — call the language model
2. **DelegationStage** — detect `[DELEGATE: agent | type]` markers, create tasks
3. **PersistenceStage** — save artifacts to run directory
4. **EventPublishStage** — publish canonical events to NATS
5. **RemembranceStage** — capture output + build context (when enabled)

When `prism serve` handles a Discord message:
1. **Debounce** → 3s per user
2. **Session** → lookup/create conversation
3. **Router** → find the right agent
4. **Context** → inject workspace files + Remembrance memories
5. **LLM** → call with streaming callback
6. **Respond** → edit placeholder message with tokens
7. **Save** → persist to session
8. **Capture** → async fire-and-forget to Remembrance

---

## Go Development Cheat Sheet

```go
// All packages are under github.com/emaharmony/prism/internal/<name>

// Create an event
evt := event.New("lumi.agent.output", map[string]any{
    "content":    "Task complete",
    "session_id": "abc-123",
})

// Publish to the bus
bus.Publish(context.Background(), evt)

// Subscribe to events
bus.Subscribe("mango.*.completed", func(e event.Event) { ... })

// Use the session manager
mgr := session.NewManager(".prism/data/sessions.db")
sess, _ := mgr.GetOrCreate("user_123", "lumi")

// Use the Remembrance client
client := remembrance.NewClient("http://localhost:8788")
available := client.IsAvailable()
ctx, _ := client.BuildContext("what is prism", "prism", "lumi", 10)
result, _ := client.Capture("Important info", "prism:lumi", "project", "")
```

### Common Patterns

- **Error handling**: All errors are returned, never panicked (except in tests)
- **Context propagation**: `context.Context` is passed to all I/O operations
- **Graceful degradation**: Remembrance, NATS, and adapters all fail gracefully
- **Idempotency**: WAL replay, capture dedup, and task operations are all idempotent
- **Synchronization**: `sync.RWMutex` for read-heavy caches, `sync.Once` for one-time init
- **Goroutine safety**: All shared state is protected; use the race detector: `go test -race ./...`

---

## Contributing

1. **Pick a task** from [TASKS.md](./TASKS.md) — look for items marked 🟡 (in progress) or ⬜ (not started)
2. **Create a branch**: `vXX-feature-name` (e.g., `v27-streaming-responses`)
3. **Implement + test**: Every feature must have tests before moving on
4. **Code review**: Lumi reviews all diffs before push; Mango runs Loop 3 security review
5. **Create a PR**: All changes go through pull requests (no direct pushes to main)
6. **Ema merges**: Only Ema merges to main

### PR Naming

```
feat(vXX): Short description of what changed
fix(vXX): Bug fix description
docs: What documentation was updated
```

### Branch Naming

```
vXX-feature-name    # e.g., v27-streaming-responses
```

---

## Architecture Evolution

For detailed design history, see the [docs/](./docs/) directory. Key milestones:

| Version | What | Design Doc |
|---------|------|-----------|
| V1-V5 | Foundation: CLI, events, tools, approvals, validation | See `docs/` |
| V8-V9 | Policy engine, adapter contract | V8, V9 design docs |
| V10-V11 | State projections, dashboard | V10, V11 design docs |
| V12-V13 | Architectural refactor, multi-agent events | V12, V13 design docs |
| V14a-e | Pipeline stages, crash recovery, providers, Discord | V14a-e design docs |
| V15 | Vector search | V15 design doc |
| V16-V17 | Intelligence arc, performance | V16, V17 design docs |
| V18-V19 | OpenClaw config, smart context | V18, V19 design docs |
| V20 | Live orchestrator, `prism serve` | [V20 design doc](./V20-LIVE-ORCHESTRATOR-DESIGN.md) |
| V21 | Full conversation pipeline | [V21 design doc](./V21-FULL-CONVERSATION-DESIGN.md) |
| V22 | Multi-agent delegation | [V22 design doc](./V22-MULTI-AGENT-ORCHESTRATION-DESIGN.md) |
| V23 | Platform: API, dashboard, bridge, SDK | [V23 design doc](./V23-PLATFORM-DESIGN.md) |
| V24-V25 | Visual representations + workflow editor | V24, V25 design docs |
| V26 | Remembrance integration | [V26 design doc](./V26-REMEMBRANCE-INTEGRATION-DESIGN.md) |