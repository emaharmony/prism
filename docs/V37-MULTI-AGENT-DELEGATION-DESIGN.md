# V37 — Multi-Agent Delegation Wiring

**Status:** Source-current
**Last Updated:** 2026-06-29

## Problem

The gated loop shipped with a `DelegationManager` (packet building, completion
handling, timeout checks) and a `Delegations` slice on the workflow state, but
nothing ever called it: `Engine.Drive` never delegated, `handleExternalEvent`
updated task status directly instead of through the manager, and timeouts were
never checked. Multi-agent collaboration — a core promise of the platform — was a
stub.

## Design

Delegation is **model-driven and symmetric with `run_validation`**: the model
issues a `delegate` tool call; the engine services it directly (not via the
external tool executor) so it reuses the manager and stays unit-testable.

### `delegate` tool

`{"type":"tool_request","tool":"delegate","input":{"task_id":"T2"}}`

`Engine.handleDelegate`:

1. Validates a delegation manager exists, a plan exists, the `task_id` is given,
   the task is in the plan, and it has an `agent` assigned (each failure returns a
   clear model-readable message rather than erroring the run).
2. Calls `DelegationManager.DelegateTask`, which records a `DelegationState`
   (status `sent`), marks the task `in_progress`, and returns the `TaskPacket`.
3. Publishes the packet via the injected transport (`SetTaskPublisher`); without a
   transport the delegation is still recorded (test/offline friendly).
4. Emits `task.delegated` and acknowledges to the model, which continues other
   work while the result arrives asynchronously.

### Completion

`RunGatedLoop` subscribes to `<agent-subject>.complete` and forwards matching
messages (filtered by `workflow_id`) as `task_complete` external events.
`handleExternalEvent` now routes these through
`DelegationManager.HandleTaskCompletion`, which closes out the `DelegationState`
and updates the task status/artifacts. The `task_completion` gate then reflects
delegated work, so the loop naturally waits for delegated tasks (bounded by the
phase iteration budget) without bespoke blocking logic.

### Timeouts and retry

Each EXECUTION iteration calls `DelegationManager.CheckTimeouts`. A delegation past
its per-agent deadline is first **retried** while retries remain
(`DelegationManager.RetryDelegation`): the retry count bumps, the clock re-arms to
`sent`, a fresh packet is republished, and `delegation.retry` is emitted. Once
`maxRetries` is exhausted the task is marked `failed` (delegation `timed_out`) via
`WorkflowState.FailDelegatedTask`, `delegation.timeout` is emitted, and the model is
nudged to handle it directly — so a transient sub-agent hiccup gets another chance
while a genuinely dead one still can't hold the gate hostage.

## Safety & testability

- `DelegateTask` performs no network I/O (the caller publishes), so the whole
  delegation path is unit-testable without NATS.
- Delegation is bounded by the same guards as the rest of the loop: phase
  iteration budget, run budgets (time/token), and stuck-loop detection (spamming
  `delegate` trips the repeat cap).

## Tests

`internal/workflow/v2/delegation_wiring_test.go`: happy-path delegate (records +
publishes + marks in_progress), guard messages (no manager / no plan / missing
task_id / unknown task / unassigned agent), end-to-end delegate→complete→gate, and
`FailDelegatedTask`.

## Roster-aware targeting (shipped)

When the workflow knows its agent roster (`config.agents` populated →
`state.AgentRegistry`), `handleDelegate` validates the target before recording a
delegation: an unregistered agent is rejected with the list of known agents, and an
`offline` agent is rejected with a nudge to pick an available one. When the roster
is empty (agents resolved dynamically from the running config), the check is skipped
and delegation proceeds — so the guard never blocks the common dynamic-roster case.

## Follow-ups

- Capability *matching* (target agent's `Capabilities` vs a task-required
  capability) once plan tasks carry a required-capability field.
