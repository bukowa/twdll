#pragma once
// tw.h — twdll internal shared machinery
// Single header included by every game-specific translation unit.
// Provides: memory helpers, Lua object unwrapping, typed property
// accessor templates, and modern Property/Getter classes.

#include "lua_api.h"
#include "log.h"
#include <cstddef>
#include <cstdio>
#include <type_traits>

// ── Memory helpers ────────────────────────────────────────────────────────────
// Raw typed read/write at a byte offset from a base pointer.

template <typename T>
inline T tw_read(void* base, size_t off) {
    return *reinterpret_cast<T*>(static_cast<char*>(base) + off);
}

template <typename T>
inline void tw_write(void* base, size_t off, T v) {
    *reinterpret_cast<T*>(static_cast<char*>(base) + off) = v;
}

// ── Lua object unwrapping ─────────────────────────────────────────────────────
// The game passes script objects as userdata wrapping a pointer-to-pointer.
// Stack slot 1: void** (wrapper)  →  [ptr_off]  →  void* (real object)

inline void* tw_get_obj(lua_State* L, const char* tag, size_t ptr_off) {
    void** p_wrapper = static_cast<void**>(l_touserdata(L, 1));
    if (!p_wrapper || !*p_wrapper) {
        Log("[twdll] tw_get_obj: invalid wrapper for '%s'", tag);
        return nullptr;
    }
    void* real = tw_read<void*>(*p_wrapper, ptr_off);
    if (!real) {
        Log("[twdll] tw_get_obj: null real ptr for '%s'", tag);
        return nullptr;
    }
    return real;
}

// ── Typed field accessors (member-pointer based) ──────────────────────────────
// tw_get / tw_set use C++ member pointers for type-safe field access.
// Supports int/short/char (→ lua_Integer) and float/double (→ lua_Number).

template <typename S, typename F>
inline int tw_get(lua_State* L, size_t ptr_off, F S::* field, const char* tag) {
    void* obj = tw_get_obj(L, tag, ptr_off);
    if (!obj) { l_pushnil(L); return 1; }
    F v = static_cast<S*>(obj)->*field;
    if constexpr (std::is_integral_v<F>)
        l_pushinteger(L, static_cast<lua_Integer>(v));
    else if constexpr (std::is_floating_point_v<F>)
        l_pushnumber(L, static_cast<double>(v));
    return 1;
}

template <typename S, typename F>
inline int tw_set(lua_State* L, size_t ptr_off, F S::* field, const char* tag) {
    void* obj = tw_get_obj(L, tag, ptr_off);
    if (!obj) return 0;
    if constexpr (std::is_integral_v<F>)
        static_cast<S*>(obj)->*field = static_cast<F>(l_tointeger(L, 2));
    else if constexpr (std::is_floating_point_v<F>)
        static_cast<S*>(obj)->*field = static_cast<F>(l_tonumber(L, 2));
    return 0;
}

// Nested variant: obj → ptr_field → nested struct → field
template <typename S, typename N, typename F>
inline int tw_get_nested(lua_State* L, size_t ptr_off, N* S::* nested_ptr,
                         F N::* field, const char* tag) {
    void* obj = tw_get_obj(L, tag, ptr_off);
    if (!obj) { l_pushnil(L); return 1; }
    N* nested = static_cast<S*>(obj)->*nested_ptr;
    if (!nested) { l_pushnil(L); return 1; }
    F v = nested->*field;
    if constexpr (std::is_integral_v<F>)
        l_pushinteger(L, static_cast<lua_Integer>(v));
    else if constexpr (std::is_floating_point_v<F>)
        l_pushnumber(L, static_cast<double>(v));
    return 1;
}

// ── Raw offset helpers (for diagnostic / exploratory use) ────────────────────

inline int tw_get_int_at(lua_State* L, const char* tag, size_t ptr_off) {
    void* obj = tw_get_obj(L, tag, ptr_off);
    if (!obj) { l_pushnil(L); return 1; }
    size_t offset = static_cast<size_t>(l_tointeger(L, 2));
    l_pushinteger(L, static_cast<lua_Integer>(tw_read<int>(obj, offset)));
    return 1;
}

inline int tw_set_int_at(lua_State* L, const char* tag, size_t ptr_off) {
    void* obj = tw_get_obj(L, tag, ptr_off);
    if (!obj) return 0;
    size_t offset = static_cast<size_t>(l_tointeger(L, 2));
    int    value  = static_cast<int>(l_tointeger(L, 3));
    tw_write<int>(obj, offset, value);
    return 0;
}

inline int tw_mem_address(lua_State* L, const char* tag, size_t ptr_off) {
    void* obj = tw_get_obj(L, tag, ptr_off);
    if (!obj) { l_pushnil(L); return 1; }
    char buf[20];
    snprintf(buf, sizeof(buf), "0x%08zX", reinterpret_cast<size_t>(obj));
    l_pushstring(L, buf);
    return 1;
}

// ── Property class ───────────────────────────────────────────────────────
// Modern property accessor using templates instead of macros.

namespace twdll {

template <typename T, typename S>
class Property {
public:
    Property(T S::* field, size_t ptr_off, const char* tag)
        : field_(field), ptr_off_(ptr_off), tag_(tag) {}
    int get(lua_State* L) { return tw_get(L, ptr_off_, field_, tag_); }
    int set(lua_State* L) { return tw_set(L, ptr_off_, field_, tag_); }
private:
    T S::* field_;
    size_t ptr_off_;
    const char* tag_;
};

// ── Getter class ───────────────────────────────────────────────────────
// For read-only properties.

template <typename T, typename S>
class Getter {
public:
    Getter(T S::* field, size_t ptr_off, const char* tag)
        : field_(field), ptr_off_(ptr_off), tag_(tag) {}
    int get(lua_State* L) { return tw_get(L, ptr_off_, field_, tag_); }
private:
    T S::* field_;
    size_t ptr_off_;
    const char* tag_;
};

// ── Nested property getter ────────────────────────────────────────────────────
// For accessing properties within nested structures.

template <typename T, typename S, typename N>
class NestedProperty {
public:
    NestedProperty(T N::* field, N* S::* nested_ptr, size_t ptr_off, const char* tag)
        : field_(field), nested_ptr_(nested_ptr), ptr_off_(ptr_off), tag_(tag) {}
    int get(lua_State* L) { return tw_get_nested(L, ptr_off_, nested_ptr_, field_, tag_); }
private:
    T N::* field_;
    N* S::* nested_ptr_;
    size_t ptr_off_;
    const char* tag_;
};

} // namespace twdll
