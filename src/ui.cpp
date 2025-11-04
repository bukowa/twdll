#include "ui.h"
#include "imgui.h"
#include "game/g_campaign.h" // For Game::RenderCampaignGlobalsImGui()

// --- ADDED: Define the rendering callback function for ImGui ---
void RenderMyUI() {
    static bool showCampaignGlobalsWindow = false; // Local static flag for Campaign Globals window visibility

    ImGui::SetNextWindowSize(ImVec2(400, 200), ImGuiCond_FirstUseEver); // Set a default size for the root window
    // TWDLL Root Window
    if (ImGui::Begin("TWDLL Root Window", NULL, ImGuiWindowFlags_MenuBar)) // Explicitly enable MenuBar flag
    {
        ImGui::Text("RenderMyUI is active and rendering."); // Debug text
        if (ImGui::BeginMenuBar())
        {
            if (ImGui::BeginMenu("Windows"))
            {
                // Menu item to toggle the Campaign Globals sub-window
                if (ImGui::MenuItem("Campaign Globals")) {
                    showCampaignGlobalsWindow = !showCampaignGlobalsWindow; // Toggle local flag
                }
                ImGui::EndMenu();
            }
            ImGui::EndMenuBar();
        }
        ImGui::Text("Hook is active!");
    }
    ImGui::End();

    // Render Campaign Globals sub-window if its flag is set
    if (showCampaignGlobalsWindow)
    {
        // Use ImGui::Begin with a closable flag to allow the user to close it
        if (ImGui::Begin("Campaign Globals", &showCampaignGlobalsWindow)) // Use local flag
        {
            Game::RenderCampaignGlobalsImGui();
        }
        ImGui::End();
    }
}
