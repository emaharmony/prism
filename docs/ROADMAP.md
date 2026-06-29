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
| V39 | Source-current | `prism doctor` — preflight health check (workspace, provider auth, validation profile, git remote, NATS, Remembrance) |
| V40 | Source-current | Durable `runs/<id>/REPORT.md` proof-of-work artifact written at end of each gated-loop run |
| V41 | Source-current | Diff preview (`git diff --stat`) attached to the FEEDBACK_POST review message so approvers see what changed |
| V42 | Source-current | Gate-needs-you notifications — feedback pauses @-mention the named approvers/reviewers to ping them |
| V43 | Source-current | `prism preview` — static gated-loop preview (phases, gates, budgets, verification) before launching |
| V44 | Source-current | Rich Discord approval cards — interactive Approve/Changes/Reject buttons at feedback gates |
| V45 | Source-current | De-hardcoded gate personas — review/approval messages name config-driven reviewers/approvers; agent glyphs overridable; config-driven per-agent delegation timeouts (no baked-in Lumi/Mango) |
| V46 | Source-current | `prism runs` — list past gated-loop runs and read their persisted REPORT.md from the terminal |
| V47 | Source-current | `--json` output for `prism doctor` and `prism runs` (CI/dashboard-consumable inspection) |

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
