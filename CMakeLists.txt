cmake_minimum_required(VERSION 3.29)

# ========================
# Project Config
# ========================
set(CMAKE_C_STANDARD 99)
project(libtwdll C)
#set(CMAKE_VERBOSE_MAKEFILE ON)

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
add_library(twdll SHARED library.c)
target_link_libraries(twdll PRIVATE $<TARGET_OBJECTS:lua>)
set_target_properties(twdll PROPERTIES OUTPUT_NAME "libtwdll")

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

set(RELEASE_DLL "${RELEASE_DIR}/libtwdll.dll")
set(RELEASE_PACK "${RELEASE_DIR}/libtwdll.pack")

## ======================================
## Release Setup
## ======================================
add_custom_target(
        build_release ALL
        DEPENDS ${RELEASE_PACK} ${RELEASE_DLL} ${RELEASE_LUA_EXE} ${RELEASE_LUAC_EXE} ${RELEASE_LUA_DLL} ${RELEASE_LUASOCKET_DLL} ${RELEASE_LUASOCKET_LUA_SOCKET}
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
# Install Files Setup
# ===========================================
message("INSTALL_MOD is set to: ${INSTALL_MOD}")
if (INSTALL_MOD)
    # =============================================================================
    message("INSTALL_DIR is set to: ${INSTALL_DIR}")
    add_custom_target(install_mod ALL)
    # =============================================================================
    message("SHOULD_INSTALL_DLL is set to: ${SHOULD_INSTALL_DLL}")

    # Create a custom target for copying the DLL, only if it should be installed
    if (SHOULD_INSTALL_DLL)
        add_custom_command(
                DEPENDS ${RELEASE_DLL}
                OUTPUT ${INSTALL_DLL}
                COMMAND ${CMAKE_COMMAND} -E copy ${RELEASE_DLL} ${INSTALL_DLL}
                COMMENT "Copying dll to install dir: ${INSTALL_DLL}"
        )
        add_custom_target(copy_dll ALL DEPENDS ${INSTALL_DLL})
        add_dependencies(install_mod copy_dll)
    endif ()
    # =============================================================================
    # Always copy the pack file
    add_custom_command(
            DEPENDS ${RELEASE_PACK}
            OUTPUT ${INSTALL_PACK}
            COMMAND ${CMAKE_COMMAND} -E copy ${RELEASE_PACK} ${INSTALL_PACK}
            COMMENT "Copying pack to install dir: ${INSTALL_PACK}"
    )
    add_custom_target(copy_pack ALL DEPENDS ${INSTALL_PACK})
    add_dependencies(install_mod copy_pack)
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
