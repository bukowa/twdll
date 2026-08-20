/// @module RELIGION_SCRIPT_INTERFACE
/// Script interface for a religion entry in a region.
#include "../common/tw.h"
#include "tw_types.h"

using twdll::TW_ReligionRecord;

static const char* kReligionMetatable = "RELIGION_SCRIPT_INTERFACE";

struct ReligionUserdata {
    const TW_ReligionRecord* record;
    float proportion;
};

static ReligionUserdata* get_religion(lua_State* L) {
    return static_cast<ReligionUserdata*>(l_touserdata(L, 1));
}

/***
Database record key of this religion from `religions_tables` (e.g. `"att_rel_chr_catholic"`).
@function GetKey
@treturn string religion database key
@usage
local key = rel:GetKey()
*/
static int GetKey(lua_State* L) {
    auto* rel = get_religion(L);
    if (!rel || !rel->record) {
        l_pushnil(L);
        return 1;
    }
    l_pushstring(L, rel->record->m_key.m_data ? rel->record->m_key.m_data : "");
    return 1;
}

/***
Proportion of this religion in the region as a normalized float (0.0 to 1.0).
@function GetProportion
@treturn number religion proportion (0.0 to 1.0)
@usage
local pct = rel:GetProportion() * 100
-- e.g. 75.0 for 75%
*/
static int GetProportion(lua_State* L) {
    auto* rel = get_religion(L);
    if (!rel) {
        l_pushnumber(L, 0.0);
        return 1;
    }
    l_pushnumber(L, rel->proportion);
    return 1;
}

/***
UI icon image path for this religion.
@function GetIconPath
@treturn string icon file path (e.g. `"UI/Religions/att_rel_chr_catholic.png"`)
@usage
local icon = rel:GetIconPath()
*/
static int GetIconPath(lua_State* L) {
    auto* rel = get_religion(L);
    if (!rel || !rel->record) {
        l_pushnil(L);
        return 1;
    }
    l_pushstring(L, rel->record->m_icon_path.m_data ? rel->record->m_icon_path.m_data : "");
    return 1;
}

static const luaL_Reg religion_methods[] = {
    {"GetKey",        GetKey},
    {"key",           GetKey},
    {"GetProportion", GetProportion},
    {"proportion",    GetProportion},
    {"GetIconPath",   GetIconPath},
    {"icon_path",     GetIconPath},
    {nullptr, nullptr}
};

void register_religion_methods(lua_State* L) {
    l_newmetatable(L, kReligionMetatable);
    l_createtable(L, 0, 6);
    for (const luaL_Reg* f = religion_methods; f->name; ++f) {
        l_pushstring(L, f->name);
        l_pushcclosure(L, f->func, 0);
        l_settable(L, -3);
    }
    l_setfield(L, -2, "__index");
    l_pop(L, 1);
    Log("[twdll] RELIGION_SCRIPT_INTERFACE registered");
}

void push_religion(lua_State* L, const TW_ReligionRecord* record, float proportion) {
    if (!record) {
        l_pushnil(L);
        return;
    }
    auto* ud = static_cast<ReligionUserdata*>(l_newuserdata(L, sizeof(ReligionUserdata)));
    ud->record = record;
    ud->proportion = proportion;
    l_newmetatable(L, kReligionMetatable);
    l_setmetatable(L, -2);
}
