/// @module twdll.battle_unit
/// Battle unit properties for Rome 2.
#include "../common/tw.h"
#include <cstddef>

#pragma pack(push, 1)
struct TW_BattleUnit {
    char pad_00[0x948];
    float fatigue;      // 0x948
};
#pragma pack(pop)

constexpr size_t BATTLE_UNIT_PTR = 0x4;

static twdll::Getter<float, TW_BattleUnit> Fatigue{&TW_BattleUnit::fatigue, BATTLE_UNIT_PTR, "battle_unit"};
static int GetFatigue(lua_State* L) { return Fatigue.get(L); }

static int GetMemAddress(lua_State* L) { return tw_mem_address(L, "battle_unit", BATTLE_UNIT_PTR); }

extern const luaL_Reg battle_unit_functions[] = {
    {"GetMemoryAddress", GetMemAddress},
    {"GetFatigue",       GetFatigue},
    {nullptr, nullptr}
};
