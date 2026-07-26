/// @module twdll.military_force
/// Military force properties for Total War: Attila.
#include "../common/tw.h"
#include "tw_types.h"

using twdll::TW_MilitaryForce;

constexpr size_t MIL_FORCE_PTR = twdll::TW_PtrOffset<TW_MilitaryForce>::value;

namespace Props {
    static twdll::Getter RecruitmentQueueSize{&TW_MilitaryForce::recruitment_queue_size, MIL_FORCE_PTR, "military_force"};
}

static int GetMemAddress           (lua_State* L) { return tw_mem_address(L, "military_force", MIL_FORCE_PTR); }
static int GetRecruitmentQueueSize (lua_State* L) { return Props::RecruitmentQueueSize.get(L); }

extern const luaL_Reg military_force_functions[] = {
    {"GetMemoryAddress",        GetMemAddress},
    {"GetRecruitmentQueueSize", GetRecruitmentQueueSize},
    {nullptr, nullptr}
};
