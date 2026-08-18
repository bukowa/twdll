# Campaign AI (CAI) Telemetry & Decision Intelligence Architecture Standard

**Document Version:** 1.0.0  
**Target Engine:** Total War: Attila (32-bit Warscape Engine / `empire.retail.dll`)  
**Scope:** `twdll` Campaign AI Hooks, Logging, and Autonomous Decision Monitoring  

---

## 1. Executive Summary & Core Philosophy

### The "Reasoning-First" Telemetry Principle
Traditional reverse-engineering hooks and debug logs in game modification typically capture only **the final outcome** of a function (e.g. `Decision: SACK (id=1)` or `Constructed: bld_farm_3`). 

For Campaign AI analysis, debugging, and autonomous LLM agent reasoning, **bare outcomes are insufficient and misleading**. An observer seeing `Decision: SACK` cannot determine:
- Why did the AI prefer `SACK` over `RAZE` or `OCCUPY`?
- Did it choose `SACK` because it was starving for gold/food, or because its personality weights strongly favored raiding?
- What other options were considered, what were their respective mathematical weights, and was the choice deterministic or a weighted lottery roll?
- If the AI did nothing (`NONE`), was the target rejected due to cultural DB rules, physical state incompatibility (e.g. a ruin), or low utility score?

**Mandatory Project Standard:**
> **Every CAI telemetry hook must log the *Complete Decision Path* — capturing the inputs, environmental pressures, candidate options, mathematical weight breakdown, and final resolution mechanism.**

---

## 2. The 5-Stage CAI Decision Chain

Every AI decision in the Warscape engine follows a structured 5-stage pipeline. Telemetry hooks must capture each stage in sequence:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. STRATEGIC CONTEXT & FACTION NEEDS (Internal Pressures)                   │
│    - Kingdom status (Regions, Horde vs Landed, Major/Minor, Treasury)       │
│    - Systemic pressures (Food deficit/surplus, Squalor, Threat level)       │
├─────────────────────────────────────────────────────────────────────────────┤
│ 2. TARGET / ENVIRONMENTAL STATE (External Context)                          │
│    - Target entity (Settlement, Army, Province, Diplomatic Faction)         │
│    - Target characteristics (Ruin vs Active, Fertility, Garrison, Climate)  │
├─────────────────────────────────────────────────────────────────────────────┤
│ 3. CANDIDATE OPTION FILTERING (Feasibility Matrix)                          │
│    - Allowed Mask (Faction-level cultural / DB / diplomatic permissions)    │
│    - In-Context Mask (Situational permissions dictated by target state)     │
│    - Effective Set (Intersection of Allowed ∩ In-Context)                   │
├─────────────────────────────────────────────────────────────────────────────┤
│ 4. UTILITY SCORING & MATHEMATICAL WEIGHTING (The "Why")                     │
│    - Personality Base Weights (from DB/CAI personality component)           │
│    - Dynamic Modifiers (Food deficit multipliers, Climate penalty, etc.)    │
│    - Final Score Vector [w_0, w_1, ..., w_N] and Probability Distribution   │
├─────────────────────────────────────────────────────────────────────────────┤
│ 5. RESOLUTION & EXECUTION (The Outcome)                                     │
│    - Resolution method (Deterministic Max vs `random_proportional_select`)  │
│    - Winning action ID and human-readable descriptor                        │
│    - Or explicit Rejection Reason if outcome is NONE                        │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Standardized Log Grammar & Visual Hierarchy

To prevent log clutter while maximizing semantic density for humans and LLMs, all CAI telemetry must adhere to a standardized 3-tier hierarchy:

### Tier 1: Faction Context Boundary
Whenever the AI begins processing a new faction (or context switches between factions during a turn), emit a single boundary header:

```
[twdll][CAI] ── Faction Context: '<faction_key>' (regions: N, major: yes/no, food: ±N, treasury: N) ──
```

### Tier 2: Subsystem Evaluation Block
Each evaluation event within a subsystem emits a structured block detailing the target, the driving pressures, the candidate options with weights, and the outcome.

#### Example A: Successful Weighted Decision (Occupation)
```
[twdll][CAI:OCCUPATION] Target: 'att_reg_thracia_marcianopolis' (fertility: 3, food: +20, state: occupied)
  ├─ Drivers: Food deficit (-5) increases raid utility (+150%); High fertility boosts occupy score
  ├─ Candidate Weights:
  │    • SACK:                15.2 (76.0% probability)
  │    • RAZE_WITHOUT_OCCUPY:  4.8 (24.0% probability)
  └─ Resolution: SACK (id=1) selected via proportional lottery roll
```

#### Example B: Target Rejection / Incompatible Options
```
[twdll][CAI:OCCUPATION] Target: 'att_reg_dacia_apulum' (state: abandoned_ruin)
  ├─ Drivers: Target requires [RESETTLE, COLONISE]
  ├─ Constraints: Faction rules only permit [SACK, RAZE, VASSALISE, RAZE_WITHOUT_OCCUPY]
  └─ Resolution: REJECTED (NONE, id=11) — Zero compatible actions between faction rules and target state
```

---

## 4. Subsystem Telemetry Blueprints

### 4.1 Occupation & Tactical Target Selection (`cai_occupation.cpp`)
* **Primary Hook:** `EMPIRECAMPAIGNAI::CAI_INTERFACE::make_occupation_decision` (`0x10D05C80` in 32-bit / `0x00D05C80` RVA).
* **Canonical Action Map (DWARF Ground Truth):**
  - `0`: `LOOT` (Pillage treasury & take ownership)
  - `1`: `SACK` (Pillage wealth, damage buildings, withdraw without ownership)
  - `2`: `RAZE` (Level settlement, burn structures, take ownership)
  - `3`: `OCCUPY` (Peaceful conquest, incur public order resistance)
  - `4`: `LIBERATE` (Resurrect / restore original faction)
  - `5`: `VASSALISE` (Enforce client state status)
  - `6`: `RAZE_WITHOUT_OCCUPY` (Attila nomad mechanic: burn to ruin, gain horde growth points, abandon)
  - `7`: `COLONISE` (Found colony in ruins, expend treasury gold and army manpower)
  - `8`: `DO_NOTHING` (Leave unmolested)
  - `9`: `RESETTLE` (Civilian resettlement)
  - `10`: `GIFT_TO_ANOTHER_FACTION` (Transfer captured region to ally)
  - `11`: `NONE` (`NUM_OCCUPATION_DECISIONS` / No viable score or compatibility)
* **Key Drivers to Log:**
  - `CAI_FOOD_AND_SQUALOR_ANALYSER`: Faction food balance (`predicted_food`) and region fertility.
  - `CAI_PERSONALITY`: Personality base weights for looting vs empire building.
  - `CLIMATE`: Proximity to climate degradation step.

---

### 4.2 Building Construction & Slot Planning (`cai_construction.cpp`)
* **Target Hook:** `acquire_building_valuations_accross_the_faction` & `evaluate_building_chain_construction`.
* **Telemetry Requirements:**
  - Log which province and slot is being evaluated (Province Capital vs Minor Settlement).
  - Log current province deficits: Food balance, Public Order trends, Religious conversion pressure, Sanitation/Disease risk.
  - Log scored building chains:
    ```
    [twdll][CAI:CONSTRUCTION] Province: 'att_prov_thracia' | Settlement: 'att_reg_thracia_constantinople' (Slot 3)
      ├─ Pressures: Food: -12 (CRITICAL), Public Order: +4 (STABLE), Sanitation: -2 (SQUALOR_RISK)
      ├─ Scored Building Chains:
      │    • 'att_bld_civil_farm_3'       (Food):       Score = 245.0  [TOP CHOICE]
      │    • 'att_bld_sanitation_canal_2' (Sanitation): Score = 110.5
      │    • 'att_bld_military_barracks_2' (Recruit):   Score =  35.0
      └─ Resolution: COMMITTED 'att_bld_civil_farm_3' (Cost: 1800 gold, 4 turns)
    ```

---

### 4.3 Military Recruitment & Army Composition (`cai_recruitment.cpp`)
* **Target Hook:** `evaluate_unit_recruitment_for_force` / `CAI_RECRUITMENT_MANAGER`.
* **Telemetry Requirements:**
  - Target Military Force and Commanding General.
  - Current army composition ratio vs AI Personality Target Template (e.g. 50% Infantry / 30% Missile / 20% Cavalry).
  - Available units in recruitment pool with utility scores (Cost-efficiency vs upkeep vs technology level).
  - Resolution: Unit enlisted, mercenary hired, or recruitment skipped due to treasury cap.

---

### 4.4 Diplomacy & Treaties (`cai_diplomacy.cpp`)
* **Target Hook:** `evaluate_diplomatic_proposal` / `determine_strategic_attitude`.
* **Telemetry Requirements:**
  - Proposer and Recipient factions.
  - Proposed Deal Components: Non-Aggression, Trade, Military Alliance, Peace, Gold Gift.
  - Valuation Breakdown: Strategic balance of power, historical grievances, mutual enemies, military threat.
  - Resolution: ACCEPTED (Valuation > 0.0) or REJECTED with deficit points.

---

## 5. C++ Implementation Rules for `twdll`

1. **SEH Safety (`__try / __except`):**
   - Game engine analytical objects (`CAI_FACTION`, `CAI_REGION`, `CAI_PERSONALITY`) must be accessed under Structured Exception Handling. Never assume secondary analytical pointers are non-null during turn initialization.
2. **Zero-Allocation Logging:**
   - Use fixed-size stack buffers (`char buffer[512]`, `snprintf`) for telemetry string assembly. Never allocate dynamic heap objects or `std::string` copies inside high-frequency CAI loop hooks.
3. **No Invasive Side-Effects:**
   - Telemetry hooks must be purely observational. The original function (`orig_func`) must receive intact registers, unmodified stack arguments, and return exact native values.
4. **Modularity:**
   - Every subsystem must reside in its own dedicated source file under `src/attila/cai/` (`cai_occupation.cpp`, `cai_construction.cpp`, `cai_recruitment.cpp`, `cai_diplomacy.cpp`) referencing shared utilities via `cai_common.h`.

---

## 6. Verification & Validation Checklist

Before any CAI telemetry module is merged into production:
- [ ] Telemetry clearly identifies the evaluating Faction, Target, and Current State.
- [ ] Log provides the **reasons / drivers** (pressures, deficits, bonuses) behind the choice.
- [ ] Log lists all **viable options with weights/probabilities**, not just the winner.
- [ ] Negative / Rejected outcomes (`NONE`) explicitly state **why** they were skipped.
- [ ] C++ code compiles cleanly with 0 warnings under `/W4 /utf-8`.
- [ ] LDoc documentation is written and generated in `docs/attila/modules/`.
- [ ] `CHANGELOG.md` is updated with user-facing Lua/telemetry capabilities.
