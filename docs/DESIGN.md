# Prism — Event-Native AI Agent Platform

## Design Document v0.1
**Date:** 2026-05-07
**Authors:** Ema + Lumi
**Status:** Draft — Pre-Implementation

---

## Vision

Prism is the spiritual successor to OpenClaw, built event-native from the ground up. Where OpenClaw is a request-response platform with hooks bolted on, Prism treats **events as the core primitive** — every action, decision, state change, and communication flows through an event bus that anything can subscribe to.

**One event in → spectrum of reactions out.**

## Core Metaphor

A prism refracts white light into its component colors. Prism (the platform) refracts a single event into parallel, specialized agent reactions. Lumi (the light) flows through the prism (the event bus) and becomes a spectrum (fan-out to specialized agents).

```
Event → [Prism Bus] → Agent A (fire element)
                      → Agent B (water element)
                      → Agent C (audit logger)
                      → Memory Store
                      → Notification Channel
```

## Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Core runtime | Go | Goroutines = natural actor model, concurrency primitives, single binary, fast |
| Agent SDK | Python | LLM ecosystem lives in Python (LangGraph, AutoGen, CrewAI) |
| Event backbone | NATS (JetStream) | Built-in persistence, replay, consumer groups, wildcards, Go-native |
| Migration | OpenClaw config import | Read `openclaw.json` → emit Prism config |
| License | Source-available | All-rights-reserved, permission-required. Future licensing may be evaluated |
| State storage | SQLite (local) / PostgreSQL (server) | Event sourcing + projection tables |

## System Architecture

```
┌─────────────────────────────────────────────────┐
│                   Prism Runtime (Go)              │
│                                                   │
│  ┌─────────┐  ┌─────────┐  ┌─────────────────┐  │
│  │ Channel  │  │  Cron    │  │  ACP / A2A      │  │
│  │ Adapters │  │ Scheduler│  │  Gateway        │  │
│  └────┬─────┘  └────┬────┘  └───────┬─────────┘  │
│       │              │               │            │
│       ▼              ▼               ▼            │
│  ┌─────────────────────────────────────────────┐ │
│  │            Event Bus (NATS JetStream)         │ │
│  │                                              │ │
│  │  Subjects:                                   │ │
│  │    prism.agent.*      — agent lifecycle      │ │
│  │    prism.tool.*       — tool invocations     │ │
│  │    prism.memory.*     — memory operations    │ │
│  │    prism.channel.*    — channel messages      │ │
│  │    prism.cron.*       — scheduled triggers   │ │
│  │    prism.session.*    — session lifecycle     │ │
│  │    prism.decision.*   — agent decisions       │ │
│  └──────────┬──────────────┬─────────────────────┘ │
│             │              │                        │
│    ┌────────▼──────┐  ┌───▼──────────┐             │
│    │ Agent Runtime │  │ Memory Store  │             │
│    │ (Go + Python) │  │ (Event Sourced)│            │
│    └───────────────┘  └──────────────┘              │
│                                                     │
│    ┌───────────────┐  ┌──────────────┐              │
│    │ Tool Registry │  │ State Mgr    │              │
│    └───────────────┘  └──────────────┘              │
└─────────────────────────────────────────────────────┘
```

## Event Schema

Every event in Prism follows a canonical schema:

```json
{
  "id": "evt_01HXXXXXXXXXX",
  "type": "prism.agent.decision",
  "source": "lumi",
  "timestamp": "2026-05-07T22:52:00Z",
  "correlation_id": "corr_01HXXXXXXXXXX",
  "parent_id": "evt_01HYYYYYYYYYY",
  "payload": { },
  "metadata": {
    "model": "glm-5.1:cloud",
    "prompt_hash": "sha256:abc123",
    "token_cost": 4280,
    "session_id": "sess_01HZZZZZZZZZZ",
    "latency_ms": 3200
  }
}
```

Fields:
- `id` — ULID, globally unique, time-sortable
- `type` — namespaced event type (dot-separated)
- `source` — which agent/component produced this
- `timestamp` — ISO 8601 UTC
- `correlation_id` — links all events in a single workflow/request
- `parent_id` — direct causal parent (event sourcing chain)
- `payload` — event-specific data
- `metadata` — LLM provenance, cost tracking, session context

## Event Types (Subject Tree)

```
prism.agent.spawned        — agent instance created
prism.agent.decision       — agent made a reasoning decision
prism.agent.output         — agent produced output for user
prism.agent.error         — agent encountered error
prism.agent.completed      — agent finished its task

prism.tool.called          — tool invocation started
prism.tool.result          — tool returned result
prism.tool.error          — tool invocation failed

prism.memory.stored        — memory entry written
prism.memory.retrieved     — memory entry read
prism.memory.consolidated  — memory compaction ran

prism.channel.received     — message arrived from channel (Discord, Signal, etc.)
prism.channel.sent         — message sent to channel

prism.cron.triggered       — scheduled job fired
prism.cron.completed       — scheduled job finished

prism.session.created      — session started
prism.session.ended        — session closed

prism.state.changed        — any state mutation (CQRS write side)
```

## Core Components

### 1. Event Bus (NATS JetStream)
The backbone. Every component publishes to and subscribes from the bus.

Features used:
- **Consumer groups** — parallel fan-out (multiple agents subscribe to same event independently)
- **Durable consumers** — events persist until acknowledged (guaranteed delivery)
- **Replay** — new consumers can replay from beginning (event sourcing, bootstrapping)
- **Wildcards** — `prism.agent.*` subscribes to all agent events
- **Backpressure** — push or pull delivery, rate limits per consumer

### 2. Agent Runtime
Go-managed goroutines that host agent instances. Each agent:
- Subscribes to event subjects matching its capabilities
- Processes events through LLM reasoning
- Emits new events (decisions, outputs, tool calls)
- Reports health and latency via heartbeat events

Python agents communicate via gRPC to the Go runtime:
```
Python SDK → gRPC → Go Agent Runtime → Event Bus
```

### 3. Memory Store (Event-Sourced)
Not a flat file system. An append-only event log with projection tables.

- **Event log** — every `prism.memory.*` event, immutable, replayable
- **Projections** — materialized views for fast queries (current state, search index, vector index)
- **Consolidation** — background process that compacts old events into summaries
- **Vector index** — embedding-based semantic search over memory events (future: pluggable vector DB)

### 4. Channel Adapters
Each channel (Discord, Telegram, Signal, WhatsApp, webchat) is an adapter that:
- Subscribes to `prism.channel.received` for its channel type
- Publishes `prism.channel.sent` when the agent responds
- Translates between channel-specific formats and Prism's canonical event schema

### 5. Tool Registry
Tools register themselves by publishing a `prism.tool.registered` event with their schema. Agents discover tools by subscribing to tool registration events or querying the tool registry projection.

Tool execution flow:
1. Agent emits `prism.tool.called` with tool name + args
2. Tool handler subscribes to `prism.tool.called.{tool_name}`
3. Tool executes and emits `prism.tool.result` or `prism.tool.error`
4. Correlation ID links the call to its result

### 6. Cron Scheduler
Schedules are just time-based event producers:
1. Cron config stored as a persistent consumer with a time trigger
2. When time fires, emits `prism.cron.triggered` with job payload
3. Target agent subscribes to `prism.cron.triggered.{job_name}`
4. On completion, emits `prism.cron.completed`

### 7. State Manager (CQRS)
Commands (write) go through events. Queries (read) go through projections.

- **Write path:** Component emits `prism.state.changed` → event log → projection updated
- **Read path:** Component queries projection (SQLite/Postgres) → fast response
- Projections are rebuilt by replaying the event log if corrupted

## Python SDK

```python
import prism

# Subscribe to events
@prism.on("prism.channel.received.discord")
async def handle_discord_message(event):
    response = await agent.reason(event.payload["text"])
    prism.emit("prism.channel.sent", {
        "channel": "discord",
        "text": response,
        "correlation_id": event.correlation_id
    })

# Emit events
prism.emit("prism.agent.decision", {
    "reasoning": "User needs deployment help",
    "action": "spawn_coder",
    "confidence": 0.92
})

# Use tools (emits prism.tool.called, waits for prism.tool.result)
result = await prism.tool("github.create_issue", {
    "repo": "emaharmony/prism",
    "title": "Design event schema"
})
```

## OpenClaw Migration Tool

A CLI command that reads OpenClaw config and emits Prism config:

```bash
prism migrate --from-openclaw ~/.openclaw/openclaw.json
```

Mapping:
| OpenClaw | Prism |
|----------|-------|
| `channels.discord` | Channel adapter: `prism.channel.received.discord` |
| `session.dmScope` | Session config: `prism.session.*` |
| `hooks.internal` | Event subscriptions: `prism.agent.*` → handler |
| `plugins.entries` | Tool registrations: `prism.tool.registered.*` |
| `gateway.bind` | Runtime config: host/port |
| `models.*` | LLM provider config in agent definitions |
| Memory DB (`MEMORY.md`) | Import into event log as `prism.memory.stored` events |
| Cron jobs | `prism.cron.triggered.*` with schedule config |
| Sub-agent spawns | `prism.agent.spawned` with parent correlation |

## Monetization Model (Future Evaluation)

| Tier | What's Included | Price |
|------|----------------|-------|
| **Community** | Full Prism runtime, event bus, agent SDK, memory, basic tools | Source-available, permission-required |
| **Cloud** | Managed Prism hosting, auto-scaling, monitoring dashboard | Usage-based |
| **Enterprise** | SSO, audit logging, custom retention, SLA, priority support | Annual license |

The current license is all-rights-reserved and permission-required. Future licensing or commercial models may be evaluated later. The core framework is source-available for review and feedback; use requires written permission.

## What This Solves (vs OpenClaw)

| Problem | OpenClaw | Prism |
|---------|----------|-------|
| Can't hook into internal events | Hooks are limited, not extensible | Every action emits an event, subscribe to anything |
| Memory is flat files | MEMORY.md + SQLite with no history | Event-sourced memory with replay, projections, vector search |
| Polling for work | Cron wakes agent on timer | Events trigger agents immediately |
| Sub-agent results are fire-and-forget | Completion events are limited | Full event chain with correlation IDs |
| No audit trail | Logs to JSONL files | Every decision is an immutable event |
| Can't debug agent reasoning post-hoc | Read logs, hope context is there | Time-travel: replay events to reconstruct agent state |
| Plugins are opaque | Only whitelisted plugins, no custom hooks | Any component that subscribes to events IS a plugin |
| Rate limiting is manual | Sleep/retry in agent code | Backpressure via NATS consumer limits |
| Memory consolidation is fragile | Weekly cron, manual scripts | Event-sourced compaction with replay safety |

## Next Steps

1. ~~**Validate NATS JetStream**~~ ✅ Confirmed — NATS JetStream for event bus
2. ~~**Prototype the event bus**~~ ✅ Done — `cmd/prism-bus/main.go`, embedded NATS, test events passing
3. ~~**Build the Python SDK**~~ ✅ Done — `sdk/prism/` with client, events, decorators, tool calls
4. ~~**Channel adapters**~~ ✅ Done — Discord + Telegram adapters built
5. **Agent runtime** — Go goroutines hosting Python agents, LLM calls
6. **Memory store** — Event-sourced append-only log + projections + vector search
7. **Migration tool** — `prism migrate --from-openclaw`
8. **Web UI** — Real-time event stream viewer

---

*"One beam of light. One event. A spectrum of reactions.""