#pragma once
#include "../../common/tw.h"
#include "../../common/log.h"
#include "../tw_types.h"
#include "../game_api.h"
#include <MinHook.h>
#include <windows.h>

// Global CAI logging flags
extern bool g_cai_logging_enabled;

// String & pointer resolution helpers
const char* get_cai_faction_key(void* faction_cai);
const char* get_cai_settlement_key(void* settlement_cai);

// Subsystem install/uninstall prototypes
void install_cai_occupation_hook(uintptr_t base, size_t size);
void uninstall_cai_occupation_hook();
