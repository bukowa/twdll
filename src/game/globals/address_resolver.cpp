#include "address_resolver.h"
#include "log.h"
#include "signature_scanner.h"
#include <windows.h>
#include <Psapi.h>

namespace Game
{
    OffsetAddressResolver::OffsetAddressResolver(const std::string& module_name, uintptr_t offset)
        : m_module_name(module_name), m_offset(offset)
    {
    }

    uintptr_t OffsetAddressResolver::Resolve() const
    {
        HMODULE hModule = GetModuleHandleA(m_module_name.c_str());
        if (hModule == nullptr)
        {
            spdlog::error("Failed to get module handle for {}. Error: {}", m_module_name, GetLastError());
            return 0;
        }
        return (uintptr_t)hModule + m_offset;
    }

    SignatureAddressResolver::SignatureAddressResolver(const std::string& module_name, const std::string& signature)
        : m_module_name(module_name), m_signature(signature)
    {
    }

    uintptr_t SignatureAddressResolver::Resolve() const
    {
        HMODULE hModule = GetModuleHandleA(m_module_name.c_str());
        if (hModule == nullptr)
        {
            spdlog::error("Failed to get module handle for {}. Error: {}", m_module_name, GetLastError());
            return 0;
        }

        MODULEINFO mi = { 0 };
        if (!GetModuleInformation(GetCurrentProcess(), hModule, &mi, sizeof(mi)))
        {
            spdlog::error("Failed to get module information for {}. Error: {}", m_module_name, GetLastError());
            return 0;
        }
        size_t module_size = mi.SizeOfImage;

        uintptr_t found_address = find_signature((uintptr_t)hModule, module_size, m_signature.c_str());

        if (!found_address)
        {
            spdlog::error("Signature not found for '{}' in module '{}'.", m_signature, m_module_name);
        }
        return found_address;
    }
}
