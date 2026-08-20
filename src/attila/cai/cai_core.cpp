/// @module twdll.cai
/// Campaign AI decision logging and telemetry extensions.
#include "cai_common.h"

bool g_cai_logging_enabled = true;

const char* get_cai_faction_key(void* faction_cai) {
    if (!faction_cai) return "null_faction";
    __try {
        char* fc = reinterpret_cast<char*>(faction_cai);
        char* faction = *reinterpret_cast<char**>(fc + 0xEC);
        if (faction) {
            char* record = *reinterpret_cast<char**>(faction + 0x800);
            if (record) {
                // CA::String in 32-bit Warscape: [+0x0]=length, [+0x4]=capacity, [+0x8]=const char* ptr
                const char* key = *reinterpret_cast<const char**>(record + 0x8);
                if (key && key[0] != '\0') return key;
            }
        }
    } __except (EXCEPTION_EXECUTE_HANDLER) {}
    return "unknown_faction";
}

const char* get_cai_settlement_key(void* settlement_cai) {
    if (!settlement_cai) return "null_settlement";
    __try {
        char* sc = reinterpret_cast<char*>(settlement_cai);
        char* cai_region = *reinterpret_cast<char**>(sc + 0x10);
        if (cai_region) {
            const char* key = *reinterpret_cast<const char**>(cai_region + 0x114);
            if (key && key[0] != '\0') return key;
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

static bool get_boolean_arg(lua_State* L, int idx, bool default_val) {
    if (l_type(L, idx) == LUA_TBOOLEAN) {
        auto* pState = reinterpret_cast<uintptr_t*>(L);
        auto* pVal = reinterpret_cast<uint32_t*>(pState[3] + 8 * (idx - 1));
        return pVal[0] != 0;
    }
    return default_val;
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
    if (l_type(L, 1) == LUA_TBOOLEAN) {
        g_cai_logging_enabled = get_boolean_arg(L, 1, g_cai_logging_enabled);
    }
    l_pushboolean(L, g_cai_logging_enabled ? 1 : 0);
    return 1;
}

extern const luaL_Reg cai_functions_export[] = {
    {"EnableLogging", EnableLogging},
    {nullptr, nullptr}
};
