#pragma once

#include <string>
#include <memory>
#include "game/game_context.h"


namespace Game {

// The common, non-templated interface for all global variables.
// The UI and Lua systems will interact with this.
class IGlobalAddress {
public:
    virtual ~IGlobalAddress() = default;

    // Renders the appropriate ImGui widget for this global.
    virtual void RenderImGuiWidget() = 0;

    // Gets the name of the global variable.
    virtual const std::string& GetName() const = 0;

    // Gets the description of the global variable.
    virtual const std::string& GetDescription() const = 0;

    virtual Context GetContext() const = 0;
};

} // namespace Game
