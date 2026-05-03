/// @module twdll_battle_unit
/// Battle unit properties for Total War: Attila.
#include "../tw.h"
#include <cstddef>

// ── Memory layout ─────────────────────────────────────────────────────────────
// NOTE: Layout currently matches Rome 2. Verify before changing offsets.
#pragma pack(push, 1)
struct TW_UnitStats {
    char  pad_00[0x010C];
    float charge_bonus;   // 0x010C
    float float_example;  // 0x0110
    char  pad_114[0x4];
    float base_morale;    // 0x0118
    float melee_attack;   // 0x011C
};

struct TW_BattleUnit {
    char         pad_00[0x20];
    TW_UnitStats* unit_stats;  // 0x20
    char         pad_24[0x924];
    float        fatigue;      // 0x948
};
#pragma pack(pop)

static_assert(offsetof(TW_UnitStats,  charge_bonus) == 0x010C, "TW_UnitStats Attila: charge_bonus");
static_assert(offsetof(TW_UnitStats,  melee_attack) == 0x011C, "TW_UnitStats Attila: melee_attack");
static_assert(offsetof(TW_BattleUnit, unit_stats)   == 0x20,   "TW_BattleUnit Attila: unit_stats");
static_assert(offsetof(TW_BattleUnit, fatigue)      == 0x948,  "TW_BattleUnit Attila: fatigue");

constexpr size_t BATTLE_UNIT_PTR = 0x4;

// ── Accessors ─────────────────────────────────────────────────────────────────
/***
Gets the fatigue level of the battle unit.
@function GetFatigue
@tparam userdata battle_unit the battle unit object (first argument)
@treturn number fatigue level
*/
TW_GET(Fatigue, TW_BattleUnit, fatigue, BATTLE_UNIT_PTR, "battle_unit")

/***
Gets the charge bonus of the battle unit.
@function GetChargeBonus
@tparam userdata battle_unit the battle unit object (first argument)
@treturn number charge bonus
*/
TW_GET_NESTED(ChargeBonus,    TW_BattleUnit, unit_stats, TW_UnitStats, charge_bonus,  BATTLE_UNIT_PTR, "battle_unit")

/***
Gets the melee attack of the battle unit.
@function GetMeleeAttack
@tparam userdata battle_unit the battle unit object (first argument)
@treturn number melee attack
*/
TW_GET_NESTED(MeleeAttack,    TW_BattleUnit, unit_stats, TW_UnitStats, melee_attack,  BATTLE_UNIT_PTR, "battle_unit")

/***
Gets the base morale of the battle unit.
@function GetBaseMorale
@tparam userdata battle_unit the battle unit object (first argument)
@treturn number base morale
*/
TW_GET_NESTED(BaseMorale,     TW_BattleUnit, unit_stats, TW_UnitStats, base_morale,   BATTLE_UNIT_PTR, "battle_unit")

/***
Gets a float value of the battle unit.
@function GetSomeFloatValue
@tparam userdata battle_unit the battle unit object (first argument)
@treturn number float value
*/
TW_GET_NESTED(SomeFloatValue, TW_BattleUnit, unit_stats, TW_UnitStats, float_example, BATTLE_UNIT_PTR, "battle_unit")

/***
Returns the memory address of the real battle unit object.
@function GetMemoryAddress
@tparam userdata battle_unit the battle unit object (first argument)
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemAddress(lua_State* L) { return tw_mem_address(L, "battle_unit", BATTLE_UNIT_PTR); }

// ── Lua registration table ────────────────────────────────────────────────────
extern const luaL_Reg battle_unit_functions[] = {
    {"GetMemoryAddress",  GetMemAddress},
    {"GetFatigue",        GetFatigue},
    {"GetChargeBonus",    GetChargeBonus},
    {"GetMeleeAttack",    GetMeleeAttack},
    {"GetBaseMorale",     GetBaseMorale},
    {"GetSomeFloatValue", GetSomeFloatValue},
    {nullptr, nullptr}
};
