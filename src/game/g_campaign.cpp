#include "g_campaign.h"
#include <windows.h>

namespace Game
{
    // Global variable for max units in army, will be pointed to the resolved address
    int* g_max_units_in_army_ptr = nullptr;
    // Global variable for max units in navy, will be pointed to the resolved address
    int* g_max_units_in_navy_ptr = nullptr;

    void GlobalAddressManager::RenderImGui()
    {
        for (auto& addr : m_addresses)
        {
            ImGui::PushID(addr.name.c_str());

            if (!addr.description.empty())
            {
                ImGui::TextDisabled("(?)");
                if (ImGui::IsItemHovered())
                {
                    ImGui::BeginTooltip();
                    ImGui::PushTextWrapPos(ImGui::GetFontSize() * 35.0f);
                    ImGui::TextUnformatted(addr.description.c_str());
                    ImGui::PopTextWrapPos();
                    ImGui::EndTooltip();
                }
                ImGui::SameLine();
            }

            switch (addr.type)
            {
                case GlobalAddressType::Int:
                {
                    int* val = std::get<int*>(addr.address);
                    if (addr.min_int != addr.max_int)
                    {
                        ImGui::SliderInt(addr.name.c_str(), val, addr.min_int, addr.max_int);
                    }
                    else
                    {
                        ImGui::InputInt(addr.name.c_str(), val);
                    }
                    break;
                }
                case GlobalAddressType::Float:
                {
                    float* val = std::get<float*>(addr.address);
                    if (addr.min_float != addr.max_float)
                    {
                        ImGui::SliderFloat(addr.name.c_str(), val, addr.min_float, addr.max_float);
                    }
                    else
                    {
                        ImGui::InputFloat(addr.name.c_str(), val);
                    }
                    break;
                }
                case GlobalAddressType::Bool:
                {
                    bool* val = std::get<bool*>(addr.address);
                    ImGui::Checkbox(addr.name.c_str(), val);
                    break;
                }
            }
            ImGui::PopID();
        }
    }

    void RenderCampaignGlobalsImGui()
    {
        if (ImGui::Begin("Campaign Globals"))
        {
            GlobalAddressManager::GetInstance().RenderImGui();
            ImGui::End();
        }
    }

    // Function to initialize and register campaign global addresses
    void RegisterCampaignGlobalAddresses()
    {
        // Resolve and register "Max Units in Army"
        HMODULE empireRetailModule = GetModuleHandleA("Empire.Retail.dll");
        if (empireRetailModule != nullptr)
        {
            uintptr_t baseAddress = (uintptr_t)empireRetailModule;
            uintptr_t maxUnitsArmyOffset = 0x193FC68;
            g_max_units_in_army_ptr = (int*)(baseAddress + maxUnitsArmyOffset);

            GlobalAddressManager::GetInstance().RegisterAddress(
                GlobalAddress("Max Units in Army", g_max_units_in_army_ptr, "Maximum number of units allowed in an army", 1, 40));

            uintptr_t maxUnitsNavyOffset = 0x193FC6C;
            g_max_units_in_navy_ptr = (int*)(baseAddress + maxUnitsNavyOffset);

            GlobalAddressManager::GetInstance().RegisterAddress(
                GlobalAddress("Max Units in Navy", g_max_units_in_navy_ptr, "Maximum number of units allowed in a navy", 1, 40));
        }
    }
}
