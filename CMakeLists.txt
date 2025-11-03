cmake_minimum_required(VERSION 3.29)

# ========================
# Project Config
# ========================
project(libtwdll)

enable_language(C CXX)

set(CMAKE_C_STANDARD 11)
set(CMAKE_CXX_STANDARD 17)

# ========================
# Project Variables
# ========================
get_filename_component(CURRENT_DIR ${CMAKE_CURRENT_SOURCE_DIR} REALPATH)

# ========================
# Python Config
# ========================
set(PYTHON_INCLUDE_DIR ${CMAKE_CURRENT_SOURCE_DIR}/include/python)
set(PYTHON_LIBRARY_RELEASE ${CMAKE_CURRENT_SOURCE_DIR}/include/python/libs/python313.lib)
set(PYTHON_LIBRARY_DEBUG ${CMAKE_CURRENT_SOURCE_DIR}/include/python/libs/python313_d.lib)

# ========================
# Lua Config
# ========================
set(LUA_VERSION "5.1.2")
message("LUA_VERSION is set to: ${LUA_VERSION}")
set(LUA_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/include/lua-ver-${LUA_VERSION}/src")

# ========================
# Lua Files
# ========================
file(GLOB LUA_SOURCES "${LUA_SRC_DIR}/*.c")
list(REMOVE_ITEM LUA_SOURCES "${LUA_SRC_DIR}/lua.c" "${LUA_SRC_DIR}/luac.c")
list(SORT LUA_SOURCES)

# ========================
# Lua Setup (luadll)
# ========================
add_library(luadll SHARED ${LUA_SOURCES})

target_include_directories(luadll
        PRIVATE
        ${LUA_SRC_DIR}
)

set_target_properties(luadll PROPERTIES
    OUTPUT_NAME "lua"
    PDB_NAME "lua_dll"
)

add_executable(lua_for_docs "${LUA_SRC_DIR}/lua.c")

target_include_directories(lua_for_docs
        PRIVATE
        ${LUA_SRC_DIR}
)

target_link_libraries(lua_for_docs PRIVATE luadll)

target_compile_definitions(luadll
        PRIVATE
        LUA_BUILD_AS_DLL
        LUA_CORE
        LUA_WIN
        PUBLIC
        LUA_LIB
)

# ========================
# Lua Setup (lua.exe)
# ========================
add_executable(lua.exe "${LUA_SRC_DIR}/lua.c" ${LUA_SOURCES})

target_include_directories(lua.exe
        PRIVATE
        ${LUA_SRC_DIR}
)

target_compile_definitions(lua.exe PRIVATE LUA_CORE LUA_WIN)
set_target_properties(lua.exe PROPERTIES
    OUTPUT_NAME "lua"
    PDB_NAME "lua_exe"
)

# ========================
# Lua Setup (luac.exe)
# ========================
add_executable(luac.exe "${LUA_SRC_DIR}/luac.c" ${LUA_SOURCES})

target_include_directories(luac.exe
        PRIVATE
        ${LUA_SRC_DIR}
)

target_compile_definitions(luac.exe PRIVATE LUA_CORE LUA_WIN)
set_target_properties(luac.exe PROPERTIES OUTPUT_NAME "luac")

# ========================
# Lib Setup (twdll.dll)
# ========================
set(MINHOOK_INCLUDE_DIR ${CMAKE_CURRENT_SOURCE_DIR}/include/minhook/include)
set(MINHOOK_LIBRARY ${CMAKE_CURRENT_SOURCE_DIR}/include/minhook/lib/libMinHook.x86.lib)

add_library(twdll SHARED
        src/libtwdll.cpp
        src/log.cpp
        src/unit_script_interface.cpp
        src/battle_unit_script_interface.cpp
        src/character_script_interface.cpp
        src/faction_script_interface.cpp
        src/military_force_script_interface.cpp
        src/character_script_interface.h
        src/faction_script_interface.h
        src/log.h
        src/unit_script_interface.h
        src/script_utils.cpp
        src/script_utils.h
        src/battle_unit_script_interface.h
        src/military_force_script_interface.h
        src/dx_finder.cpp
        src/dx_finder.h
        src/signature_scanner.cpp
        src/game_lua_api.cpp
        src/lua_forward_declarations.h
        src/module.h
        src/module.cpp
        include/imgui/imgui.cpp
        include/imgui/imgui_draw.cpp
        include/imgui/imgui_tables.cpp
        include/imgui/imgui_widgets.cpp
        include/imgui/backends/imgui_impl_win32.cpp
        include/imgui/backends/imgui_impl_dx11.cpp
        include/imgui/imgui_demo.cpp
        src/performance_test.h
        src/performance_test.cpp
)


set_target_properties(twdll PROPERTIES OUTPUT_NAME "twdll")
set_target_properties(twdll PROPERTIES DEBUG_INFORMATION_RELEASE TRUE)

# Force PDB generation for all configurations
target_compile_options(twdll PRIVATE "/Zi")
target_link_options(twdll PRIVATE "/DEBUG")

set_target_properties(twdll PROPERTIES
    PDB_NAME "twdll"
    PDB_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}"
)

target_include_directories(twdll
        PUBLIC
        src
        include/imgui
        include/imgui/backends
        ${PYTHON_INCLUDE_DIR}
        PRIVATE
        ${MINHOOK_INCLUDE_DIR}
)

target_link_libraries(twdll PRIVATE
        ${MINHOOK_LIBRARY}
        optimized ${PYTHON_LIBRARY_RELEASE}
        debug ${PYTHON_LIBRARY_DEBUG}
)

# ========================
# Compiler Settings
# ========================
if(MSVC)
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>DLL")
endif()

# ================================
# Mod Files
# ================================
set(MOD_DIR "${CURRENT_DIR}/pack")
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

    COMMAND ${CMAKE_COMMAND} -E copy
            ${CMAKE_CURRENT_SOURCE_DIR}/include/python/embed/python313.dll
            ${RELEASE_DIR}/python313.dll

    COMMAND ${CMAKE_COMMAND} -E copy_directory
            "${CMAKE_CURRENT_SOURCE_DIR}/luadoc"
            ${RELEASE_DIR}/twdlldocs

    COMMAND ${CMAKE_COMMAND} -E copy_directory 
            ${CMAKE_CURRENT_SOURCE_DIR}/include/python/embed 
            ${RELEASE_DIR}/python

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

# ========================
# Compiler Settings
# ========================
add_definitions(-D_CRT_SECURE_NO_WARNINGS)
target_compile_options(luadll PRIVATE "/wd4005")

if (MSVC)
    set(CMAKE_CXX_FLAGS_RELEASE "/O2 /Zi /DNDEBUG /W4")
    set(CMAKE_CXX_FLAGS_DEBUG "/Od /Zi /D_DEBUG /W4")
    set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>DLL")
endif()

# ========================
# LuaFileSystem Setup (lfs.dll)
# ========================
add_subdirectory(include/luafilesystem)
# ===========================================
# Documentation Generation (ldoc)
# ===========================================
file(GLOB_RECURSE DOC_SOURCES
        CONFIGURE_DEPENDS
        "${CMAKE_CURRENT_SOURCE_DIR}/src/*.cpp"
        "${CMAKE_CURRENT_SOURCE_DIR}/src/*.h"
)
set(DOCS_MAIN_OUTPUT_FILE "${CMAKE_CURRENT_SOURCE_DIR}/luadoc/index.html")
set(LUA_MODULE_PATH
        "${CMAKE_CURRENT_SOURCE_DIR}/tools/ldoc/?.lua;"
        "${CMAKE_CURRENT_SOURCE_DIR}/tools/penlight/lua/?.lua;"
        "${CMAKE_CURRENT_SOURCE_DIR}/tools/penlight/lua/?/init.lua"
)
set(LUA_CMODULE_PATH
        "$<TARGET_FILE_DIR:lfs>/?.dll"
)

add_custom_command(
        OUTPUT ${DOCS_MAIN_OUTPUT_FILE}
        COMMAND ${CMAKE_COMMAND} -E env "LUA_PATH=${LUA_MODULE_PATH}" "LUA_CPATH=${LUA_CMODULE_PATH}"
        $<TARGET_FILE:lua_for_docs>
        "${CMAKE_CURRENT_SOURCE_DIR}/tools/ldoc/ldoc.lua" .
        DEPENDS
        lua_for_docs
        lfs
        ${DOC_SOURCES}
        WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
        COMMENT "Generating Lua documentation with LDoc..."
        VERBATIM
)

add_custom_target(docs
        DEPENDS ${DOCS_MAIN_OUTPUT_FILE}
)

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