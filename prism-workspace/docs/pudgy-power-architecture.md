# Pudgy Power — Technical Architecture

**Created:** 2026-06-06
**Status:** V1 Architecture Draft
**Game:** Pudgy Power — Frutiger Aero Platformer Tycoon

---

## System Overview

```
┌─────────────────────────────────────────────────────────┐
│                     ROBLOX CLIENT                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌─────────┐ │
│  │ Launch   │  │ Hub      │  │ Plant    │  │ Egg     │ │
│  │ System   │  │ World    │  │ Builder  │  │ Shop    │ │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬────┘ │
│       │             │             │              │       │
│  ┌────┴─────────────┴─────────────┴──────────────┴────┐ │
│  │              Remotes (Client→Server)                │ │
│  └────────────────────┬───────────────────────────────┘ │
└───────────────────────┼─────────────────────────────────┘
                        │
┌───────────────────────┼─────────────────────────────────┐
│                     SERVER                              │
│  ┌────────────────────┴───────────────────────────────┐ │
│  │              Remotes (Server Handlers)               │ │
│  └────┬──────┬──────┬──────┬──────┬──────┬───────────┘ │
│       │      │      │      │      │      │              │
│  ┌────┴──┐┌──┴───┐┌─┴───┐┌─┴────┐┌┴─────┐┌┴──────┐    │
│  │Creature││Launch││Plant││Economy││Session││Events│    │
│  │Manager ││Engine││Mgr ││Manager││Manager││Manager│   │
│  └────┬──┘└──┬───┘└──┬──┘└──┬───┘└──┬───┘└──┬───┘    │
│       │       │       │       │        │       │         │
│  ┌────┴───────┴───────┴───────┴────────┴───────┴──┐    │
│  │              DataStoreService                      │    │
│  └──────────────────────┬───────────────────────────┘    │
└─────────────────────────┼──────────────────────────────┘
                          │
                   ┌──────┴──────┐
                   │ DataStore   │
                   │ (Roblox)    │
                   └─────────────┘
```

---

## Module Architecture

### Server Modules (ServerScriptService)

| Module | Purpose | Priority |
|--------|---------|----------|
| `GameInit.server.luau` | Boot sequence, service wiring, DataStore init | P0 |
| `CreatureManager.luau` | Creature data, stats, growth, modifiers, rarity | P0 |
| `LaunchEngine.luau` | Launch physics, trajectory, collision, rewards | P0 |
| `PlantManager.luau` | Power plant state, income calculation, upgrade logic | P0 |
| `EconomyManager.luau` | Light Shards + Prismatic Cores, transactions, shop | P0 |
| `SessionManager.luau` | Player data load/save, offline income calc (24hr cap) | P0 |
| `EggManager.luau` | Egg purchasing, hatching, rarity rolls | P0 |
| `HubManager.luau` | Hub world, player plant placement, visiting | P1 |
| `EventManager.luau` | Seasonal events, limited eggs, daily challenges | P2 |
| `LeaderboardManager.luau` | Distance records, plant output rankings | P2 |
| `TradeManager.luau` | Player-to-player trading | P3 |

### Client Modules (StarterPlayerScripts / StarterGui)

| Module | Purpose | Priority |
|--------|---------|----------|
| `LaunchUI.client.luau` | Aiming, power gauge, launch button | P0 |
| `HubHUD.client.luau` | Player plant, living light, other players' plants | P0 |
| `PlantUI.client.luau` | Plant building, decorating, upgrading | P1 |
| `CreatureHUD.client.luau` | Active creature display, stats, level | P0 |
| `EggShopUI.client.luau` | Egg purchasing, rarity display, hatching animation | P0 |
| `InventoryUI.client.luau` | Creature collection, filters, modifiers | P1 |
| `LeaderboardUI.client.luau` | Distance records, plant output | P2 |

### Shared Modules (ReplicatedStorage)

| Module | Purpose | Priority |
|--------|---------|----------|
| `Config.luau` | All game constants, tuning values, rarity tables | P0 |
| `CreatureData.luau` | Base creature definitions (families, stats, modifiers) | P0 |
| `EggData.luau` | Egg types, costs, rarity chances, hatch tables | P0 |
| `PlantData.luau` | Plant tiers, equipment, upgrade costs, output rates | P0 |
| `LaunchData.luau` | Launch zone configs, obstacle types, reward tiers | P0 |
| `Remotes.luau` | All RemoteEvent/RemoteFunction definitions | P0 |
| `ModifierData.luau` | Modifier definitions (Golden, Plasma, Crystal, etc.) | P1 |
| `SeasonalData.luau` | Seasonal events, limited-time content | P2 |

---

## Data Model

### Creature

```lua
Creature = {
    id: string,              -- unique instance ID
    family: string,           -- Roller, Bouncer, Glider, Anchor, Sparker
    rarity: string,           -- Common, Uncommon, Rare, Epic, Legendary, Mythic
    modifier: string?,        -- Golden, Plasma, Crystal, Neon, Shadow, Prismatic, Glitch
    level: number,            -- 1-100
    size: number,             -- derived from level + family base
    speed: number,            -- derived from size + family
    momentum: number,         -- launch power multiplier
    power_output: number,     -- passive income rate in plant
    xp: number,               -- current XP towards next level
    food_collected: number,   -- lifetime food collected
}
```

### PlayerData

```lua
PlayerData = {
    player_id: number,
    light_shards: number,     -- common currency
    prismatic_cores: number,  -- rare currency
    creatures: {Creature},    -- owned creatures
    active_creature: string,  -- creature ID currently equipped
    plant: PlantData,         -- player's power plant state
    last_online: number,      -- timestamp for offline income
    offline_income_cap: 86400, -- 24 hours in seconds
    total_launches: number,
    total_distance: number,   -- best launch distances
    prestige_level: number,   -- rebirth count (V3+)
}
```

### Plant

```lua
Plant = {
    tier: number,             -- 1-10 (unlock progression)
    equipment: {Equipment},   -- installed equipment
    layout: {LayoutNode},     -- player's arrangement
    output_rate: number,      -- Light Shards per minute
    cosmetic_items: {string}, -- decorations, themes
    hub_position: Vector3,    -- position in shared hub
}
```

### LaunchResult

```lua
LaunchResult = {
    creature_id: string,
    distance: number,          -- total distance traveled
    food_collected: {FoodItem},
    crystals_hit: number,      -- Light Shards earned
    rare_crystals: number,     -- Prismatic Cores earned
    obstacles_hit: number,
    boost_pads: number,
    zone_reached: string,      -- which zone they landed in
}
```

---

## Core Systems

### 1. Launch Engine

The core gameplay loop. Players aim and launch creatures through procedural courses.

**Flow:**
1. Player selects creature → enters launch zone
2. Aim phase: drag to set angle, hold for power
3. Launch: physics simulation with creature-specific flight behavior
4. Flight: collect food/crystals, hit boost pads, bounce off surfaces
5. Landing: distance determines reward tier
6. Results: XP, currency, rare drops

**Modularity:** Each launch zone is a config in `LaunchData.luau`. New zones = new config entry. Obstacles, boost pads, and targets are all data-driven.

### 2. Creature Growth

**Level progression:** Food XP → Level Up → Size/Speed/Momentum increase
**Family differences:** Each family has different growth curves and stat distributions.
**Modifiers:** Visual variants (Golden, Plasma, etc.) that may have stat multipliers.

**Size affects gameplay:**
- Bigger creatures = more momentum = further launches
- Bigger creatures = more plant output
- Bigger creatures access new launch zones (gates)

### 3. Power Plant (Home)

**Income model:**
- Active: creature generates Light Shards while in plant
- Passive: offline income at reduced rate (up to 24 hours)
- Upgrades: better equipment = higher output, new slots = more creatures

**Customization:**
- Layout: arrange equipment in grid
- Cosmetics: themes, decorations, visual effects
- Hub position: visible to other players

### 4. Egg System

**Rarity tiers:**
| Tier | Cost (Shards) | Chance | Creatures |
|------|---------------|--------|-----------|
| Basic | 100 | 70% Common, 25% Uncommon, 5% Rare | Common families |
| Premium | 500 | 50% Uncommon, 30% Rare, 15% Epic, 5% Legendary | All families |
| Prismatic | 5 Prismatic Cores | 30% Rare, 40% Epic, 25% Legendary, 5% Mythic | All families + exclusives |

**Modifiers:** Hatched creatures have a small chance of a modifier (Golden: 2%, Plasma: 1.5%, Crystal: 1%, Neon: 0.5%, Shadow: 0.3%, Prismatic: 0.1%, Glitch: 0.05%)

### 5. Hub World

- Shared server: 5-8 players
- Each player has a plot for their power plant
- Living light in the center/sky grows with collective server output
- Visit others' plants (view-only for V1, interaction for V2)
- Social proximity: see other players launching

### 6. Economy (Dual Currency)

**Light Shards (Common):**
- Earned from: launches, passive plant income, selling duplicates
- Spent on: Basic eggs, plant upgrades, basic cosmetics

**Prismatic Cores (Rare):**
- Earned from: rare launch targets, daily challenges, mythic creature bonus, prestige
- Spent on: Premium/Prismatic eggs, premium cosmetics, plant expansions, special launchers

---

## V1 Scope (Prototype)

**Must Have:**
- ✅ Launch mechanic (aim, power, launch, collect, land)
- ✅ 5 creature families (Roller, Bouncer, Glider, Anchor, Sparker)
- ✅ 3 modifiers (Golden, Plasma, Crystal)
- ✅ Basic power plant (passive income, 1 upgrade tier)
- ✅ Hub world (5 players, basic plots)
- ✅ Light Shards currency + basic egg shop
- ✅ 1 launch zone with procedural variation
- ✅ Creature growth (level, size, stats)
- ✅ Player data save/load

**Nice to Have (V1.5):**
- Plant layout customization
- 2nd launch zone
- Prismatic Cores currency
- Leaderboard
- 3 more modifiers (Neon, Shadow, Prismatic)

**Not V1:**
- Trading, co-op, prestige, seasonal events, 800+ creatures

---

## File Structure (Rojo)

```
PudgyPower/
├── default.project.json
├── docs/
│   ├── summary.md
│   ├── tasks.md
│   ├── game-design.md          ← this document
│   ├── game-description.md
│   └── playtest/
├── assets/
│   └── meshes/                 ← free Roblox assets + Rodin exports
├── src/
│   ├── ServerScriptService/
│   │   ├── GameInit.server.luau
│   │   ├── CreatureManager.luau
│   │   ├── LaunchEngine.luau
│   │   ├── PlantManager.luau
│   │   ├── EconomyManager.luau
│   │   ├── SessionManager.luau
│   │   ├── EggManager.luau
│   │   ├── HubManager.luau
│   │   └── EventManager.luau
│   ├── StarterPlayerScripts/
│   │   ├── LaunchUI.client.luau
│   │   ├── HubHUD.client.luau
│   │   ├── CreatureHUD.client.luau
│   │   └── EggShopUI.client.luau
│   ├── StarterGui/
│   │   ├── PlantUI.client.luau
│   │   └── InventoryUI.client.luau
│   └── ReplicatedStorage/
│       ├── Config.luau
│       ├── CreatureData.luau
│       ├── EggData.luau
│       ├── PlantData.luau
│       ├── LaunchData.luau
│       ├── Remotes.luau
│       └── ModifierData.luau
```

---

## Asset Pipeline

**Strategy:**
- **Common/Uncommon creatures:** Free Roblox sphere models + recolors
- **Rare/Epic creatures:** Modified free assets with custom textures
- **Legendary/Mythic creatures:** Rodin-generated custom meshes (before month end!)
- **Modifiers:** Shader/particle effects layered on base meshes (Golden = sparkle, Plasma = electric arcs, Crystal = refraction, etc.)
- **Launch zones:** Roblox terrain + custom obstacles
- **Hub:** Modular building pieces (Frutiger Aero aesthetic — glass, chrome, organic curves)
- **UI:** Translucent glass panels with soft gradients

---

## Session Log

| Date | Session | Accomplished | Efficiency |
|------|---------|-------------|------------|
| 2026-06-06 | Architecture Draft | Full technical architecture, data model, module structure, V1 scope defined | — |