# Token Limit Work Notes

Branch: `token-limit`

## Goal

Find token-related defaults that allowed project or autonomous work to run without a finite token limit, replace those defaults with bounded behavior, and verify the behavior with tests.

## Completed

- `read_project` now has a default aggregate output budget of `50_000` estimated tokens.
- `read_project` accepts optional `max_tokens` for smaller caller-specified caps.
- `read_project` reports token metadata: `total_tokens`, `max_tokens`, `token_budget_exhausted`, and per-file `tokens` / `truncated`.
- Workflow configs normalize omitted or zero `global.max_total_tokens` to `DefaultMaxTotalTokens` (`2_000_000`).
- `v2.NewEngine` also normalizes token budgets, so direct engine callers do not bypass config-load normalization.
- Projects now support `token_budget`, which overrides the workflow run cap for that project.
- Negative project token budgets fail config validation.
- Delegated sub-agent loops now default to `DefaultMaxTokens` (`100_000`) instead of unlimited.
- Run tracking now records `DefaultMaxTokens` (`4096`) instead of zero/unlimited for new runs.
- `examples/workflows/natural-gates-default.yaml` and `prism.yaml.example` document finite caps.

## Regression Coverage

Added or updated tests for:

- workflow config files with omitted or zero `max_total_tokens`;
- direct `NewEngine` construction with zero `MaxTotalTokens`;
- project-level token budget overrides;
- negative project token budget validation;
- `read_project` aggregate token truncation;
- sub-agent default token budget;
- runtrack default token budget.

## Verification

Focused tests passed:

```powershell
go test ./internal/tool ./internal/workflow/v2 ./internal/orchestrator ./internal/subagent ./internal/runtrack ./cmd/prism-cli
```

Full suite passed:

```powershell
go test ./...
```

## Unrelated Working Tree Items

At the time this branch work continued, the working tree also had untracked local files:

- `.claude/settings.local.json`

Do not stage that file unless it is intentionally part of a separate change.
