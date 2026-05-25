# Prism Roadmap

**Last Updated:** 2026-05-21
**Status:** Active — transitioning from batch CLI to live agentic environment

---

## Phases

### Phase 1: Live Orchestrator (V20) — 🔴 P0
**Goal:** Prism runs persistently, receives messages, routes to agents, talks back on Discord.

| Milestone | Description | Depends On |
|-----------|-------------|------------|
| M1.1 | `prism serve` — persistent daemon with heartbeat | Event bus (✅) |
| M1.2 | Per-agent event namespaces — `lumi.*`, `mango.*`, `remembrance.*` | Event bus (✅) |
| M1.3 | Session Manager — conversation continuity, daily/idle reset | M1.1 |
| M1.4 | Agent Router — route messages to the right agent based on context | M1.1, M1.3 |
| M1.5 | Discord Adapter (inbound) — receive messages → `*.channel.received` | M1.1, M1.2 |
| M1.6 | Discord Adapter (outbound) — `*.channel.sent` → send messages to Discord | M1.5 |
| M1.7 | Registered Actions — events trigger actions (webhook-style) | M1.1, M1.2 |
| M1.8 | End-to-end test: talk on Discord → agent responds → message back | M1.3–M1.6 |

**Exit Criteria:** You can talk to Prism on Discord and get a response. Session persists across messages. Events flow end-to-end.

### Phase 2: Full Conversation (V21) — 🟡 P1
**Goal:** Prism remembers conversations, streams responses, migrates from OpenClaw.

| Milestone | Description | Depends On |
|-----------|-------------|------------|
| M2.1 | Remembrance Embedding — memory as a Prism service on the event bus | Phase 1 |
| M2.2 | Context Auto-Save — Remembrance saves key context after every agent run | M2.1 |
| M2.3 | Streaming Responses — tokens flow back to Discord as they're generated | M1.6 |
| M2.4 | Cron/Scheduling — recurring tasks, wake events, idle heartbeat | M1.1 |
| M2.5 | Migration Tool — `prism migrate --from-openclaw` one-command import | Phase 1 |
| M2.6 | OpenClaw Config Transfer v2 — import channels, agents, cron jobs, sessions | M2.5 |
| M2.7 | End-to-end test: full conversation with memory across sessions | M2.1–M2.4 |

**Exit Criteria:** Prism remembers conversations across sessions. You can migrate from OpenClaw. Cron tasks run automatically.

### Phase 3: Multi-Agent Orchestration (V22) — 🟡 P1
**Goal:** Lumi delegates to Mango, results flow back, parallel agents work on different tasks.

| Milestone | Description | Depends On |
|-----------|-------------|------------|
| M3.1 | Agent Delegation — Lumi delegates tasks to Mango, results flow back | Phase 1 |
| M3.2 | Parallel Agents — multiple agents work on different tasks simultaneously | M3.1 |
| M3.3 | Role Assignment — Orchestrator, Developer, Researcher, etc. | M3.1 |
| M3.4 | Task Tracking — every task tracked end-to-end, no dropped tasks | M3.1, M1.7 |
| M3.5 | Approval Gates in Chat — "Approve this push?" → you type "yes" → push | Phase 2 |
| M3.6 | End-to-end test: Lumi delegates to Mango, Mango codes, Lumi reviews, PR created | M3.1–M3.4 |

**Exit Criteria:** Lumi can delegate to Mango through the event bus. Parallel agents work on separate tasks. Approval works in Discord.

### Phase 4: Platform (V23+) — 🟢 P2
**Goal:** Prism is a platform others can extend and deploy.

| Milestone | Description | Depends On |
|-----------|-------------|------------|
| M4.1 | Multi-Prism Communication — two Prism environments send tasks to each other | Phase 3 |
| M4.2 | Dashboard v2 — real-time event stream, cost tracking, agent status, task tracking | Phase 2 |
| M4.3 | HTTP API — REST/SSE for external integrations | Phase 2 |
| M4.4 | Adapter SDK — third-party adapter development kit | M4.3 |
| M4.5 | IoT Adapter — control smart home devices through events | M4.4 |
| M4.6 | Business Template — pre-configured Prism for team environments | M4.4 |
| M4.7 | License update — evaluate opening restrictive license | Phase 4 completion |

**Exit Criteria:** Prism is a platform. Third parties can write adapters. Two Prisms can communicate. Dashboard shows real-time status.

---

## Current Position

```
V1  ██ Event bus                              ✅ Shipped
V2  ██ LLM providers + context                ✅ Shipped
V3  ██ Tool execution                         ✅ Shipped
V4  ██ Approval gates                          ✅ Shipped
V5  ██ Validation + review                    ✅ Shipped
V6  ██ Memory (basic)                         ✅ Shipped
V7  ██ Workflows                               ✅ Shipped
V8  ██ Policy engine                          ✅ Shipped
V9  ██ Adapter system                          ✅ Shipped
V10 ██ Projections                             ✅ Shipped
V11 ██ Dashboard                               ✅ Shipped
V12 ██ Architectural refactor                  ✅ Shipped
V13 ██ Multi-agent                             ✅ Shipped
V14 ██ Crash recovery + providers              ✅ Shipped
V15 ██ Vector search                           ✅ Shipped
V16 ██ Intelligence arc (cost, enrichment)      ✅ Shipped
V17 ██ Performance (HNSW, pooling, indexes)   ✅ Shipped
V18 ██ OpenClaw config transfer                ✅ Shipped
V19 ██ Smart context injection                 ✅ Shipped
     ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓  Phase 1 starts here
V20 ██ Orchestrator + Discord + Sessions       ✅ Shipped
V21 ██ Memory + Streaming + Migration          ✅ Shipped
V22 ██ Multi-agent orchestration               ✅ Shipped
V23 ██ Platform (API, Bridge, SDK)             ✅ Shipped
V24 ██ Visual Workflow Representations          ✅ Shipped
V25 ░░ Visual Workflow Editor                   🟢 P2
```

---

## Key Decisions (Locked)

1. **Events are per-agent** — `lumi.llm.called`, not `prism.llm.called`
2. **Registered actions, not model commands** — system handles persistence, model focuses on task
3. **Separate services through event bus** — not a monolith
4. **Go + Python** — Go for orchestrator/adapters, Python for Remembrance/LLM ecosystem
5. **Remembrance embedded into Prism** — hooks into event bus, not external
6. **Designed for everyone** — not just Ema
7. **Restrictive license for now** — open later
8. **ASAP timeline** — pain is real, urgency is high