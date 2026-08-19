/// @module MILITARY_FORCE_SCRIPT_INTERFACE
/// Extensions to the game's military force object.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

#include <vector>

using twdll::TW_MilitaryForce;
using twdll::TW_Unit;

constexpr size_t MIL_FORCE_PTR = twdll::TW_PtrOffset<TW_MilitaryForce>::value;

namespace Props {
    static twdll::Getter RecruitmentQueueSize{&TW_MilitaryForce::recruitment_queue_size, MIL_FORCE_PTR, "military_force"};
}

/***
Returns the memory address of the military force object as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemoryAddress           (lua_State* L) { return tw_mem_address(L, "military_force", MIL_FORCE_PTR); }

/***
Returns the number of units in the recruitment queue.
@function GetRecruitmentQueueSize
@treturn integer number of units
*/
static int GetRecruitmentQueueSize (lua_State* L) { return Props::RecruitmentQueueSize.get(L); }

/***
Disbands (permanently removes) one or more units from this military force.
Accepts UNIT userdata objects or integer unit indices (0-indexed matching unit_list:item_at), passed as varargs.
@function DisbandUnits
@param ... unit userdata or integer indices to disband
@treturn boolean true if units were successfully disbanded, false otherwise
*/
static int DisbandUnits(lua_State* L) {
    if (!g_disband_units || !g_campaign_model) {
        Log("[twdll] force:DisbandUnits: signatures or campaign model not ready");
        l_pushboolean(L, 0);
        return 1;
    }

    auto* force = twdll::tw_unwrap<TW_MilitaryForce>(L, 1);
    if (!force) {
        Log("[twdll] force:DisbandUnits: null force");
        l_pushboolean(L, 0);
        return 1;
    }

    std::vector<void*> units_to_disband;

    auto resolve_unit_from_index = [&](int unit_idx) -> TW_Unit* {
        l_getfield(L, 1, "unit_list");
        l_pushvalue(L, 1);
        TW_Unit* result_unit = nullptr;
        if (l_pcall(L, 1, 1, 0) == 0 && l_type(L, -1) == LUA_TUSERDATA) {
            l_getfield(L, -1, "item_at");
            l_pushvalue(L, -2);
            l_pushinteger(L, unit_idx);
            if (l_pcall(L, 2, 1, 0) == 0 && l_type(L, -1) == LUA_TUSERDATA) {
                result_unit = twdll::tw_unwrap<TW_Unit>(L, -1);
            }
            l_pop(L, 1);
        }
        l_pop(L, 1);
        return result_unit;
    };

    int arg_idx = 2;
    while (l_type(L, arg_idx) != LUA_TNONE) {
        int t = l_type(L, arg_idx);
        if (t == LUA_TUSERDATA) {
            auto* u = twdll::tw_unwrap<TW_Unit>(L, arg_idx);
            if (u) units_to_disband.push_back(u);
        } else if (t == LUA_TNUMBER) {
            int unit_idx = static_cast<int>(l_tointeger(L, arg_idx));
            auto* u = resolve_unit_from_index(unit_idx);
            if (u) units_to_disband.push_back(u);
            else Log("[twdll] force:DisbandUnits: could not resolve unit at index %d", unit_idx);
        }
        ++arg_idx;
    }

    if (units_to_disband.empty()) {
        Log("[twdll] force:DisbandUnits: no valid units to disband");
        l_pushboolean(L, 0);
        return 1;
    }

    void* vec[3] = {
        reinterpret_cast<void*>(units_to_disband.size()),
        reinterpret_cast<void*>(units_to_disband.size()),
        units_to_disband.data()
    };
    Log("[twdll] force:DisbandUnits: force=0x%08X disbanding %d units",
        reinterpret_cast<uintptr_t>(force), static_cast<int>(units_to_disband.size()));
    g_disband_units(vec, g_campaign_model);
    l_pushboolean(L, 1);
    return 1;
}

/***
Returns the current integrity (army morale) value between 0.0 and 100.0, or nil if none.
@function GetIntegrity
@treturn number integrity value (0.0 to 100.0) or nil
*/
static int GetIntegrity(lua_State* L) {
    auto* force = twdll::tw_unwrap<TW_MilitaryForce>(L, 1);
    if (!force || !force->m_morale) {
        l_pushnil(L);
        return 1;
    }
    l_pushnumber(L, force->m_morale->m_morale);
    return 1;
}

/***
Sets the integrity (army morale) value, clamped between 0.0 and 100.0.
@function SetIntegrity
@tparam number value new integrity value (0.0 to 100.0)
@treturn boolean true on success, false otherwise
*/
static int SetIntegrity(lua_State* L) {
    auto* force = twdll::tw_unwrap<TW_MilitaryForce>(L, 1);
    if (!force || !force->m_morale) {
        l_pushboolean(L, 0);
        return 1;
    }
    float val = static_cast<float>(l_tonumber(L, 2));
    if (val < 0.0f) val = 0.0f;
    if (val > 100.0f) val = 100.0f;
    force->m_morale->m_morale = val;
    Log("[twdll] force:SetIntegrity: force=0x%08X integrity set to %.2f",
        reinterpret_cast<uintptr_t>(force), val);
    l_pushboolean(L, 1);
    return 1;
}

/***
Returns whether this military force uses the integrity / army morale system.
@function HasIntegrity
@treturn boolean true if the force has integrity tracking
*/
static int HasIntegrity(lua_State* L) {
    auto* force = twdll::tw_unwrap<TW_MilitaryForce>(L, 1);
    l_pushboolean(L, (force && force->m_morale) ? 1 : 0);
    return 1;
}

extern const luaL_Reg military_force_functions[] = {
    {nullptr, nullptr}
};

static const luaL_Reg military_force_methods[] = {
    {"GetMemoryAddress",        GetMemoryAddress},
    {"GetRecruitmentQueueSize", GetRecruitmentQueueSize},
    {"DisbandUnits",            DisbandUnits},
    {"GetIntegrity",            GetIntegrity},
    {"SetIntegrity",            SetIntegrity},
    {"HasIntegrity",            HasIntegrity},
    {nullptr, nullptr}
};

void register_military_force_methods(lua_State* L) {
    l_newmetatable(L, "MILITARY_FORCE_SCRIPT_INTERFACE");
    l_getfield(L, -1, "__index");
    if (l_type(L, -1) == LUA_TTABLE) {
        for (const luaL_Reg* f = military_force_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] MILITARY_FORCE_SCRIPT_INTERFACE extended");
    } else {
        Log("[twdll] WARNING: MILITARY_FORCE_SCRIPT_INTERFACE __index not found");
    }
    l_pop(L, 2);
}
