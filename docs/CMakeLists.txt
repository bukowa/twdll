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
        # twdll.* global modules (alphabetical)
        "${PROJECT_SOURCE_DIR}/src/attila/battle_hooks.cpp"         # twdll.battle
        "${PROJECT_SOURCE_DIR}/src/attila/cai/cai_core.cpp"          # twdll.cai
        "${PROJECT_SOURCE_DIR}/src/attila/campaign_ui.cpp"          # twdll.campaign_ui
        "${PROJECT_SOURCE_DIR}/src/common/lua_core.cpp"             # twdll.core
        "${PROJECT_SOURCE_DIR}/src/attila/model.cpp"                # twdll.model
        "${PROJECT_SOURCE_DIR}/src/attila/world.cpp"                # twdll.world

        # Script interfaces (alphabetical)
        "${PROJECT_SOURCE_DIR}/src/attila/art_set.cpp"              # ARTSET_SCRIPT_INTERFACE
        "${PROJECT_SOURCE_DIR}/src/attila/political_party.cpp"      # CAMPAIGN_POLITICAL_PARTY_SCRIPT_INTERFACE
        "${PROJECT_SOURCE_DIR}/src/attila/character.cpp"            # CHARACTER_SCRIPT_INTERFACE
        "${PROJECT_SOURCE_DIR}/src/attila/faction.cpp"              # FACTION_SCRIPT_INTERFACE
        "${PROJECT_SOURCE_DIR}/src/attila/military_force.cpp"       # MILITARY_FORCE_SCRIPT_INTERFACE
        "${PROJECT_SOURCE_DIR}/src/attila/political_party_list.cpp" # POLITICAL_PARTY_LIST_SCRIPT_INTERFACE
        "${PROJECT_SOURCE_DIR}/src/attila/region.cpp"               # REGION_SCRIPT_INTERFACE
        "${PROJECT_SOURCE_DIR}/src/attila/religion_list.cpp"        # RELIGION_LIST_SCRIPT_INTERFACE
        "${PROJECT_SOURCE_DIR}/src/attila/religion.cpp"             # RELIGION_SCRIPT_INTERFACE
        "${PROJECT_SOURCE_DIR}/src/attila/slot.cpp"                 # SLOT_SCRIPT_INTERFACE
        "${PROJECT_SOURCE_DIR}/src/attila/unit.cpp"                 # UNIT_SCRIPT_INTERFACE
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
