/// @module CHARACTER_SCRIPT_INTERFACE
/// Extensions to the game's character object.
#include "../common/tw.h"
#include "../common/signature_scanner.h"
#include "../common/log.h"
#include "game_api.h"
#include "tw_types.h"
#include <windows.h>

using twdll::TW_Character;
using twdll::TW_GeneralBodyguardDetails;
using twdll::TW_CampaignModel;
using twdll::TW_CampaignEnv;
using twdll::TW_GameCore;
using twdll::TW_Databases;

constexpr size_t CHAR_PTR = twdll::TW_PtrOffset<TW_Character>::value;

namespace Props {
    static twdll::Property ActionPoints{&TW_Character::action_points, CHAR_PTR, "character"};
    static twdll::Property Ambition    {&TW_Character::ambition,      CHAR_PTR, "character"};
    static twdll::Property Gravitas    {&TW_Character::gravitas,      CHAR_PTR, "character"};
}

/***
Returns the memory address of the character object as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemoryAddress (lua_State* L) { return tw_mem_address(L, "character", CHAR_PTR); }

/***
Gets the current action points of the character.
@function GetActionPoints
@treturn integer action points
*/
static int GetActionPoints  (lua_State* L) { return Props::ActionPoints.get(L); }

/***
Sets the action points of the character.
@function SetActionPoints
@tparam integer value new action points
*/
static int SetActionPoints  (lua_State* L) { return Props::ActionPoints.set(L); }

/***
Gets the ambition value of the character.
@function GetAmbition
@treturn integer ambition
*/
static int GetAmbition      (lua_State* L) { return Props::Ambition.get(L); }

/***
Sets the ambition value of the character.
@function SetAmbition
@tparam integer value new ambition
*/
static int SetAmbition      (lua_State* L) { return Props::Ambition.set(L); }

/***
Gets the gravitas value of the character.
@function GetGravitas
@treturn integer gravitas
*/
static int GetGravitas      (lua_State* L) { return Props::Gravitas.get(L); }

/***
Sets the gravitas value of the character.
@function SetGravitas
@tparam integer value new gravitas
*/
static int SetGravitas      (lua_State* L) { return Props::Gravitas.set(L); }

/***
Overrides the default bodyguard unit record for a general so that whenever
the general is recruited into an army (including re-recruitment after being
wounded or disbanded, or through 'Replace this general' in the UI), they
always receive this unit type as their bodyguard. The record is stored directly
in the persistent GENERAL_BODYGUARD_DETAILS struct (serialised with savegames)
and enforced natively at the recruitment choke point.
@function SetDefaultBodyGuard
@tparam string unit_key unit record key (e.g. "att_rom_cav_general_guards")
@treturn boolean true if the record was found and applied, false otherwise
*/
static int SetDefaultBodyGuard(lua_State* L) {
    if (!g_record_index) {
        Log("[twdll] SetDefaultBodyGuard: g_record_index not resolved");
        return 0;
    }
    if (!g_campaign_model) {
        Log("[twdll] SetDefaultBodyGuard: campaign model not available");
        return 0;
    }
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        Log("[twdll] SetDefaultBodyGuard: null character");
        return 0;
    }
    size_t key_len = 0;
    const char* key = l_checklstring(L, 2, &key_len);
    if (!key || key_len == 0) {
        Log("[twdll] SetDefaultBodyGuard: unit_key not a string");
        return 0;
    }

    auto* env       = static_cast<TW_CampaignModel*>(g_campaign_model)->m_campaign_env;
    auto* game_core = static_cast<TW_CampaignEnv*>(env)->m_game_core;
    auto* databases = static_cast<TW_GameCore*>(game_core)->m_databases;
    void* units_table = static_cast<TW_Databases*>(databases)->m_main_units_table;
    if (!units_table) {
        Log("[twdll] SetDefaultBodyGuard: main_units_table not loaded");
        return 0;
    }

    struct RecordKey { uint32_t m_len; uint32_t m_pad; const char* m_data; }
        key_string = { static_cast<uint32_t>(key_len), 0, key };

    void* record = g_record_index(units_table, &key_string);
    if (!record) {
        Log("[twdll] SetDefaultBodyGuard: no record for key '%s'", key);
        l_pushboolean(L, 0);
        return 1;
    }

    ch->m_initial_general_bodyguard_details.m_unit = record;

    Log("[twdll] SetDefaultBodyGuard: character=0x%08X record=0x%08X key='%s'",
        reinterpret_cast<uintptr_t>(ch),
        reinterpret_cast<uintptr_t>(record), key);

    l_pushboolean(L, 1);
    return 1;
}

// ── Native Recruitment Bodyguard Choke-Point Hook ───────────────────────────
// Detour in recruit_character_entry_impl (@ 0x107E6DBB) right before UNIT::UNIT
// is constructed for the general's bodyguard. If the character has a persistent
// custom bodyguard assigned (via SetDefaultBodyGuard or loaded from savegame),
// we replace the unit record in GENERAL_BODYGUARD_DETAILS before creation.
static uintptr_t g_bodyguard_hook_addr = 0;
static uintptr_t g_bodyguard_hook_ret  = 0;
static uint8_t   g_bodyguard_hook_orig_bytes[7] = {0};

static void __cdecl OnBeforeCreateBodyguard(TW_Character* ch, TW_GeneralBodyguardDetails* bg) {
    if (ch && bg && ch->m_initial_general_bodyguard_details.m_unit != nullptr) {
        bg->m_unit = ch->m_initial_general_bodyguard_details.m_unit;
        Log("[twdll] [BODYGUARD] override character 0x%08X with bodyguard 0x%08X",
            reinterpret_cast<uintptr_t>(ch),
            reinterpret_cast<uintptr_t>(bg->m_unit));
    }
}

__declspec(naked) static void Hooked_CreateBodyguard() {
    __asm {
        mov ebx, [esp + 0x54]
        pushad
        push ebx   // bg
        push edi   // ch
        call OnBeforeCreateBodyguard
        add esp, 8
        popad
        cmp dword ptr [ebx], 0
        jmp dword ptr [g_bodyguard_hook_ret]
    }
}

void install_recruit_bodyguard_hook(uintptr_t base, size_t size) {
    const char* sig = "84 C0 0F 84 E7 03 00 00 8B 5C 24 54 83 3B 00";
    uintptr_t match = Scanner::find_signature(base, size, sig);
    if (!match) {
        Log("[twdll] [RECRUIT_BODYGUARD] signature not found");
        return;
    }

    uintptr_t hook_addr = match + 8; // points to 8B 5C 24 54 83 3B 00 (0x107E6DBB)
    g_bodyguard_hook_addr = hook_addr;
    g_bodyguard_hook_ret  = hook_addr + 7;

    memcpy(g_bodyguard_hook_orig_bytes, reinterpret_cast<void*>(hook_addr), 7);

    DWORD old_protect = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(hook_addr), 7, PAGE_EXECUTE_READWRITE, &old_protect)) {
        Log("[twdll] [RECRUIT_BODYGUARD] VirtualProtect failed (%lu)", GetLastError());
        return;
    }

    uintptr_t target = reinterpret_cast<uintptr_t>(Hooked_CreateBodyguard);
    int32_t rel_offset = static_cast<int32_t>(target - (hook_addr + 5));

    uint8_t patch[7];
    patch[0] = 0xE9; // JMP rel32
    *reinterpret_cast<int32_t*>(&patch[1]) = rel_offset;
    patch[5] = 0x90; // NOP
    patch[6] = 0x90; // NOP

    memcpy(reinterpret_cast<void*>(hook_addr), patch, 7);
    VirtualProtect(reinterpret_cast<void*>(hook_addr), 7, old_protect, &old_protect);
    FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(hook_addr), 7);

    Log("[twdll] [RECRUIT_BODYGUARD] hook installed OK @ 0x%08X -> 0x%08X", hook_addr, target);
}

void uninstall_recruit_bodyguard_hook() {
    if (g_bodyguard_hook_addr) {
        DWORD old_protect = 0;
        if (VirtualProtect(reinterpret_cast<void*>(g_bodyguard_hook_addr), 7, PAGE_EXECUTE_READWRITE, &old_protect)) {
            memcpy(reinterpret_cast<void*>(g_bodyguard_hook_addr), g_bodyguard_hook_orig_bytes, 7);
            VirtualProtect(reinterpret_cast<void*>(g_bodyguard_hook_addr), 7, old_protect, &old_protect);
            FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(g_bodyguard_hook_addr), 7);
        }
        g_bodyguard_hook_addr = 0;
        g_bodyguard_hook_ret  = 0;
    }
}

extern const luaL_Reg character_functions[] = {
    {nullptr, nullptr}
};

static const luaL_Reg character_methods[] = {
    {"GetMemoryAddress",      GetMemoryAddress},
    {"GetActionPoints",       GetActionPoints},
    {"SetActionPoints",       SetActionPoints},
    {"GetAmbition",           GetAmbition},
    {"SetAmbition",           SetAmbition},
    {"GetGravitas",           GetGravitas},
    {"SetGravitas",           SetGravitas},
    {"SetDefaultBodyGuard",   SetDefaultBodyGuard},
    {nullptr, nullptr}
};

void register_character_methods(lua_State* L) {
    l_newmetatable(L, "CHARACTER_SCRIPT_INTERFACE");
    l_getfield(L, -1, "__index");
    if (l_type(L, -1) == LUA_TTABLE) {
        for (const luaL_Reg* f = character_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] CHARACTER_SCRIPT_INTERFACE extended");
    } else {
        Log("[twdll] WARNING: CHARACTER_SCRIPT_INTERFACE __index not found");
    }
    l_pop(L, 2);
}