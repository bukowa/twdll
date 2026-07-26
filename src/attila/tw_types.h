#pragma once
// tw_types.h — Engine object layouts for Total War: Attila (32-bit).
// Do NOT share with rome2/ — offsets differ between games.

#include <cstdint>
#include <cstddef>
#include <cstring>
#include "../common/lua_api.h"

namespace twdll {

struct TW_FamilyMember;

#pragma pack(push, 1)

struct TW_Faction {
    char pad_00[0x7DC];
    int  gold;              // 0x7DC
};

struct TW_Character {
    char             pad_00[0x14];
    int              movement_points;  // 0x14
    char             pad_18[0x1F0];
    TW_FamilyMember* family_member;    // 0x208
    char             pad_20C[0x34C];
    int              ambition;         // 0x558
    int              gravitas;         // 0x55C
};

struct TW_World {
    char pad_00[0x50];
    int  faction_count;    // 0x50
};

struct TW_MilitaryForce {
    char pad_00[0x45C];
    int  recruitment_queue_size;  // 0x45C
};

struct TW_Unit {
    char pad_00[0x44];
    int  current_number_of_men;  // 0x44
    int  max_number_of_men;      // 0x48
    char pad_4C[0x18];
    int  movement_points;        // 0x64
};

// EMPIREBATTLE::LAND_STATS — live battle statistics (embedded in UNIT via LAND_STATS_MANAGER)
// Offsets relative to start of LAND_STATS. Pure int32/float32 — identical in 32-bit and 64-bit.
struct TW_LandStats {
    char pad_00[0x38];
    int  charge_bonus;    // 0x38
    char pad_3C[0x8];
    int  morale;          // 0x44
    int  melee_attack;    // 0x48
    char pad_4C[0xC];
    int  melee_defence;   // 0x58
};

// EMPIREBATTLE::UNIT — selected fields only (32-bit Attila layout)
// m_stat_modifier_manager.m_stats is embedded (no pointer), verified via BATTLE_SCRIPT_UNIT vtable disasm
struct TW_BattleUnit {
    char         pad_00[0x1220];
    TW_LandStats stats;           // 0x1220  (m_stat_modifier_manager.m_stats)
    char         pad_127C[0x9A8]; // pad to m_num_men_initial (0x127C + 0x9A8 = 0x1C24)
    int          num_men_initial; // 0x1C24
};

// EMPIREBATTLE::BATTLE_SCRIPT_UNIT — Lua-facing wrapper
struct TW_BattleScriptUnit {
    void*         vtable;  // 0x0
    TW_BattleUnit* m_unit;  // 0x4
};

struct TW_CampaignUi {
};

#pragma pack(pop)

#define TW_ASSERT_OFFSET(S, F, O) \
    static_assert(offsetof(S, F) == O, #S " Attila: " #F " expected at " #O)

TW_ASSERT_OFFSET(TW_Faction,       gold,                    0x7DC);
TW_ASSERT_OFFSET(TW_Character,     movement_points,         0x14);
TW_ASSERT_OFFSET(TW_Character,     family_member,           0x208);
TW_ASSERT_OFFSET(TW_Character,     ambition,                0x558);
TW_ASSERT_OFFSET(TW_Character,     gravitas,                0x55C);
TW_ASSERT_OFFSET(TW_World,         faction_count,           0x50);
TW_ASSERT_OFFSET(TW_MilitaryForce, recruitment_queue_size,  0x45C);
TW_ASSERT_OFFSET(TW_Unit,          current_number_of_men,   0x44);
TW_ASSERT_OFFSET(TW_Unit,          max_number_of_men,       0x48);
TW_ASSERT_OFFSET(TW_Unit,          movement_points,         0x64);
TW_ASSERT_OFFSET(TW_LandStats,     charge_bonus,            0x38);
TW_ASSERT_OFFSET(TW_LandStats,     morale,                  0x44);
TW_ASSERT_OFFSET(TW_LandStats,     melee_attack,            0x48);
TW_ASSERT_OFFSET(TW_LandStats,     melee_defence,           0x58);
TW_ASSERT_OFFSET(TW_BattleUnit,    stats,                   0x1220);
TW_ASSERT_OFFSET(TW_BattleUnit,    num_men_initial,         0x1C24);
TW_ASSERT_OFFSET(TW_BattleScriptUnit, m_unit,              0x4);

// Per-type pointer offset inside the Lua userdata wrapper.
// Specialize via TW_PTR_OFFSET(T, offset) for each type.
// No default — missing specialization is a compile error.
template<typename T> struct TW_PtrOffset {
    static_assert(sizeof(T) == 0,
        "TW_PtrOffset not specialized for this type — add TW_PTR_OFFSET(T, offset) in tw_types.h");
};

#define TW_PTR_OFFSET(T, O) \
    template<> struct TW_PtrOffset<T> { static constexpr size_t value = O; }

TW_PTR_OFFSET(TW_Faction,       0x8);
TW_PTR_OFFSET(TW_Character,     0x8);
TW_PTR_OFFSET(TW_MilitaryForce, 0x8);
TW_PTR_OFFSET(TW_Unit,          0x8);
TW_PTR_OFFSET(TW_BattleUnit,    0x4);

template<typename T>
struct GameScriptInterface {
    char pad[TW_PtrOffset<T>::value];
    T*   m_wrapped_object;
};

static_assert(offsetof(GameScriptInterface<TW_Faction>,   m_wrapped_object) == 0x8);
static_assert(offsetof(GameScriptInterface<TW_Character>, m_wrapped_object) == 0x8);
static_assert(offsetof(GameScriptInterface<TW_BattleUnit>, m_wrapped_object) == 0x4);

template<typename T> T * tw_unwrap(lua_State* L, int slot) {
    void** ud = static_cast<void**>(l_touserdata(L, slot));
    if (!ud || !*ud) return nullptr;
    return *reinterpret_cast<T**>(static_cast<char*>(*ud) + TW_PtrOffset<T>::value);
}

template<typename T> void tw_push_wrapped(lua_State* L, T* raw_ptr) {
    auto* ud = static_cast<GameScriptInterface<T>*>(
        l_newuserdata(L, sizeof(GameScriptInterface<T>)));
    memset(ud, 0, sizeof(GameScriptInterface<T>));
    ud->m_wrapped_object = raw_ptr;
}

}