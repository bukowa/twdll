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
Art set record key ID from `campaign_character_art_sets_tables` (e.g. `"att_huns_general_01"`, `"att_general_nomadic_16"`).
@function GetKey
@treturn string art set record key
@usage
local art_key = art_set:GetKey()
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
Allocated culture key for this art set (e.g. `"att_cult_nomadic"`, `"att_cult_roman"`).
@function GetCulture
@treturn string culture database key
@usage
local culture = art_set:GetCulture()
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
Allocated subculture key for this art set (e.g. `"att_sub_cult_nomadic_hunnic"`).
@function GetSubculture
@treturn string subculture database key
@usage
local subculture = art_set:GetSubculture()
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
Allocated faction key for this art set (e.g. `"att_fact_hunni"`), or empty string if culture-wide.
@function GetFaction
@treturn string faction database key
@usage
local faction_key = art_set:GetFaction()
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
Allocated agent type key (e.g. `"general"`, `"champion"`, `"spy"`, `"dignitary"`).
@function GetAgent
@treturn string agent database key
@usage
if art_set:GetAgent() == "general" then
    -- Art set is designed for army commanders
end
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
Checks whether this art set is a custom unique set (e.g. Attila or historical general).
@function IsCustom
@treturn boolean true if this is a custom unique art set, false otherwise
@usage
if art_set:IsCustom() then
    -- Unique historical art set
end
*/
static int IsCustom(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    auto* rec = GetRecord(info);
    l_pushboolean(L, rec ? (rec->m_is_custom ? 1 : 0) : 0);
    return 1;
}

/***
Checks whether this art set represents a male character model.
@function IsMale
@treturn boolean true if male, false if female
@usage
local is_male = art_set:IsMale()
*/
static int IsMale(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    auto* rec = GetRecord(info);
    l_pushboolean(L, rec ? (rec->m_is_male ? 1 : 0) : 0);
    return 1;
}

/***
Checks whether this art set supports visual aging stages (young, middle-aged, old).
@function HasAging
@treturn boolean true if aging variations are enabled
@usage
local has_aging = art_set:HasAging()
*/
static int HasAging(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_aging_set) ? 1 : 0);
    return 1;
}

/***
Checks whether this art set supports seasonal visual variations (winter furs/gear).
@function HasSeasonal
@treturn boolean true if seasonal variations are enabled
@usage
local has_seasonal = art_set:HasSeasonal()
*/
static int HasSeasonal(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_seasonal_set) ? 1 : 0);
    return 1;
}

/***
Checks whether this art set supports rank-based visual upgrades (improved armor/regalia with character rank).
@function HasLevelling
@treturn boolean true if levelling variations are enabled
@usage
local has_levelling = art_set:HasLevelling()
*/
static int HasLevelling(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_levelling_set) ? 1 : 0);
    return 1;
}

/***
Checks whether this art set supports health/wound visual variations.
@function HasHealth
@treturn boolean true if health variation is enabled
@usage
local has_health = art_set:HasHealth()
*/
static int HasHealth(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_health_set) ? 1 : 0);
    return 1;
}

/***
Checks whether this art set supports religious visual variations.
@function HasReligion
@treturn boolean true if religion variation is enabled
@usage
local has_religion = art_set:HasReligion()
*/
static int HasReligion(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_religion_set) ? 1 : 0);
    return 1;
}

/***
Checks whether this art set is specifically configured for faction leaders (e.g. wearing crowns/diadems).
@function IsFactionLeaderSet
@treturn boolean true if configured as a faction leader art set
@usage
if art_set:IsFactionLeaderSet() then
    -- Character wears regal faction leader visual assets
end
*/
static int IsFactionLeaderSet(lua_State* L) {
    auto* info = twdll::tw_unwrap<TW_CharacterDetailsArtSetInfo>(L, 1);
    l_pushboolean(L, (info && info->m_art_set && info->m_art_set->m_faction_leader_set) ? 1 : 0);
    return 1;
}

/***
Resolved 2D portrait diffuse PNG file path used by the UI.
@function GetPortraitPath
@treturn string portrait diffuse file path (e.g. `"UI/Portraits/Portholes/att_cult_nomadic/att_frontend_faction_leader_huns_0.png"`)
@usage
local png_path = art_set:GetPortraitPath()
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
Unique 3D portrait camera settings ID string (e.g. `"att_huns_general_010"`).
@function GetSettingsId
@treturn string portrait settings unique identifier
@usage
local cam_id = art_set:GetSettingsId()
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
Campaign group key from `campaign_character_art_set_groups_tables` (e.g. `"main"`), or empty string if none.
@function GetGroup
@treturn string campaign group key
@usage
local group_key = art_set:GetGroup()
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
