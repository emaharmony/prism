# Prism — Task Tracker

**Last Updated:** 2026-05-26
**Status:** V26 (Remembrance Integration) in progress. M6.6 complete, V25 merged.

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
| Create `cmd/prism-cli/cmd_serve.go` | ✅ | CLI subcommand, not separate binary |
| Implement graceful shutdown (SIGINT, SIGTERM) | ✅ | Drain events, close connections |
| Add health check endpoint (`/health`) | ✅ | Returns JSON status with agent count, discord ready |
| Create `internal/orchestrator/orchestrator.go` | ✅ | Orchestrator lifecycle management |
| Create `internal/orchestrator/config.go` | ✅ | prism.yaml config loading, validation, agent ID resolution |
| Config: `prism.yaml` example | ✅ | Separate from OpenClaw, Prism owns its own config |

### M1.2: Per-Agent Event Namespaces (Dynamic)

| Task | Status | Notes |
|------|--------|-------|
| Design namespace translation layer | ✅ | `internal/agentns/namespace.go` — dynamic `<agent-id>.*` generation |
| Auto-generate agent IDs when not provided | ✅ | `prism1`, `prism2`, `prism3`... |
| Add `<agent-id>.*` event types | ✅ | Dynamic based on agent config, not hardcoded |
| Add `remembrance.*` event types | ⬜ | Memory events (Phase 2) |
| Update event schema validation | ⬜ | V19 schema.go needs dynamic agent namespaces |
| Backward compatibility test | ✅ | Existing `prism.*` events still work — agent namespace is additive |
| Update cost tracking for agent namespaces | ⬜ | `<agent-id>.llm.completed` tracks per-agent costs (Phase 2) |

### M1.3: Session Manager

| Task | Status | Notes |
|------|--------|-------|
| Create `internal/session/manager.go` | ✅ | SQLite-backed, WAL mode |
| Session persistence in SQLite | ✅ | Sessions + messages tables |
| Session compaction (truncate) | ✅ | Removes oldest messages when over budget (V20) |
| Daily reset at 4am local | ⬜ | Timer-based, not yet implemented |
| Idle reset after N minutes | ✅ | FindActive checks idle timeout |
| Session restore on restart | ✅ | LoadFromDB on startup |
| CLI: `prism session list/show` | ⬜ | Not yet implemented |

### M1.4: Agent Router

| Task | Status | Notes |
|------|--------|-------|
| Create `internal/router/router.go` | ✅ | Route messages to agents |
| Direct address detection | ✅ | "Lumi, fix this" → lumi |
| @mention routing | ✅ | "@Mango write tests" → mango |
| Default agent fallback | ✅ | No address → primary agent (`primary: true` or first) |
| Intent detection (stretch) | ⬜ | Deferred to Phase 2 |
| Delegation flow | ⬜ | Lumi → Mango → results flow back (Phase 3) |

### M1.5: Discord Adapter (Inbound)

| Task | Status | Notes |
|------|--------|-------|
| Create `internal/adapter/builtin/discordbot/bot.go` | ✅ | discordgo gateway bot |
| Convert Discord message → InboundMessage | ✅ | Channel, user, content, guild, DM detection |
| Handle Discord connection lifecycle | ✅ | connect, reconnect, ready event |
| Handle message types | ✅ | Text, DMs, ignores bot messages |
| Handle message length splitting | ✅ | 2000 char Discord limit, split at newlines |
| Handle threading/replies | ✅ | ChannelMessageSendReply support |
| Bot token from prism.yaml config | ✅ | ChannelConfig.Token |

### M1.6: Discord Adapter (Outbound)

| Task | Status | Notes |
|------|--------|-------|
| Send messages to Discord | ✅ | Bot.Send() method |
| Handle message length limits | ✅ | Split long messages at newlines |
| Handle threading/replies | ✅ | IsReply + ReplyTo support |
| Streaming support (stretch) | ⬜ | Token-by-token delivery (Phase 2) |

### M1.7: Registered Actions

| Task | Status | Notes |
|------|--------|-------|
| Create `internal/action/registry.go` | ✅ | Register actions triggered by events |
| Action configuration in YAML | ✅ | prism.yaml actions section |
| Implement trigger matching | ✅ | Wildcards (`*`, `**`) on event types |
| Built-in actions | ⬜ | prism.cost.track, remembrance.gate.extract (Phase 2) |
| CLI: `prism action list/register` | ⬜ | Not yet implemented |
| Integration test: event → action fires | ✅ | action/registry_test.go |

### M1.8: End-to-End Test

| Task | Status | Notes |
|------|--------|-------|
| Test: config → agents → session → router → action | ✅ | internal/integration/e2e_test.go |
| Test: Discord bot adapter instantiation | ✅ | Bot creation, message handling, send failure without connection |
| Test: session compaction | ✅ | Truncation when over budget |
| Test: action wildcard matching | ✅ | `*.tool.completed`, `**.failed` |
| Test: auto-generated agent IDs | ✅ | prism1, prism2 |

---

## Phase 2: Full Conversation (V21) — 🟡 P1

| Task | Status | Notes |
|------|--------|-------|
| Remembrance as Prism service | ✅ | V26: NATS subscriber auto-captures *.agent.output |
| Context auto-save after every run | ✅ | V26: RemembranceStage captures + builds context |
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

## Phase 5: Visual Representations (V24) — ✅ Complete

| Task | Status | Notes |
|------|--------|-------|
| Agent Topology diagram | ✅ | SVG generator with roles, capabilities, primary marker |
| Feedback Loops diagram | ✅ | Three Feedback Loops with solid/dashed/dotted arrows |
| Delegation Flow diagram | ✅ | Pipeline stages, task lifecycle, approval gate diamond |
| Approval Gate diagram | ✅ | Request → diamond → grant/deny with event names |
| Event Flow diagram | ✅ | Per-agent namespaces, NATS bus, system events |
| Dashboard Workflow tab | ✅ | 6-tab dashboard with SVG rendering, diagram type selector |
| API endpoint | ✅ | GET /api/v1/workflows/{type} and /api/v1/workflows/list |

## Phase 6: Visual Workflow Editor (V25) — ✅ Complete

| Task | Status | Notes |
|------|--------|-------|
| EditorState model + ConfigWriter | ✅ | Node/Edge model, ConfigToEditorState, WriteConfigYAML, validation |
| API endpoints for editor CRUD | ✅ | 10 endpoints: GET/PUT editor, CRUD nodes/edges, save |
| Dashboard interactive editor | ✅ | SVG drag-and-drop, edge drawing, properties panel |
| Config round-trip test | ✅ | yaml → editor → yaml round-trip verified |
| Edge drawing + deletion | ✅ | Click-to-connect edges, keyboard delete |
| Save/write-back to prism.yaml | ⬜ | Write YAML to disk (needs approval gate) |

## Phase 7: Remembrance Integration (V26) — ✅ Complete

| Task | Status | Notes |
|------|--------|-------|
| NATS subscriber in Remembrance Python | ✅ | nats_sub.py: *.agent.output → auto-capture |
| Serve command (REST + NATS) | ✅ | serve.py: starts both REST API and NATS subscriber |
| RemembranceStage: capture + context | ✅ | Enhanced stage with Capture() and BuildContext() |
| RemembranceStage: unit tests | ✅ | 14 tests covering all paths + health cache |
| Full test suite pass | ✅ | 54 packages, 0 failures |
| E2E test: agent output → memory → context | ✅ | Integration tests with mock Remembrance (5 tests) |
| Wire Remembrance context into serve pipeline | ✅ | Step 7b: BuildContext before LLM call |
| Dream cycle trigger (cron + event) | ✅ | Nightly 3AM + after 10 PERSIST captures |
| Context caching (60s TTL) | ✅ | remembranceCache with RWMutex, cache invalidated on capture |

---

## Phase 4: Platform (V23) — ✅ Complete

| Task | Status | Notes |
|------|--------|-------|
| HTTP API (REST/SSE) | ✅ | 13 endpoints, wired into prism serve:8322 |
| Dashboard v2 | ✅ | 5-tab dashboard with SSE, approval actions |
| Multi-Prism Communication (Bridge) | ✅ | NATS-based, origin tagging, loop prevention |
| Adapter SDK | ✅ | LifecycleAdapter, EventBus, SDKManifest |
| IoT adapter | ⬜ | Smart home control (V25+)
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