# Prism Documentation

The documentation hub for Prism. Start here to find the right guide, reference, or
design note. New here? Read [Getting Started](GETTING_STARTED.md) first.

> Prism is source-available under an all-rights-reserved license (see
> [LICENSE](../LICENSE)) and is a **preview-stage** project — some features are
> experimental. See [Capability Status](CAPABILITY_STATUS.md) before relying on any
> capability.

## Guides

Practical, task-oriented docs for getting things done.

| Doc | What it covers |
|---|---|
| [Getting Started](GETTING_STARTED.md) | Install, build, test, and run your first workflow. |
| [Configuration](CONFIGURATION.md) | Where config files live and how Prism loads them. |
| [Commands](COMMANDS.md) | The CLI command surface and what each does. |
| [Examples](EXAMPLES.md) | Guided demo flows. |
| [Scheduler](SCHEDULER.md) | Cron jobs and scheduled wake actions. |
| [Windows Setup](WINDOWS_SETUP.md) | Windows-specific build and run notes. |
| [Troubleshooting](TROUBLESHOOTING.md) | Common setup and runtime issues. |
| [Onboarding](ONBOARDING.md) | Developer onboarding for contributors. |

## Reference

Look-up material: schemas, status, and safety.

| Doc | What it covers |
|---|---|
| [YAML Reference](YAML_REFERENCE.md) | Workflow, policy, adapter, provider, and agent YAML. |
| [Capability Status](CAPABILITY_STATUS.md) | Stable vs. experimental features, and safe defaults. |
| [Safety Model](SAFETY.md) | Human-in-the-loop, policy vs. validators, autopatch risks. |
| [Event Manual](EVENT-MANUAL.md) | The canonical event types and how the bus is used. |

## Architecture & Design

How Prism is built and why.

| Doc | What it covers |
|---|---|
| [Architecture](ARCHITECTURE.md) | A concise map of how Prism works internally. |
| [System Design](DESIGN.md) | The full design rationale. |
| [Natural Gates Workflow](natural-gates-workflow.md) | The gated-loop (V2) workflow that drives autonomous runs. |
| [Vision](PRISM-VISION.md) | The long-term product vision. |
| [Roadmap](ROADMAP.md) | Project direction and priorities. |
| [Version History](VERSION_HISTORY.md) | The annotated V1–V59 development story (links every design note). |
| [Public Preview Checklist](PUBLIC_PREVIEW_CHECKLIST.md) | Release-prep status. |

## Design Archive

Per-version design notes (`V1`–`V59`), planning reviews, and historical handoffs
live in **[`design/`](design/README.md)**. They record how the system was built and
are kept for context, not as current user guides. [Version History](VERSION_HISTORY.md)
is the best annotated index into the V-series.

## Contributing to the Docs

New and edited docs follow the conventions in
[DOCUMENTATION_STANDARD.md](DOCUMENTATION_STANDARD.md).

## See Also

- [Project README](../README.md)
- [Contributing](../CONTRIBUTING.md)
- [Changelog](../CHANGELOG.md)
- [Security Policy](../SECURITY.md)
