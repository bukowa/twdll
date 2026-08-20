/// @module REGION_SCRIPT_INTERFACE
/// Extensions to the game's region object.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

using twdll::TW_Region;
using twdll::TW_FACTION_PROVINCE_MANAGER;

constexpr size_t REGION_PTR = twdll::TW_PtrOffset<TW_Region>::value;

// Resolves the FACTION_PROVINCE_MANAGER for the region via the engine function.
// This cannot be a typed pointer deref from REGION (the FPM is reached through
// CONST_SAFE_PTR derefs + a PROVINCE sub-call), so we call it like SetFactionLeader.
static TW_FACTION_PROVINCE_MANAGER* get_fpm(lua_State* L) {
    auto* region = twdll::tw_unwrap<TW_Region>(L, 1);
    if (!region) return nullptr;
    if (!g_faction_province_manager) {
        Log("[twdll] region: REGION::faction_province_manager not resolved");
        return nullptr;
    }
    return static_cast<TW_FACTION_PROVINCE_MANAGER*>(g_faction_province_manager(region));
}

/***
Memory address of the region object in hexadecimal format.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
@usage
local addr = region:GetMemoryAddress()
*/
static int GetMemoryAddress    (lua_State* L) { return tw_mem_address(L, "region", REGION_PTR); }

/***
Province development population surplus points (used to unlock new building slots).
@function GetPopulationSurplus
@treturn integer population surplus points
@usage
local surplus = region:GetPopulationSurplus()
if surplus >= 4 then
    -- Province has enough surplus points to expand settlement slots
end
*/
static int GetPopulationSurplus(lua_State* L) {
    auto* fpm = get_fpm(L);
    if (!fpm) { l_pushnil(L); return 1; }
    l_pushinteger(L, static_cast<lua_Integer>(fpm->m_province_development.m_development_points));
    return 1;
}

/***
Sets the province development population surplus points (used to unlock new building slots).
Persisted in savegames and immediately reflected in province development UI.
@function SetPopulationSurplus
@tparam integer value new population surplus value (>= 0)
@usage
-- Grant 5 population surplus points for fast slot expansion:
region:SetPopulationSurplus(5)
*/
static int SetPopulationSurplus(lua_State* L) {
    auto* fpm = get_fpm(L);
    if (!fpm) return 0;
    fpm->m_province_development.m_development_points =
        static_cast<unsigned int>(l_tointeger(L, 2));
    return 0;
}

/***
Accumulated province growth points progressing towards the next population surplus point.
@function GetGrowthPoints
@treturn integer accumulated growth points
@usage
local growth = region:GetGrowthPoints()
*/
static int GetGrowthPoints(lua_State* L) {
    auto* fpm = get_fpm(L);
    if (!fpm) { l_pushnil(L); return 1; }
    l_pushinteger(L, static_cast<lua_Integer>(fpm->m_province_development.m_accumulated_growth));
    return 1;
}

/***
Sets the accumulated province growth points progressing towards the next population surplus point.
@function SetGrowthPoints
@tparam integer value new growth points value
@usage
region:SetGrowthPoints(500)
*/
static int SetGrowthPoints(lua_State* L) {
    auto* fpm = get_fpm(L);
    if (!fpm) return 0;
    fpm->m_province_development.m_accumulated_growth =
        static_cast<unsigned int>(l_tointeger(L, 2));
    return 0;
}

void push_religion_list(lua_State* L, const twdll::TW_ReligionProportion* items, int count);

/***
List interface (@{RELIGION_LIST_SCRIPT_INTERFACE}) containing all religious denominations present in this region.
Use `num_items()` and zero-based `item_at(index)` to iterate through the breakdown.
@function GetReligionList
@treturn RELIGION_LIST_SCRIPT_INTERFACE list interface containing all religions present in this region
@usage
local rel_list = region:GetReligionList()
for i = 0, rel_list:num_items() - 1 do
    local rel = rel_list:item_at(i)
    local key = rel:GetKey()
    local pct = rel:GetProportion() * 100
    -- e.g. key = "att_rel_chr_catholic", pct = 75.0
end
*/
static int GetReligionList(lua_State* L) {
    auto* region = twdll::tw_unwrap<TW_Region>(L, 1);
    if (!region) {
        push_religion_list(L, nullptr, 0);
        return 1;
    }
    auto* elements = reinterpret_cast<const twdll::TW_ReligionProportion*>(region->m_religion_breakdown.m_elements);
    int count = static_cast<int>(region->m_religion_breakdown.m_size);
    push_religion_list(L, elements, count);
    return 1;
}

/***
Proportion of a specific religion in this region as a normalized float (0.0 to 1.0).
@function GetReligionProportion
@tparam string religion_key database key from `religions_tables` (e.g. `"att_rel_chr_catholic"`, `"att_rel_pagan_germanic"`)
@treturn number religion proportion (0.0 to 1.0), or 0.0 if not present
@usage
local pagan_pct = region:GetReligionProportion("att_rel_pagan_germanic") * 100
if pagan_pct > 50.0 then
    -- Paganism is the dominant majority religion in this region
end
*/
static int GetReligionProportion(lua_State* L) {
    auto* region = twdll::tw_unwrap<TW_Region>(L, 1);
    if (!region || l_type(L, 2) != LUA_TSTRING) {
        l_pushnumber(L, 0.0f);
        return 1;
    }
    const char* key = l_checklstring(L, 2, nullptr);
    if (!key) {
        l_pushnumber(L, 0.0f);
        return 1;
    }
    auto* elements = reinterpret_cast<const twdll::TW_ReligionProportion*>(region->m_religion_breakdown.m_elements);
    int count = static_cast<int>(region->m_religion_breakdown.m_size);
    for (int i = 0; i < count; ++i) {
        if (elements[i].m_religion && elements[i].m_religion->m_key.m_data &&
            std::strcmp(elements[i].m_religion->m_key.m_data, key) == 0) {
            l_pushnumber(L, elements[i].m_proportion);
            return 1;
        }
    }
    l_pushnumber(L, 0.0f);
    return 1;
}

extern const luaL_Reg region_functions[] = {
    {nullptr, nullptr}
};

static const luaL_Reg region_methods[] = {
    {"GetMemoryAddress",      GetMemoryAddress},
    {"GetPopulationSurplus",  GetPopulationSurplus},
    {"SetPopulationSurplus",  SetPopulationSurplus},
    {"GetGrowthPoints",       GetGrowthPoints},
    {"SetGrowthPoints",       SetGrowthPoints},
    {"GetReligionList",       GetReligionList},
    {"GetReligions",          GetReligionList},
    {"religion_list",         GetReligionList},
    {"GetReligionProportion", GetReligionProportion},
    {nullptr, nullptr}
};

void register_region_methods(lua_State* L) {
    l_newmetatable(L, "REGION_SCRIPT_INTERFACE");
    l_getfield(L, -1, "__index");
    if (l_type(L, -1) == LUA_TTABLE) {
        for (const luaL_Reg* f = region_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] REGION_SCRIPT_INTERFACE extended");
    } else {
        Log("[twdll] WARNING: REGION_SCRIPT_INTERFACE __index not found");
    }
    l_pop(L, 2);
}
