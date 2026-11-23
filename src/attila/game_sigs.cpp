#include "game_api.h"
#include "tw_types.h"
#include "../common/tw.h"
#include "../common/game_api.h"
#include "../common/log.h"

// clang-format off
#define NEW_FACTION_LEADER_SIG  "83 EC ? 53 55 56 57 8B F9 8D 4C 24 ? 68 ? ? ? ? 8B B7 ? ? ? ? E8 ? ? ? ? 8D 44 24"
#define DISBAND_UNITS_SIG   "81 EC 98 00 00 00 33 C0"
#define FACTION_PROVINCE_MANAGER_SIG "53 56 8B F1 57 8D 8E ? ? ? ? E8 ? ? ? ? 8D 8E"
#define REINF_CAP_SIG   "8B 80 3C 01 00 00 89 46 28"
#define BATTLE_CTOR_SIG "83 EC 0C 53 55 8B 6C 24 ? 56 57 8B F9 8B 45"
#define BATTLE_DTOR_SIG "51 53 55 56 57 8B F9 8B B7 ? ? ? ? 85 F6"
#define SETTLEMENT_CB_INITIALIZE_SIG "83 EC 74 53 55 56 8B F1 33 DB"
// clang-format on

#define INSTANT_SET_RESEARCHED_SIG "56 FF 74 24 ? 8B F1 E8 ? ? ? ? 85 C0 74 ? 6A 00 FF 74 24"
#define RECORD_INDEX_SIG "83 EC 0C 53 8B D9 56 8B 74 24 ? 56 8D 4B ? E8 ? ? ? ? 85 C0 74 ? 8B 08 8B 43 ? EB ? 57 8B CE E8 ? ? ? ? 8D 4B ? 8B F8 E8 ? ? ? ? 68 ? ? ? ? 8D 4C 24 ? 8B F0 E8 ? ? ? ? 57 8D 44 24 ? 56 50 E8 ? ? ? ? 83 C4 0C 8D 4C 24 ? E8 ? ? ? ? 8B 4B ? 8B C1 5F 3B C8 73 ? 8B 43 ? 5E 5B 8B 04 88 83 C4 0C"
#define CONVERT_UNIT_SIG "53 8B 5C 24 08 55 56 57 8D 4B 34 E8 ? ? ? ? 8D 4B 30 E8 ? ? ? ? 8D 88 F0 10 00 00 E8 ? ? ? ? 8B 6C 24 1C 8D 48 18 FF 75 28"
#define CAMPAIGN_MODEL_MAX_UNITS_SIG "C7 05 ? ? ? ? 14 00 00 00 C7 05 ? ? ? ? 0E 00 00 00"
#define RESOLVE_PORTRAIT_PATH_SIG "83 EC 08 55 8B E9 56 83 BD 54 03 00 00 00 0F 84"
#define CA_STRING_ASSIGN_SIG "83 EC 0C 56 57 8B F9 8B 4C 24 18 8B F1 8D 56 01"
#define CA_UNISTRING_ASSIGN_SIG "56 8B 74 24 08 8B C6 57 8B F9 66 0F 1F 44 00 00 0F B7 10"
#define UPDATE_ART_SET_SIG "83 EC 30 53 56 57 8B 7C 24 40 8B D9 8B 37 8B 43 3C 89 44 24"
#define ADD_TRAIT_SIG "56 8B F1 8D 8E ? ? ? ? E8 ? ? ? ? 8B C8 E8 ? ? ? ? FF 74 24 ? 8B C8 E8 ? ? ? ? 85 C0 74 ? FF 74 24 ? 8B CE FF 74 24"
#define SET_EFFECT_LIST_SIG "83 EC 0C 53 55 8B D9 8D 4C 24"
#define GET_LOYALTY_SIG "83 EC 20 56 8B F1 57 8B 8E"
#define GET_LOYALTY_FACTORS_SIG "83 EC 08 53 55 8B D9 C6 44 24 ? ? 56 57 8D BB"
#define GET_FACTION_RECORD_SIG "8B 81 ? ? ? ? 85 C0 75 ? 81 C1 58 01 00 00"
#define REASSIGN_FACTION_SIG "83 EC 08 53 55 8B D9 C6 44 24 ? ? 56 57 8D 8B"
#define SPAWN_AGENT_SIG "81 EC 08 05 00 00 53 55"
#define MAKE_OCCUPATION_DECISION_SIG "81 EC 50 01 00 00 53 55 56 57 8B BC 24"
#define SAVE_GAME_SIG "81 EC FC 01 00 00 53 55 56 8B B4 24"
#define LOAD_GAME_SIG "83 EC 14 56 FF 74 24 ? 8B F1 C6 44 24"
#define LOOKUP_CAMPAIGN_TECH_SIG "53 8B 59 0C 55 56 33 F6 57 85 DB 74 1B 8B 79 10"
#define UPDATE_TECH_EFFECTS_SIG  "83 EC 30 53 8B D9 56 57 89 5C 24 0C 8D 4B 24 E8 ? ? ? ? 33 C9 89 4C 24"
#define GET_TWEAKER_MAP_SIG "64 A1 ? ? ? ? 83 EC 18 8B 0D ? ? ? ? 8B 0C 88 A1 ? ? ? ? 3B 81 ? ? ? ? 7F ? B8 ? ? ? ? 83 C4 18 C3 68 ? ? ? ? E8 ? ? ? ? 83 C4 04 83 3D ? ? ? ? FF 75 ? 57 68 ? ? ? ? 8D 4C 24 ? E8 ? ? ? ? 68 ? ? ? ? 8D 4C 24 ? E8 ? ? ? ? 8D 44 24 ? C7 05 ? ? ? ? 08 00 00 00"

#define RECRUIT_CHARACTER_ENTRY_IMPL_SIG "83 EC 20 53 8B 5C 24 ? 55 56 8B F1 8B 43"
#define KILL_INSTANT_AND_COMMANDED_UNIT_SIG "83 EC 24 56 8B F1 57 8D 8E"
#define UPDATE_CARD_INFO_SIG "56 8B F1 57 8B 8E AC 00 00 00 E8 ? ? ? ? 8B 7C 24 0C 84 C0 75 0F 8B 86 AC 00 00 00 8B 80 04 01 00 00"
// Tail of the function that maps a campaign key to its content (DLC) flag:
//   test al,al / jnz +4 / xor eax,eax / pop esi / ret / mov eax,0x80 ... / mov eax,0x20 ... / mov eax,1 / pop esi / ret
// Bytes 4-5 ("xor eax,eax", the "unknown key" result) are wildcards so an already
// patched game file (EB 10) is matched too.
#define CAMPAIGN_KEY_CONTENT_TAIL_SIG "84 C0 75 04 ? ? 5E C3 B8 80 00 00 00 5E C3 B8 20 00 00 00 5E C3 B8 01 00 00 00 5E C3"
// "Gift Region" diplomacy availability (both directions, called with giver/receiver factions).
// The 2-byte conditional jumps that world.AllowGiftRegion patches are wildcards, so an already
// patched process or file still matches.
#define GIFT_REGION_AVAILABILITY_SIG "83 EC 1C 53 56 8B 74 24 2C 33 DB 57 8B 7C 24 2C 89 5C 24 0C 38 9F 4C 08 00 00 74 4D 8B CF E8 ? ? ? ? 84 C0 ? ? 8B CE E8 ? ? ? ? 84 C0 ? ? 8B CE E8 ? ? ? ? 84 C0 ? ? 8B 8F 24 08 00 00 56 E8 ? ? ? ? 8B 40 10 39 58 10 74 18 56 8D 44 24 20 8B CF 50 E8 ? ? ? ? BB 01 00 00 00 83 78 04 00 75 55 80 BE 4C 08 00 00 00 74 53 8B CE E8 ? ? ? ? 84 C0 ? ? 8B CF E8 ? ? ? ? 84 C0 ? ? 8B CF E8 ? ? ? ? 84 C0 ? ? 8B 8E"
// Inside the giftable-region list builder: "receiver given? then only its listed regions".
#define GIFT_REGION_LIST_FILTER_SIG "8B BE 28 01 00 00 8D 4F 5C E8 ? ? ? ? 85 C0 75 6E 8D 4F 7C E8 ? ? ? ? 85 C0 75 62 8B 54 24 20 85 D2 ? ?"
// Custom battle setup: selects the default campaign map with
//   mov eax,[edi+0x80] / lea ecx,[esp+8] / push offset "main_attila_map" / mov esi,[eax+0x184] / call String ctor / ... / call select
// The pushed string address (+0x0B) is a wildcard so a repointed immediate still matches.
#define CUSTOM_BATTLE_DEFAULT_MAP_SIG "8B 87 80 00 00 00 8D 4C 24 08 68 ? ? ? ? 8B B0 84 01 00 00 E8 ? ? ? ? 8D 44 24 08 8B CE 50 E8"
// Allocates N MERCENARY_POOL_UNIT on the game heap (used by the pool vector push_back).
#define MERC_ALLOC_UNITS_SIG "8B 44 24 04 33 C9 85 C0 6A 00 0F 49 C8 6B C1 2C 50 E8 ? ? ? ? 83 C4 08 C3"
// Frees a block from the CA game heap (cdecl, one argument).
#define CA_FREE_SIG "83 EC 08 56 8B 74 24 10 85 F6 74 2C 8D 44 24 04 56 50 E8 ? ? ? ? 8B 4C 24 10 83 C4 08 FF 74 24 04 E8 ? ? ? ? A1"
#define BATTLE_HEALTH_BAR_ON_UPDATE_PULSE_SIG "A1 ? ? ? ? 83 EC 48 8B 80 18 01 00 00 55 8B E9 80 B8 28 44 06 00 00"

FnNewFactionLeader g_new_faction_leader = nullptr;
FnDisbandUnits     g_disband_units       = nullptr;
HMODULE            g_empire_module        = nullptr;
#define GET_CHARACTER_MAP_PIECE_SIG "51 8D 44 24 ? 50 E8 ? ? ? ? 59 C3 CC CC CC 53"

FnFactionProvinceManager g_faction_province_manager = nullptr;
FnResolvePortraitPath    g_resolve_portrait_path    = nullptr;
FnCaStringAssign         g_ca_string_assign         = nullptr;
FnCaUniStringAssign      g_ca_unistring_assign      = nullptr;
FnUpdateArtSet           g_update_art_set           = nullptr;
FnRecruitCharacterEntryImpl g_recruit_character_entry_impl = nullptr;
FnGetCharacterMapPiece   g_get_character_map_piece  = nullptr;
FnKillInstantAndCommandedUnit g_kill_instant_and_commanded_unit = nullptr;
uintptr_t          g_reinf_cap_insn_addr = 0;
uintptr_t          g_battle_ctor_addr    = 0;
uintptr_t          g_battle_dtor_addr    = 0;
uintptr_t          g_settlement_cb_initialize_addr = 0;
uintptr_t          g_make_occupation_decision_addr = 0;
uintptr_t          g_update_card_info_addr = 0;
uintptr_t          g_battle_health_bar_on_update_pulse_addr = 0;
uintptr_t          g_campaign_key_content_tail_addr = 0;
uintptr_t          g_gift_region_availability_addr = 0;
uintptr_t          g_gift_region_list_filter_addr = 0;
uintptr_t          g_custom_battle_default_map_addr = 0;
FnMercAllocUnits   g_merc_alloc_units = nullptr;
FnCaFree           g_ca_free = nullptr;
static uintptr_t   g_max_units_init_insn_addr = 0;
static int*        g_max_units_in_army = nullptr;
static int*        g_max_units_in_navy = nullptr;
using FnGetTweakerMap = twdll::TW_TweakerMap*(__cdecl*)();
FnGetTweakerMap    g_get_tweaker_map = nullptr;
twdll::TW_TweakerMap* g_tweaker_map = nullptr;

FnInstantSetResearched g_instant_set_researched = nullptr;
FnLookupCampaignTech   g_lookup_campaign_tech   = nullptr;
FnUpdateTechEffects    g_update_tech_effects    = nullptr;
FnRecordIndex          g_record_index           = nullptr;
FnConvertUnit          g_convert_unit           = nullptr;
FnAddTrait             g_add_trait              = nullptr;
FnSetEffectList        g_set_effect_list        = nullptr;
FnGetLoyalty           g_get_loyalty            = nullptr;
FnGetLoyaltyFactors    g_get_loyalty_factors    = nullptr;
FnGetFactionRecord     g_get_faction_record     = nullptr;
FnReassignFaction      g_reassign_faction       = nullptr;
FnSpawnAgent           g_spawn_agent            = nullptr;
FnSaveGame             g_save_game              = nullptr;
FnLoadGame             g_load_game              = nullptr;

int* get_max_units_in_army() {
    if (!g_max_units_in_army && g_max_units_init_insn_addr) {
        g_max_units_in_army = *reinterpret_cast<int**>(g_max_units_init_insn_addr + 2);
        g_max_units_in_navy = *reinterpret_cast<int**>(g_max_units_init_insn_addr + 12);
        Log("[twdll] Resolved max_units_army @ 0x%08X (val: %d), max_units_navy @ 0x%08X (val: %d)",
            reinterpret_cast<uintptr_t>(g_max_units_in_army), g_max_units_in_army ? *g_max_units_in_army : 0,
            reinterpret_cast<uintptr_t>(g_max_units_in_navy), g_max_units_in_navy ? *g_max_units_in_navy : 0);
    }
    return g_max_units_in_army;
}

int* get_max_units_in_navy() {
    if (!g_max_units_in_navy && g_max_units_init_insn_addr) {
        g_max_units_in_army = *reinterpret_cast<int**>(g_max_units_init_insn_addr + 2);
        g_max_units_in_navy = *reinterpret_cast<int**>(g_max_units_init_insn_addr + 12);
        Log("[twdll] Resolved max_units_army @ 0x%08X (val: %d), max_units_navy @ 0x%08X (val: %d)",
            reinterpret_cast<uintptr_t>(g_max_units_in_army), g_max_units_in_army ? *g_max_units_in_army : 0,
            reinterpret_cast<uintptr_t>(g_max_units_in_navy), g_max_units_in_navy ? *g_max_units_in_navy : 0);
    }
    return g_max_units_in_navy;
}

twdll::TW_ITweaker* find_engine_tweaker(const char* name, size_t len) {
    if (!g_tweaker_map && g_get_tweaker_map) {
        g_tweaker_map = g_get_tweaker_map();
        Log("[twdll] Resolved global tweaker map @ 0x%08X (capacity: %u)",
            reinterpret_cast<uintptr_t>(g_tweaker_map),
            g_tweaker_map ? g_tweaker_map->m_capacity : 0);
    }
    if (!g_tweaker_map || !name || len == 0) return nullptr;
    std::wstring wname = tw_utf8_to_wide(name, len);
    return g_tweaker_map->find(wname.c_str(), wname.length());
}

void refresh_settlements_display() {
    auto* tweaker = find_engine_tweaker("force_settlement_refresh", 24);
    if (tweaker) {
        tweaker->set_raw_value(1);
        Log("[twdll] Triggered full settlement display refresh via tweaker");
    } else {
        Log("[twdll] WARNING: force_settlement_refresh tweaker not resolved");
    }
}

const TW_GameSigInfo g_game_signatures[] = {
    {"CHARACTER_DETAILS::portrait_path", (void**)&g_resolve_portrait_path, RESOLVE_PORTRAIT_PATH_SIG},
    {"CA::String::operator=",           (void**)&g_ca_string_assign,      CA_STRING_ASSIGN_SIG},
    {"CA::UniString::operator=",        (void**)&g_ca_unistring_assign,   CA_UNISTRING_ASSIGN_SIG},
    {"CHARACTER_DETAILS_ART_SET_INFO::update_art_set", (void**)&g_update_art_set, UPDATE_ART_SET_SIG},
    {"FACTION::new_faction_leader",     (void**)&g_new_faction_leader, NEW_FACTION_LEADER_SIG},
    {"UNIT::disband_units",             (void**)&g_disband_units,      DISBAND_UNITS_SIG},
    {"REGION::faction_province_manager", (void**)&g_faction_province_manager, FACTION_PROVINCE_MANAGER_SIG},
    {"REINFORCEMENTS_MANAGER::max_units_load", (void**)&g_reinf_cap_insn_addr, REINF_CAP_SIG},
    {"EMPIREBATTLE::MANAGER::ctor",            (void**)&g_battle_ctor_addr,    BATTLE_CTOR_SIG},
    {"EMPIREBATTLE::MANAGER::dtor",            (void**)&g_battle_dtor_addr,    BATTLE_DTOR_SIG},
    {"CampaignSettlementCallback::Initialize", (void**)&g_settlement_cb_initialize_addr, SETTLEMENT_CB_INITIALIZE_SIG},
    {"FACTION_TECHNOLOGY_MANAGER::instant_set_researched", (void**)&g_instant_set_researched, INSTANT_SET_RESEARCHED_SIG},
    {"FACTION_TECHNOLOGY_MANAGER::lookup_tech",            (void**)&g_lookup_campaign_tech,   LOOKUP_CAMPAIGN_TECH_SIG},
    {"FACTION_TECHNOLOGY_MANAGER::update_effect_list",      (void**)&g_update_tech_effects,    UPDATE_TECH_EFFECTS_SIG},
    {"DATABASE_TABLE::record_index",                       (void**)&g_record_index,           RECORD_INDEX_SIG},
    {"UNIT::convert_unit",                                 (void**)&g_convert_unit,           CONVERT_UNIT_SIG},
    {"CAMPAIGN_MODEL::max_units_init",                     (void**)&g_max_units_init_insn_addr, CAMPAIGN_MODEL_MAX_UNITS_SIG},
    {"CHARACTER::add_trait",                               (void**)&g_add_trait,              ADD_TRAIT_SIG},
    {"CHARACTER_TRAITS::set_effect_list",                  (void**)&g_set_effect_list,         SET_EFFECT_LIST_SIG},
    {"CHARACTER::get_loyalty",                             (void**)&g_get_loyalty,            GET_LOYALTY_SIG},
    {"CHARACTER::get_loyalty_factors",                     (void**)&g_get_loyalty_factors,    GET_LOYALTY_FACTORS_SIG},
    {"FACTION::get_faction_record",                        (void**)&g_get_faction_record,     GET_FACTION_RECORD_SIG},
    {"CHARACTER::reassign_faction",                        (void**)&g_reassign_faction,       REASSIGN_FACTION_SIG},
    {"CHARACTER_RECRUITMENT_POOL::spawn_agent",            (void**)&g_spawn_agent,            SPAWN_AGENT_SIG},
    {"CAI_DECISION::make_occupation_decision",             (void**)&g_make_occupation_decision_addr, MAKE_OCCUPATION_DECISION_SIG},
    {"CAMPAIGN_ENV::save_game",                            (void**)&g_save_game,              SAVE_GAME_SIG},
    {"CAMPAIGN_ENV::load_game",                            (void**)&g_load_game,              LOAD_GAME_SIG},
    {"UTILITYDLL::get_tweaker_map",                        (void**)&g_get_tweaker_map,        GET_TWEAKER_MAP_SIG},
    {"CHARACTER_RECRUITMENT_POOL_MANAGER::recruit_character_entry_impl", (void**)&g_recruit_character_entry_impl, RECRUIT_CHARACTER_ENTRY_IMPL_SIG},
    {"CHARACTER::get_map_piece",                           (void**)&g_get_character_map_piece, GET_CHARACTER_MAP_PIECE_SIG},
    {"CHARACTER::kill_instant_and_commanded_unit",         (void**)&g_kill_instant_and_commanded_unit, KILL_INSTANT_AND_COMMANDED_UNIT_SIG},
    {"UIDLL::BattleLandUnitCardStyle::update_information_changed", (void**)&g_update_card_info_addr, UPDATE_CARD_INFO_SIG},
    {"UIDLL::BattleHealthBar::OnUpdatePulse",              (void**)&g_battle_health_bar_on_update_pulse_addr, BATTLE_HEALTH_BAR_ON_UPDATE_PULSE_SIG},
    {"campaign_key_to_content_flag (tail)",                (void**)&g_campaign_key_content_tail_addr, CAMPAIGN_KEY_CONTENT_TAIL_SIG},
    {"diplomacy gift_region availability",                 (void**)&g_gift_region_availability_addr, GIFT_REGION_AVAILABILITY_SIG},
    {"diplomacy gift_region list filter",                  (void**)&g_gift_region_list_filter_addr, GIFT_REGION_LIST_FILTER_SIG},
    {"custom battle default campaign map",                 (void**)&g_custom_battle_default_map_addr, CUSTOM_BATTLE_DEFAULT_MAP_SIG},
    {"MERCENARY_POOL_UNIT allocate",                       (void**)&g_merc_alloc_units, MERC_ALLOC_UNITS_SIG},
    {"CA heap free",                                       (void**)&g_ca_free, CA_FREE_SIG},
    {nullptr, nullptr, nullptr}
};
