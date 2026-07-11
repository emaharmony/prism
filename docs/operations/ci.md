# Continuous integration

CI separates concerns so a platform-specific failure is visible:

- Linux: formatting, vet, build, deterministic tests, full race tests,
  integration package tests, coverage, staticcheck, and govulncheck.
- Windows: build, tests, config validation, doctor, CLI help, and the model-free
  echo workflow.
- Python: editable installs for Remembrance and the SDK, pytest, and Ruff.
- Documentation: actionlint, markdown lint, and deterministic local-link
  validation on every pull request.
- External links: a separate weekly/manual workflow uses caching and retries so
  transient websites do not make pull-request validation nondeterministic.
- Quality artifacts: Go and race JSON, pytest JUnit, coverage profiles,
  benchmarks, release-smoke archives, and generated quality reports.

The full race job requires CGO and a C compiler. `govulncheck` may require
access to the Go vulnerability database. CI failures are not waived silently;
document external outages and rerun.

Local equivalents are in [QUALITY.md](../../QUALITY.md). Windows users may need
`powershell -ExecutionPolicy Bypass -File scripts/quality-report.ps1` when the
machine blocks local scripts.
