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
| V1 | Foundation: CLI, event bus, canonical events | [V1](./design/V1-FOUNDATION-DESIGN.md) |
| V2 | Real LLM execution and provider interface | [V2](./design/V2-REAL-LLM-EXECUTION-DESIGN.md) |
| V3 | Controlled tool execution | [V3](./design/V3-CONTROLLED-TOOL-EXECUTION-DESIGN.md) |
| V4 | Approval-gated mutations | [V4](./design/V4-APPROVAL-GATED-MUTATIONS-DESIGN.md) |
| V5 | Validation and deterministic review | [V5](./design/V5-VALIDATION-REVIEW-DESIGN.md) |
| V6 | Gate/trading work moved out of core Prism | [V6](./design/V6-GATE-TRADING-MOVED.md) |
| V7 | Workflow runtime | [V7](./design/V7-WORKFLOW-RUNTIME-DESIGN.md) |
| V8 | Policy engine | [V8](./design/V8-POLICY-ENGINE-DESIGN.md) |
| V9 | Adapter contract and SDK | [V9](./design/V9-ADAPTER-CONTRACT-DESIGN.md) |
| V10 | State projections | [V10](./design/V10-STATE-PROJECTIONS-DESIGN.md) |
| V11 | Dashboard | [V11](./design/V11-DASHBOARD-DESIGN.md) |
| V12 | CLI and architecture refactor | [V12](./design/V12-ARCHITECTURAL-REFACTOR-DESIGN.md) |
| V13 | Multi-agent orchestration | [V13](./design/V13-MULTI-AGENT-DESIGN.md) |

## Platform Expansion (V14–V34)

| Version | What | Design Doc |
|---------|------|------------|
| V14a-e | Pipeline, streaming, providers, SQLite, Discord coverage | [V14a](./design/V14a-DECOMPOSE-STREAM-DESIGN.md) |
| V15 | Vector search | [V15](./design/V15-VECTOR-SEARCH-DESIGN.md) |
| V16 | Intelligence arc | [V16](./design/V16-INTELLIGENCE-ARC-DESIGN.md) |
| V17 | Performance | [V17](./design/V17-PERFORMANCE-DESIGN.md) |
| V18 | OpenClaw config transfer | [V18](./design/V18-OPENCLAW-CONFIG-DESIGN.md) |
| V19 | Smart context injection | [V19](./design/V19-SMART-CONTEXT-DESIGN.md) |
| V20 | Live orchestrator | [V20](./design/V20-LIVE-ORCHESTRATOR-DESIGN.md) |
| V21 | Full conversation pipeline | [V21](./design/V21-FULL-CONVERSATION-DESIGN.md) |
| V22 | Multi-agent delegation | [V22](./design/V22-MULTI-AGENT-ORCHESTRATION-DESIGN.md) |
| V23 | Platform API, bridge, dashboard, SDK | [V23](./design/V23-PLATFORM-DESIGN.md) |
| V24 | Visual workflow diagrams | [V24](./design/V24-VISUAL-WORKFLOW-DESIGN.md) |
| V25 | Visual workflow editor | [V25](./design/V25-VISUAL-WORKFLOW-EDITOR-DESIGN.md) |
| V26 | Remembrance integration | [V26](./design/V26-REMEMBRANCE-INTEGRATION-DESIGN.md) |
| V27 | Serve-mode tool executor | [V27](./design/V27-SERVE-TOOL-EXECUTOR-DESIGN.md) |
| V28 | Project and git tools | [V28](./design/V28-PROJECT-GIT-TOOLS-DESIGN.md) |
| V29 | Tool guidance and session awareness | [V29](./design/V29-TOOL-GUIDANCE-SESSION-AWARENESS-DESIGN.md) |
| V30 | Native Ollama tool calling | [V30](./design/V30-NATIVE-TOOL-CALLING-DESIGN.md) |
| V31 | Native chat streaming gap note | [V31](./design/V31-CHAT-STREAMING-GAP.md) |
| V32 | Operating environment: state, plans, guard, wake | [V32](./design/V32-LUMI-OPERATING-ENVIRONMENT.md) |
| V33 | Conversation awareness and channel context | [V33](./design/V33-CONVERSATION-AWARENESS.md) |
| V34 | OpenAI Responses provider | [V34](./design/V34-OPENAI-RESPONSES-DESIGN.md) |

## Gated Loop & Observability (V35–V48)

| Version | What | Design Doc |
|---------|------|------------|
| V35 | Objective verification gate in the gated loop | [V35](./design/V35-VERIFICATION-GATE-DESIGN.md) |
| V37 | Multi-agent delegation wired into the gated loop | [V37](./design/V37-MULTI-AGENT-DELEGATION-DESIGN.md) |
| V38 | `prism watch` live run visibility (SSE) | [V38](./design/V38-LIVE-WATCH-DESIGN.md) |
| V39 | `prism doctor` preflight health check | [V39](./design/V39-DOCTOR-PREFLIGHT-DESIGN.md) |
| V40 | Durable `REPORT.md` proof-of-work artifact | [V40](./design/V40-REPORT-ARTIFACT-DESIGN.md) |
| V41 | Diff preview at the feedback gate | [V41](./design/V41-DIFF-PREVIEW-DESIGN.md) |
| V42 | Gate-needs-you @-mention notifications | [V42](./design/V42-GATE-NOTIFICATIONS-DESIGN.md) |
| V43 | `prism preview` static gated-loop preview | [V43](./design/V43-WORKFLOW-PREVIEW-DESIGN.md) |
| V44 | Rich Discord approval cards (buttons) | [V44](./design/V44-APPROVAL-CARDS-DESIGN.md) |
| V45 | De-hardcoded gate personas (config-driven roster) | [V45](./design/V45-DYNAMIC-ROSTER-DESIGN.md) |
| V46 | `prism runs` browse past runs & reports | [V46](./design/V46-RUNS-BROWSER-DESIGN.md) |
| V47 | `--json` inspection output (doctor, runs) | [V47](./design/V47-JSON-OUTPUT-DESIGN.md) |
| V48 | Actions on flows in the visual workflow editor | [V48](./design/V48-EDGE-ACTIONS-DESIGN.md) |

## Advanced / Experimental (V49–V58)

| Version | What | Design Doc |
|---------|------|------------|
| V49 | MCP client foundation (consume external MCP tool servers) | [V49](./design/V49-MCP-CLIENT-DESIGN.md) |
| V50 | Self-patching PR mode (autopatch opens pull requests) | [V50](./design/V50-AUTOPATCH-PR-DESIGN.md) |
| V51 | Issue-discovery scanner + `prism scan` (self-directed autopatch) | [V50](./design/V50-AUTOPATCH-PR-DESIGN.md) |
| V52 | `prism config` validate + summarize prism.yaml | [V52](./design/V39-DOCTOR-PREFLIGHT-DESIGN.md) |
| V54 | Skill-use capabilities (Claude Code / OpenClaw SKILL.md) | [V54](./design/V54-SKILLS-DESIGN.md) |
| V55 | Config wizard + OpenClaw→prism.yaml import | [V55](./design/V55-CONFIG-WIZARD-DESIGN.md) |
| V56 | Gated-loop worktree isolation | [V56](./design/V56-WORKTREE-ISOLATION-DESIGN.md) |
| V57 | Auto-rollback for failed loops + per-phase token budgets | [V57](./design/V57-AUTO-ROLLBACK-DESIGN.md) |
| V58 | Full autonomy: generic sub-agent worker (bounded tool-loop, worktree isolation, capability routing) | [V58](./design/V58-FULL-AUTONOMY-DESIGN.md) |
