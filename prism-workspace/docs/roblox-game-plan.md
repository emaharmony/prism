# Roblox Game — Planning Phase

**Created:** 2026-06-05
**Status:** PLANNING — all key decisions made, high-level design next

---

## Decisions ✅

| # | Question | Answer |
|---|----------|--------|
| 1 | Which game? | **New game first**, Eggventura pickup later |
| 2 | Genre? | **Platformer + Tycoon hybrid** |
| 3 | Scope? | **Full release, phased:** V1 = prototype, V2 = published, V3+ = ongoing updates |
| 4 | Astraea's role? | **Factory Operator + Collaborator** — suggestions evaluated by Lumi+Mango, must score ≥ 8.8 |

---

## What We're Building

A **Platformer + Tycoon hybrid** Roblox game, from scratch to full release.

**Phased release:**
- **V1** — Prototype (playable core loop)
- **V2** — Published (on Roblox, game passes)
- **V3+** — Ongoing updates (content, features, events)

**The Loop:**
1. Lumi plans, designs, and directs (lead + final say)
2. Astraea/Forge executes on Snippy (Factory + Roblox Studio)
3. Results flow back to Lumi for review
4. Repeat until shipped

**Astraea's role:** Factory operator + collaborator. Her suggestions go through Lumi+Mango review and must score ≥ 8.8 before acceptance.

**Principle:** No code until the end goal is 100% clear and we have all needed information.

---

## Architecture

| Component | Machine | Role | Model |
|-----------|---------|------|-------|
| Lumi | Mac mini | Lead developer, orchestrator, final decisions | glm-5.1:cloud |
| Astraea | Windows (Snippy) | Factory operator, collaborator (input < Lumi) | qwen3.5:9b |
| Forge | Windows (Snippy) | Coder, implementer | deepseek-v4-pro:cloud |
| Codex | Windows (Snippy) | Code/file operations on Windows | gpt-5.5 |

**Communication:** Discord channel `1491622757409226783` (factory-loop)

---

## High-Level Design — NEXT SESSION

Platformer + tycoon specifics to be defined:
- Core mechanic (obby runs? parkour earning? level building?)
- Player fantasy and progression
- Monetization model (cosmetics only? game passes?)
- World structure and art direction
- Technical architecture (modules, data model)

---

## Existing Infrastructure

**Roblox Projects on Mac:**

| Project | Files | Size | Rating | Branch |
|---------|-------|------|--------|--------|
| Eggventura | 55 | 732K | ★★★★★ | main |
| PetTycoon | 57 | 728K | ★★★★★ | main |
| Spellforge | 21 | 156K | ★★★ | feature/balance-v2 |
| FuseArena | 16 | 136K | ★★★ | feature/fusearena-v2 |
| GearheadTycoon | 10 | 56K | ★★★ | main |
| NeonBastion | 12 | 48K | ★★ | main |
| DungeonDepths | 12 | 72K | ★ | feature/dungeon-depths-m1 |

**Factory Pipeline (Snippy):**
- Location: `D:\_projects_\roblox-factory`
- Tech: LangGraph + DeepSeek V4 Pro (cloud) + Codex CLI
- Status: V2.5 built but had bugs — needs hardening

**Prism Agent-to-Agent:**
- Lumi ↔ Astraea via Discord factory channel
- `listen_to_agents` config on both sides

---

## Process Documentation

| Document | Purpose | Status |
|----------|---------|--------|
| `roblox-game-plan.md` | Game concept, scope, phases | ✅ Updated with all decisions |
| `roblox-factory-process.md` | Workflow: idea → code → test → publish | ✅ Created |
| Session template + Mango scorecard | Inside factory-process.md | ✅ Created |

---

## Project Docs Created

Every project has `docs/summary.md` + `docs/tasks.md`:

| Project | Summary | Tasks |
|---------|---------|-------|
| Eggventura | ✅ | ✅ |
| PetTycoon | ✅ | ✅ |
| NeonBastion | ✅ | ✅ |
| FuseArena | ✅ | ✅ |
| GearheadTycoon | ✅ | ✅ |
| DungeonDepths | ✅ | ✅ |
| Spellforge | ✅ | ✅ |

---

## Session Log

| Date | Session | Accomplished | Efficiency |
|------|---------|-------------|------------|
| 2026-06-05 | Planning Kickoff | Infrastructure reviewed, all project docs created, factory process written, all 4 decisions captured | — |
| 2026-06-06 | Design Kickoff | Ema concept captured, gaps analyzed, design doc created | — |