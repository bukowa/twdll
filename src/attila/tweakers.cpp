/***
Engine tweakers and campaign variables runtime access.

Provides unified read/write access to all 3,731 internal engine variables:

- **Direct Tweakers (~2,341)**: Shaders, battle physics, camera, terrain and UI — updated in-place in engine memory.
- **KV Tweakers (518)**: Live database row cells in RAM (`TWEAKABLE_KV_ITEM`).
- **Campaign Variables (714)**: Routed directly to the active `CAMPAIGN_MODEL` (+0x1100) and `EMPIRE_DATABASES` (+0x0000).
- **Campaign AI Variables (158)**: CAI evaluator weights and priority thresholds.

All modified values automatically snapshot their original vanilla state and are cleanly
restored on Lua teardown / game reload.

For the complete catalog of all 3,731 variables, categories, and descriptions, see
[Complete Engine Tweakers Inventory](../topics/engine_tweakers_inventory.md.html).

### Usage:

```lua
-- 1. Direct property access:
twdll.tweakers.general_admiral_action_point_bonus.value = 500
twdll.tweakers.terrain_override_season.value = 3  -- 0=Spring, 1=Summer, 2=Autumn, 3=Winter
twdll.tweakers.AI_FORCE_ATTACK_PLAN.value = true

-- 2. Find and inspect a tweaker:
local t = twdll.tweakers.Find("max_traits")
if t then
    print(t.name)         --> "max_traits"
    print(t.category)     --> "all"
    print(t.description)  --> description string
    print(t.value)        --> 15
    t.value = 30          -- update value
    t:SetInt(30)          -- or via method
end

-- 3. Iterate all available tweakers:
for _, tw in ipairs(twdll.tweakers.GetList()) do
    if tw.category == "Campaign Variable Tweakers" then
        print(tw.name, tw.value)
    end
end
```

@module twdll.tweakers
*/
#include "../common/tw.h"
#include "../common/lua_api.h"
#include "../common/campaign_hooks.h"
#include "../common/log.h"
#include "game_api.h"
#include "tw_types.h"

#include <unordered_map>
#include <string>
#include <vector>
#include <cmath>

namespace {

static const char* kTweakerMetatable = "TWEAKER_SCRIPT_INTERFACE";

// 714 Campaign Variables mapping table (Total War: Attila 32-bit layout)
static const std::unordered_map<std::string, int> g_campaign_var_indices = {
#include "campaign_var_table.inc"
};

// Snapshot maps to ensure automatic state rollback on Lua teardown
static std::unordered_map<twdll::TW_ITweaker*, uint32_t> g_tweaker_snapshots;
static std::unordered_map<int, float> g_campaign_var_snapshots;

static void snapshot_tweaker_if_needed(twdll::TW_ITweaker* tweaker) {
    if (g_tweaker_snapshots.find(tweaker) == g_tweaker_snapshots.end()) {
        g_tweaker_snapshots[tweaker] = tweaker->raw_value();
    }
}

static std::string uni_to_utf8(const twdll::TW_CAUniString& str) {
    if (!str.m_data || str.m_len == 0) return "";
    return tw_wide_to_utf8(str.m_data, str.m_len);
}

static int get_campaign_var_index(twdll::TW_ITweaker* tweaker) {
    if (!tweaker) return -1;
    std::string name = uni_to_utf8(tweaker->m_name);
    auto it = g_campaign_var_indices.find(name);
    if (it != g_campaign_var_indices.end()) return it->second;
    return -1;
}

static int get_campaign_var_index_by_name(const char* name) {
    if (!name) return -1;
    auto it = g_campaign_var_indices.find(name);
    if (it != g_campaign_var_indices.end()) return it->second;
    return -1;
}

static void sync_campaign_var_to_engine(int var_idx, float val) {
    if (var_idx < 0 || var_idx >= 714) return;
    if (g_campaign_model) {
        auto* cm = static_cast<twdll::TW_CampaignModel*>(g_campaign_model);
        if (g_campaign_var_snapshots.find(var_idx) == g_campaign_var_snapshots.end()) {
            g_campaign_var_snapshots[var_idx] = cm->m_campaign_variables[var_idx];
        }
        cm->m_campaign_variables[var_idx] = val;
    }
    auto* dbs = twdll::TW_Databases::get();
    if (dbs) {
        if (g_campaign_var_snapshots.find(var_idx) == g_campaign_var_snapshots.end()) {
            g_campaign_var_snapshots[var_idx] = dbs->m_campaign_variables[var_idx];
        }
        dbs->m_campaign_variables[var_idx] = val;
    }
}

static bool get_campaign_var_runtime(int var_idx, float& out_val) {
    if (var_idx < 0 || var_idx >= 714) return false;
    if (g_campaign_model) {
        auto* cm = static_cast<twdll::TW_CampaignModel*>(g_campaign_model);
        out_val = cm->m_campaign_variables[var_idx];
        return true;
    }
    auto* dbs = twdll::TW_Databases::get();
    if (dbs) {
        out_val = dbs->m_campaign_variables[var_idx];
        return true;
    }
    return false;
}

} // anonymous namespace

void push_tweaker(lua_State* L, twdll::TW_ITweaker* tweaker) {
    if (!tweaker) {
        l_pushnil(L);
        return;
    }
    twdll::tw_push_wrapped<twdll::TW_ITweaker>(L, tweaker);
    l_newmetatable(L, kTweakerMetatable);
    l_setmetatable(L, -2);
}

void uninstall_tweakers() {
    for (auto& pair : g_tweaker_snapshots) {
        pair.first->set_raw_value(pair.second);
        std::string name = uni_to_utf8(pair.first->m_name);
        Log("[twdll] Restored tweaker '%s' to original value: 0x%08X", name.c_str(), pair.second);
    }
    g_tweaker_snapshots.clear();

    if (g_campaign_model) {
        auto* cm = static_cast<twdll::TW_CampaignModel*>(g_campaign_model);
        for (auto& pair : g_campaign_var_snapshots) {
            cm->m_campaign_variables[pair.first] = pair.second;
        }
    }
    auto* dbs = twdll::TW_Databases::get();
    if (dbs) {
        for (auto& pair : g_campaign_var_snapshots) {
            dbs->m_campaign_variables[pair.first] = pair.second;
        }
    }
    g_campaign_var_snapshots.clear();
}

// ============================================================================
// TWEAKER_SCRIPT_INTERFACE Methods
// ============================================================================

/***
Name of the tweaker (e.g. `"max_traits"`).
@function GetName
@treturn string tweaker name
*/
static int Tweaker_GetName(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    l_pushstring(L, uni_to_utf8(t->m_name).c_str());
    return 1;
}

/***
Category of the tweaker (e.g. `"Campaign Variable Tweakers"`, `"KV Tweakers"`).
@function GetCategory
@treturn string tweaker category
*/
static int Tweaker_GetCategory(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    l_pushstring(L, uni_to_utf8(t->m_category).c_str());
    return 1;
}

/***
Tooltip title of the tweaker.
@function GetTitle
@treturn string tweaker title
*/
static int Tweaker_GetTitle(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    l_pushstring(L, uni_to_utf8(t->m_tooltip_title).c_str());
    return 1;
}

/***
Detailed description / documentation tooltip from the engine developers.
@function GetDescription
@treturn string tweaker description
*/
static int Tweaker_GetDescription(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    l_pushstring(L, uni_to_utf8(t->m_tooltip_text).c_str());
    return 1;
}

/***
Source code filename in the CA repository where this tweaker was defined.
@function GetFile
@treturn string source code file path
*/
static int Tweaker_GetFile(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    l_pushstring(L, uni_to_utf8(t->m_file_name).c_str());
    return 1;
}

/***
Source code line number where this tweaker was defined.
@function GetLine
@treturn integer source line number
*/
static int Tweaker_GetLine(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    l_pushinteger(L, t->m_line_number);
    return 1;
}

/***
Gets the current integer value of the tweaker.
@function GetInt
@treturn integer current integer value
*/
static int Tweaker_GetInt(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    int var_idx = get_campaign_var_index(t);
    float cv = 0.0f;
    if (var_idx >= 0 && get_campaign_var_runtime(var_idx, cv)) {
        l_pushinteger(L, static_cast<lua_Integer>(cv));
    } else {
        l_pushinteger(L, t->value<int32_t>());
    }
    return 1;
}

/***
Sets the integer value of the tweaker with state rollback on teardown.
@function SetInt
@tparam integer value new integer value
@treturn boolean true on success
*/
static int Tweaker_SetInt(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushboolean(L, 0); return 1; }
    int32_t val = static_cast<int32_t>(l_tointeger(L, 2));
    snapshot_tweaker_if_needed(t);
    t->value<int32_t>() = val;
    t->m_dirty = 1;
    int var_idx = get_campaign_var_index(t);
    if (var_idx >= 0) {
        sync_campaign_var_to_engine(var_idx, static_cast<float>(val));
    }
    l_pushboolean(L, 1);
    return 1;
}

/***
Gets the current float value of the tweaker.
@function GetFloat
@treturn number current float value
*/
static int Tweaker_GetFloat(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    int var_idx = get_campaign_var_index(t);
    float cv = 0.0f;
    if (var_idx >= 0 && get_campaign_var_runtime(var_idx, cv)) {
        l_pushnumber(L, cv);
    } else {
        l_pushnumber(L, t->value<float>());
    }
    return 1;
}

/***
Sets the float value of the tweaker with state rollback on teardown.
@function SetFloat
@tparam number value new float value
@treturn boolean true on success
*/
static int Tweaker_SetFloat(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushboolean(L, 0); return 1; }
    float val = static_cast<float>(l_tonumber(L, 2));
    snapshot_tweaker_if_needed(t);
    t->value<float>() = val;
    t->m_dirty = 1;
    int var_idx = get_campaign_var_index(t);
    if (var_idx >= 0) {
        sync_campaign_var_to_engine(var_idx, val);
    }
    l_pushboolean(L, 1);
    return 1;
}

/***
Gets the current boolean flag value of the tweaker.
@function GetBool
@treturn boolean current boolean flag
*/
static int Tweaker_GetBool(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    int var_idx = get_campaign_var_index(t);
    float cv = 0.0f;
    if (var_idx >= 0 && get_campaign_var_runtime(var_idx, cv)) {
        l_pushboolean(L, cv != 0.0f ? 1 : 0);
    } else {
        l_pushboolean(L, t->value<uint8_t>() ? 1 : 0);
    }
    return 1;
}

/***
Sets the boolean flag value of the tweaker with state rollback on teardown.
@function SetBool
@tparam boolean value new boolean flag
@treturn boolean true on success
*/
static int Tweaker_SetBool(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushboolean(L, 0); return 1; }
    bool b = l_tobool(L, 2);
    snapshot_tweaker_if_needed(t);
    t->value<uint8_t>() = b ? 1 : 0;
    t->m_dirty = 1;
    int var_idx = get_campaign_var_index(t);
    if (var_idx >= 0) {
        sync_campaign_var_to_engine(var_idx, b ? 1.0f : 0.0f);
    }
    l_pushboolean(L, 1);
    return 1;
}

/***
Gets the raw 32-bit integer representation of the tweaker's memory value.
@function GetRawValue
@treturn integer raw 32-bit integer value
*/
static int Tweaker_GetRawValue(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    l_pushinteger(L, t->raw_value());
    return 1;
}

/***
Sets the raw 32-bit integer representation of the tweaker's memory value.
@function SetRawValue
@tparam integer value raw 32-bit integer value
@treturn boolean true on success
*/
static int Tweaker_SetRawValue(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushboolean(L, 0); return 1; }
    uint32_t val = static_cast<uint32_t>(l_tointeger(L, 2));
    snapshot_tweaker_if_needed(t);
    t->set_raw_value(val);
    int var_idx = get_campaign_var_index(t);
    if (var_idx >= 0) {
        sync_campaign_var_to_engine(var_idx, static_cast<float>(val));
    }
    l_pushboolean(L, 1);
    return 1;
}

/***
Gets the polymorphic value of the tweaker (auto-detected integer/float/bool).
@function GetValue
@treturn any current value
*/
static int Tweaker_GetValue(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    int var_idx = get_campaign_var_index(t);
    float cv = 0.0f;
    if (var_idx >= 0 && get_campaign_var_runtime(var_idx, cv)) {
        if (cv == static_cast<float>(static_cast<int32_t>(cv))) {
            l_pushinteger(L, static_cast<lua_Integer>(cv));
        } else {
            l_pushnumber(L, cv);
        }
        return 1;
    }
    l_pushinteger(L, t->value<int32_t>());
    return 1;
}

/***
Sets the polymorphic value of the tweaker with auto-detected type.
@function SetValue
@tparam any value new value
@treturn boolean true on success
*/
static int Tweaker_SetValue(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushboolean(L, 0); return 1; }
    snapshot_tweaker_if_needed(t);
    int var_idx = get_campaign_var_index(t);
    if (l_type(L, 2) == LUA_TBOOLEAN) {
        bool b = l_tobool(L, 2);
        t->value<uint8_t>() = b ? 1 : 0;
        if (var_idx >= 0) {
            sync_campaign_var_to_engine(var_idx, b ? 1.0f : 0.0f);
        }
    } else if (l_type(L, 2) == LUA_TNUMBER) {
        double d = l_tonumber(L, 2);
        if (d == static_cast<double>(static_cast<int32_t>(d))) {
            t->value<int32_t>() = static_cast<int32_t>(d);
        } else {
            t->value<float>() = static_cast<float>(d);
        }
        if (var_idx >= 0) {
            sync_campaign_var_to_engine(var_idx, static_cast<float>(d));
        }
    }
    t->m_dirty = 1;
    l_pushboolean(L, 1);
    return 1;
}

static const luaL_Reg tweaker_object_methods[] = {
    {"GetName",        Tweaker_GetName},
    {"GetCategory",    Tweaker_GetCategory},
    {"GetTitle",       Tweaker_GetTitle},
    {"GetDescription", Tweaker_GetDescription},
    {"GetFile",        Tweaker_GetFile},
    {"GetLine",        Tweaker_GetLine},
    {"GetInt",         Tweaker_GetInt},
    {"SetInt",         Tweaker_SetInt},
    {"GetFloat",       Tweaker_GetFloat},
    {"SetFloat",       Tweaker_SetFloat},
    {"GetBool",        Tweaker_GetBool},
    {"SetBool",        Tweaker_SetBool},
    {"GetRawValue",    Tweaker_GetRawValue},
    {"SetRawValue",    Tweaker_SetRawValue},
    {"GetValue",       Tweaker_GetValue},
    {"SetValue",       Tweaker_SetValue},
    {nullptr,          nullptr}
};

// __index metamethod for TWEAKER_SCRIPT_INTERFACE (enables t.name, t.value, t.category etc.)
static int Tweaker_MetaIndex(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) { l_pushnil(L); return 1; }
    size_t key_len = 0;
    const char* key = l_checklstring(L, 2, &key_len);
    if (!key) { l_pushnil(L); return 1; }

    // Check methods first:
    for (const luaL_Reg* f = tweaker_object_methods; f->name; ++f) {
        if (std::strcmp(f->name, key) == 0) {
            l_pushcclosure(L, f->func, 0);
            return 1;
        }
    }

    // Properties:
    if (std::strcmp(key, "name") == 0) {
        l_pushstring(L, uni_to_utf8(t->m_name).c_str());
        return 1;
    }
    if (std::strcmp(key, "category") == 0) {
        l_pushstring(L, uni_to_utf8(t->m_category).c_str());
        return 1;
    }
    if (std::strcmp(key, "title") == 0) {
        l_pushstring(L, uni_to_utf8(t->m_tooltip_title).c_str());
        return 1;
    }
    if (std::strcmp(key, "description") == 0) {
        l_pushstring(L, uni_to_utf8(t->m_tooltip_text).c_str());
        return 1;
    }
    if (std::strcmp(key, "file") == 0) {
        l_pushstring(L, uni_to_utf8(t->m_file_name).c_str());
        return 1;
    }
    if (std::strcmp(key, "line") == 0) {
        l_pushinteger(L, t->m_line_number);
        return 1;
    }
    if (std::strcmp(key, "raw_value") == 0) {
        l_pushinteger(L, t->raw_value());
        return 1;
    }
    if (std::strcmp(key, "value") == 0 || std::strcmp(key, "int") == 0) {
        int var_idx = get_campaign_var_index(t);
        float cv = 0.0f;
        if (var_idx >= 0 && get_campaign_var_runtime(var_idx, cv)) {
            if (std::strcmp(key, "value") == 0 && cv != static_cast<float>(static_cast<int32_t>(cv))) {
                l_pushnumber(L, cv);
            } else {
                l_pushinteger(L, static_cast<lua_Integer>(cv));
            }
        } else {
            l_pushinteger(L, t->value<int32_t>());
        }
        return 1;
    }
    if (std::strcmp(key, "float") == 0) {
        int var_idx = get_campaign_var_index(t);
        float cv = 0.0f;
        if (var_idx >= 0 && get_campaign_var_runtime(var_idx, cv)) {
            l_pushnumber(L, cv);
        } else {
            l_pushnumber(L, t->value<float>());
        }
        return 1;
    }
    if (std::strcmp(key, "bool") == 0) {
        int var_idx = get_campaign_var_index(t);
        float cv = 0.0f;
        if (var_idx >= 0 && get_campaign_var_runtime(var_idx, cv)) {
            l_pushboolean(L, cv != 0.0f ? 1 : 0);
        } else {
            l_pushboolean(L, t->value<uint8_t>() ? 1 : 0);
        }
        return 1;
    }

    l_pushnil(L);
    return 1;
}

// __newindex metamethod for TWEAKER_SCRIPT_INTERFACE (enables t.value = 30, t.float = 1.5 etc.)
static int Tweaker_MetaNewIndex(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) return 0;
    size_t key_len = 0;
    const char* key = l_checklstring(L, 2, &key_len);
    if (!key) return 0;

    snapshot_tweaker_if_needed(t);
    int var_idx = get_campaign_var_index(t);

    if (std::strcmp(key, "value") == 0) {
        if (l_type(L, 3) == LUA_TBOOLEAN) {
            bool b = l_tobool(L, 3);
            t->value<uint8_t>() = b ? 1 : 0;
            if (var_idx >= 0) sync_campaign_var_to_engine(var_idx, b ? 1.0f : 0.0f);
        } else if (l_type(L, 3) == LUA_TNUMBER) {
            double d = l_tonumber(L, 3);
            if (d == static_cast<double>(static_cast<int32_t>(d))) {
                t->value<int32_t>() = static_cast<int32_t>(d);
            } else {
                t->value<float>() = static_cast<float>(d);
            }
            if (var_idx >= 0) sync_campaign_var_to_engine(var_idx, static_cast<float>(d));
        }
        t->m_dirty = 1;
        return 0;
    }
    if (std::strcmp(key, "int") == 0) {
        int32_t val = static_cast<int32_t>(l_tointeger(L, 3));
        t->value<int32_t>() = val;
        t->m_dirty = 1;
        if (var_idx >= 0) sync_campaign_var_to_engine(var_idx, static_cast<float>(val));
        return 0;
    }
    if (std::strcmp(key, "float") == 0) {
        float val = static_cast<float>(l_tonumber(L, 3));
        t->value<float>() = val;
        t->m_dirty = 1;
        if (var_idx >= 0) sync_campaign_var_to_engine(var_idx, val);
        return 0;
    }
    if (std::strcmp(key, "bool") == 0) {
        bool b = l_tobool(L, 3);
        t->value<uint8_t>() = b ? 1 : 0;
        t->m_dirty = 1;
        if (var_idx >= 0) sync_campaign_var_to_engine(var_idx, b ? 1.0f : 0.0f);
        return 0;
    }
    if (std::strcmp(key, "raw_value") == 0) {
        uint32_t val = static_cast<uint32_t>(l_tointeger(L, 3));
        t->set_raw_value(val);
        if (var_idx >= 0) sync_campaign_var_to_engine(var_idx, static_cast<float>(val));
        return 0;
    }

    return 0;
}

static int Tweaker_ToString(lua_State* L) {
    auto* t = twdll::tw_unwrap<twdll::TW_ITweaker>(L, 1);
    if (!t) {
        l_pushstring(L, "[Tweaker: null]");
        return 1;
    }
    char buf[512];
    std::string name = uni_to_utf8(t->m_name);
    std::string cat  = uni_to_utf8(t->m_category);
    std::snprintf(buf, sizeof(buf), "[Tweaker: '%s' | Category: '%s' | Value: %u]",
                  name.c_str(), cat.c_str(), t->raw_value());
    l_pushstring(L, buf);
    return 1;
}

// ============================================================================
// twdll.tweakers Module Functions
// ============================================================================

/***
Finds an engine tweaker object by name.
@function Find
@tparam string name tweaker name (e.g. "max_traits", "AI_FORCE_ATTACK_PLAN")
@treturn TWEAKER_SCRIPT_INTERFACE|nil tweaker object, or nil if not found
@usage local t = twdll.tweakers.Find("max_traits")
t.value = 30
*/
static int Find(lua_State* L) {
    size_t len = 0;
    const char* name = l_checklstring(L, 1, &len);
    auto* t = find_engine_tweaker(name, len);
    if (!t) {
        l_pushnil(L);
        return 1;
    }
    push_tweaker(L, t);
    return 1;
}

/***
Returns a list of all registered engine tweakers.
@function GetList
@treturn table array of TWEAKER_SCRIPT_INTERFACE objects
@usage local list = twdll.tweakers.GetList()
for i, t in ipairs(list) do
    print(t.name, t.category, t.value)
end
*/
static int GetList(lua_State* L) {
    if (!g_tweaker_map || !g_tweaker_map->m_buckets) {
        find_engine_tweaker("max_traits", 10);
    }
    if (!g_tweaker_map || !g_tweaker_map->m_buckets) {
        l_newtable(L);
        return 1;
    }

    l_createtable(L, g_tweaker_map->m_capacity, 0);

    int idx = 1;
    for (uint32_t i = 0; i < g_tweaker_map->m_capacity; ++i) {
        auto& entry = g_tweaker_map->m_buckets[i];
        if (entry.m_tweaker && entry.m_key.m_data && entry.m_key.m_len > 0) {
            l_pushinteger(L, idx++);
            push_tweaker(L, entry.m_tweaker);
            l_settable(L, -3);
        }
    }
    return 1;
}

/***
Dumps all registered engine tweakers from the global registry to twdll.log.
@function Dump
@treturn integer total count of registered engine tweakers discovered
@usage twdll.tweakers.Dump()
*/
static int Dump(lua_State* L) {
    if (!g_tweaker_map || !g_tweaker_map->m_buckets) {
        find_engine_tweaker("max_traits", 10);
    }
    if (!g_tweaker_map || !g_tweaker_map->m_buckets) {
        Log("[twdll] tweakers:Dump failed — tweaker map not resolved");
        l_pushinteger(L, 0);
        return 1;
    }

    int count = 0;
    Log("[twdll] ==================== ENGINE TWEAKERS DUMP ====================");
    Log("[twdll] Tweaker map capacity: %u buckets", g_tweaker_map->m_capacity);

    for (uint32_t i = 0; i < g_tweaker_map->m_capacity; ++i) {
        auto& entry = g_tweaker_map->m_buckets[i];
        if (entry.m_tweaker && entry.m_key.m_data && entry.m_key.m_len > 0) {
            auto* t = entry.m_tweaker;
            std::string name = uni_to_utf8(t->m_name);
            std::string cat = uni_to_utf8(t->m_category);
            std::string desc = uni_to_utf8(t->m_tooltip_text);
            uint32_t raw_val = t->raw_value();
            float float_val = t->value<float>();

            Log("[twdll] [%d] Name: '%s' | Category: '%s' | Value: %u (float: %.3f) | Desc: '%s'",
                count + 1, name.c_str(), cat.c_str(), raw_val, float_val, desc.c_str());
            count++;
        }
    }

    Log("[twdll] ==================== END TWEAKERS DUMP (TOTAL: %d) ====================", count);
    l_pushinteger(L, count);
    return 1;
}

/***
Gets the integer value of an engine tweaker by name.
@function GetInt
@tparam string name tweaker name
@treturn integer|nil current value, or nil if not found
*/
static int Mod_GetInt(lua_State* L) {
    size_t len = 0;
    const char* name = l_checklstring(L, 1, &len);
    int var_idx = get_campaign_var_index_by_name(name);
    float cv = 0.0f;
    if (var_idx >= 0 && get_campaign_var_runtime(var_idx, cv)) {
        l_pushinteger(L, static_cast<lua_Integer>(cv));
        return 1;
    }
    auto* t = find_engine_tweaker(name, len);
    if (!t) { l_pushnil(L); return 1; }
    l_pushinteger(L, t->value<int32_t>());
    return 1;
}

/***
Sets the integer value of an engine tweaker with state rollback on teardown.
@function SetInt
@tparam string name tweaker name
@tparam integer value new integer value
@treturn boolean true on success
*/
static int Mod_SetInt(lua_State* L) {
    size_t len = 0;
    const char* name = l_checklstring(L, 1, &len);
    int32_t val = static_cast<int32_t>(l_tointeger(L, 2));
    int var_idx = get_campaign_var_index_by_name(name);
    if (var_idx >= 0) {
        sync_campaign_var_to_engine(var_idx, static_cast<float>(val));
    }
    auto* t = find_engine_tweaker(name, len);
    if (t) {
        snapshot_tweaker_if_needed(t);
        t->value<int32_t>() = val;
        t->m_dirty = 1;
    }
    l_pushboolean(L, 1);
    return 1;
}

/***
Gets the float value of an engine tweaker by name.
@function GetFloat
@tparam string name tweaker name
@treturn number|nil current float value, or nil if not found
*/
static int Mod_GetFloat(lua_State* L) {
    size_t len = 0;
    const char* name = l_checklstring(L, 1, &len);
    int var_idx = get_campaign_var_index_by_name(name);
    float cv = 0.0f;
    if (var_idx >= 0 && get_campaign_var_runtime(var_idx, cv)) {
        l_pushnumber(L, cv);
        return 1;
    }
    auto* t = find_engine_tweaker(name, len);
    if (!t) { l_pushnil(L); return 1; }
    l_pushnumber(L, t->value<float>());
    return 1;
}

/***
Sets the float value of an engine tweaker with state rollback on teardown.
@function SetFloat
@tparam string name tweaker name
@tparam number value new float value
@treturn boolean true on success
*/
static int Mod_SetFloat(lua_State* L) {
    size_t len = 0;
    const char* name = l_checklstring(L, 1, &len);
    float val = static_cast<float>(l_tonumber(L, 2));
    int var_idx = get_campaign_var_index_by_name(name);
    if (var_idx >= 0) {
        sync_campaign_var_to_engine(var_idx, val);
    }
    auto* t = find_engine_tweaker(name, len);
    if (t) {
        snapshot_tweaker_if_needed(t);
        t->value<float>() = val;
        t->m_dirty = 1;
    }
    if (var_idx < 0 && !t) {
        l_pushboolean(L, 0);
        return 1;
    }
    l_pushboolean(L, 1);
    return 1;
}

/***
Gets the boolean value of an engine tweaker by name.
@function GetBool
@tparam string name tweaker name
@treturn boolean|nil current boolean value, or nil if not found
*/
static int Mod_GetBool(lua_State* L) {
    size_t len = 0;
    const char* name = l_checklstring(L, 1, &len);
    int var_idx = get_campaign_var_index_by_name(name);
    float cv = 0.0f;
    if (var_idx >= 0 && get_campaign_var_runtime(var_idx, cv)) {
        l_pushboolean(L, cv != 0.0f ? 1 : 0);
        return 1;
    }
    auto* t = find_engine_tweaker(name, len);
    if (!t) { l_pushnil(L); return 1; }
    l_pushboolean(L, t->value<uint8_t>() ? 1 : 0);
    return 1;
}

/***
Sets the boolean value of an engine tweaker with state rollback on teardown.
@function SetBool
@tparam string name tweaker name
@tparam boolean value new boolean flag
@treturn boolean true on success
*/
static int Mod_SetBool(lua_State* L) {
    size_t len = 0;
    const char* name = l_checklstring(L, 1, &len);
    bool b = l_tobool(L, 2);
    int var_idx = get_campaign_var_index_by_name(name);
    if (var_idx >= 0) {
        sync_campaign_var_to_engine(var_idx, b ? 1.0f : 0.0f);
    }
    auto* t = find_engine_tweaker(name, len);
    if (t) {
        snapshot_tweaker_if_needed(t);
        t->value<uint8_t>() = b ? 1 : 0;
        t->m_dirty = 1;
    }
    if (var_idx < 0 && !t) {
        l_pushboolean(L, 0);
        return 1;
    }
    l_pushboolean(L, 1);
    return 1;
}

extern const luaL_Reg tweaker_functions[] = {
    {"Find",     Find},
    {"GetList",  GetList},
    {"Dump",     Dump},
    {"GetInt",   Mod_GetInt},
    {"SetInt",   Mod_SetInt},
    {"GetFloat", Mod_GetFloat},
    {"SetFloat", Mod_SetFloat},
    {"GetBool",  Mod_GetBool},
    {"SetBool",  Mod_SetBool},
    {nullptr,    nullptr}
};

// __index metamethod on the twdll.tweakers module table (enables twdll.tweakers.max_traits.value = 30)
static int Tweakers_ModuleMetaIndex(lua_State* L) {
    size_t key_len = 0;
    const char* key = l_checklstring(L, 2, &key_len);
    if (!key) { l_pushnil(L); return 1; }

    // Check module functions first:
    for (const luaL_Reg* f = tweaker_functions; f->name; ++f) {
        if (std::strcmp(f->name, key) == 0) {
            l_pushcclosure(L, f->func, 0);
            return 1;
        }
    }

    // If not a function, look up tweaker by name:
    auto* t = find_engine_tweaker(key, key_len);
    if (t) {
        push_tweaker(L, t);
        return 1;
    }

    l_pushnil(L);
    return 1;
}

// __newindex metamethod on the twdll.tweakers module table (enables twdll.tweakers.max_traits = 30)
static int Tweakers_ModuleMetaNewIndex(lua_State* L) {
    size_t key_len = 0;
    const char* key = l_checklstring(L, 2, &key_len);
    if (!key) return 0;
    int var_idx = get_campaign_var_index_by_name(key);
    auto* t = find_engine_tweaker(key, key_len);
    if (t) {
        snapshot_tweaker_if_needed(t);
        if (l_type(L, 3) == LUA_TBOOLEAN) {
            bool b = l_tobool(L, 3);
            t->value<uint8_t>() = b ? 1 : 0;
            if (var_idx >= 0) sync_campaign_var_to_engine(var_idx, b ? 1.0f : 0.0f);
        } else if (l_type(L, 3) == LUA_TNUMBER) {
            double d = l_tonumber(L, 3);
            if (d == static_cast<double>(static_cast<int32_t>(d))) {
                t->value<int32_t>() = static_cast<int32_t>(d);
            } else {
                t->value<float>() = static_cast<float>(d);
            }
            if (var_idx >= 0) sync_campaign_var_to_engine(var_idx, static_cast<float>(d));
        }
        t->m_dirty = 1;
        return 0;
    } else if (var_idx >= 0) {
        if (l_type(L, 3) == LUA_TBOOLEAN) {
            sync_campaign_var_to_engine(var_idx, l_tobool(L, 3) ? 1.0f : 0.0f);
        } else if (l_type(L, 3) == LUA_TNUMBER) {
            sync_campaign_var_to_engine(var_idx, static_cast<float>(l_tonumber(L, 3)));
        }
        return 0;
    }
    return 0;
}

void register_tweaker_methods(lua_State* L) {
    // Register TWEAKER_SCRIPT_INTERFACE metatable
    l_newmetatable(L, kTweakerMetatable);
    l_pushcclosure(L, Tweaker_MetaIndex, 0);
    l_setfield(L, -2, "__index");
    l_pushcclosure(L, Tweaker_MetaNewIndex, 0);
    l_setfield(L, -2, "__newindex");
    l_pushcclosure(L, Tweaker_ToString, 0);
    l_setfield(L, -2, "__tostring");
    l_pop(L, 1);

    // Apply metatable to the twdll.tweakers table currently at top of stack
    l_createtable(L, 0, 2);
    l_pushcclosure(L, Tweakers_ModuleMetaIndex, 0);
    l_setfield(L, -2, "__index");
    l_pushcclosure(L, Tweakers_ModuleMetaNewIndex, 0);
    l_setfield(L, -2, "__newindex");
    l_setmetatable(L, -2);

    Log("[twdll] TWEAKER_SCRIPT_INTERFACE and twdll.tweakers metatables registered");
}
