cmake_minimum_required(VERSION 3.29)

# ========================
# Project Config
# ========================
project(libtwdll LANGUAGES C CXX)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_MESSAGE_CONTEXT_SHOW ON)

# ========================
# Compiler Settings
# ========================
add_compile_definitions(_CRT_SECURE_NO_WARNINGS)

if(MSVC)
    set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>DLL")
    set(CMAKE_CXX_FLAGS_RELEASE "/O2 /Zi /DNDEBUG /W4")
    set(CMAKE_CXX_FLAGS_DEBUG "/Od /Zi /D_DEBUG /W4")
endif()

# ========================
# Lib Setup (twdll.dll)
# ========================

set(TWDLL_DOC_SOURCES
    src/unit_script_interface.cpp
    src/battle_unit_script_interface.cpp
    src/character_script_interface.cpp
    src/faction_script_interface.cpp
    src/military_force_script_interface.cpp
)

set(TWDLL_NON_DOC_SOURCES
    src/lua/lua_api.cpp
    src/lua/lua_api.h
    src/libtwdll.cpp
    src/log.cpp
    src/ui.cpp
    include/imgui/imgui.cpp
    include/imgui/imgui_draw.cpp
    include/imgui/imgui_tables.cpp
    include/imgui/imgui_widgets.cpp
    include/imgui/backends/imgui_impl_win32.cpp
    include/imgui/backends/imgui_impl_dx11.cpp
    include/imgui/imgui_demo.cpp
    src/script_utils.cpp
    src/dx_finder.cpp
    src/signature_scanner.cpp
    src/module.cpp
    src/game/g_campaign.cpp

    src/game/globals/game_globals.cpp
    src/game/globals/address_resolver.cpp
)

add_library(twdll SHARED
    ${TWDLL_DOC_SOURCES}
    ${TWDLL_NON_DOC_SOURCES}
)

set (TWDLL_INCLUDE_DIRS
    include/
    include/spdlog
    include/imgui
    include/imgui/backends
    include/minhook/include
)

target_include_directories(twdll 
    PRIVATE
    src
    PUBLIC
    ${TWDLL_INCLUDE_DIRS}
)

if(STEAM_BUILD)
    target_compile_definitions(twdll PUBLIC STEAM_BUILD)

endif()

target_link_libraries(twdll PRIVATE
    "${CMAKE_CURRENT_SOURCE_DIR}/include/minhook/lib/libMinHook.x86.lib"
)

target_compile_options(twdll PRIVATE
    "/Zi"
    "$<$<CXX_COMPILER_ID:MSVC>:/utf-8>"
)

set_target_properties(twdll PROPERTIES
    OUTPUT_NAME "twdll"
    PDB_NAME "twdll"
    PDB_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}"
    DEBUG_INFORMATION_RELEASE TRUE
)

target_link_options(twdll PRIVATE "/DEBUG")

# ================================
# Mod Setup (twdll.pack)
# ================================
set(MOD_DIR "${CMAKE_CURRENT_SOURCE_DIR}/pack")
set(MOD_PACK "${CMAKE_BINARY_DIR}/twdll.pack")
file(GLOB_RECURSE MOD_FILES CONFIGURE_DEPENDS "${MOD_DIR}/*.*")

add_custom_target(
        build_mod_pack ALL
        DEPENDS ${MOD_PACK}
)
add_custom_command(
        OUTPUT ${MOD_PACK}
        COMMAND rpfm_cli --game rome_2 pack create --pack-path=${MOD_PACK}
        COMMAND rpfm_cli --game rome_2 pack add --pack-path=${MOD_PACK} -F "${MOD_DIR};"
        DEPENDS ${MOD_FILES}
        COMMENT "Building mod .pack file"
)

# ======================================
# Release Setup
# ======================================
set(RELEASE_DIR "${CMAKE_BINARY_DIR}/release")
file(MAKE_DIRECTORY ${RELEASE_DIR})

add_custom_target(
    build_release ALL
    COMMENT "Assembling release package in ${RELEASE_DIR}"
    COMMAND ${CMAKE_COMMAND} -E copy 
            ${MOD_PACK} 
            ${RELEASE_DIR}/data/twdll.pack

    COMMAND ${CMAKE_COMMAND} -E copy 
            $<TARGET_FILE:twdll> 
            ${RELEASE_DIR}/twdll.dll

    COMMAND ${CMAKE_COMMAND} -E copy_directory
            "${CMAKE_CURRENT_SOURCE_DIR}/docs/lua/doc"
            ${RELEASE_DIR}/twdlldocs
    DEPENDS
        twdll
        docs
        ${MOD_PACK}
    VERBATIM
)

# ===========================================
# Install Files
# ===========================================
option(INSTALL_MOD "Install mod into game dir" OFF)
option(GAME_INSTALL_DIR "C:/Games/Total War - Rome 2")

add_custom_target(kill_game
    COMMAND powershell -NoProfile -ExecutionPolicy Bypass -File "${CMAKE_CURRENT_SOURCE_DIR}/scripts/kill_game.ps1"
    COMMENT "Killing Rome2.exe process before build..."
    VERBATIM
)

add_custom_target(run_alone
    DEPENDS
        twdll
        install_mod
    COMMAND powershell -NoProfile -ExecutionPolicy Bypass -File "${CMAKE_CURRENT_SOURCE_DIR}/scripts/run_game.ps1" -GameLaunchDir "${GAME_INSTALL_DIR}"
    COMMENT "Installing mod, and launching the game via scripts..."
    VERBATIM
)

if(INSTALL_MOD)
        message(STATUS "Mod will be installed to: ${GAME_INSTALL_DIR}")

        add_custom_target(install_mod ALL
            DEPENDS
                build_release
                kill_game

            COMMAND ${CMAKE_COMMAND} -E copy_directory
                    "${RELEASE_DIR}/"
                    "${GAME_INSTALL_DIR}/"

            COMMENT "Deploying assembled mod from '${RELEASE_DIR}' to '${GAME_INSTALL_DIR}'"
            VERBATIM
        )
endif()

# ===========================================
# Documentation Generation (ldoc)
# ===========================================
add_subdirectory(docs/lua)

# ===========================================
# Testing
# ===========================================
include(CTest)

if(BUILD_TESTING)
    message(STATUS "Test build enabled, configuring Google Test...")

    include(FetchContent)
    FetchContent_Declare(
      googletest
      GIT_REPOSITORY https://github.com/google/googletest.git
      GIT_TAG v1.17.0
    )

    set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
    FetchContent_MakeAvailable(googletest)

    add_subdirectory(tests/cpp)
endif()
