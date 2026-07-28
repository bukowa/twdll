/// @module twdll.battle_unit
/// Battle unit properties for Total War: Attila.
#include "../common/tw.h"
#include "tw_types.h"

using twdll::TW_BattleUnit;

constexpr size_t BATTLE_UNIT_PTR = twdll::TW_PtrOffset<TW_BattleUnit>::value;

namespace Props {
    static twdll::Getter<int, TW_BattleUnit> ChargeBonus {twdll::offset_tag, offsetof(TW_BattleUnit, stats.charge_bonus),  BATTLE_UNIT_PTR, "battle_unit"};
    static twdll::Getter<int, TW_BattleUnit> MeleeAttack {twdll::offset_tag, offsetof(TW_BattleUnit, stats.melee_attack),  BATTLE_UNIT_PTR, "battle_unit"};
    static twdll::Getter<int, TW_BattleUnit> BaseMorale  {twdll::offset_tag, offsetof(TW_BattleUnit, stats.morale),        BATTLE_UNIT_PTR, "battle_unit"};
    static twdll::Getter<int, TW_BattleUnit> MeleeDefence{twdll::offset_tag, offsetof(TW_BattleUnit, stats.melee_defence), BATTLE_UNIT_PTR, "battle_unit"};
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
