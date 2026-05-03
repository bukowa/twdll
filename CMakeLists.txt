cmake_minimum_required(VERSION 3.29)
project(libtwdll LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")

if(NOT MSVC)
    message(FATAL_ERROR "MSVC (Visual Studio) compiler required.")
endif()

# ── Game selection ────────────────────────────────────────────────────────────
set(TW_GAME "rome2" CACHE STRING "Target game: rome2 | attila")
set_property(CACHE TW_GAME PROPERTY STRINGS rome2 attila)
message(STATUS "Building for game: ${TW_GAME}")

# ── Source files ──────────────────────────────────────────────────────────────
# Common sources shared across all games
set(TWDLL_COMMON_SOURCES
    src/main.cpp
    src/log.cpp
    src/lua/lua_api.cpp
    src/lua/lua_core.cpp
    src/signature_scanner.cpp
)

# Game-specific sources (struct layouts, Lua signatures)
file(GLOB TWDLL_GAME_SOURCES CONFIGURE_DEPENDS "src/${TW_GAME}/*.cpp")

# ── Library target ────────────────────────────────────────────────────────────
add_library(twdll SHARED
    ${TWDLL_COMMON_SOURCES}
    ${TWDLL_GAME_SOURCES}
)

target_include_directories(twdll PRIVATE src)

set_target_properties(twdll PROPERTIES
    OUTPUT_NAME              "twdll"
    PDB_NAME                 "twdll"
    RUNTIME_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}"
    LIBRARY_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}"
    ARCHIVE_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}"
    PDB_OUTPUT_DIRECTORY     "${CMAKE_BINARY_DIR}"
    DEBUG_INFORMATION_RELEASE TRUE
)

target_compile_options(twdll PRIVATE /Zi /utf-8 /W4)
target_link_options(twdll PRIVATE /DEBUG)
target_compile_definitions(twdll PRIVATE _CRT_SECURE_NO_WARNINGS)

# ── Task targets (delegate to tools/twdll.ps1) ────────────────────────────────
# Logic lives in the script; these targets exist for IDE integration.
set(TW_PS1 "${CMAKE_SOURCE_DIR}/tools/twdll.ps1")

add_custom_target(tw-pack
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" pack "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Building mod pack (${TW_GAME})"
    VERBATIM
)

add_custom_target(tw-install
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" install "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Installing twdll (${TW_GAME})"
    VERBATIM
)

add_custom_target(tw-run
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" run "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Running game (${TW_GAME})"
    VERBATIM
)

add_custom_target(tw-test
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" test "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Running tests (${TW_GAME})"
    VERBATIM USES_TERMINAL
)

# ── Documentation ─────────────────────────────────────────────────────────────
# Only parse common and game-specific files to avoid duplicate LDoc modules
file(GLOB TWDLL_DOC_SOURCES "src/*.cpp" "src/${TW_GAME}/*.cpp")
add_subdirectory(docs/lua)

add_custom_target(tw-docs
    DEPENDS docs
    COMMENT "Building unified documentation"
)
