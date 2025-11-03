cmake_minimum_required(VERSION 3.29)

# ========================
# Project Config
# ========================
project(libtwdll LANGUAGES C CXX)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

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
add_library(twdll SHARED
        src/lua/lua_api.cpp
        src/libtwdll.cpp
        src/log.cpp
        src/unit_script_interface.cpp
        src/battle_unit_script_interface.cpp
        src/character_script_interface.cpp
        src/faction_script_interface.cpp
        src/military_force_script_interface.cpp
        src/script_utils.cpp
        src/dx_finder.cpp
        src/signature_scanner.cpp
        src/module.cpp
        include/imgui/imgui.cpp
        include/imgui/imgui_draw.cpp
        include/imgui/imgui_tables.cpp
        include/imgui/imgui_widgets.cpp
        include/imgui/backends/imgui_impl_win32.cpp
        include/imgui/backends/imgui_impl_dx11.cpp
        include/imgui/imgui_demo.cpp
)

target_include_directories(twdll
        PRIVATE
        src
        PUBLIC
        include/
        include/spdlog
        include/imgui
        include/imgui/backends
        include/minhook/include
)

target_link_libraries(twdll PRIVATE
        "${CMAKE_CURRENT_SOURCE_DIR}/include/minhook/lib/libMinHook.x86.lib"
)

set_target_properties(twdll PROPERTIES OUTPUT_NAME "twdll")
set_target_properties(twdll PROPERTIES DEBUG_INFORMATION_RELEASE TRUE)

# Force PDB generation for all configurations
target_compile_options(twdll PRIVATE "/Zi")
target_compile_options(twdll PRIVATE "$<$<CXX_COMPILER_ID:MSVC>:/utf-8>")
target_link_options(twdll PRIVATE "/DEBUG")

set_target_properties(twdll PROPERTIES
    PDB_NAME "twdll"
    PDB_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}"
)

# ================================
# Mod Files
# ================================
set(MOD_DIR "${CMAKE_CURRENT_SOURCE_DIR}/pack")
set(MOD_PACK "${CMAKE_BINARY_DIR}/twdll.pack")
file(GLOB_RECURSE MOD_FILES CONFIGURE_DEPENDS "${MOD_DIR}/*.*")

# ================================
# Mod Setup (twdll.pack)
# ================================
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
# Release Files
# ======================================
set(RELEASE_DIR "${CMAKE_BINARY_DIR}/release")
file(MAKE_DIRECTORY ${RELEASE_DIR})

# ======================================
# Release Setup
# ======================================

# This single target defines all the steps for creating the release layout.
add_custom_target(
    build_release ALL
    
    # These COMMANDS are executed when you build this target.
    # We use generator expressions to get the path to the output file at build time.
    COMMAND ${CMAKE_COMMAND} -E copy 
            ${MOD_PACK} 
            ${RELEASE_DIR}/data/twdll.pack

    COMMAND ${CMAKE_COMMAND} -E copy 
            $<TARGET_FILE:twdll> 
            ${RELEASE_DIR}/twdll.dll

    COMMAND ${CMAKE_COMMAND} -E copy_directory
            "${CMAKE_CURRENT_SOURCE_DIR}/docs/lua/html"
            ${RELEASE_DIR}/twdlldocs

    # These are the things that must be built *before* the commands above can run.
    # It's better to depend on the TARGET `twdll` rather than just the file.
    DEPENDS
        twdll
        docs
        ${MOD_PACK}

    # A descriptive comment that will show up during the build.
    COMMENT "Assembling release package in ${RELEASE_DIR}"
    
    # This is good practice for custom commands to ensure they are executed correctly
    # by the shell, especially if they contain special characters.
    VERBATIM
)

# ===========================================
# Install Files
# ===========================================
option(INSTALL_MOD "Install mod into game dir" OFF)
set(INSTALL_DIRS
        "C:/Games/Total War - Rome 2"
        "C:/Program Files (x86)/Steam/steamapps/common/Total War Rome II"
)

option(SHOULD_INSTALL_DLL "Install dll into game dir" ON)
# ===========================================
# Installation / Deployment Setup
# ===========================================

option(INSTALL_MOD "Install/deploy mod directly into the game directory" OFF)

if(INSTALL_MOD)
    # --- 1. Define the game directory location ---
    # We use a CACHE variable so the user can easily override it with -DGAME_INSTALL_DIR=...
    set(GAME_INSTALL_DIR "" CACHE PATH "Path to the Total War: Rome II game directory.")

    # --- 2. Auto-detect the game directory if the user hasn't specified one ---
    if(NOT GAME_INSTALL_DIR)
        message(STATUS "GAME_INSTALL_DIR not set. Searching for game directory...")
        set(SEARCH_DIRS
                "C:/Games/Total War - Rome 2"
                "C:/Program Files (x86)/Steam/steamapps/common/Total War Rome II"
                # Add any other common paths here
        )
        foreach(DIR IN LISTS SEARCH_DIRS)
            if(EXISTS "${DIR}/Rome2.exe") # A good way to verify it's the right folder
                message(STATUS "Found game at: ${DIR}")
                set(GAME_INSTALL_DIR ${DIR} CACHE PATH "Path to the Total War: Rome II game directory." FORCE)
                break()
            endif()
        endforeach()
    endif()

    # --- 3. Create the simplified install_mod target ---
    # Abort if we still couldn't find the game directory.
    if(NOT GAME_INSTALL_DIR OR NOT EXISTS "${GAME_INSTALL_DIR}")
        message(FATAL_ERROR "INSTALL_MOD is ON, but the game directory could not be found or specified. "
                            "Please configure it with -DGAME_INSTALL_DIR=\"C:/path/to/game\"")
    else()
        message(STATUS "Mod will be installed to: ${GAME_INSTALL_DIR}")

        # This single target does all the work.
        add_custom_target(install_mod ALL
            # CRITICAL: This target must run *after* the entire release package is assembled.
            DEPENDS
                build_release
                kill_game

            # Command 1: Copy the entire assembled release directory.
            # The trailing slash is important: it copies the *contents* of release/ into the game dir.
            COMMAND ${CMAKE_COMMAND} -E copy_directory
                    "${RELEASE_DIR}/"
                    "${GAME_INSTALL_DIR}/"

            # Command 2: Copy the tests directory, if it exists.
            COMMAND ${CMAKE_COMMAND} -E copy_directory
                    "${CMAKE_CURRENT_SOURCE_DIR}/tests"
                    "${GAME_INSTALL_DIR}/tests"

            COMMENT "Deploying assembled mod from '${RELEASE_DIR}' to '${GAME_INSTALL_DIR}'"
            VERBATIM
        )
    endif()
endif()

# ===========================================
# Documentation Generation (ldoc)
# ===========================================
add_subdirectory(docs/lua)

# ===========================================
# Run Target (mimics make run-alone)
# ===========================================

# Add a custom target to kill the game process
add_custom_target(kill_game
    COMMAND powershell -NoProfile -ExecutionPolicy Bypass -File "${CMAKE_CURRENT_SOURCE_DIR}/scripts/kill_game.ps1"
    COMMENT "Killing Rome2.exe process before build..."
    VERBATIM
)

# Define the directory to launch the game from.
if(INSTALL_DIRS)
    list(GET INSTALL_DIRS 0 GAME_LAUNCH_DIR)
    message(STATUS "Run target will launch game from: ${GAME_LAUNCH_DIR}")
else()
    message(FATAL_ERROR "INSTALL_DIRS is not set. Cannot define 'run_alone' target.")
endif()

# Define the main "run_alone" target.
# This target now orchestrates by calling the run_game.ps1 script.
add_custom_target(run_alone
    # Dependencies remain the same, ensuring the correct order of operations.
    DEPENDS
        twdll
        install_mod

    # The single command calls our script and passes the required paths as named arguments.
    # This is clean, readable, and robust.
    COMMAND powershell -NoProfile -ExecutionPolicy Bypass -File "${CMAKE_CURRENT_SOURCE_DIR}/scripts/run_game.ps1" -GameLaunchDir "${GAME_LAUNCH_DIR}"

    COMMENT "Installing mod, and launching the game via scripts..."
    VERBATIM
)
