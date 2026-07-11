# Continuous integration

CI separates concerns so a platform-specific failure is visible:

- Linux: formatting, vet, build, deterministic tests, full race tests,
  integration package tests, coverage, staticcheck, and govulncheck.
- Windows: build, tests, config validation, doctor, CLI help, and the model-free
  echo workflow.
- Python: editable installs for Remembrance and the SDK, pytest, and Ruff.
- Documentation: markdown lint and local/external link validation.
- Quality artifact: the generated report and Go coverage profile are uploaded.

The full race job requires CGO and a C compiler. `govulncheck` may require
access to the Go vulnerability database. CI failures are not waived silently;
document external outages and rerun.

Local equivalents are in [QUALITY.md](../../QUALITY.md). Windows users may need
`powershell -ExecutionPolicy Bypass -File scripts/quality-report.ps1` when the
machine blocks local scripts.
