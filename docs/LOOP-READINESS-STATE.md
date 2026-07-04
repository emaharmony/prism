# Loop Readiness State

Cross-cycle state file for the `/prism-loop` autonomous cycle (see
`.claude/commands/prism-loop.md`). The loop reads this file at the start of
every cycle, works the topmost unchecked backlog item (BUILDER mode), and
switches to driving Prism's own harness (OPERATOR mode) once the backlog is
empty. Appends one line to the cycle log per cycle.

Origin: Lumi's "Loop Engineering: Core Concepts & Prism Readiness" report
(2026-07-04). Of its six gaps, four were already closed on `staging`
(independent verifier V35, token budgets/retry V35, SKILL.md skills V54,
MCP client V49). The backlog below is what remains.

## Backlog (ordered — take the topmost unchecked item)

- [ ] **V56 — Gated-loop worktree isolation**: per-run `git worktree` for the
  wake-handler gated loop, reusing `internal/autopatch/autopatch.go:createWorktree`
  / `removeWorktree`; config flag `project.worktree_isolation` (default off).
  Today only autopatch is worktree-isolated; parallel gated-loop runs in one
  repo share the filesystem.
- [ ] **V57 — Automated rollback**: when gated-loop verification is still
  failing after attempts/budget are exhausted, auto `git revert` (or discard
  the run branch) behind config flag `workflow.auto_rollback` (default off).
  PR mode remains the human safety net. Today the loop only fixes forward
  (`internal/workflow/v2/driver.go`).
- [ ] **Wiring fix — `prism.ollama_url` in serve mode**:
  `createOllamaProvider` (`cmd/prism-cli/cmd_serve.go:~1736`) constructs
  `ollama.New("")`, always hitting the default `localhost:11434`. Thread the
  configured `prism.ollama_url` through (the one-shot `prism run --ollama-url`
  path and vector search already honor it).
- [ ] **Per-phase token budgets**: V35 follow-up in
  `internal/workflow/v2/config.go` — only a per-run ceiling
  (`MaxTotalTokens`) exists today; add optional per-phase caps.
- [ ] *(stretch)* **Enforce per-skill `allowed-tools`**: advisory-only in the
  prompt today (`internal/skill/`, V54 follow-up).
- [ ] *(stretch)* **MCP streamable-HTTP transport**: client is stdio-only
  (`internal/tool/mcp/stdio.go`, V49 follow-up).

## Cycle log

<!-- One line per cycle, appended by the loop. Format:
YYYY-MM-DD HH:MM | BUILDER|OPERATOR | what shipped / was found | verification | next -->
