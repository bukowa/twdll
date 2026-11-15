#ifdef _WIN32
#define TW_NOINLINE __declspec(noinline)
#define TW_CDECL __cdecl
#define TW_EXPORT extern "C" __declspec(dllexport)
#else
#define TW_NOINLINE __attribute__((noinline, used, visibility("default")))
#define TW_CDECL
#define TW_EXPORT extern "C" __attribute__((used, visibility("default")))
#endif

namespace warscape {

static const char kTickSignature[] = "CA::CampaignModel::Tick";
static const char kLuaRegister[] = "luaL_register";
static const char kFixtureTag[] = "attila_mini_fixture";

struct UnitController {
    int base_morale;

    constexpr UnitController(int morale = 40) : base_morale(morale) {}

    virtual int Tick(int dt) const {
        return base_morale + dt + static_cast<int>(kTickSignature[0]);
    }

    virtual const char* FactionKey() const {
        return kFixtureTag;
    }
};

struct RomanLegionController : public UnitController {
    int veterancy_bonus;

    constexpr RomanLegionController(int morale = 100, int bonus = 25)
        : UnitController(morale), veterancy_bonus(bonus) {}

    int Tick(int dt) const override {
        return base_morale + veterancy_bonus + (dt * 2) + static_cast<int>(kLuaRegister[0]);
    }

    const char* FactionKey() const override {
        return "rom_rome";
    }
};

static const UnitController g_base_unit(45);
static const RomanLegionController g_roman_legion(120, 30);
static const UnitController* const g_roster[2] = {&g_base_unit, &g_roman_legion};
static volatile int g_active_slot = 1;

TW_NOINLINE int DispatchCampaignTick(const UnitController* ctrl, int dt) {
    if (!ctrl) {
        return 0;
    }
    const char* key = ctrl->FactionKey();
    int hash = key ? static_cast<int>(key[0]) : 0;
    return ctrl->Tick(dt) + hash + static_cast<int>(kTickSignature[4]) + static_cast<int>(kLuaRegister[4]);
}

} // namespace warscape

TW_EXPORT int TW_CDECL tw_campaign_step(int dt) {
    int idx = warscape::g_active_slot & 1;
    return warscape::DispatchCampaignTick(warscape::g_roster[idx], dt);
}

TW_EXPORT const char* TW_CDECL tw_faction_name(void) {
    int idx = warscape::g_active_slot & 1;
    const warscape::UnitController* ctrl = warscape::g_roster[idx];
    return ctrl ? ctrl->FactionKey() : warscape::kFixtureTag;
}

#ifdef _WIN32
extern "C" int __cdecl MiniDllEntry(void* hinst, unsigned int reason, void* reserved) {
    (void)hinst;
    (void)reason;
    (void)reserved;
    return 1;
}
#else
extern "C" __attribute__((used, visibility("default"))) void _start(void) {
    volatile int step_sink = tw_campaign_step(3);
    volatile const char* name_sink = tw_faction_name();
    (void)step_sink;
    (void)name_sink;
}
#endif
