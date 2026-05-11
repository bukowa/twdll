# ── Documentation Targets ─────────────────────────────────────────────────────
# This file is only included when building documentation.
# It sets up Lua build tools and LDoc generation per-game.

# Setup per-game output directories
set(TWDLL_DOCS_DIR "${PROJECT_SOURCE_DIR}/docs/${TW_GAME}")
set(TWDLL_DOC_OUTPUT "${TWDLL_DOCS_DIR}/index.html")

# Collect source files for documentation
# Only parse common and game-specific files to avoid duplicate LDoc modules
file(GLOB TWDLL_DOC_SOURCES
    "${PROJECT_SOURCE_DIR}/src/*.cpp"
    "${PROJECT_SOURCE_DIR}/src/common/*.cpp"
    "${PROJECT_SOURCE_DIR}/src/${TW_GAME}/*.cpp"
)

# Include Lua targets for LDoc generation
add_subdirectory(lua)

