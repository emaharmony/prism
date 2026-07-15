# Prism YAML Reference

Field-by-field reference for the YAML files Prism reads. Use it as a manual: find
your file type in the index, copy the example, and edit fields with confidence.

## Overview

Prism uses YAML for workflows, policies, adapters, and the main runtime config
(`prism.yaml`, which contains agent, provider, channel, scheduler, and
remembrance sections). Each section below shows a real example and documents its
fields. Status labels:

- **Preview/Stable** — implemented and exercised by demos/tests.
- **Preview** — implemented; interface may still change.
- **Experimental** — implemented but advanced/optional; expect rough edges.

## YAML File Types

| YAML Type | Example File | Purpose | Status |
|---|---|---|---|
| Workflow YAML | `examples/workflows/demo-echo.yaml` | Ordered workflow steps | Preview/Stable |
| Policy YAML | `policies/default.yaml` | Allow/deny/approval rules | Preview |
| Adapter step (workflow) | `examples/workflows/demo-adapter.yaml` | Dispatch to an adapter | Preview |
| Agent config | `prism.yaml` (`agents:`) | Agent roles/providers | Experimental |
| Provider config | `prism.yaml` (`agents[].provider/model`) | Model providers | Experimental |
| Scheduler config | `prism.yaml` (`prism.scheduler`) | Scheduled runs | Experimental |
| MCP config | `prism.yaml` (`mcp_servers`) | MCP tool servers | Experimental |
| Remembrance config | `prism.yaml` (`remembrance`) | Memory service client | Experimental |

## Workflow YAML

### Purpose

Defines a named workflow that runs Prism steps in order. Workflows are loaded
from `examples/workflows/`.

### Example

```yaml
name: demo.echo_tool
description: Demo workflow that runs an echo tool.
version: 1

steps:
  - id: echo
    type: tool.execute
    tool: echo
    input:
      text: "hello from workflow"
```

### Fields

| Field | Type | Required? | Description |
|---|---|---:|---|
| `name` | string | Yes | Unique workflow name (used by `workflow run <name>`) |
| `description` | string | No | Human-readable description |
| `version` | number | Recommended | Workflow version |
| `steps` | list | Yes | Ordered workflow steps |

### Natural Gates Token Budgets

Natural Gates workflow configs (`version: 2`, such as `examples/workflows/gated-loop.yaml`) use `global.max_total_tokens` as the run-wide prompt+completion ceiling:

| Value | Meaning |
|---:|---|
| `-1` | Explicitly unlimited. Use only when an external guardrail bounds the run. |
| `0` or omitted | Built-in default ceiling (`2,000,000` tokens). |
| Positive integer | Explicit run ceiling in tokens. |
| Less than `-1` | Invalid; config loading rejects it. |

Project entries in `prism.yaml` may set `projects[].token_budget` with the same `-1` / `0` / positive semantics. A project token budget overrides the workflow's `global.max_total_tokens` for that project.

### Step Fields

| Field | Type | Required? | Description |
|---|---|---:|---|
| `id` | string | Yes | Unique step ID inside the workflow |
| `type` | string | Yes | Step type, e.g. `tool.execute`, `dispatch.run`, `delegate` |
| `when` | string | No | Condition guarding step execution |
| `input` | object | No | Step-specific input |
| `tool` | string | For `tool.execute` | Tool name |
| `adapter` | string | For `dispatch.run` | Dispatch adapter name |
| `action` | string | For `dispatch.run` | Adapter action name |
| `agent` | string | For `delegate` | Target agent id |

### Adapter Dispatch Example

```yaml
name: demo.adapter_echo
description: Demo workflow that uses the echo adapter
version: 1
steps:
  - id: echo
    type: dispatch.run
    adapter: echo
    action: echo
    input:
      message: "hello from adapter"
```

### Multi-Agent Delegation Example

```yaml
name: code-and-review
version: 1
description: "Plan, implement, and review code with multiple agents"
steps:
  - id: plan
    type: delegate
    agent: planner
    input:
      task: "Break down this task: {{ .task }}"
  - id: implement
    type: delegate
    agent: coder
    when: "step.plan.status == completed"
    input:
      task: "Implement the plan:\n{{ step.plan.output }}"
```

## Policy YAML

### Purpose

Defines allow/deny/approval rules. Policy decides permission; local validators
still enforce input safety.

### Example

```yaml
policies:
  - id: deny_shell_execution
    description: Block shell execution.
    match:
      action: tool.execute
      resource.name: run_command
    decision: denied
    reason: Shell execution is not supported.
    severity: critical
```

### Fields

| Field | Type | Required? | Description |
|---|---|---:|---|
| `policies` | list | Yes | List of policy rules |
| `id` | string | Yes | Unique policy rule ID |
| `description` | string | No | Human-readable explanation |
| `match` | object | Yes | Exact-match criteria (e.g. `action`, `resource.name`, `context.mode`) |
| `decision` | string | Yes | `allowed`, `denied`, or `requires_approval` |
| `reason` | string | Recommended | Explanation recorded in artifacts/events |
| `severity` | string | No | e.g. `warning`, `critical` |

## Adapter YAML

Adapter dispatch is expressed as a workflow step (`type: dispatch.run`) as shown
above. Inspect available adapters with `prism adapter list|show|health`. The
`echo` adapter is safe and used by the adapter demo. Domain adapters (e.g.
trading) are gated by policy — see the `deny_live_trading_adapter` rule in
`policies/default.yaml`.

## Agent YAML

### Purpose

Agents are defined under `agents:` in `prism.yaml`. Each agent's `id` becomes its
event namespace prefix.

### Example

```yaml
agents:
  - id: astraea
    role: orchestrator
    provider: ollama
    model: "llama3.2"
    primary: true
    context:
      - soul
      - agents
      - user
    capabilities:
      - plan
      - route
      - review
      - report
```

### Fields

| Field | Type | Required? | Description |
|---|---|---:|---|
| `id` | string | Yes | Unique agent id / event namespace prefix |
| `role` | string | Recommended | Agent role (e.g. `orchestrator`, `coder`, `reviewer`) |
| `provider` | string | Yes | Provider: `mock`, `ollama`, `openai`, `openai_responses`, `anthropic`, `gemini`, `claude_code` |
| `model` | string | Yes | Model name for the provider |
| `primary` | bool | No | Marks the default agent |
| `context` | list | No | Context layers to inject (e.g. `soul`, `agents`, `user`) |
| `capabilities` | list | No | Capability tags used for routing |
| `invocable_via_api` | bool | No | Opt into `POST /api/v1/agents/<id>/invoke` (requires API token) |

## Provider YAML

Providers are not a separate file; they are selected per agent via `provider:`
and `model:` (see Agent YAML) or via `--provider`/`--model` on `prism run`.
API-backed providers require credentials in environment variables (e.g.
`OPENAI_API_KEY`).

## Scheduler YAML

### Example

```yaml
prism:
  scheduler:
    enabled: false
    jobs:
      - name: "status-report"
        schedule: "0 */2 * * *"
        event: "prism.task.scheduled"
        payload:
          action: "status_report"
        enabled: true
```

Full cron reference: [docs/SCHEDULER.md](../operations/SCHEDULER.md).

## MCP YAML

### Example

```yaml
mcp_servers:
  - name: "filesystem"
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-filesystem", "D:/projects"]
    env: []
    enabled: true

mcp_auto_approve: false
```

MCP tools register as `mcp_<name>_<tool>` and run through the same policy engine
as built-in tools. Probe before enabling: `prism mcp probe`.

## Remembrance YAML

### Example

```yaml
remembrance:
  enabled: false
  url: "http://localhost:18790"
  timeout_seconds: 60
```

## Full Examples

- Workflow: [`examples/workflows/demo-echo.yaml`](../../examples/workflows/demo-echo.yaml)
- Adapter step: [`examples/workflows/demo-adapter.yaml`](../../examples/workflows/demo-adapter.yaml)
- Multi-agent: [`examples/workflows/demo-agents.yaml`](../../examples/workflows/demo-agents.yaml)
- Gated loop: [`examples/workflows/gated-loop.yaml`](../../examples/workflows/gated-loop.yaml)
- Policy: [`policies/default.yaml`](../../policies/default.yaml)
- Full runtime config: [`prism.yaml.example`](../../prism.yaml.example)

## Validation Rules

- Workflow names should be unique.
- Step IDs must be unique inside a workflow.
- Unknown step types should fail clearly.
- Policy rule IDs should be unique.
- `decision` must be one of `allowed`, `denied`, `requires_approval`.
- Secrets should not be stored in YAML — use environment variables for API keys.

Validate your `prism.yaml` with:

```bash
go run ./cmd/prism-cli config --config prism.yaml
```

## Common Mistakes

### Duplicate step IDs

Bad:

```yaml
steps:
  - id: run
    type: tool.execute
  - id: run
    type: validation.run
```

Good:

```yaml
steps:
  - id: run_tool
    type: tool.execute
  - id: run_validation
    type: validation.run
```

### Storing secrets in YAML

Bad:

```yaml
openai_api_key: sk-...
```

Good — reference an environment variable instead:

```yaml
# set OPENAI_API_KEY in your environment; do not inline the key
auth_token_env: "PRISM_API_TOKEN"
```
