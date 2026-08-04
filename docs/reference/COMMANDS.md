# Prism Command Reference

Every major `prism` CLI command grouped by purpose, with what it does and an
example. Commands are shown as `prism <command>`; when running from source use
`go run ./cmd/prism-cli <command>`.

> This list mirrors `prism --help` (the grouped usage in `cmd/prism-cli`). If a
> command differs from your build, prefer the output of `prism --help`.

## Overview

Run the built-in help to see the grouped command list at any time:

```bash
go run ./cmd/prism-cli --help
```

Prism has no single top-level "global flags" set; most flags are per-command
(e.g. `--config`, `--json`, `--project`, `--agent`).

## Run & Interact

| Command | Action | Status | Example |
|---|---|---|---|
| `prism run --task <desc>` | Run a one-shot task lifecycle | Preview | `prism run --task "Explain event-driven architecture" --provider ollama --model llama3.2` |
| `prism chat` | Interactive terminal chat | Preview | `prism chat --config prism.yaml --agent astraea` |
| `prism serve` | Start the persistent daemon (API, dashboard, bot) | Preview | `prism serve --config prism.yaml` |

`prism run` expects a NATS bus at `nats://localhost:4222`; use `serve` for
embedded NATS or start `cmd/prism-bus` separately. Key `run` flags:
`--provider`, `--model`, `--temperature`, `--max-tokens`, `--timeout`,
`--dry-run-prompt`, `--project`, `--agent`, `--run-dir`, `--memory-enabled`.

## Set Up Config

| Command | Action | Status | Example |
|---|---|---|---|
| `prism config wizard` | Interactive setup — generate a `prism.yaml` | Preview | `prism config wizard --out prism.yaml` |
| `prism config import <file>` | Convert an OpenClaw JSON to `prism.yaml` | Preview | `prism config import openclaw.json --out prism.yaml` |

## Inspect & Validate

| Command | Action | Status | Example |
|---|---|---|---|
| `prism config` | Validate `prism.yaml` and summarize it | Preview | `prism config --config prism.yaml --json` |
| `prism doctor` | Preflight check of run dependencies | Preview | `prism doctor --config prism.yaml --json` |
| `prism preview` | Static preview of the gated-loop workflow | Preview | `prism preview --workflow examples/workflows/gated-loop.yaml` |
| `prism agent list\|show <name>` | List / show registered agents | Preview | `prism agent list` |
| `prism tool list\|run <name>` | List / run built-in tools | Preview | `prism tool run echo --input '{"text":"hi"}'` |
| `prism validation list\|run <name>` | List / run validation profiles | Preview | `prism validation list` |
| `prism context show` | Show context that would be injected | Preview | `prism context show --context soul,agents` |

## Workflow Commands

| Command | Action | Status | Example |
|---|---|---|---|
| `prism workflow list` | List registered workflows | Preview | `prism workflow list` |
| `prism workflow show <name>` | Show a workflow's steps | Preview | `prism workflow show demo.echo_tool` |
| `prism workflow run <name>` | Run a named workflow | Preview | `prism workflow run demo.echo_tool` |
| `prism workflow status <run_id>` | Show a workflow run's status | Preview | `prism workflow status <run_id>` |
| `prism workflow cancel <run_id>` | Request durable multi-agent cancellation | Phase 1 | `prism workflow cancel <run_id>` |
| `prism workflow resume <run_id>` | Safely resume a durable multi-agent run | Phase 1 | `prism workflow resume <run_id>` |
| `prism workflow report <run_id>` | Show a terminal multi-agent report | Phase 1 | `prism workflow report <run_id> --json` |
| `prism workflow start` | Start a gated-loop run for a project | Preview | `prism workflow start --project example --prompt "..."` |

Generic `workflow run` accepts optional `--input <file.json>` and
`--run-dir <dir>`. The `multi-agent-software-task` workflow requires a strict
JSON input and also accepts `--config <prism.yaml>`. Its status and report
commands support `--json`. See the
[Multi-Agent Workflow](../MULTI_AGENT_WORKFLOW.md) for its input and artifacts.

## Policy Commands

| Command | Action | Status | Example |
|---|---|---|---|
| `prism policy list` | List policy rules | Preview | `prism policy list` |
| `prism policy evaluate --input <file>` | Evaluate a JSON policy request | Preview | `prism policy evaluate --input request.json` |

## Approval Commands

| Command | Action | Status | Example |
|---|---|---|---|
| `prism approval list` | List approvals | Preview | `prism approval list` |
| `prism approval show <id>` | Show one approval | Preview | `prism approval show <id> --run <run_id>` |
| `prism approval approve <id>` | Approve a mutation | Preview | `prism approval approve <id> --by ema` |
| `prism approval deny <id>` | Deny a mutation | Preview | `prism approval deny <id> --by ema` |

## Observe Runs

| Command | Action | Status | Example |
|---|---|---|---|
| `prism watch` | Live view of a running gated-loop workflow | Preview | `prism watch --config prism.yaml` |
| `prism runs [<id>\|latest]` | List runs / show a run's report | Preview | `prism runs latest --json` |
| `prism cost <run_id>` | Show token usage and cost report | Preview | `prism cost <run_id>` |
| `prism trace <run_id>` | Show event trace (causal DAG) | Preview | `prism trace <run_id>` |
| `prism dashboard` | Start the local read-only dashboard | Preview | `prism dashboard --port 8080` |
| `prism status` | Show live service status | Preview | `prism status` |
| `prism health` | Check bus health | Preview | `prism health` |

## Adapter Commands

| Command | Action | Status | Example |
|---|---|---|---|
| `prism adapter list\|show\|health [<name>]` | Inspect adapters | Preview | `prism adapter list` |

## MCP Commands

| Command | Action | Status | Example |
|---|---|---|---|
| `prism mcp` | List configured MCP servers | Experimental | `prism mcp --json` |
| `prism mcp probe <name>` | Live-probe a server's tools | Experimental | `prism mcp probe filesystem` |

## Skills Commands

| Command | Action | Status | Example |
|---|---|---|---|
| `prism skills` | List SKILL.md skills | Experimental | `prism skills --json` |
| `prism skills show <name>` | Show a skill's instructions | Experimental | `prism skills show <name>` |

## Self-Patching Commands

| Command | Action | Status | Example |
|---|---|---|---|
| `prism scan` | Scan for issues (`--start` fixes the top one) | Experimental | `prism scan --severity high --json` |

## Advanced Commands

| Command | Action | Status | Example |
|---|---|---|---|
| `prism search --query <text>` | Search the vector store | Experimental | `prism search --query "event bus"` |
| `prism projection list\|rebuild\|query` | Manage CQRS projection snapshots | Experimental | `prism projection list` |
| `prism remembrance health\|status\|serve` | Manage the Remembrance memory service | Experimental | `prism remembrance health` |
| `prism version` | Print version | Preview | `prism version` |

## Command Cheat Sheet

### Run tests

```bash
go test ./...
```

### See available workflows

```bash
go run ./cmd/prism-cli workflow list
```

### Run the first workflow

```bash
go run ./cmd/prism-cli workflow run demo.echo_tool
```

### Inspect run status

```bash
go run ./cmd/prism-cli workflow status <run_id>
```

### Start serve mode (API + dashboard)

```bash
go run ./cmd/prism-cli serve --config prism.yaml
```

### Open the read-only dashboard

```bash
go run ./cmd/prism-cli dashboard
```

## Common Command Flows

### First local demo

1. Run tests: `go test ./...`
2. List workflows: `workflow list`
3. Run the demo workflow: `workflow run demo.echo_tool`
4. Inspect artifacts: `runs latest --json`

### Policy test flow

1. List policies: `policy list`
2. Evaluate a JSON request: `policy evaluate --input request.json`
3. Inspect the written artifact under `runs/policy/`.

### Approval flow

1. Run a workflow that creates an approval (file mutation).
2. List pending approvals: `approval list`
3. Approve or deny: `approval approve <id> --by <name>`

### Debug a failed run

1. Check status: `workflow status <run_id>` or `runs <run_id>`
2. Inspect `events.jsonl` in the run directory.
3. Inspect `summary.json` for the final status.
4. Inspect `output.md` / `prompt.md` for detail.
