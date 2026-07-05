# V58 — Full Autonomy: Generic Sub-Agent Worker

Status: in-progress (built incrementally via the autonomous builder loop)
Started: 2026-07-05

## Goal

Promote the multi-agent roster from "orchestrator-routed" (one brain adopts
roles / calls tools) to **genuinely autonomous sub-agents** that run
independently and report back. This is the deferred v2 piece from the Roblox
team design (see docs/ROBLOX-TEAM.md, [[roblox-agent-team]]).

Today the delegation path is half-built:
- **Send** exists: `v2.DelegationManager.DelegateTask` builds a `TaskPacket`,
  the wake handler publishes it via `NATSPublisher` to the delegation subject.
- **Completion routing** exists: completions on `prism.workflow.task.complete`
  → `NATSListener` → `ExternalEvent{task_complete}` → `HandleTaskCompletion`.
- **MISSING (the consumer):** nothing subscribes to the delegation subject,
  runs the target agent, and publishes a `TaskCompletion`. The serve-side
  handler is a stub (`cmd_serve.go:449` "Task processing will be wired in
  M3.1d"). Only Codex + cross-Prism are real runners.

V58 builds that consumer as a production-grade, testable package and wires it in.

## Architecture

`internal/subagent` — a transport-agnostic worker that turns a `TaskPacket`
into a `TaskCompletion`:

```
delegation subject ──▶ Worker.Handle(packet)
                          ├─ resolve target agent (provider/model/tools/caps)
                          ├─ run a bounded execution (TaskRunner)
                          └─ build TaskCompletion (artifacts/summary/status)
                       ──▶ publish on completionSubject
```

Key seams (all interfaces, so each layer is unit-testable and swappable):
- `AgentResolver.Resolve(agentID)` → the agent's runtime (or not-found).
- `TaskRunner.Run(ctx, packet, runtime)` → artifacts + summary (the pluggable
  execution backend: a mock in tests; a real bounded tool-loop / RunGatedLoop
  slice in later iterations).
- `Publisher.PublishCompletion(completion)` → transport (NATS in serve; capture
  in tests).

Design principles for production/long-term:
- **Fail closed & always report:** unknown agent, runner error, or timeout each
  yield a `status:"failed"` completion — a dead sub-agent never silently holds a
  gate (mirrors the existing timeout/retry semantics in `delegation.go`).
- **Bounded:** every run honors the packet `Deadline`; the worker enforces a
  context timeout so a runaway sub-agent can't run forever.
- **Isolation-ready:** parallel sub-agents reuse V56 worktree isolation + V57
  rollback + per-run budgets already in the tree.
- **No behavior change until wired:** the package lands and is tested before any
  serve wiring flips the stub, so each iteration keeps a green build.

## Iterations (loop-tracked)

- [x] **1 — Foundation:** `internal/subagent` package: `Worker`, the three
  interfaces, `Handle` with resolve/run/timeout/failure paths, and a mock-based
  smoke test. No serve wiring. (this fire)
- [x] **2 — Real runner:** `LoopRunner` (a `TaskRunner`) — bounded single-agent
  tool loop with iteration + token budgets, tool-failure feedback, artifact
  capture, and context-deadline honoring. Model/tool specifics injected via a
  `Backend` (mock in tests; provider registry + tool executor in iteration 3).
- [ ] **3 — Serve wiring:** subscribe the worker to the delegation subject in
  `cmd_serve.go` (replace the M3.1d stub), publish completions; feature-flagged.
- [ ] **4 — Capability routing + concurrency:** worktree-isolated parallel runs,
  per-agent tool scoping, capability-aware assignment.
- [ ] **5 — Full system run:** one proper end-to-end run against the Eggventura
  project (D:/Projects/Roblox/Eggventura), report-only first, then gated.

## Report of changes (append per iteration)

- **Iteration 1 (2026-07-05):** Added `internal/subagent` (worker + interfaces +
  smoke tests). Transport/runtime-agnostic; green build, no serve behavior change.
- **Iteration 2 (2026-07-05):** Added `LoopRunner` — the real bounded tool-loop
  `TaskRunner` (iteration + token budgets, tool-failure feedback, artifact
  capture, deadline honoring) with a `Backend` seam. 8 runner tests incl. an
  end-to-end Worker+LoopRunner path. Still no serve wiring; green build.

## Verification
`go build ./... && go vet ./... && go test ./...` green each iteration. Smoke
tests per iteration exercise the new slice end-to-end with mocks; iteration 5 is
the live Eggventura run.
