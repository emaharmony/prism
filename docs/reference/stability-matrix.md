# Stability matrix

Status is a contract signal, not a production claim. Prism as a whole remains
public preview and is not production-ready.

| Feature | Status | Default | Config flag | Limitations | Testing | Production recommendation |
|---|---|---|---|---|---|---|
| Event lifecycle and run artifacts | Preview | On | n/a | Local durability; no published SLO | Unit + integration | Evaluate with backups and monitoring |
| Workflow list/show/run/status | Preview | On | n/a | Contract may evolve before 1.0 | Unit + integration + smoke | Suitable for local evaluation |
| Policy evaluation | Preview | On | policy files | Policy coverage depends on registry wiring | Unit + integration | Require independent policy review |
| Approval-gated mutation | Preview | On | policy/approvers | Human identity is deployment-specific | Unit + integration | Keep human approval mandatory |
| Validation profiles | Preview | On | workflow verification | Allowlisted commands only | Unit + integration | Use blocking profiles for mutations |
| Path/worktree safety | Preview | On | write roots / isolation | Symlink tests may skip where host forbids symlinks | Cross-platform unit | Enable isolation and least-privilege roots |
| Embedded NATS + SQLite | Preview | Serve | `prism.nats_url`, data paths | No HA or published contention SLO | Unit + integration | Single-node evaluation only |
| API/dashboard/SSE | Preview | Serve | `prism.bind_host`, `api.*` | Local-first; auth required off-loopback | Unit + integration | Bind loopback unless hardened |
| Provider abstraction and mock | Preview | Mock | agent/provider | Provider behavior varies | Contract/unit | Mock is the deterministic test path |
| External model providers | Experimental | Off | provider credentials | Network, billing, provider drift | Mocked HTTP | Do not treat as deterministic |
| Scheduler | Experimental | Off | `prism.scheduler.enabled` | No published delivery SLO | Unit | Supervise and make jobs idempotent |
| Multi-agent/sub-agent delegation | Experimental | Off | worker/env/project settings | Complex concurrency and failure modes | Unit + e2e | Keep bounded and reviewed |
| MCP client | Experimental | Off | `mcp_servers`, `mcp_auto_approve` | Stdio only; server is untrusted | Unit | Never auto-approve unknown servers |
| Autopatch / scan | Experimental | Off | `autopatch.enabled` | Local git/`gh`; patch workers vary | Unit + git integration | Propose mode; human review required |
| Remembrance | Experimental | Off | `remembrance.enabled` | Separate Python service | Go mocks + Python unit | Optional context only, not authority |
| Discord / Cross-Prism / Factory | Experimental | Off | integration sections | External identity/network trust | Unit + mocked integration | Evaluation only |
| Python SDK | Experimental | n/a | n/a | No verified tests in this checkout | No dedicated tests found | Do not promise compatibility |

Compatibility expectations: Preview contracts receive best-effort migration
notes before 1.0; Experimental contracts may change without compatibility;
Internal milestone documents carry no compatibility promise. No subsystem is
classified Stable until release artifacts and CI history demonstrate it.
