# Prism Version History

Prism grew through a long series of incremental versions (V1–V58+). This page
preserves that development story so the [README](../README.md) can stay focused
on the current state. Each row links to the original design document.

> These design docs are point-in-time snapshots. For current behavior, prefer
> the [README](../README.md), [Architecture](ARCHITECTURE.md),
> [Capability Status](CAPABILITY_STATUS.md), and [Roadmap](ROADMAP.md).

## Foundations (V1–V13)

| Version | What | Design Doc |
|---------|------|------------|
| V1 | Foundation: CLI, event bus, canonical events | [V1](./V1-FOUNDATION-DESIGN.md) |
| V2 | Real LLM execution and provider interface | [V2](./V2-REAL-LLM-EXECUTION-DESIGN.md) |
| V3 | Controlled tool execution | [V3](./V3-CONTROLLED-TOOL-EXECUTION-DESIGN.md) |
| V4 | Approval-gated mutations | [V4](./V4-APPROVAL-GATED-MUTATIONS-DESIGN.md) |
| V5 | Validation and deterministic review | [V5](./V5-VALIDATION-REVIEW-DESIGN.md) |
| V6 | Gate/trading work moved out of core Prism | [V6](./V6-GATE-TRADING-MOVED.md) |
| V7 | Workflow runtime | [V7](./V7-WORKFLOW-RUNTIME-DESIGN.md) |
| V8 | Policy engine | [V8](./V8-POLICY-ENGINE-DESIGN.md) |
| V9 | Adapter contract and SDK | [V9](./V9-ADAPTER-CONTRACT-DESIGN.md) |
| V10 | State projections | [V10](./V10-STATE-PROJECTIONS-DESIGN.md) |
| V11 | Dashboard | [V11](./V11-DASHBOARD-DESIGN.md) |
| V12 | CLI and architecture refactor | [V12](./V12-ARCHITECTURAL-REFACTOR-DESIGN.md) |
| V13 | Multi-agent orchestration | [V13](./V13-MULTI-AGENT-DESIGN.md) |

## Platform Expansion (V14–V34)

| Version | What | Design Doc |
|---------|------|------------|
| V14a-e | Pipeline, streaming, providers, SQLite, Discord coverage | [V14a](./V14a-DECOMPOSE-STREAM-DESIGN.md) |
| V15 | Vector search | [V15](./V15-VECTOR-SEARCH-DESIGN.md) |
| V16 | Intelligence arc | [V16](./V16-INTELLIGENCE-ARC-DESIGN.md) |
| V17 | Performance | [V17](./V17-PERFORMANCE-DESIGN.md) |
| V18 | OpenClaw config transfer | [V18](./V18-OPENCLAW-CONFIG-DESIGN.md) |
| V19 | Smart context injection | [V19](./V19-SMART-CONTEXT-DESIGN.md) |
| V20 | Live orchestrator | [V20](./V20-LIVE-ORCHESTRATOR-DESIGN.md) |
| V21 | Full conversation pipeline | [V21](./V21-FULL-CONVERSATION-DESIGN.md) |
| V22 | Multi-agent delegation | [V22](./V22-MULTI-AGENT-ORCHESTRATION-DESIGN.md) |
| V23 | Platform API, bridge, dashboard, SDK | [V23](./V23-PLATFORM-DESIGN.md) |
| V24 | Visual workflow diagrams | [V24](./V24-VISUAL-WORKFLOW-DESIGN.md) |
| V25 | Visual workflow editor | [V25](./V25-VISUAL-WORKFLOW-EDITOR-DESIGN.md) |
| V26 | Remembrance integration | [V26](./V26-REMEMBRANCE-INTEGRATION-DESIGN.md) |
| V27 | Serve-mode tool executor | [V27](./V27-SERVE-TOOL-EXECUTOR-DESIGN.md) |
| V28 | Project and git tools | [V28](./V28-PROJECT-GIT-TOOLS-DESIGN.md) |
| V29 | Tool guidance and session awareness | [V29](./V29-TOOL-GUIDANCE-SESSION-AWARENESS-DESIGN.md) |
| V30 | Native Ollama tool calling | [V30](./V30-NATIVE-TOOL-CALLING-DESIGN.md) |
| V31 | Native chat streaming gap note | [V31](./V31-CHAT-STREAMING-GAP.md) |
| V32 | Operating environment: state, plans, guard, wake | [V32](./V32-LUMI-OPERATING-ENVIRONMENT.md) |
| V33 | Conversation awareness and channel context | [V33](./V33-CONVERSATION-AWARENESS.md) |
| V34 | OpenAI Responses provider | [V34](./V34-OPENAI-RESPONSES-DESIGN.md) |

## Gated Loop & Observability (V35–V48)

| Version | What | Design Doc |
|---------|------|------------|
| V35 | Objective verification gate in the gated loop | [V35](./V35-VERIFICATION-GATE-DESIGN.md) |
| V37 | Multi-agent delegation wired into the gated loop | [V37](./V37-MULTI-AGENT-DELEGATION-DESIGN.md) |
| V38 | `prism watch` live run visibility (SSE) | [V38](./V38-LIVE-WATCH-DESIGN.md) |
| V39 | `prism doctor` preflight health check | [V39](./V39-DOCTOR-PREFLIGHT-DESIGN.md) |
| V40 | Durable `REPORT.md` proof-of-work artifact | [V40](./V40-REPORT-ARTIFACT-DESIGN.md) |
| V41 | Diff preview at the feedback gate | [V41](./V41-DIFF-PREVIEW-DESIGN.md) |
| V42 | Gate-needs-you @-mention notifications | [V42](./V42-GATE-NOTIFICATIONS-DESIGN.md) |
| V43 | `prism preview` static gated-loop preview | [V43](./V43-WORKFLOW-PREVIEW-DESIGN.md) |
| V44 | Rich Discord approval cards (buttons) | [V44](./V44-APPROVAL-CARDS-DESIGN.md) |
| V45 | De-hardcoded gate personas (config-driven roster) | [V45](./V45-DYNAMIC-ROSTER-DESIGN.md) |
| V46 | `prism runs` browse past runs & reports | [V46](./V46-RUNS-BROWSER-DESIGN.md) |
| V47 | `--json` inspection output (doctor, runs) | [V47](./V47-JSON-OUTPUT-DESIGN.md) |
| V48 | Actions on flows in the visual workflow editor | [V48](./V48-EDGE-ACTIONS-DESIGN.md) |

## Advanced / Experimental (V49–V58)

| Version | What | Design Doc |
|---------|------|------------|
| V49 | MCP client foundation (consume external MCP tool servers) | [V49](./V49-MCP-CLIENT-DESIGN.md) |
| V50 | Self-patching PR mode (autopatch opens pull requests) | [V50](./V50-AUTOPATCH-PR-DESIGN.md) |
| V51 | Issue-discovery scanner + `prism scan` (self-directed autopatch) | [V50](./V50-AUTOPATCH-PR-DESIGN.md) |
| V52 | `prism config` validate + summarize prism.yaml | [V52](./V39-DOCTOR-PREFLIGHT-DESIGN.md) |
| V54 | Skill-use capabilities (Claude Code / OpenClaw SKILL.md) | [V54](./V54-SKILLS-DESIGN.md) |
| V55 | Config wizard + OpenClaw→prism.yaml import | [V55](./V55-CONFIG-WIZARD-DESIGN.md) |
| V56 | Gated-loop worktree isolation | [V56](./V56-WORKTREE-ISOLATION-DESIGN.md) |
| V57 | Auto-rollback for failed loops + per-phase token budgets | [V57](./V57-AUTO-ROLLBACK-DESIGN.md) |
| V58 | Full autonomy: generic sub-agent worker (bounded tool-loop, worktree isolation, capability routing) | [V58](./V58-FULL-AUTONOMY-DESIGN.md) |
