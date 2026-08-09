#pragma once
// tw_types.h — Engine object layouts for Total War: Attila (32-bit).
// Do NOT share with rome2/ — offsets differ between games.

#include <cstdint>
#include <cstddef>
#include <cstring>
#include "../common/lua_api.h"

namespace twdll {

struct TW_FamilyMember {
    char             pad_00[0x18];
    TW_FamilyMember* mother;          // 0x18
    TW_FamilyMember* father;          // 0x1C
};

#pragma pack(push, 1)

struct TW_Faction {
    char pad_00[0x7DC];
    int  treasury;                  // 0x7DC
    char pad_7E0[0xB0];
    void* m_home_region;            // 0x890
    void* m_original_home_region;   // 0x894
    void* m_home_theatre;           // 0x898
};

struct TW_Character {
    char             pad_00[0x14];
    int              action_points;    // 0x14
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
    int  num_men;                // 0x44
    int  max_num_men;            // 0x48
    char pad_4C[0x18];
    int  action_points;          // 0x64
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

struct TW_VectorNcc {
    void*  m_capacity_and_allocator;  // 0x0  (low bits: capacity, high bits: allocator ptr)
    int    m_size;                    // 0x4
    void** m_elements;                // 0x8
};

struct TW_ReinforcementsManager {
    void*         m_battle_env;                    // 0x0
    bool          m_debug_draw;                    // 0x4
    char          pad_05[0x3];
    void*         m_debug_draw_menu_item;          // 0x8
    TW_VectorNcc  m_spawn_zones;                   // 0xC
    TW_VectorNcc  m_reinforcements;                // 0x18
    bool          m_reinforcements_message_issued; // 0x24
    char          pad_25[0x3];
    int           m_max_num_units_per_army;        // 0x28
};

struct TW_Battle {
    void*                      m_battle_env;               // 0x0
    char                       pad_04[0x10];               // 0x4
    int                        m_battle_phase;             // 0x14
    char                       pad_18[0x2530];             // 0x18
    TW_ReinforcementsManager*  m_reinforcements_manager;   // 0x2548
};

struct TW_CampaignUi {
};

// EMPIRECAMPAIGN::CampaignSettlementCallback — selected fields (32-bit Attila layout).
// Gap analysis vs 64-bit DWARF (m_region@0x68, m_is_capital@0x70, m_max_slots@0x74,
// m_num_available_slots@0x78): the ComponentCallbacks+LISTENER base shrinks by
// 0x28 (10 pointers 8B→4B), so m_region@0x40, m_is_capital@0x44, m_max_slots@0x48,
// m_num_available_slots@0x4C. Verified via disasm of CampaignSettlementCallback__Initialize
// (0x113E1D50):
//   113e1d66  mov [esi+40h], edi    ; m_region = region arg
//   113e1d7b  mov [esi+4Ch], eax    ; m_num_available_slots = settlement->m_slots.m_size
//   113e2513  cmp [esi+48h], ebx    ; slot-render loop bound = m_max_slots
//   113e25cb  cmp ebx, [esi+4Ch]    ; avail-slot gate
// m_is_capital (0x44) distinguishes the main/selected settlement card (1) from the
// other province cards (0). Verified:
//   113c2210  mov byte ptr [ecx+44h], 0  ; Construct — minor defaults
//   113eab70  mov byte ptr [ecx+44h], 1  ; SetAsCapital — major
//   113e89b9  call SetAsCapital          ; BEFORE Initialize @113e89c8 (major card)
//   113e8a42..113e8a9a loop calls Initialize directly (minor cards keep 0)
struct TW_SettlementCallback {
    char  pad_00[0x40];
    void* m_region;                 // 0x40  (EMPIRE_UTILITY SAFE_PTR deref -> REGION)
    bool  m_is_capital;             // 0x44  (1 = main/selected settlement card)
    char  pad_45[0x3];
    int   m_max_slots;              // 0x48  (max slots to render; 4 normal, 6 capital)
    int   m_num_available_slots;    // 0x4C  (settlement->m_slots.m_size)
};

// EMPIRECAMPAIGN::PROVINCE_DEVELOPMENT — 3 fields, 12B in 32-bit.
// Gap analysis vs 64-bit DWARF (m_faction_province_manager@0x0 ptr,
// m_development_points@0x8, m_accumulated_growth@0xC): the back-ref pointer
// shrinks 8B->4B, so both card32 fields shift to 0x4 and 0x8. Verified via disasm:
//   10b6b71f  add [ebx+4], eax    ; m_development_points += eax
//   10b6c4b6  mov [esi+8], eax    ; m_accumulated_growth = eax
//   10b6c4e5  sub [esi+8], edi    ; m_accumulated_growth -= threshold
struct TW_PROVINCE_DEVELOPMENT {
    void*        m_faction_province_manager; // 0x0  (back-ref to FPM)
    unsigned int m_development_points;       // 0x4  (spent dev points)
    unsigned int m_accumulated_growth;       // 0x8  (surplus population accumulator)
};

// EMPIRECAMPAIGN::FACTION_PROVINCE_MANAGER — selected fields (32-bit Attila layout).
// Gap analysis vs 64-bit DWARF (m_province_development@0x340): FPM's leading
// pointer members shrink 8B->4B before m_province_development, so it lands at 0x23C.
// Verified via disasm of SCRIPTING_INTERFACE::add_development_points_to_region:
//   1073d4e0  lea ecx, [eax+23Ch]  ; ecx = &fpm->m_province_development
struct TW_FACTION_PROVINCE_MANAGER {
    char pad_00[0x23C];
    TW_PROVINCE_DEVELOPMENT m_province_development; // 0x23C
};

// EMPIRECAMPAIGN::REGION — selected fields (32-bit Attila layout).
// Gap analysis vs 64-bit DWARF (m_region_data@0x78): REGION's leading members
// shrink 8B->4B before m_region_data, so it lands at 0x50. Verified via disasm
// of REGION::theatre (sub_1094FCA0 @ 0x1094FCA0):
//   1094fca0  mov eax, [ecx+50h]   ; eax = this->m_region_data
//   1094fca3  mov eax, [eax+94h]   ; eax = m_region_data->m_theatre
struct TW_RegionData {
    char  pad_00[0x94];
    void* m_theatre;   // 0x94  (const CAMPAIGN_THEATRE*)
};

struct TW_Region {
    char           pad_00[0x50];
    TW_RegionData* m_region_data;   // 0x50
};

#pragma pack(pop)

#define TW_ASSERT_OFFSET(S, F, O) \
    static_assert(offsetof(S, F) == O, #S " Attila: " #F " expected at " #O)

TW_ASSERT_OFFSET(TW_Faction,       treasury,                0x7DC);
TW_ASSERT_OFFSET(TW_Faction,       m_home_region,            0x890);
TW_ASSERT_OFFSET(TW_Faction,       m_original_home_region,   0x894);
TW_ASSERT_OFFSET(TW_Faction,       m_home_theatre,           0x898);
TW_ASSERT_OFFSET(TW_Character,     action_points,           0x14);
TW_ASSERT_OFFSET(TW_Character,     family_member,           0x208);
TW_ASSERT_OFFSET(TW_Character,     ambition,                0x558);
TW_ASSERT_OFFSET(TW_Character,     gravitas,                0x55C);
TW_ASSERT_OFFSET(TW_World,         faction_count,           0x50);
TW_ASSERT_OFFSET(TW_MilitaryForce, recruitment_queue_size,  0x45C);
TW_ASSERT_OFFSET(TW_Unit,          num_men,                 0x44);
TW_ASSERT_OFFSET(TW_Unit,          max_num_men,             0x48);
TW_ASSERT_OFFSET(TW_Unit,          action_points,           0x64);
TW_ASSERT_OFFSET(TW_LandStats,     charge_bonus,            0x38);
TW_ASSERT_OFFSET(TW_LandStats,     morale,                  0x44);
TW_ASSERT_OFFSET(TW_LandStats,     melee_attack,            0x48);
TW_ASSERT_OFFSET(TW_LandStats,     melee_defence,           0x58);
TW_ASSERT_OFFSET(TW_BattleUnit,    stats,                   0x1220);
TW_ASSERT_OFFSET(TW_BattleUnit,    num_men_initial,         0x1C24);
TW_ASSERT_OFFSET(TW_BattleScriptUnit, m_unit,              0x4);
TW_ASSERT_OFFSET(TW_FamilyMember,    mother,                  0x18);
TW_ASSERT_OFFSET(TW_FamilyMember,    father,                  0x1C);
TW_ASSERT_OFFSET(TW_VectorNcc,             m_capacity_and_allocator, 0x0);
TW_ASSERT_OFFSET(TW_VectorNcc,             m_size,                   0x4);
TW_ASSERT_OFFSET(TW_VectorNcc,             m_elements,               0x8);
TW_ASSERT_OFFSET(TW_ReinforcementsManager, m_battle_env,             0x0);
TW_ASSERT_OFFSET(TW_ReinforcementsManager, m_debug_draw,             0x4);
TW_ASSERT_OFFSET(TW_ReinforcementsManager, m_debug_draw_menu_item,   0x8);
TW_ASSERT_OFFSET(TW_ReinforcementsManager, m_spawn_zones,            0xC);
TW_ASSERT_OFFSET(TW_ReinforcementsManager, m_reinforcements,         0x18);
TW_ASSERT_OFFSET(TW_ReinforcementsManager, m_reinforcements_message_issued, 0x24);
TW_ASSERT_OFFSET(TW_ReinforcementsManager, m_max_num_units_per_army, 0x28);
TW_ASSERT_OFFSET(TW_Battle,            m_battle_env,            0x0);
TW_ASSERT_OFFSET(TW_Battle,            m_battle_phase,          0x14);
TW_ASSERT_OFFSET(TW_Battle,            m_reinforcements_manager, 0x2548);
TW_ASSERT_OFFSET(TW_SettlementCallback, m_region,               0x40);
TW_ASSERT_OFFSET(TW_SettlementCallback, m_is_capital,           0x44);
TW_ASSERT_OFFSET(TW_SettlementCallback, m_max_slots,            0x48);
TW_ASSERT_OFFSET(TW_SettlementCallback, m_num_available_slots,  0x4C);
TW_ASSERT_OFFSET(TW_PROVINCE_DEVELOPMENT,     m_development_points,  0x4);
TW_ASSERT_OFFSET(TW_PROVINCE_DEVELOPMENT,     m_accumulated_growth,  0x8);
TW_ASSERT_OFFSET(TW_FACTION_PROVINCE_MANAGER, m_province_development, 0x23C);
TW_ASSERT_OFFSET(TW_Region,        m_region_data,            0x50);
TW_ASSERT_OFFSET(TW_RegionData,    m_theatre,                0x94);

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
TW_PTR_OFFSET(TW_FamilyMember,  0x8);
TW_PTR_OFFSET(TW_MilitaryForce, 0x8);
TW_PTR_OFFSET(TW_Unit,          0x8);
TW_PTR_OFFSET(TW_BattleUnit,    0x4);
TW_PTR_OFFSET(TW_Region,        0x8);

template<typename T> T * tw_unwrap(lua_State* L, int slot) {
    void** ud = static_cast<void**>(l_touserdata(L, slot));
    if (!ud || !*ud) return nullptr;
    return *reinterpret_cast<T**>(static_cast<char*>(*ud) + TW_PtrOffset<T>::value);
}

template<typename T> void tw_push_wrapped(lua_State* L, T* raw_ptr) {
    if (!raw_ptr) {
        l_pushnil(L);
        return;
    }
    constexpr size_t off = TW_PtrOffset<T>::value;
    void** ud = static_cast<void**>(l_newuserdata(L, off + sizeof(void*)));
    memset(ud, 0, off + sizeof(void*));
    *ud = ud;
    *reinterpret_cast<T**>(reinterpret_cast<char*>(ud) + off) = raw_ptr;
}

}