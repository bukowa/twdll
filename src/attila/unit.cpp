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

constexpr size_t UNIT_PTR = twdll::TW_PtrOffset<TW_Unit>::value;

namespace Props {
    static twdll::Property NumMen      {&TW_Unit::num_men,        UNIT_PTR, "unit"};
    static twdll::Property MaxNumMen   {&TW_Unit::max_num_men,    UNIT_PTR, "unit"};
    static twdll::Property ActionPoints{&TW_Unit::action_points,  UNIT_PTR, "unit"};
}

/***
Returns the memory address of the unit object as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemoryAddress    (lua_State* L) { return tw_mem_address(L, "unit", UNIT_PTR); }

/***
Gets the current number of men in the unit.
@function GetNumMen
@treturn integer current number of men
*/
static int GetNumMen        (lua_State* L) { return Props::NumMen.get(L); }

/***
Sets the current number of men in the unit.
@function SetNumMen
@tparam integer value new number of men
*/
static int SetNumMen        (lua_State* L) { return Props::NumMen.set(L); }

/***
Gets the maximum number of men the unit can have.
@function GetMaxNumMen
@treturn integer maximum number of men
*/
static int GetMaxNumMen     (lua_State* L) { return Props::MaxNumMen.get(L); }

/***
Sets the maximum number of men the unit can have.
@function SetMaxNumMen
@tparam integer value new maximum number of men
*/
static int SetMaxNumMen     (lua_State* L) { return Props::MaxNumMen.set(L); }

/***
Gets the action points remaining for the unit.
@function GetActionPoints
@treturn integer action points
*/
static int GetActionPoints  (lua_State* L) { return Props::ActionPoints.get(L); }

/***
Sets the action points for the unit.
@function SetActionPoints
@tparam integer value new action points
*/
static int SetActionPoints  (lua_State* L) { return Props::ActionPoints.set(L); }

/***
Replaces the unit with a new unit of the given type, in the same army, using
the engine's own unit conversion path (the same one used for religion and
technology upgrades). The new unit keeps the old unit's men count, experience
and combat statistics. The old unit object is destroyed in the process, so the
original unit reference is no longer valid afterwards.
@function ConvertUnit
@tparam string unit_key the unit record key (e.g. "att_inf_melee_spear_att")
@treturn boolean true if the unit was converted, false otherwise
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

    // Resolve the force through the unit's force link (verified against the
    // engine's own UNIT_SCRIPT_INTERFACE::military_force in the 64-bit source):
    //   unit->m_force_link.m_link.m_object          -> ONE_TO_MANY_LINK*
    //     ->m_object.m_object                       -> UNIT_CONTAINER*
    //     ->m_military_force.m_object               -> MILITARY_FORCE*
    void* one_to_many = *reinterpret_cast<void**>(reinterpret_cast<char*>(unit) + 0x38);
    if (!one_to_many) {
        Log("[twdll] ConvertUnit: unit has no force link");
        return 0;
    }
    void* container = *reinterpret_cast<void**>(one_to_many);
    if (!container) {
        Log("[twdll] ConvertUnit: unit has no container");
        return 0;
    }
    void* force = *reinterpret_cast<void**>(container);
    if (!force) {
        Log("[twdll] ConvertUnit: unit has no military force");
        return 0;
    }

    // Resolve the game's database list the same way the engine looks up
    // records (chain verified in tw_types.h).
    if (!g_campaign_model) {
        Log("[twdll] ConvertUnit: campaign model not available");
        return 0;
    }
    auto* env = static_cast<TW_CampaignModel*>(g_campaign_model)->m_campaign_env;
    if (!env) {
        Log("[twdll] ConvertUnit: campaign env not resolved");
        return 0;
    }
    auto* game_core = static_cast<TW_CampaignEnv*>(env)->m_game_core;
    if (!game_core) {
        Log("[twdll] ConvertUnit: game core not resolved");
        return 0;
    }
    auto* databases = static_cast<TW_GameCore*>(game_core)->m_databases;
    if (!databases) {
        Log("[twdll] ConvertUnit: databases not resolved");
        return 0;
    }
    void* units_table = static_cast<TW_Databases*>(databases)->m_main_units_table;
    if (!units_table) {
        Log("[twdll] ConvertUnit: main_units_table not loaded");
        return 0;
    }

    struct RecordKey {
        uint32_t    m_len;
        uint32_t    m_pad;
        const char* m_data;
    } key_string = { static_cast<uint32_t>(key_len), 0, key };

    void* record = g_record_index(units_table, &key_string);
    if (!record) {
        Log("[twdll] ConvertUnit: no record for key '%s'", key);
        l_pushboolean(L, 0);
        return 1;
    }

    Log("[twdll] ConvertUnit: unit=0x%08X force=0x%08X record=0x%08X key='%s'",
        reinterpret_cast<uintptr_t>(unit),
        reinterpret_cast<uintptr_t>(force),
        reinterpret_cast<uintptr_t>(record), key);

    void* new_unit = g_convert_unit(unit, force, record);
    l_pushboolean(L, new_unit != nullptr);
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
