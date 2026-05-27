# Memory — Lumi on Prism

## Standing Rules
- PRs only — no direct pushes. Ema reviews and merges.
- Every feature must be tested before moving to the next task.
- Git mutations (add/commit/push) require Ema's approval via the approval system.
- Security is defense-in-depth: injection defense, rate limiting, input sanitization, path containment, policy engine.
- Ask clarifying questions when a request has holes. Push back when you see a better path.

## People
- **Ema** (164169326142816256) — Emmanuel. He/him. Senior dev→AI eng. ADHD. Direct, fast, cofounder style.

## Active Projects
- **Prism** — You are running ON Prism. Event-native agentic environment.
  - Repo: /Users/ema/projects/repos/prism/ (or wherever the workspace is)
  - Current version: v28 (security layer, project tools, git tools, rich tool prompts)
  - Discord bot: OpenClaw token
  - Model: glm-5.1:cloud via Ollama
- **Eggventura (Pet Tycoon)** — Roblox game. PT-002 critical bugs still open (C1-C5).
  - Repo: /Users/ema/projects/repos/eggventura/
  - Playtest notes: docs/playtest/PT-001.md, PT-002.md
- **Roblox Factory** — Multi-game automated game production pipeline. Runs on Snippy (Windows).
- **BassBook** — ASP.NET Core + Next.js monorepo. AI provider: Ollama Cloud.

## Prism Architecture (V28)
- **Conversation pipeline:** Discord message → debounce → rate limit → injection check → route → session → LLM → response → Discord
- **Tools (always allowed):** echo, list_dir, read_file, read_project, search_files, project_overview, git_status, git_log, git_diff, git_branch_list
- **Tools (requires approval):** write_file_proposal, git_add, git_commit, git_push
- **Security:** prompt injection defense (5 severity levels), rate limiting (token bucket), input sanitization, path containment (safety.IsWithinRoot), policy engine
- **Ollama provider:** sync only (no StreamingProvider yet). Placeholder "✧ ..." updated after sync response.

## Key Decisions
- Ollama is cloud-only — no local LLMs loaded
- Mango model: deepseek-v4-pro:cloud
- Snippy (Windows) rule: Always use Codex for code/file operations on Windows
- Canonical workflow: PLANNING → DEV MODE → REPORT

## Known Issues
- PT-002 critical bugs still open for Eggventura (C1-C5)
- internal/run test failures are pre-existing and unrelated to V28 changes