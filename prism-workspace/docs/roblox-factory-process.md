# Roblox Factory Process — Workflow Documentation

**Created:** 2026-06-05
**Status:** DRAFT — foundation for Factory rebuild

---

## Overview

This document defines the end-to-end workflow for building a Roblox game using the Lumi + Astraea feedback loop. It serves as the process foundation that the Roblox Factory will operationalize.

---

## Phase 1: PLANNING

**Owner:** Lumi (with Ema)
**Duration:** 1-2 sessions
**Output:** `roblox-game-plan.md` (complete)

### Steps
1. **Game Concept** — genre, core loop, player fantasy, monetization
2. **Scope Definition** — what's in V1, what's deferred
3. **Architecture Decision** — data model, service boundaries, module structure
4. **Asset Inventory** — what 3D models, UI, audio are needed
5. **Exit Criteria** — concrete checklist for "planning is done"

**Gate:** Ema approves the game plan before any code.

---

## Phase 2: SCAFFOLDING

**Owner:** Lumi directs, Astraea/Forge/Codex executes
**Duration:** 1 session
**Output:** Empty project with correct structure

### Steps
1. **Project Setup** — Rojo project, place files, directory structure
2. **Module Skeletons** — all ModuleScripts with interfaces defined
3. **Config Foundation** — Config module with all game constants
4. **Exit Criteria** — project builds in Studio, all modules require() without error

---

## Phase 3: CORE LOOP

**Owner:** Lumi directs, Astraea/Forge implements
**Duration:** 2-3 sessions
**Output:** Playable core game loop

### Steps
1. **Core Mechanic** — the one thing the player does over and over
2. **Game State** — save/load, progression tracking
3. **Basic UI** — HUD, shop, inventory (functional, not polished)
4. **Exit Criteria** — core loop is playable for 5+ minutes

---

## Phase 4: CONTENT & POLISH

**Owner:** Lumi directs, Astraea/Forge implements
**Duration:** 3-5 sessions
**Output:** Content-complete game

### Steps
1. **Content Expansion** — all pets/levels/items/worlds
2. **UI Polish** — animations, transitions, VFX
3. **Audio** — sound effects, music
4. **Exit Criteria** — game is content-complete

---

## Phase 5: SHIP

**Owner:** Lumi + Ema
**Duration:** 1-2 sessions
**Output:** Published game

### Steps
1. **Playtest** — full playthrough, bug hunting
2. **Game Pass / Dev Product** setup
3. **Store Page** — description, thumbnails, icons
4. **Publish** — to Roblox
5. **Post-Launch** — monitor, hotfix, iterate

---

## Session Workflow (Every Session)

```
1. START: Load context (memory, project docs, last session summary)
2. SCOPE: What are we doing this session? (1-3 tasks max)
3. EXECUTE: Work through tasks (Lumi directs, Astraea implements)
4. REVIEW: Did we accomplish what we set out to do?
5. SCORE: Rate efficiency (Mango scorecard)
6. DOCUMENT: Write session summary to docs/playtest/ or docs/sessions/
7. PERSIST: Update memory, project docs, task tracker
```

---

## Mango Efficiency Scorecard

After every session, rate the following on a 1-10 scale:

| Category | Description |
|----------|-------------|
| **Planning Clarity** | Were tasks well-defined before starting? |
| **Execution Speed** | How fast did we move from plan to result? |
| **Context Loss** | Did we re-derive information we already knew? |
| **Rework Rate** | How much work had to be redone? |
| **Tool Efficiency** | Were the right tools used for each task? |
| **Communication** | Were instructions clear between Lumi and Astraea? |
| **Overall** | Weighted average |

Target: 9.5+ overall. Adjust process if below 8.0.

---

## End-of-Session Summary Template

```markdown
# Session — [DATE]

## Scope
- [What we planned to do]

## Accomplished
- [What we actually did]

## Blocked / Deferred
- [What we couldn't finish and why]

## Key Decisions
- [Decisions made this session]

## Mango Scorecard
| Category | Score |
|----------|-------|
| Planning Clarity | ?/10 |
| Execution Speed | ?/10 |
| Context Loss | ?/10 |
| Rework Rate | ?/10 |
| Tool Efficiency | ?/10 |
| Communication | ?/10 |
| **Overall** | **?/10** |

## Next Session
- [What to pick up next]
```