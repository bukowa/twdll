/// @module twdll.model
/// Campaign model-level operations for Total War: Attila.
#include "common/tw.h"
#include "game_api.h"
#include "tw_types.h"

#include <vector>

using twdll::TW_Unit;

/***
Disbands (permanently removes) one or more units from the game. This mirrors
the game's own UNIT::disband_units path, so each unit is fully removed from
the world and all bookkeeping (events, dirty flags, force teardown) runs
correctly. Unlike a bare container remove, this is save/load safe.
@function DisbandUnits
@param units vararg userdata, one or more UNIT userdata values to disband
*/
static int DisbandUnits(lua_State* L) {
    if (!g_disband_units) {
        Log("[twdll] DisbandUnits: disband_units signature not resolved");
        return 0;
    }
    if (!g_campaign_model) {
        Log("[twdll] DisbandUnits: campaign model not available yet");
        return 0;
    }

    int n = 0;
    while (l_type(L, n + 1) != LUA_TNONE) {
        ++n;
    }
    if (n == 0) {
        Log("[twdll] DisbandUnits: no units given");
        return 0;
    }

    std::vector<void*> elems(static_cast<size_t>(n));
    for (int i = 0; i < n; ++i) {
        auto* unit = twdll::tw_unwrap<TW_Unit>(L, i + 1);
        if (!unit) {
            Log("[twdll] DisbandUnits: argument %d is not a unit", i + 1);
            return 0;
        }
        elems[static_cast<size_t>(i)] = unit;
    }

    void* vec[3] = {
        reinterpret_cast<void*>(static_cast<size_t>(n)),
        reinterpret_cast<void*>(static_cast<size_t>(n)),
        elems.data()
    };
    Log("[twdll] DisbandUnits: n=%d model=0x%08X", n,
        reinterpret_cast<uintptr_t>(g_campaign_model));
    g_disband_units(vec, g_campaign_model);
    return 0;
}

extern const luaL_Reg model_functions[] = {
    {"DisbandUnits", DisbandUnits},
    {nullptr, nullptr}
};
