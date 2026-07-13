# Repository hygiene

## Source versus runtime

Source includes Go/Python code, policies, configuration examples, workflows,
documentation, and sanitized samples. Runtime state includes `runs/`,
`prism-data/`, `.prism/`, local `prism.yaml`, workspaces, databases, logs,
coverage files, reports, caches, and built binaries.

The tracked-file audit found no tracked executables, databases, logs, or real
run output. `runs/.gitkeep` retains the directory and `examples/runs/**`
contains intentionally sanitized examples. Root `.exe` files and local runtime
directories visible in this checkout are ignored and were not deleted because
they are user-owned state.

## Controls

- `.gitignore` excludes runtime state, secrets, binaries, coverage, quality
  reports, and test reports.
- `.gitattributes` pins source and documentation to LF, PowerShell to CRLF, and
  marks image/binary formats as binary.
- Examples belong under `examples/`; reusable developer utilities belong under
  `scripts/` or `tools/`.
- Historical milestones belong under `docs/history/` over time. Existing files
  stay in place until links can be migrated without breakage.

Before committing, run `git status --short`, the quality report, and a tracked
artifact audit such as `git ls-files '*.exe' '*.db' '*.log'`.
