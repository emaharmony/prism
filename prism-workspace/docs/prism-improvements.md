# Prism Improvement Log

**Created:** 2026-06-06
**Purpose:** Track bugs, issues, and improvement ideas for Prism discovered during Pudgy Power development

---

## Bugs & Issues

| # | Date | Severity | Description | Status |
|---|------|----------|-------------|--------|
| P-001 | 2026-06-06 | Medium | Dream cycle returns 404 — Prism called POST `/dream` but Remembrance routes are under `/v1/`. Fixed: added `/v1/dream` endpoint to Remembrance + updated Prism client to `/v1/dream`. Placeholder implementation — full dream logic pending. | Fixed |
| P-002 | 2026-06-06 | Low | No CLI command to send messages to other agents. Need `prism message --agent <id> --channel <id> --text <msg>` or similar. | Open |
| P-003 | 2026-06-06 | Medium | Agent-to-agent task needs Prism CLI support. Currently requires Discord message from external bot, not clean Prism-to-Prism communication. | Open |
| P-005 | 2026-06-07 | High | openclaw-lumi messages ignored by Prism agents. Bot ID 1512994928769237002 not in listen_to_agents for either Prism Lumi or Astraea. Need to add to both configs. | Open |
| P-004 | 2026-06-06 | High | OpenClaw bot lacked access to factory channel. Fixed — new bot token provided by Ema. Bot `openclaw-lumi#3208` (ID 1512994928769237002) now has access to build-room channel (1491622824991920231). | Fixed |

## Improvement Ideas

| # | Date | Description | Priority |
|---|------|-------------|----------|
| — | — | — | — |

## Notes

This file logs Prism system issues found while using Lumi ↔ Astraea for game development. We'll address these after the game architecture session.
| P-006 | 2026-06-07 | ~~High~~ Not a bug | Agent messages appeared empty — NOT a Prism bug. openclaw-lumi bot only had VIEW_CHANNEL (1024) permission, missing READ_MESSAGE_HISTORY (131072). Messages ARE there, just not readable by openclaw-lumi. | Closed |
| P-007 | 2026-06-07 | ~~Medium~~ High (fixed) | Go map iteration randomness in `executeChatTool` and `formatToolResult` — both iterated `result.Output` map values randomly, sometimes picking "path" before "content". LLM got file paths but not file contents. Fixed with priority key lookup: content → output → result → body → JSON fallback. | Fixed |
| P-008 | 2026-06-07 | High (implemented) | Tool overeagerness — agents call read_file/project_overview for simple greetings. Architectural fix: ToolRelevanceGate pipeline stage with heuristic keyword+pattern matching. Three decisions: Include/Exclude/Subset. 15 unit tests. Mango review pending. | Implemented (Mango review pending) |
