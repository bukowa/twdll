#pragma once

namespace Game {
    enum class Context {
        Frontend,
        Campaign,
        Battle,
    };

    void SetCurrentContext(Context context);
    Context GetCurrentContext();
}
