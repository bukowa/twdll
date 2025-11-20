// hook_manager.cpp
#include "hook_manager.h"
#include "hooks.h"
#include "MinHook.h"
#include "asmlib_hooks.h"
#include "log.h"

static const Hook all_hooks[] = {
    {"_memmove", "empire.retail.dll", false, 0x116BD8D0, (void*)&CustomMemmove, nullptr},
    {"_memmove_0", "empire.retail.dll", false, 0x116BDE50, (void*)&CustomMemmove, nullptr},
    {"_memcpy_s", "empire.retail.dll", false, 0x1168ECA0, (void*)&CustomMemcpy_s, nullptr},
    {"_memset", "empire.retail.dll", false, 0x116BE510, (void*)&CustomMemset, nullptr},
    {"_memcmp", "empire.retail.dll", false, 0x116BF7D0, (void*)&CustomMemcmp, nullptr},
};

void InstallHooksForModule(const char* moduleName, HMODULE moduleHandle) {
    const char* effectiveModuleName = moduleName ? moduleName : "main executable";
    spdlog::info("Installing hooks for module: {}", effectiveModuleName);

    constexpr uintptr_t idaImageBase = 0x10000000;
    uintptr_t moduleBase = (uintptr_t)moduleHandle;

    for (const auto& hook : all_hooks) {
        bool should_hook_on_attach = (moduleName == nullptr && hook.is_immediate);
        bool module_name_matches = (moduleName != nullptr && _stricmp(moduleName, hook.module_name) == 0);

        if (should_hook_on_attach || module_name_matches) {
            if (hook.ida_address == 0) {
                continue;
            }
            void* pTarget = (void*)(moduleBase + (hook.ida_address - idaImageBase));
            
            if (MH_CreateHook(pTarget, hook.replacement_function, hook.original_function) == MH_OK) {
                spdlog::info("  Hook created for {} at address {:p}", hook.name, pTarget);
            } else {
                spdlog::error("  Failed to create hook for {} at address {:p}", hook.name, pTarget);
            }
        }
    }
}
