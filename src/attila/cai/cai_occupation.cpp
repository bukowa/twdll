#include "cai_common.h"

static const uintptr_t RVA_MAKE_OCCUPATION_DECISION = 0x00D05C80;
static uintptr_t g_make_occupation_decision_addr = 0;

typedef int (__thiscall *fn_make_occupation_decision)(
    void* this_ptr,
    void* faction_cai,
    void* settlement_cai,
    unsigned int* available_decisions_mask,
    unsigned int* allowed_decisions_mask
);
static fn_make_occupation_decision orig_make_occupation_decision = nullptr;

static const char* OCCUPATION_DECISION_NAMES[12] = {
    "LOOT",
    "SACK",
    "RAZE",
    "OCCUPY",
    "LIBERATE",
    "VASSALISE",
    "RAZE_WITHOUT_OCCUPY",
    "COLONISE",
    "DO_NOTHING",
    "RESETTLE",
    "GIFT_TO_ANOTHER_FACTION",
    "NONE"
};

static void format_decision_mask(unsigned int mask, char* out_buf, size_t buf_size) {
    if (mask == 0) {
        snprintf(out_buf, buf_size, "[]");
        return;
    }
    size_t pos = 0;
    out_buf[0] = '[';
    pos = 1;
    bool first = true;
    for (int i = 0; i < 11; ++i) {
        if (mask & (1u << i)) {
            const char* name = OCCUPATION_DECISION_NAMES[i];
            size_t nlen = strlen(name);
            if (pos + nlen + 3 < buf_size) {
                if (!first) {
                    out_buf[pos++] = ',';
                    out_buf[pos++] = ' ';
                }
                memcpy(out_buf + pos, name, nlen);
                pos += nlen;
                first = false;
            }
        }
    }
    if (pos < buf_size - 1) {
        out_buf[pos++] = ']';
        out_buf[pos] = '\0';
    } else {
        out_buf[buf_size - 1] = '\0';
    }
}

static int __fastcall Hooked_make_occupation_decision(
    void* this_ptr,
    void* /*edx*/,
    void* faction_cai,
    void* settlement_cai,
    unsigned int* available_decisions_mask,
    unsigned int* allowed_decisions_mask
) {
    int decision = orig_make_occupation_decision(
        this_ptr,
        faction_cai,
        settlement_cai,
        available_decisions_mask,
        allowed_decisions_mask
    );

    if (g_cai_logging_enabled) {
        const char* dec_name = (decision >= 0 && decision < 12) 
            ? OCCUPATION_DECISION_NAMES[decision] 
            : "UNKNOWN";

        const char* fact_name = get_cai_faction_key(faction_cai);
        const char* sett_name = get_cai_settlement_key(settlement_cai);

        unsigned int allowed = allowed_decisions_mask ? *allowed_decisions_mask : 0;
        unsigned int available = available_decisions_mask ? *available_decisions_mask : 0;
        unsigned int effective = allowed & available;

        char allowed_str[128];
        char avail_str[128];
        char eff_str[128];
        format_decision_mask(allowed, allowed_str, sizeof(allowed_str));
        format_decision_mask(available, avail_str, sizeof(avail_str));
        format_decision_mask(effective, eff_str, sizeof(eff_str));

        int num_regions = 0;
        bool is_major = false;
        __try {
            if (faction_cai) {
                char* fc = reinterpret_cast<char*>(faction_cai);
                num_regions = *reinterpret_cast<int*>(fc + 0xA4);
                char* faction = *reinterpret_cast<char**>(fc + 0xEC);
                if (faction) {
                    is_major = *reinterpret_cast<bool*>(faction + 0x84C);
                }
            }
        } __except (EXCEPTION_EXECUTE_HANDLER) {}

        Log("[twdll][CAI:OCCUPATION] '%s' (regions=%d, major=%s) -> '%s' | Effective: %s | Decision: %s (id=%d) [Allowed: %s | In-Context: %s]",
            fact_name, num_regions, is_major ? "yes" : "no", sett_name, 
            eff_str, dec_name, decision, allowed_str, avail_str);
    }

    return decision;
}

void install_cai_occupation_hook(uintptr_t base, size_t size) {
    (void)size;
    g_make_occupation_decision_addr = base + RVA_MAKE_OCCUPATION_DECISION;

    MH_STATUS mhs = MH_CreateHook(
        reinterpret_cast<void*>(g_make_occupation_decision_addr),
        reinterpret_cast<void*>(Hooked_make_occupation_decision),
        reinterpret_cast<void**>(&orig_make_occupation_decision)
    );
    if (mhs != MH_OK) {
        Log("[twdll] [CAI_OCCUPATION] MH_CreateHook failed (%d)", mhs);
        return;
    }

    mhs = MH_EnableHook(reinterpret_cast<void*>(g_make_occupation_decision_addr));
    if (mhs != MH_OK) {
        Log("[twdll] [CAI_OCCUPATION] MH_EnableHook failed (%d)", mhs);
        return;
    }

    Log("[twdll][CAI:OCCUPATION] Hook installed at 0x%08X (make_occupation_decision)", static_cast<unsigned int>(g_make_occupation_decision_addr));
}

void uninstall_cai_occupation_hook() {
    if (g_make_occupation_decision_addr) {
        MH_DisableHook(reinterpret_cast<void*>(g_make_occupation_decision_addr));
        MH_RemoveHook(reinterpret_cast<void*>(g_make_occupation_decision_addr));
        g_make_occupation_decision_addr = 0;
    }
    orig_make_occupation_decision = nullptr;
}
