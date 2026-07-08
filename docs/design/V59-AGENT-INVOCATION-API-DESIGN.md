# V59 — Agent Invocation API

**Status:** Source-current
**Last Updated:** 2026-07-07

## Goal

Give external processes — addons that run outside the Prism binary and don't import
its internal Go packages — a minimal, public way to ask one configured agent one
question and get a structured result back over plain HTTP. This is the "general
addon mechanism": any future addon (not just the first consumer, an OBS stream-clip
detector called Clippy) reuses this with zero further core changes.

## Why it was needed

Prism had no production-ready contract for "submit a prompt to a named agent, get a
result" that's both intentionally public and on by default:

- `POST /api/v1/workflows/start` has no agent-targeting field and routes through the
  heavyweight gated-loop engine — built for dev-workflow prompts, not single
  judgment calls.
- The NATS `TaskPacket`/`TaskCompletion` pair on `prism.agent.openclaw` /
  `prism.workflow.task.complete` (`cmd/prism-cli/subagent_worker.go`) is the closest
  existing shape, but is feature-flagged off by default (`PRISM_SUBAGENT_WORKER`),
  requires an externally-reachable NATS broker (the default embedded bus is
  loopback-only), and its own handler comment notes task processing isn't fully
  wired.
- The adapter SDK (`internal/adapter/sdk.go`) and MCP-client design
  (`docs/V49-MCP-CLIENT-DESIGN.md`) are both explicitly in-process-only /
  one-directional-outbound by design — see `docs/V9-ADAPTER-CONTRACT-DESIGN.md`'s
  "What V9 Does NOT Include: Plugin marketplace or remote loading."

## Design

`POST /api/v1/agents/{id}/invoke` + `GET /api/v1/agents/{id}/invocations/{invocation_id}`,
added to the existing `internal/api` server (`cmd/prism-cli/cmd_serve.go`'s
`prism serve`), reusing everything that already exists rather than inventing new
infrastructure:

- **Auth**: the same `authMiddleware`/Bearer-token check every other state-changing
  POST already goes through. The GET poll endpoint is unauthenticated, matching the
  existing "read-only GETs stay open" convention — an invocation ID is an
  unguessable ULID and its response contains only the judgment result.
- **Opt-in**: `AgentConfig.InvocableViaAPI` (`internal/orchestrator/config.go`,
  default `false`). Without this, any process with API access could invoke *any*
  agent, including ones with `delegate`/`code` capabilities. A 403 is returned for
  agents that haven't opted in.
- **Execution**: a genuinely single-shot LLM call — not the session/stage/tool-loop
  pipeline. `runInvocation` in `internal/api/server.go` resolves the agent's
  `Provider`/`Model`/`ConversationPostfix` from its `AgentConfig` and calls
  `provider.ChatProvider.ChatGenerate`, the same call shape `wake_handler.go`
  already uses for its own direct/scripted wake actions. No session persistence, no
  tool loop, no approval-gate machinery — the invocation itself is read-only (a
  judgment call), so any consequential action stays entirely on the caller's side.
- **Result delivery**: async (`202 Accepted` with an `invocation_id` — LLM calls take
  seconds). Callers either poll `GET .../invocations/{id}` or hold an SSE connection
  on the *existing* `GET /api/v1/events/stream?subject=<agent-id>.invocation.>`
  endpoint, since that's already a generic NATS→SSE bridge — completion is published
  to `<agent-id>.invocation.completed` (`internal/agentns`) for free, no new
  streaming code needed.
- **State**: `internal/invocation.Store` is an in-memory map, not SQLite. Invocations
  are short-lived (seconds) — callers are expected to retrieve a result within the
  invocation's lifetime, so durability across a `prism serve` restart isn't a
  requirement here, unlike session state.
- **Result parsing**: agents invoked this way are expected to respond with strict
  JSON per their `conversation_postfix` instructions. `invocation.ParseResult`
  attempts to parse the raw response as a JSON object; on failure it wraps the raw
  text as `{"text": "..."}` so callers always get a stable response shape rather
  than a parse error.

## Non-goals

- No tool calling, no multi-turn conversation, no approval gate. If an addon needs
  those, it should use the full session/workflow surfaces, not this endpoint.
- No new auth mechanism, no new NATS infrastructure, no plugin-loading /
  dynamic-registration system for addons in general — this is intentionally the
  smallest useful primitive, not a general plugin framework.

## First consumer

Clippy — a separate, independently distributed addon (its own git repo, own `go.mod`,
own binary) that watches OBS + Twitch chat for clip-worthy stream moments and calls
`POST /api/v1/agents/clippy/invoke` for the final cloud judgment call, after two
cheaper local tiers (signal detection, local vision confirmation) have already
filtered out noise. See the `clippy` agent example in `prism.yaml.example`.
