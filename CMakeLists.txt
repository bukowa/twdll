cmake_minimum_required(VERSION 3.29)

# ========================
# Project Config
# ========================
project(libtwdll) # <-- Remove the language here

# Explicitly enable both C and C++ as project languages
enable_language(C CXX)

# Set the standards for each language
set(CMAKE_C_STANDARD 99)
set(CMAKE_CXX_STANDARD 11) # A reasonable standard for modern C++

# ========================
# CLion Environment Setup
# ========================
string(REPLACE ";" ":" CMAKE_PATH_MODIFIED "$ENV{PATH}")
string(REPLACE "C:\\" "/c/" CMAKE_PATH_MODIFIED "${CMAKE_PATH_MODIFIED}")
string(REPLACE "c:\\" "/c/" CMAKE_PATH_MODIFIED "${CMAKE_PATH_MODIFIED}")
file(WRITE cmakeenv "export PATH=\"${CMAKE_PATH_MODIFIED}:$PATH\"\n")
file(APPEND cmakeenv "alias cmake='\"${CMAKE_COMMAND}\"'\n")

# ========================
# Project Variables
# ========================
get_filename_component(CURRENT_DIR ${CMAKE_CURRENT_SOURCE_DIR} REALPATH)

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
include_directories(${LUA_SRC_DIR})

# ========================
# Lua Setup (lua)
# ========================
add_library(lua OBJECT ${LUA_SOURCES})
target_compile_definitions(lua PRIVATE -DLUA_CORE)
target_compile_definitions(lua PRIVATE -DLUA_WIN)

# ========================
# Lua Setup (luadll)
# ========================
add_library(luadll SHARED $<TARGET_OBJECTS:lua>)
set_target_properties(luadll PROPERTIES OUTPUT_NAME "luadll")

# ========================
# Lua Setup (lua.exe)
# ========================
add_executable(lua.exe "${LUA_SRC_DIR}/lua.c")
target_link_libraries(lua.exe PRIVATE $<TARGET_OBJECTS:lua>)
set_target_properties(lua.exe PROPERTIES OUTPUT_NAME "lua")

# ========================
# Lua Setup (luac.exe)
# ========================
add_executable(luac.exe "${LUA_SRC_DIR}/luac.c")
target_link_libraries(luac.exe PRIVATE $<TARGET_OBJECTS:lua>)
set_target_properties(luac.exe PROPERTIES OUTPUT_NAME "luac")
# ========================
# Lib Setup (libtwdll.dll)
# ========================
set(MINHOOK_INCLUDE_DIR ${CMAKE_CURRENT_SOURCE_DIR}/include/minhook/include)
set(MINHOOK_LIBRARY ${CMAKE_CURRENT_SOURCE_DIR}/include/minhook/lib/libMinHook.x86.lib)

add_library(twdll SHARED
        src/libtwdll.cpp
        src/log.cpp
        src/unit_script_interface.cpp
        src/battle_unit_script_interface.cpp
        src/character_script_interface.cpp
        src/character_script_interface.h
        src/log.h
        src/unit_script_interface.h
        src/battle_unit_script_interface.h
)

set_target_properties(twdll PROPERTIES OUTPUT_NAME "libtwdll")

target_include_directories(twdll
        PUBLIC
        src # Its own headers are public
        PRIVATE
        ${MINHOOK_INCLUDE_DIR} # Dependency headers are private
)

target_link_libraries(twdll PRIVATE
        lua # Link to the CMake target
        ${MINHOOK_LIBRARY}
)

# ========================
# LuaSocket Setup (luasocket.dll)
# ========================
set(LUASOCKET_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/include/luasocket/src")
set(LUASOCKET_SOURCES
        luasocket.c timeout.c buffer.c io.c auxiliar.c options.c
        compat.c inet.c except.c select.c tcp.c udp.c compat.c wsocket.c
)
list(TRANSFORM LUASOCKET_SOURCES PREPEND "${LUASOCKET_SRC_DIR}/")

add_library(luasocket SHARED ${LUASOCKET_SOURCES})
target_link_libraries(luasocket PRIVATE $<TARGET_OBJECTS:lua>)
target_link_libraries(luasocket PRIVATE ws2_32.lib)
set_target_properties(luasocket PROPERTIES OUTPUT_NAME "socket")

# ================================
# Mod Files
# ================================
set(MOD_DIR "${CURRENT_DIR}/pack")
set(MOD_PACK "${CMAKE_BINARY_DIR}/libtwdll.pack")
file(GLOB_RECURSE MOD_FILES CONFIGURE_DEPENDS "${MOD_DIR}/*.*")

# ================================
# Mod Setup (libtwdll.pack)
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
set(RELEASE_LUASOCKET_DLL "${RELEASE_DIR}/socket.dll")
set(RELEASE_LUASOCKET_LUA_SOCKET "${RELEASE_DIR}/socket.lua")
set(DOCS_RELEASE_DIR "${RELEASE_DIR}/luadoc")
set(RELEASE_DOCS_STAMP "${RELEASE_DIR}/luadoc.stamp")
set(RELEASE_DLL "${RELEASE_DIR}/libtwdll.dll")
set(RELEASE_PACK "${RELEASE_DIR}/libtwdll.pack")

## ======================================
## Release Setup
## ======================================
add_custom_target(
        build_release ALL
        DEPENDS ${RELEASE_PACK} ${RELEASE_DLL} ${RELEASE_LUA_EXE} ${RELEASE_LUAC_EXE} ${RELEASE_LUA_DLL} ${RELEASE_LUASOCKET_DLL} ${RELEASE_LUASOCKET_LUA_SOCKET} ${RELEASE_DOCS_STAMP}
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
        DEPENDS $<TARGET_FILE:lua.exe>
        OUTPUT ${RELEASE_LUA_EXE}
        COMMAND ${CMAKE_COMMAND} -E copy_if_different $<TARGET_FILE:lua.exe> ${RELEASE_LUA_EXE}
        COMMENT "Copying lua.exe to release directory"
)

add_custom_command(
        DEPENDS $<TARGET_FILE:luac.exe>
        OUTPUT ${RELEASE_LUAC_EXE}
        COMMAND ${CMAKE_COMMAND} -E copy_if_different $<TARGET_FILE:luac.exe> ${RELEASE_LUAC_EXE}
        COMMENT "Copying luac.exe to release directory"
)

add_custom_command(
        DEPENDS $<TARGET_FILE:luadll>
        OUTPUT ${RELEASE_LUA_DLL}
        COMMAND ${CMAKE_COMMAND} -E copy_if_different $<TARGET_FILE:luadll> ${RELEASE_LUA_DLL}
        COMMENT "Copying lua.dll to release directory"
)

add_custom_command(
        DEPENDS $<TARGET_FILE:luasocket>
        OUTPUT ${RELEASE_LUASOCKET_DLL}
        COMMAND ${CMAKE_COMMAND} -E copy_if_different $<TARGET_FILE:luasocket> ${RELEASE_LUASOCKET_DLL}
        COMMENT "Copying socket.dll to release directory"
)

add_custom_command(
        DEPENDS $<TARGET_FILE:luasocket>
        OUTPUT ${RELEASE_LUASOCKET_LUA_SOCKET}
        COMMAND ${CMAKE_COMMAND} -E copy_if_different ${LUASOCKET_SRC_DIR}/socket.lua ${RELEASE_LUASOCKET_LUA_SOCKET}
        COMMENT "Copying socket.lua to release directory"
)

add_custom_command(
        OUTPUT ${RELEASE_DOCS_STAMP}

        # The command to copy the directory.
        COMMAND ${CMAKE_COMMAND} -E copy_directory
        "${CMAKE_CURRENT_SOURCE_DIR}/luadoc" # Source
        ${DOCS_RELEASE_DIR} # Destination

        # After copying, create the stamp file in the release dir.
        COMMAND ${CMAKE_COMMAND} -E touch ${RELEASE_DOCS_STAMP}

        # CRUCIAL: This copy command depends on the 'docs' target.
        # This ensures 'docs' runs first and generates the luadoc directory
        # before this command tries to copy it.
        DEPENDS docs

        COMMENT "Copying documentation to release directory"
        VERBATIM
)

# ===========================================
# Install Files
# ===========================================
option(INSTALL_MOD "Install mod into game dir" OFF)
set(INSTALL_DIR "C:\\Games\\Total War - Rome 2")

get_filename_component(INSTALL_DLL ${RELEASE_DLL} NAME)
set(INSTALL_DLL ${INSTALL_DIR}/${INSTALL_DLL})
option(SHOULD_INSTALL_DLL "Install dll into game dir" ON)

get_filename_component(INSTALL_PACK ${RELEASE_PACK} NAME)
set(INSTALL_PACK ${INSTALL_DIR}/data/${INSTALL_PACK})

# ===========================================
# Install Files
# ===========================================
option(INSTALL_MOD "Install mod into game dir" OFF)

# Define a list of all directories to install to.
# Using forward slashes is recommended in CMake for cross-platform compatibility.
set(INSTALL_DIRS
        "C:/Games/Total War - Rome 2"
        "C:/Program Files (x86)/Steam/steamapps/common/Total War Rome II"
)

# Get just the filenames, which are constant for all installations
get_filename_component(DLL_NAME ${RELEASE_DLL} NAME)
get_filename_component(PACK_NAME ${RELEASE_PACK} NAME)

option(SHOULD_INSTALL_DLL "Install dll into game dir" ON)

# ===========================================
# Install Files Setup
# ===========================================
message("INSTALL_MOD is set to: ${INSTALL_MOD}")
if (INSTALL_MOD)
    # Create a single parent target to group all copy operations
    add_custom_target(install_mod ALL)
    message("SHOULD_INSTALL_DLL is set to: ${SHOULD_INSTALL_DLL}")

    # Loop over each directory in the list
    foreach(CURRENT_DIR IN LISTS INSTALL_DIRS)
        message("Setting up install for directory: ${CURRENT_DIR}")

        # Sanitize the directory path to create a valid and unique target name suffix
        string(MAKE_C_IDENTIFIER "${CURRENT_DIR}" DIR_ID)

        # =============================================================================
        # Install DLL for the current directory
        if (SHOULD_INSTALL_DLL)
            set(INSTALL_DLL "${CURRENT_DIR}/${DLL_NAME}")

            add_custom_command(
                    DEPENDS ${RELEASE_DLL}
                    OUTPUT ${INSTALL_DLL}
                    COMMAND ${CMAKE_COMMAND} -E copy_if_different ${RELEASE_DLL} ${INSTALL_DLL}
                    COMMENT "Copying dll to: ${INSTALL_DLL}"
            )
            # Create a unique target name for this specific copy operation
            add_custom_target(copy_dll_${DIR_ID} ALL DEPENDS ${INSTALL_DLL})
            add_dependencies(install_mod copy_dll_${DIR_ID})
        endif ()

        # =============================================================================
        # Install Pack file for the current directory
        set(INSTALL_PACK "${CURRENT_DIR}/data/${PACK_NAME}")

        add_custom_command(
                DEPENDS ${RELEASE_PACK}
                OUTPUT ${INSTALL_PACK}
                COMMAND ${CMAKE_COMMAND} -E copy_if_different ${RELEASE_PACK} ${INSTALL_PACK}
                COMMENT "Copying pack to: ${INSTALL_PACK}"
        )
        # Create a unique target name for this specific copy operation
        add_custom_target(copy_pack_${DIR_ID} ALL DEPENDS ${INSTALL_PACK})
        add_dependencies(install_mod copy_pack_${DIR_ID})

    endforeach()
endif ()
# ========================
# Compiler Settings
# ========================
add_definitions(-D_CRT_SECURE_NO_WARNINGS)
target_compile_options(lua PRIVATE "/wd4005")

if (MSVC)
    set(CMAKE_C_FLAGS_RELEASE "/O2 /Z7 /DNDEBUG /W4")
    set(CMAKE_C_FLAGS_DEBUG "/Od /Zi /D_DEBUG /W4")
endif()

# ========================
# LuaFileSystem Setup (lfs.dll)
# ========================
add_subdirectory(include/luafilesystem)

# Tworzymy nowy, własny target, którego zadaniem jest skopiowanie lfs.dll.
# Słowo kluczowe 'ALL' sprawia, że ten target będzie częścią domyślnego builda.
add_custom_target(copy_lfs_for_docs ALL
        # Komenda do wykonania: kopiowanie pliku
        COMMAND ${CMAKE_COMMAND} -E copy
        $<TARGET_FILE:lfs>
        ${CMAKE_SOURCE_DIR}/tools/bin/lfs.dll

        # KLUCZOWY MOMENT: Ten target zależy od 'lfs'. CMake zapewni, że 'lfs'
        # zostanie zbudowany ZANIM ta komenda kopiowania się uruchomi.
        DEPENDS lfs

        COMMENT "Kopiowanie lfs.dll do tools/bin..."
)

# ===========================================
# Documentation Generation (ldoc)
# ===========================================

# Gather the source files that LDoc will scan.
file(GLOB_RECURSE DOC_SOURCES
        CONFIGURE_DEPENDS
        "${CMAKE_CURRENT_SOURCE_DIR}/src/*.cpp"
        "${CMAKE_CURRENT_SOURCE_DIR}/src/*.h"
)

# Define the path to the main file that LDoc creates. This is the real output.
set(DOCS_MAIN_OUTPUT_FILE "${CMAKE_CURRENT_SOURCE_DIR}/luadoc/index.html")

# Construct the paths for LUA_PATH and LUA_CPATH.
set(LUA_MODULE_PATH
        "${CMAKE_CURRENT_SOURCE_DIR}/tools/ldoc/?.lua;"
        "${CMAKE_CURRENT_SOURCE_DIR}/tools/penlight/lua/?.lua;"
        "${CMAKE_CURRENT_SOURCE_DIR}/tools/penlight/lua/?/init.lua"
)
set(LUA_CMODULE_PATH "${CMAKE_CURRENT_SOURCE_DIR}/tools/bin/?.dll")

# This command's OUTPUT is the actual file it creates, which fixes the error.
add_custom_command(
        OUTPUT ${DOCS_MAIN_OUTPUT_FILE}

        COMMAND ${CMAKE_COMMAND} -E env
        "LUA_PATH=${LUA_MODULE_PATH}"
        "LUA_CPATH=${LUA_CMODULE_PATH}"
        $<TARGET_FILE:lua.exe>
        "${CMAKE_CURRENT_SOURCE_DIR}/tools/ldoc/ldoc.lua" .

        DEPENDS
        lua.exe
        copy_lfs_for_docs
        ${DOC_SOURCES}

        WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
        COMMENT "Generating Lua documentation with LDoc..."
        VERBATIM
)

# The 'docs' target now depends on the real output file.
add_custom_target(docs
        DEPENDS ${DOCS_MAIN_OUTPUT_FILE}
)