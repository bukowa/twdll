#pragma once
// tw.h — twdll internal shared machinery
// Single header included by every game-specific translation unit.
// Provides: memory helpers, Lua object unwrapping, typed property
// accessor templates, and modern Property/Getter classes.

#include "lua_api.h"
#include "log.h"
#include <cstddef>
#include <cstdio>
#include <string>
#include <type_traits>
#include <windows.h>

// ── String conversion helpers ────────────────────────────────────────────────
inline std::string tw_wide_to_utf8(const wchar_t* wstr, size_t len) {
    if (!wstr || len == 0) return "";
    int needed = WideCharToMultiByte(CP_UTF8, 0, wstr, static_cast<int>(len), nullptr, 0, nullptr, nullptr);
    if (needed <= 0) return "";
    std::string s(needed, '\0');
    WideCharToMultiByte(CP_UTF8, 0, wstr, static_cast<int>(len), &s[0], needed, nullptr, nullptr);
    return s;
}

inline std::wstring tw_utf8_to_wide(const char* text, size_t len) {
    if (!text || len == 0) return L"";
    int needed = MultiByteToWideChar(CP_UTF8, 0, text, static_cast<int>(len), nullptr, 0);
    if (needed <= 0) return L"";
    std::wstring ws(needed, L'\0');
    MultiByteToWideChar(CP_UTF8, 0, text, static_cast<int>(len), &ws[0], needed);
    return ws;
}

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

namespace twdll {

template <size_t MaxN = 4>
struct OffsetChain {
    size_t offsets[MaxN]{};
    size_t count = 0;

    constexpr OffsetChain() = default;
    constexpr OffsetChain(std::initializer_list<size_t> list) {
        for (size_t off : list) {
            if (count < MaxN) offsets[count++] = off;
        }
    }

    void* apply(void* base) const {
        char* ptr = static_cast<char*>(base);
        for (size_t i = 0; i < count; ++i) {
            if (!ptr) return nullptr;
            ptr += offsets[i];
        }
        return ptr;
    }
};

} // namespace twdll

// ── Lua object unwrapping ─────────────────────────────────────────────────────
// The game passes script objects as userdata wrapping a pointer-to-pointer.
// Stack slot 1: void** (wrapper)  →  [ptr_off]  →  void* (real object)

inline void* tw_get_obj(lua_State* L, const char* tag, size_t ptr_off,
                        const twdll::OffsetChain<4>& chain = {}) {
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
    return chain.apply(real);
}

// ── Typed field accessors (member-pointer based) ──────────────────────────────
// tw_get / tw_set use C++ member pointers for type-safe field access.
// Supports int/short/char (→ lua_Integer) and float/double
// (→ lua_pushnumber / lua_tonumber).

template <typename S, typename F>
inline int tw_get(lua_State* L, size_t ptr_off, F S::* field, const char* tag,
                  const twdll::OffsetChain<4>& chain = {}) {
    void* obj = tw_get_obj(L, tag, ptr_off, chain);
    if (!obj) { l_pushnil(L); return 1; }
    F v = static_cast<S*>(obj)->*field;
    if constexpr (std::is_integral_v<F>)
        l_pushinteger(L, static_cast<lua_Integer>(v));
    else if constexpr (std::is_floating_point_v<F>)
        l_pushnumber(L, static_cast<float>(v));
    return 1;
}

template <typename S, typename F>
inline int tw_set(lua_State* L, size_t ptr_off, F S::* field, const char* tag,
                  const twdll::OffsetChain<4>& chain = {}) {
    void* obj = tw_get_obj(L, tag, ptr_off, chain);
    if (!obj) return 0;
    if constexpr (std::is_integral_v<F>)
        static_cast<S*>(obj)->*field = static_cast<F>(l_tointeger(L, 2));
    else if constexpr (std::is_floating_point_v<F>)
        static_cast<S*>(obj)->*field = static_cast<F>(l_tonumber(L, 2));
    return 0;
}



// Global variant: reads from a raw global typed pointer instead of Lua userdata.
template <typename S, typename F>
inline int tw_get_global(lua_State* L, S** global_ptr, F S::* field) {
    if (!global_ptr || !*global_ptr) { l_pushnil(L); return 1; }
    void* obj = *global_ptr;
    F v = static_cast<S*>(obj)->*field;
    if constexpr (std::is_integral_v<F>)
        l_pushinteger(L, static_cast<lua_Integer>(v));
    else if constexpr (std::is_floating_point_v<F>)
        l_pushnumber(L, static_cast<float>(v));
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
        : field_(field), ptr_off_(ptr_off), tag_(tag), chain_{} {}

    Property(T S::* field, size_t ptr_off, const char* tag, std::initializer_list<size_t> chain)
        : field_(field), ptr_off_(ptr_off), tag_(tag), chain_(chain) {}

    int get(lua_State* L) { return tw_get(L, ptr_off_, field_, tag_, chain_); }
    int set(lua_State* L) { return tw_set(L, ptr_off_, field_, tag_, chain_); }
private:
    T S::*         field_;
    size_t         ptr_off_;
    const char*    tag_;
    OffsetChain<4> chain_;
};

// ── Getter class ───────────────────────────────────────────────────────
// For read-only properties.

template <typename T, typename S>
class Getter {
public:
    Getter(T S::* field, size_t ptr_off, const char* tag)
        : field_(field), ptr_off_(ptr_off), tag_(tag), chain_{} {}

    Getter(T S::* field, size_t ptr_off, const char* tag, std::initializer_list<size_t> chain)
        : field_(field), ptr_off_(ptr_off), tag_(tag), chain_(chain) {}

    int get(lua_State* L) {
        return tw_get(L, ptr_off_, field_, tag_, chain_);
    }
private:
    T S::*         field_;
    size_t         ptr_off_;
    const char*    tag_;
    OffsetChain<4> chain_;
};

// ── Global Getter class ──────────────────────────────────────────────────
// For read-only properties on global singleton objects.

template <typename T, typename S>
class GlobalGetter {
public:
    GlobalGetter(T S::* field, S** global_ptr)
        : field_(field), global_ptr_(global_ptr) {}
    int get(lua_State* L) { return tw_get_global(L, global_ptr_, field_); }
private:
    T S::* field_;
    S** global_ptr_;
};

} // namespace twdll
