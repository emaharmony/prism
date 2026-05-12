# Prism - Ownership and Creator Note

**Project:** Prism  
**Creator / Originator:** Emmanuel Vinas  
**Role:** Founder, Architect, and Primary System Designer  
**Repository:** `prism`  
**Project Type:** Event-native AI agent framework / agent runtime infrastructure  

---

## Creator Statement

Prism was designed and initiated by **Emmanuel Vinas** as an event-native AI agent framework for observable, memory-aware, policy-controlled agent workflows.

The original concept behind Prism is that AI agent systems should not rely only on prompt chains or request-response execution. Instead, every meaningful action in an agent workflow should become a canonical event that can be observed, subscribed to, audited, replayed, extended, and used to trigger specialized reactions.

Prism was created to explore and implement that idea from the ground up.

---

## Core Concept

Prism treats events as the core primitive of the framework.

A single task, message, decision, tool call, memory retrieval, failure, approval, or output becomes an event flowing through the Prism event bus. Other systems can then react to those events without needing to be tightly coupled to the original component.

In simple terms:

```text
One event enters Prism.
The event bus refracts it into many possible reactions:
- agent execution
- memory retrieval
- tool invocation
- logging
- validation
- notification
- audit trails
- future replay
```

This design is intended to make AI workflows more reliable, observable, modular, and safe.

---

## Technical Direction

Prism is being developed as a Go and Python framework.

The intended architecture separates runtime reliability from AI intelligence:

- **Go Runtime:** event manager, task lifecycle, agent lifecycle, embedded NATS JetStream, command-line execution, correlation IDs, durable event logs, and future orchestration features.
- **Python AI Layer:** agent SDK, memory intelligence, Remembrance vector memory, embedding workflows, context building, and future model/provider integrations.

The framework is designed around canonical event schemas, correlation chains, parent event links, durable JSONL run artifacts, and memory-aware task execution.

---

## Related System: Remembrance

Remembrance is the memory layer designed for Prism.

Its purpose is to help agents retrieve relevant project context before work begins. The planned and prototyped direction includes:

- vector memory search
- LanceDB for semantic retrieval
- SQLite for metadata and audit records
- Ollama embeddings
- hybrid ranking
- context-pack generation
- project-scoped memory
- future memory consolidation and evaluation

Remembrance is intended to make Prism agents more context-aware without flooding model prompts with irrelevant history.

---

## Current V1 Foundation

The first stabilized version of Prism proved the core event lifecycle:

```text
task input
→ canonical event
→ NATS JetStream flow
→ agent trigger
→ structured output
→ durable run artifacts
→ events.jsonl and summary.json
```

The V1 foundation includes:

- `prism run --task "..."` CLI lifecycle
- canonical event package
- correlation ID propagation
- parent event chains
- Remembrance context hook with graceful fallback
- strict memory-required failure mode
- durable event persistence
- health checks
- integration tests using real embedded NATS

---

## Design Values

Prism is guided by the following design principles:

1. **Events over hidden side effects** - important actions should be observable.
2. **Framework controls the model** - models generate outputs, but the runtime controls lifecycle and policy.
3. **Memory should be explicit and auditable** - retrieved context should be traceable.
4. **Modular reactions over hardcoded workflows** - subscribers and hooks should extend behavior cleanly.
5. **Local-first, company-ready later** - the framework should work locally first but be designed for future team and enterprise use.
6. **Safety and auditability are core features** - tool calls, memory writes, and agent actions should be traceable.
7. **Open core philosophy** - the core framework should remain accessible while future hosted, managed, or enterprise services may provide revenue opportunities.

---

## Attribution

Prism was conceived, directed, and architected by **Emmanuel Vinas**.

Any future documentation, public release, derivative work, funding pitch, portfolio material, or resume reference should preserve attribution to Emmanuel Vinas as the originator and primary system designer of the Prism framework concept and implementation direction.

---

## Resume Summary

**Prism - Event-Native AI Agent Framework**  
Designed and implemented a Go/Python event-driven AI agent runtime using embedded NATS JetStream, canonical event schemas, lifecycle orchestration, Remembrance vector memory concepts, and durable JSONL audit trails to make AI workflows observable, replayable, memory-aware, and policy-controlled.

---

## Creator

**Emmanuel Vinas**  
AI / Agentic Systems Engineer  
Senior Software Engineer  
Backend, Workflow Automation, and Platform Architecture
