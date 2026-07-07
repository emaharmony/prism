# Token Limit Handoff

Checkpoint created on branch `token-limit`.

## Goal

Find token-related defaults that allow project or autonomous work to run without a finite token limit, replace those defaults with bounded behavior, and verify the behavior with tests.

## Completed In This Checkpoint

- Added a default aggregate token budget for `read_project` tool output in `internal/tool/builtins.go`.
- Added optional `max_tokens` input for `read_project` so callers can request a smaller output cap.
- Added token metadata to `read_project` output: `total_tokens`, `max_tokens`, `token_budget_exhausted`, and per-file `tokens` / `truncated`.
- Added `DefaultMaxTotalTokens` in `internal/workflow/v2/config.go`.
- Changed workflow config loading so omitted or zero `global.max_total_tokens` normalizes to `DefaultMaxTotalTokens`.
- Updated `DefaultConfig()` to use `DefaultMaxTotalTokens`.

## Not Yet Completed

- Add engine-level token normalization in `internal/workflow/v2/engine.go` so direct `NewEngine` callers cannot bypass loaded-config normalization.
- Add project-level token limit config, likely on `orchestrator.ProjectConfig`, and apply it when `WakeHandler.loadWorkflowConfig` / `RunGatedLoop` resolves a project workflow.
- Validate negative project token caps in `internal/orchestrator/config.go`.
- Update `prism.yaml.example` to document the project token-limit field.
- Update bundled workflows, especially `examples/workflows/natural-gates-default.yaml`, so examples do not imply unlimited tokens.
- Add regression tests for:
  - workflow config files with omitted or zero `max_total_tokens`;
  - project-specific token cap override;
  - `read_project` aggregate token truncation;
  - direct engine construction with zero `MaxTotalTokens`;
  - any changed sub-agent or runtrack defaults if those are bounded.
- Review other zero-token defaults:
  - `internal/subagent/runner.go` currently documents `MaxTokens` zero as unlimited;
  - `internal/runtrack/run.go` initializes `MaxTokens` to zero.

## Verification Status

No tests were run after this checkpoint because the work was interrupted. Run focused tests first:

```powershell
go test ./internal/tool ./internal/workflow/v2 ./internal/orchestrator
```

Then run the full suite:

```powershell
go test ./...
```

## Unrelated Working Tree Items

At checkpoint time, the working tree also showed:

- deleted `tmp_natspub/main.go`
- untracked `.claude/settings.local.json`

Those were not included in the token-limit checkpoint unless intentionally staged later.
