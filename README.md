# Prism - Event-Native AI Agent Platform

[![Go 1.26+](https://img.shields.io/badge/go-1.26%2B-blue)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-go%20test%20.%2F...-brightgreen)](./.github/workflows/ci.yml)
[![Packages: 57](https://img.shields.io/badge/packages-57-green)](./internal/)
[![License: All Rights Reserved](https://img.shields.io/badge/license-all%20rights%20reserved-red)](./LICENSE)

> **License Notice:** Prism is source-available under an all-rights-reserved license. You may view the repository, but use, modification, distribution, or incorporation requires written permission. See [LICENSE](./LICENSE) for details.

Prism is a Go event-native AI agent platform that runs as a persistent service. Agents communicate through a NATS event bus, maintain conversation sessions, remember context through Remembrance, use tools under policy, and expose a local API/dashboard. Autonomous sub-agents can run bounded tool loops with worktree isolation, capability-aware routing, and per-run budgets.

The framework controls lifecycle, safety, context, routing, and persistence. The model generates outputs inside that lifecycle.

Local root binaries can lag behind source and are ignored by Git. For a fresh
setup, build from `cmd/prism-cli` and treat the source tree as authoritative.

---

## Status

Prism is in **public-preview** development. It is source-available, local-first,
and **not production-ready**.

**Stable enough to explore:**

- local CLI workflows and the event spine
- workflow runtime and run artifacts
- policy checks and tool execution
- approvals / validation / review
- event logging and local run inspection

**Experimental (opt-in, off by default):**

- multi-agent delegation and sub-agent workers
- MCP client integration
- self-patching / autopatch
- cross-Prism bridge and Factory handoff
- external provider integrations (OpenAI, Anthropic, Gemini, Claude Code, Codex)
- Remembrance memory integration
- scheduler / dashboard-editor features

**Not production-ready / out of scope:**

- high-risk unattended automation
- live trading or financial execution
- unattended code mutation without human review
- enterprise multi-user deployment

See the normative [Stability Matrix](./docs/reference/stability-matrix.md) for
default state, limitations, testing level, and compatibility expectations.

---

## Golden Path Demo

The fastest way to understand Prism is to run a workflow and inspect the
generated events and artifacts. The built-in echo workflow needs no model and
runs fully locally:

```bash
go test ./...                                   # verify the build
go run ./cmd/prism-cli doctor --json            # validate the local runtime
go run ./cmd/prism-cli workflow list            # see available workflows
go run ./cmd/prism-cli workflow show demo.echo_tool
go run ./cmd/prism-cli workflow run demo.echo_tool
go run ./cmd/prism-cli workflow status <run_id>
```

What to expect:

- workflow and tool events are emitted for the run
- artifacts are written under `runs/<run_id>/`
- `events.jsonl` shows the full event trail
- `summary.json` shows the run result

A sanitized sample run is checked in at
[`examples/runs/sample-run/`](./examples/runs/sample-run). For a step-by-step
walkthrough see [Getting Started](./docs/GETTING_STARTED.md) and
[Examples](./docs/EXAMPLES.md).

---

## Architecture

See the [system overview and trust boundaries](./docs/architecture/system-overview.md)
and [Core / Platform / Integration map](./docs/architecture/product-layers.md).

```text
Prism Runtime
  - Config + orchestrator
  - Embedded or external NATS JetStream
  - SQLite-backed sessions, tasks, approvals, events, and run artifacts
  - Agent router, tool executor, policy/guard checks, plan/state managers
  - Sub-agent worker: bounded tool-loop delegation with worktree isolation (V58)
  - Remembrance HTTP client and cache
  - HTTP API, SSE event stream, dashboard, visual workflow editor
  - Optional cross-Prism bridge and Roblox Factory handoff
  - Idle guard + schedule optimization (zero tokens when nothing to do)

Ingress/Egress
  - Discord bot (interactive approval buttons, channel roles)
  - CLI run/chat commands
  - REST API clients
  - NATS subjects and scheduled wake events
```

### Core Packages

| Package | Purpose |
|---------|---------|
| `bus/`, `event/`, `run/`, `runtrack/` | Embedded NATS, canonical events, run lifecycle, WAL-style artifacts, run tracking |
| `provider/` | Mock, Ollama, OpenAI chat completions, OpenAI Responses, Anthropic, Gemini, Claude Code CLI, Codex CLI |
| `agent/`, `agentns/`, `router/`, `orchestrator/` | Agent registry, agent namespaces, routing, config loading, live service lifecycle |
| `session/`, `task/`, `approval/`, `delegation/`, `subagent/` | Conversation state, task tracking, approvals, multi-agent delegation, autonomous sub-agent worker |
| `tool/`, `policy/`, `safety/`, `mutation/`, `guard/` | Tool registry, policy gates, path safety, mutations, plan-aware guard checks |
| `state/`, `plan/`, `prompt/`, `context/` | Working state, task plans, prompt layering, workspace context injection |
| `stage/`, `workstart/` | Gated-loop phase stages, work-start lifecycle |
| `skill/` | SKILL.md skill-use capabilities (Claude Code / OpenClaw) |
| `remembrance/` | Go HTTP client and cache for the separate Remembrance memory service |
| `api/`, `dashboard/`, `editor/`, `workflow/` | REST/SSE API, dashboard, visual editor, SVG workflow diagrams |
| `bridge/`, `crossprism/`, `factory/`, `factorymonitor/` | Cross-Prism protocol, Factory handoff, Factory queue monitoring |
| `autopatch/`, `improve/` | Self-patching PR mode, issue scanner, improvement proposals |
| `claudecli/`, `claudeworker/`, `codexworker/` | Claude Code CLI resolution, Claude reviewer worker, Codex subscription worker |
| `gitx/` | Shared git worktree helpers (used by sub-agents, autopatch, gated loop) |
| `codesummary/` | Codebase summary generation |
| `vector/`, `sse/`, `cost/`, `projection/`, `validation/`, `review/` | Search, streaming helpers, cost tracking, CQRS projections, checks, review artifacts |
| `action/`, `debounce/`, `retry/`, `checksum/`, `scheduler/` | Event actions, debounce, retry logic, checksums, cron scheduler |
| `config/`, `sqlite/`, `integration/` | Configuration loading, SQLite persistence, integration tests |

---

## Documentation

New to Prism? Start at the [documentation home](./docs/README.md), then use:

1. [Getting Started](docs/GETTING_STARTED.md) — install, build, test, and run your first workflow.
2. [Configuration Guide](docs/CONFIGURATION.md) — where config files live and how Prism loads them.
3. [YAML Reference](docs/YAML_REFERENCE.md) — workflow, policy, adapter, provider, and agent YAML.
4. [Command Reference](docs/COMMANDS.md) — all major CLI commands and what they do.
5. [Examples](docs/EXAMPLES.md) — guided demo flows.
6. [Troubleshooting](docs/TROUBLESHOOTING.md) — common setup and runtime issues.
7. [Architecture](docs/ARCHITECTURE.md) — how Prism works internally.
8. [Capability Status](docs/CAPABILITY_STATUS.md) — stable vs experimental features.
9. [Safety Model](docs/SAFETY.md) — human-in-the-loop, policy vs validators, autopatch risks.
10. [Roadmap](docs/ROADMAP.md) — project direction.
11. [Version History](docs/VERSION_HISTORY.md) — the full V1–V58+ development story.
12. [Public Preview Checklist](docs/PUBLIC_PREVIEW_CHECKLIST.md) — release-prep status.
13. [Quality and Verification](QUALITY.md) — reproducible metrics and checks.

---

## Requirements

- **Go 1.26+** (module requires 1.26.2)
- **Git** (for project tools, worktree isolation, autopatch)
- **Ollama** — if using local Ollama models
- **Python 3.11+** — only if running the Remembrance memory service
- **Claude CLI** — only if using the `claude_code` provider or Claude reviewer (subscription-based, no API key)
- **Codex CLI** — only if using the Codex worker (`codex login` required)
- **`gh` CLI** — only if using autopatch in `"pr"` mode (GitHub pull requests)

No external database or message broker is required for basic operation — Prism embeds NATS JetStream and uses SQLite.

---

## Quick Start

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
go test ./... -count=1 -race
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
- **Dashboard: `http://localhost:8322/`** — served by `prism serve` itself (no separate process, no CORS setup)

The dashboard pages (all same-origin on the API port):

| Page | Purpose |
|---|---|
| `/config.html` | **Settings** editor — instance/paths, feature toggles, autopatch, remembrance, workflow run-behavior |
| `/scheduler.html` | **Cron Jobs** editor — add/edit jobs with live cron validation + action presets |
| `/index.html` | Runs browser · `/v2.html` Status · `/editor.html` Agent graph · `/workflow-editor.html` Workflow |

Settings and cron edits are written back to `prism.yaml` surgically (comments and untouched sections preserved) and **apply on the next `prism serve` restart**. `prism serve` starts an embedded NATS server when `prism.nats_url` is empty.

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

### Autonomous Sub-Agent Worker (V58)

Sub-agents run independently with bounded tool loops, per-agent tool scoping (capability-based gating), worktree-per-subagent isolation, and deadline enforcement. Enable with `PRISM_SUBAGENT_WORKER=1`. Code-capable agents get isolated git worktrees; read-only agents skip isolation. Failed or timed-out sub-agents always emit a `status:"failed"` completion — they never silently hold a gate.

### Plan-Aware Tool Execution

Serve and chat modes register read, project, git, state, and plan tools. Read-only tools can run directly inside allowed paths; mutation tools such as `git_add`, `git_commit`, and `git_push` are policy-gated.

### Verified Gated Loop

The gated dev loop (`PROBE → RESEARCH → PLAN → FEEDBACK_PRE → EXECUTION → FEEDBACK_POST → REPORT`) enforces objective build/test verification in EXECUTION: after the model commits, Prism runs an allowlisted V5 validation profile (e.g. `go_test_all` → `go test ./...`). With `blocking: true` the phase cannot complete until it passes — the failing output is fed back so the model fixes the real problem and re-commits. The model can also call the `run_validation` tool to self-check before committing. The loop is bounded by run budgets (`max_total_time`, `max_total_tokens`; `-1` explicitly means unlimited, `0` uses the default token ceiling) and stuck-loop detection (`max_repeated_tool_calls`), each emitting an event and stopping gracefully. Configure it per phase under `verification` (see [docs/V35-VERIFICATION-GATE-DESIGN.md](./docs/V35-VERIFICATION-GATE-DESIGN.md)).

### Scout Sub-Agent

A lightweight local model (e.g. qwen3:8b, gemma3:4b) gathers codebase context before the cloud model runs, reducing token cost. The scout can also collect reference images via the `collect_reference_images` tool (with Firecrawl support for real image downloads).

### Cross-Prism and Factory Handoff

The bridge verifies signed cross-Prism messages over shared NATS, stores generic delegated tasks, and can route selected target profiles into adapters such as Roblox Factory. Discord can issue `/prism delegate`, `/prism status`, and `/prism stop` commands, but autonomous Prism-to-Prism communication stays on NATS so Discord bot greeting loops are avoided. See [docs/CROSS-PRISM-FACTORY-SETUP.md](./docs/CROSS-PRISM-FACTORY-SETUP.md).

### Roblox Game-Dev Team

A multi-agent studio (orchestrator, researcher, game planner, Factory master, asset maker) that designs and builds Roblox games — with native reference-image tools, a Blender-MCP asset pipeline, and a cross-Prism rubric handshake. See [docs/ROBLOX-TEAM.md](./docs/ROBLOX-TEAM.md).

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

### Claude Code Provider

Use the Claude CLI as an orchestrator brain for the gated loop (no API key — uses your subscription). Define an agent with `provider: claude_code` and point a project at it with `project.orchestrator`. The provider shells out to `claude -p` with tools disabled, so Prism still owns tool execution, gates, and policy. Claude Code can also serve as a reviewer at feedback gates.

```yaml
claude_code:
  enabled: true
  reviewer_name: "claude"
  timeout_minutes: 10
```

### Auto-Patching

Autopatch turns explicit bug reports or validation failures into reviewable patch proposals. It creates an isolated git worktree, tries configured patch workers in order, runs allowlisted validation profiles, and stores artifacts under `.prism/data/autopatch/<task-id>/`. Two modes: `"propose"` (default — patch artifact only, never touches the main worktree) and `"pr"` (V50 — pushes a branch and opens a GitHub pull request via the `gh` CLI; `prism doctor` preflights `gh` auth).

```yaml
autopatch:
  enabled: true
  mode: "propose"            # or "pr" to open pull requests
  require_clean_worktree: true
  max_attempts: 2
  validation_profiles: ["go_test_all"]
  worker_order: ["codex", "local_agent"]
  local_agent: "forge"
  worktree_root: ".prism/worktrees"
  base_branch: ""            # PR base in "pr" mode; empty = repo default
```

Start a task with `POST /api/v1/autopatch` using `{"description":"tests are failing, fix this bug"}`. In Discord, explicit requests such as "auto patch this bug" or "tests are failing, fix this bug" start the same tracked `auto_patch` task.

### Skill-Use Capabilities (V54)

Agents can consume SKILL.md files (Claude Code / OpenClaw format) via the `use_skill` tool, enabling structured skill injection into prompts.

### MCP Client (V49)

Consume external MCP (Model Context Protocol) tool servers. Tools register as `mcp_<name>_<tool>` and run through the same policy engine as built-in tools. Probe a server before enabling:

```bash
prism mcp probe --command npx --args "-y,@modelcontextprotocol/server-filesystem,D:/_projects_"
```

```yaml
mcp_servers:
  - name: "filesystem"
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-filesystem", "D:/_projects_"]
    enabled: true
mcp_auto_approve: false
```

---

## CLI Reference

### Daemon and Chat

```bash
prism serve [--config prism.yaml] [--port 8321]
prism chat [--config prism.yaml] [--agent <id>]
prism status [--config prism.yaml]
prism dashboard [--port 8080] [--run-dir ./runs] [--policy-dir policies]  # optional; serve already hosts the UI on the API port
```

### One-Shot Runs

```bash
prism run --task "..." [--provider mock|ollama|openai|anthropic|gemini]
prism run --task "..." --model llama3.2 --ollama-url http://localhost:11434
prism run --task "..." --memory-enabled --memory-url http://localhost:18790
prism run --task "..." --dry-run-prompt
```

### Workflows and Projects

```bash
prism workflow list
prism workflow show <name>
prism workflow run <name> --input input.json
prism workflow start --project <id> --prompt "..."
prism workflow status <run_id>
prism preview [--config prism.yaml]          # static gated-loop preview
prism watch [--config prism.yaml]            # live SSE run visibility
prism runs [--json]                          # browse past runs & reports
prism runs latest                            # latest run shortcut
```

### Management

```bash
prism health [--bus-url nats://localhost:4222]
prism doctor [--json]                        # preflight health check
prism config validate                        # validate prism.yaml
prism config summarize                       # summarize config
prism config wizard                          # interactive config setup
prism config import                          # OpenClaw→prism.yaml import
prism agent list
prism agent show <id>
prism approval list [--run <run_id>]
prism approval approve <id> --by <name> --run <run_id> [--validate]
prism approval deny <id> --by <name> --run <run_id>
prism tool list
prism tool run <name> --input '{"key":"value"}' --workspace .
prism skills list                            # list available SKILL.md files
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
prism context show --context soul,agents --workspace-root .
prism cost <run_id>
prism trace <run_id>
prism search --query "text" [--top-k 10] [--provider mock|openai|ollama]
prism scan [--start]                         # issue-discovery scanner
prism mcp probe --command <cmd> --args <args> # probe MCP server
prism remembrance status                     # check Remembrance connection
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
  bind_host: "127.0.0.1"            # "0.0.0.0" to expose on network (requires api.auth_token)
  read_roots:
    - "D:/"
    - "C:/Users/emaha"
  write_roots:
    - "D:/_projects_"
    - "D:/Projects"
  allowed_paths: []                  # legacy alias if split roots are omitted
  scheduler:                         # built-in cron; see docs/SCHEDULER.md
    enabled: false
    jobs: []

api:
  auth_token: ""                     # bearer token for state-changing endpoints
  auth_token_env: ""                 # e.g. "PRISM_API_TOKEN" (takes priority)
  allowed_origins: []                # CORS origin allowlist

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

projects:
  - id: my-project
    repo_path: "/path/to/your/repo"
    state_file: "PROJECT_STATE.md"
    default_branch: "main"
    channel: ""
    workflow_config: ""
    orchestrator: ""                 # agent ID (e.g. "claude_code"); empty = primary agent
    worktree_isolation: false        # V56: per-run git worktree
    default: true

agents:
  - id: astraea
    role: orchestrator
    provider: ollama                  # ollama, openai, openai_responses, anthropic, gemini, claude_code
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
  persistence: true
  resume_after_idle: true
  compaction_strategy: "summarize"
  keep_archived_messages: true
  daily_reset_hour: 4
  continuity_scope: "owner_agent"    # or "channel_user" for per-channel
  recall_window_mode: "calendar_week"
  recall_timezone: "Local"
  short_term_window_days: 7
  verbatim_recent_messages: 40

remembrance:
  enabled: false
  url: "http://localhost:18790"
  timeout_seconds: 60

autopatch:
  enabled: false
  mode: "propose"
  require_clean_worktree: true
  max_attempts: 2
  validation_profiles: ["go_test_all"]
  worker_order: ["codex", "local_agent"]
  local_agent: "forge"

codex:
  enabled: false
  sandbox: "workspace-write"
  approval_policy: "on-request"
  timeout_minutes: 30

claude_code:
  enabled: false
  reviewer_name: "claude"
  timeout_minutes: 10
  allowed_tools: ""                  # empty = read-only default

mcp_servers: []
mcp_auto_approve: false

factory_monitor:
  enabled: false
  poll_seconds: 30
  stuck_after_minutes: 30
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
| `PRISM_SUBAGENT_WORKER` | Set to `1` to enable the V58 autonomous sub-agent worker in serve mode |
| `PRISM_API_TOKEN` | Bearer token for API authentication (via `api.auth_token_env`) |
| `DISCORD_BOT_TOKEN` | Discord bot token (referenced in channel config) |

---

## Setting Up a Project Loop

Prism can autonomously work on a project in a scheduled loop — reading a state file for tasks, implementing changes, self-reviewing, and pushing branches. Here's how to set it up:

### 1. Add a project to `prism.yaml`

```yaml
projects:
  - id: my-project
    repo_path: "/path/to/your/repo"
    state_file: "PROJECT_STATE.md"        # task assignment file (relative to repo_path)
    default_branch: "main"               # protected branch (writes blocked here)
    channel: "1234567890123456789"       # Discord channel ID for feedback/reports
    workflow_config: "examples/workflows/fast-loop.yaml"  # optional per-project workflow
    worktree_isolation: false             # V56: per-run git worktree (parallel runs)
    default: true                         # used when no --project is specified
```

### 2. Create a `PROJECT_STATE.md` in the repo

```markdown
# Project State

## FEATURE PRIORITY — DO THESE IN ORDER

- [ ] Task 1: Brief description (risk: low)
- [ ] Task 2: Brief description (risk: medium)
- [x] Task 3: Completed task (strikethrough for done items)
```

Prism reads this file at the start of each cycle, picks the topmost unchecked task, and works on it. When done, she marks it `[x]`.

### 3. Configure the scheduler job

```yaml
scheduler:
  enabled: true
  jobs:
    - name: "project-work"
      schedule: "*/10 * * * *"          # every 10 minutes (cron expression)
      event: "prism.task.scheduled"
      payload:
        action: "project_work"
      enabled: true

    - name: "status-report"
      schedule: "0 */2 * * *"           # every 2 hours
      event: "prism.task.scheduled"
      payload:
        action: "status_report"
      enabled: true
```

- **project-work**: Runs the gated loop — discovers tasks, implements, reviews, pushes
- **status-report**: Reads recent run summaries + PROJECT_STATE.md, posts a report to Discord

Full cron reference (all fields, actions, examples): [docs/SCHEDULER.md](./docs/SCHEDULER.md).

### 4. Choose a workflow config

| Config | Best for | Key settings |
|---|---|---|
| `gated-loop.yaml` (default) | Human-in-the-loop | 60m budget, approval gates, full iteration caps |
| `fast-loop.yaml` | Autonomous loops | 15m budget, `auto_approve: true`, reduced iteration caps |

Point your project at either one via `workflow_config`. Or omit it to use the built-in default.

### 5. Workflow config options

```yaml
global:
  auto_approve: true          # skip FEEDBACK_PRE/FEEDBACK_POST gates (no human needed)
  auto_rollback: false         # V57: auto git revert on failed verification
  max_total_time: "15m"       # hard time budget per run
  max_total_tokens: 500000    # token ceiling (-1 = unlimited, 0 = default)
  max_repeated_tool_calls: 3  # stuck-loop detection

phases:
  - name: PROBE
    max_iterations: 8         # how many LLM calls before forcing phase advance
    gate:
      type: assumption_threshold
      threshold: 1.0
    fallback:
      on_max_iterations: proceed_with_open_assumptions  # don't block
      blocks: false
```

### 6. Start immediately (optional)

Don't want to wait for the next cron tick?

```bash
./prism workflow start --project my-project --prompt "Work on the next task in PROJECT_STATE.md"
```

Or via the API:

```bash
curl -X POST http://localhost:8322/api/v1/workflows/start \
  -H "Content-Type: application/json" \
  -d '{"project":"my-project","prompt":"Work on the next task in PROJECT_STATE.md"}'
```

### 7. Discord approval buttons

When a workflow pauses at a feedback gate, Prism posts a message with interactive buttons to the configured channel:

- **Approve** — resume the workflow
- **Request changes** — loop back to EXECUTION
- **Reject** — end the run (FEEDBACK_PRE only)

Buttons are automatically disabled after one is clicked. With `auto_approve: true`, feedback gates are skipped entirely and no buttons are sent.

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
- `POST /api/v1/workflows/start`
- `GET/PUT /api/v1/editor`
- `GET/POST /api/v1/editor/nodes`, `PUT/DELETE /api/v1/editor/nodes/{id}`
- `GET/POST /api/v1/editor/edges`, `DELETE /api/v1/editor/edges/{id}`
- `POST /api/v1/editor/save`
- `GET /api/v1/costs`
- `POST /api/v1/autopatch`

---

## Makefile Targets

| Target | Command | Description |
|--------|---------|-------------|
| `make dev` | `go run ./cmd/prism-cli run ...` | Run in development mode |
| `make test` | `go test ./... -count=1 -race` | Run all tests with race detector |
| `make test-short` | `go test ./... -count=1 -short` | Run tests without race detector (faster) |
| `make test-coverage` | Generates `coverage.html` | Tests with coverage report |
| `make lint` | `go vet ./...` | Run linters |
| `make ci` | vet → build → test | Full CI gate |
| `make build` | `CGO_ENABLED=0 go build ...` | Build binary for current platform |
| `make build-all` | Cross-compile | Linux amd64, Darwin arm64, Windows amd64 |
| `make docker-build` | `docker build -t prism:latest .` | Build Docker image |
| `make docker-run` | `docker-compose up -d` | Run in Docker |
| `make docker-stop` | `docker-compose down` | Stop Docker containers |
| `make clean` | Remove build artifacts | Clean up binaries and coverage files |

---

## Docker

```bash
docker build -t prism:latest .
docker-compose up -d
```

> **Note:** The Dockerfile currently uses `golang:1.24-alpine` as the build image. <!-- TODO: update to golang:1.26-alpine when available -->

The Docker setup exposes port 8080 and mounts `./runs` and `./policies` as volumes.

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
- Sub-agents fail closed: unknown agent, runner error, or timeout always yields a failed completion — never silently holds a gate.
- Idle guard prevents token waste when no work is scheduled.

---

## Project Structure

```text
prism/
├── cmd/
│   ├── prism-cli/          # Main CLI binary (serve, run, chat, doctor, config, workflow, etc.)
│   ├── prism-bus/          # Standalone NATS bus binary
│   └── prism-agent/        # Standalone agent binary
├── internal/               # All core packages (~57 domains)
│   ├── action/             # Event-triggered actions
│   ├── adapter/            # Adapter contract
│   ├── agent/              # Agent registry and lifecycle
│   ├── agentns/            # Agent namespace helpers
│   ├── api/                # REST/SSE HTTP API
│   ├── approval/           # Approval gates
│   ├── autopatch/          # Self-patching (propose + PR modes)
│   ├── bridge/             # Cross-Prism bridge protocol
│   ├── bus/                # Embedded NATS JetStream
│   ├── checksum/           # Deterministic checksums
│   ├── claudecli/          # Claude Code CLI executable resolution
│   ├── claudeworker/       # Claude reviewer worker
│   ├── codesummary/        # Codebase summary generation
│   ├── codexworker/        # Codex CLI subscription worker
│   ├── config/             # Configuration loading
│   ├── context/            # Workspace context injection
│   ├── cost/               # Token/cost tracking
│   ├── crossprism/         # Cross-Prism messaging
│   ├── dashboard/          # Dashboard UI
│   ├── debounce/           # Debounce helpers
│   ├── delegation/         # Multi-agent delegation
│   ├── editor/             # Visual workflow editor
│   ├── event/              # Canonical event types
│   ├── factory/            # Roblox Factory adapter
│   ├── factorymonitor/     # Factory queue monitoring
│   ├── gitx/               # Shared git worktree helpers
│   ├── guard/              # Plan-aware guard checks
│   ├── improve/            # Improvement proposals
│   ├── integration/        # Integration tests
│   ├── mutation/           # Mutation tracking
│   ├── orchestrator/       # Live orchestrator lifecycle
│   ├── plan/               # Task plan management
│   ├── policy/             # Policy engine
│   ├── projection/         # CQRS state projections
│   ├── prompt/             # Prompt layering
│   ├── provider/           # LLM providers (Mock, Ollama, OpenAI, Anthropic, Gemini, Claude Code, Codex)
│   ├── remembrance/        # Remembrance HTTP client + cache
│   ├── retry/              # Retry logic
│   ├── review/             # Review artifacts
│   ├── router/             # Agent routing
│   ├── run/                # Run lifecycle + WAL artifacts
│   ├── runtrack/           # Run tracking
│   ├── safety/             # Path safety checks
│   ├── scheduler/          # Built-in cron scheduler
│   ├── session/            # Conversation sessions
│   ├── skill/              # SKILL.md parsing and injection
│   ├── sqlite/             # SQLite persistence
│   ├── sse/                # SSE streaming helpers
│   ├── stage/              # Gated-loop phase stages
│   ├── state/              # Working state management
│   ├── subagent/           # Autonomous sub-agent worker (V58)
│   ├── task/               # Task tracking
│   ├── tool/               # Tool registry and execution
│   ├── validation/         # Validation profiles
│   ├── vector/             # Vector search
│   ├── workflow/           # Workflow runtime
│   └── workstart/          # Work-start lifecycle
├── adapters/               # External adapter implementations
│   ├── echo/               # Echo adapter (example)
│   └── remembrance/        # Remembrance adapter
├── sdk/                    # SDK for external consumers
│   ├── examples/
│   └── prism/
├── remembrance/            # Separate Python memory service
│   ├── configs/
│   ├── src/
│   └── tests/
├── policies/               # Policy definition files
├── examples/               # Example configs and workflows
│   ├── runs/
│   └── workflows/
├── docs/                   # Design documents (V1–V58)
├── scripts/                # Utility scripts
├── Makefile                # Build, test, lint, CI, Docker targets
├── Dockerfile              # Multi-stage Docker build
├── docker-compose.yaml     # Docker Compose setup
├── go.mod / go.sum         # Go module (github.com/emaharmony/prism)
├── prism.yaml.example      # Reference configuration
└── LICENSE                 # All rights reserved
```

---

## Testing

```bash
go test ./... -count=1           # all tests
go test ./... -count=1 -race     # all tests with race detector
go test ./internal/stage/... -race
go test ./internal/vector/... -bench=.
go test ./internal/checksum/ -v -count=1  # single package
```

Tests use the external test package pattern (`package foo_test`), stdlib `testing` only (no third-party frameworks), and assertions via `t.Errorf` / `t.Fatalf`.

---

## Dependencies

Direct runtime dependencies (from `go.mod`):

| Dependency | Purpose |
|------------|---------|
| `modernc.org/sqlite` | Pure-Go SQLite (no CGO required) |
| `nats-io/nats-server/v2`, `nats-io/nats.go` | Embedded NATS JetStream server and client |
| `gopkg.in/yaml.v3` | YAML configuration parsing |
| `bwmarrin/discordgo` | Discord bot integration |
| `ajstarks/svgo` | SVG workflow diagram generation |
| `gofrs/flock` | File locking |
| `oklog/ulid/v2`, `rs/xid` | ULID and XID generation |

No external database requirement for local development.

---

## Version History

Prism grew through many incremental versions (V1–V58+). The full development
story — with links to each design document — lives in
[docs/VERSION_HISTORY.md](./docs/VERSION_HISTORY.md).

At a glance:

- **Foundations (V1–V13):** event spine, LLM execution, tool execution,
  approvals, validation, workflow runtime, policy engine, adapters,
  projections, dashboard, multi-agent orchestration.
- **Platform expansion (V14–V34):** vector search, providers, persistent serve
  mode, Discord, sessions, Remembrance, git/project tools.
- **Gated loop & observability (V35–V48):** verification gates, `prism watch`,
  `prism doctor`, run artifacts, approval cards, `prism runs`.
- **Advanced / experimental (V49–V58):** MCP client, self-patching autopatch,
  skills, config wizard, worktree isolation, auto-rollback, sub-agent worker.

---

## Project Status

Prism is source-available and in **public-preview** development — see the
[Status](#status) section above and the full
[Capability Status](./docs/CAPABILITY_STATUS.md) matrix for what is stable
versus experimental. It is not production-ready.

Current focus is stabilization: keeping runtime behavior, docs, and
configuration aligned; a clear golden-path demo; repo hygiene; and hardening the
experimental sub-agent worker, idle-guard optimization, and cross-Prism/Factory
handoff.

See [docs/ROADMAP.md](./docs/ROADMAP.md) and [docs/TASKS.md](./docs/TASKS.md).

---

## License

All rights reserved. Copyright (c) 2025–2026 Emmanuel Vinas. See [LICENSE](./LICENSE) for details.
