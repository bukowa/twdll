#pragma once
// tw_types.h — Engine object layouts for Total War: Attila (32-bit).
// Do NOT share with rome2/ — offsets differ between games.

#include <cstdint>
#include <cstddef>
#include <cstring>
#include "game_api.h"
#include "../common/lua_api.h"

namespace twdll {

struct TW_FamilyMember {
    char             pad_00[0x18];
    TW_FamilyMember* mother;          // 0x18
    TW_FamilyMember* father;          // 0x1C
};

#pragma pack(push, 1)

// EMPIREUTILITY::POLITICAL_PARTY_RECORD — 32-bit layout, derived via gap
// analysis from the 64-bit DWARF (112B) where pointers shrink 8B->4B.
// The static DB row behind each campaign party. m_key is a CA::String whose
// 32-bit layout {m_len@0, m_pad@4, m_data@8} is verified in faction.cpp.
// Only fields needed so far are mapped; m_initial_power is used to seed a
// party's senators in the CAMPAIGN_POLITICAL_PARTY ctor (sub_10BF2640):
//   10bf269d  movd xmm0, dword ptr [eax+44h]   ; record->m_initial_power
struct TW_CAString {
    uint32_t    m_len;    // 0x0
    uint32_t    m_pad;    // 0x4
    const char* m_data;   // 0x8
};

struct TW_PoliticalPartyRecord {
    TW_CAString m_key;          // 0x0
    char        pad_0C[0x38];
    float       m_initial_power;   // 0x44
};

// EMPIRECAMPAIGN::CAMPAIGN_POLITICAL_PARTY — one runtime party inside a
// faction's politics map. 32-bit layout derived via gap analysis from 64-bit
// DWARF (64B): m_politics 8->4, m_party_record 8->4, m_senators_string
// (CA::UniString) 16->12, so m_senators 0x20->0x14 and m_power 0x24->0x18.
// Verified via disasm of the party ctor sub_10BF2640:
//   10bf264a  mov [esi], eax            ; m_politics
//   10bf2653  mov [esi+4], eax          ; m_party_record
//   10bf2656  call sub_100DB6C0         ; m_senators_string ctor @ +0x8
//   10bf26e1  mov [esi+14h], eax        ; m_senators = (int)(initial_power * senator_total * mult)
struct TW_CampaignPoliticalParty {
    void* m_politics;        // 0x0  (CAMPAIGN_POLITICS*)
    void* m_party_record;    // 0x4  (const POLITICAL_PARTY_RECORD*)
    char  pad_08[0xC];       // 0x8  m_senators_string (CA::UniString)
    int   m_senators;        // 0x14 (CA::card32)
    float m_power;           // 0x18 (CA::float32)
    char  pad_1C[0x10];      // 0x1C m_active_effects, 0x20 m_possible_effects (unused so far)
};

// CA_STD::UNORDERED_MAP_NCC bucket walk layout — the m_political_parties map
// (24B) lives at politics+0x28 (verified below). Member order follows CA_STD
// (same as TW_VectorNcc: capacity first, then size, then elements). Each
// bucket is 12B:
//   bucket+0  m_head_and_allocator.m_member  (head NODE* or &bucket+4 if empty)
//   bucket+4  m_fake_node                    (terminator sentinel)
// Each NODE is 12B header + the value:
//   node+0    m_previous
//   node+4    m_next
//   node+8    m_data.first    (const POLITICAL_PARTY_RECORD*)
//   node+0xC  m_data.second   (CAMPAIGN_POLITICAL_PARTY)
// Verified via disasm of politics ctor insert loop (sub_10BF27C0):
//   10bf2832  add edi, 28h            ; edi = &this->m_political_parties (map base)
//   10bf2903  mov esi, [edi+8]        ; esi = map->m_size (bucket count)
//   10bf2911  mov eax, [edi+0Ch]      ; eax = map->m_elements
//   10bf2921  mov esi, [ecx+eax]      ; esi = buckets[idx*12].m_head
//   10bf2936  mov esi, [esi+4]        ; node = node->m_next
//   10bf29e2  mov [edi+8], ecx        ; node->m_data.first = record
//   10bf29e5  lea ecx, [edi+0Ch]      ; &node->m_data.second (party)
// And rehash sub_10C6A9D0: [ebp+8] count, [ebp+0Ch] elements, [ebp+10h] count
// of entries, [ebp+14h] max_load_factor; bucket stride: add edi, 0Ch @ 10c6ab3d
struct TW_PoliticalPartiesMap {
    char   pad_00[0x4];     // m_hash_function (1B) + m_equality_comparison (1B) + pad
    void*  m_capacity;      // 0x4  bucket vector capacity
    int    m_size;          // 0x8  bucket count
    void** m_elements;      // 0xC  buckets array (each element 12B)
    int    m_count;         // 0x10 number of entries (map size)
    float  m_max_load_factor; // 0x14

    template <typename Fn>
    void for_each(Fn&& fn) const {
        for (int i = 0; i < m_size; ++i) {
            char* bucket = reinterpret_cast<char*>(m_elements) + i * 12;
            char* fake   = bucket + 4;
            char* node   = *reinterpret_cast<char**>(bucket);
            while (node && node != fake) {
                auto* party = reinterpret_cast<TW_CampaignPoliticalParty*>(node + 0xC);
                fn(party);
                node = *reinterpret_cast<char**>(node + 4);
            }
        }
    }

    TW_CampaignPoliticalParty* find_by_key(const char* key) const {
        TW_CampaignPoliticalParty* found = nullptr;
        for_each([&](TW_CampaignPoliticalParty* p) {
            if (found) return;
            auto* rec = static_cast<TW_PoliticalPartyRecord*>(p->m_party_record);
            if (rec && rec->m_key.m_data && std::strcmp(rec->m_key.m_data, key) == 0)
                found = p;
        });
        return found;
    }

    TW_CampaignPoliticalParty* find_by_record(const void* record) const {
        TW_CampaignPoliticalParty* found = nullptr;
        for_each([&](TW_CampaignPoliticalParty* p) {
            if (found) return;
            if (p->m_party_record == record)
                found = p;
        });
        return found;
    }
};

// EMPIRECAMPAIGN::CAMPAIGN_POLITICS — per-faction politics manager, embedded
// in FACTION at 0x112C. 32-bit layout derived via gap analysis from 64-bit
// DWARF (184B @ 0x1518): two embedded SAFER_REPORTERs shrink 32->16B each,
// CAMPAIGN_ENV_MODEL_ACCESS 8->4B, so the map lands at +0x28 and m_active at
// +0x48 (64-bit: map @ +0x50, m_active @ +0x7C). Verified via disasm of the
// politics ctor sub_10BF27C0:
//   10bf27d5  lea eax, [esi+158h]     ; faction env access (0x158)
//   10bf27dc  lea ecx, [edi+20h]      ; CAMPAIGN_ENV_MODEL_ACCESS @ +0x20
//   10bf282f  mov [edi+24h], esi      ; m_faction @ +0x24
//   10bf2832  add edi, 28h            ; m_political_parties @ +0x28
//   10bf283e  call sub_10BE4D20(4,..) ; map ctor (4 buckets)
//   10bf285e  mov byte ptr [eax+48h], 1  ; m_active @ +0x48
//   10bf2868  mov dword ptr [eax+40h], 0 ; m_primary_party @ +0x40
//   10bf2865  lea ecx, [eax+5Ch]      ; m_political_event_data @ +0x5C
struct TW_CampaignPolitics {
    char                 pad_00[0x24];
    void*                m_faction;           // 0x24 (FACTION*)
    TW_PoliticalPartiesMap m_political_parties; // 0x28
    void*                m_primary_party;     // 0x40 (const POLITICAL_PARTY_RECORD*)
    char                 pad_44[0x4];
    bool                 m_active;            // 0x48
    char                 pad_49[0x13];
    char                 m_political_event_data[0x18]; // 0x5C
};

struct TW_Faction {
    char pad_00[0x7DC];
    int  treasury;                  // 0x7DC
    char pad_7E0[0xB0];
    void* m_home_region;            // 0x890
    void* m_original_home_region;   // 0x894
    void* m_home_theatre;           // 0x898
    char  pad_89C[0x98];
    void* m_faction_technology_manager;  // 0x934  verified via disasm: mov eax,[ecx+934h] @ sub_10705560
    char  pad_938[0x7F4];
    TW_CampaignPolitics m_politics; // 0x112C verified via disasm: lea ecx,[ebx+112Ch]; push ebx; call sub_10BF27C0 @ 0x106c2140
};

// EMPIRECAMPAIGN::GENERAL_BODYGUARD_DETAILS (32-bit layout, size 0x24)
// Verified via CHARACTER_DETAILS::update_initial_general_bodyguard (sub_107F8A50):
//   0x107f8a5c  mov eax, [edx]       ; m_unit (MAIN_UNIT_RECORD*)
//   0x107f8a61  mov [ecx+31Ch], eax  ; stored in CHARACTER_DETAILS+0x31C (= CHARACTER+0x520)
//   0x107f8a67  movzx eax, word ptr [edx+4] ; m_men -> [ecx+320h]
//   0x107f8a72  movzx eax, word ptr [edx+6] ; m_men_in_fully_replenished -> [ecx+322h]
//   0x107f8a7d  mov eax, [edx+8]     ; experience score -> [ecx+324h]
//   0x107f8a86  mov al, [edx+0Ch]    ; experience level -> [ecx+328h]
//   0x107f8a8f  mov eax, [edx+10h]   ; experience progress -> [ecx+32Ch]

// EMPIREUTILITY::CAMPAIGN_CHARACTER_ART_SETS_CAMPAIGN_GROUP_RECORD
struct TW_CampaignCharacterArtSetsCampaignGroupRecord {
    TW_CAString m_key;   // 0x00 (e.g. "main")
};

// EMPIREUTILITY::CAMPAIGN_CHARACTER_ART_SET_RECORD (size 0x44)
// Verified via 64-bit DWARF & 32-bit DB loader (sub_10E1AE60 / sub_10EFC250)
struct TW_CampaignCharacterArtSetRecord {
    TW_CAString m_agent_type;   // 0x00
    TW_CAString m_art_set_id;   // 0x0C (e.g. "att_cult_barbarian")
    TW_CAString m_culture;      // 0x18
    TW_CAString m_subculture;   // 0x24
    TW_CAString m_faction;      // 0x30
    bool        m_is_custom;    // 0x3C
    bool        m_is_male;      // 0x3D
    char        pad_3E[2];
    TW_CampaignCharacterArtSetsCampaignGroupRecord* m_group; // 0x40
};

// EMPIRECAMPAIGN::CHARACTER_ART_SET (size 0x20)
// Verified via 64-bit DWARF (0x1587a00) and gap analysis
struct TW_CharacterArtSet {
    bool          m_aging_set;          // 0x00
    bool          m_seasonal_set;       // 0x01
    bool          m_levelling_set;      // 0x02
    bool          m_health_set;         // 0x03
    bool          m_religion_set;       // 0x04
    bool          m_faction_leader_set; // 0x05
    char          pad_06[2];
    char          m_art_set_map[0x18];  // 0x08 (CA_STD::UNORDERED_MAP<String, CHARACTER_ART*>)
    TW_CampaignCharacterArtSetRecord* m_art_set_record; // 0x20
};

// EMPIRECAMPAIGN::PORTRAIT_CAMERA_SETTINGS (size 0x2C)
// Verified via CHARACTER_DETAILS + 0x354 (sub_107DC9E0 @ cmp [ebp+354h], 0)
struct TW_PortraitCameraSettings {
    float       m_camera_distance;       // 0x00
    float       m_theta;                 // 0x04
    float       m_phi;                   // 0x08
    float       m_fov;                   // 0x0C
    void*       m_portrait_infos_start;  // 0x10
    int         m_portrait_infos_size;   // 0x14
    void*       m_portrait_infos_end;    // 0x18
    uint32_t    m_unique_id;             // 0x1C
    TW_CAString m_unique_id_string;      // 0x20 (e.g. "att_general_barbarian_01")
};

// EMPIRECAMPAIGN::CHARACTER_DETAILS_ART_SET_INFO (size 0x40)
// Embedded in CHARACTER_DETAILS at offset 0xAC (= CHARACTER + 0x2B0)
struct TW_CharacterDetailsArtSetInfo {
    TW_CAString         m_faction_key;          // 0x00
    TW_CAString         m_culture_key;          // 0x0C
    TW_CAString         m_subculture_key;       // 0x18
    TW_CAString         m_agent_key;            // 0x24
    TW_CAString         m_art_set_to_allocate;  // 0x30
    TW_CharacterArtSet* m_art_set;              // 0x3C
};

struct TW_GeneralBodyguardDetails {
    void*    m_unit;                      // 0x00 (MAIN_UNIT_RECORD*)
    uint16_t m_men;                       // 0x04
    uint16_t m_men_in_fully_replenished;  // 0x06
    uint32_t m_experience_score;          // 0x08
    uint8_t  m_experience_level;          // 0x0C
    uint8_t  pad_0D[3];
    float    m_experience_progress;       // 0x10
};

struct TW_TraitEntry {
    void*    m_record;       // 0x00 (const TRAIT_INFO_RECORD*)
    void*    m_level_record; // 0x04 (const CHARACTER_TRAIT_LEVEL_RECORD*)
    int32_t  m_points;       // 0x08
};

struct TW_Traits {
    void*           _vptr;      // 0x00
    void*           m_capacity; // 0x04
    uint32_t        m_size;     // 0x08
    TW_TraitEntry*  m_elements; // 0x0C
    char            pad_10[0x18]; // 0x10 m_effects (CAMPAIGN_EFFECT_LIST)
};
static_assert(sizeof(TW_TraitEntry) == 12, "TW_TraitEntry size must be 12");
static_assert(sizeof(TW_Traits) == 0x28, "TW_Traits size must be 0x28");

struct TW_CharacterDetails {
    char                          pad_00[0x4];
    TW_FamilyMember*              family_member;       // 0x04  (= CHARACTER+0x208)
    char                          pad_08[0xA4];
    TW_CharacterDetailsArtSetInfo m_art_set_info;      // 0xAC  (= CHARACTER+0x2B0)
    char                          pad_EC[0x18];
    TW_Traits                     traits;              // 0x104 (= CHARACTER+0x308)
    char                          pad_12C[0x198];
    int8_t                        m_loyalty_modifier;  // 0x2C4 (= CHARACTER+0x4C8, direct loyalty modifier)
    char                          pad_2C5[0x27];
    void*                         m_political_party;   // 0x2EC (= CHARACTER+0x4F0, const POLITICAL_PARTY_RECORD*)
    char                          pad_2F0[0x4];
    int                           political_gravitas;  // 0x2F4 (= CHARACTER+0x4F8, CA::card32)
                                                       //         verified: sub_107DC770 @ mov eax,[ecx+2F4h]
    char                          pad_2F8[0x24];
    TW_GeneralBodyguardDetails    m_initial_general_bodyguard_details; // 0x31C (= CHARACTER+0x520)
    char                          pad_330[0x24];
    TW_PortraitCameraSettings*    m_portrait_camera_settings;          // 0x354 (= CHARACTER+0x558)
};

struct TW_Character {
    char                pad_00[0x14];
    int                 action_points;       // 0x14  verified: LOCOMOTABLE::m_action_points, gap analysis
    char                pad_18[0x1C4];
    void*               commanded_unit_link; // 0x1DC  m_commanded_unit.m_link.m_object
    char                pad_1E0[0x24];
    TW_CharacterDetails details;             // 0x204  CHARACTER_DETAILS embedded sub-struct
                                             //         verified: sub_107F8A00 @ lea ecx,[esi+204h]
};

struct TW_World {
    char pad_00[0x50];
    int  faction_count;    // 0x50
};

struct TW_MilitaryForceMorale {
    float m_morale;   // 0x00 (0.0 to 100.0)
};

struct TW_MilitaryForce {
    char                    pad_00[0x3E0];
    void*                   m_morale_vtable;   // 0x3E0 (SAFE_PTR vtable)
    TW_MilitaryForceMorale* m_morale;          // 0x3E4 (SAFE_PTR object)
    char                    pad_3E8[0x74];
    int                     recruitment_queue_size;  // 0x45C
};

struct TW_Unit {
    char  pad_00[0x38];
    void* m_force_link;          // 0x38  m_force_link.m_link.m_object -> ONE_TO_MANY_LINK<UNIT_CONTAINER,UNIT>*
    char  pad_3C[0x8];           // 0x3C  m_faction, 0x40 m_unit_record
    int   num_men;               // 0x44
    int   max_num_men;           // 0x48
    char  pad_4C[0x18];
    int   action_points;         // 0x64
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

struct TW_SettlementExpansionSlot {
    void*        m_data;      // 0x00 (const SLOT_MAP*)
    void*        m_slot;      // 0x04 (REGION_SLOT*)
    uint32_t     m_rotation;  // 0x08 (0..5, rotation * 60 deg)
};

struct TW_SettlementExpansionManager {
    char         pad_00[0x14];
    TW_VectorNcc m_slots;     // 0x14 (vector<SETTLEMENT_EXPANSION_SLOT*>, m_size@0x18, m_elements@0x1C)
};

struct TW_Settlement {
    char pad_00[0x1A8];
    TW_SettlementExpansionManager* m_settlement_expansion_manager; // 0x1A8
};

struct TW_RegionSlot {
    char  pad_00[0x1C8];
    void* m_slot_manager;     // 0x1C8
};

// EMPIREUTILITY::RELIGION_RECORD (32-bit layout, size 0x28)
// Verified via disasm of sub_10848EE0 / sub_100DBCB0
struct TW_ReligionRecord {
    TW_CAString m_key;            // 0x00 (m_len @0, m_pad @4, m_data @8)
    void*       m_onscreen;       // 0x0C (const CA::UniString*)
    int32_t     m_convertibility; // 0x10
    TW_CAString m_icon_path;      // 0x14
    uint32_t    m_colour;         // 0x20
    int32_t     m_sort_order;     // 0x24
};

// std::pair<const RELIGION_RECORD*, float> (32-bit layout, size 0x8)
// Verified via disasm of PROVINCE::majority_religion_proportion (sub_10BA28D0)
struct TW_ReligionProportion {
    const TW_ReligionRecord* m_religion;   // 0x00
    float                    m_proportion; // 0x04 (raw float 0.0 .. 1.0)
};

struct TW_Region {
    char           pad_00[0x50];
    TW_RegionData* m_region_data;        // 0x50
    char           pad_54[0x88];
    TW_VectorNcc   m_religion_breakdown; // 0xDC (m_size @0xE0, m_elements @0xE4)
};

// EMPIRECAMPAIGN::CAMPAIGN_MODEL — selected fields (32-bit Attila layout).
// CAMPAIGN_ENV_MODEL_ACCESS (single pointer m_campaign_env) is embedded here;
// in 64-bit it sits at 0x21E0 behind the REPORTING_NEXUS base, which shrinks
// to 0x10F0 in 32-bit (all pointers 8B->4B). Verified via disasm of the 32-bit
// unlock wrapper sub_1073C7F0: `lea ecx,[ecx+10F0h]; call sub_109C8F70`.
struct TW_CampaignModel {
    char  pad_00[0x10F0];
    void* m_campaign_env;   // 0x10F0  CAMPAIGN_ENV_MODEL_ACCESS::m_campaign_env (CAMPAIGN_ENV*)
};

// EMPIRECAMPAIGN::CAMPAIGN_ENV — selected fields (32-bit Attila layout).
// 64-bit m_game_core@0x58 -> 32-bit 0x30 (leading members shrink 8B->4B).
// Verified via disasm of sub_109C8F70: `mov eax,[eax+30h]`.
struct TW_CampaignEnv {
    char  pad_00[0x30];
    void* m_game_core;      // 0x30  EMPIRECOMMON::GAME_CORE*
};

// EMPIRECOMMON::GAME_CORE — selected fields (32-bit Attila layout).
// 64-bit m_databases@0x20 -> 32-bit 0x10 (leading pointers shrink 8B->4B).
// Verified via disasm of sub_109C8F70: `mov eax,[eax+10h]`.
struct TW_GameCore {
    char  pad_00[0x10];
    void* m_databases;      // 0x10  EMPIREUTILITY::EMPIRE_DATABASES*
};

// EMPIREUTILITY::DATABASE_TABLE — single DB table lookup interface.
// Verified via disasm of sub_10192660 (DATABASE_TABLE::record_index).
struct TW_DatabaseTable {
    void* find_record(const char* key, size_t key_len = 0) const {
        if (!this || !g_record_index || !key) return nullptr;
        if (key_len == 0) key_len = std::strlen(key);
        struct RecordKey { uint32_t len; uint32_t pad; const char* data; }
            k = { static_cast<uint32_t>(key_len), 0, key };
        return g_record_index(const_cast<TW_DatabaseTable*>(this), &k);
    }
};

// EMPIREUTILITY::EMPIRE_DATABASES — container for all game database tables (32-bit Attila layout).
// 64-bit m_technologies_table@0x20E0 -> 32-bit 0x1604, m_main_units_table@0x1000.
// Lazy-loader cache fields: populated on the first tick of any running campaign.
struct TW_Databases {
    char              pad_00[0xF18];
    TW_DatabaseTable* political_parties; // 0xF18  (POLITICAL_PARTIES_TABLE)
    char              pad_F1C[0xE4];
    TW_DatabaseTable* main_units;        // 0x1000 (MAIN_UNITS_TABLE)
    char              pad_1004[0x164];
    TW_DatabaseTable* loyalty_factors;   // 0x1168 (LOYALTY_FACTORS_TABLE)
    char              pad_116C[0x498];
    TW_DatabaseTable* technologies;      // 0x1604 (TECHNOLOGIES_TABLE)

    static TW_Databases* get() {
        if (!g_campaign_model) return nullptr;
        auto* cm = static_cast<TW_CampaignModel*>(g_campaign_model);
        if (!cm->m_campaign_env) return nullptr;
        auto* env = static_cast<TW_CampaignEnv*>(cm->m_campaign_env);
        if (!env->m_game_core) return nullptr;
        return static_cast<TW_Databases*>(static_cast<TW_GameCore*>(env->m_game_core)->m_databases);
    }
};

#pragma pack(pop)

#define TW_ASSERT_OFFSET(S, F, O) \
    static_assert(offsetof(S, F) == O, #S " Attila: " #F " expected at " #O)

TW_ASSERT_OFFSET(TW_Faction,       treasury,                0x7DC);
TW_ASSERT_OFFSET(TW_Faction,       m_home_region,            0x890);
TW_ASSERT_OFFSET(TW_Faction,       m_original_home_region,   0x894);
TW_ASSERT_OFFSET(TW_Faction,       m_home_theatre,           0x898);
TW_ASSERT_OFFSET(TW_Faction,       m_faction_technology_manager, 0x934);
TW_ASSERT_OFFSET(TW_Faction,       m_politics,               0x112C);
TW_ASSERT_OFFSET(TW_Character,        action_points,                        0x14);
TW_ASSERT_OFFSET(TW_Character,        commanded_unit_link,                  0x1DC);
TW_ASSERT_OFFSET(TW_Character,        details,                              0x204);
TW_ASSERT_OFFSET(TW_CharacterDetails,  family_member,                        0x4);
TW_ASSERT_OFFSET(TW_CharacterDetails,  m_art_set_info,                       0xAC);
TW_ASSERT_OFFSET(TW_CharacterDetails,  m_loyalty_modifier,                   0x2C4);
TW_ASSERT_OFFSET(TW_CharacterDetails,  m_political_party,                    0x2EC);
TW_ASSERT_OFFSET(TW_CharacterDetails,  political_gravitas,                   0x2F4);
TW_ASSERT_OFFSET(TW_CharacterDetails,  m_initial_general_bodyguard_details,  0x31C);
TW_ASSERT_OFFSET(TW_CharacterDetails,  m_portrait_camera_settings,          0x354);

TW_ASSERT_OFFSET(TW_CharacterDetailsArtSetInfo, m_faction_key,          0x00);
TW_ASSERT_OFFSET(TW_CharacterDetailsArtSetInfo, m_culture_key,          0x0C);
TW_ASSERT_OFFSET(TW_CharacterDetailsArtSetInfo, m_subculture_key,       0x18);
TW_ASSERT_OFFSET(TW_CharacterDetailsArtSetInfo, m_agent_key,            0x24);
TW_ASSERT_OFFSET(TW_CharacterDetailsArtSetInfo, m_art_set_to_allocate,  0x30);
TW_ASSERT_OFFSET(TW_CharacterDetailsArtSetInfo, m_art_set,              0x3C);

TW_ASSERT_OFFSET(TW_CampaignCharacterArtSetRecord, m_agent_type,   0x00);
TW_ASSERT_OFFSET(TW_CampaignCharacterArtSetRecord, m_art_set_id,    0x0C);
TW_ASSERT_OFFSET(TW_CampaignCharacterArtSetRecord, m_culture,       0x18);
TW_ASSERT_OFFSET(TW_CampaignCharacterArtSetRecord, m_subculture,    0x24);
TW_ASSERT_OFFSET(TW_CampaignCharacterArtSetRecord, m_faction,       0x30);
TW_ASSERT_OFFSET(TW_CampaignCharacterArtSetRecord, m_is_custom,     0x3C);
TW_ASSERT_OFFSET(TW_CampaignCharacterArtSetRecord, m_is_male,       0x3D);
TW_ASSERT_OFFSET(TW_CampaignCharacterArtSetRecord, m_group,         0x40);

TW_ASSERT_OFFSET(TW_CharacterArtSet, m_aging_set,          0x00);
TW_ASSERT_OFFSET(TW_CharacterArtSet, m_seasonal_set,       0x01);
TW_ASSERT_OFFSET(TW_CharacterArtSet, m_levelling_set,      0x02);
TW_ASSERT_OFFSET(TW_CharacterArtSet, m_health_set,         0x03);
TW_ASSERT_OFFSET(TW_CharacterArtSet, m_religion_set,       0x04);
TW_ASSERT_OFFSET(TW_CharacterArtSet, m_faction_leader_set, 0x05);
TW_ASSERT_OFFSET(TW_CharacterArtSet, m_art_set_record,     0x20);

TW_ASSERT_OFFSET(TW_PortraitCameraSettings, m_camera_distance,  0x00);
TW_ASSERT_OFFSET(TW_PortraitCameraSettings, m_theta,            0x04);
TW_ASSERT_OFFSET(TW_PortraitCameraSettings, m_phi,              0x08);
TW_ASSERT_OFFSET(TW_PortraitCameraSettings, m_fov,              0x0C);
TW_ASSERT_OFFSET(TW_PortraitCameraSettings, m_unique_id,        0x1C);
TW_ASSERT_OFFSET(TW_PortraitCameraSettings, m_unique_id_string, 0x20);
TW_ASSERT_OFFSET(TW_World,         faction_count,           0x50);
TW_ASSERT_OFFSET(TW_MilitaryForceMorale, m_morale,          0x00);
TW_ASSERT_OFFSET(TW_MilitaryForce, m_morale,                0x3E4);
TW_ASSERT_OFFSET(TW_MilitaryForce, recruitment_queue_size,  0x45C);
TW_ASSERT_OFFSET(TW_Unit,          m_force_link,            0x38);
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
TW_ASSERT_OFFSET(TW_Region,        m_religion_breakdown,     0xDC);
TW_ASSERT_OFFSET(TW_ReligionRecord, m_key,                    0x0);
TW_ASSERT_OFFSET(TW_ReligionRecord, m_icon_path,              0x14);
TW_ASSERT_OFFSET(TW_ReligionProportion, m_religion,           0x0);
TW_ASSERT_OFFSET(TW_ReligionProportion, m_proportion,         0x4);
TW_ASSERT_OFFSET(TW_RegionData,    m_theatre,                0x94);
TW_ASSERT_OFFSET(TW_CampaignModel, m_campaign_env,           0x10F0);
TW_ASSERT_OFFSET(TW_CampaignEnv,   m_game_core,              0x30);
TW_ASSERT_OFFSET(TW_GameCore,      m_databases,              0x10);
TW_ASSERT_OFFSET(TW_Databases,     political_parties,        0xF18);
TW_ASSERT_OFFSET(TW_Databases,     main_units,               0x1000);
TW_ASSERT_OFFSET(TW_Databases,     loyalty_factors,          0x1168);
TW_ASSERT_OFFSET(TW_Databases,     technologies,             0x1604);
TW_ASSERT_OFFSET(TW_PoliticalPartyRecord, m_key,            0x0);
TW_ASSERT_OFFSET(TW_PoliticalPartyRecord, m_initial_power,   0x44);
TW_ASSERT_OFFSET(TW_CampaignPoliticalParty, m_politics,      0x0);
TW_ASSERT_OFFSET(TW_CampaignPoliticalParty, m_party_record,  0x4);
TW_ASSERT_OFFSET(TW_CampaignPoliticalParty, m_senators,      0x14);
TW_ASSERT_OFFSET(TW_CampaignPoliticalParty, m_power,         0x18);
TW_ASSERT_OFFSET(TW_PoliticalPartiesMap, m_size,             0x8);
TW_ASSERT_OFFSET(TW_PoliticalPartiesMap, m_elements,         0xC);
TW_ASSERT_OFFSET(TW_CampaignPolitics, m_faction,             0x24);
TW_ASSERT_OFFSET(TW_CampaignPolitics, m_political_parties,   0x28);
TW_ASSERT_OFFSET(TW_CampaignPolitics, m_primary_party,       0x40);
TW_ASSERT_OFFSET(TW_CampaignPolitics, m_active,              0x48);

TW_ASSERT_OFFSET(TW_SettlementExpansionSlot, m_data,                  0x00);
TW_ASSERT_OFFSET(TW_SettlementExpansionSlot, m_slot,                  0x04);
TW_ASSERT_OFFSET(TW_SettlementExpansionSlot, m_rotation,              0x08);
TW_ASSERT_OFFSET(TW_SettlementExpansionManager, m_slots,              0x14);
TW_ASSERT_OFFSET(TW_Settlement, m_settlement_expansion_manager,       0x1A8);
TW_ASSERT_OFFSET(TW_RegionSlot, m_slot_manager,                       0x1C8);
TW_ASSERT_OFFSET(TW_CharacterDetails, traits,                         0x104);
TW_ASSERT_OFFSET(TW_Traits, m_size,                                   0x8);
TW_ASSERT_OFFSET(TW_Traits, m_elements,                               0xC);

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
TW_PTR_OFFSET(TW_RegionSlot,    0x8);
TW_PTR_OFFSET(TW_Settlement,    0x8);
TW_PTR_OFFSET(TW_PoliticalPartyRecord,   0x8);
TW_PTR_OFFSET(TW_CampaignPoliticalParty, 0x8);
TW_PTR_OFFSET(TW_CharacterDetailsArtSetInfo, 0x8);

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