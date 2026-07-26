#pragma once
// tw_types.h — Engine object layouts and Lua unwrap helpers (Attila 32-bit).
// Do NOT share with rome2/ — offsets differ between games.
// For Rome 2, copy to rome2/tw_types.h and adjust offsets.

#include <cstdint>
#include <cstddef>
#include "../common/lua_api.h"

namespace twdll {

struct TW_FamilyMember;

#pragma pack(push, 1)

struct TW_Faction {
    char pad_00[0x7DC];
    int  gold;              // 0x7DC
};

struct TW_Character {
    char pad_00[0x14];
    int  movement_points;        // 0x14
    char pad_18[0x1F0];          // 0x18..0x207
    TW_FamilyMember* family_member; // 0x208
    char pad_20C[0x34C];         // 0x20C..0x557
    int  ambition;               // 0x558
    int  gravitas;               // 0x55C
};

#pragma pack(pop)

#define TW_ASSERT_OFFSET(S, F, O) \
    static_assert(offsetof(S, F) == O, #S " Attila: " #F " expected at " #O)

TW_ASSERT_OFFSET(TW_Faction,   gold,            0x7DC);
TW_ASSERT_OFFSET(TW_Character, movement_points, 0x14);
TW_ASSERT_OFFSET(TW_Character, family_member,   0x208);
TW_ASSERT_OFFSET(TW_Character, ambition,        0x558);
TW_ASSERT_OFFSET(TW_Character, gravitas,        0x55C);

// Mirrors the memory layout of any *_SCRIPT_INTERFACE object in the game.
template<typename T>
struct GameScriptInterface {
    void*    vtable;           // 0x0
    uint32_t context_or_pad;   // 0x4
    T*       m_wrapped_object; // 0x8
};

static_assert(offsetof(GameScriptInterface<TW_Faction>,   m_wrapped_object) == 0x8);
static_assert(offsetof(GameScriptInterface<TW_Character>, m_wrapped_object) == 0x8);

// Default ptr offset is 0x8. Use TW_PTR_OFFSET(T, O) in foo.cpp for exceptions.
template<typename T> struct TW_PtrOffset { static constexpr size_t value = 0x8; };

#define TW_PTR_OFFSET(T, O) \
    template<> struct TW_PtrOffset<T> { static constexpr size_t value = O; }

// Deref twice: touserdata() → *ud (SCRIPT_INTERFACE*) → +TW_PtrOffset<T> → real ptr.
template<typename T>
inline T* tw_unwrap(lua_State* L, int slot) {
    void** ud = static_cast<void**>(l_touserdata(L, slot));
    if (!ud || !*ud) return nullptr;
    return *reinterpret_cast<T**>(static_cast<char*>(*ud) + TW_PtrOffset<T>::value);
}

template<typename T>
inline void tw_push_wrapped(lua_State* L, T* raw_ptr) {
    auto* ud = static_cast<GameScriptInterface<T>*>(
        l_newuserdata(L, sizeof(GameScriptInterface<T>)));
    ud->vtable           = nullptr;
    ud->context_or_pad   = 0;
    ud->m_wrapped_object = raw_ptr;
}

} // namespace twdll
