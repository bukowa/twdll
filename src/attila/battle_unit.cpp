/// @module twdll.battle_unit
/// Battle unit properties for Total War: Attila.
#include "../common/tw.h"
#include "tw_types.h"

using twdll::TW_BattleUnit;
using twdll::TW_LandStats;

constexpr size_t BATTLE_UNIT_PTR = twdll::TW_PtrOffset<TW_BattleUnit>::value;

namespace Props {
    static twdll::Getter ChargeBonus {&TW_LandStats::charge_bonus,  BATTLE_UNIT_PTR, "battle_unit", {offsetof(TW_BattleUnit, stats)}};
    static twdll::Getter MeleeAttack {&TW_LandStats::melee_attack,  BATTLE_UNIT_PTR, "battle_unit", {offsetof(TW_BattleUnit, stats)}};
    static twdll::Getter BaseMorale  {&TW_LandStats::morale,        BATTLE_UNIT_PTR, "battle_unit", {offsetof(TW_BattleUnit, stats)}};
    static twdll::Getter MeleeDefence{&TW_LandStats::melee_defence, BATTLE_UNIT_PTR, "battle_unit", {offsetof(TW_BattleUnit, stats)}};
}

static int GetMemoryAddress   (lua_State* L) { return tw_mem_address(L, "battle_unit", BATTLE_UNIT_PTR); }
static int GetChargeBonus  (lua_State* L) { return Props::ChargeBonus.get(L); }
static int GetMeleeAttack  (lua_State* L) { return Props::MeleeAttack.get(L); }
static int GetBaseMorale   (lua_State* L) { return Props::BaseMorale.get(L); }
static int GetMeleeDefence (lua_State* L) { return Props::MeleeDefence.get(L); }

extern const luaL_Reg battle_unit_functions[] = {
    {"GetMemoryAddress",  GetMemoryAddress},
    {"GetChargeBonus",    GetChargeBonus},
    {"GetMeleeAttack",    GetMeleeAttack},
    {"GetBaseMorale",     GetBaseMorale},
    {"GetMeleeDefence",   GetMeleeDefence},
    {nullptr, nullptr}
};
