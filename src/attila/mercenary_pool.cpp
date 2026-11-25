/// @module MERCENARY_POOLS
/// Mercenary pool methods on the game's region and faction objects (Attila).
///
/// Every province has a mercenary pool and every non-rebel faction has a faction-unique
/// pool. These methods are available on `REGION_SCRIPT_INTERFACE` objects (the pool of the
/// region's province) and on `FACTION_SCRIPT_INTERFACE` objects (the faction-unique pool):
///
///     region:GetMercenaryPool()                faction:GetMercenaryPool()
///     region:AddMercenaryUnits(key, n[, opts]) faction:AddMercenaryUnits(key, n[, opts])
///     region:RemoveMercenaryUnits(key, n)      faction:RemoveMercenaryUnits(key, n)
///     region:SetMercenaryUnitCap(key, cap)     faction:SetMercenaryUnitCap(key, cap)
///     region:SetMercenaryUnitReplenishment(key, chance, max_per_turn)  (and on faction)
///
/// Pools are part of the campaign model and are saved with the game, so changes persist.
/// At the start of each turn the engine sets every unit's current cap to
/// `base cap x cap modifier` (effects `mercenary_province_pool_cap_mod` /
/// `mercenary_faction_pool_cap_mod`), clamps the available count to it and replenishes.
///
/// Multiplayer: call these from campaign model events that fire on every machine
/// (e.g. `FactionTurnStart`, `RegionTurnStart`), never from UI events such as clicks,
/// which only fire for one player and would desync the campaign.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

#include <cmath>
#include <cstddef>
#include <windows.h>

using twdll::TW_Databases;
using twdll::TW_Faction;
using twdll::TW_Region;

namespace {

// Offsets for empire.retail.dll (Attila 1.6.0, x86), from the engine's save code:
//   FACTION::save   passes FACTION+0xEBC  to the FACTION_MERCENARY_POOL section writer
//   PROVINCE::save  passes PROVINCE+0x60  to the PROVINCE_MERCENARY_POOL section writer
//   REGION::faction_province_manager reads the province through the link at REGION+0x350
// Every access is validated (owner back-pointer, vtable in the game image, vector bounds)
// before anything is read or written, so a different game build fails safely.
constexpr size_t kFactionPoolOffset   = 0xEBC;
constexpr size_t kProvincePoolOffset  = 0x60;
constexpr size_t kRegionProvinceLink  = 0x350;

// EMPIRECAMPAIGN::MERCENARY_POOL_UNIT (0x2C bytes)
struct TW_MercPoolUnit {
    uint32_t    m_id;                         // 0x00
    const void* m_unit_record;                // 0x04 MAIN_UNIT_RECORD*
    uint32_t    m_unit_count;                 // 0x08 available now
    float       m_chance_to_replenish;        // 0x0C
    uint32_t    m_base_max_count;             // 0x10 cap before modifiers
    uint32_t    m_current_max_count;          // 0x14 cap after modifiers
    uint32_t    m_max_replenish_per_turn;     // 0x18
    uint32_t    m_unit_experience;            // 0x1C
    const void* m_faction_restricted_record;  // 0x20
    const void* m_subculture_restricted_record; // 0x24
    const void* m_technology_restricted_record; // 0x28
};
static_assert(sizeof(TW_MercPoolUnit) == 0x2C, "MERCENARY_POOL_UNIT size");

// FACTION_MERCENARY_POOL / PROVINCE_MERCENARY_POOL
struct TW_MercPool {
    void*            m_vtable;     // 0x00
    uint32_t         m_index;      // 0x04 serialisable index
    void*            m_table;      // 0x08
    void*            m_env;        // 0x0C CAMPAIGN_ENV_MODEL_ACCESS
    uint32_t         m_capacity;   // 0x10 CA_STD::VECTOR<MERCENARY_POOL_UNIT>
    uint32_t         m_size;       // 0x14
    TW_MercPoolUnit* m_elements;   // 0x18
    const void*      m_owner;      // 0x1C m_faction / m_province
};
static_assert(offsetof(TW_MercPool, m_owner) == 0x1C, "pool owner offset");

bool readable(const void* p, size_t n) {
    if (!p) return false;
    MEMORY_BASIC_INFORMATION mbi = {};
    if (!VirtualQuery(p, &mbi, sizeof(mbi)) || mbi.State != MEM_COMMIT) return false;
    if (mbi.Protect & (PAGE_NOACCESS | PAGE_GUARD)) return false;
    const auto end = reinterpret_cast<uintptr_t>(mbi.BaseAddress) + mbi.RegionSize;
    return reinterpret_cast<uintptr_t>(p) + n <= end;
}

bool in_game_image(const void* p) {
    if (!g_empire_module) return false;
    const auto base = reinterpret_cast<uintptr_t>(g_empire_module);
    const auto* dos = reinterpret_cast<const IMAGE_DOS_HEADER*>(base);
    const auto* nt  = reinterpret_cast<const IMAGE_NT_HEADERS*>(base + dos->e_lfanew);
    const auto  v   = reinterpret_cast<uintptr_t>(p);
    return v >= base && v < base + nt->OptionalHeader.SizeOfImage;
}

TW_MercPool* checked_pool(void* owner, size_t offset, const char* what) {
    if (!owner) return nullptr;
    auto* pool = reinterpret_cast<TW_MercPool*>(static_cast<char*>(owner) + offset);
    if (!readable(pool, sizeof(TW_MercPool))) {
        Log("[twdll] mercenary pool: %s pool memory not readable", what);
        return nullptr;
    }
    if (pool->m_owner != owner || !in_game_image(pool->m_vtable) ||
        pool->m_size > pool->m_capacity || pool->m_capacity > 4096 ||
        (pool->m_size && !readable(pool->m_elements, pool->m_size * sizeof(TW_MercPoolUnit)))) {
        Log("[twdll] mercenary pool: %s pool layout not recognised (other game build?) - not touched", what);
        return nullptr;
    }
    return pool;
}

// self (arg 1) is either a REGION or a FACTION script object.
TW_MercPool* pool_from_self(lua_State* L, bool is_faction) {
    if (is_faction) {
        auto* faction = twdll::tw_unwrap<TW_Faction>(L, 1);
        return checked_pool(faction, kFactionPoolOffset, "faction");
    }
    auto* region = twdll::tw_unwrap<TW_Region>(L, 1);
    if (!region) return nullptr;
    auto* link = *reinterpret_cast<void**>(reinterpret_cast<char*>(region) + kRegionProvinceLink + 4);
    if (!readable(link, sizeof(void*))) return nullptr;
    void* province = *static_cast<void**>(link);
    return checked_pool(province, kProvincePoolOffset, "province");
}

const char* unit_key_of(const TW_MercPoolUnit& u) {
    auto* rec = static_cast<const twdll::TW_MainUnitRecord*>(u.m_unit_record);
    return (rec && rec->m_key.m_data) ? rec->m_key.m_data : "";
}

const void* find_unit_record(lua_State* L, int idx, const char* fn) {
    size_t len = 0;
    const char* key = l_type(L, idx) == LUA_TSTRING ? l_checklstring(L, idx, &len) : nullptr;
    if (!key || !len) { Log("[twdll] %s: unit_key must be a string", fn); return nullptr; }
    auto* dbs = TW_Databases::get();
    if (!dbs || !dbs->main_units) { Log("[twdll] %s: main_units table not loaded", fn); return nullptr; }
    const void* rec = dbs->main_units->find_record(key, len);
    if (!rec) Log("[twdll] %s: no main_units record '%s'", fn, key);
    return rec;
}

TW_MercPoolUnit* find_entry(TW_MercPool* pool, const void* rec) {
    for (uint32_t i = 0; i < pool->m_size; ++i)
        if (pool->m_elements[i].m_unit_record == rec) return &pool->m_elements[i];
    return nullptr;
}

// Appends an entry the way the engine's own push_back does (capacity doubles, buffer
// from the game heap, old buffer freed), so the game can later free it itself.
TW_MercPoolUnit* append_entry(TW_MercPool* pool, const TW_MercPoolUnit& value) {
    if (!g_merc_alloc_units || !g_ca_free) {
        Log("[twdll] AddMercenaryUnits: allocator signatures not resolved");
        return nullptr;
    }
    if (pool->m_size == pool->m_capacity) {
        const uint32_t new_cap = pool->m_capacity ? pool->m_capacity * 2 : 1;
        auto* buf = static_cast<TW_MercPoolUnit*>(g_merc_alloc_units(static_cast<int>(new_cap)));
        if (!buf) return nullptr;
        if (pool->m_size) memcpy(buf, pool->m_elements, pool->m_size * sizeof(TW_MercPoolUnit));
        TW_MercPoolUnit* old = pool->m_elements;
        pool->m_elements = buf;
        pool->m_capacity = new_cap;
        if (old) g_ca_free(old);
    }
    pool->m_elements[pool->m_size] = value;
    return &pool->m_elements[pool->m_size++];
}

uint32_t opt_uint(lua_State* L, int t, const char* k, uint32_t def) {
    if (l_type(L, t) != LUA_TTABLE) return def;
    l_getfield(L, t, k);
    uint32_t v = def;
    if (l_type(L, -1) == LUA_TNUMBER) {
        const lua_Integer i = l_tointeger(L, -1);
        v = i < 0 ? 0u : static_cast<uint32_t>(i);
    }
    l_pop(L, 1);
    return v;
}

float opt_float(lua_State* L, int t, const char* k, float def) {
    if (l_type(L, t) != LUA_TTABLE) return def;
    l_getfield(L, t, k);
    float v = def;
    if (l_type(L, -1) == LUA_TNUMBER) v = static_cast<float>(l_tonumber(L, -1));
    l_pop(L, 1);
    return v;
}

uint32_t arg_uint(lua_State* L, int idx) {
    if (l_type(L, idx) != LUA_TNUMBER) return 0;
    const lua_Integer i = l_tointeger(L, idx);
    return i < 0 ? 0u : static_cast<uint32_t>(i);
}

void push_entry(lua_State* L, const TW_MercPoolUnit& u) {
    l_createtable(L, 0, 7);
    l_pushstring(L, unit_key_of(u));                                 l_setfield(L, -2, "unit");
    l_pushinteger(L, u.m_unit_count);                                l_setfield(L, -2, "count");
    l_pushinteger(L, u.m_current_max_count);                         l_setfield(L, -2, "cap");
    l_pushinteger(L, u.m_base_max_count);                            l_setfield(L, -2, "base_cap");
    l_pushnumber(L, u.m_chance_to_replenish);                        l_setfield(L, -2, "replenish_chance");
    l_pushinteger(L, u.m_max_replenish_per_turn);                    l_setfield(L, -2, "max_replenish");
    l_pushinteger(L, u.m_unit_experience);                           l_setfield(L, -2, "experience");
}

// ---- shared implementations (is_faction selects the pool) ---------------------------

int impl_get(lua_State* L, bool is_faction) {
    TW_MercPool* pool = pool_from_self(L, is_faction);
    if (!pool) { l_pushnil(L); return 1; }
    l_createtable(L, static_cast<int>(pool->m_size), 0);
    for (uint32_t i = 0; i < pool->m_size; ++i) {
        l_pushinteger(L, static_cast<lua_Integer>(i) + 1);
        push_entry(L, pool->m_elements[i]);
        l_settable(L, -3);
    }
    return 1;
}

int impl_add(lua_State* L, bool is_faction) {
    TW_MercPool* pool = pool_from_self(L, is_faction);
    const void* rec = pool ? find_unit_record(L, 2, "AddMercenaryUnits") : nullptr;
    if (!rec) { l_pushnil(L); return 1; }
    const uint32_t n = arg_uint(L, 3);

    TW_MercPoolUnit* e = find_entry(pool, rec);
    if (!e) {
        TW_MercPoolUnit v = {};
        v.m_id                     = pool->m_size;  // same rule as the engine's own pool builder
        v.m_unit_record            = rec;
        v.m_base_max_count         = opt_uint(L, 4, "cap", n);
        v.m_current_max_count      = v.m_base_max_count;
        v.m_chance_to_replenish    = opt_float(L, 4, "replenish_chance", 0.0f);
        v.m_max_replenish_per_turn = opt_uint(L, 4, "max_replenish", 0);
        v.m_unit_count             = n < v.m_current_max_count ? n : v.m_current_max_count;
        e = append_entry(pool, v);
        if (!e) { l_pushnil(L); return 1; }
        Log("[twdll] AddMercenaryUnits: new %s pool entry '%s' count %u cap %u",
            is_faction ? "faction" : "province", unit_key_of(*e), e->m_unit_count, e->m_base_max_count);
    } else {
        const uint64_t sum = static_cast<uint64_t>(e->m_unit_count) + n;
        e->m_unit_count = sum > e->m_current_max_count ? e->m_current_max_count : static_cast<uint32_t>(sum);
        Log("[twdll] AddMercenaryUnits: '%s' count now %u (cap %u)", unit_key_of(*e), e->m_unit_count, e->m_current_max_count);
    }
    l_pushinteger(L, e->m_unit_count);
    return 1;
}

int impl_remove(lua_State* L, bool is_faction) {
    TW_MercPool* pool = pool_from_self(L, is_faction);
    const void* rec = pool ? find_unit_record(L, 2, "RemoveMercenaryUnits") : nullptr;
    TW_MercPoolUnit* e = rec ? find_entry(pool, rec) : nullptr;
    if (!e) { l_pushnil(L); return 1; }
    const uint32_t n = arg_uint(L, 3);
    e->m_unit_count = n >= e->m_unit_count ? 0 : e->m_unit_count - n;
    Log("[twdll] RemoveMercenaryUnits: '%s' count now %u", unit_key_of(*e), e->m_unit_count);
    l_pushinteger(L, e->m_unit_count);
    return 1;
}

int impl_set_cap(lua_State* L, bool is_faction) {
    TW_MercPool* pool = pool_from_self(L, is_faction);
    const void* rec = pool ? find_unit_record(L, 2, "SetMercenaryUnitCap") : nullptr;
    TW_MercPoolUnit* e = rec ? find_entry(pool, rec) : nullptr;
    if (!e) { l_pushnil(L); return 1; }
    const uint32_t cap = arg_uint(L, 3);
    // Keep the current cap modifier (current/base) until the engine recomputes it next turn.
    uint32_t current = cap;
    if (e->m_base_max_count > 0) {
        const double ratio = static_cast<double>(e->m_current_max_count) / e->m_base_max_count;
        current = static_cast<uint32_t>(std::floor(cap * ratio + 0.5));
    }
    e->m_base_max_count    = cap;
    e->m_current_max_count = current;
    if (e->m_unit_count > current) e->m_unit_count = current;
    Log("[twdll] SetMercenaryUnitCap: '%s' base cap %u, current cap %u, count %u",
        unit_key_of(*e), cap, current, e->m_unit_count);
    l_pushinteger(L, current);
    return 1;
}

int impl_set_replenishment(lua_State* L, bool is_faction) {
    TW_MercPool* pool = pool_from_self(L, is_faction);
    const void* rec = pool ? find_unit_record(L, 2, "SetMercenaryUnitReplenishment") : nullptr;
    TW_MercPoolUnit* e = rec ? find_entry(pool, rec) : nullptr;
    if (!e) { l_pushboolean(L, 0); return 1; }
    float chance = l_type(L, 3) == LUA_TNUMBER ? static_cast<float>(l_tonumber(L, 3)) : 0.0f;
    if (chance < 0.0f) chance = 0.0f;
    e->m_chance_to_replenish    = chance;
    e->m_max_replenish_per_turn = arg_uint(L, 4);
    Log("[twdll] SetMercenaryUnitReplenishment: '%s' chance %.3f max %u",
        unit_key_of(*e), chance, e->m_max_replenish_per_turn);
    l_pushboolean(L, 1);
    return 1;
}

} // namespace

/***
Returns the units in the mercenary pool: for a region, its province's pool; for a faction,
its faction-unique pool.
@function GetMercenaryPool
@treturn table|nil array of `{ unit, count, cap, base_cap, replenish_chance, max_replenish, experience }`
(`cap` is the current cap after modifiers), or nil if the pool could not be read
@usage
for _, u in ipairs(region:GetMercenaryPool() or {}) do
    out(u.unit .. " " .. u.count .. "/" .. u.cap)
end
*/
static int Region_GetMercenaryPool(lua_State* L)  { return impl_get(L, false); }
static int Faction_GetMercenaryPool(lua_State* L) { return impl_get(L, true); }

/***
Adds units to the mercenary pool. If the unit is not in the pool yet it is added as a new
entry (persisted in saves); `options` then sets its cap and replenishment. Existing entries
are only raised up to their current cap; raise the cap first with `SetMercenaryUnitCap`.
@function AddMercenaryUnits
@tparam string unit_key `main_units` key
@tparam integer count units to add (>= 0)
@tparam[opt] table options for a new entry: `{ cap = integer (default count), replenish_chance = number (default 0), max_replenish = integer (default 0) }`
@treturn integer|nil units now available, or nil on failure
@usage
-- In a FactionTurnStart listener (runs on every machine in multiplayer):
region:AddMercenaryUnits("att_merc_ger_agathyrsi_warriors", 2, { cap = 4, replenish_chance = 0.25, max_replenish = 1 })
*/
static int Region_AddMercenaryUnits(lua_State* L)  { return impl_add(L, false); }
static int Faction_AddMercenaryUnits(lua_State* L) { return impl_add(L, true); }

/***
Removes units from the mercenary pool (the count never goes below 0). The entry stays in the
pool; set its cap to 0 to stop it replenishing.
@function RemoveMercenaryUnits
@tparam string unit_key `main_units` key
@tparam integer count units to remove (>= 0)
@treturn integer|nil units now available, or nil if the unit is not in the pool
@usage
faction:RemoveMercenaryUnits("att_merc_ger_agathyrsi_warriors", 1)
*/
static int Region_RemoveMercenaryUnits(lua_State* L)  { return impl_remove(L, false); }
static int Faction_RemoveMercenaryUnits(lua_State* L) { return impl_remove(L, true); }

/***
Sets a pool unit's base cap. The current cap keeps the existing modifier ratio until the
engine recomputes it at the next turn start (`base cap x cap modifier` effects); the
available count is clamped to it.
@function SetMercenaryUnitCap
@tparam string unit_key `main_units` key
@tparam integer cap new base cap (>= 0; 0 effectively removes the unit)
@treturn integer|nil the current cap after the change, or nil if the unit is not in the pool
@usage
region:SetMercenaryUnitCap("att_merc_ger_agathyrsi_warriors", 6)
*/
static int Region_SetMercenaryUnitCap(lua_State* L)  { return impl_set_cap(L, false); }
static int Faction_SetMercenaryUnitCap(lua_State* L) { return impl_set_cap(L, true); }

/***
Sets how a pool unit replenishes at turn start: with probability `chance` (times the pool's
replenishment modifier) it gains 1..`max_per_turn` units, up to its cap.
@function SetMercenaryUnitReplenishment
@tparam string unit_key `main_units` key
@tparam number chance chance per turn (0..1)
@tparam integer max_per_turn maximum units gained per turn (0 = never replenishes)
@treturn boolean true if changed, false if the unit is not in the pool
@usage
faction:SetMercenaryUnitReplenishment("att_merc_ger_agathyrsi_warriors", 0.5, 1)
*/
static int Region_SetMercenaryUnitReplenishment(lua_State* L)  { return impl_set_replenishment(L, false); }
static int Faction_SetMercenaryUnitReplenishment(lua_State* L) { return impl_set_replenishment(L, true); }

static void add_methods(lua_State* L, const char* metatable, const luaL_Reg* fns) {
    l_newmetatable(L, metatable);
    l_getfield(L, -1, "__index");
    if (l_type(L, -1) == LUA_TTABLE) {
        for (const luaL_Reg* f = fns; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] %s: mercenary pool methods added", metatable);
    } else {
        Log("[twdll] WARNING: %s __index not found - mercenary pool methods not added", metatable);
    }
    l_pop(L, 2);
}

static const luaL_Reg region_pool_methods[] = {
    {"GetMercenaryPool",              Region_GetMercenaryPool},
    {"AddMercenaryUnits",             Region_AddMercenaryUnits},
    {"RemoveMercenaryUnits",          Region_RemoveMercenaryUnits},
    {"SetMercenaryUnitCap",           Region_SetMercenaryUnitCap},
    {"SetMercenaryUnitReplenishment", Region_SetMercenaryUnitReplenishment},
    {nullptr, nullptr}
};

static const luaL_Reg faction_pool_methods[] = {
    {"GetMercenaryPool",              Faction_GetMercenaryPool},
    {"AddMercenaryUnits",             Faction_AddMercenaryUnits},
    {"RemoveMercenaryUnits",          Faction_RemoveMercenaryUnits},
    {"SetMercenaryUnitCap",           Faction_SetMercenaryUnitCap},
    {"SetMercenaryUnitReplenishment", Faction_SetMercenaryUnitReplenishment},
    {nullptr, nullptr}
};

void register_mercenary_pool_methods(lua_State* L) {
    add_methods(L, "REGION_SCRIPT_INTERFACE", region_pool_methods);
    add_methods(L, "FACTION_SCRIPT_INTERFACE", faction_pool_methods);
}
