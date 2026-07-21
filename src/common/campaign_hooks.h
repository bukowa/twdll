#pragma once
// campaign_hooks.h — String-anchored dynamic hooking for campaign singletons.
// Reusable across Warscape engine games (Attila, Rome 2).

#include <cstdint>

// ── Global singletons ─────────────────────────────────────────────────────────
// Set to nullptr until the game calls the respective constructor.
extern void* g_world;        // WORLD*
extern void* g_campaign_ui;  // CAMPAIGN_UI*

// ── Initialisation ────────────────────────────────────────────────────────────
// Scans the module, locates the constructors, and installs MinHook hooks.
void install_campaign_hooks();
