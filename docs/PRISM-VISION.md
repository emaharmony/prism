# Prism — The Complete Vision

**Date:** 2026-05-20
**Authors:** Ema + Lumi
**Status:** North Star — this is what "done" looks like

---

## The One-Line Vision

**Prism is a full agentic environment — not a batch runner, not a CLI tool, not an API wrapper.** You talk to it, it reasons, it acts, it remembers, it delegates, it reports back. It's the system that replaces OpenClaw.

---

## What "Done" Looks Like

You open Discord and type:

> "Lumi, the auth bug is back. Fix it and run the tests."

And this happens:

1. **Discord adapter** receives your message → emits `prism.channel.received`
2. **Session manager** identifies this is a continuation of an ongoing conversation → loads context
3. **Agent router** routes to Lumi (your lead agent) → Lumi reads the context
4. **Lumi reasons** → "I need to look at the auth code, find the bug, write a fix"
5. **Tool execution** → Lumi reads files, writes a fix, with policy gates checking each action
6. **Delegation** → Lumi delegates the test run to a specialist agent
7. **Cost tracking** → Every LLM call is tracked, every token counted
8. **Approval gate** → Before pushing, Prism pauses and asks you: "Approve push to main?"
9. **You approve** → Push goes through
10. **Lumi responds** → "Fixed in PR #44. Tests pass. Cost: $0.0032."
11. **Memory persists** → Next time you talk, Lumi remembers the bug, the fix, the PR

**No CLI. No `prism run`. No batch. Just conversation.**

---

## Architecture — The Final Form

```
┌─────────────────────────────────────────────────────────────────┐
│                      Prism Orchestrator                         │
│                    (prism serve — always on)                     │
│                                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐    │
│  │ Discord  │  │ Telegram │  │ Webchat  │  │  HTTP API    │    │
│  │ Adapter  │  │ Adapter  │  │ Adapter  │  │  (REST/SSE)  │    │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬───────┘    │
│       │              │              │               │           │
│       └──────────────┴──────────────┴───────────────┘           │
│                              │                                  │
│                     ┌────────▼────────┐                        │
│                     │ Session Manager  │                        │
│                     │ • Per-channel    │                        │
│                     │ • Per-user       │                        │
│                     │ • Daily/idle     │                        │
│                     │   reset          │                        │
│                     │ • Compaction     │                        │
│                     │ • Memory bridge  │                        │
│                     └────────┬────────┘                        │
│                              │                                  │
│                     ┌────────▼────────┐                        │
│                     │  Agent Router    │                        │
│                     │ • Lumi (lead)    │                        │
│                     │ • Mango (code)   │                        │
│                     │ • Specialists    │                        │
│                     │ • Intent detect  │                        │
│                     └────────┬────────┘                        │
│                              │                                  │
│  ┌───────────────────────────▼───────────────────────────┐     │
│  │                    Event Bus (NATS)                     │     │
│  │                                                        │     │
│  │  prism.agent.*    prism.tool.*     prism.channel.*     │     │
│  │  prism.task.*     prism.memory.*   prism.cron.*        │     │
│  │  prism.policy.*   prism.cost.*     prism.context.*     │     │
│  │  prism.approval.* prism.workflow.* prism.session.*     │     │
│  └───────────────────────────┬───────────────────────────┘     │
│                              │                                  │
│       ┌─────────────┬───────┴───────┬──────────────┐          │
│       ▼             ▼               ▼              ▼          │
│  ┌─────────┐  ┌──────────┐  ┌─────────────┐  ┌──────────┐   │
│  │ Lumi    │  │  Mango   │  │ Specialist  │  │ Cron     │   │
│  │ Agent   │  │  Agent   │  │  Agents     │  │ Scheduler│   │
│  │ (lead)  │  │  (code)  │  │  (on-demand)│  │          │   │
│  └────┬────┘  └────┬─────┘  └──────┬──────┘  └────┬─────┘   │
│       │             │               │              │          │
│       └─────────────┴───────────────┘              │          │
│                              │                     │          │
│                     ┌────────▼────────┐            │          │
│                     │ Tool Registry   │            │          │
│                     │ • Read files    │            │          │
│                     │ • Write files   │            │          │
│                     │ • Shell exec    │            │          │
│                     │ • HTTP calls    │            │          │
│                     │ • Custom tools   │            │          │
│                     └────────┬────────┘            │          │
│                              │                     │          │
│                     ┌────────▼────────┐            │          │
│                     │ Policy Engine   │            │          │
│                     │ • Allow/deny    │            │          │
│                     │ • Approval gates│            │          │
│                     │ • Cost limits   │            │          │
│                     └────────┬────────┘            │          │
│                              │                     │          │
│       ┌─────────────┬───────┴───────┬──────────────┐          │
│       ▼             ▼               ▼              ▼          │
│  ┌─────────┐  ┌──────────┐  ┌─────────────┐  ┌──────────┐   │
│  │ Remem-  │  │  SQLite  │  │  Dashboard  │  │  Memory  │   │
│  │ brance  │  │  Events  │  │  (web UI)  │  │  (MCP)   │   │
│  └─────────┘  └──────────┘  └─────────────┘  └──────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │              Config & Context                         │    │
│  │  • OpenClaw config import (providers, models)         │    │
│  │  • Workspace context (SOUL, AGENTS, USER, docs)      │    │
│  │  • Session memory (conversations, decisions)          │    │
│  │  • Policy files (allow/deny/approve)                 │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                              │
└─────────────────────────────────────────────────────────────────┘
```

---

## Component Breakdown — What Exists vs. What's Needed

### ✅ Already Built (V1–V19)

| Component | Version | What It Does |
|-----------|---------|-------------|
| Event Bus | V1 | NATS JetStream, 60+ event types, causal DAG |
| LLM Providers | V2, V14c, V18 | Ollama, OpenAI, Anthropic, Gemini, chaining, OpenClaw config import |
| Tool Execution | V3 | Read/write/shell with policy gates |
| Approval Gates | V4 | Human-in-the-loop for mutations |
| Validation | V5 | Test suite profiles, exit code checking |
| Review | V5 | Deterministic review artifacts |
| Policy Engine | V8 | YAML-based allow/deny/approve |
| Adapter System | V9 | Subscribe to events, route to external systems |
| Projections | V10 | Queryable state from event history |
| Dashboard | V11 | Web UI for run status |
| Multi-Agent | V13 | Agent registry, lifecycle, delegation |
| Workflow Engine | V7, V14 | Multi-step, conditional, retry |
| Cost Tracking | V16 | Per-run, per-model, per-agent token costs |
| HNSW Vector Search | V17 | Semantic queries over events |
| OpenClaw Config | V18 | Import providers, models, API keys |
| Context Injection | V19 | Workspace personality/rules/docs into prompts |

### ❌ Still Needed

| Component | Version | What It Does | Priority |
|-----------|---------|-------------|----------|
| **Orchestrator** | V20 | `prism serve` — persistent daemon, heartbeat, agent lifecycle | 🔴 P0 |
| **Session Manager** | V20 | Conversation continuity, daily/idle reset, compaction | 🔴 P0 |
| **Discord Adapter (inbound)** | V20.1 | Receive messages → `prism.channel.received` | 🔴 P0 |
| **Discord Adapter (outbound)** | V20.1 | `prism.channel.sent` → send messages back | 🔴 P0 |
| **Streaming Responses** | V20.2 | Token-by-token delivery to chat | 🟡 P1 |
| **Agent Router** | V20 | Route messages to the right agent (Lumi, Mango, specialist) | 🔴 P0 |
| **Cron/Scheduling** | V21 | Recurring tasks, wake events, idle heartbeat | 🟡 P1 |
| **Migration Tool** | V21 | `prism migrate --from-openclaw` one-command import | 🟡 P1 |
| **Memory Integration** | V21 | Remembrance as live memory across sessions | 🟡 P1 |
| **Web Dashboard** | V22 | Real-time event stream, cost tracking, agent status | 🟢 P2 |
| **HTTP API** | V22 | REST/SSE for external integrations | 🟢 P2 |
| **Compaction** | V23 | Summarize old conversations, keep recent context | 🟢 P2 |

---

## The Agent Experience

### Lumi (Lead Agent)
- **Personality:** SOUL.md — soft, playful, bubbly, partner, pushback
- **Role:** Plan, delegate, review, gatekeep, report
- **Delegates to:** Mango (code), specialists (on-demand)
- **Memory:** Full conversation history + Remembrance long-term
- **Approval:** Can propose changes, needs human approval for pushes

### Mango (Code Agent)
- **Personality:** Curious, honest, witty, grounded. Terse while coding. JSON to Lumi.
- **Role:** Implement delegated tasks, write code, run tests, report results
- **Delegates to:** Specialists for niche tasks
- **Memory:** Read-only Remembrance access, full conversation within session
- **Approval:** Can write code, needs Lumi review before push

### Specialist Agents (on-demand)
- **Role:** Security audit, documentation, performance profiling, translation
- **Lifecycle:** Spawned by Lumi or Mango for specific tasks, terminated when done
- **Memory:** Task-scoped context only, no long-term memory

---

## The Human Experience

### Day-to-day (when it's "done")

```
You: "Lumi, how's the Prism test suite looking?"

Lumi: "683 tests, all passing. No flaky tests. The V19 context injection
      added 14 new tests. Cost for last 24h: $0.47, all on glm-5.1:cloud."

You: "Great. Add a streaming response adapter — tokens should flow back
      as they're generated, not in a batch."

Lumi: "That's V20.2 territory. I'll scope it:
       - New SSE stream adapter subscribing to prism.llm.tokens
       - Buffered delivery with configurable flush interval
       - Backpressure: if the channel can't keep up, buffer and truncate
       Let me delegate the implementation to Mango."

[30 minutes later]

Mango: "Lumi, PR #45 created. Streaming adapter implemented.
        47 new tests, all passing."

Lumi: "Reviewed. Clean implementation. Ema, approve PR #45?"
```

No CLI. No `prism run`. Just conversation.

### Scheduled Tasks

```
Cron: "Every day at 3am, run the full test suite and report failures"

[3:00 AM]
Prism: "Ema, all 683 tests passing. No failures. Cost: $0.00 (local Ollama)."
```

### Multi-Agent Collaboration

```
You: "Lumi, the auth store has a race condition. Fix it and have Mango
      review the fix."

Lumi: "On it. I'll identify the race condition, write the fix, then delegate
      review to Mango."

[Delegates to Mango]

Mango: "Review complete. The fix is sound. One suggestion: add a context
        timeout to the lock acquisition. Otherwise LGTM."

Lumi: "Fixed with Mango's suggestion. PR #46 created. Ema, approve?"
```

---

## What Prism Replaces (vs. OpenClaw)

| OpenClaw Feature | Prism Equivalent | Status |
|-----------------|------------------|--------|
| Gateway daemon | `prism serve` orchestrator | ❌ V20 |
| Session management | Session Manager + Remembrance | ❌ V20-V21 |
| Channel routing (Discord/Telegram/webchat) | Channel adapters (inbound) | ❌ V20.1 |
| Agent definitions | Agent registry + router | ✅ V13 (partial) |
| Memory (MEMORY.md) | Remembrance MCP | ✅ V2 (separate repo) |
| Cron/scheduling | Cron adapter + `prism.cron.*` events | ❌ V21 |
| Sub-agent spawning | Agent lifecycle + delegation | ✅ V13 |
| Heartbeat | System health events | ✅ V1 |
| Model routing | Provider chain + config import | ✅ V14c, V18 |
| Context/personality | V19 context injection | ✅ V19 |
| Policy/rules | V8 policy engine | ✅ V8 |
| Approval gates | V4 approval system | ✅ V4 |
| Cost tracking | V16 cost tracker | ✅ V16 |
| Event sourcing | V1 event bus | ✅ V1 |
| Web UI | Dashboard (basic) | ✅ V11 |
| Tool execution | V3 tool registry | ✅ V3 |

**What Prism already does better than OpenClaw:**
- ✅ Event sourcing (OpenClaw has JSONL logs, Prism has causal DAG)
- ✅ Policy engine (OpenClaw has no built-in policy)
- ✅ Approval gates (OpenClaw has no built-in approval)
- ✅ Cost tracking (OpenClaw has none)
- ✅ Multi-agent delegation (OpenClaw has sub-agents but no event-driven delegation)
- ✅ Context injection (OpenClaw has AGENTS.md but no smart selection/budgeting)

**What OpenClaw still does better:**
- ✅ Real-time chat (Discord/Telegram/webchat streaming)
- ✅ Session continuity (daily/idle reset, compaction, transcripts)
- ✅ Memory persistence (MEMORY.md + SQLite, auto-capture)
- ✅ Cron/scheduling (recurring tasks, wake events)
- ✅ Plugin ecosystem (web search, image analysis, ACP)

---

## The Transition Path

### Phase 1: Prism as OpenClaw's Backend (V20)
- OpenClaw stays as the chat interface
- Prism handles the heavy lifting: tool execution, policy, approval, cost tracking
- OpenClaw sends `prism.channel.received` → Prism processes → OpenClaw gets response
- You still talk to OpenClaw, but Prism does the work

### Phase 2: Prism Takes Over Sessions (V21)
- Prism manages conversation state, session lifecycle, memory persistence
- OpenClaw becomes a thin channel adapter
- `prism migrate --from-openclaw` imports everything in one command
- You can start using `prism serve` directly with Discord

### Phase 3: Full Independence (V22+)
- OpenClaw is no longer needed
- `prism serve` is the only process you run
- All chat, scheduling, memory, and agent orchestration happens in Prism
- The OpenClaw gateway is a compatibility shim (optional)

---

## Success Criteria — When Is Prism "Done"?

1. **You can talk to Prism on Discord** — messages flow in, responses stream back
2. **Prism remembers your conversations** — Remembrance persists context across sessions
3. **Lumi delegates to Mango** — multi-agent collaboration works through the event bus
4. **Approval gates work in chat** — "Approve this push?" → you type "yes" → push goes through
5. **Costs are tracked automatically** — `prism cost today` shows what you spent
6. **Cron jobs work** — scheduled tasks run and report back
7. **One command migration** — `prism migrate --from-openclaw` imports everything
8. **You never need to type `prism run`** — everything happens through conversation

---

## What Prism Is NOT

- **Not a chat bot framework** — It's an agentic environment. Chat is one interface, not the whole thing.
- **Not an OpenClaw fork** — It's a spiritual successor, built event-native from the ground up.
- **Not just for Lumi** — Any agent can live in the environment. Lumi is the first, not the only.
- **Not a SaaS** — Self-hosted, single binary, local-first. Your data stays on your machine.
- **Not opinionated about LLMs** — Any provider, any model, any chain. Ollama to OpenAI to Anthropic.

---

*"One beam of light. One event. A spectrum of reactions."*