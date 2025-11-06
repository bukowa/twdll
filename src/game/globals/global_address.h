#pragma once

#include "i_global_address.h"
#include "address_resolver.h"
#include "imgui.h"
#include <string>
#include <memory>
#include <type_traits> // For std::is_same_v

namespace Game {

// Templated, type-safe implementation of the IGlobalAddress interface.
template <typename T>
class GlobalAddress : public IGlobalAddress {
public:
    GlobalAddress(std::string name, std::string description, std::unique_ptr<IAddressResolver> resolver, T* target_ptr, T min_val, T max_val, Context context)
        : name_(std::move(name)),
          description_(std::move(description)),
          resolver_(std::move(resolver)),
          target_variable_ptr_(target_ptr),
          min_value_(min_val),
          max_value_(max_val),
          context_(context) {}

    const std::string& GetName() const override { return name_; }
    const std::string& GetDescription() const override { return description_; }
    Context GetContext() const override { return context_; }

    void RenderImGuiWidget() override {
        if (GetContext() != GetCurrentContext()) {
            return;
        }
        // Resolve the address if we haven't already
        if (!resolved_address_) {
            uintptr_t address = resolver_->Resolve();
            if (address != 0) {
                resolved_address_ = reinterpret_cast<T*>(address);
            }
        }

        // If we have a resolved address, use it. Otherwise, fall back to the direct pointer.
        T* current_ptr = resolved_address_ ? resolved_address_ : target_variable_ptr_;

        if (!current_ptr) {
            ImGui::Text("%s: Address not resolved", name_.c_str());
            return;
        }

        // Use compile-time `if constexpr` to generate type-specific ImGui widgets.
        // This is highly efficient and completely type-safe.
        if constexpr (std::is_same_v<T, int>) {
            ImGui::SliderInt(name_.c_str(), current_ptr, min_value_, max_value_);
        } else if constexpr (std::is_same_v<T, float>) {
            ImGui::SliderFloat(name_.c_str(), current_ptr, min_value_, max_value_);
        } else if constexpr (std::is_same_v<T, bool>) {
            ImGui::Checkbox(name_.c_str(), current_ptr);
        } else {
            // Fallback for unsupported types
            ImGui::Text("Unsupported type for: %s", name_.c_str());
        }

        if (ImGui::IsItemHovered()) {
            ImGui::SetTooltip("%s", description_.c_str());
        }
    }

private:
    std::string name_;
    std::string description_;
    std::unique_ptr<IAddressResolver> resolver_;
    T* target_variable_ptr_; // Type-safe pointer to the C++ global
    T* resolved_address_ = nullptr; // The dynamically resolved address
    T min_value_;
    T max_value_;
    Context context_;
};

// Factory function to make creating these objects cleaner and safer.
// It deduces the template type `T` from the pointer you pass it.
template <typename T>
std::unique_ptr<IGlobalAddress> make_global_address(
    std::string name,
    std::string description,
    std::unique_ptr<IAddressResolver> resolver,
    T* target_ptr,
    T min_val,
    T max_val,
    Context context) {
    return std::make_unique<GlobalAddress<T>>(std::move(name), std::move(description), std::move(resolver), target_ptr, min_val, max_val, context);
}

} // namespace Game
