#include "game_api.h"
#include "tw_types.h"
#include "../common/tw.h"
#include "../common/game_api.h"
#include "../common/log.h"

// clang-format off
#define NEW_FACTION_LEADER_SIG  "83 EC ? 53 55 56 57 8B F9 8D 4C 24 ? 68 ? ? ? ? 8B B7 ? ? ? ? E8 ? ? ? ? 8D 44 24"
#define DISBAND_UNITS_SIG   "81 EC 98 00 00 00 33 C0"
// REGION::faction_province_manager entry: mov esi,ecx (this=REGION*) then two
// CONST_SAFE_PTR::deref calls (province link +0x350, owning faction +0x120) and a
// PROVINCE sub-call, with a fallback that reads [province+0x5C]->[0]. Verified unique.
#define FACTION_PROVINCE_MANAGER_SIG "53 56 8B F1 57 8D 8E ? ? ? ? E8 ? ? ? ? 8D 8E"
// REINFORCEMENTS_MANAGER ctor: `mov eax, [eax+0x13C]` (load max units per army
// from the battle setup) followed by `mov [esi+0x28], eax` (store into the
// manager field). Unique in the image; used as the reinforcement cap patch site.
#define REINF_CAP_SIG   "8B 80 3C 01 00 00 89 46 28"
// EMPIREBATTLE::MANAGER ctor/dtor entry points. Verified unique in the image;
// both signatures start at the function entry, so they can be hooked directly.
#define BATTLE_CTOR_SIG "83 EC 0C 53 55 8B 6C 24 ? 56 57 8B F9 8B 45"
#define BATTLE_DTOR_SIG "51 53 55 56 57 8B F9 8B B7 ? ? ? ? 85 F6"
// CampaignSettlementCallback::Initialize entry point (thiscall, this in ecx).
// The building-slot render loop reads m_max_slots at this+0x48 on every
// iteration (cmp [esi+48h], ... @ 0x113E2513 / 0x113E274C), so hooking here
// lets twdll override the slot count before the panel renders. Verified unique.
#define SETTLEMENT_CB_INITIALIZE_SIG "83 EC 74 53 55 56 8B F1 33 DB"
// clang-format on

// FACTION_TECHNOLOGY_MANAGER instant-research path (see game_api.h).
// instant_set_researched_without_updating_effects wrapper:
//   `push esi; push [esp+4+arg_0]; mov esi,ecx; call finder; ...` @ 0x10B9D3D0
#define INSTANT_SET_RESEARCHED_SIG "56 FF 74 24 ? 8B F1 E8 ? ? ? ? 85 C0 74 ? 6A 00 FF 74 24"
// DATABASE_TABLE::record_index(table, &CA::String key) @ 0x10192660.
// The `68 ? ? ? ?` wildcards the `push 0x11AB8010` (error-string address): it is
// an absolute image pointer that ASLR relocates at load, so only the on-disk
// bytes would match a hardcoded sig — it must stay wildcarded.
#define RECORD_INDEX_SIG "83 EC 0C 53 8B D9 56 8B 74 24 ? 56 8D 4B ? E8 ? ? ? ? 85 C0 74 ? 8B 08 8B 43 ? EB ? 57 8B CE E8 ? ? ? ? 8D 4B ? 8B F8 E8 ? ? ? ? 68 ? ? ? ? 8D 4C 24 ? 8B F0 E8 ? ? ? ? 57 8D 44 24 ? 56 50 E8 ? ? ? ? 83 C4 0C 8D 4C 24 ? E8 ? ? ? ? 8B 4B ? 8B C1 5F 3B C8 73 ? 8B 43 ? 5E 5B 8B 04 88 83 C4 0C"
// UNIT::convert_unit @ 0x106FC010: break force link (lea ecx,[ebx+34h]),
// read m_soldiers, clamp vs unit-multiplier(num_men), LIST_POOL::allocate(0x164),
// 4-arg ctor, subsume into force, destroy old. Verified unique.
#define CONVERT_UNIT_SIG "53 8B 5C 24 08 55 56 57 8D 4B 34 E8 ? ? ? ? 8D 4B 30 E8 ? ? ? ? 8D 88 F0 10 00 00 E8 ? ? ? ? 8B 6C 24 1C 8D 48 18 FF 75 28"
// CAMPAIGN_BUILDING_DISPLAY::update_animation @ 0x10B1A790
// Reads force_settlement_refresh tweak @ offset 0x17 from entry (mov ecx, &tweak).
#define UPDATE_ANIMATION_SIG "51 56 8B F1 8B 86 ? ? ? ? 8B 48"
// CHARACTER_DETAILS::portrait_path @ 0x107DC9E0
#define RESOLVE_PORTRAIT_PATH_SIG "83 EC 08 55 8B E9 56 83 BD 54 03 00 00 00 0F 84"
// CA::String::operator=(const char*) @ 0x100DBD40
#define CA_STRING_ASSIGN_SIG "83 EC 0C 56 57 8B F9 8B 4C 24 18 8B F1 8D 56 01"
// CA::UniString::operator=(const wchar_t*) @ 0x100DB660
#define CA_UNISTRING_ASSIGN_SIG "56 8B 74 24 08 8B C6 57 8B F9 66 0F 1F 44 00 00 0F B7 10"
// CHARACTER_DETAILS_ART_SET_INFO::update_art_set @ 0x107F83D0
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
// Direct signature for UTILITYDLL::get_tweaker_map singleton getter (sub_11631B20)
#define GET_TWEAKER_MAP_SIG "64 A1 ? ? ? ? 83 EC 18 8B 0D ? ? ? ? 8B 0C 88 A1 ? ? ? ? 3B 81 ? ? ? ? 7F ? B8 ? ? ? ? 83 C4 18 C3 68 ? ? ? ? E8 ? ? ? ? 83 C4 04 83 3D ? ? ? ? FF 75 ? 57 68 ? ? ? ? 8D 4C 24 ? E8 ? ? ? ? 68 ? ? ? ? 8D 4C 24 ? E8 ? ? ? ? 8D 44 24 ? C7 05 ? ? ? ? 08 00 00 00"

FnNewFactionLeader g_new_faction_leader = nullptr;
FnDisbandUnits     g_disband_units       = nullptr;
HMODULE            g_empire_module        = nullptr;
FnFactionProvinceManager g_faction_province_manager = nullptr;
FnResolvePortraitPath    g_resolve_portrait_path    = nullptr;
FnCaStringAssign         g_ca_string_assign         = nullptr;
FnCaUniStringAssign      g_ca_unistring_assign      = nullptr;
FnUpdateArtSet           g_update_art_set           = nullptr;
uintptr_t          g_reinf_cap_insn_addr = 0;
uintptr_t          g_battle_ctor_addr    = 0;
uintptr_t          g_battle_dtor_addr    = 0;
uintptr_t          g_settlement_cb_initialize_addr = 0;
uintptr_t          g_make_occupation_decision_addr = 0;
static uintptr_t   g_update_animation_addr = 0;
static uintptr_t   g_force_settlement_refresh_addr = 0;
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

const uintptr_t OFFSET_MAX_UNITS_ARMY = 0x1CC91F0;
const uintptr_t OFFSET_MAX_UNITS_NAVY = 0x1CC91F4;
const uintptr_t OFFSET_MAX_TRAITS     = 0x2188610;

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
    if (!g_force_settlement_refresh_addr && g_update_animation_addr) {
        g_force_settlement_refresh_addr = *reinterpret_cast<uintptr_t*>(g_update_animation_addr + 0x17);
        Log("[twdll] Resolved force_settlement_refresh tweak @ 0x%08X", g_force_settlement_refresh_addr);
    }
    if (g_force_settlement_refresh_addr) {
        auto* tweaker = reinterpret_cast<twdll::TW_Tweaker<uint8_t>*>(g_force_settlement_refresh_addr);
        tweaker->m_value = 1;
        Log("[twdll] Triggered full settlement display refresh");
    } else {
        Log("[twdll] WARNING: force_settlement_refresh tweak address not resolved");
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
    {"CAMPAIGN_BUILDING_DISPLAY::update_animation",        (void**)&g_update_animation_addr,  UPDATE_ANIMATION_SIG},
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
    {nullptr, nullptr, nullptr}
};
