#pragma once

#include <string>
#include <vector>
#include <variant>
#include <functional>

#include "imgui.h"

namespace Game
{
    // Enum to represent the type of the global address
    enum class GlobalAddressType
    {
        Int,
        Float,
        Bool
    };

    // Struct to hold information about a global address
    struct GlobalAddress
    {
        std::string name;
        GlobalAddressType type;
        std::variant<int*, float*, bool*> address; // Pointer to the actual global variable
        std::string description; // Optional description for tooltip

        // For ImGui sliders/inputs
        int min_int = 0;
        int max_int = 0;
        float min_float = 0.0f;
        float max_float = 0.0f;

        // Constructor for int
        GlobalAddress(const std::string& name, int* addr, const std::string& desc = "", int min = 0, int max = 0)
            : name(name), type(GlobalAddressType::Int), address(addr), description(desc), min_int(min), max_int(max) {}

        // Constructor for float
        GlobalAddress(const std::string& name, float* addr, const std::string& desc = "", float min = 0.0f, float max = 0.0f)
            : name(name), type(GlobalAddressType::Float), address(addr), description(desc), min_float(min), max_float(max) {}

        // Constructor for bool
        GlobalAddress(const std::string& name, bool* addr, const std::string& desc = "")
            : name(name), type(GlobalAddressType::Bool), address(addr), description(desc) {}

        // Generic getter
        template<typename T>
        T getValue() const
        {
            return *std::get<T*>(address);
        }

        // Generic setter
        template<typename T>
        void setValue(T value)
        {
            *std::get<T*>(address) = value;
        }
    };

    // Manager class for global addresses
    class GlobalAddressManager
    {
    public:
        static GlobalAddressManager& GetInstance()
        {
            static GlobalAddressManager instance;
            return instance;
        }

        void RegisterAddress(const GlobalAddress& addr)
        {
            m_addresses.push_back(addr);
        }

        void RenderImGui();

    private:
        GlobalAddressManager() = default;
        ~GlobalAddressManager() = default;
        GlobalAddressManager(const GlobalAddressManager&) = delete;
        GlobalAddressManager& operator=(const GlobalAddressManager&) = delete;

        std::vector<GlobalAddress> m_addresses;
    };

    void RegisterCampaignGlobalAddresses();
    void RenderCampaignGlobalsImGui();
}
