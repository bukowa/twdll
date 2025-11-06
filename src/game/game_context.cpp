#include "game_context.h"

namespace Game {
    Context g_current_context = Context::Frontend;

    void SetCurrentContext(Context context) {
        g_current_context = context;
    }

    Context GetCurrentContext() {
        return g_current_context;
    }
}
