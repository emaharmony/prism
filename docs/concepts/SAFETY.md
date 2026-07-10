# Prism Safety Model

Prism is designed so that the framework — not the model — controls what actually
happens. Models generate outputs and request actions; the runtime decides
whether those actions run. This page explains the human-in-the-loop model and
the boundaries you should rely on.

> See also: [SECURITY.md](../SECURITY.md) for reporting and known risk areas,
> and [Capability Status](CAPABILITY_STATUS.md) for stable vs experimental
> surface.

## Core Principles

- **The framework controls the lifecycle.** Policy, approvals, and validators
  gate every action. A clever prompt cannot talk the runtime into a denied
  action.
- **Policy is not the same as validation.** The policy engine decides
  *permission* (allow / deny / requires approval) deterministically, with no LLM
  involved. Local validators separately enforce *input safety* (paths,
  allowlists, sizes). Both must pass.
- **LLMs cannot approve their own mutations.** File mutations require an
  approval gate resolved by a human (or an explicitly configured reviewer),
  never by the acting model.
- **Local-first and explicit.** External integrations are opt-in and disabled by
  default. The HTTP API binds to loopback unless you set an auth token.

## Secrets and Artifacts

- **Never commit secrets.** Do not commit `.env`, API keys, tokens, broker or
  Discord credentials. Prefer environment variables
  (e.g. `${DISCORD_BOT_TOKEN}`, `api.auth_token_env`, `bridge.secret_env`).
- **Run artifacts may contain sensitive data.** Files under `runs/<run_id>/`
  (prompts, outputs, event logs) can include private context. They are
  gitignored by default; only sanitized examples under `examples/runs/` are
  tracked. Do not commit real run artifacts.

## Self-Patching / Autopatch

Autopatch is **experimental** and disabled by default.

- Prefer `propose` mode, which produces a patch artifact for review and never
  touches your repository.
- `pr` mode opens a pull request via the `gh` CLI — it should not auto-merge or
  auto-apply without human review.
- Keep `require_clean_worktree: true` and do not run autopatch on sensitive
  repositories without explicit consent.
- Fixes are authored in isolated git worktrees and gated by validation profiles.

## Explicitly Out of Scope

- **Live trading / financial execution** is not supported. Trading-style
  adapters are report-only examples, never live order execution.
- **Unattended high-risk automation** and **enterprise multi-user deployment**
  are not production-ready.

## No False Guarantees

Prism reduces risk through layered controls; it does not make automation
"fully safe" or "fully autonomous." Review experimental features before enabling
them, and keep a human in the loop for any mutation or high-impact action.
