/// @module MILITARY_FORCE_SCRIPT_INTERFACE
/// Extensions to the game's military force object.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

#include <vector>

using twdll::TW_MilitaryForce;
using twdll::TW_Character;
using twdll::TW_Unit;
using twdll::TW_AvailableCharacterRecruitmentItem;
using twdll::TW_Databases;

constexpr size_t MIL_FORCE_PTR = twdll::TW_PtrOffset<TW_MilitaryForce>::value;

namespace Props {
    static twdll::Getter RecruitmentQueueSize{&TW_MilitaryForce::recruitment_queue_size, MIL_FORCE_PTR, "military_force"};
}

/***
Memory address of the military force object in hexadecimal format.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
@usage
local addr = force:GetMemoryAddress()
*/
static int GetMemoryAddress           (lua_State* L) { return tw_mem_address(L, "military_force", MIL_FORCE_PTR); }

/***
Number of units currently in this military force's recruitment queue.
@function GetRecruitmentQueueSize
@treturn integer queued unit count
@usage
local queue_size = force:GetRecruitmentQueueSize()
if queue_size > 0 then
    -- Force is actively recruiting
end
*/
static int GetRecruitmentQueueSize (lua_State* L) { return Props::RecruitmentQueueSize.get(L); }

/***
Disbands one or more units from this military force in a single engine transaction.
Accepts UNIT userdata objects or zero-based list indices matching `unit_list:item_at`.
@function DisbandUnits
@tparam UNIT_SCRIPT_INTERFACE|integer ... unit objects or 0-based integer indices to disband
@treturn boolean true if units were successfully disbanded, false otherwise
@usage
-- Option 1: Disband using unit userdata objects:
local ul = force:unit_list()
force:DisbandUnits(ul:item_at(0), ul:item_at(1))

-- Option 2: Disband using 0-based list indices:
force:DisbandUnits(0, 1)
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

    auto vec = twdll::TW_VectorNcc<void*>::from_span(units_to_disband.data(), units_to_disband.size());
    Log("[twdll] force:DisbandUnits: force=0x%08X disbanding %d units",
        reinterpret_cast<uintptr_t>(force), static_cast<int>(units_to_disband.size()));
    g_disband_units(&vec, g_campaign_model);
    l_pushboolean(L, 1);
    return 1;
}

/***
Current integrity (army morale) value of the military force.
@function GetIntegrity
@treturn number|nil integrity value (0.0 to 100.0), or nil if the force has no integrity tracker
@usage
local integrity = force:GetIntegrity()
if integrity and integrity < 25.0 then
    -- Force is suffering from severe mutiny / desertion risks
end
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
Sets the integrity (army morale) value for this military force, clamped between 0.0 and 100.0.
@function SetIntegrity
@tparam number value new integrity value (0.0 to 100.0)
@treturn boolean true on success, false otherwise
@usage
-- Restore army morale / integrity to 100%:
force:SetIntegrity(100.0)
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
Checks whether this military force tracks integrity (army morale).
@function HasIntegrity
@treturn boolean true if the force has integrity tracking, false otherwise
@usage
if force:HasIntegrity() then
    local morale = force:GetIntegrity()
end
*/
static int HasIntegrity(lua_State* L) {
    auto* force = twdll::tw_unwrap<TW_MilitaryForce>(L, 1);
    l_pushboolean(L, (force && force->m_morale) ? 1 : 0);
    return 1;
}

/***
Appoints and assigns the specified character as the commanding general of this military force.
Instantiates the general's bodyguard unit, subsumes it into the force, dismisses the former general
back to the faction court, and promotes the character to commanding general.
@function AppointCharacter
@tparam CHARACTER_SCRIPT_INTERFACE character character object to appoint as commanding general
@treturn boolean true if the character was successfully appointed, false otherwise
@usage
local candidate = faction:character_list():item_at(1)
force:AppointCharacter(candidate)
*/
static int AppointCharacter(lua_State* L) {
    auto* force = twdll::tw_unwrap<TW_MilitaryForce>(L, 1);
    auto* character = twdll::tw_unwrap<TW_Character>(L, 2);

    if (!force || !character) {
        Log("[twdll] force:AppointCharacter: invalid force or character userdata");
        l_pushboolean(L, 0);
        return 1;
    }

    auto* current_gen = force->m_general_link.get();
    if (current_gen == character) {
        l_pushboolean(L, 1);
        return 1;
    }

    if (!g_recruit_character_entry_impl) {
        Log("[twdll] force:AppointCharacter: required engine signature for recruit_character_entry_impl not resolved");
        l_pushboolean(L, 0);
        return 1;
    }

    auto* faction = force->m_faction_link.get();
    void* pool_mgr = faction ? faction->m_character_recruitment_pool : nullptr;
    if (!pool_mgr) {
        Log("[twdll] force:AppointCharacter: character recruitment pool manager is null");
        l_pushboolean(L, 0);
        return 1;
    }

    auto* dbs = TW_Databases::get();
    void* agent_rec = nullptr;
    if (dbs && dbs->agents) {
        agent_rec = dbs->agents->find_record("general");
    }

    // Ensure candidate character has valid initial general bodyguard details:
    if (!character->details.m_initial_general_bodyguard_details.m_unit && current_gen) {
        character->details.m_initial_general_bodyguard_details.m_unit = current_gen->details.m_initial_general_bodyguard_details.m_unit;
        character->details.m_initial_general_bodyguard_details.m_men = current_gen->details.m_initial_general_bodyguard_details.m_men;
        character->details.m_initial_general_bodyguard_details.m_men_in_fully_replenished = current_gen->details.m_initial_general_bodyguard_details.m_men_in_fully_replenished;
    }

    uint32_t army_pos = 0;
    void* region_ptr = nullptr;
    if (current_gen) {
        if (g_get_character_map_piece) {
            void* piece = g_get_character_map_piece(current_gen);
            if (piece) {
                army_pos = *reinterpret_cast<uint32_t*>(reinterpret_cast<uintptr_t>(piece) + 8);
            }
        }
        if (!army_pos) {
            army_pos = current_gen->logical_position;
        }
        region_ptr = *reinterpret_cast<void**>(reinterpret_cast<uintptr_t>(current_gen) + 0x5CC);
    }

    Log("[twdll] force:AppointCharacter: force=0x%08X appointing character=0x%08X (former_general=0x%08X, region=0x%08X, army_pos=0x%08X: x=%d, y=%d)",
        reinterpret_cast<uintptr_t>(force),
        reinterpret_cast<uintptr_t>(character),
        reinterpret_cast<uintptr_t>(current_gen),
        reinterpret_cast<uintptr_t>(region_ptr),
        army_pos,
        army_pos & 0xFFFF,
        army_pos >> 16);

    TW_AvailableCharacterRecruitmentItem item{};
    item.m_character = character;
    item.m_cost = 0;

    void* res = g_recruit_character_entry_impl(
        pool_mgr,
        &item,
        region_ptr,
        force,
        reinterpret_cast<void*>(army_pos),
        agent_rec,
        0,
        nullptr,
        nullptr,
        &character->details.m_initial_general_bodyguard_details,
        0,
        reinterpret_cast<void*>(1) // ignore_agent_cap = 1
    );

    if (!res) {
        Log("[twdll] force:AppointCharacter: g_recruit_character_entry_impl returned null");
        l_pushboolean(L, 0);
        return 1;
    }

    Log("[twdll] force:AppointCharacter: force=0x%08X successfully appointed character=0x%08X as general",
        reinterpret_cast<uintptr_t>(force), reinterpret_cast<uintptr_t>(character));
    l_pushboolean(L, 1);
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
    {"AppointCharacter",        AppointCharacter},
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
