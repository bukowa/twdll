/// @module twdll.world
/// Campaign WORLD singleton accessor for Total War: Rome 2.
#include "../common/tw.h"
#include "../common/campaign_hooks.h"
#include <cstddef>
#include <cstdio>

#pragma pack(push, 1)
struct TW_World {
    char pad_00[0x50];
    int  faction_count;
};
#pragma pack(pop)

TW_World* g_world = nullptr;

static int GetMemoryAddress(lua_State* L) {
    if (!g_world) { l_pushnil(L); return 1; }
    char buf[20];
    snprintf(buf, sizeof(buf), "0x%08X", reinterpret_cast<unsigned int>(g_world));
    l_pushstring(L, buf);
    return 1;
}

static twdll::GlobalGetter<int, TW_World> FactionCount{&TW_World::faction_count, &g_world};

static int GetFactionCount(lua_State* L) { return FactionCount.get(L); }

extern const luaL_Reg world_functions[] = {
    {"GetMemoryAddress", GetMemoryAddress},
    {"GetFactionCount",  GetFactionCount},
    {nullptr, nullptr}
};
