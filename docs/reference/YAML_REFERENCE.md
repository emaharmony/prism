# Prizm YAML Reference

Complete field-by-field reference for every Prizm configuration option. Each field includes what it does, which feature it powers, and sensible defaults.

> **Tip:** Validate your config anytime with `prizm config --config prizm.yaml --json`

---

## Table of Contents

1. [prizm.yaml — Main Runtime Config](#prizmyaml--main-runtime-config)
   - [prizm (top-level)](#prizm-top-level)
   - [agents](#agents)
   - [projects](#projects)
   - [channels](#channels)
   - [channel_roles](#channel_roles)
   - [api](#api)
   - [cost](#cost)
   - [usage](#usage)
   - [sessions](#sessions)
   - [scheduler](#scheduler)
   - [remembrance](#remembrance)
   - [memory](#memory)
   - [bridge](#bridge)
   - [codex](#codex)
   - [claude_code](#claude_code)
   - [autopatch](#autopatch)
   - [factory_monitor](#factory_monitor)
   - [shell](#shell)
   - [users](#users)
   - [actions](#actions)
   - [mcp_servers](#mcp_servers)
2. [Workflow YAML](#workflow-yaml)
3. [Policy YAML](#policy-yaml)

---

## prizm.yaml — Main Runtime Config

The single config file that controls everything. Copy `prizm.yaml.example` and edit.

### prizm (top-level)

Core instance settings — identity, networking, paths, logging, and LLM behavior.

```yaml
prizm:
  instance_id: "lumi-mac"              # Unique identifier for this Prizm instance. Used in cross-Prizm messages and event source tags.
  nats_url: "nats://127.0.0.1:4222"    # NATS server URL. Empty string = embedded NATS (no external broker needed).
  data_dir: ".prizm/data"              # Where SQLite databases and run artifacts are stored. Created if missing.
  runs_dir: "runs"                     # Per-run artifact directory (approval records, summaries, events.jsonl). Relative to CWD unless absolute.
  workspace: "/path/to/workspace"      # Root for context injection (SOUL.md, AGENTS.md, etc.). Default: $HOME/.openclaw/workspace
  port: 8321                           # Health check server port. API server uses port+1 (8322).
  bind_host: "127.0.0.1"               # Network interface for HTTP servers. "127.0.0.1" = loopback only (safe without auth). "0.0.0.0" = network-exposed (requires api.auth_token).
  log_level: "info"                    # Logging verbosity: debug, info, warn, error.
  allowed_paths:                        # Additional directory roots the agent can access beyond workspace. Workspace is always implicit.
    - "/Users/ema/projects/repos"
  read_roots:                           # Explicit read/search/list access roots. Falls back to allowed_paths when empty.
    - "/Users/ema/projects/repos"
  write_roots:                          # Explicit write mutation roots. Falls back to allowed_paths when empty.
    - "/Users/ema/projects/repos"
  context_token_budget: 4000            # Max tokens for workspace context injection. Higher = more context, less room for conversation.
  llm_timeout_seconds: 1200            # Timeout per LLM call in serve mode. 20 min default for slow local models.
  workflow_config: "examples/workflows/gated-loop.yaml"  # Path to gated-loop workflow definition. Overrides built-in default phases.
```

#### tts (within prizm)

Text-to-speech via Voicebox. When enabled, Prizm generates voice messages alongside text.

```yaml
  tts:
    enabled: true                                         # Enable TTS voice responses.
    profile_id: "63ca0dc3-..."                            # Voicebox voice profile ID.
    engine: "kokoro"                                      # TTS engine name.
    voicebox_url: "http://localhost:17493"                 # Voicebox service URL.
    max_chars: 500                                        # Max characters per voice message.
```

### agents

Define the AI agents Prizm can use. Each agent gets its own model, role, context, and capabilities.

```yaml
agents:
  - id: lumi                              # Unique agent ID. Becomes event namespace prefix (prizm.agent.lumi.*).
    role: "lead"                          # Agent role: lead, researcher, implementation, planning, review, etc.
    provider: "ollama"                    # LLM provider: ollama, openai, openai_responses, anthropic, gemini, claude_code, codex
    model: "glm-5.2:cloud"               # Model identifier for the provider.
    primary: true                        # Marks this agent as default for unaddressed messages. Only one should be primary.
    first_class_tools: true               # Give agent direct tool access without per-call policy evaluation. For trusted agents in free mode.
    fallbacks:                            # Ordered list of fallback providers if primary fails.
      - provider: "claude_code"
        model: "claude-sonnet-5"
      - provider: "ollama"
        model: "qwen3.5:4b"              # Local model fallback for when cloud providers are unreachable.
    context:                              # Context layers to inject into system prompt.
      - soul                              # SOUL.md — personality and tone
      - agents                            # AGENTS.md — agent roster and roles
      - user                              # USER.md — user profile and preferences
      - identity                          # IDENTITY.md — who the agent is
      - memory                            # MEMORY.md — memory brief
      - snippy                            # SNIPPY.md — Windows machine notes
      - correspondence                    # correspondence/ — message history
    listen_to_agents:                      # Discord bot user IDs to treat as agent-to-agent peers (not ignore).
      - "1496557450852306965"              # Other bot IDs
    subscriptions:                        # NATS subjects this agent subscribes to for delegated tasks.
      - "prizm.subagent.lumi"
    conversation_postfix: "..."           # Appended to system prompt. Shapes conversation behavior.
    invocable_via_api: false              # Allow POST /api/v1/agents/{id}/invoke. Opt-in network surface.
    state_actions:                        # Context-dependent prompt injections keyed by channel role.
      manager-room:
        inject: "You are in the manager room — full dev mode."
      fun:
        inject: "You are in the fun channel — no tools, no code, pure vibes."
      agent:
        inject: "This message is from another AI agent. Respond with structured data."
```

### projects

Assignable project configs for the gated loop. Replace hardcoded repo paths.

```yaml
projects:
  - id: bassbook                           # Unique project identifier.
    repo_path: "/path/to/BassBook"         # Absolute path to the project's git repo.
    state_file: "PROJECT_STATE.md"          # Project state file the agent reads for task assignment.
    default_branch: "main"                 # Branch protected from direct commit/push.
    channel: "1526378127528427602"          # Discord channel ID for results/feedback.
    workflow_config: "examples/workflows/fast-loop.yaml"  # Override global workflow for this project.
    token_budget: 0                        # Override workflow's max_total_tokens. -1=unlimited, 0=default cap, >0=explicit.
    orchestrator: "lumi"                   # Agent ID whose model drives this project's gated loop.
    worktree_isolation: false              # Run in isolated git worktree per run. Default: shared main worktree.
    default: true                          # Use this project when no project is specified.
```

### channels

Messaging channel configurations. Currently supports Discord.

```yaml
channels:
  - type: "discord"                         # Channel type. Currently only "discord" is supported.
    token: "BOT_TOKEN_HERE"                 # Discord bot token.
    channels:                                # Discord channel IDs to listen in.
      - "1491622824991920231"                # build-room
      - "1491622581348864162"                # manager-room
      - "1526378127528427602"                # sys-manager
```

### channel_roles

Map Discord channel IDs to behavior modes, tool access, personality, and context.

```yaml
channel_roles:
  - id: "1491622581348864162"              # Discord channel ID.
    role: manager-room                      # Role name (matches state_actions keys).
    mode: free                              # "gated" (proposal/approval) or "free" (direct execution).
    shell_policy: tier_3                    # Shell access tier: none, tier_1, tier_2, tier_3 (see shell.allowlists).
    tools: all                              # Tool access: "all", "read-only", or "none".
    personality: direct                     # Communication style: direct, terse, bubbly, social.
    tts: true                               # Enable voice messages in this channel.
    tagged_only: false                      # Only respond when @mentioned.
    context: |                              # Structured channel context (replaces state_actions.inject).
      You are in #manager-room. Full dev mode.
```

**Personality modes:**
- `direct` — Make decisions, push back, no menus of options
- `terse` — Structured data, concise, no pleasantries
- `bubbly` — Exaggerated, playful, enthusiastic
- `social` — Warm, conversational, present

### api

HTTP API authentication and CORS configuration.

```yaml
api:
  auth_token: ""                           # Static bearer token for state-changing endpoints. Empty = no auth (loopback only).
  auth_token_env: "PRIZM_API_TOKEN"        # Environment variable name for the token. Takes priority over auth_token.
  allowed_origins: []                      # CORS origin allowlist. Empty = same-origin only. "*" = allow all (dev only).
  max_request_bytes: 1048576               # Max JSON body size for mutating endpoints. Default 1 MiB.
  max_workspace_file_bytes: 4194304        # Max single workspace file write. Default 4 MiB.
```

### cost

Per-model token pricing overrides for cost estimation.

```yaml
cost:
  pricing:
    glm-5.2:cloud:                         # Model ID.
      input: 0.0015                         # USD per 1K input tokens.
      output: 0.006                         # USD per 1K output tokens.
    claude-sonnet-5:
      input: 0.003
      output: 0.015
```

### usage

Dashboard token-usage tracker time windows.

```yaml
usage:
  windows:
    - range: "day"                          # Usage range: session, day, week, month, year, lifetime
      window: "24h"                         # Lookback period as Go duration.
      bucket: "1h"                          # Aggregation bucket width.
```

### sessions

Session management settings.

```yaml
sessions:
  max_context_messages: 100               # Max messages kept in conversation context before compaction.
  idle_timeout_minutes: 0                  # Minutes of inactivity before session resets. 0 = never.
  compaction_strategy: "truncate"          # How to compress: "truncate" (drop oldest) or "summarize".
  daily_reset_hour: 4                      # Hour (local time) at which daily stats reset. Default 4 AM.
```

### scheduler

Cron-style scheduled tasks that fire NATS events.

```yaml
prizm:
  scheduler:
    enabled: false                         # Enable the scheduler. Default false.
    jobs:
      - name: "daily-review"               # Human-readable job name.
        schedule: "0 3 * * *"              # Cron expression (min hour dayOfMonth month dayOfWeek).
        event: "prizm.task.scheduled"       # NATS subject to publish.
        payload:                            # JSON payload for the event.
          action: "daily_review"
        enabled: false                      # Enable this specific job.
```

### remembrance

Configuration for the Recall (Remembrance v2) memory service client.

```yaml
remembrance:
  enabled: true                            # Enable Recall integration. When false, agents use local memory only.
  url: "http://127.0.0.1:18790"            # Recall service URL.
  timeout_seconds: 60                      # HTTP timeout for Recall requests. Capture may run synchronous extraction.
```

### memory ⭐ NEW

Local memory store configuration. Phase 1 uses MarkdownStore (file-based). Phase 2 will add SQLite with vector search.

```yaml
memory:
  store_type: "markdown"                   # Storage backend: "markdown" (Phase 1) or "sqlite" (Phase 2, future).
  store_path: "memory"                    # Path to memory directory, relative to workspace root. Default: "memory".
  gate_model: "nemotron-3-nano:4b"         # Primary local Ollama model for gate decisions (is this worth remembering?).
  extract_model: "nemotron-3-nano:4b"      # Primary local Ollama model for memory extraction (structure the memory).
  model_fallback_chain:                    # Ordered list of models to try if primary fails.
    - "nemotron-3-nano:4b"                 # Try first — purpose-built for extraction.
    - "qwen3.5:4b"                        # Try second — decent general model.
  ollama_url: "http://localhost:11434"     # Ollama API endpoint for local model calls.
  auto_capture: true                       # Automatically gate conversation turns for memory worthiness.
  min_importance: 0.5                      # Gate threshold (0.0–1.0). Below this = not worth remembering.
  max_memories_per_turn: 3                 # Max memories extracted from a single conversation turn.
  recall_sync: "async"                    # Push to Recall after local write: "async" (fire-and-forget) or "off" (local only).
```

**How the memory pipeline works:**

1. **Gate** — `gate_model` decides if a turn is worth remembering (saves cloud tokens)
2. **Extract** — `extract_model` structures it (category, tier, summary, topics)
3. **Store** — writes to `memory/YYYY-MM-DD.md` (shared with OpenClaw via symlink)
4. **Emit** — publishes `prizm.memory.*` events on the NATS bus
5. **Sync** — if `recall_sync: async`, pushes to Recall in the background

**Token savings:** Local gate+extract ≈ 230 tokens/write vs ~600 on cloud. ~62% savings.

### bridge

Cross-Prizm protocol bridge for multi-instance communication.

```yaml
bridge:
  enabled: false                           # Enable cross-Prizm bridge. Default false.
  mode: "shared_nats"                     # Bridge mode: "shared_nats".
  secret_env: "PRIZM_BRIDGE_SECRET"       # Environment variable holding the shared secret for bridge auth.
  allowed_subjects:                         # NATS subjects allowed to cross the bridge.
    - "prizm.cross.context_sync"
    - "prizm.cross.task_request"
    - "prizm.cross.status_request"
  factory:
    enabled: false                         # Enable Roblox Factory bridge. Default false.
```

### codex

Codex CLI sub-agent configuration for task delegation.

```yaml
codex:
  enabled: true                            # Enable Codex CLI integration.
  executable: ""                            # Path to codex binary. Empty = auto-detect ("codex" on PATH).
  model: ""                                # Model override. Empty = Codex CLI default/profile model.
  sandbox: "workspace-write"               # Sandbox policy: "read-only", "workspace-write", "danger-full-access".
  approval_policy: "on-request"            # When Codex asks for approval: "on-request", "never", "always".
  timeout_minutes: 30                      # Max time per Codex task before timeout.
```

### claude_code

Claude Code CLI configuration for review and implementation tasks.

```yaml
claude_code:
  enabled: true                            # Enable Claude Code CLI integration.
  executable: "claude"                     # Path to claude binary.
  model: "claude-sonnet-5"                 # Model to use.
  reviewer_name: "mango"                    # Agent ID that auto-fulfills review_pass gates.
  timeout_minutes: 10                       # Max time per Claude Code task.
```

### autopatch

Diagnose-and-propose patch task configuration.

```yaml
autopatch:
  enabled: false                           # Enable auto-patch tasks. Default false.
```

### factory_monitor

Local Roblox Factory status notifications.

```yaml
factory_monitor:
  enabled: false                           # Enable Factory monitor. Default false.
```

### shell

Shell tool access control for free-mode channels.

```yaml
shell:
  master_user_id: "164169326142816256"     # Discord user ID that gets unrestricted shell access.
  allowlists:
    tier_1:                                # Read-only commands. Safe for any context.
      - "go build*"
      - "git status*"
      - "cat *"
      - "ls *"
    tier_2:                                # Standard dev commands. Needs trusted context.
      - "npm *"
      - "go *"
      - "git *"
      - "docker *"
    tier_3:                                # Full access. Only in manager-room.
      - "*"                                # Matches everything.
  defaults:
    timeout_seconds: 30                    # Default shell command timeout.
    max_output_bytes: 10240                # Max bytes of output captured.
    blocked_patterns:                      # Always blocked, regardless of tier.
      - "rm -rf /"
      - "rm -rf ~"
      - "mkfs*"
      - "dd if=*of=/dev/*"
```

### users

Map external channel identities to stable owner IDs for session continuity.

```yaml
users:
  - discord_id: "164169326142816256"        # Discord user ID.
    owner_id: "ema"                         # Stable internal ID. Used for session continuity and memory.
    display_name: "Ema"                     # Human-readable name.
    aliases: ["emmanuel", "rhemma"]          # Additional name variants for matching.
```

### actions

Event-triggered actions (webhook-style). Currently unused in default config.

```yaml
actions: []                                # List of action configs. Empty = no actions.
```

### mcp_servers

External Model Context Protocol tool servers registered at startup.

```yaml
mcp_servers:
  - name: "filesystem"                     # Logical server name. Tools register as mcp_<name>_<tool>.
    command: "npx"                         # Executable to spawn.
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/repo"]  # Arguments.
    env: []                                # Extra KEY=VALUE environment entries.
    enabled: true                          # Skip when false.
mcp_auto_approve: false                    # Auto-approve MCP tool calls (skips approval gate). Default false — MCP tools require approval.
```

---

## Workflow YAML

Defines named step pipelines for the gated loop or multi-agent workflows.

```yaml
name: demo.echo_tool                      # Unique workflow name (used by `prizm workflow run <name>`).
description: "Demo workflow that runs an echo tool."  # Human-readable description.
version: 1                                 # Workflow version.

steps:
  - id: echo                               # Unique step ID inside the workflow.
    type: tool.execute                     # Step type: tool.execute, dispatch.run, delegate
    tool: echo                              # Tool name (for tool.execute).
    input:                                  # Step-specific input.
      text: "hello from workflow"
    when: "step.prev.status == completed"   # Optional condition guarding step execution.
```

**Token budgets** (`global.max_total_tokens`):
- `-1` = explicitly unlimited (only when external guardrails exist)
- `0` or omitted = built-in default ceiling (2,000,000 tokens)
- Positive integer = explicit cap in tokens

**Step types:**
| Type | Purpose | Key Fields |
|------|---------|------------|
| `tool.execute` | Run a built-in tool | `tool`, `input` |
| `dispatch.run` | Dispatch to an adapter | `adapter`, `action`, `input` |
| `delegate` | Delegate to another agent | `agent`, `input` |

---

## Policy YAML

Allow/deny/approval rules. Policy decides permission; local validators enforce input safety.

```yaml
policies:
  - id: deny_shell_execution               # Unique policy rule ID.
    description: "Block shell execution."  # Human-readable explanation.
    match:                                  # Exact-match criteria.
      action: tool.execute                  # Action type.
      resource.name: run_command            # Resource name.
    decision: denied                        # allowed, denied, or requires_approval.
    reason: "Shell execution is not supported."  # Reason recorded in events.
    severity: critical                     # warning or critical.
```

**Decision values:**
- `allowed` — execute without asking
- `denied` — block entirely
- `requires_approval` — pause for human approval

---

## Common Mistakes

### Duplicate step IDs in workflows

```yaml
# ❌ Bad
steps:
  - id: run
    type: tool.execute
  - id: run
    type: validation.run

# ✅ Good
steps:
  - id: run_tool
    type: tool.execute
  - id: run_validation
    type: validation.run
```

### Storing secrets in YAML

```yaml
# ❌ Bad — secrets in config files
openai_api_key: sk-...

# ✅ Good — reference an environment variable
auth_token_env: "PRIZM_API_TOKEN"
```

### Non-loopback bind without auth

```yaml
# ❌ Dangerous — exposed without auth
bind_host: "0.0.0.0"  # network accessible
# auth_token: ""       # no auth

# ✅ Safe — either loopback or auth required
bind_host: "127.0.0.1"  # loopback only
# OR
bind_host: "0.0.0.0"
auth_token_env: "PRIZM_API_TOKEN"  # auth required for network exposure
```