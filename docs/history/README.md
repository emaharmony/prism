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

The canonical semantic version is `v0.1.0`. It is stored in the root `VERSION`
contract and shared by the CLI, changelog, Prism SDK, and Remembrance package.
Release builds embed the exact tag version. No release tag is present in this
checkout; creating one remains a maintainer action.

Historical milestone documents remain at `docs/V*-DESIGN.md` and are indexed
in [Version History](../VERSION_HISTORY.md).
