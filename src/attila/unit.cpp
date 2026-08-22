/// @module UNIT_SCRIPT_INTERFACE
/// Extensions to the game's unit object.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

using twdll::TW_Unit;
using twdll::TW_CampaignModel;
using twdll::TW_CampaignEnv;
using twdll::TW_GameCore;
using twdll::TW_Databases;

using twdll::TW_Character;

constexpr size_t UNIT_PTR = twdll::TW_PtrOffset<TW_Unit>::value;

namespace Props {
    static twdll::Property NumMen      {&TW_Unit::num_men,        UNIT_PTR, "unit"};
    static twdll::Property MaxNumMen   {&TW_Unit::max_num_men,    UNIT_PTR, "unit"};
    static twdll::Property ActionPoints{&TW_Unit::action_points,  UNIT_PTR, "unit"};
}

/***
Memory address of the unit object in hexadecimal format.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
@usage
local addr = unit:GetMemoryAddress()
*/
static int GetMemoryAddress    (lua_State* L) { return tw_mem_address(L, "unit", UNIT_PTR); }

/***
Current number of surviving soldiers in the unit.
@function GetNumMen
@treturn integer current number of men
@usage
local men = unit:GetNumMen()
local max_men = unit:GetMaxNumMen()
if men < max_men * 0.5 then
    -- Unit is below 50% strength and heavily depleted
end
*/
static int GetNumMen        (lua_State* L) { return Props::NumMen.get(L); }

/***
Sets the number of surviving soldiers in the unit.
Immediately updates the unit's health bar and strength percentage.
@function SetNumMen
@tparam integer value new number of men (clamped between 0 and `max_num_men`)
@treturn boolean true on success, false otherwise
@usage
-- Replenish unit immediately to full strength:
unit:SetNumMen(unit:GetMaxNumMen())

-- Or set specific casualty count after an event:
unit:SetNumMen(20)
*/
static int SetNumMen        (lua_State* L) { return Props::NumMen.set(L); }

/***
Maximum soldier capacity (full strength count) for this unit.
@function GetMaxNumMen
@treturn integer maximum number of men
@usage
local max_men = unit:GetMaxNumMen()
*/
static int GetMaxNumMen     (lua_State* L) { return Props::MaxNumMen.get(L); }

/***
Sets the maximum soldier capacity (full strength count) for this unit.
@function SetMaxNumMen
@tparam integer value new maximum number of men
@treturn boolean true on success, false otherwise
@usage
-- Increase unit max strength cap to 150:
unit:SetMaxNumMen(150)
*/
static int SetMaxNumMen     (lua_State* L) { return Props::MaxNumMen.set(L); }

/***
Remaining movement action points for this unit.
@function GetActionPoints
@treturn integer action points
@usage
local ap = unit:GetActionPoints()
*/
static int GetActionPoints  (lua_State* L) { return Props::ActionPoints.get(L); }

/***
Sets the movement action points for this unit.
@function SetActionPoints
@tparam integer value new action points
@treturn boolean true on success, false otherwise
@usage
-- Reset unit action points:
unit:SetActionPoints(100)
*/
static int SetActionPoints  (lua_State* L) { return Props::ActionPoints.set(L); }

/***
Replaces the unit with a new unit type in the same military force (matching the engine upgrade path).

Key mechanics:
- Scales soldier count proportionally to the new unit's max capacity.
- Preserves experience chevron rank, battle kills, and combat statistics.
- If the unit is a general's bodyguard, synchronises the commander's bodyguard record.
- **Note**: Destroys the old unit object; retrieve the replacement from the force's unit list.
@function ConvertUnit
@tparam string unit_key the unit record key from `main_units_tables` (e.g. `"att_merc_ger_agathyrsi_warriors"`, `"att_inf_melee_spear_att"`)
@treturn boolean true if the unit was converted, false otherwise
@usage
-- Upgrade or convert a unit in an army:
local unit = force:unit_list():item_at(0)
local ok = unit:ConvertUnit("att_merc_ger_agathyrsi_warriors")
if ok then
    -- Note: 'unit' is now invalid; fetch the updated unit from force:unit_list()
    local updated_unit = force:unit_list():item_at(0)
end
*/
static int ConvertUnit(lua_State* L) {
    if (!g_convert_unit || !g_record_index) {
        Log("[twdll] ConvertUnit: signatures not resolved");
        return 0;
    }

    auto* unit = twdll::tw_unwrap<TW_Unit>(L, 1);
    if (!unit) {
        Log("[twdll] ConvertUnit: null unit");
        return 0;
    }

    size_t key_len = 0;
    const char* key = l_checklstring(L, 2, &key_len);
    if (!key) {
        Log("[twdll] ConvertUnit: unit key not a string");
        return 0;
    }

    // Resolve the force through the unit's force link:
    auto* force = unit->get_military_force();
    if (!force) {
        Log("[twdll] ConvertUnit: unit has no military force");
        return 0;
    }

    auto* dbs = TW_Databases::get();
    if (!dbs || !dbs->main_units) {
        Log("[twdll] ConvertUnit: main_units table not loaded");
        return 0;
    }

    void* record = dbs->main_units->find_record(key, key_len);
    if (!record) {
        Log("[twdll] ConvertUnit: no record for key '%s'", key);
        l_pushboolean(L, 0);
        return 1;
    }

    int old_num_men = unit->num_men;
    int old_max_num_men = unit->max_num_men;

    Log("[twdll] ConvertUnit: unit=0x%08X force=0x%08X record=0x%08X key='%s'",
        reinterpret_cast<uintptr_t>(unit),
        reinterpret_cast<uintptr_t>(force),
        reinterpret_cast<uintptr_t>(record), key);

    auto* new_unit = static_cast<TW_Unit*>(g_convert_unit(unit, force, record));
    if (!new_unit) {
        l_pushboolean(L, 0);
        return 1;
    }

    // Proportional soldier count scaling:
    if (old_max_num_men > 0 && new_unit->max_num_men > 0) {
        if (old_num_men >= old_max_num_men) {
            new_unit->num_men = new_unit->max_num_men;
        } else {
            float ratio = static_cast<float>(old_num_men) / static_cast<float>(old_max_num_men);
            int scaled_men = static_cast<int>(ratio * new_unit->max_num_men + 0.5f);
            if (scaled_men < 1 && old_num_men > 0) scaled_men = 1;
            if (scaled_men > new_unit->max_num_men) scaled_men = new_unit->max_num_men;
            new_unit->num_men = scaled_men;
        }
    }

    // If this unit is a general's bodyguard, synchronise the character's persistent snapshot:
    auto* commander_link = new_unit->m_commander_link;
    if (commander_link && commander_link->m_object) {
        auto* commander = commander_link->m_object;
        commander->details.m_initial_general_bodyguard_details.m_unit = record;
        commander->details.m_initial_general_bodyguard_details.m_men = static_cast<uint16_t>(new_unit->num_men);
        commander->details.m_initial_general_bodyguard_details.m_men_in_fully_replenished = static_cast<uint16_t>(new_unit->max_num_men);
        Log("[twdll] ConvertUnit: updated commander=0x%08X bodyguard snapshot (men=%d/%d)",
            reinterpret_cast<uintptr_t>(commander), new_unit->num_men, new_unit->max_num_men);
    }

    l_pushboolean(L, 1);
    return 1;
}

/***
Disbands and removes this unit from its military force.

Updates campaign force state, unit counts, and UI bookkeeping in a single transaction. Save/load safe.
@function Disband
@treturn boolean true if successfully disbanded, false otherwise
@usage
local unit = force:unit_list():item_at(force:unit_list():num_items() - 1)
local ok = unit:Disband()
*/
static int Disband(lua_State* L) {
    if (!g_disband_units || !g_campaign_model) {
        Log("[twdll] unit:Disband: signatures or campaign model not ready");
        l_pushboolean(L, 0);
        return 1;
    }
    auto* unit = twdll::tw_unwrap<TW_Unit>(L, 1);
    if (!unit) {
        Log("[twdll] unit:Disband: null unit");
        l_pushboolean(L, 0);
        return 1;
    }
    void* elems[1] = { unit };
    auto vec = twdll::TW_VectorNcc<void*>::from_span(elems, 1);
    Log("[twdll] unit:Disband: unit=0x%08X", reinterpret_cast<uintptr_t>(unit));
    g_disband_units(&vec, g_campaign_model);
    l_pushboolean(L, 1);
    return 1;
}

extern const luaL_Reg unit_functions[] = {
    {nullptr, nullptr}
};

static const luaL_Reg unit_methods[] = {
    {"GetMemoryAddress",  GetMemoryAddress},
    {"GetNumMen",         GetNumMen},
    {"SetNumMen",         SetNumMen},
    {"GetMaxNumMen",      GetMaxNumMen},
    {"SetMaxNumMen",      SetMaxNumMen},
    {"GetActionPoints",   GetActionPoints},
    {"SetActionPoints",   SetActionPoints},
    {"ConvertUnit",       ConvertUnit},
    {"Disband",           Disband},
    {nullptr, nullptr}
};

void register_unit_methods(lua_State* L) {
    l_newmetatable(L, "UNIT_SCRIPT_INTERFACE");
    l_getfield(L, -1, "__index");
    if (l_type(L, -1) == LUA_TTABLE) {
        for (const luaL_Reg* f = unit_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] UNIT_SCRIPT_INTERFACE extended");
    } else {
        Log("[twdll] WARNING: UNIT_SCRIPT_INTERFACE __index not found");
    }
    l_pop(L, 2);
}
