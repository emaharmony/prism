# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Cross-platform coverage gates, release-build smoke tests, SDK contract tests,
  immutable CI action pins, and deterministic local-link validation.

### Changed

- Centralized the development version at `v0.1.0`; release builds embed and
  validate the exact semantic-version tag.

### Fixed

- Rejected approval decisions without an actor, preserved correlation safety in
  SDK tool responses, and corrected the Python SDK build backend.

## [0.1.0] - 2026-07-07

### Added

- Idle guard for scheduled tasks — `hasWorkToDo()` checks locally before calling
  cloud LLM, achieving ~93 % token reduction on idle runs
  ([29fed19](https://github.com/emaharmony/prism/commit/29fed19)).
- Per-skill work detection: `project_work` inspects `PROJECT_STATE.md` and
  `git status`; `auto_patch` checks failing tests, open PRs, and Remembrance
  tasks; `review_improvements` checks for active proposals
  ([29fed19](https://github.com/emaharmony/prism/commit/29fed19)).

### Fixed

- `collect_reference_images` now works with Firecrawl — added
  `POST /v2/search` branch with `sources:["images"]` and hardened
  `downloadImage` to validate content-type and magic bytes, rejecting anti-bot
  HTML responses
  ([2eb8d28](https://github.com/emaharmony/prism/commit/2eb8d28)).

### Changed

- Schedule intervals tuned: `project_work` 10 → 15 min, `pr-check` 10 → 30 min,
  `auto_patch` 4 → 6 h
  ([29fed19](https://github.com/emaharmony/prism/commit/29fed19)).
- README updated with sub-agent delegation, worktree isolation, skill-use, and
  Claude Code provider documentation
  ([9010cf8](https://github.com/emaharmony/prism/commit/9010cf8)).

### Removed

- Outdated `checksum_test.go` (replaced by current test coverage)
  ([9010cf8](https://github.com/emaharmony/prism/commit/9010cf8)).

[0.1.0]: https://github.com/emaharmony/prism/commits/HEAD
[Unreleased]: https://github.com/emaharmony/prism/compare/v0.1.0...HEAD
