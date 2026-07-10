# V40 — Durable Report Artifact

**Status:** Source-current
**Last Updated:** 2026-06-29

## Problem

The REPORT phase produces a rich proof-of-work summary (phase outcomes with gate
scores, verification result, delegations, task results, feedback), but it was only
delivered as a Discord/terminal message — which scrolls away. There was no durable,
on-disk record a user could open later or attach to a PR.

## Design

`v2.WriteReportArtifact(state, baseDir)` formats the final report
(`FormatFinalReport`) and writes it to `<baseDir>/<runID>/REPORT.md`, alongside the
validation stdout/stderr/json artifacts that already live under
`<stateDir>/<runID>/`. `RunGatedLoop` calls it right after `Engine.Drive` returns.

- **Path:** `runs/gated-loop/<runID>/REPORT.md` by default (follows
  `global.state_persistence_dir`). RunID falls back to `gated-loop` when unset.
- **Non-fatal:** it is an output helper, not part of the run loop — a write failure
  is logged and ignored; the report is still returned in-band to the caller, so a
  read-only filesystem never breaks a run.
- **No core change:** the function lives next to the other v2 output formatters and
  does not touch the driver, gates, or state machine.

## Tests

`internal/workflow/v2/report_artifact_test.go`: writes to the expected
`<base>/<runID>/REPORT.md` path with the report content (workflow header, run id,
verification profile, task ids); defaults the directory to `gated-loop` when RunID
is empty; and errors on a nil state.

## Follow-ups (UX roadmap)

Remaining brainstormed items: diff preview at feedback gates, dry-run/plan preview,
rich Discord approval cards, and gate-needs-you notifications.
