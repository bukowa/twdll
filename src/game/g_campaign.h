#pragma once

#include <vector>
#include <memory>
#include "globals/i_global_address.h"

namespace Game {

    // This function will be responsible for creating and registering all the global variables.
    void InitializeCampaignGlobalAddresses();

    // This function will render the ImGui widgets for all registered globals.
    void RenderCampaignGlobalsImGui();

    // This function provides a clean way to register a new global from anywhere.
    void RegisterCampaignGlobalAddress(std::unique_ptr<IGlobalAddress> address);

} // namespace Game
