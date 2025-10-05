#include "script_utils.h"
#include "log.h"
#include <cstdio>

// --- Generic Lua Scripting Helpers ---

void *get_object_from_indirect_wrapper(lua_State *L, const char *object_type_name, size_t pointer_offset) {
    void **p_wrapper = static_cast<void **>(lua_touserdata(L, 1));
    if (!p_wrapper || !*p_wrapper) {
        char log_buffer[128];
        sprintf_s(log_buffer, sizeof(log_buffer), "ERROR (get_%s_indirect): Argument is not a valid pointer to a wrapper.", object_type_name);
        Log(log_buffer);
        return nullptr;
    }
    void *wrapper = *p_wrapper;
    void *object = read_from<void*>(wrapper, pointer_offset);
    if (!object) {
        char log_buffer[128];
        sprintf_s(log_buffer, sizeof(log_buffer), "ERROR (get_%s_indirect): Real %s pointer is null.", object_type_name, object_type_name);
        Log(log_buffer);
        return nullptr;
    }
    return object;
}

// --- Common Lua-facing Implementations ---

int read_int_property(lua_State *L, size_t pointer_offset, size_t property_offset, const char *object_type_name) {
    return read_property_lua<int, lua_Integer>(L, pointer_offset, property_offset, object_type_name, lua_pushinteger);
}

int write_int_property(lua_State *L, size_t pointer_offset, size_t property_offset, const char *object_type_name) {
    return write_property_lua<int, lua_Integer>(L, pointer_offset, property_offset, object_type_name, lua_tointeger);
}

int read_float_property(lua_State *L, size_t pointer_offset, size_t property_offset, const char *object_type_name) {
    return read_property_lua<float, lua_Number>(L, pointer_offset, property_offset, object_type_name, lua_pushnumber);
}

int write_float_property(lua_State *L, size_t pointer_offset, size_t property_offset, const char *object_type_name) {
    return write_property_lua<float, lua_Number>(L, pointer_offset, property_offset, object_type_name, lua_tonumber);
}

int read_nested_int_property(lua_State* L, size_t pointer_offset, size_t nested_obj_ptr_offset, size_t final_property_offset, const char* object_type_name) {
    return read_nested_property_lua<int, lua_Integer>(L, pointer_offset, nested_obj_ptr_offset, final_property_offset, object_type_name, lua_pushinteger);
}

int read_nested_float_property(lua_State* L, size_t pointer_offset, size_t nested_obj_ptr_offset, size_t final_property_offset, const char* object_type_name) {
    return read_nested_property_lua<float, lua_Number>(L, pointer_offset, nested_obj_ptr_offset, final_property_offset, object_type_name, lua_pushnumber);
}

int get_memory_address_lua(lua_State* L, const char* object_type_name, size_t pointer_offset) {
    void* object = get_object_from_indirect_wrapper(L, object_type_name, pointer_offset);
    if (!object) {
        lua_pushnil(L);
        return 1;
    }
    char address_buffer[64];
    sprintf_s(address_buffer, sizeof(address_buffer), "0x%p", object);
    lua_pushstring(L, address_buffer);
    return 1;
}
