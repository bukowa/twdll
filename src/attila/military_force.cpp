/// @module twdll_military_force
/// Military force properties for Total War: Attila.
#include "../tw.h"
#include <cstddef>

// ── Memory layout ─────────────────────────────────────────────────────────────
// NOTE: Layout currently matches Rome 2. Verify before changing offsets.
#pragma pack(push, 1)
struct TW_MilitaryForce {
    char pad_00[0x45C];
    int  recruitment_queue_size;  // 0x45C
};
#pragma pack(pop)

static_assert(offsetof(TW_MilitaryForce, recruitment_queue_size) == 0x45C,
              "TW_MilitaryForce Attila: recruitment_queue_size");

constexpr size_t MIL_FORCE_PTR = 0x8;

// ── Accessors ─────────────────────────────────────────────────────────────────
/***
Returns the number of units in the recruitment queue.
@function GetRecruitmentQueueSize
@tparam userdata military_force the military force object (first argument)
@treturn integer number of units
*/
TW_GET(RecruitmentQueueSize, TW_MilitaryForce, recruitment_queue_size, MIL_FORCE_PTR, "military_force")

/***
Returns the memory address of the real military force object.
@function GetMemoryAddress
@tparam userdata military_force the military force object (first argument)
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemAddress(lua_State* L) { return tw_mem_address(L, "military_force", MIL_FORCE_PTR); }

// ── Lua registration table ────────────────────────────────────────────────────
extern const luaL_Reg military_force_functions[] = {
    {"GetMemoryAddress",        GetMemAddress},
    {"GetRecruitmentQueueSize", GetRecruitmentQueueSize},
    {nullptr, nullptr}
};
