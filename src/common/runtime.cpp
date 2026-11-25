#include "runtime.h"
#include "log.h"

#include <cstdio>
#include <cstring>
#include <psapi.h>

#ifndef TWDLL_BUILD_SHA
#define TWDLL_BUILD_SHA "unknown"
#endif

namespace {

HMODULE g_self = nullptr;
bool    g_pinned = false;

// 0 = not decided yet, 1 = this module owns the runtime, -1 = refused.
int         g_owner_state = 0;
std::string g_refusal_reason;
HANDLE      g_owner_mapping = nullptr;   // kept open for the process lifetime

constexpr uint32_t kOwnerMagic = 0x4F445754; // 'TWDO'

#pragma pack(push, 1)
struct OwnerRecord {
    uint32_t magic;
    uint32_t record_size;
    uint32_t api_version;
    uint32_t module_base;
    char     name_space[32];
    char     build_sha[48];
    char     module_path[MAX_PATH];
};
#pragma pack(pop)

std::string wide_to_utf8(const wchar_t* w) {
    if (!w || !*w) return {};
    int n = WideCharToMultiByte(CP_UTF8, 0, w, -1, nullptr, 0, nullptr, nullptr);
    if (n <= 1) return {};
    std::string s(static_cast<size_t>(n - 1), '\0');
    WideCharToMultiByte(CP_UTF8, 0, w, -1, &s[0], n, nullptr, nullptr);
    return s;
}

std::string module_path_utf8(HMODULE m) {
    wchar_t buf[MAX_PATH] = {};
    DWORD n = GetModuleFileNameW(m, buf, MAX_PATH);
    if (n == 0 || n >= MAX_PATH) return {};
    return wide_to_utf8(buf);
}

void copy_str(char* dst, size_t cap, const std::string& src) {
    size_t n = src.size() < cap - 1 ? src.size() : cap - 1;
    memcpy(dst, src.data(), n);
    dst[n] = '\0';
}

// Any other loaded module exporting luaopen_twdll is a twdll build from another
// mod (or an older build without the owner record). Two MinHook instances on the
// same engine functions corrupt each other on uninstall, so we refuse to start.
bool find_foreign_twdll(std::string& path_out) {
    HMODULE mods[1024];
    DWORD needed = 0;
    if (!EnumProcessModules(GetCurrentProcess(), mods, sizeof(mods), &needed)) return false;
    size_t count = needed / sizeof(HMODULE);
    if (count > 1024) count = 1024;
    for (size_t i = 0; i < count; ++i) {
        if (mods[i] == g_self) continue;
        if (GetProcAddress(mods[i], "luaopen_twdll")) {
            path_out = module_path_utf8(mods[i]);
            return true;
        }
    }
    return false;
}

} // namespace

void runtime_set_module(HMODULE self) { g_self = self; }
HMODULE runtime_module() { return g_self; }

std::string runtime_module_path() { return module_path_utf8(g_self); }

void runtime_pin_module() {
    if (g_pinned) return;
    HMODULE pinned = nullptr;
    if (GetModuleHandleExW(GET_MODULE_HANDLE_EX_FLAG_PIN | GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS,
                           reinterpret_cast<LPCWSTR>(&runtime_pin_module), &pinned)) {
        g_pinned = true;
        Log("[twdll] runtime: module pinned for the process lifetime");
    } else {
        Log("[twdll] runtime: GetModuleHandleExW(PIN) failed (%lu)", GetLastError());
    }
}

bool runtime_claim_owner(std::string& reason) {
    if (g_owner_state == 1) return true;
    if (g_owner_state == -1) { reason = g_refusal_reason; return false; }

    wchar_t name[64];
    swprintf(name, 64, L"Local\\twdll.owner.%lu", GetCurrentProcessId());

    HANDLE h = CreateFileMappingW(INVALID_HANDLE_VALUE, nullptr, PAGE_READWRITE, 0,
                                  sizeof(OwnerRecord), name);
    if (!h) {
        // Cannot create the record (should not happen). Fall back to the module scan only.
        Log("[twdll] runtime: CreateFileMappingW failed (%lu); owner record unavailable", GetLastError());
    }
    const bool existed = h && GetLastError() == ERROR_ALREADY_EXISTS;

    OwnerRecord* rec = nullptr;
    if (h) {
        rec = static_cast<OwnerRecord*>(MapViewOfFile(h, FILE_MAP_ALL_ACCESS, 0, 0, sizeof(OwnerRecord)));
    }

    if (existed && rec && rec->magic == kOwnerMagic &&
        rec->module_base != static_cast<uint32_t>(reinterpret_cast<uintptr_t>(g_self))) {
        char buf[512];
        snprintf(buf, sizeof(buf),
                 "twdll runtime already active in this process: namespace '%.31s', build %.12s, api %u, module %.260s",
                 rec->name_space, rec->build_sha, rec->api_version, rec->module_path);
        g_refusal_reason = buf;
        UnmapViewOfFile(rec);
        CloseHandle(h);
        g_owner_state = -1;
        reason = g_refusal_reason;
        Log("[twdll] runtime: refusing to start — %s", g_refusal_reason.c_str());
        return false;
    }

    std::string foreign;
    if (find_foreign_twdll(foreign)) {
        g_refusal_reason = "another twdll module is already loaded in this process: " + foreign;
        if (rec) UnmapViewOfFile(rec);
        if (h) CloseHandle(h);
        g_owner_state = -1;
        reason = g_refusal_reason;
        Log("[twdll] runtime: refusing to start — %s", g_refusal_reason.c_str());
        return false;
    }

    if (rec) {
        memset(rec, 0, sizeof(OwnerRecord));
        rec->record_size = sizeof(OwnerRecord);
        rec->api_version = TWDLL_API_VERSION;
        rec->module_base = static_cast<uint32_t>(reinterpret_cast<uintptr_t>(g_self));
        copy_str(rec->name_space, sizeof(rec->name_space), TWDLL_NAMESPACE);
        copy_str(rec->build_sha, sizeof(rec->build_sha), TWDLL_BUILD_SHA);
        copy_str(rec->module_path, sizeof(rec->module_path), runtime_module_path());
        rec->magic = kOwnerMagic;
        UnmapViewOfFile(rec);
    }
    g_owner_mapping = h; // intentionally never closed: ownership lasts until process exit
    g_owner_state = 1;
    Log("[twdll] runtime: owner record claimed (namespace '%s', api %d)", TWDLL_NAMESPACE, TWDLL_API_VERSION);
    return true;
}
