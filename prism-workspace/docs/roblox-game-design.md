# New Game — High-Level Design Document

**Created:** 2026-06-06
**Status:** DRAFT — concept from Ema, gaps being filled
**Genre:** Platformer + Tycoon hybrid

---

## Concept (Ema's Draft)

Players collect **spherical creatures** that grow bigger as they level up. Creatures are used to:
1. **Platform** — the bigger/stronger the creature, the more momentum, the further you jump
2. **Run a business** — creatures generate income (power plant or factory)

**Core Loop:**
```
Platform with creature → Collect food (XP) → Creature grows → More speed/momentum → Further jumps → Better food → Stronger creature
         ↓                                                                              ↓
  Creature works in plant → Generates income → Upgrade plant → Unlock equipment → More income
         ↓                                                                              ↓
  More money → Buy rarer eggs → New creature types → New strategies → Deeper gameplay
```

**Design Pillars:**
1. Spherical creatures — shape is important
2. Size = speed = momentum — bigger creatures platform better
3. Factory/business — creatures generate passive income
4. Rare egg economy — money sink that deepens gameplay
5. Long-term retention — months, not days

---

## Gaps & Analysis (Lumi)

### Theme: Frutiger Aero ✅
Glossy translucent nature + technology. Think Windows 7 wallpaper turned into a game universe.
- Creatures are translucent glowing orbs with internal patterns (glass marbles)
- Power plant is organic-chrome — vines on glowing pipes, waterfalls powering turbines
- Living light visible in hub sky — grows brighter as server powers it
- UI: glassy translucent panels, soft gradients
- Sound: soft chimes, water, ambient electronic hums

### Platformer: Launch Mechanic ✅
**Confirmed: Launch-based, not traditional 2D platforming**

Creatures are placed in a launcher → aim + power → launch through procedural course → collect crystals/food → landing distance = reward tier.

3D with fixed camera (isometric/slight top-down). Modular by design — new zones, obstacles, and creature behaviors drop in easily.

Creature types change flight behavior:
- **Rollers** — fast, low arc, great for distance
- **Bouncers** — high bounce, great for vertical targets
- **Gliders** — hover and steer mid-flight
- **Anchors** — heavy, plow through obstacles, short but powerful
- **Sparkers** — random bursts, unpredictable, high risk/reward

### Creature System: 100+ Base × Modifiers ✅
**Confirmed: Triple digits base creatures × modifiers = 800+ unique**

Eggventura-style modifiers (Golden, Plasma, Crystal, Neon, Shadow, Prismatic, Glitch, etc.)

Asset strategy:
- Common/Uncommon: Free Roblox assets
- Rare/Epic: Modified free assets with custom textures
- Legendary/Mythic: Rodin-generated custom meshes (before month end)
- Modifiers: Shader/particle effects layered on base meshes

### Hub World & Personalization ✅
5+ players per server, each with their own power plant visible in the hub.

Personalization is the retention hook:
- Build/arrange plant layout
- Decorate with earned cosmetics
- Your plant's light contribution visible to everyone
- Visit others' plants
- "I made this" feeling keeps players returning

The living light grows as the server collectively powers it — social incentive.

### Retention Hooks (for months)
1. Prestige/Rebirth system (reset for permanent multipliers)
2. 800+ creature collection (base × modifiers)
3. Hub world personalization (build, decorate, visit others)
4. Dual currency economy (Light Shards + Prismatic Cores)
5. Living light social incentive (server-wide goal)
6. Seasonal events (limited eggs, themed launch zones)
7. Offline passive income (24hr cap)
8. Leaderboards (distance records, plant output)
9. Trading
10. Co-op launch runs

---

## Design Decisions (Answered)

1. **Art direction:** ✅ Frutiger Aero — glossy nature + tech, translucent, auroras, organic-chrome. Think Windows 7 wallpaper meets sci-fi.
2. **Platformer perspective:** ✅ Launch mechanic (3D, fixed camera) — creatures are launched through procedural courses. Not traditional 2D platforming.
3. **Creature count:** ✅ Triple digits base × modifiers = 800+ unique creatures. Free Roblox assets for common, Rodin for mythic (before month end).
4. **Factory gameplay:** ✅ Both — active play earns faster, passive generates offline (24hr cap). Hub world shared with 5+ players.
5. **Game name:** ✅ **Pudgy Power** — alliterative, dual meaning (energy + strength), Roblox search-friendly

Rare creatures named after earlier brainstorm: Glowbe, Lumina, Orbora, Sphero, Aeroball

## Dual Currency

| Currency | Name | Acquisition | Use |
|----------|------|-------------|-----|
| Common | Light Shards | Launch runs, passive, selling duplicates | Eggs, plant upgrades, basic cosmetics |
| Rare | Prismatic Cores | Rare targets, daily challenges, mythic bonus | Rare eggs, premium cosmetics, plant expansions, special launchers |

---

## Phase Plan

### V1 — Prototype
- Launch mechanic (aim, launch, collect, land)
- 5 creature families (Rollers, Bouncers, Gliders, Anchors, Sparkers)
- 2-3 modifiers (Golden, Plasma, +1)
- Basic power plant + hub (shared, 5 players)
- Light Shards currency + basic egg shop
- 1-2 launch zones
- Passive income (with 24hr offline cap)

### V2 — Published
- 20+ creature families, 6+ modifiers
- 5+ launch zones with procedural variety
- Full power plant customization + home building
- Dual currency (Light Shards + Prismatic Cores)
- Rare egg system (5 rarity tiers)
- Living light visible in hub (mystery element)
- Game passes
- Visit others' plants

### V3+ — Ongoing
- Prestige system (Rebirth)
- 100+ base creatures × modifiers (800+)
- Seasonal events (limited eggs, themed launch zones)
- Co-op launch runs
- Trading
- Leaderboards (distance records, plant output)
- Plant expansions and premium cosmetics

---

## Session Log

| Date | Session | Accomplished | Efficiency |
|------|---------|-------------|------------|
| 2026-06-06 | Design Kickoff | Ema concept captured, gaps analyzed, recommendations drafted | — |
| 2026-06-06 | Design Decisions | Theme=Frutiger Aero, Launch mechanic, 800+ creatures, Hub world, Dual currency, name TBD | — |