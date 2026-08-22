#pragma once
// tw_types.h — Engine object layouts for Total War: Attila (32-bit).
// Do NOT share with rome2/ — offsets differ between games.

#include <cstdint>
#include <cstddef>
#include <cstring>
#include <cwchar>
#include "game_api.h"
#include "../common/lua_api.h"

namespace twdll {

struct TW_FamilyMember {
    char             pad_00[0x18];
    TW_FamilyMember* mother;          // 0x18
    TW_FamilyMember* father;          // 0x1C
};

#pragma pack(push, 1)

// EMPIREUTILITY::POLITICAL_PARTY_RECORD — database record for political parties.
struct TW_CAString {
    uint32_t    m_len;      // 0x0
    uint32_t    m_capacity; // 0x4
    const char* m_data;     // 0x8
};

// CA::UniString layout (32-bit: length, capacity, UTF-16 wchar_t* buffer)
struct TW_CAUniString {
    uint32_t       m_len;      // 0x0
    uint32_t       m_capacity; // 0x4
    const wchar_t* m_data;     // 0x8
};

template <typename T> struct TW_Tweaker;

// UTILITYDLL::I_TWEAKER — base class for all engine tweakers (32-bit: size 0x48)
struct TW_ITweaker {
    void*          _vptr;            // 0x00
    TW_CAUniString m_name;           // 0x04
    TW_CAUniString m_file_name;      // 0x10
    int32_t        m_line_number;    // 0x1C
    TW_CAUniString m_tooltip_title;  // 0x20
    TW_CAUniString m_tooltip_text;   // 0x2C
    TW_CAUniString m_category;       // 0x38
    uint8_t        m_ev;             // 0x44
    uint8_t        m_dirty;          // 0x45
    uint8_t        pad_46[2];        // 0x46

    template <typename T>
    T& value();

    template <typename T>
    const T& value() const;

    uint32_t raw_value();
    void set_raw_value(uint32_t val);
};

// UTILITYDLL::TWEAKER<T> — engine tweak wrapper struct
template <typename T>
struct TW_Tweaker : TW_ITweaker {
    T m_value; // 0x48
};

template <typename T>
inline T& TW_ITweaker::value() {
    return static_cast<TW_Tweaker<T>*>(this)->m_value;
}

template <typename T>
inline const T& TW_ITweaker::value() const {
    return static_cast<const TW_Tweaker<T>*>(this)->m_value;
}

inline uint32_t TW_ITweaker::raw_value() {
    return value<uint32_t>();
}

inline void TW_ITweaker::set_raw_value(uint32_t val) {
    value<uint32_t>() = val;
    m_dirty = 1;
}

// Global Tweaker Registry dense_hash_map entry (size 0x10 = 16B)
struct TW_TweakerMapEntry {
    TW_CAUniString m_key;      // 0x00
    TW_ITweaker*   m_tweaker;  // 0x0C
};

// Global Tweaker Registry dense_hash_map table
struct TW_TweakerMap {
    TW_TweakerMapEntry* m_buckets;      // 0x00
    uint32_t            m_capacity;     // 0x04
    TW_CAUniString      m_empty_key;    // 0x08
    TW_CAUniString      m_deleted_key;  // 0x14

    TW_ITweaker* find(const wchar_t* name, size_t len) const {
        if (!m_buckets || !name || len == 0) return nullptr;
        for (uint32_t i = 0; i < m_capacity; ++i) {
            const auto& entry = m_buckets[i];
            if (entry.m_tweaker && entry.m_key.m_data && entry.m_key.m_len == len) {
                if (wcsncmp(entry.m_key.m_data, name, len) == 0) {
                    return entry.m_tweaker;
                }
            }
        }
        return nullptr;
    }
};


struct TW_PoliticalPartyRecord {
    TW_CAString m_key;          // 0x0
    char        pad_0C[0x38];
    float       m_initial_power;   // 0x44
};

// EMPIRECAMPAIGN::CAMPAIGN_POLITICAL_PARTY — active political party instance.
struct TW_CampaignPoliticalParty {
    void* m_politics;        // 0x0  (CAMPAIGN_POLITICS*)
    void* m_party_record;    // 0x4  (const POLITICAL_PARTY_RECORD*)
    char  pad_08[0xC];       // 0x8  m_senators_string (CA::UniString)
    int   m_senators;        // 0x14 (CA::card32)
    float m_power;           // 0x18 (CA::float32)
    char  pad_1C[0x10];      // 0x1C
};

// CA_STD::UNORDERED_MAP_NCC node layout (32-bit)
template <typename ValueT>
struct TW_HashNodeNCC {
    uint32_t                m_hash_or_prev; // 0x00
    TW_HashNodeNCC<ValueT>* m_next;         // 0x04
    void*                   m_key;          // 0x08
    ValueT                  m_value;        // 0x0C
};

// CA_STD::UNORDERED_MAP_NCC bucket layout (32-bit: size 12B)
template <typename NodeT>
struct TW_HashBucketNCC {
    NodeT* m_first;          // 0x00
    void*  m_sentinel;       // 0x04 (points to &m_sentinel in vanilla)
    void*  m_pad;            // 0x08
};

// CA_STD::UNORDERED_MAP_NCC — container layout for political parties.
struct TW_PoliticalPartiesMap {
    char                                                         pad_00[0x4];       // m_hash_function (1B) + m_equality_comparison (1B) + pad
    void*                                                        m_capacity;        // 0x4  bucket vector capacity
    int                                                          m_size;            // 0x8  bucket count
    TW_HashBucketNCC<TW_HashNodeNCC<TW_CampaignPoliticalParty>>* m_elements;        // 0xC  buckets array
    int                                                          m_count;           // 0x10 number of entries (map size)
    float                                                        m_max_load_factor; // 0x14

    template <typename Fn>
    void for_each(Fn&& fn) const {
        if (!m_elements || m_size <= 0) return;
        for (int i = 0; i < m_size; ++i) {
            const auto& bucket = m_elements[i];
            auto* node = bucket.m_first;
            const void* sentinel = &bucket.m_sentinel;
            while (node && node != reinterpret_cast<const TW_HashNodeNCC<TW_CampaignPoliticalParty>*>(sentinel)) {
                fn(&node->m_value);
                node = node->m_next;
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

// EMPIRECAMPAIGN::CAMPAIGN_POLITICS — faction politics manager.
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

struct TW_FactionRecord {
    TW_CAString m_key; // 0x00
};

struct TW_Faction {
    char pad_00[0x7DC];
    int  treasury;                  // 0x7DC
    char pad_7E0[0x20];
    TW_FactionRecord* m_faction_record; // 0x800
    char pad_804[0x48];
    bool is_major;                  // 0x84C
    char pad_84D[0x43];
    void* m_home_region;            // 0x890
    void* m_original_home_region;   // 0x894
    void* m_home_theatre;           // 0x898
    char  pad_89C[0x98];
    void* m_faction_technology_manager;  // 0x934
    char  pad_938[0x5A8];
    void* m_character_recruitment_pool;  // 0xEE0  (CHARACTER_RECRUITMENT_POOL_MANAGER*)
    char  pad_EE4[0x248];
    TW_CampaignPolitics m_politics;      // 0x112C
};

// EMPIRECAMPAIGN::TECHNOLOGY_STATUS (32-bit / 64-bit enum)
enum class TW_TechnologyStatus : uint32_t {
    RESEARCHED                  = 0,  // Completed / researched
    RESEARCHED_BUT_DISABLED     = 1,  // Researched but disabled (e.g. via bonus caps)
    BEING_RESEARCHED            = 2,  // Currently researching this turn
    AVAILABLE                   = 3,  // Available to research (clickable in UI)
    UNAVAILABLE                 = 4,  // Unavailable (prerequisites not met)
    NOT_PRESENT                 = 5,  // Not present in tree
    LOCKED_FACTION_LEVEL        = 6,  // Locked by faction / imperium level
    NUM_STATES                  = 7
};

// EMPIRECAMPAIGN::CAMPAIGN_TECHNOLOGY (32-bit layout, size >= 0x2C)
struct TW_CampaignTechnology {
    void*    m_technology_node_record;        // 0x00
    uint32_t m_technology_status;             // 0x04 (TW_TechnologyStatus)
    uint32_t m_research_points_completed;     // 0x08
    uint32_t pad_0C;                          // 0x0C
    char     m_parent_links[0xC];             // 0x10 (CA_STD::VECTOR)
    char     pad_1C[0xC];                     // 0x1C
    void*    m_technologies_for_category;     // 0x28
};

// EMPIREUTILITY::CAMPAIGN_CHARACTER_ART_SETS_CAMPAIGN_GROUP_RECORD
struct TW_CampaignCharacterArtSetsCampaignGroupRecord {
    TW_CAString m_key;   // 0x00 (e.g. "main")
};

// EMPIREUTILITY::CAMPAIGN_CHARACTER_ART_SET_RECORD (size 0x44)
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

struct TW_CharacterTraitRecord {
    TW_CAString m_key; // 0x00
};

struct TW_TraitInfoRecord {
    const TW_CharacterTraitRecord* m_character_trait; // 0x00
};

struct TW_MainUnitRecord {
    TW_CAString m_key;       // 0x00
    char        pad_0C[0x1C];
    uint32_t    m_num_men;   // 0x28
};

struct TW_LoyaltyFactorRecord {
    TW_CAString m_key; // 0x00
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
    const TW_TraitInfoRecord* m_record;       // 0x00 (const TRAIT_INFO_RECORD*)
    void*                     m_level_record; // 0x04 (const CHARACTER_TRAIT_LEVEL_RECORD*)
    int32_t                   m_points;       // 0x08
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

// EMPIRECAMPAIGN::CAMPAIGN_LOCALISATION (size 0x1C / 28B in 32-bit)
struct TW_CampaignLocalisation {
    TW_CAString            m_localisation_key;  // 0x00 (e.g. "names_name_12345")
    const TW_CAUniString*  m_localised_string;  // 0x0C (const CA::UniString* from DB loc cache)
    TW_CAUniString         m_custom_string;     // 0x10 (CA::UniString: len, cap, const wchar_t* data)
};
static_assert(sizeof(TW_CampaignLocalisation) == 0x1C, "TW_CampaignLocalisation size must be 0x1C");

// EMPIRECAMPAIGN::CHARACTER_NAME element (pair<CAMPAIGN_LOCALISATION, NAME_TYPE>, size 0x20 / 32B in 32-bit)
struct TW_CharacterNameEntry {
    TW_CampaignLocalisation m_localisation; // 0x00
    uint32_t                m_type;         // 0x1C (0=FORENAME, 1=FAMILY_NAME, 2=CLAN_NAME, 3=OTHER_NAME)
};
static_assert(sizeof(TW_CharacterNameEntry) == 0x20, "TW_CharacterNameEntry size must be 0x20");

struct TW_CharacterName {
    TW_CharacterNameEntry m_entries[4];     // 0x00 (Forename, FamilyName, ClanName, OtherName)
};
static_assert(sizeof(TW_CharacterName) == 0x80, "TW_CharacterName size must be 0x80");

struct TW_CharacterDetails {
    char                          pad_00[0x4];
    TW_FamilyMember*              family_member;       // 0x04  (= CHARACTER+0x208)
    TW_CharacterName              m_name;              // 0x08  (= CHARACTER+0x20C, size 0x80)
    char                          pad_88[0x24];        // 0x88  m_dob (12B), m_death_date (12B), m_death_type (4B), pad (8B)
    TW_CharacterDetailsArtSetInfo m_art_set_info;      // 0xAC  (= CHARACTER+0x2B0)
    char                          pad_EC[0x18];
    TW_Traits                     traits;              // 0x104 (= CHARACTER+0x308)
    char                          pad_12C[0x198];
    int8_t                        m_loyalty_modifier;  // 0x2C4 (= CHARACTER+0x4C8, direct loyalty modifier)
    char                          pad_2C5[0x27];
    void*                         m_political_party;   // 0x2EC (= CHARACTER+0x4F0, const POLITICAL_PARTY_RECORD*)
    char                          pad_2F0[0x4];
    int                           political_gravitas;  // 0x2F4 (= CHARACTER+0x4F8, CA::card32)
    char                          pad_2F8[0x24];
    TW_GeneralBodyguardDetails    m_initial_general_bodyguard_details; // 0x31C (= CHARACTER+0x520)
    char                          pad_330[0x18];
    bool                          m_is_immortal;                       // 0x348 (= CHARACTER+0x54C)
    char                          pad_349[3];
    uint32_t                      m_turns_to_resurrection;             // 0x34C (= CHARACTER+0x550)
    char                          pad_350[4];
    TW_PortraitCameraSettings*    m_portrait_camera_settings;          // 0x354 (= CHARACTER+0x558)
};

struct TW_Character {
    char                pad_00[0x14];
    int                 action_points;       // 0x14
    char                pad_18[0x1C4];
    void*               commanded_unit_link; // 0x1DC  m_commanded_unit.m_link.m_object
    char                pad_1E0[0x24];
    TW_CharacterDetails details;             // 0x204  CHARACTER_DETAILS embedded sub-struct
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

struct TW_UnitContainer {
    TW_MilitaryForce* m_military_force; // 0x00
};

struct TW_UnitForceLink {
    TW_UnitContainer* m_container;      // 0x00
};

struct TW_Unit {
    char              pad_00[0x38];
    TW_UnitForceLink* m_force_link;     // 0x38  (ONE_TO_MANY_LINK<UNIT_CONTAINER,UNIT>*)
    char              pad_3C[0x8];      // 0x3C  m_faction, 0x40 m_unit_record
    int               num_men;          // 0x44
    int               max_num_men;      // 0x48
    char              pad_4C[0x18];
    int               action_points;    // 0x64
    char              pad_68[0xAC];
    void*             m_commander_link; // 0x114 (ONE_TO_ONE_LINK to CHARACTER)

    TW_MilitaryForce* get_military_force() const {
        if (!m_force_link || !m_force_link->m_container) return nullptr;
        return m_force_link->m_container->m_military_force;
    }
};

struct TW_CaiFaction {
    char        pad_00[0xA4];
    int         m_num_regions; // 0xA4
    char        pad_A8[0x44];
    TW_Faction* m_faction;     // 0xEC
};

struct TW_CaiRegion {
    char        pad_00[0x114];
    const char* m_settlement_key; // 0x114
};

struct TW_CaiSettlement {
    char          pad_00[0x10];
    TW_CaiRegion* m_cai_region;   // 0x10
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

// EMPIRECAMPAIGN::CampaignSettlementCallback — UI slot rendering callback.
struct TW_SettlementCallback {
    char  pad_00[0x40];
    void* m_region;                 // 0x40  (EMPIRE_UTILITY SAFE_PTR deref -> REGION)
    bool  m_is_capital;             // 0x44  (1 = main/selected settlement card)
    char  pad_45[0x3];
    int   m_max_slots;              // 0x48  (max slots to render; 4 normal, 6 capital)
    int   m_num_available_slots;    // 0x4C  (settlement->m_slots.m_size)
};

// EMPIRECAMPAIGN::PROVINCE_DEVELOPMENT — province growth and development points.
struct TW_PROVINCE_DEVELOPMENT {
    void*        m_faction_province_manager; // 0x0  (back-ref to FPM)
    unsigned int m_development_points;       // 0x4  (spent dev points)
    unsigned int m_accumulated_growth;       // 0x8  (surplus population accumulator)
};

// EMPIRECAMPAIGN::FACTION_PROVINCE_MANAGER
struct TW_FACTION_PROVINCE_MANAGER {
    char pad_00[0x23C];
    TW_PROVINCE_DEVELOPMENT m_province_development; // 0x23C
};

// EMPIRECAMPAIGN::REGION_DATA
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

// EMPIREUTILITY::RELIGION_RECORD
struct TW_ReligionRecord {
    TW_CAString m_key;            // 0x00 (m_len @0, m_capacity @4, m_data @8)
    void*       m_onscreen;       // 0x0C (const CA::UniString*)
    int32_t     m_convertibility; // 0x10
    TW_CAString m_icon_path;      // 0x14
    uint32_t    m_colour;         // 0x20
    int32_t     m_sort_order;     // 0x24
};

// std::pair<const RELIGION_RECORD*, float>
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

// EMPIRECAMPAIGN::CAMPAIGN_MODEL
struct TW_CampaignModel {
    char  pad_00[0x10F0];
    void* m_campaign_env;             // 0x10F0  CAMPAIGN_ENV_MODEL_ACCESS::m_campaign_env (CAMPAIGN_ENV*)
    char  pad_10F4[0xC];              // 0x10F4
    float m_campaign_variables[714];  // 0x1100  CAMPAIGN_VARIABLES_ARRAY (714 floats, 2856B)
};

// EMPIRECAMPAIGN::CAMPAIGN_LOAD_GAME_DESCRIPTION
struct TW_CampaignLoadGameDescription {
    bool           m_pending;         // 0x00 (0x5C in CAMPAIGN_ENV)
    char           pad_01[3];
    TW_CAUniString m_filename;        // 0x04 (0x60 in CAMPAIGN_ENV)
    bool           m_load_from_cloud; // 0x10 (0x6C in CAMPAIGN_ENV)
    char           pad_11[3];
};

struct TW_CampaignEnv {
    char                           pad_00[0x30];
    void*                          m_game_core;         // 0x30  EMPIRECOMMON::GAME_CORE*
    char                           pad_34[0x28];
    TW_CampaignLoadGameDescription m_load_game;         // 0x5C  load game request descriptor
    char                           pad_70[0x2C];
    bool                           m_quit_to_main_menu; // 0x9C  request exit to main menu
    bool                           m_quit_to_windows;   // 0x9D  request exit to windows
};

// EMPIRECOMMON::GAME_CORE
struct TW_GameCore {
    char  pad_00[0x10];
    void* m_databases;      // 0x10  EMPIREUTILITY::EMPIRE_DATABASES*
};

// EMPIREUTILITY::DATABASE_TABLE — database table lookup interface.
struct TW_DatabaseTable {
    void*        _vptr;       // 0x00
    void*        m_capacity;  // 0x04
    uint32_t     m_size;      // 0x08
    void**       m_elements;  // 0x0C

    void* find_record(const char* key, size_t key_len = 0) const {
        if (!this || !g_record_index || !key) return nullptr;
        if (key_len == 0) key_len = std::strlen(key);
        struct RecordKey { uint32_t len; uint32_t pad; const char* data; }
            k = { static_cast<uint32_t>(key_len), 0, key };
        return g_record_index(const_cast<TW_DatabaseTable*>(this), &k);
    }
};

// EMPIREUTILITY::EMPIRE_DATABASES — container for game database tables.
struct TW_Databases {
    float             m_campaign_variables[714]; // 0x0000 (CAMPAIGN_VARIABLES_ARRAY, 714 floats, 0xB28 bytes)
    char              pad_B28[0x3F0];            // 0x0B28
    TW_DatabaseTable* political_parties; // 0xF18  (POLITICAL_PARTIES_TABLE)
    char              pad_F1C[0xE4];
    TW_DatabaseTable* main_units;        // 0x1000 (MAIN_UNITS_TABLE)
    char              pad_1004[0x164];
    TW_DatabaseTable* loyalty_factors;   // 0x1168 (LOYALTY_FACTORS_TABLE)
    char              pad_116C[0x3C0];
    TW_DatabaseTable* agents;            // 0x152C (AGENTS_TABLE)
    char              pad_1530[0xD4];
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

static_assert(sizeof(TW_ITweaker) == 0x48, "TW_ITweaker size must be 0x48");
TW_ASSERT_OFFSET(TW_ITweaker, m_name,          0x04);
TW_ASSERT_OFFSET(TW_ITweaker, m_file_name,     0x10);
TW_ASSERT_OFFSET(TW_ITweaker, m_line_number,   0x1C);
TW_ASSERT_OFFSET(TW_ITweaker, m_tooltip_title, 0x20);
TW_ASSERT_OFFSET(TW_ITweaker, m_tooltip_text,  0x2C);
TW_ASSERT_OFFSET(TW_ITweaker, m_category,      0x38);
TW_ASSERT_OFFSET(TW_ITweaker, m_ev,            0x44);
TW_ASSERT_OFFSET(TW_ITweaker, m_dirty,         0x45);
TW_ASSERT_OFFSET(TW_Tweaker<uint8_t>, m_value, 0x48);
static_assert(sizeof(TW_TweakerMapEntry) == 0x10, "TW_TweakerMapEntry size must be 0x10");
TW_ASSERT_OFFSET(TW_TweakerMapEntry, m_key,     0x00);
TW_ASSERT_OFFSET(TW_TweakerMapEntry, m_tweaker, 0x0C);
TW_ASSERT_OFFSET(TW_TweakerMap, m_buckets,      0x00);
TW_ASSERT_OFFSET(TW_TweakerMap, m_capacity,     0x04);
TW_ASSERT_OFFSET(TW_TweakerMap, m_empty_key,    0x08);
TW_ASSERT_OFFSET(TW_TweakerMap, m_deleted_key,  0x14);
TW_ASSERT_OFFSET(TW_Faction,       treasury,                0x7DC);
TW_ASSERT_OFFSET(TW_Faction,       m_faction_record,        0x800);
TW_ASSERT_OFFSET(TW_FactionRecord, m_key,                   0x0);
TW_ASSERT_OFFSET(TW_Faction,       is_major,                0x84C);
TW_ASSERT_OFFSET(TW_Faction,       m_home_region,            0x890);
TW_ASSERT_OFFSET(TW_Faction,       m_original_home_region,   0x894);
TW_ASSERT_OFFSET(TW_Faction,       m_home_theatre,           0x898);
TW_ASSERT_OFFSET(TW_Faction,       m_faction_technology_manager, 0x934);
TW_ASSERT_OFFSET(TW_Faction,       m_character_recruitment_pool, 0xEE0);
TW_ASSERT_OFFSET(TW_Faction,       m_politics,               0x112C);

TW_ASSERT_OFFSET(TW_CampaignTechnology, m_technology_node_record, 0x00);
TW_ASSERT_OFFSET(TW_CampaignTechnology, m_technology_status, 0x04);
TW_ASSERT_OFFSET(TW_CampaignTechnology, m_research_points_completed, 0x08);
TW_ASSERT_OFFSET(TW_CampaignTechnology, m_technologies_for_category, 0x28);

TW_ASSERT_OFFSET(TW_Databases,     agents,                   0x152C);
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
TW_ASSERT_OFFSET(TW_UnitContainer, m_military_force,        0x0);
TW_ASSERT_OFFSET(TW_UnitForceLink, m_container,             0x0);
TW_ASSERT_OFFSET(TW_Unit,          m_force_link,            0x38);
TW_ASSERT_OFFSET(TW_Unit,          num_men,                 0x44);
TW_ASSERT_OFFSET(TW_Unit,          max_num_men,             0x48);
TW_ASSERT_OFFSET(TW_Unit,          action_points,           0x64);
TW_ASSERT_OFFSET(TW_Unit,          m_commander_link,        0x114);
TW_ASSERT_OFFSET(TW_CaiFaction,    m_num_regions,           0xA4);
TW_ASSERT_OFFSET(TW_CaiFaction,    m_faction,               0xEC);
TW_ASSERT_OFFSET(TW_CaiRegion,     m_settlement_key,        0x114);
TW_ASSERT_OFFSET(TW_CaiSettlement, m_cai_region,            0x10);
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
TW_ASSERT_OFFSET(TW_CampaignModel, m_campaign_variables,     0x1100);
TW_ASSERT_OFFSET(TW_Databases,     m_campaign_variables,     0x0000);
TW_ASSERT_OFFSET(TW_CampaignEnv,   m_game_core,              0x30);
TW_ASSERT_OFFSET(TW_CampaignEnv,   m_load_game,              0x5C);
TW_ASSERT_OFFSET(TW_CampaignEnv,   m_quit_to_main_menu,      0x9C);
TW_ASSERT_OFFSET(TW_CampaignEnv,   m_quit_to_windows,        0x9D);
TW_ASSERT_OFFSET(TW_CampaignLoadGameDescription, m_pending,         0x00);
TW_ASSERT_OFFSET(TW_CampaignLoadGameDescription, m_filename,        0x04);
TW_ASSERT_OFFSET(TW_CampaignLoadGameDescription, m_load_from_cloud, 0x10);
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
TW_ASSERT_OFFSET(TW_PoliticalPartiesMap, m_count,            0x10);
static_assert(sizeof(TW_HashBucketNCC<TW_HashNodeNCC<TW_CampaignPoliticalParty>>) == 12, "TW_HashBucketNCC size must be 12");
TW_ASSERT_OFFSET(TW_HashNodeNCC<TW_CampaignPoliticalParty>, m_next,  0x04);
TW_ASSERT_OFFSET(TW_HashNodeNCC<TW_CampaignPoliticalParty>, m_value, 0x0C);
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
TW_ASSERT_OFFSET(TW_CharacterDetails, m_name,                         0x08);
TW_ASSERT_OFFSET(TW_CharacterDetails, m_is_immortal,                   0x348);
TW_ASSERT_OFFSET(TW_CharacterDetails, m_turns_to_resurrection,         0x34C);
TW_ASSERT_OFFSET(TW_Traits, m_size,                                   0x8);
TW_ASSERT_OFFSET(TW_Traits, m_elements,                               0xC);
TW_ASSERT_OFFSET(TW_CharacterTraitRecord, m_key,                      0x0);
TW_ASSERT_OFFSET(TW_TraitInfoRecord, m_character_trait,               0x0);
TW_ASSERT_OFFSET(TW_LoyaltyFactorRecord, m_key,                       0x0);
TW_ASSERT_OFFSET(TW_MainUnitRecord, m_key,                            0x0);
TW_ASSERT_OFFSET(TW_MainUnitRecord, m_num_men,                        0x28);
TW_ASSERT_OFFSET(TW_Unit, m_commander_link,                           0x114);
TW_ASSERT_OFFSET(TW_DatabaseTable, m_size,                            0x8);
TW_ASSERT_OFFSET(TW_DatabaseTable, m_elements,                        0xC);

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
TW_PTR_OFFSET(TW_ITweaker,                0x8);

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