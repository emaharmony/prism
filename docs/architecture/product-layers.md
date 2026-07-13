# Product layers

Layering makes required trust boundaries explicit without forcing a risky
package move. `internal/` remains the physical layout; this page is the
normative ownership map for incremental separation.

Owner below means the repository owner/maintainer identified in
`PRISM_OWNERSHIP.md`. Optional systems must not weaken a Core decision.

| Subsystem | Layer | Stability | Required | Default | Trust boundary |
|---|---|---|---:|---|---|
| Events, lifecycle, workflows, routing, planning | Core | Preview | Yes | On | Models emit proposals; runtime owns transitions |
| Policy, approvals, validation, mutation | Core | Preview | Yes | On | Deterministic checks and human gates authorize effects |
| Persistence contracts, artifacts, tracing | Core | Preview | Yes | On | Durable records are runtime-owned evidence |
| Tool and capability enforcement | Core | Preview | Yes | On | Registry + policy + validators bound execution |
| API, health, dashboard, sessions | Platform | Preview | No | Serve-only | Auth and loopback binding protect state surfaces |
| Scheduler, service mode, configuration | Platform | Preview | No | Off/explicit | Config cannot bypass Core gates |
| Provider abstraction | Platform | Preview | No | Mock only | Untrusted model output remains inside Core lifecycle |
| Remembrance client | Platform | Experimental | No | Off | Remote memory is untrusted context, never authority |
| Discord | Integration | Experimental | No | Off | External messages enter through channel and policy checks |
| MCP | Integration | Experimental | No | Off | External tools use the same policy executor; no auto-approval by default |
| Codex and Claude CLIs | Integration | Experimental | No | Off | Child process output cannot self-authorize mutation |
| Cross-Prism / Factory / Roblox | Integration | Experimental | No | Off | Signed/adapted messages remain outside Core authority |
| Firecrawl and external CLIs | Integration | Experimental | No | Off | Network/process boundary; explicit configuration required |

All rows are owned by Emmanuel Vinas until a `CODEOWNERS` or subsystem owner is
assigned. See the [stability matrix](../reference/stability-matrix.md) for
testing and production recommendations.
