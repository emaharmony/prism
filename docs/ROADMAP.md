# Prism Roadmap

**Last Updated:** 2026-06-09
**Status:** Core runtime is beyond V26 in source. Current work is runtime/docs alignment, plan/state/guard hardening, event-driven wake, and cross-Prism/Factory reliability.

---

## Progress

| Version | Status | Summary |
|---------|--------|---------|
| V1-V5 | Shipped | Events, CLI, LLM calls, tools, approvals, validation/review |
| V6-V9 | Shipped | Domain cleanup, workflows, policy engine, adapter contract |
| V10-V13 | Shipped | Projections, dashboard, refactor, multi-agent orchestration |
| V14a-e | Shipped | Pipeline stages, streaming provider interface, provider coverage, SQLite, Discord coverage |
| V15-V19 | Shipped | Vector search, cost/context intelligence, performance, OpenClaw config transfer, smart context |
| V20 | Shipped | Persistent `prism serve`, Discord bot, sessions |
| V21 | Shipped | Full conversation pipeline and StreamCallback pattern |
| V22 | Shipped | Delegation, task lifecycle, capabilities, approvals |
| V23 | Shipped | REST/SSE API, dashboard, bridge, adapter SDK |
| V24-V25 | Shipped | SVG diagrams and visual workflow editor |
| V26 | Shipped | Remembrance capture/context/cache integration |
| V27 | Source-current | Serve-mode tool executor and policy wiring |
| V28 | Source-current | Project comprehension, git tools, rate limiting |
| V29 | Source-current | Stronger tool guidance and session-aware prompt behavior |
| V30 | Source-current | Native Ollama chat tool calling |
| V31 | Gap documented | Native chat tool-loop streaming is not implemented yet |
| V32 | Active/draft | State, plans, guard, scheduler, event-driven wake |
| V33 | Active/draft | Channel/context-aware conversation behavior |
| V34 | Source-current | OpenAI Responses provider |
| V35 | Source-current | Gated-loop robustness: build/test verification gate + `run_validation` self-check, run budgets (time/token), stuck-loop detection, idempotent-tool retry (+ phase loop-exit fix) |
| V37 | Source-current | Multi-agent delegation wired into the gated loop: `delegate` tool, async completion routing, timeout escalation |
| V38 | Source-current | `prism watch` — live terminal view of a running gated loop (phase tree, budget meter, delegation status) via SSE |
| V39 | Source-current | `prism doctor` — preflight health check (workspace, provider auth, validation profile, git remote, NATS, Remembrance, autopatch-pr/`gh`, mcp servers, workflow config) |
| V52 | Source-current | `prism config` — validate prism.yaml and summarize what Prism understood (agents, channels, MCP, autopatch mode, workflow) |
| V53 | Source-current | Grouped CLI help — `prism` usage organized into Run/Inspect/Observe/Self-patching/MCP/Approvals/Advanced sections (discoverability for 25+ commands) |
| V54 | Source-current | Skill-use capabilities (Claude Code + OpenClaw SKILL.md) — loader/registry, `use_skill` tool (policy-gated), prompt advertising, `prism serve` wiring, `prism skills` CLI + doctor check. Rated 0→8.75 |
| V55 | Source-current | Config setup made simple — `prism config wizard` (interactive prism.yaml generation) and `prism config import <openclaw.json>` (OpenClaw JSON → prism.yaml, one agent per provider). Emits minimal YAML overlaid on defaults; every generated config is validated by round-tripping through the real loader before write |
| V40 | Source-current | Durable `runs/<id>/REPORT.md` proof-of-work artifact written at end of each gated-loop run |
| V41 | Source-current | Diff preview (`git diff --stat`) attached to the FEEDBACK_POST review message so approvers see what changed |
| V42 | Source-current | Gate-needs-you notifications — feedback pauses @-mention the named approvers/reviewers to ping them |
| V43 | Source-current | `prism preview` — static gated-loop preview (phases, gates, budgets, verification) before launching |
| V44 | Source-current | Rich Discord approval cards — interactive Approve/Changes/Reject buttons at feedback gates |
| V45 | Source-current | De-hardcoded gate personas — review/approval messages name config-driven reviewers/approvers; agent glyphs overridable; config-driven per-agent delegation timeouts (no baked-in Lumi/Mango) |
| V46 | Source-current | `prism runs` — list past gated-loop runs and read their persisted REPORT.md from the terminal |
| V47 | Source-current | `--json` output for `prism doctor` and `prism runs` (CI/dashboard-consumable inspection) |
| V48 | Source-current | Actions on flows — visual workflow editor edges carry an assignable action (clickable lines, ⚡ badge, PUT /editor/edges/{id}) |
| V49 | Source-current | MCP client — adapt external MCP tool servers into the policy-gated tool.Registry; stdio JSON-RPC transport; `mcp_servers:` config wired into `prism serve`; MCP tools approval-required by default (separate AutoApproveMCP opt-in); `prism mcp` inspection + `prism mcp probe <name>` live tool listing |
| V50 | Source-current | Self-patching `pr` mode — validated autopatch fix → branch/commit/push/PR via injectable PROpener (gh default); PR-failure preserves the patch |
| V51 | Source-current | Issue-discovery scanner — autopatch proactively finds issues (CRLF-aware vet/todo/format detectors), ranks them, starts a fix→PR for the top one; `prism scan` surfaces findings with `--severity` filter and `--start` to fix the top issue |
| V56 | Source-current | Gated-loop worktree isolation — `internal/gitx` shared git package (lifted from autopatch); `projects[].worktree_isolation` runs each loop in `<repo>/.prism/worktrees/<run-id>` on a `prism/<run-id>` branch so parallel runs can't collide; fails closed |
| V57 | Source-current | Auto-rollback for failed loops (opt-in `global.auto_rollback`) — verification-attempts cap, blocking-fallback, and end-of-run-red triggers; `workflow.rollback` event; `rolled_back` run status; reset to start SHA / discard run branch; pushed commits never force-removed. Plus per-phase token budgets (`phases[].max_tokens`, `phase.budget_exhausted`) and a 10-fix wiring sweep (autopatch `pr` mode reachable, `prism.ollama_url` live, port/doctor/registry/run-context/gemini/remembrance disconnects) |

---

## Current Priorities

| Priority | Work | Notes |
|----------|------|-------|
| P0 | Keep docs, config, and source aligned | README, onboarding, roadmap, tasks, version docs |
| P0 | Plan/state/guard hardening | Ensure mutation tools are blocked or approved according to plan state |
| P1 | Event-driven wake | Scheduler and wake handler exist; continue testing real workflows |
| P1 | Cross-Prism/Factory handoff | Keep bridge report-only by default; harden validation artifacts |
| P1 | Native chat streaming | V31 gap: tool-loop chat path is sync even though streaming providers exist |
| P2 | API auth middleware | Not currently part of the local-only default |
| P2 | IoT/business templates | Future adapter/template work |

---

## Stable Runtime Surface

- `prism serve` starts health on `8321` and API/SSE/dashboard on `8322` by default.
- `prism run` executes one-shot task lifecycles and expects an external NATS bus.
- `prism chat` provides terminal access to configured agents, tools, state, and plans.
- Providers include mock, Ollama, OpenAI chat completions, OpenAI Responses, Anthropic, and Gemini.
- Remembrance is a separate service on `http://localhost:18790` by current config.
- Tool access is scoped by workspace/allowed paths, policy, channel role, and guard/plan state.
- Cross-Prism and Factory integrations are optional and disabled by default.

---

## Key Decisions

1. Events remain the source of truth.
2. Agent namespaces are dynamic from config IDs.
3. The model does not authorize its own mutations.
4. Remembrance remains a separate HTTP/NATS service.
5. Local development should work with a single Go binary, SQLite, and embedded NATS in serve mode.
6. Historical version docs are snapshots; living docs track current source behavior.
