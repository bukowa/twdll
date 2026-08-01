/// @module MILITARY_FORCE_SCRIPT_INTERFACE
/// Extensions to the game's military force object.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

using twdll::TW_MilitaryForce;

constexpr size_t MIL_FORCE_PTR = twdll::TW_PtrOffset<TW_MilitaryForce>::value;

namespace Props {
    static twdll::Getter RecruitmentQueueSize{&TW_MilitaryForce::recruitment_queue_size, MIL_FORCE_PTR, "military_force"};
}

static int GetMemoryAddress           (lua_State* L) { return tw_mem_address(L, "military_force", MIL_FORCE_PTR); }
static int GetRecruitmentQueueSize (lua_State* L) { return Props::RecruitmentQueueSize.get(L); }

extern const luaL_Reg military_force_functions[] = {
    {"GetMemoryAddress",        GetMemoryAddress},
    {"GetRecruitmentQueueSize", GetRecruitmentQueueSize},
    {nullptr, nullptr}
};
