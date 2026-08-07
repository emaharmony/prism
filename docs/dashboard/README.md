# Prizm dashboard

Prizm serves its embedded dashboard from `prizm serve` at `http://127.0.0.1:8322/`.
It is deliberately framework-free: static HTML, shared CSS tokens in
`internal/dashboard/static/app.css`, and a small shared client in `app.js`.

## Pages

- Overview (`index.html`): live status, task queue, approvals, and agent roster.
- Operations (`v2.html`): detailed live system views.
- Agents, Workflow, Scheduler, and Settings: existing operational editors.

The dashboard consumes the existing `/api/v1` API only. Run `go test
./internal/dashboard` after UI changes; build or restart `prizm serve` to refresh
the embedded assets.
