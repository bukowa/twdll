/// @module twdll.campaign_ui
/// Campaign UI singleton accessor for Total War: Rome 2.
/// The pointer is captured at runtime when the game constructs the CAMPAIGN_UI object.
#include "../common/tw.h"
#include "../common/campaign_hooks.h"
#include <cstddef>
#include <cstdio>

// ── Memory layout ─────────────────────────────────────────────────────────────
// Fields TBD — structure not yet mapped for Rome 2.
#pragma pack(push, 1)
struct TW_CampaignUi {
    // placeholder — add verified fields as they are discovered
};
#pragma pack(pop)

// ── Accessors (global singleton — not Lua userdata) ───────────────────────────

/***
Returns the memory address of the CAMPAIGN_UI singleton as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678"), or nil if not yet initialised
*/
static int GetMemoryAddress(lua_State* L) {
    if (!g_campaign_ui) { l_pushnil(L); return 1; }
    char buf[20];
    snprintf(buf, sizeof(buf), "0x%08X", reinterpret_cast<unsigned int>(g_campaign_ui));
    l_pushstring(L, buf);
    return 1;
}

// ── Lua registration table ────────────────────────────────────────────────────
extern const luaL_Reg campaign_ui_functions[] = {
    {"GetMemoryAddress", GetMemoryAddress},
    {nullptr, nullptr}
};
