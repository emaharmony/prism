# V49 — MCP Client Foundation

**Status:** Source-current (foundation; transport + wiring are follow-ups)
**Last Updated:** 2026-06-29

## Goal

Let Prism consume external **Model Context Protocol (MCP)** tool servers
(filesystem, GitHub, Slack, databases, …) so the autonomous-dev loop can use the
whole MCP ecosystem — while keeping Prism's safety model. Per the MCP analysis: the
high-leverage direction is Prism-as-MCP-**client**, registering remote tools into
the existing policy-gated `tool.Registry`.

## Why it fits

Prism's `tool.Tool` interface (`Name/Description/Schema/Execute`) maps almost 1:1
onto an MCP tool (`name/description/inputSchema/tools-call`). Crucially, **every**
tool runs through `tool.Executor.ExecuteWithPolicy` (policy → approval → execute →
events). So an MCP-backed tool registered in the `Registry` inherits policy gating,
approval, path/write enforcement, audit events, and the gated loop's per-phase
`allowed_tools` whitelist — with **no engine or core change**.

## This iteration (foundation)

`internal/tool/mcp`:

- **`Client` interface** — `ListTools(ctx)` / `CallTool(ctx, name, args)`. The
  package is transport-agnostic; a concrete stdio JSON-RPC / streamable-HTTP client
  is a separate, swappable implementation. This keeps the mapping logic
  unit-testable without spawning a subprocess or opening a socket.
- **`ToolDef` / `CallResult`** — the MCP shapes Prism needs.
- **`ToolName(server, tool)`** — namespaces remote tools as
  `mcp_<server>_<tool>` (sanitized) so servers can't collide and the names are easy
  to policy-scope.
- **`SchemaFromJSON`** — maps an MCP JSON-Schema input object to a Prism
  `ToolSchema` (properties → `ParamSpec`, honoring `required`), degrading gracefully
  on odd/empty schemas.
- **`mappedTool`** — adapts one MCP tool to `tool.Tool`; `Execute` proxies the
  `CallTool`, maps an MCP `isError` (or transport error) to a failed `ToolResult`
  rather than a Go error, so the agent can recover.
- **`RegisterServer(ctx, reg, server, client)`** — lists the server's tools and
  registers a wrapper per tool into a `tool.Registry`, returning the names and
  collecting per-tool registration errors without aborting the rest.

## Tests

`internal/tool/mcp/mcp_test.go`: namespacing/sanitize; schema mapping (types,
required, nil); **end-to-end** register-into-a-real-`tool.Registry` then
`reg.Execute(...)` proxies to a mock server (proving the remote tool is a
first-class Prism tool); MCP-`isError` and transport-error → failed result; and
`RegisterServer` guards (nil registry, list error).

## Follow-ups (the rest of MCP client support)

1. **Transport:** a concrete stdio JSON-RPC 2.0 client (spawn `npx <server>` etc.)
   implementing `Client`, then a streamable-HTTP transport — modeled on the
   existing `claudecode`/subprocess patterns.
2. **Config:** an `mcp_servers:` block in `prism.yaml` (command/args/env or url),
   wired in `prism serve` to call `RegisterServer` at startup.
3. **Policy defaults:** MCP tools default to **approval-required** (untrusted remote
   tool descriptions/schemas are attacker-controlled text reaching the model;
   opt-in per server). Reuse the policy engine — no new safety primitive.
4. **Reverse direction (optional):** expose Prism's `Registry` as an MCP **server**
   for interop with Claude Desktop / other hosts.
