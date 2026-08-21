/// @module twdll.cai
/// Campaign AI decision logging and telemetry extensions.
#include "cai_common.h"

bool g_cai_logging_enabled = true;

const char* get_cai_faction_key(void* faction_cai) {
    if (!faction_cai) return "null_faction";
    __try {
        auto* fc = static_cast<twdll::TW_CaiFaction*>(faction_cai);
        if (fc && fc->m_faction && fc->m_faction->m_faction_record && fc->m_faction->m_faction_record->m_key.m_data) {
            const char* key = fc->m_faction->m_faction_record->m_key.m_data;
            if (key[0] != '\0') return key;
        }
    } __except (EXCEPTION_EXECUTE_HANDLER) {}
    return "unknown_faction";
}

const char* get_cai_settlement_key(void* settlement_cai) {
    if (!settlement_cai) return "null_settlement";
    __try {
        auto* sc = static_cast<twdll::TW_CaiSettlement*>(settlement_cai);
        if (sc && sc->m_cai_region && sc->m_cai_region->m_settlement_key) {
            const char* key = sc->m_cai_region->m_settlement_key;
            if (key[0] != '\0') return key;
        }
    } __except (EXCEPTION_EXECUTE_HANDLER) {}
    return "unknown_settlement";
}

void install_cai_hooks(uintptr_t base, size_t size) {
    Log("[twdll][CAI] ========================================================");
    Log("[twdll][CAI] Campaign AI Telemetry System Initializing...");
    install_cai_occupation_hook(base, size);
    Log("[twdll][CAI] Campaign AI Telemetry System Initialized");
    Log("[twdll][CAI] ========================================================");
}

void uninstall_cai_hooks() {
    uninstall_cai_occupation_hook();
}

/***
Enables or disables real-time Campaign AI decision telemetry output to `twdll.log`.

When enabled, logs the entire evaluating reasoning chain:
- Faction context & persona.
- Environmental drivers and strategic needs (food, squalor, threats).
- Candidate option set with mathematical weights/probabilities.
- Chosen decision outcome or explicit rejection reasons.
@function EnableLogging
@tparam[opt] boolean enabled whether CAI telemetry logging is enabled
@treturn boolean current enabled state
@usage
-- Enable Campaign AI telemetry output:
twdll.cai.EnableLogging(true)

-- Query current telemetry state:
local is_logging = twdll.cai.EnableLogging()
*/
static int EnableLogging(lua_State* L) {
    if (l_type(L, 1) != LUA_TNONE && l_type(L, 1) != LUA_TNIL) {
        g_cai_logging_enabled = l_tobool(L, 1);
    }
    l_pushboolean(L, g_cai_logging_enabled ? 1 : 0);
    return 1;
}

extern const luaL_Reg cai_functions_export[] = {
    {"EnableLogging", EnableLogging},
    {nullptr, nullptr}
};
