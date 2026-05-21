# Prism — Task Tracker

**Last Updated:** 2026-05-21
**Status:** Phase 1 (V20) starts next

---

## Legend

- ✅ Done
- 🔴 P0 — Critical, blocks everything
- 🟡 P1 — Important, blocks user-facing features
- 🟢 P2 — Nice to have, can defer
- ⬜ Not started

---

## Phase 1: Live Orchestrator (V20) — 🔴 P0

### M1.1: Persistent Daemon (`prism serve`)

| Task | Status | Notes |
|------|--------|-------|
| Create `cmd/prism-serve/main.go` | ⬜ | Persistent process with heartbeat |
| Implement graceful shutdown (SIGINT, SIGTERM) | ⬜ | Drain events, close connections |
| Add health check endpoint (`/health`) | ⬜ | Returns orchestrator status |
| Add `prism status` CLI command | ⬜ | Shows running agents, sessions, uptime |
| Integration test: start, run, stop | ⬜ | Verify daemon lifecycle |

### M1.2: Per-Agent Event Namespaces (Dynamic)

| Task | Status | Notes |
|------|--------|-------|
| Design namespace translation layer | ⬜ | `<agent-id>.*` ↔ `prism.*` bridge, agent ID from config |
| Add `<agent-id>.*` event types | ⬜ | Dynamic based on agent config, not hardcoded |
| Add `remembrance.*` event types | ⬜ | Memory events |
| Update event schema validation | ⬜ | V19 schema.go needs dynamic agent namespaces |
| Backward compatibility test | ⬜ | Existing `prism.*` events still work |
| Update cost tracking for agent namespaces | ⬜ | `<agent-id>.llm.completed` tracks per-agent costs |

### M1.3: Session Manager

| Task | Status | Notes |
|------|--------|-------|
| Create `internal/session/manager.go` | ⬜ | Session lifecycle (create, active, compact, idle, daily, end) |
| Session persistence in SQLite | ⬜ | Sessions table with channel, user, agent, timestamps |
| Session compaction | ⬜ | Summarize old context when it exceeds budget |
| Daily reset at 4am local | ⬜ | New session, fresh context |
| Idle reset after N minutes | ⬜ | Configurable idle timeout |
| Session restore on restart | ⬜ | Reconnect to existing sessions |
| CLI: `prism session list/show` | ⬜ | Inspect sessions |

### M1.4: Agent Router

| Task | Status | Notes |
|------|--------|-------|
| Create `internal/router/router.go` | ⬜ | Route messages to agents |
| Direct address detection | ⬜ | "Lumi, fix this" → Lumi |
| @mention routing | ⬜ | "@Mango write tests" → Mango |
| Default agent fallback | ⬜ | No address → primary agent (Lumi) |
| Intent detection (stretch) | ⬜ | "Write code for..." → Mango |
| Delegation flow | ⬜ | Lumi → Mango → results flow back |

### M1.5: Discord Adapter (Inbound)

| Task | Status | Notes |
|------|--------|-------|
| Create `internal/adapter/builtin/discord/discord.go` (inbound) | ⬜ | Subscribe to Discord messages |
| Convert Discord message → `*.channel.received` event | ⬜ | Map user, channel, content |
| Handle Discord connection lifecycle | ⬜ | Connect, reconnect, disconnect |
| Handle rate limiting | ⬜ | Respect Discord rate limits |
| Handle message types | ⬜ | Text, embeds, threads |
| Bot token from OpenClaw config | ⬜ | V18 config transfer |

### M1.6: Discord Adapter (Outbound)

| Task | Status | Notes |
|------|--------|-------|
| Subscribe to `*.channel.sent` events | ⬜ | Agent output → Discord message |
| Convert event → Discord message | ⬜ | Format agent output for Discord |
| Handle message length limits | ⬜ | Split long messages (2000 char limit) |
| Handle threading/replies | ⬜ | Reply in thread if original was in thread |
| Handle rate limiting | ⬜ | Respect Discord rate limits |
| Streaming support (stretch) | ⬜ | Token-by-token delivery (M2.3) |

### M1.7: Registered Actions

| Task | Status | Notes |
|------|--------|-------|
| Create `internal/action/registry.go` | ⬜ | Register actions triggered by events |
| Action configuration in YAML | ⬜ | Define triggers and actions |
| Implement trigger matching | ⬜ | Wildcards, agent namespaces, event types |
| Built-in actions | ⬜ | remembrance.gate.extract, prism.cost.track |
| CLI: `prism action list/register` | ⬜ | Manage registered actions |
| Integration test: event → action fires | ⬜ | End-to-end action triggering |

### M1.8: End-to-End Test

| Task | Status | Notes |
|------|--------|-------|
| Test: talk on Discord → agent responds | ⬜ | Full loop with real Discord bot |
| Test: session persists across messages | ⬜ | Context carries over |
| Test: events flow end-to-end | ⬜ | All events on bus |
| Test: registered actions fire | ⬜ | Memory saved after agent output |
| Test: cost tracking works | ⬜ | Token costs recorded |
| Test: graceful shutdown | ⬜ | No dropped events |

---

## Phase 2: Full Conversation (V21) — 🟡 P1

| Task | Status | Notes |
|------|--------|-------|
| Remembrance as Prism service | ⬜ | Subscribe to `*.agent.output`, gate/extract/persist |
| Context auto-save after every run | ⬜ | Registered action: agent output → Remembrance |
| Streaming responses | ⬜ | Token-by-token delivery to Discord |
| Cron scheduler | ⬜ | `prism.cron.triggered` events |
| `prism migrate --from-openclaw` | ⬜ | Import channels, agents, cron jobs, sessions |
| OpenClaw config transfer v2 | ⬜ | Import Discord/Telegram tokens, agent definitions |
| E2E test: full conversation with memory | ⬜ | Session + Remembrance + Discord |

---

## Phase 3: Multi-Agent Orchestration (V22) — 🟡 P1

| Task | Status | Notes |
|------|--------|-------|
| Agent delegation through events | ⬜ | Lumi → `mango.task.created` → Mango → `mango.task.completed` |
| Parallel agent execution | ⬜ | Multiple agents on different tasks |
| Role assignment | ⬜ | Orchestrator, Developer, Researcher definitions |
| Task tracking end-to-end | ⬜ | No task dropped |
| Approval gates in Discord | ⬜ | "Approve?" → "yes" → approved |
| E2E test: Lumi delegates to Mango | ⬜ | Full delegation flow |

---

## Phase 4: Platform (V23+) — 🟢 P2

| Task | Status | Notes |
|------|--------|-------|
| Multi-Prism communication | ⬜ | Two Prism environments send tasks |
| Dashboard v2 | ⬜ | Real-time event stream, costs, agents |
| HTTP API (REST/SSE) | ⬜ | External integrations |
| Adapter SDK | ⬜ | Third-party adapter development kit |
| IoT adapter | ⬜ | Smart home control through events |
| License evaluation | ⬜ | Open restrictive license when ready |

---

## Completed (V1–V19)

| Version | Feature | Date |
|---------|---------|------|
| V1 | Event bus, NATS, canonical schema | 2026-05-11 |
| V2 | LLM providers, context injection | 2026-05-12 |
| V3 | Tool execution, policy gates | 2026-05-13 |
| V4 | Approval gates, mutations | 2026-05-14 |
| V5 | Validation, review | 2026-05-15 |
| V6 | Memory (basic) | 2026-05-15 |
| V7 | Workflow engine | 2026-05-16 |
| V8 | Policy engine | 2026-05-17 |
| V9 | Adapter system | 2026-05-18 |
| V10 | State projections | 2026-05-18 |
| V11 | Dashboard | 2026-05-18 |
| V12 | Architectural refactor | 2026-05-18 |
| V13 | Multi-agent | 2026-05-18 |
| V14 | Crash recovery, providers, concurrent SQLite, Discord | 2026-05-18 |
| V15 | Vector search | 2026-05-18 |
| V16 | Intelligence arc (cost tracking, enrichment) | 2026-05-19 |
| V17 | Performance (HNSW, pooling, indexes) | 2026-05-19 |
| V18 | OpenClaw config transfer | 2026-05-19 |
| V19 | Smart context injection | 2026-05-20 |

---

## Blocked / Decisions Needed

| Item | Status | Notes |
|------|--------|-------|
| Per-agent namespace (dynamic) | ✅ Decided | `<agent-id>.*` from config, not hardcoded. Need to design bridge layer. |
| Remembrance integration depth | 🟡 Decide | Embedded service vs. HTTP calls? Ema said embedded. |
| Discord library choice | ⬜ Decide | discordgo vs. disgo vs. custom? |
| Session storage format | ⬜ Decide | SQLite schema for sessions |
| Streaming protocol | ⬜ Decide | SSE vs. WebSocket vs. Discord native streaming |
| Agent configuration format | ⬜ Decide | YAML config with id/role/model/context fields |

---

## Notes

- All tasks require PR review before merge (PRs Only rule)
- Ema reviews all PRs before merge
- Lumi is lead developer — plans, delegates, codes major features
- Mango codes delegated tasks, reports JSON
- 683 tests currently passing, 0 failures