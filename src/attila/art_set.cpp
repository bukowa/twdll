/// @module ARTSET_SCRIPT_INTERFACE
/// Art set userdata for character portraits and cultural variations.
#include "common/tw.h"
#include "game_api.h"
#include "tw_types.h"

using twdll::TW_CharacterDetailsArtSetInfo;
using twdll::TW_CharacterDetails;
using twdll::TW_CharacterArtSet;
using twdll::TW_CampaignCharacterArtSetRecord;
using twdll::TW_PortraitCameraSettings;
using twdll::TW_CAString;

static const char* kArtSetMetatable = "ARTSET_SCRIPT_INTERFACE";

static TW_CharacterDetails* GetDetailsFromArtInfo(TW_CharacterDetailsArtSetInfo* info) {
    if (!info) return nullptr;
    return reinterpret_cast<TW_CharacterDetails*>(
        reinterpret_cast<char*>(info) - offsetof(TW_CharacterDetails, m_art_set_info)
    );
}

static TW_CampaignCharacterArtSetRecord* GetRecord(TW_CharacterDetailsArtSetInfo* info) {
    if (!info || !info->m_art_set) return nullptr;
    return info->m_art_set->m_art_set_record;
}

/***
Returns the art set record key ID (e.g. "att_cult_barbarian").
@function GetKey
@treturn string art set record key
*/
static int GetKey(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    auto* rec = GetRecord(info);
    if (rec && rec->m_art_set_id.m_data) {
        l_pushstring(L, rec->m_art_set_id.m_data);
    } else if (info && info->m_art_set_to_allocate.m_data) {
        l_pushstring(L, info->m_art_set_to_allocate.m_data);
    } else {
        l_pushstring(L, "");
    }
    return 1;
}

/***
Returns the allocated culture key.
@function GetCulture
@treturn string culture key
*/
static int GetCulture(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    if (!info || !info->m_culture_key.m_data) {
        l_pushstring(L, "");
        return 1;
    }
    l_pushstring(L, info->m_culture_key.m_data);
    return 1;
}

/***
Returns the allocated subculture key.
@function GetSubculture
@treturn string subculture key
*/
static int GetSubculture(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    if (!info || !info->m_subculture_key.m_data) {
        l_pushstring(L, "");
        return 1;
    }
    l_pushstring(L, info->m_subculture_key.m_data);
    return 1;
}

/***
Returns the allocated faction key.
@function GetFaction
@treturn string faction key
*/
static int GetFaction(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    if (!info || !info->m_faction_key.m_data) {
        l_pushstring(L, "");
        return 1;
    }
    l_pushstring(L, info->m_faction_key.m_data);
    return 1;
}

/***
Returns the allocated agent type key (e.g. "general", "champion").
@function GetAgent
@treturn string agent key
*/
static int GetAgent(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    if (!info || !info->m_agent_key.m_data) {
        l_pushstring(L, "");
        return 1;
    }
    l_pushstring(L, info->m_agent_key.m_data);
    return 1;
}

/***
Returns whether this art set is a custom override.
@function IsCustom
@treturn boolean true if custom art set
*/
static int IsCustom(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    auto* rec = GetRecord(info);
    l_pushboolean(L, rec ? (rec->m_is_custom ? 1 : 0) : 0);
    return 1;
}

/***
Returns whether this art set is male.
@function IsMale
@treturn boolean true if male
*/
static int IsMale(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    auto* rec = GetRecord(info);
    l_pushboolean(L, rec ? (rec->m_is_male ? 1 : 0) : 0);
    return 1;
}

/***
Returns whether this art set supports aging variations.
@function HasAging
@treturn boolean true if aging enabled
*/
static int HasAging(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_aging_set) ? 1 : 0);
    return 1;
}

/***
Returns whether this art set supports seasonal variations.
@function HasSeasonal
@treturn boolean true if seasonal enabled
*/
static int HasSeasonal(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_seasonal_set) ? 1 : 0);
    return 1;
}

/***
Returns whether this art set supports levelling / rank variations.
@function HasLevelling
@treturn boolean true if levelling enabled
*/
static int HasLevelling(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_levelling_set) ? 1 : 0);
    return 1;
}

/***
Returns whether this art set supports health variations.
@function HasHealth
@treturn boolean true if health variation enabled
*/
static int HasHealth(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_health_set) ? 1 : 0);
    return 1;
}

/***
Returns whether this art set supports religion variations.
@function HasReligion
@treturn boolean true if religion variation enabled
*/
static int HasReligion(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_religion_set) ? 1 : 0);
    return 1;
}

/***
Returns whether this art set is specifically for faction leaders.
@function IsFactionLeaderSet
@treturn boolean true if faction leader set
*/
static int IsFactionLeaderSet(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_faction_leader_set) ? 1 : 0);
    return 1;
}

/***
Returns the resolved 2D portrait diffuse PNG path for this character.
@function GetPortraitPath
@treturn string portrait diffuse file path
*/
static int GetPortraitPath(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    auto* details = GetDetailsFromArtInfo(info);
    if (!details || !g_resolve_portrait_path || !g_campaign_model) {
        l_pushstring(L, "");
        return 1;
    }
    TW_CAString resolved_path = {};
    g_resolve_portrait_path(details, &resolved_path, g_campaign_model);
    l_pushstring(L, resolved_path.m_data ? resolved_path.m_data : "");
    return 1;
}

/***
Returns the unique portrait settings ID string (e.g. "att_general_barbarian_01").
@function GetSettingsId
@treturn string portrait settings unique identifier
*/
static int GetSettingsId(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    auto* details = GetDetailsFromArtInfo(info);
    if (details && details->m_portrait_camera_settings && details->m_portrait_camera_settings->m_unique_id_string.m_data) {
        l_pushstring(L, details->m_portrait_camera_settings->m_unique_id_string.m_data);
    } else {
        l_pushstring(L, "");
    }
    return 1;
}

/***
Returns the campaign group key (e.g. "main"), or empty string if none.
@function GetGroup
@treturn string campaign group key
*/
static int GetGroup(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    auto* rec = GetRecord(info);
    if (rec && rec->m_group && rec->m_group->m_key.m_data) {
        l_pushstring(L, rec->m_group->m_key.m_data);
    } else {
        l_pushstring(L, "");
    }
    return 1;
}

bool SetCharacterArtSet(TW_CharacterDetailsArtSetInfo* info, const char* art_set_key) {
    if (!info || !art_set_key || !g_update_art_set || !g_ca_string_assign || !g_campaign_model) return false;
    auto* details = GetDetailsFromArtInfo(info);
    if (!details) return false;
    g_ca_string_assign(&info->m_art_set_to_allocate, art_set_key);
    info->m_art_set = nullptr;
    g_update_art_set(info, details, g_campaign_model);
    Log("[twdll] SetCharacterArtSet: art_set_to_allocate set to '%s' (re-allocated)", art_set_key);
    return true;
}

static const luaL_Reg art_set_methods[] = {
    {"GetKey",               GetKey},
    {"GetCulture",           GetCulture},
    {"GetSubculture",        GetSubculture},
    {"GetFaction",           GetFaction},
    {"GetAgent",             GetAgent},
    {"GetGroup",             GetGroup},
    {"IsCustom",             IsCustom},
    {"IsMale",               IsMale},
    {"HasAging",             HasAging},
    {"HasSeasonal",          HasSeasonal},
    {"HasLevelling",         HasLevelling},
    {"HasHealth",            HasHealth},
    {"HasReligion",          HasReligion},
    {"IsFactionLeaderSet",   IsFactionLeaderSet},
    {"GetPortraitPath",      GetPortraitPath},
    {"GetSettingsId",        GetSettingsId},
    {nullptr, nullptr}
};

void register_art_set_methods(lua_State* L) {
    l_newmetatable(L, kArtSetMetatable);
    l_createtable(L, 0, 16);
    for (const luaL_Reg* f = art_set_methods; f->name; ++f) {
        l_pushstring(L, f->name);
        l_pushcclosure(L, f->func, 0);
        l_settable(L, -3);
    }
    l_setfield(L, -2, "__index");
    l_pop(L, 1);
    Log("[twdll] ARTSET_SCRIPT_INTERFACE registered");
}

void push_art_set(lua_State* L, TW_CharacterDetailsArtSetInfo* art_info) {
    if (!art_info) {
        l_pushnil(L);
        return;
    }
    twdll::tw_push_wrapped<TW_CharacterDetailsArtSetInfo>(L, art_info);
    l_newmetatable(L, kArtSetMetatable);
    l_setmetatable(L, -2);
}
