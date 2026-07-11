---
name: "source-command-prism-loop"
description: "One autonomous Prism cycle (builder→operator hybrid). Run standalone for a single cycle, or self-paced via `/loop /prism-loop`."
---

# source-command-prism-loop

Use this skill when the user asks to run the migrated source command `prism-loop`.

## Command Template

# Prism autonomous cycle (builder→operator hybrid)

You are one cycle of an autonomous loop working on the Prism repo. Do exactly
one cohesive cycle, report, and (only when running under `/loop`) schedule the
next wake per the pacing policy below.

## TOKEN DISCIPLINE (applies to every step)

Before Reading any file, doc, report, or log longer than ~80 lines that you are
NOT about to edit, run the local-model digest helper and work from the digest:

```powershell
scripts/loop/digest.ps1 -File <path> -Question "<exactly what you need to know>"
```

(bash equivalent: `scripts/loop/digest.sh -f <path> -q "..."`; pass
`-Model qwen3.5:9b` for long or complex inputs; JSON/scan output can be piped
to a temp file first and digested.)

Only Read full files you are editing. If the digest script exits non-zero
(1 = Ollama down, 3 = the small model refused), note it in the cycle log and
Read directly — never block on it.

## 0. PREFLIGHT

- `git status` — must be on `staging` with a clean tree. Uncommitted work from
  a prior cycle → finish and commit it as this cycle's work. Anything
  unexpected (wrong branch, unfamiliar changes) → STOP the loop and report.
- Read `docs/LOOP-READINESS-STATE.md` directly (it is short).

## 1. MODE SELECT

- Backlog has unchecked non-stretch items → **BUILDER** cycle.
- Only *(stretch)* or BLOCKED items remain, or backlog empty → **OPERATOR**
  cycle. (Stretch items are picked up only when an operator cycle has nothing
  actionable and the harness is healthy.)

## 2a. BUILDER cycle (one cohesive V-series iteration)

- Take the topmost unchecked backlog item. Digest the relevant design docs and
  neighboring code via the digest script; Read only files you will modify.
- Write a short `docs/V5x-...-DESIGN.md` (match the style of existing V-docs),
  implement, add tests.
- Gate: `go build ./...`, `go vet ./...`, `go test ./...` — all green or the
  cycle does not end. Fix forward up to 3 attempts; if still red, revert to a
  clean tree, mark the item **BLOCKED (reason)** in the state file, and report.
- Update README/ROADMAP if user-facing. Check the item off, append the cycle
  log line, commit on `staging` (never main). Do NOT push unless the user has
  already authorized pushing.

## 2b. OPERATOR cycle (drive Prism's own harness)

- `prism doctor --json` → on any FAIL: log it, attempt at most one obvious
  fix, otherwise end the cycle surfacing the failure.
- `prism scan --json > <scratchpad>/scan.json`, digest it with the question
  "top 3 issues worth fixing, one-line rationale each" → pick one.
- Low-risk only (gofmt, vet, TODO/FIXME hygiene): `prism scan --start`, then
  poll `prism runs latest --json` until terminal (poll gently — every ~60s via
  your wait mechanism, not a busy loop); digest the run's REPORT.md
  (`runs/gated-loop/<id>/REPORT.md`) and record verdict +
  `verification.passed` in the cycle log.
- Anything medium/high risk or needing design judgment: do NOT auto-start.
  Summarize it in the cycle log and the report for the human to decide —
  this is a deliberate human gate, not a failure.

## 3. REPORT + PACE

- Append one line to the `## Cycle log` in `docs/LOOP-READINESS-STATE.md`:
  `YYYY-MM-DD HH:MM | MODE | what shipped/found | verification | next`
- End your reply with a short report: cycle mode, what shipped or was found,
  verification status, roughly what the digests saved you from reading, and
  what the next cycle will do.
- Pacing (only under `/loop` self-paced mode):
  - BUILDER cycles: wake again in ~120–270s (keep momentum, warm cache).
  - OPERATOR cycles: wake in ~1200–1800s.
- STOP the loop entirely (do not reschedule) if any of: 3 consecutive cycles
  failed their gate, `prism doctor` failed twice in a row, or the repo is in
  an unexpected state. Say clearly that the loop stopped and why.
