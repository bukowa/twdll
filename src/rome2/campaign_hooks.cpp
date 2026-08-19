#include "../common/campaign_hooks.h"
#include "../common/log.h"
#include "../common/signature_scanner.h"
#include <windows.h>
#include <psapi.h>
#include <cstdint>
#include <vector>

static const char* TARGET_MODULE = "empire.retail.dll";

// Test default: 0.02f
static float g_custom_spacing = 0.02f;
static float g_custom_radius  = 0.010f; // g_custom_spacing / 2.0f

static std::vector<uintptr_t> g_patched_addrs;
static uintptr_t g_patched_radius_fn = 0;
static uint8_t   g_orig_radius_bytes[7] = {};

float set_transport_spacing(float new_spacing) {
    if (new_spacing <= 0.0f) new_spacing = 0.01f;
    g_custom_spacing = new_spacing;
    g_custom_radius  = new_spacing / 2.0f;

    for (uintptr_t addr : g_patched_addrs) {
        DWORD old_protect = 0;
        if (VirtualProtect(reinterpret_cast<LPVOID>(addr), sizeof(float), PAGE_EXECUTE_READWRITE, &old_protect)) {
            *reinterpret_cast<float*>(addr) = g_custom_spacing;
            VirtualProtect(reinterpret_cast<LPVOID>(addr), sizeof(float), old_protect, &old_protect);
        }
    }

    Log("[twdll] Realtime updated transport spacing to %.4ff (sites=%zu)", g_custom_spacing, g_patched_addrs.size());
    return g_custom_spacing;
}

float get_transport_spacing() {
    return g_custom_spacing;
}

void install_campaign_hooks() {
    Log("[twdll] install_campaign_hooks: starting for %s", TARGET_MODULE);

    HMODULE hMod = GetModuleHandleA(TARGET_MODULE);
    if (!hMod) {
        Log("[twdll] install_campaign_hooks: %s not loaded yet — skipping", TARGET_MODULE);
        return;
    }

    MODULEINFO mi = {};
    if (!GetModuleInformation(GetCurrentProcess(), hMod, &mi, sizeof(mi))) {
        Log("[twdll] install_campaign_hooks: GetModuleInformation failed (%lu)", GetLastError());
        return;
    }

    uintptr_t base = reinterpret_cast<uintptr_t>(hMod);
    size_t size = mi.SizeOfImage;

    // ── Patch 1: SHIP_TECH::create_entity_formation (prototype geometry templates) ─
    uintptr_t marines_str = Scanner::FindString(base, size, "marines");
    if (marines_str) {
        uintptr_t push_ref = Scanner::FindPushRef(base, size, marines_str);
        if (push_ref) {
            const uint8_t* scan_start = reinterpret_cast<const uint8_t*>(push_ref);
            size_t scan_limit = 0x800;
            int count = 0;
            for (size_t i = 0; i + 4 <= scan_limit; ++i) {
                if (*reinterpret_cast<const uint32_t*>(&scan_start[i]) == 0x3F333333) { // 0.70f
                    uintptr_t float_addr = reinterpret_cast<uintptr_t>(&scan_start[i]);
                    DWORD old_protect = 0;
                    if (VirtualProtect(reinterpret_cast<LPVOID>(float_addr), sizeof(float), PAGE_EXECUTE_READWRITE, &old_protect)) {
                        *reinterpret_cast<float*>(float_addr) = g_custom_spacing;
                        VirtualProtect(reinterpret_cast<LPVOID>(float_addr), sizeof(float), old_protect, &old_protect);
                        g_patched_addrs.push_back(float_addr);
                        count++;
                        Log("[twdll] [Patch 1] SHIP_TECH spacing at 0x%08X set to %.4ff", float_addr, g_custom_spacing);
                    }
                }
            }
            Log("[twdll] [Patch 1] SHIP_TECH patched %d formation sites", count);
        } else {
            Log("[twdll] [Patch 1] WARNING: Could not find push reference to 'marines'");
        }
    } else {
        Log("[twdll] [Patch 1] WARNING: Could not find string 'marines'");
    }

    // ── Patch 2: BATTLE_ENTITIES_TABLE::largest_infantry_radius (live battle ship instances) ─
    uintptr_t radius_fn = Scanner::find_signature(base, size, "D9 41 58 C3");
    if (radius_fn) {
        g_patched_radius_fn = radius_fn;
        memcpy(g_orig_radius_bytes, reinterpret_cast<void*>(radius_fn), 7);
        DWORD old_protect = 0;
        if (VirtualProtect(reinterpret_cast<LPVOID>(radius_fn), 7, PAGE_EXECUTE_READWRITE, &old_protect)) {
            uint8_t patch[7] = { 0xD9, 0x05, 0, 0, 0, 0, 0xC3 };
            *reinterpret_cast<uintptr_t*>(&patch[2]) = reinterpret_cast<uintptr_t>(&g_custom_radius);
            memcpy(reinterpret_cast<void*>(radius_fn), patch, 7);
            VirtualProtect(reinterpret_cast<LPVOID>(radius_fn), 7, old_protect, &old_protect);
            Log("[twdll] [Patch 2] largest_infantry_radius at 0x%08X set to return %.4ff", radius_fn, g_custom_radius);
        } else {
            Log("[twdll] [Patch 2] ERROR: VirtualProtect failed on largest_infantry_radius (%lu)", GetLastError());
        }
    } else {
        Log("[twdll] [Patch 2] WARNING: Could not find largest_infantry_radius signature");
    }

    Log("[twdll] install_campaign_hooks: done");
}

void uninstall_campaign_hooks() {
    for (uintptr_t addr : g_patched_addrs) {
        DWORD old_protect = 0;
        if (VirtualProtect(reinterpret_cast<LPVOID>(addr), sizeof(float), PAGE_EXECUTE_READWRITE, &old_protect)) {
            *reinterpret_cast<float*>(addr) = 0.70f;
            VirtualProtect(reinterpret_cast<LPVOID>(addr), sizeof(float), old_protect, &old_protect);
        }
    }
    g_patched_addrs.clear();

    if (g_patched_radius_fn) {
        DWORD old_protect = 0;
        if (VirtualProtect(reinterpret_cast<LPVOID>(g_patched_radius_fn), 7, PAGE_EXECUTE_READWRITE, &old_protect)) {
            memcpy(reinterpret_cast<void*>(g_patched_radius_fn), g_orig_radius_bytes, 7);
            VirtualProtect(reinterpret_cast<LPVOID>(g_patched_radius_fn), 7, old_protect, &old_protect);
        }
        g_patched_radius_fn = 0;
    }
}
