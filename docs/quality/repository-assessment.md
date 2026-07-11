# Repository assessment

Assessment date: 2026-07-11. Values below were measured from this checkout on
Windows/amd64 with Go 1.26.4. They are a baseline, not a permanent badge.

## Inventory and baseline

| Item | Verified value |
|---|---|
| Languages | Go, Python, shell, PowerShell, HTML/CSS/JavaScript, YAML |
| Tracked files | 605 before this upgrade |
| Go packages | 81 |
| Go test files / test functions | 182 / 1,581 |
| Integration test files / tests | 6 / 17 |
| Go benchmarks | 4 (vector HNSW/brute-force) |
| Python tests | 6 (Remembrance); no SDK tests found |
| CLI binaries | `prism-cli`, `prism-bus`, `prism-agent` |
| Services | Prism serve/API/dashboard, embedded NATS, optional Remembrance |
| CI before upgrade | One Ubuntu job: vet, build, test, race on 3 package trees |
| Race-tested packages before upgrade | `internal/stage/...`, `internal/session/...`, `internal/safety/...` |
| Supported OS evidenced before upgrade | Linux CI; Windows instructions and code/tests; Darwin build target only |
| Semantic version evidence | CLI `v0.24.0`; changelog/Python `0.1.0`; no git tags |
| Stability | Public preview; not production-ready |
| Release process before upgrade | No release workflow or tags found |
| Tracked runtime/generated files | `runs/.gitkeep` and sanitized `examples/runs/**`; no tracked binaries/databases/logs |

Build commands are `go build ./...` and Makefile targets. Test commands are
`go test ./... -count=1`, integration tests through the full suite, and
Remembrance `pytest`. Linting was `go vet ./...`; staticcheck, govulncheck,
Python Ruff, markdown lint, and link checking were not in CI.

Baseline execution: vet and build passed. The full test suite passed except for
symlink tests that ignored `os.Symlink` creation errors on Windows; those tests
were corrected to skip only when the host cannot create a symlink. Full local
race testing was blocked because CGO had no C compiler. Python tests were
blocked because Python was not installed. The model-free echo workflow and
`doctor --json` passed. Top-level `--help` printed help but exited 1.

## Risk assessment

### Critical

- Policy/approval/mutation bypass: Core controls exist and are extensively
  tested, but any registry wiring omission could bypass them. Keep deny-by-
  default integration tests and never permit model self-approval.

### High

- Concurrency, NATS lifecycle, SQLite contention, event persistence, scheduler
  delivery, and goroutine cleanup lack published stress/SLO evidence.
- Cross-agent delegation and worktree isolation span filesystem, git, process,
  and provider boundaries; keep them experimental and bounded.
- Windows path and symlink behavior differs by privilege; CI must exercise both
  ordinary containment and symlink tests where the runner permits.

### Medium

- Configuration has a large surface with multiple precedence paths.
- External provider, MCP, Discord, Remembrance, Factory, Firecrawl, and CLI
  integrations are coupled to changing external contracts.
- API and event schemas have no published compatibility policy before 1.0.
- Documentation showed version and CLI-help drift.

### Low

- Generated/runtime output is already ignored, but root binaries and local
  workspaces can remain present and confuse operators.
- Historical V-series documents crowded the living documentation namespace.

## Baseline scorecard

| Dimension | Score | Rationale |
|---|---:|---|
| Architecture | 9.0 | Strong event/policy design; layers and trust boundaries were implicit |
| Safety | 9.1 | Central containment and approval gates; cross-platform test premise was brittle |
| Testing | 9.2 | 1,581 Go tests and integration coverage; Python/race coverage not universally proven |
| CI | 7.4 | One Linux job with a narrow race subset |
| Documentation | 8.8 | Extensive but flat, historical, and partially drifting |
| Developer experience | 8.4 | Working echo path; help exit/drift and toolchain ambiguity |
| Repository hygiene | 8.9 | Runtime artifacts ignored; no `.gitattributes` or quality artifact rules |
| Maintainability | 8.8 | Focused packages; large CLI/config surfaces and milestone complexity |
| Release readiness | 6.8 | Version mismatch, no tags, no release workflow |
| External adoption | 7.6 | Preview/license limits, no cross-platform proof history |

Arithmetic mean: **8.4/10** for this verified baseline. The prompt's 8.7 estimate
is plausible, but this report does not fabricate a higher measured value.

## Upgrade evidence

The hardening checkout resolves the semantic-version mismatch at `v0.1.0` and
adds SDK tests, cross-platform release builds, immutable CI action pins, strict
local-link checks, and behavior-focused authority tests. Local Go evidence on
2026-07-11 measured 55.4% aggregate coverage, with safety 95.4%, policy 92.5%,
approval 85.6%, mutation 82.7%, validation 90.6%, guard 96.1%, and the combined
tool-authority files 86.3%. Vet, build, deterministic tests, CLI smokes, and
release-smoke archives remain part of the delivery verification. A 9.7+ final
score is withheld until the pull request supplies green Linux race/static and
Python 3.11 evidence.
