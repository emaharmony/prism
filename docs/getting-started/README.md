# Getting started

The canonical guide remains [Getting Started](../GETTING_STARTED.md) while
links are migrated into the categorized documentation layout.

The model-free golden path is:

```bash
go test ./...
go run ./cmd/prism-cli doctor --json
go run ./cmd/prism-cli workflow run demo.echo_tool
```

The echo workflow requires no API key, model, external NATS server, database,
or Python service. It writes inspectable events and summary artifacts beneath
`runs/`.
