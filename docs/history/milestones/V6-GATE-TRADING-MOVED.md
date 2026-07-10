# V6 — Gate System + Trading Domain

## Note

V6 (Gate System + Trading Domain) was moved to the **AI-Hedge-Prism** repository
and is not implemented in the core Prism repo.

The gate system introduced domain-specific concepts (risk gates, position sizing,
market data evaluation) that belong in a trading domain layer, not the
domain-agnostic Prism core. Prism remains a general-purpose agent runtime;
domain logic lives in separate repositories that use Prism as a dependency.

The V7 workflow runtime (commit `781abd4`) includes a `gate.evaluate` step type
with function-based `GateEvaluateFunc` callback — keeping Prism domain-agnostic
while allowing domain repos to provide their own gate implementations.

For the trading domain implementation, refer to the AI-Hedge-Prism repository.
