/// @module twdll_battle_unit
/// Battle unit properties for Rome 2.
#include "../common/tw.h"
#include <cstddef>

// ── Memory layout ─────────────────────────────────────────────────────────────
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

static_assert(offsetof(TW_UnitStats,  charge_bonus) == 0x010C, "TW_UnitStats Rome2: charge_bonus");
static_assert(offsetof(TW_UnitStats,  melee_attack) == 0x011C, "TW_UnitStats Rome2: melee_attack");
static_assert(offsetof(TW_BattleUnit, unit_stats)   == 0x20,   "TW_BattleUnit Rome2: unit_stats");
static_assert(offsetof(TW_BattleUnit, fatigue)      == 0x948,  "TW_BattleUnit Rome2: fatigue");

constexpr size_t BATTLE_UNIT_PTR = 0x4;

// ── Accessors ─────────────────────────────────────────────────────────────────
// Direct properties
/***
Gets the fatigue level of the battle unit.
@function GetFatigue
@tparam userdata battle_unit the battle unit object (first argument)
@treturn number fatigue level
*/
static twdll::Getter<float, TW_BattleUnit> Fatigue{&TW_BattleUnit::fatigue, BATTLE_UNIT_PTR, "battle_unit"};
static int GetFatigue(lua_State* L) { return Fatigue.get(L); }

// Nested properties - UnitStats
namespace UnitStatsProps {
    // These are accessed through the parent's unit_stats pointer field
    /***
    Gets the charge bonus of the battle unit.
    @function GetChargeBonus
    @tparam userdata battle_unit the battle unit object (first argument)
    @treturn number charge bonus
    */
    static twdll::NestedProperty<float, TW_BattleUnit, TW_UnitStats> ChargeBonus{
        &TW_UnitStats::charge_bonus, &TW_BattleUnit::unit_stats, BATTLE_UNIT_PTR, "battle_unit"
    };

    /***
    Gets the melee attack of the battle unit.
    @function GetMeleeAttack
    @tparam userdata battle_unit the battle unit object (first argument)
    @treturn number melee attack
    */
    static twdll::NestedProperty<float, TW_BattleUnit, TW_UnitStats> MeleeAttack{
        &TW_UnitStats::melee_attack, &TW_BattleUnit::unit_stats, BATTLE_UNIT_PTR, "battle_unit"
    };

    /***
    Gets the base morale of the battle unit.
    @function GetBaseMorale
    @tparam userdata battle_unit the battle unit object (first argument)
    @treturn number base morale
    */
    static twdll::NestedProperty<float, TW_BattleUnit, TW_UnitStats> BaseMorale{
        &TW_UnitStats::base_morale, &TW_BattleUnit::unit_stats, BATTLE_UNIT_PTR, "battle_unit"
    };

    /***
    Gets a float value of the battle unit.
    @function GetSomeFloatValue
    @tparam userdata battle_unit the battle unit object (first argument)
    @treturn number float value
    */
    static twdll::NestedProperty<float, TW_BattleUnit, TW_UnitStats> SomeFloatValue{
        &TW_UnitStats::float_example, &TW_BattleUnit::unit_stats, BATTLE_UNIT_PTR, "battle_unit"
    };
}

// Wrapper functions for Lua registration
static int GetChargeBonus(lua_State* L) { return UnitStatsProps::ChargeBonus.get(L); }
static int GetMeleeAttack(lua_State* L) { return UnitStatsProps::MeleeAttack.get(L); }
static int GetBaseMorale(lua_State* L) { return UnitStatsProps::BaseMorale.get(L); }
static int GetSomeFloatValue(lua_State* L) { return UnitStatsProps::SomeFloatValue.get(L); }

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
