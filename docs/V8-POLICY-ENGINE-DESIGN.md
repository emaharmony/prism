# V8 — Core Policy Engine

**Status:** Merged (PR #14)
**Date:** 2026-05-11

## What Changed

V8 centralizes all policy decisions across Prism. Instead of each component deciding what's allowed, a single deterministic policy engine evaluates rules and emits consistent allow/deny/require_approval decisions.

### Key Components

- **Policy Registry** (`internal/policy/registry.go`) — Stores named policy rules. Policies are loaded from YAML files (`policies/default.yaml`).
- **Policy Evaluator** (`internal/policy/evaluator.go`) — Evaluates rules against a request context. Returns allow, deny, or require_approval.
- **Policy Rules** (`internal/policy/rule.go`) — Declarative rules: `action`, `resource`, `effect` (allow/deny/require_approval), `condition` (optional).
- **Policy Events** — `prism.policy.requested`, `prism.policy.evaluated`, `prism.policy.allowed`, `prism.policy.denied`, `prism.policy.approval_required`, `prism.policy.failed`
- **Policy Request** (`internal/policy/request.go`) — Structured request with action, resource, subject, and context.
- **Policy Artifacts** (`internal/policy/artifacts.go`) — Written to `runs/<run_id>/policies/` for audit.

### Design Decisions

1. **Deterministic only** — No LLM-based risk assessment. Every decision is traceable to a specific rule.
2. **YAML-driven** — Policies are loaded from `policies/default.yaml`, not hardcoded.
3. **Three effects** — `allow`, `deny`, `require_approval`. No ambiguous "maybe."
4. **Rule ordering matters** — First matching rule wins. This is intentional and documented.
5. **Default deny** — If no rule matches, the action is denied.

### Policy Rule Format

```yaml
rules:
  - action: "tool.execute"
    resource: "read_file"
    effect: "allow"

  - action: "tool.execute"
    resource: "write_file"
    effect: "require_approval"

  - action: "tool.execute"
    resource: "delete_file"
    effect: "deny"
```

### CLI Integration

```bash
prism policy list              # List all loaded rules
prism policy evaluate <action> <resource>  # Test a rule
```

### Test Coverage

- Policy evaluation: allow, deny, require_approval
- Rule matching: first-match wins
- Default deny: unmatched actions denied
- YAML loading: valid and invalid configs
- Event emission: all 6 policy events