cmake_minimum_required(VERSION 3.29)
project(libtwdll LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# ── MinHook ───────────────────────────────────────────────────────────────────
add_subdirectory(vendor/minhook EXCLUDE_FROM_ALL)

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
    src/common/log.cpp
    src/common/lua_api.cpp
    src/common/lua_core.cpp
    src/common/signature_scanner.cpp
    src/common/campaign_hooks.cpp
)

# Game-specific sources (struct layouts, Lua signatures)
file(GLOB TWDLL_GAME_SOURCES CONFIGURE_DEPENDS "src/${TW_GAME}/*.cpp")

# ── Library target ────────────────────────────────────────────────────────────
add_library(twdll SHARED
    ${TWDLL_COMMON_SOURCES}
    ${TWDLL_GAME_SOURCES}
)

target_include_directories(twdll PRIVATE src)
target_link_libraries(twdll PRIVATE minhook)

# Add Windows SDK and compiler standard library paths
if(MSVC)
    target_include_directories(twdll PRIVATE
        "$ENV{INCLUDE}"
    )
    # Also need LIB path for linking
    string(REPLACE ";" "\\;" LIB_PATHS "$ENV{LIB}")
    target_link_directories(twdll PRIVATE ${LIB_PATHS})
endif()

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

# ── Game preprocessor define ──────────────────────────────────────────────────
if(TW_GAME STREQUAL "attila")
    target_compile_definitions(twdll PRIVATE TW_ATTILA)
elseif(TW_GAME STREQUAL "rome2")
    target_compile_definitions(twdll PRIVATE TW_ROME2)
endif()

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

# ── Documentation targets (defer to docs/CMakeLists.txt) ────────────────────
add_subdirectory(docs)

add_custom_target(tw-docs
    DEPENDS docs
    COMMENT "Building documentation (${TW_GAME})"
)
