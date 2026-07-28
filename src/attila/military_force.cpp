/// @module MILITARY_FORCE_SCRIPT_INTERFACE
/// Extensions to the game's military force object.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

using twdll::TW_MilitaryForce;
using twdll::TW_Unit;

constexpr size_t MIL_FORCE_PTR = twdll::TW_PtrOffset<TW_MilitaryForce>::value;

namespace Props {
    static twdll::Getter RecruitmentQueueSize{&TW_MilitaryForce::recruitment_queue_size, MIL_FORCE_PTR, "military_force"};
}

static int GetMemAddress           (lua_State* L) { return tw_mem_address(L, "military_force", MIL_FORCE_PTR); }
static int GetRecruitmentQueueSize (lua_State* L) { return Props::RecruitmentQueueSize.get(L); }

/***
Removes a unit from this military force.
@function RemoveUnit
@tparam userdata unit the unit to remove from the force
*/
static int RemoveUnit(lua_State* L) {
    auto* mf   = twdll::tw_unwrap<TW_MilitaryForce>(L, 1);
    auto* unit = twdll::tw_unwrap<TW_Unit>(L, 2);
    if (!mf || !unit) {
        Log("[twdll] RemoveUnit: null military_force or unit");
        return 0;
    }
    Log("[twdll] RemoveUnit: mf=0x%08X unit=0x%08X", mf, unit);
    g_remove_unit(mf, unit);
    return 0;
}

extern const luaL_Reg military_force_functions[] = {
    {"GetMemoryAddress",        GetMemAddress},
    {"GetRecruitmentQueueSize", GetRecruitmentQueueSize},
    {nullptr, nullptr}
};

static const luaL_Reg military_force_methods[] = {
    {"RemoveUnit", RemoveUnit},
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
