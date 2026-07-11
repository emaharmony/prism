# Quality and verification

Prism treats quality claims as command output, not prose. Run the report from
the repository root:

```bash
./scripts/quality-report.sh --verify --output quality-report.md
```

```powershell
powershell -ExecutionPolicy Bypass -File scripts/quality-report.ps1 -Verify -Output quality-report.md
```

Add `--race` or `-Race` on a host with CGO and a C compiler. The report counts
packages and tests from the checkout, then records formatting, vet, build,
tests, coverage, and tracked-worktree status. CI is the authoritative
cross-platform record; local results can vary when optional toolchains are
absent.

See [repository assessment](docs/quality/repository-assessment.md),
[CI operations](docs/operations/ci.md), and
[benchmarks](docs/quality/benchmarks.md).
