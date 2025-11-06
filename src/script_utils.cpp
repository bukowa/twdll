#include "script_utils.h"
#include "log.h"
#include "lua/lua_api.h" // Include game_lua_api.h for function pointers

// --- Generic Lua Scripting Helpers ---

void *get_object_from_indirect_wrapper(lua_State *L, const char *object_type_name,
                                       size_t pointer_offset) {
    void **p_wrapper = static_cast<void **>(l_touserdata(L, 1));
    if (!p_wrapper || !*p_wrapper) {
        spdlog::error("ERROR (get_{}_indirect): Argument is not a valid pointer to a wrapper.", object_type_name);
        return nullptr;
    }
    spdlog::info("p_wrapper: {0:p}", static_cast<const void*>(p_wrapper));
    void *wrapper = *p_wrapper;
    spdlog::info("wrapper: {0:p}", static_cast<const void*>(wrapper));
    void *object = read_from<void *>(wrapper, pointer_offset);
    if (!object) {
        spdlog::error("ERROR (get_{}_indirect): Real {} pointer is null.", object_type_name, object_type_name);
        return nullptr;
    }
    spdlog::info("object: {0:p}", static_cast<const void*>(wrapper));
    return object;
}

// --- Common Lua-facing Implementations ---

int read_int_property(lua_State *L, size_t pointer_offset, size_t property_offset,
                      const char *object_type_name) {
    return read_property_lua<int, lua_Integer>(L, pointer_offset, property_offset, object_type_name,
                                               l_pushinteger);
}

int write_int_property(lua_State *L, size_t pointer_offset, size_t property_offset,
                       const char *object_type_name) {
    return write_property_lua<int, lua_Integer>(L, pointer_offset, property_offset,
                                                object_type_name, l_tointeger);
}

int read_float_property(lua_State *L, size_t pointer_offset, size_t property_offset,
                        const char *object_type_name) {
    return read_property_lua<float, lua_Number>(L, pointer_offset, property_offset,
                                                object_type_name, l_pushnumber);
}

int write_float_property(lua_State *L, size_t pointer_offset, size_t property_offset,
                         const char *object_type_name) {
    return write_property_lua<float, lua_Number>(L, pointer_offset, property_offset,
                                                 object_type_name, l_tonumber);
}

int read_nested_int_property(lua_State *L, size_t pointer_offset, size_t nested_obj_ptr_offset,
                             size_t final_property_offset, const char *object_type_name) {
    return read_nested_property_lua<int, lua_Integer>(L, pointer_offset, nested_obj_ptr_offset,
                                                      final_property_offset, object_type_name,
                                                      l_pushinteger);
}

int read_nested_float_property(lua_State *L, size_t pointer_offset, size_t nested_obj_ptr_offset,
                               size_t final_property_offset, const char *object_type_name) {
    return read_nested_property_lua<float, lua_Number>(L, pointer_offset, nested_obj_ptr_offset,
                                                       final_property_offset, object_type_name,
                                                       l_pushnumber);
}

int get_memory_address_lua(lua_State *L, const char *object_type_name, size_t pointer_offset) {
    void *object = get_object_from_indirect_wrapper(L, object_type_name, pointer_offset);
    if (!object) {
        l_pushnil(L);
        return 1;
    }
    char address_buffer[64];
    sprintf_s(address_buffer, sizeof(address_buffer), "0x%p", object);
    l_pushstring(L, address_buffer);
    return 1;
}
