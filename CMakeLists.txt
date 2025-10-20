cmake_minimum_required(VERSION 3.29)

# ========================
# Project Config
# ========================
project(libtwdll)

enable_language(C CXX)

set(CMAKE_C_STANDARD 99)
set(CMAKE_CXX_STANDARD 11)

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
# Lua Setup (luadll)
# ========================
add_library(luadll SHARED ${LUA_SOURCES})
set_target_properties(luadll PROPERTIES OUTPUT_NAME "lua")

add_executable(lua_for_docs "${LUA_SRC_DIR}/lua.c")
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
target_compile_definitions(lua.exe PRIVATE LUA_CORE LUA_WIN)
set_target_properties(lua.exe PROPERTIES OUTPUT_NAME "lua")

# ========================
# Lua Setup (luac.exe)
# ========================
add_executable(luac.exe "${LUA_SRC_DIR}/luac.c" ${LUA_SOURCES})
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
        src/character_script_interface.h
        src/faction_script_interface.h
        src/log.h
        src/unit_script_interface.h
        src/script_utils.cpp
        src/script_utils.h
        src/battle_unit_script_interface.h
)
set_target_properties(twdll PROPERTIES OUTPUT_NAME "twdll")

target_include_directories(twdll
        PUBLIC
        src
        PRIVATE
        ${MINHOOK_INCLUDE_DIR}
)

target_link_libraries(twdll PRIVATE
        luadll
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
target_link_libraries(luasocket PRIVATE luadll)
target_link_libraries(luasocket PRIVATE ws2_32.lib)
set_target_properties(luasocket PROPERTIES OUTPUT_NAME "socket")

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
set(RELEASE_LUASOCKET_DLL "${RELEASE_DIR}/socket.dll")
set(RELEASE_LUASOCKET_LUA_SOCKET "${RELEASE_DIR}/socket.lua")
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
        "$<TARGET_FILE_DIR:lfs>/?.dll;"
        "$<TARGET_FILE_DIR:luasocket>/?.dll"
)

add_custom_command(
        OUTPUT ${DOCS_MAIN_OUTPUT_FILE}
        COMMAND ${CMAKE_COMMAND} -E env "LUA_PATH=${LUA_MODULE_PATH}" "LUA_CPATH=${LUA_CMODULE_PATH}"
        $<TARGET_FILE:lua_for_docs>
        "${CMAKE_CURRENT_SOURCE_DIR}/tools/ldoc/ldoc.lua" .
        DEPENDS
        lua_for_docs
        lfs
        luasocket
        ${DOC_SOURCES}
        WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
        COMMENT "Generating Lua documentation with LDoc..."
        VERBATIM
)

add_custom_target(docs
        DEPENDS ${DOCS_MAIN_OUTPUT_FILE}
)
