# V48 — Actions on Flows (Visual Workflow Editor)

**Status:** Source-current
**Last Updated:** 2026-06-29

## Goal

Make the visual workflow editor a *behavior* diagram, not just a topology picture:
boxes are systems/agents, arrows are the flows between them, and **each flow can
carry an action** — the thing that happens when one agent communicates with
another along that line. (Draw.io-style: systems as boxes, flows as arrows, actions
assigned to the lines.)

## What already existed

`internal/editor` is a full node/edge model with a draggable canvas
(`dashboard/static/editor.html`): agent boxes, SVG arrows colored/dashed by edge
type (delegation/review/approval/event), an Edge connect tool, and a REST API
(`/api/v1/editor/...`) that round-trips to/from agent config + YAML. The missing
piece was that edges had `type/label/style` but no action.

## Change

### Model (`internal/editor/model.go`)

- `EditorEdge.Action string` — the action invoked along the flow (a registered
  action key like `remembrance.gate.extract`, or a free label).
- `EdgeUpdate{Label,Type,Style,Action *string}` + `EditorState.UpdateEdge(id, upd)`
  — a partial update so the editor can change just the action of a flow; a changed
  `Type` is validated against the known edge types; `Action` is trimmed.

### API (`internal/api/server.go`)

- `PUT /api/v1/editor/edges/{id}` → `updateEdge`, applying an `EdgeUpdate`. (The
  edge CRUD handler previously supported only DELETE.) The `Action` field flows
  through the existing GET/POST automatically via JSON.

### Canvas (`dashboard/static/editor.html`)

- Each flow gets a transparent thick **hit-line** so it's clickable; clicking opens
  `editEdgeAction`, which prompts for the action and `PUT`s it.
- An assigned action renders as a `⚡ <action>` badge on the flow, colored to match
  the edge type, and is itself clickable to edit.

## Tests

`internal/editor/model_test.go`: `TestEditorEdgeActionRoundTrip` (action stored on
add) and `TestUpdateEdge` (set/trim action, partial update preserves action, change
type+label, invalid type rejected, clear action, unknown edge errors). The canvas JS
is covered by build; the model + API logic it drives is unit-tested.

## Follow-ups

- A picker that lists the project's registered action keys (from config
  `state_actions`) instead of a free-text prompt.
- Drag-to-connect for creating edges directly on the canvas (today the Edge tool +
  two clicks); per-edge type switching from the canvas.
- Persisting edge actions into the generated workflow/agent YAML so the diagram is
  executable, not just descriptive.
