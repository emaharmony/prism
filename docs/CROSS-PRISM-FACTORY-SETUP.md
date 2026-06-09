# Cross-Prism Factory Setup

This setup connects two Prism environments over a shared NATS broker without relying on Discord as the transport.

## Roles

- `lumi-ceo`: Mac-side CEO Prism.
- `astraea-manager`: Windows-side manager Prism.
- `forge`: Windows-side coder/Factory support agent.
- NATS broker: Windows host, default `nats://127.0.0.1:4222`.
- Factory target: `D:/Projects/Roblox/eggventura`.

## Required Environment

Set these on both Prism environments:

```powershell
$env:PRISM_BRIDGE_SECRET="use-a-long-random-shared-secret"
```

Set this on the Windows Prism if Discord is enabled:

```powershell
$env:DISCORD_BOT_TOKEN="..."
```

Set this only for agents using `provider: openai_responses` or `provider: openai`:

```powershell
$env:OPENAI_API_KEY="..."
```

OpenAI ChatGPT subscriptions and OpenAI API platform billing are separate. Prism can use OpenAI API models through `OPENAI_API_KEY`; it cannot spend a ChatGPT web subscription directly as an API model entitlement. Official references:

- https://help.openai.com/en/articles/9039756-managing-billing-settings-on-chatgpt-web-and-platform
- https://platform.openai.com/docs/api-reference/responses/create

## Protocol Subjects

Only these signed protocol subjects should be enabled:

- `prism.cross.context_sync`
- `prism.cross.task_request`
- `prism.cross.status_request`
- `prism.cross.validation_request`
- `prism.cross.task_response`

Each message includes `from`, `to`, `message_type`, `correlation_id`, timestamp, nonce, and an HMAC-SHA256 signature. Messages not addressed to the local `prism.instance_id` are ignored.

## Factory Handoff

The Windows Prism accepts signed `task_request` messages addressed to `astraea-manager`. With the current config it writes:

- Request markdown: `D:/_projects_/roblox-factory/prompts/prism-<correlation>.md`
- Queue task JSON: `D:/_projects_/roblox-factory/inbox/prism-<correlation>.json`

Current milestone defaults:

- `approval_mode: report_only`
- `run_codex: false`
- `vision_review: none`
- `playtest_mode: none`

Implementation and Codex execution stay off until explicitly enabled.

## OpenAI Model Option

Use this per-agent config when API billing is configured:

```yaml
provider: openai_responses
model: "gpt-5.1"
```

The existing `provider: openai` path remains available for Chat Completions-compatible behavior.
