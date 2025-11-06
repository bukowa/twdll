#pragma once

#include "log.h"
#include "lua/lua_api.h"

// --- Generic Memory Access (Templates stay in header) ---

template<typename T>
T read_from(void *base_address, const size_t offset) {
    return *reinterpret_cast<T *>(static_cast<char *>(base_address) + offset);
}


template<typename T>
void write_to(void *base_address, const size_t offset, T value) {
    *reinterpret_cast<T *>(static_cast<char *>(base_address) + offset) = value;
}


// --- Generic Lua Scripting Helpers (Declarations and Templates) ---

// Declaration for the function implemented in the .cpp file
void *get_object_from_indirect_wrapper(lua_State *L, const char *object_type_name, size_t pointer_offset);

// Template implementations remain in the header
template<typename T, typename U>
int read_property_lua(lua_State *L, size_t pointer_offset, size_t property_offset, const char *object_type_name,
                      void (*push_function)(lua_State *, U)) {
    void *object = get_object_from_indirect_wrapper(L, object_type_name, pointer_offset);
    if (!object) {
        l_pushnil(L);
        return 1;
    }
    T current_value = read_from<T>(object, property_offset);
    push_function(L, static_cast<U>(current_value));
    return 1;
}


template<typename T, typename U>
int write_property_lua(lua_State *L, size_t pointer_offset, size_t property_offset, const char *object_type_name,
                       U (*to_function)(lua_State *, int)) {
    void *object = get_object_from_indirect_wrapper(L, object_type_name, pointer_offset);
    if (!object) {
        return 0; // Error already logged by helper
    }
    T new_value = static_cast<T>(to_function(L, 2));
    write_to<T>(object, property_offset, new_value);
    return 0;
}


template<typename T, typename U>
int read_nested_property_lua(lua_State *L, size_t pointer_offset, size_t nested_obj_ptr_offset,
                             size_t final_property_offset, const char *object_type_name,
                             void (*push_function)(lua_State *, U)) {
    void *base_object = get_object_from_indirect_wrapper(L, object_type_name, pointer_offset);
    if (!base_object) {
        l_pushnil(L);
        return 1;
    }

    void *nested_obj = read_from<void *>(base_object, nested_obj_ptr_offset);
    if (!nested_obj) {
        l_pushnil(L); // Nested object pointer is null, return nil.
        return 1;
    }

    T final_value = read_from<T>(nested_obj, final_property_offset);
    push_function(L, static_cast<U>(final_value));
    return 1;
}


// --- Common Lua-facing Implementations (Declarations) ---

int read_int_property(lua_State *L, size_t pointer_offset, size_t property_offset, const char *object_type_name);
int write_int_property(lua_State *L, size_t pointer_offset, size_t property_offset, const char *object_type_name);

int read_float_property(lua_State *L, size_t pointer_offset, size_t property_offset, const char *object_type_name);
int write_float_property(lua_State *L, size_t pointer_offset, size_t property_offset, const char *object_type_name);

int read_nested_int_property(lua_State *L, size_t pointer_offset, size_t nested_obj_ptr_offset,
                             size_t final_property_offset, const char *object_type_name);

int read_nested_float_property(lua_State *L, size_t pointer_offset, size_t nested_obj_ptr_offset,
                               size_t final_property_offset, const char *object_type_name);

int get_memory_address_lua(lua_State *L, const char *object_type_name, size_t pointer_offset);

