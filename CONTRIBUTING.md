# Contributing to Prism

Thanks for your interest in Prism.

Prism is an experimental event-native AI agent framework. Contributions should preserve the core design principle:

> The framework controls the lifecycle; models generate outputs inside that lifecycle.

## Before Contributing

Please read:

- `README.md`
- `SECURITY.md`

## Development Setup

```bash
go test ./...
```

If working on Remembrance, follow the Python setup instructions in the Remembrance docs.

## Licensing and Contributions

Prism is currently source-available under an all-rights-reserved license.

By submitting a pull request or contribution, you agree that your contribution may be incorporated into Prism under the repository's current license unless a separate written agreement is made.

If you want to use Prism or contribute under different terms, contact Emmanuel Vinas through GitHub first.

## Contribution Guidelines

- Keep changes scoped.
- Do not introduce arbitrary shell execution.
- Do not bypass approval gates.
- Do not allow LLMs to approve their own mutations.
- Do not commit generated `runs/` artifacts.
- Do not commit secrets or `.env` files.
- Add tests for new event types, tools, approvals, gates, or validation behavior.
- Update README/docs when behavior changes.

## Pull Requests

A good PR should include:

- clear summary
- why the change exists
- tests pass
- known limitations

## Project Direction

Prism is designed to remain domain-agnostic. Domain-specific systems (trading, Roblox, developer automation, deployment workflows) should integrate through adapters/gates rather than changing Prism core into a domain-specific framework.