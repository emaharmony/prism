# Prism Stability Matrix

Every major feature in Prism is assigned a stability level to set expectations for production use and API longevity.

## Stability Levels

| Level | Description | Backward Compatibility | Production Ready |
|---|---|---|---|
| **Stable** | Hardened, tested, and used in production-like environments. | Yes (SemVer) | Yes |
| **Preview** | Feature complete but awaiting broader feedback. | Likely | Partial |
| **Experimental** | New ideas or integration explorations. | No | No |
| **Internal** | Not intended for external use. | N/A | No |
| **Deprecated** | Planned for removal. | No | No |

## Feature Classification

| Feature | Status | Default Enabled | Flag | Production Recommendation |
|---|---|---|---|---|
| Workflow Engine | **Stable** | Yes | N/A | Recommended |
| Event Bus (Embedded NATS) | **Stable** | Yes | N/A | Recommended |
| Canonical Run Artifacts | **Stable** | Yes | N/A | Recommended |
| Policy Engine | **Stable** | Yes | N/A | Recommended |
| Approval Gates | **Stable** | Yes | N/A | Recommended |
| Validation Gates | **Stable** | Yes | N/A | Recommended |
| Tool Registry | **Stable** | Yes | N/A | Recommended |
| Local Providers | **Stable** | Yes | N/A | Recommended |
| Remote Providers | **Stable** | Yes | N/A | Recommended |
| Remembrance Integration | **Experimental** | No | `remembrance.enabled` | Lab only |
| Scheduler | **Preview** | Yes | N/A | Evaluation |
| Dashboard | **Preview** | Yes | N/A | Evaluation |
| API (v1) | **Preview** | Yes | N/A | Evaluation |
| Discord Adapter | **Preview** | No | `channels` config | Evaluation |
| MCP Integration | **Experimental** | No | `mcp.enabled` | Lab only |
| Cross-Prism Delegation | **Experimental** | No | `bridge.enabled` | Lab only |
| Sub-agents | **Experimental** | No | `subagents.enabled` | Lab only |
| Autopatch | **Experimental** | No | `autopatch.enabled` | Lab only |
| Codex Worker | **Preview** | No | `agents.capabilities` | Evaluation |
| Claude Code Worker | **Experimental** | No | `agents.capabilities` | Lab only |
| Roblox Factory | **Experimental** | No | `agents.capabilities` | Lab only |
| Firecrawl | **Experimental** | No | `agents.capabilities` | Lab only |
| Visual Workflow Editing | **Experimental** | No | N/A | Lab only |
| Worktree Mutation | **Preview** | No | `autopatch.enabled` | Evaluation |

## Known Limitations

*   **Concurrency:** High-load concurrent API access may experience rare data races in status polling.
*   **Windows Paths:** While supported, some experimental integrations may have path-separator issues.
*   **Memory Overhead:** Remembrance service (Python) requires separate resource management.
*   **NATS Persistence:** Embedded NATS is configured for file storage by default; ensure disk space availability in `prism-data`.
