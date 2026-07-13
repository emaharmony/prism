# Getting Started with Prism

This is the step-by-step setup guide for a new user. It is practical: by the end
you will have built the Prism CLI, run the test suite, executed your first
workflow, and inspected the artifacts it produced.

> Prism is source-available under an all-rights-reserved license. See
> [LICENSE](../LICENSE). It is a preview-stage project — some features are
> experimental. See [Capability Status](CAPABILITY_STATUS.md).

## What You Will Do

- Install the required tools.
- Clone the repository.
- Build the `prism` CLI from `cmd/prism-cli`.
- Run the test suite.
- Run your first workflow (the built-in echo demo).
- Inspect the output artifacts a run produces.

## Requirements

| Requirement | Required? | Recommended Version | Purpose |
|---|---:|---|---|
| Go | Yes | Go 1.26+ (module requires 1.26.2) | Build and test Prism |
| Git | Yes | Latest stable | Clone the repo; project/worktree/autopatch tools |
| Make | Optional | Latest stable | Shortcut commands via the `Makefile` |
| SQLite | Optional | Bundled | Persistence is built in (embedded); no separate install needed |
| NATS | No | Embedded | Event bus is embedded; only needed externally for advanced setups |
| Ollama | Optional | Latest stable | Local model/provider testing |
| Python | Optional | 3.11+ | Only for the separate Remembrance memory service |
| Claude CLI | Optional | Latest stable | Only for the `claude_code` provider / Claude reviewer |
| Codex CLI | Optional | Latest stable | Only for the Codex worker |
| `gh` CLI | Optional | Latest stable | Only for autopatch `"pr"` mode |
| Node.js | Optional | LTS | Not required — the dashboard is served by `prism serve` |

No external database or message broker is required for basic operation — Prism
embeds NATS JetStream and uses SQLite.

## Clone the Repository

```bash
git clone https://github.com/emaharmony/prism.git
cd prism
```

## Install Dependencies

Go modules are fetched automatically on the first build or test run. To fetch
them ahead of time:

```bash
go mod download
```

## Build Prism

```bash
go build -o prism ./cmd/prism-cli
```

On Windows PowerShell:

```powershell
go build -o .\prism-current.exe .\cmd\prism-cli
```

For a static binary:

```bash
CGO_ENABLED=0 go build -o prism ./cmd/prism-cli
```

> Local root binaries can lag behind source and are ignored by Git. Always build from
> `cmd/prism-cli` and treat the source tree as authoritative.

## Run Tests

```bash
go test ./... -count=1 -race
```

On systems without a C compiler, run `go test ./... -count=1` locally and rely
on CI for the required full race pass; do not report the race check as passed.

Run the model-free preflight:

```bash
go run ./cmd/prism-cli doctor --json
```

Or via Make:

```bash
make test
```

## Run the First Demo

The most reliable first demo is the built-in echo workflow. It runs entirely
locally, needs no model, and produces artifacts you can inspect.

List available workflows:

```bash
go run ./cmd/prism-cli workflow list
```

Show the demo workflow's steps:

```bash
go run ./cmd/prism-cli workflow show demo.echo_tool
```

Run it:

```bash
go run ./cmd/prism-cli workflow run demo.echo_tool
```

The workflow definition lives at
[`examples/workflows/demo-echo.yaml`](../examples/workflows/demo-echo.yaml).
Passing input is optional; if you want to supply input, use a JSON file:

```bash
go run ./cmd/prism-cli workflow run demo.echo_tool --input path/to/input.json
```

Check a run's status by its run id:

```bash
go run ./cmd/prism-cli workflow status <run_id>
```

## Inspect Output Artifacts

Runs write artifacts under the run directory (default `./runs`). A sample run is
checked in at [`examples/runs/sample-run/`](../examples/runs) so you can see the
shape before running your own:

| Artifact | Purpose |
|---|---|
| `events.jsonl` | Append-only event log for the run (one JSON event per line) |
| `prompt.md` | The prompt that was assembled for the run |
| `output.md` | The rendered output/result of the run |
| `summary.json` | Machine-readable summary (status, timing, key fields) |

Browse past runs and reports with:

```bash
go run ./cmd/prism-cli runs
go run ./cmd/prism-cli runs latest --json
```

## Next Steps

- [Configuration Guide](CONFIGURATION.md) — where config lives and how it loads.
- [YAML Reference](YAML_REFERENCE.md) — field-by-field YAML documentation.
- [Command Reference](COMMANDS.md) — every major CLI command.
- [Examples](EXAMPLES.md) — guided demo flows.
- [Troubleshooting](TROUBLESHOOTING.md) — common setup and runtime issues.

To start the full daemon (API, dashboard, optional bot):

```bash
cp prism.yaml.example prism.yaml
go run ./cmd/prism-cli serve --config prism.yaml
```

## Common Setup Problems

- **`package cmd/prism-cli is not in std`** — use `go run ./cmd/prism-cli`
  (with the leading `./`), not `go run cmd/prism-cli`.
- **Wrong directory** — run Go commands from the repository root (the folder
  containing `go.mod`).
- **`workflow not found`** — workflows load from `examples/workflows/`; run
  `workflow list` to see registered names and run from the repo root.
- **Provider/model not configured** — the demo echo workflow needs no model.
  For `prism run` with a real model, install and start Ollama (or configure a
  provider) first.

See [Troubleshooting](TROUBLESHOOTING.md) for the full list.
