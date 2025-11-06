#include "g_campaign.h"
#include "globals/global_address.h"
#include "globals/game_globals.h" // For the actual g_... variables
#include <vector>
#include <memory>

namespace Game {

    // Global storage for our type-safe global addresses.
    static std::vector<std::unique_ptr<IGlobalAddress>> g_campaign_global_addresses;

    // Renders the ImGui widgets for all registered globals.
    void RenderCampaignGlobalsImGui() {
        for (const auto& global : g_campaign_global_addresses) {
            global->RenderImGuiWidget();
        }
    }

    // A clean way to register a new global.
    void RegisterCampaignGlobalAddress(std::unique_ptr<IGlobalAddress> address) {
        g_campaign_global_addresses.push_back(std::move(address));
    }

    // Creates and registers all the global variables.
    // This is now 100% compile-time type-safe.
    void InitializeCampaignGlobalAddresses() {
#ifdef STEAM_BUILD
        RegisterCampaignGlobalAddress(make_global_address(
            "Max Units in Army",
            "Maximum number of units allowed in an army.",
            std::make_unique<OffsetAddressResolver>("Empire.Retail.dll", 0x193FC68),
            &g_max_units_in_army, 1, 9999, Context::Campaign));

        RegisterCampaignGlobalAddress(make_global_address(
            "Max Units in Navy",
            "Maximum number of units allowed in a navy.",
            std::make_unique<OffsetAddressResolver>("Empire.Retail.dll", 0x193FC6C),
            &g_max_units_in_navy, 1, 9999, Context::Campaign));
#else
        // Non-Steam build specific definitions (e.g., Rome2.dll)
        RegisterCampaignGlobalAddress(make_global_address(
            "Max Units in Army",
            "Maximum number of units allowed in an army.",
            std::make_unique<OffsetAddressResolver>("Rome2.dll", 0x18F835C),
            &g_max_units_in_army, 1, 9999, Context::Campaign));

        RegisterCampaignGlobalAddress(make_global_address(
            "Max Units in Navy",
            "Maximum number of units allowed in a navy.",
            std::make_unique<OffsetAddressResolver>("Rome2.dll", 0x18F8360),
            &g_max_units_in_navy, 1, 9999, Context::Campaign));
#endif
    }

}
