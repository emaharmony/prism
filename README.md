# Prism - Event-Native AI Agent Platform

[![Go 1.26+](https://img.shields.io/badge/go-1.26%2B-blue)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-go%20test%20.%2F...-brightgreen)]()
[![Packages: 65](https://img.shields.io/badge/packages-65-green)]()
[![License: All Rights Reserved](https://img.shields.io/badge/license-all%20rights%20reserved-red)](./LICENSE)

> **License Notice:** Prism is source-available under an all-rights-reserved license. You may view the repository, but use, modification, distribution, or incorporation requires written permission. See [LICENSE](./LICENSE) for details.

Prism is a Go event-native AI agent platform that runs as a persistent service. Agents communicate through a NATS event bus, maintain conversation sessions, remember context through Remembrance, use tools under policy, and expose a local API/dashboard.

The framework controls lifecycle, safety, context, routing, and persistence. The model generates outputs inside that lifecycle.

Checked-in root binaries can lag behind source. For a fresh setup, build from `cmd/prism-cli` and treat the source tree as authoritative.

---

## Architecture

```text
Prism Runtime
  - Config + orchestrator
  - Embedded or external NATS JetStream
  - SQLite-backed sessions, tasks, approvals, events, and run artifacts
  - Agent router, tool executor, policy/guard checks, plan/state managers
  - Remembrance HTTP client and cache
  - HTTP API, SSE event stream, dashboard, visual workflow editor
  - Optional cross-Prism bridge and Roblox Factory handoff

Ingress/Egress
  - Discord bot
  - CLI run/chat commands
  - REST API clients
  - NATS subjects and scheduled wake events
```

### Core Packages

| Package | Purpose |
|---------|---------|
| `bus/`, `event/`, `run/` | Embedded NATS, canonical events, run lifecycle, WAL-style artifacts |
| `provider/` | Mock, Ollama, OpenAI chat completions, OpenAI Responses, Anthropic, Gemini |
| `agent/`, `router/`, `orchestrator/` | Agent registry, routing, config loading, live service lifecycle |
| `session/`, `task/`, `approval/`, `delegation/` | Conversation state, task tracking, approvals, multi-agent delegation |
| `tool/`, `policy/`, `safety/`, `mutation/`, `guard/` | Tool registry, policy gates, path safety, mutations, plan-aware guard checks |
| `state/`, `plan/`, `prompt/`, `context/` | Working state, task plans, prompt layering, workspace context injection |
| `remembrance/` | Go HTTP client and cache for the separate Remembrance memory service |
| `api/`, `dashboard/`, `editor/`, `workflow/` | REST/SSE API, dashboard, visual editor, SVG workflow diagrams |
| `bridge/`, `crossprism/`, `factory/`, `scheduler/` | Cross-Prism protocol, Factory handoff, scheduled wake events |
| `vector/`, `sse/`, `cost/`, `projection/`, `validation/`, `review/` | Search, streaming helpers, cost tracking, CQRS projections, checks, review artifacts |

---

## Quick Start

### Prerequisites

- Go 1.26 or newer
- Git
- Ollama if using local Ollama models
- Python 3.11+ only if running Remembrance

### Build

```bash
git clone https://github.com/emaharmony/prism.git
cd prism
go build -o prism ./cmd/prism-cli
```

On Windows PowerShell:

```powershell
$env:GOTELEMETRY = "off"
go build -o .\prism-current.exe .\cmd\prism-cli
go build -o .\prism-bus-current.exe .\cmd\prism-bus
```

For a full Windows walkthrough, see [docs/WINDOWS_SETUP.md](./docs/WINDOWS_SETUP.md).

### Test

```bash
go test ./... -count=1
```

### Start Serve Mode

```bash
cp prism.yaml.example prism.yaml
./prism serve --config prism.yaml
```

Default serve-mode URLs:

- Health: `http://localhost:8321/health`
- API status: `http://localhost:8322/api/v1/status`
- SSE events: `http://localhost:8322/api/v1/events/stream`

`prism serve` starts an embedded NATS server when `prism.nats_url` is empty.

### Start Remembrance

Remembrance is a separate Python service. From the repo's `remembrance` directory:

```bash
python -m venv .venv
source .venv/bin/activate
python -m pip install --upgrade pip
pip install -e .
uvicorn remembrance.app:app --host 127.0.0.1 --port 18790
```

Enable it in `prism.yaml`:

```yaml
remembrance:
  enabled: true
  url: "http://localhost:18790"
  timeout_seconds: 60
```

### One-Shot CLI Mode

`prism run` is a one-shot lifecycle command. It expects a NATS bus at `nats://localhost:4222`; use `prism serve` for embedded NATS or start `cmd/prism-bus` separately.

```bash
./prism run --task "Explain event-driven architecture" --provider ollama --model llama3.2
./prism run --task "Build prompt and artifacts only" --dry-run-prompt
./prism run --task "Use Remembrance context" --memory-enabled --memory-url http://localhost:18790
```

### Interactive Chat

```bash
./prism chat --config prism.yaml
./prism chat --config prism.yaml --agent astraea
```

`chat` uses the same config, workspace context, tool registry, state tools, plan tools, and provider setup as serve mode, but runs in the terminal.

---

## Use Cases

### Persistent AI Assistant with Discord

Run Prism as a daemon connected to Discord. Agents maintain sessions, use channel-aware context, call tools through policy, and capture memory through Remembrance.

```yaml
agents:
  - id: astraea
    role: orchestrator
    provider: ollama
    model: "llama3.2"
    primary: true
    context: [soul, agents, user]
    capabilities: [plan, route, review, validate, report]

channels:
  - type: discord
    token: "<bot-token>"
    channels: ["general", "dev"]
```

### Multi-Agent Delegation

Agents can delegate work through task events. Capabilities decide which agents can receive which task types, and approvals can gate risky operations.

```yaml
agents:
  - id: lumi
    role: lead
    capabilities: [plan, delegate, review, approve]
  - id: forge
    role: coder
    capabilities: [code, test, report]
```

### Plan-Aware Tool Execution

Serve and chat modes register read, project, git, state, and plan tools. Read-only tools can run directly inside allowed paths; mutation tools such as `git_add`, `git_commit`, and `git_push` are policy-gated.

### Cross-Prism and Factory Handoff

The bridge verifies signed cross-Prism messages over shared NATS, stores generic delegated tasks, and can route selected target profiles into adapters such as Roblox Factory. Discord can issue `/prism delegate`, `/prism status`, and `/prism stop` commands, but autonomous Prism-to-Prism communication stays on NATS so Discord bot greeting loops are avoided. See [docs/CROSS-PRISM-FACTORY-SETUP.md](./docs/CROSS-PRISM-FACTORY-SETUP.md).

### Codex Subscription Worker

Prism can delegate selected tasks to the local OpenAI Codex CLI through a `codex` worker. This uses the user's existing `codex login` session, so Prism does not handle ChatGPT OAuth tokens and does not route this path through `OPENAI_API_KEY`.

```yaml
codex:
  enabled: true
  sandbox: "workspace-write"
  approval_policy: "on-request"
  timeout_minutes: 30
```

After enabling it, local agents can emit `[DELEGATE: codex | code] ...`, and cross-Prism commands can target `/prism delegate target:codex task:...`.

---

## CLI Reference

### Daemon and Chat

```bash
prism serve [--config prism.yaml] [--port 8321]
prism chat [--config prism.yaml] [--agent <id>]
prism status [--config prism.yaml]
prism dashboard [--port 8080] [--run-dir ./runs] [--policy-dir policies]
```

### One-Shot Runs

```bash
prism run --task "..." [--provider mock|ollama|openai|anthropic|gemini]
prism run --task "..." --model llama3.2 --ollama-url http://localhost:11434
prism run --task "..." --memory-enabled --memory-url http://localhost:18790
prism run --task "..." --dry-run-prompt
```

### Management

```bash
prism health [--bus-url nats://localhost:4222]
prism agent list
prism agent show <id>
prism approval list [--run <run_id>]
prism approval approve <id> --by <name> --run <run_id> [--validate]
prism approval deny <id> --by <name> --run <run_id>
prism tool list
prism tool run <name> --input '{"key":"value"}' --workspace .
prism validation list
prism validation run <profile>
prism policy list
prism policy evaluate --input request.json
prism adapter list
prism adapter show <name>
prism adapter health <name>
prism projection list
prism projection rebuild --run <id>
prism projection query <name> --run <id>
prism workflow list
prism workflow show <name>
prism workflow run <name> --input input.json
prism workflow status <run_id>
prism context show --context soul,agents --workspace-root .
prism cost <run_id>
prism trace <run_id>
prism search --query "text" [--top-k 10] [--provider mock|openai|ollama]
```

---

## Configuration

`prism.yaml.example` is the current reference. Important fields:

```yaml
prism:
  instance_id: "prism"
  nats_url: ""                       # empty = embedded NATS in serve mode
  data_dir: ".prism/data"
  workspace: "D:/_projects_/prism"
  ollama_url: "http://localhost:11434"
  context_token_budget: 4000
  llm_timeout_seconds: 1200
  port: 8321
  log_level: "info"
  allowed_paths: []
  scheduler:
    enabled: false
    jobs: []

bridge:
  enabled: false
  mode: "shared_nats"
  secret_env: "PRISM_BRIDGE_SECRET"
  allowed_subjects:
    - "prism.cross.context_sync"
    - "prism.cross.task_request"
    - "prism.cross.status_request"
    - "prism.cross.validation_request"
    - "prism.cross.task_response"
  factory:
    enabled: false
    root: "D:/_projects_/roblox-factory"
    project: "eggventura"
    project_path: "D:/Projects/Roblox/eggventura"
    approval_mode: "report_only"
    run_codex: false
    vision_review: "none"
    playtest_mode: "none"
    enable_ui_generation: false
    ui_generation_dry_run: true

agents:
  - id: astraea
    role: orchestrator
    provider: ollama                  # ollama, openai, openai_responses, anthropic, gemini
    model: "llama3.2"
    primary: true
    context: [soul, agents, user]
    conversation_postfix: ""
    capabilities: [plan, route, review, validate, report]
    listen_to_agents: []
    subscriptions: []
    state_actions:
      manager-room:
        inject: "Prefer explicit status and concrete decisions."

channels:
  - type: discord
    token: "<bot-token>"
    channels: []

channel_roles:
  - id: "general"
    role: "manager-room"
    tools: "read-only"                # all, read-only, none
    personality: "direct"
    context: "Coordination room for status and task triage."

actions:
  - trigger: "*.agent.output"
    action: "remembrance.gate.extract"
    enabled: true

sessions:
  max_context_messages: 100
  idle_timeout_minutes: 30
  compaction_strategy: "truncate"
  daily_reset_hour: 4

remembrance:
  enabled: false
  url: "http://localhost:18790"
  timeout_seconds: 60
```

Windows notes:

- The current config loader does not expand `~` in YAML paths; use repo-relative or absolute paths.
- The current config loader does not expand `${ENV_VAR}` inside YAML. Put real local values in `prism.yaml` or remove optional channels while testing.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | Required by `openai` and `openai_responses` providers |
| `ANTHROPIC_API_KEY` | Required by the Anthropic provider |
| `GEMINI_API_KEY` | Required by the Gemini provider |
| `OPENAI_EMBEDDING_MODEL` | Optional model override for `prism search --provider openai` |
| `OLLAMA_BASE_URL` | Optional embedding base URL for `prism search --provider ollama` |
| `OLLAMA_EMBEDDING_MODEL` | Optional Ollama embedding model override |
| `PRISM_BRIDGE_SECRET` | Shared HMAC secret for cross-Prism bridge when enabled |

---

## API Surface

Serve mode exposes the live API on `port + 1`, so the default is `8322`.

Current routes include:

- `GET /api/v1/status`
- `GET /api/v1/agents`, `GET /api/v1/agents/{id}`
- `GET /api/v1/sessions`, `GET /api/v1/sessions/{id}`
- `GET /api/v1/tasks`, `GET /api/v1/tasks/{id}`
- `GET /api/v1/approvals`
- `POST /api/v1/approvals/{id}/grant`, `POST /api/v1/approvals/{id}/deny`
- `GET /api/v1/events/stream`
- `GET /api/v1/workflows`, `GET /api/v1/workflows/{type}`
- `GET/PUT /api/v1/editor`
- `GET/POST /api/v1/editor/nodes`, `PUT/DELETE /api/v1/editor/nodes/{id}`
- `GET/POST /api/v1/editor/edges`, `DELETE /api/v1/editor/edges/{id}`
- `POST /api/v1/editor/save`
- `GET /api/v1/costs`

---

## Key Design Decisions

- Events are the source of truth; meaningful actions become canonical events.
- Agent IDs define namespaces, for example `astraea.agent.output`.
- Policies and guard checks are deterministic; the model does not decide its own write permissions.
- SQLite and embedded NATS keep local development self-contained.
- Remembrance is a separate HTTP service, not embedded.
- ChatProvider-native tool calling is preferred where supported; text-based tool requests remain as fallback.
- Working state and plans are explicit files managed by state/plan tools.
- Cross-Prism and Factory integrations are optional adapters around the core runtime.

---

## Version History

| Version | What | Design Doc |
|---------|------|------------|
| V1 | Foundation: CLI, event bus, canonical events | [V1](./docs/V1-FOUNDATION-DESIGN.md) |
| V2 | Real LLM execution and provider interface | [V2](./docs/V2-REAL-LLM-EXECUTION-DESIGN.md) |
| V3 | Controlled tool execution | [V3](./docs/V3-CONTROLLED-TOOL-EXECUTION-DESIGN.md) |
| V4 | Approval-gated mutations | [V4](./docs/V4-APPROVAL-GATED-MUTATIONS-DESIGN.md) |
| V5 | Validation and deterministic review | [V5](./docs/V5-VALIDATION-REVIEW-DESIGN.md) |
| V6 | Gate/trading work moved out of core Prism | [V6](./docs/V6-GATE-TRADING-MOVED.md) |
| V7 | Workflow runtime | [V7](./docs/V7-WORKFLOW-RUNTIME-DESIGN.md) |
| V8 | Policy engine | [V8](./docs/V8-POLICY-ENGINE-DESIGN.md) |
| V9 | Adapter contract and SDK | [V9](./docs/V9-ADAPTER-CONTRACT-DESIGN.md) |
| V10 | State projections | [V10](./docs/V10-STATE-PROJECTIONS-DESIGN.md) |
| V11 | Dashboard | [V11](./docs/V11-DASHBOARD-DESIGN.md) |
| V12 | CLI and architecture refactor | [V12](./docs/V12-ARCHITECTURAL-REFACTOR-DESIGN.md) |
| V13 | Multi-agent orchestration | [V13](./docs/V13-MULTI-AGENT-DESIGN.md) |
| V14a-e | Pipeline, streaming, providers, SQLite, Discord coverage | [V14a](./docs/V14a-DECOMPOSE-STREAM-DESIGN.md) |
| V15 | Vector search | [V15](./docs/V15-VECTOR-SEARCH-DESIGN.md) |
| V16 | Intelligence arc | [V16](./docs/V16-INTELLIGENCE-ARC-DESIGN.md) |
| V17 | Performance | [V17](./docs/V17-PERFORMANCE-DESIGN.md) |
| V18 | OpenClaw config transfer | [V18](./docs/V18-OPENCLAW-CONFIG-DESIGN.md) |
| V19 | Smart context injection | [V19](./docs/V19-SMART-CONTEXT-DESIGN.md) |
| V20 | Live orchestrator | [V20](./docs/V20-LIVE-ORCHESTRATOR-DESIGN.md) |
| V21 | Full conversation pipeline | [V21](./docs/V21-FULL-CONVERSATION-DESIGN.md) |
| V22 | Multi-agent delegation | [V22](./docs/V22-MULTI-AGENT-ORCHESTRATION-DESIGN.md) |
| V23 | Platform API, bridge, dashboard, SDK | [V23](./docs/V23-PLATFORM-DESIGN.md) |
| V24 | Visual workflow diagrams | [V24](./docs/V24-VISUAL-WORKFLOW-DESIGN.md) |
| V25 | Visual workflow editor | [V25](./docs/V25-VISUAL-WORKFLOW-EDITOR-DESIGN.md) |
| V26 | Remembrance integration | [V26](./docs/V26-REMEMBRANCE-INTEGRATION-DESIGN.md) |
| V27 | Serve-mode tool executor | [V27](./docs/V27-SERVE-TOOL-EXECUTOR-DESIGN.md) |
| V28 | Project and git tools | [V28](./docs/V28-PROJECT-GIT-TOOLS-DESIGN.md) |
| V29 | Tool guidance and session awareness | [V29](./docs/V29-TOOL-GUIDANCE-SESSION-AWARENESS-DESIGN.md) |
| V30 | Native Ollama tool calling | [V30](./docs/V30-NATIVE-TOOL-CALLING-DESIGN.md) |
| V31 | Native chat streaming gap note | [V31](./docs/V31-CHAT-STREAMING-GAP.md) |
| V32 | Operating environment: state, plans, guard, wake | [V32](./docs/V32-LUMI-OPERATING-ENVIRONMENT.md) |
| V33 | Conversation awareness and channel context | [V33](./docs/V33-CONVERSATION-AWARENESS.md) |
| V34 | OpenAI Responses provider | [V34](./docs/V34-OPENAI-RESPONSES-DESIGN.md) |

---

## Dependencies

Direct runtime dependencies are intentionally small: pure-Go SQLite, NATS server/client, YAML, locking, ULID/XID, Discord, SVG, and WebSocket support. There is no external database requirement for local development.

---

## Testing

```bash
go test ./... -count=1
go test ./internal/stage/... -race
go test ./internal/vector/... -bench=.
```

---

## Project Status

Prism is source-available and active. The stable core includes serve mode, Discord integration, sessions, multi-agent orchestration, Remembrance, tool execution, API/dashboard/editor, state/plan tooling, scheduler hooks, cross-Prism bridge, and provider support including OpenAI Responses.

Current focus is keeping runtime behavior, docs, and configuration aligned while hardening the state/plan/guard pipeline, event-driven wake, and cross-Prism/Factory handoff.

See [docs/ROADMAP.md](./docs/ROADMAP.md) and [docs/TASKS.md](./docs/TASKS.md).

---

## License

All rights reserved. See [LICENSE](./LICENSE) for details.
