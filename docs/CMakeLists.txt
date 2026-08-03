# ── Documentation Targets ─────────────────────────────────────────────────────
# This file is only included when building documentation.
# It sets up Lua build tools and LDoc generation per-game.

# Setup per-game output directories
set(TWDLL_DOCS_DIR "${PROJECT_SOURCE_DIR}/docs/${TW_GAME}")
set(TWDLL_DOC_OUTPUT "${TWDLL_DOCS_DIR}/index.html")

# Collect source files for documentation.
# List explicitly — one entry per module registered in main.cpp (luaopen_twdll).
# Do NOT use GLOB here: adding a new module requires adding its source file here,
# otherwise its docs page is missing (e.g. twdll.battle / GetBattleInfo).
if(TW_GAME STREQUAL "attila")
    set(TWDLL_DOC_SOURCES
        "${PROJECT_SOURCE_DIR}/src/main.cpp"
        "${PROJECT_SOURCE_DIR}/src/common/lua_core.cpp"
        "${PROJECT_SOURCE_DIR}/src/attila/faction.cpp"
        "${PROJECT_SOURCE_DIR}/src/attila/unit.cpp"
        "${PROJECT_SOURCE_DIR}/src/attila/world.cpp"
        "${PROJECT_SOURCE_DIR}/src/attila/campaign_ui.cpp"
        "${PROJECT_SOURCE_DIR}/src/attila/military_force.cpp"
        "${PROJECT_SOURCE_DIR}/src/attila/model.cpp"
        "${PROJECT_SOURCE_DIR}/src/attila/battle_hooks.cpp"
    )
elseif(TW_GAME STREQUAL "rome2")
    set(TWDLL_DOC_SOURCES
        "${PROJECT_SOURCE_DIR}/src/main.cpp"
        "${PROJECT_SOURCE_DIR}/src/common/lua_core.cpp"
        "${PROJECT_SOURCE_DIR}/src/rome2/faction.cpp"
        "${PROJECT_SOURCE_DIR}/src/rome2/unit.cpp"
        "${PROJECT_SOURCE_DIR}/src/rome2/world.cpp"
        "${PROJECT_SOURCE_DIR}/src/rome2/campaign_ui.cpp"
    )
endif()

# Include Lua targets for LDoc generation
add_subdirectory(lua)

