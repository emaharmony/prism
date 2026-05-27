# Prism Roadmap

**Last Updated:** 2026-05-26
**Status:** V26 complete. All four phases shipped.

---

## Progress

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
V20 ██ Live orchestrator + Discord + Sessions  ✅ Shipped
V21 ██ Full conversation pipeline              ✅ Shipped
V22 ██ Multi-agent orchestration               ✅ Shipped
V23 ██ Platform (API, Bridge, SDK)             ✅ Shipped
V24 ██ Visual representations                  ✅ Shipped
V25 ██ Visual workflow editor                  ✅ Shipped
V26 ██ Remembrance integration                 ✅ Shipped
```

---

## Phase Summary

### Phase 1: Live Orchestrator (V20) — ✅ Complete
**Exit criteria met:** Talk to Prism on Discord → get a response. Sessions persist. Events flow end-to-end.

| Milestone | Status |
|-----------|--------|
| M1.1 `prism serve` persistent daemon | ✅ |
| M1.2 Per-agent event namespaces | ✅ |
| M1.3 Session Manager | ✅ |
| M1.4 Agent Router | ✅ |
| M1.5 Discord Adapter (inbound) | ✅ |
| M1.6 Discord Adapter (outbound) | ✅ |
| M1.7 Registered Actions | ✅ |
| M1.8 End-to-end test | ✅ |

### Phase 2: Full Conversation (V21) — 🟡 Partially Complete
**Exit criteria:** Memory works. Streaming and cron are deferred.

| Milestone | Status |
|-----------|--------|
| M2.1 Remembrance as Prism service | ✅ V26 |
| M2.2 Context auto-save after every run | ✅ V26 |
| M2.3 Streaming responses | 🟡 Deferred |
| M2.4 Cron/Scheduling | 🟡 Deferred |
| M2.5 Migration tool (`prism migrate`) | 🟡 Deferred |
| M2.6 OpenClaw config transfer v2 | 🟡 Deferred |
| M2.7 E2E test: full conversation with memory | ✅ V26 |

### Phase 3: Multi-Agent Orchestration (V22) — ✅ Complete
**Exit criteria met:** Lumi delegates to Mango. Parallel agents work. Approval in Discord.

| Milestone | Status |
|-----------|--------|
| M3.1 Agent delegation | ✅ |
| M3.2 Parallel agents | ✅ |
| M3.3 Role assignment + capabilities | ✅ |
| M3.4 Task tracking | ✅ |
| M3.5 Approval gates in chat | ✅ |
| M3.6 E2E test | ✅ |

### Phase 4: Platform (V23) — ✅ Complete
**Exit criteria met:** API, dashboard, bridge, SDK all shipped.

| Milestone | Status |
|-----------|--------|
| M4.1 Multi-Prism Bridge | ✅ |
| M4.2 Dashboard v2 | ✅ |
| M4.3 HTTP API (REST + SSE) | ✅ |
| M4.4 Adapter SDK | ✅ |
| M4.5 IoT Adapter | ⬜ Future |
| M4.6 Business Template | ⬜ Future |
| M4.7 License evaluation | ⬜ Future |

### Phase 5: Visual (V24-V25) — ✅ Complete
| Milestone | Status |
|-----------|--------|
| M5.1-M5.7 SVG diagram generators | ✅ |
| M6.1-M6.6 Visual workflow editor | ✅ |

### Phase 6: Remembrance Integration (V26) — ✅ Complete
| Milestone | Status |
|-----------|--------|
| NATS subscriber for auto-capture | ✅ |
| RemembranceStage (capture + context) | ✅ |
| Serve pipeline context injection | ✅ |
| Context caching (60s TTL) | ✅ |
| Dream cycle (3AM nightly + event trigger) | ✅ |
| E2E integration tests | ✅ |

---

## What's Next

| Priority | Task | Version |
|----------|------|---------|
| 🔴 P0 | Streaming responses (token-by-token to Discord) | V27 |
| 🔴 P0 | Auth middleware for API endpoints | V27 |
| 🟡 P1 | Cron/Scheduling (`prism.cron.triggered`) | V28 |
| 🟡 P1 | `prism migrate --from-openclaw` | V28 |
| 🟢 P2 | IoT Adapter | Future |
| 🟢 P2 | Business Template | Future |

---

## Key Decisions (Locked)

1. **Events are per-agent and dynamic** — `<agent-id>.agent.output`, not hardcoded
2. **Registered actions, not model commands** — system handles persistence, model focuses on task
3. **Separate services through event bus** — Go orchestrator + Python Remembrance
4. **Go + Python** — Go for orchestrator/adapters, Python for Remembrance/LLM ecosystem
5. **Remembrance as separate service** — HTTP + NATS, not embedded
6. **Domain-agnostic core** — trading/Roblox/OpenClaw are adapters, not core pivots
7. **Restrictive license** — source-available, all rights reserved
8. **`prism.yaml` config** — separate from OpenClaw entirely