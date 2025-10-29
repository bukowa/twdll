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
)

set_target_properties(twdll PROPERTIES OUTPUT_NAME "twdll")
set_target_properties(twdll PROPERTIES DEBUG_INFORMATION_RELEASE TRUE)

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
        delayimp
)

if(MSVC)
    # Get the base names of both release and debug libraries
    get_filename_component(PYTHON_LIB_NAME_RELEASE ${PYTHON_LIBRARY_RELEASE} NAME_WE)
    get_filename_component(PYTHON_LIB_NAME_DEBUG   ${PYTHON_LIBRARY_DEBUG}   NAME_WE)

    # Construct the corresponding DLL filenames
    set(PYTHON_DLL_FILENAME_RELEASE "${PYTHON_LIB_NAME_RELEASE}.dll") # e.g., "python313.dll"
    set(PYTHON_DLL_FILENAME_DEBUG   "${PYTHON_LIB_NAME_DEBUG}.dll")   # e.g., "python313_d.dll"

    message(STATUS "Configuring delay-load for Release: ${PYTHON_DLL_FILENAME_RELEASE}")
    message(STATUS "Configuring delay-load for Debug:   ${PYTHON_DLL_FILENAME_DEBUG}")
    
    # Use a generator expression to select the correct DLL at build time
    # For a "Debug" build, it becomes "/DELAYLOAD:python313_d.dll"
    # For a "Release" build, it becomes "/DELAYLOAD:python313.dll"
    target_link_options(twdll PRIVATE 
        "$<$<CONFIG:Debug>:/DELAYLOAD:${PYTHON_DLL_FILENAME_DEBUG}>"
        "$<$<NOT:$<CONFIG:Debug>>:/DELAYLOAD:${PYTHON_DLL_FILENAME_RELEASE}>"
    )
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

set(RELEASE_LUA_EXE "${RELEASE_DIR}/lua.exe")
set(RELEASE_LUAC_EXE "${RELEASE_DIR}/luac.exe")
set(RELEASE_LUA_DLL "${RELEASE_DIR}/lua.dll")
set(DOCS_RELEASE_DIR "${RELEASE_DIR}/luadoc")
set(RELEASE_DOCS_STAMP "${RELEASE_DIR}/luadoc.stamp")
set(RELEASE_DLL "${RELEASE_DIR}/twdll.dll")
set(RELEASE_PACK "${RELEASE_DIR}/twdll.pack")

# ======================================
# Release Setup
# ======================================
add_custom_target(
        build_release ALL
        DEPENDS
        ${RELEASE_DLL}
        ${RELEASE_LUA_DLL}
        ${RELEASE_DOCS_STAMP}
        docs
)
add_custom_command(
        DEPENDS ${MOD_PACK}
        OUTPUT ${RELEASE_PACK}
        COMMAND ${CMAKE_COMMAND} -E copy_if_different ${MOD_PACK} ${RELEASE_PACK}
        COMMENT "Copying pack to release directory"
)
add_custom_command(
        DEPENDS $<TARGET_FILE:twdll>
        OUTPUT ${RELEASE_DLL}
        COMMAND ${CMAKE_COMMAND} -E copy_if_different $<TARGET_FILE:twdll> ${RELEASE_DLL}
        COMMENT "Copying dll to release directory"
)
add_custom_command(
        DEPENDS $<TARGET_FILE:luadll>
        OUTPUT ${RELEASE_LUA_DLL}
        COMMAND ${CMAKE_COMMAND} -E copy_if_different $<TARGET_FILE:luadll> ${RELEASE_LUA_DLL}
        COMMENT "Copying lua.dll to release directory"
)

add_custom_command(
        TARGET build_release POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy_directory ${CMAKE_CURRENT_SOURCE_DIR}/include/python/embed ${RELEASE_DIR}/python
        COMMENT "Copying Python distribution to release directory"
)

add_custom_command(
        OUTPUT ${RELEASE_DOCS_STAMP}
        COMMAND ${CMAKE_COMMAND} -E copy_directory
        "${CMAKE_CURRENT_SOURCE_DIR}/luadoc"
        ${DOCS_RELEASE_DIR}
        COMMAND ${CMAKE_COMMAND} -E touch ${RELEASE_DOCS_STAMP}
        DEPENDS docs
        COMMENT "Copying documentation to release directory"
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

get_filename_component(DLL_NAME ${RELEASE_DLL} NAME)
get_filename_component(PACK_NAME ${RELEASE_PACK} NAME)
get_filename_component(LUA_DLL_NAME ${RELEASE_LUA_DLL} NAME)

option(SHOULD_INSTALL_DLL "Install dll into game dir" ON)

# ===========================================
# Install Files Setup
# ===========================================
message("INSTALL_MOD is set to: ${INSTALL_MOD}")
if (INSTALL_MOD)
    add_custom_target(install_mod ALL)
    add_dependencies(install_mod kill_game)

    message("SHOULD_INSTALL_DLL is set to: ${SHOULD_INSTALL_DLL}")

    set(DIR_COUNTER 0)

    foreach(CURRENT_DIR IN LISTS INSTALL_DIRS)
        message("Setting up install for directory: ${CURRENT_DIR}")

        set(DIR_ID ${DIR_COUNTER})

        if (SHOULD_INSTALL_DLL)
            set(INSTALL_DLL "${CURRENT_DIR}/${DLL_NAME}")
            add_custom_command(
                    DEPENDS ${RELEASE_DLL} OUTPUT ${INSTALL_DLL}
                    COMMAND ${CMAKE_COMMAND} -E copy_if_different ${RELEASE_DLL} ${INSTALL_DLL}
                    COMMENT "Copying dll to: ${INSTALL_DLL}"
            )
            add_custom_target(copy_dll_${DIR_ID} ALL DEPENDS ${INSTALL_DLL})
            add_dependencies(install_mod copy_dll_${DIR_ID})

            set(INSTALL_LUA_DLL "${CURRENT_DIR}/${LUA_DLL_NAME}")
            add_custom_command(
                    DEPENDS ${RELEASE_LUA_DLL} OUTPUT ${INSTALL_LUA_DLL}
                    COMMAND ${CMAKE_COMMAND} -E copy_if_different ${RELEASE_LUA_DLL} ${INSTALL_LUA_DLL}
                    COMMENT "Copying lua.dll to: ${INSTALL_LUA_DLL}"
            )
            add_custom_target(copy_lua_dll_${DIR_ID} ALL DEPENDS ${INSTALL_LUA_DLL})
            add_dependencies(install_mod copy_lua_dll_${DIR_ID})

            add_custom_command(
                    TARGET install_mod POST_BUILD
                    COMMAND ${CMAKE_COMMAND} -E copy_directory ${CMAKE_CURRENT_SOURCE_DIR}/include/python/embed ${CURRENT_DIR}/python
                    COMMENT "Copying Python distribution to game directory"
            )
        endif ()

        set(INSTALL_PACK "${CURRENT_DIR}/data/${PACK_NAME}")
        add_custom_command(
                DEPENDS ${RELEASE_PACK} OUTPUT ${INSTALL_PACK}
                COMMAND ${CMAKE_COMMAND} -E copy_if_different ${RELEASE_PACK} ${INSTALL_PACK}
                COMMENT "Copying pack to: ${INSTALL_PACK}"
        )
        add_custom_target(copy_pack_${DIR_ID} ALL DEPENDS ${INSTALL_PACK})
        add_dependencies(install_mod copy_pack_${DIR_ID})

        set(TESTS_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/tests")
        if(EXISTS "${TESTS_SRC_DIR}")
            set(INSTALL_TESTS_DIR "${CURRENT_DIR}/tests")
            set(INSTALL_TESTS_STAMP "${INSTALL_TESTS_DIR}/.cmake_stamp")
            file(GLOB_RECURSE TEST_FILES CONFIGURE_DEPENDS "${TESTS_SRC_DIR}/*")

            add_custom_command(
                    OUTPUT ${INSTALL_TESTS_STAMP}
                    COMMAND ${CMAKE_COMMAND} -E remove_directory "${INSTALL_TESTS_DIR}"
                    COMMAND ${CMAKE_COMMAND} -E copy_directory "${TESTS_SRC_DIR}" "${INSTALL_TESTS_DIR}"
                    COMMAND ${CMAKE_COMMAND} -E touch ${INSTALL_TESTS_STAMP}
                    DEPENDS ${TEST_FILES}
                    COMMENT "Checking/copying tests to: ${INSTALL_TESTS_DIR}"
                    VERBATIM
            )

            add_custom_target(copy_tests_${DIR_ID} ALL DEPENDS ${INSTALL_TESTS_STAMP})

            add_dependencies(install_mod copy_tests_${DIR_ID})
        endif()

        math(EXPR DIR_COUNTER "${DIR_COUNTER} + 1")
    endforeach()
endif ()
# ========================
# Compiler Settings
# ========================
add_definitions(-D_CRT_SECURE_NO_WARNINGS)
target_compile_options(luadll PRIVATE "/wd4005")

if (MSVC)
    set(CMAKE_C_FLAGS_RELEASE "/O2 /Z7 /DNDEBUG /W4")
    set(CMAKE_C_FLAGS_DEBUG "/Od /Zi /D_DEBUG /W4")
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