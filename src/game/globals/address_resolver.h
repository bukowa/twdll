#pragma once

#include <cstdint>
#include <string>
#include <memory>

namespace Game
{
    // Abstract base class for address resolvers
    class IAddressResolver
    {
    public:
        virtual ~IAddressResolver() = default;
        virtual uintptr_t Resolve() const = 0;
        virtual std::string GetModuleName() const = 0;
    };



    class OffsetAddressResolver : public IAddressResolver
    {
    public:
        OffsetAddressResolver(const std::string& module_name, uintptr_t offset);
        uintptr_t Resolve() const override;
        std::string GetModuleName() const override { return m_module_name; }

    private:
        std::string m_module_name;
        uintptr_t m_offset;
    };

    class SignatureAddressResolver : public IAddressResolver
    {
    public:
        SignatureAddressResolver(const std::string& module_name, const std::string& signature);
        uintptr_t Resolve() const override;
        std::string GetModuleName() const override { return m_module_name; }

    private:
        std::string m_module_name;
        std::string m_signature;
    };
}