# Prism Public Preview Checklist

A realistic checklist for presenting Prism as a credible public-preview project.
This tracks presentation and hygiene, not production readiness.

## Documentation

- [x] README is current and concise (status, golden path, capability matrix)
- [x] Version history moved out of README to `docs/VERSION_HISTORY.md`
- [x] Getting Started, Configuration, YAML Reference, Commands, Examples,
      Troubleshooting docs exist
- [x] Architecture doc exists (`docs/ARCHITECTURE.md`)
- [x] Safety doc exists (`docs/SAFETY.md`)
- [x] Capability status matrix exists (`docs/CAPABILITY_STATUS.md`)
- [x] Roadmap grouped into completed / current / experimental / planned

## Consistency & Claims

- [x] License language consistent (source-available / all-rights-reserved)
- [x] Prism is not called "open source"
- [x] No production-readiness claims
- [x] Stable vs experimental boundaries are explicit

## Repo Hygiene

- [x] No IDE folders tracked (`.idea/`, `.vscode/`)
- [x] No local workspace state tracked (`prism-workspace/`)
- [x] No generated `runs/` artifacts tracked (only sanitized `examples/runs/`)
- [x] Scratch/temp helpers untracked and gitignored
- [x] No secrets committed
- [x] `.gitignore` protects future local/generated clutter

## Demo & Tests

- [x] Golden-path demo works (echo workflow → events → artifacts)
- [ ] `go test ./...` passes on the target machine
- [ ] Dashboard screenshots / GIFs added (see `docs/assets/`)
- [ ] Tag a release candidate (e.g. `v0.1.0-public-preview`) — do not tag
      automatically; tag when the owner decides

> Unchecked items are follow-ups a maintainer should complete/verify locally.
> Test runs and release tagging are intentionally left to the owner.
