/// @module SLOT_SCRIPT_INTERFACE
/// Extensions to the game's settlement slot object.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

using twdll::TW_RegionSlot;
using twdll::TW_Settlement;
using twdll::TW_SettlementExpansionSlot;
using twdll::TW_SettlementExpansionManager;

constexpr size_t SLOT_PTR = twdll::TW_PtrOffset<TW_RegionSlot>::value;

static TW_SettlementExpansionSlot* get_expansion_slot(lua_State* L) {
    auto* raw_slot = twdll::tw_unwrap<TW_RegionSlot>(L, 1);
    if (!raw_slot) return nullptr;

    // Call slot:region()
    l_getfield(L, 1, "region");
    l_pushvalue(L, 1);
    if (l_pcall(L, 1, 1, 0) != 0) {
        l_pop(L, 1);
        return nullptr;
    }

    // Call region:settlement()
    l_getfield(L, -1, "settlement");
    l_pushvalue(L, -2);
    if (l_pcall(L, 1, 1, 0) != 0) {
        l_pop(L, 2);
        return nullptr;
    }

    auto* settlement = twdll::tw_unwrap<TW_Settlement>(L, -1);
    TW_SettlementExpansionSlot* result = nullptr;
    if (settlement && settlement->m_settlement_expansion_manager) {
        auto* sem = settlement->m_settlement_expansion_manager;
        auto** elements = reinterpret_cast<TW_SettlementExpansionSlot**>(sem->m_slots.m_elements);
        int count = static_cast<int>(sem->m_slots.m_size);
        if (elements && count > 0 && count < 32) {
            for (int i = 0; i < count; ++i) {
                if (elements[i] && elements[i]->m_slot == raw_slot) {
                    result = elements[i];
                    break;
                }
            }
        }
    }

    l_pop(L, 2); // pop settlement and region
    return result;
}

/***
Returns the memory address of the slot object as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemoryAddress(lua_State* L) {
    return tw_mem_address(L, "slot", SLOT_PTR);
}

/***
Gets the building model rotation on the campaign map (0..5, representing 60-degree increments).
@function GetBuildingRotation
@treturn integer rotation index (0..5)
*/
static int GetBuildingRotation(lua_State* L) {
    auto* exp_slot = get_expansion_slot(L);
    if (!exp_slot) {
        l_pushnil(L);
        return 1;
    }
    l_pushinteger(L, static_cast<lua_Integer>(exp_slot->m_rotation));
    return 1;
}

/***
Sets the building model rotation on the campaign map (0..5, representing 60-degree increments).
Persisted across save/load. Call `twdll.campaign_ui.RefreshSettlements()` or `slot:Refresh()` to apply visual changes on the campaign map.
@function SetBuildingRotation
@tparam integer rotation new rotation index (0..5)
*/
static int SetBuildingRotation(lua_State* L) {
    auto* exp_slot = get_expansion_slot(L);
    if (!exp_slot) return 0;
    auto rot = static_cast<uint32_t>(l_tointeger(L, 2));
    exp_slot->m_rotation = rot % 6;
    return 0;
}

/***
Forces an immediate visual refresh of settlement building models on the campaign map.
@function Refresh
*/
static int Refresh(lua_State*) {
    refresh_settlements_display();
    return 0;
}

static const luaL_Reg slot_methods[] = {
    {"GetMemoryAddress",     GetMemoryAddress},
    {"GetBuildingRotation",  GetBuildingRotation},
    {"SetBuildingRotation",  SetBuildingRotation},
    {"Refresh",              Refresh},
    {nullptr, nullptr}
};

void register_slot_methods(lua_State* L) {
    l_newmetatable(L, "SLOT_SCRIPT_INTERFACE");
    l_getfield(L, -1, "__index");
    if (l_type(L, -1) == LUA_TTABLE) {
        for (const luaL_Reg* f = slot_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] SLOT_SCRIPT_INTERFACE extended");
    } else {
        Log("[twdll] WARNING: SLOT_SCRIPT_INTERFACE __index not found");
    }
    l_pop(L, 2);
}
