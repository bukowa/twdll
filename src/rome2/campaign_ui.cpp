/// @module twdll.campaign_ui
/// Campaign UI singleton accessor for Total War: Rome 2.
#include "../common/tw.h"
#include "../common/campaign_hooks.h"
#include <cstddef>
#include <cstdio>

#pragma pack(push, 1)
struct TW_CampaignUi {
    char pad_00[0x10];
};
#pragma pack(pop)

TW_CampaignUi* g_campaign_ui = nullptr;

static int GetMemoryAddress(lua_State* L) {
    if (!g_campaign_ui) { l_pushnil(L); return 1; }
    char buf[20];
    snprintf(buf, sizeof(buf), "0x%08X", reinterpret_cast<unsigned int>(g_campaign_ui));
    l_pushstring(L, buf);
    return 1;
}

extern const luaL_Reg campaign_ui_functions[] = {
    {"GetMemoryAddress", GetMemoryAddress},
    {nullptr, nullptr}
};
