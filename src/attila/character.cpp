/// @module twdll.character
/// Campaign character properties for Total War: Attila.
#include "../common/tw.h"
#include "tw_types.h"

using twdll::TW_Character;

constexpr size_t CHAR_PTR = twdll::TW_PtrOffset<TW_Character>::value;

namespace Props {
    static twdll::Property ActionPoints{&TW_Character::action_points, CHAR_PTR, "character"};
    static twdll::Property Ambition    {&TW_Character::ambition,      CHAR_PTR, "character"};
    static twdll::Property Gravitas    {&TW_Character::gravitas,      CHAR_PTR, "character"};
}

static int GetMemoryAddress    (lua_State* L) { return tw_mem_address(L, "character", CHAR_PTR); }
static int GetIntAtOffset   (lua_State* L) { return tw_get_int_at(L, "character", CHAR_PTR); }
static int SetIntAtOffset   (lua_State* L) { return tw_set_int_at(L, "character", CHAR_PTR); }
static int GetActionPoints  (lua_State* L) { return Props::ActionPoints.get(L); }
static int SetActionPoints  (lua_State* L) { return Props::ActionPoints.set(L); }
static int GetAmbition      (lua_State* L) { return Props::Ambition.get(L); }
static int SetAmbition      (lua_State* L) { return Props::Ambition.set(L); }
static int GetGravitas      (lua_State* L) { return Props::Gravitas.get(L); }
static int SetGravitas      (lua_State* L) { return Props::Gravitas.set(L); }

extern const luaL_Reg character_functions[] = {
    {"GetMemoryAddress",  GetMemoryAddress},
    {"GetIntAtOffset",    GetIntAtOffset},
    {"SetIntAtOffset",    SetIntAtOffset},
    {"GetActionPoints",   GetActionPoints},
    {"SetActionPoints",   SetActionPoints},
    {"GetAmbition",       GetAmbition},
    {"SetAmbition",       SetAmbition},
    {"GetGravitas",       GetGravitas},
    {"SetGravitas",       SetGravitas},
    {nullptr, nullptr}
};
