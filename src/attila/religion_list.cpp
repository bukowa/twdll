/// @module RELIGION_LIST_SCRIPT_INTERFACE
/// List interface for religions present in a region.
#include "../common/tw.h"
#include "tw_types.h"

using twdll::TW_ReligionProportion;
using twdll::TW_ReligionRecord;

void push_religion(lua_State* L, const TW_ReligionRecord* record, float proportion);

static const char* kReligionListMetatable = "RELIGION_LIST_SCRIPT_INTERFACE";

struct ReligionListUserdata {
    int count;
    TW_ReligionProportion items[1];
};

static ReligionListUserdata* get_religion_list(lua_State* L) {
    return static_cast<ReligionListUserdata*>(l_touserdata(L, 1));
}

/***
Number of religious denominations in this list.
@function num_items
@treturn integer number of religions
@usage
local count = rel_list:num_items()
*/
static int ReligionListNumItems(lua_State* L) {
    auto* list = get_religion_list(L);
    l_pushinteger(L, list ? list->count : 0);
    return 1;
}

/***
Retrieves the religion object (@{RELIGION_SCRIPT_INTERFACE}) at a zero-based index (`0` to `num_items() - 1`).
@function item_at
@tparam integer index zero-based list index
@treturn RELIGION_SCRIPT_INTERFACE|nil religion object, or nil when out of bounds
@usage
for i = 0, rel_list:num_items() - 1 do
    local rel = rel_list:item_at(i)
    local key = rel:GetKey()
    local pct = rel:GetProportion() * 100
end
*/
static int ReligionListItemAt(lua_State* L) {
    auto* list = get_religion_list(L);
    if (!list || l_type(L, 2) != LUA_TNUMBER) {
        l_pushnil(L);
        return 1;
    }
    lua_Integer index = l_tointeger(L, 2);
    if (index < 0 || index >= list->count) {
        l_pushnil(L);
        return 1;
    }
    push_religion(L, list->items[index].m_religion, list->items[index].m_proportion);
    return 1;
}

/***
Checks whether this religion list is empty.
@function is_empty
@treturn boolean true when the list has no religions, false otherwise
@usage
if not rel_list:is_empty() then
    local dominant_rel = rel_list:item_at(0)
end
*/
static int ReligionListIsEmpty(lua_State* L) {
    auto* list = get_religion_list(L);
    l_pushboolean(L, !list || list->count == 0);
    return 1;
}

static const luaL_Reg religion_list_methods[] = {
    {"num_items", ReligionListNumItems},
    {"item_at",   ReligionListItemAt},
    {"is_empty",  ReligionListIsEmpty},
    {nullptr, nullptr}
};

void register_religion_list_methods(lua_State* L) {
    l_newmetatable(L, kReligionListMetatable);
    l_createtable(L, 0, 3);
    for (const luaL_Reg* f = religion_list_methods; f->name; ++f) {
        l_pushstring(L, f->name);
        l_pushcclosure(L, f->func, 0);
        l_settable(L, -3);
    }
    l_setfield(L, -2, "__index");
    l_pop(L, 1);
    Log("[twdll] RELIGION_LIST_SCRIPT_INTERFACE registered");
}

void push_religion_list(lua_State* L,
                        const TW_ReligionProportion* items,
                        int count) {
    if (count < 0) count = 0;
    const size_t base_size = offsetof(ReligionListUserdata, items);
    const size_t item_size = sizeof(TW_ReligionProportion);
    const size_t size = base_size + static_cast<size_t>(count) * item_size;
    auto* list = static_cast<ReligionListUserdata*>(l_newuserdata(L, size));
    list->count = count;
    for (int i = 0; i < count; ++i) {
        list->items[i] = items[i];
    }
    l_newmetatable(L, kReligionListMetatable);
    l_setmetatable(L, -2);
}
