#pragma once
// campaign_hooks.h — Dynamic hooking for campaign singletons.
// Utility layer (game-agnostic).

#include <cstdint>
#include <cstddef>

// ── Hook orchestration ────────────────────────────────────────────────────────
// Called from DllMain on DLL_PROCESS_ATTACH.
// Implemented in game-specific translations units (e.g. attila/campaign_hooks.cpp).
void install_campaign_hooks();

// ── Hook utility ──────────────────────────────────────────────────────────────
// Scans the module, locates the constructor via anchor, and installs MinHook.
bool install_singleton_hook(uintptr_t base, size_t size,
                            const char* anchor, const char* label,
                            void* hook_fn, void** orig_fn);
