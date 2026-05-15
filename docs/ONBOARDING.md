# Prism Onboarding Guide

> **For people who don't know Go.** This guide explains what Prism is, how it works, and how to read the code — even if you've never written a line of Go.

## What Is Prism?

Prism is an **event-native AI agent framework**. Instead of hiding AI work inside prompt chains, Prism turns every meaningful step into an **event** — a structured record that can be observed, replayed, audited, and extended.

Think of it like a flight data recorder for AI workflows. Every time something happens (a task starts, an LLM is called, a file is proposed, a validation runs), Prism writes it down as an event with a unique ID, a timestamp, and a link back to what caused it.

## The Big Picture

```
You type a command        Prism orchestrates          Events flow
─────────────────►        ─────────────────►          ──────────►
                                                                 
  prism run --task "..."   1. Connect to NATS         Every step
                          2. Build a prompt            becomes an
                          3. Call the LLM              event with
                          4. Execute tools             a unique ID
                          5. Write results             and parent
                          6. Run validation (V5)       chain
                          7. Generate review (V5)
                          8. Save everything to disk
```

## Core Concepts

### Events

An **event** is a JSON record of something that happened. Every event has:

| Field | What it means | Example |
|-------|---------------|---------|
| `id` | Unique identifier (ULID — sorted by time) | `evt_01KRC7AQXH2W66WNXM07` |
| `type` | What happened, dot-namespaced | `prism.task.created` |
| `source` | Which component emitted it | `prism-cli` |
| `timestamp` | When it happened (UTC, nanosecond precision) | `2026-05-11T15:13:25.123456789Z` |
| `correlation_id` | Groups all events in one run | `corr_01KRC7AQXBM1Y5ZY0QEBG56JTC` |
| `parent_id` | Direct causal predecessor | `evt_01KRC7AQT3WNFK0PV7` |
| `payload` | The actual data | `{"task": "fix the bug", "project": "prism"}` |

The **parent chain** is the key insight. If event B was caused by event A, then B's `parent_id` = A's `id`. You can trace any event back to the original task that started it.

### NATS JetStream

NATS is the **event bus** — a message broker that routes events between components. JetStream adds persistence (events are saved, not just forwarded).

Prism uses it as the central nervous system. Every event gets published to a NATS stream called `PRISM`, and any component can subscribe to events it cares about.

You don't need to understand NATS to read the code. Just know:
- `r.js.Publish(subject, data)` = send an event
- `r.js.AddStream(...)` = create the event storage
- If NATS isn't running, events are logged but not published (graceful degradation)

### Runs

A **run** is one complete execution of a task. Every run gets its own directory under `runs/`:

```
runs/
└── run_01KRC7AQXBM1Y5ZY0QEBG56JTC/
    ├── events.jsonl        ← All events, one JSON per line
    ├── summary.json        ← Run status, event counts, version info
    ├── output.md           ← What the LLM wrote
    ├── approvals/          ← V4: Approval requests and decisions
    │   └── appr_01KR...json
    ├── validation/         ← V5: Validation results
    │   ├── go_test_all.json
    │   ├── go_test_all.stdout.txt
    │   └── go_test_all.stderr.txt
    └── review.md           ← V5: Review artifact with recommendation
```

### Versions (V1 → V5)

Prism grew incrementally. Each version adds one capability:

| Version | What it added | Key idea |
|---------|---------------|----------|
| **V1** | Task lifecycle + events | Every step is an event |
| **V2** | Real LLM execution | The model actually runs |
| **V3** | Tool execution | The model can use tools (read files, list dirs) |
| **V4** | Approval-gated mutations | Model proposes, human approves file writes |
| **V5** | Validation + review | After mutation, run tests and generate review |

All versions coexist. The code doesn't branch by version — V5 code lives alongside V1 code.

## Project Structure

```
prism/
├── cmd/
│   ├── prism-cli/        ← The CLI you run (`prism run`, `prism approval list`, etc.)
│   ├── prism-bus/        ← Standalone NATS JetStream server
│   └── prism-agent/      ← Agent runtime (subscribes to events, processes them)
│
├── internal/             ← All core logic (Go convention: "internal" = not importable by outsiders)
│   ├── event/            ← The event schema — Event struct, IDs, summaries, event types
│   ├── run/              ← The Runner — orchestrates the entire lifecycle
│   ├── provider/         ← LLM providers (Ollama, Mock for testing)
│   ├── agent/            ← Placeholder agent (V1/V2 deterministic output)
│   ├── prompt/           ← Prompt builder (assembles prompt.md from template)
│   ├── tool/             ← Tool registry, policy engine, built-in tools
│   ├── approval/         ← V4: Approval model, state machine, file-based store
│   ├── mutation/         ← V4: Safe file writer after approval is granted
│   ├── validation/       ← V5: Validation profiles, executor, safety checks
│   ├── review/           ← V5: Deterministic reviewer, review artifact
│   └── remembrance/      ← Memory/context hook (HTTP client to memory system)
│
├── sdk/
│   └── prism/            ← Python SDK (PrismClient, Event class, tool helpers)
│
├── runs/                 ← Run outputs (gitignored, created at runtime)
└── docs/
    ├── DESIGN.md         ← Architecture design document
    └── ONBOARDING.md     ← This file!
```

## The Run Lifecycle (Step by Step)

When you type `prism run --task "fix the bug" --project prism`, here's what happens:

### Phase 1: Setup
1. **Connect to NATS** — The Runner opens a connection to the NATS server and creates the `PRISM` stream
2. **Create the run** — A new run ID is generated (`run_<ulid>`), a directory is created

### Phase 2: Context
3. **Inject memory context** (optional) — The Runner calls the Remembrance service to get relevant memories for the task. If it fails, the run continues without memory (graceful fallback)
4. **Build the prompt** — The prompt builder assembles `prompt.md` from a template, injecting the task, project, and any memory context

### Phase 3: Execution
5. **Call the LLM** — The configured provider (Ollama, mock, etc.) generates a response from the prompt
6. **Save the output** — The LLM's response is written to `output.md`

### Phase 4: Tool Execution (V3+)
7. **Parse tool requests** — If the LLM's output contains a JSON tool request (type, tool, input), the Runner parses it
8. **Evaluate policy** — The policy engine checks if the tool is allowed and what level of approval it needs
9. **Execute the tool** — If allowed, the tool runs and the result is recorded

### Phase 5: Approval (V4+)
10. **Handle proposals** — If the tool was `write_file_proposal`, the Runner creates an approval request (pending state) and saves it to `approvals/<approval_id>.json`. The file is NOT written yet.
11. **Wait for human** — The run ends in `pending_approval` status. A human operator reviews the proposal and uses `prism approval approve` or `prism approval deny`.

### Phase 6: Validation + Review (V5+)
12. **Apply mutation** — When approved, the mutation executor writes the file safely (path traversal check, workspace containment)
13. **Run validation** — If `--validate` was passed, each allowlisted validation profile (e.g., `go_test_all`) is executed with a timeout
14. **Generate review** — A deterministic reviewer examines the mutation + validation results and writes `review.md` with a recommendation

### Phase 7: Persistence
15. **Save everything** — Events are appended to `events.jsonl`, summary is updated in `summary.json`

## Go Concepts You'll See

If you're new to Go, here's a cheat sheet for patterns used in Prism:

### `package internal`
Go's `internal/` directory is special — code inside it can only be imported by code in the same parent directory. This is Go's way of saying "these are implementation details, not a public API."

### `func (r *Runner) Method()`
This is a **method** on the `Runner` struct (like `this.method()` in other languages). The `r` is the receiver — it's how the function accesses the Runner's data.

### `if err != nil { return err }`
Go doesn't have exceptions. Functions return both a result and an error. You'll see this pattern everywhere — it means "if something went wrong, stop and report the error up the chain."

### `defer`
`defer` runs a statement when the function exits, no matter how it exits (normal return, error return, panic). Used for cleanup like closing files or connections.

### `context.Context`
`context` is Go's way of passing deadlines, cancellation signals, and request-scoped values through a call chain. When you see `ctx context.Context`, think "this operation can be cancelled or timed out."

### `json.Marshal` / `json.Unmarshal`
Convert Go structs to/from JSON. Struct tags like `` `json:"run_id"` `` control the JSON field names.

### `_ = someValue`
The blank identifier — "I don't care about this value." Used when a function returns something you don't need.

### `interface{}`
The "any" type — can hold any value. Modern Go uses `any` as an alias. You'll see both in the codebase.

## Key Files to Read First

If you want to understand Prism by reading code, start here in order:

1. **`internal/event/event.go`** — The event schema. This is the foundation everything else builds on.
2. **`internal/run/runner.go`** — The orchestrator. This is the biggest file and the most important. It connects everything.
3. **`internal/tool/policy.go`** — The safety model. This decides what tools are allowed and what needs approval.
4. **`internal/approval/approval.go`** — The approval state machine. Simple and well-isolated.
5. **`internal/mutation/executor.go`** — How file writes happen safely after approval.
6. **`internal/validation/executor.go`** — How validation commands run safely.
7. **`internal/review/review.go`** — How reviews are generated deterministically.

## Glossary

| Term | Meaning |
|------|---------|
| **ULID** | Universally Unique Lexicographically Sortable Identifier — like UUID but sortable by time. All Prism IDs use ULIDs. |
| **NATS** | High-performance messaging system. Prism uses it as the event bus. |
| **JetStream** | NATS persistence layer. Stores events durably instead of just forwarding them. |
| **correlation_id** | Shared by all events in one run. Links them together like a thread. |
| **parent_id** | Points to the direct cause of an event. Forms a causal chain. |
| **provider** | An LLM backend (Ollama, OpenAI, mock). The `Provider` interface defines how Prism talks to any LLM. |
| **policy** | Rules about what tools can do. `allowed` = run it, `requires_approval` = ask a human first, `denied` = block it. |
| **mutation** | A file write operation. "Mutation" because it changes the filesystem. |
| **validation profile** | A named, allowlisted command that can run after a mutation (e.g., `go_test_all`). |
| **review** | A deterministic analysis of the mutation + validation results, written as `review.md`. |
| **remembrance** | The memory/context service. Optional — if it's down, Prism keeps working. |
| **dry run** | A mode where Prism shows what it would do without actually doing it. Used for tool preview. |

## Common Workflows

### "I want to see what happened in a run"
```bash
cat runs/run_01KR.../summary.json    # Quick status overview
cat runs/run_01KR.../events.jsonl    # Full event log
cat runs/run_01KR.../review.md      # V5 review of the change
```

### "I want to approve or deny a file write"
```bash
prism approval list                              # See pending approvals
prism approval show appr_01KR... --run run_01KR  # Read the proposal
prism approval approve appr_01KR --by ema --run run_01KR  # Approve it
prism approval deny appr_01KR --by ema --reason "Not needed"  # Deny it
```

### "I want to run validation after a mutation"
```bash
prism validation list                      # See available profiles
prism validation run go_test_all --project prism  # Run tests
prism approval approve appr_01KR --by ema --validate  # Approve + validate
```

## Architecture Diagram

```
                    ┌─────────────┐
                    │  prism-cli  │  Your command goes here
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Runner    │  Orchestrates everything
                    └──────┬──────┘
                           │
              ┌────────────┼────────────────┐
              │            │                │
       ┌──────▼──────┐ ┌──▼───┐ ┌─────────▼────────┐
       │  Provider    │ │ Tool │ │   Remembrance    │
       │  (LLM call)  │ │ Exec │ │   (memory hook)  │
       └─────────────┘ └──┬───┘ └──────────────────┘
                          │
                 ┌────────┼─────────┐
                 │        │         │
          ┌──────▼──┐ ┌──▼────┐ ┌──▼──────┐
          │ Policy  │ │Approval│ │Mutation │
          │ (allow/ │ │(pending│ │(safe    │
          │ deny)   │ │→yes/no)│ │ write)  │
          └─────────┘ └───────┘ └────┬────┘
                                       │
                              ┌────────▼────────┐
                              │   Validation    │  V5
                              │  (run tests)    │
                              └────────┬────────┘
                                       │
                              ┌────────▼────────┐
                              │     Review      │  V5
                              │  (summarize)    │
                              └─────────────────┘
                                       │
                              ┌────────▼────────┐
                              │   NATS Bus      │  Events flow here
                              │  (JetStream)    │
                              └─────────────────┘
```

## Contributing

All changes go through pull requests. See the project rules:
- One branch per task
- Ema reviews and merges all PRs
- Every feature must be tested before moving on
- No direct pushes to main

---

*Welcome to Prism. Every event tells a story. 💎*