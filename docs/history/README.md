# Versions, milestones, and schemas

Prism uses three independent version concepts:

- **Semantic release versions** identify distributed artifacts and are printed
  by `prism version`. They are the only versions with compatibility meaning.
- **V-series milestones** (V1, V13, V58, and similar) are design-history labels.
  They are not semantic versions and do not imply that every subsystem shares
  one stability level.
- **Schema versions** belong to an individual event, workflow, adapter, API, or
  persisted-artifact contract. A schema changes only under that contract's
  compatibility rules; it does not advance automatically with a milestone.

No release tag is currently present in the verified checkout. The CLI reports
`v0.24.0`, while the changelog and Python packages currently describe `0.1.0`.
That mismatch must be resolved by the maintainer before a public release; this
documentation does not invent a canonical replacement.

Historical milestone documents remain at `docs/V*-DESIGN.md` and are indexed
in [Version History](../VERSION_HISTORY.md).
