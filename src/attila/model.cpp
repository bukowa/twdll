/// @module twdll.model
/// Campaign model-level operations for Total War: Attila.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

#include <vector>

using twdll::TW_Unit;

/***
Disbands and removes one or more units across any military forces in the campaign world in a single transaction.

Handles full removal from the campaign world, updating army bookkeeping, disband events, and UI state.
@function DisbandUnits
@tparam UNIT_SCRIPT_INTERFACE ... one or more unit objects to disband
@usage
-- Disband arbitrary units across the campaign:
twdll.model.DisbandUnits(unit1, unit2)
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

    twdll::TW_VectorNcc vec = {
        reinterpret_cast<void*>(static_cast<size_t>(n)),
        n,
        elems.data()
    };
    Log("[twdll] DisbandUnits: n=%d model=0x%08X", n,
        reinterpret_cast<uintptr_t>(g_campaign_model));
    g_disband_units(&vec, g_campaign_model);
    return 0;
}

extern const luaL_Reg model_functions[] = {
    {"DisbandUnits", DisbandUnits},
    {nullptr, nullptr}
};
