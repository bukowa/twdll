# This file is included from the main CMakeLists.txt and contains all logic
# for generating the Lua documentation using ldoc.

# ===========================================
# Lua Executable for Documentation
# ===========================================
# We need a lua executable to run the ldoc script.
# This target is only for documentation generation.
set(LUA_SRC_DIR "${PROJECT_SOURCE_DIR}/docs/lua/include/lua/src")
add_executable(lua_for_docs "${LUA_SRC_DIR}/lua.c")
target_include_directories(lua_for_docs PRIVATE ${LUA_SRC_DIR})
target_link_libraries(lua_for_docs PRIVATE luadll luafilesystem)
set_target_properties(lua_for_docs PROPERTIES FOLDER "Docs")

# ========================
# Lua Config
# ========================
set(LUA_SRC_DIR "${PROJECT_SOURCE_DIR}/docs/lua/include/lua/src")
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

target_compile_definitions(luadll
        PRIVATE
        LUA_BUILD_AS_DLL
        LUA_CORE
        LUA_WIN
        PUBLIC
        LUA_LIB
)

target_compile_options(luadll PRIVATE "/wd4005")
target_compile_options(luadll PRIVATE "$<$<CXX_COMPILER_ID:MSVC>:/utf-8>")

# ========================
# LuaFileSystem Setup
# ========================
add_library(luafilesystem SHARED
    ${PROJECT_SOURCE_DIR}/docs/lua/include/luafilesystem/src/lfs.c
)

target_include_directories(luafilesystem PRIVATE
    ${PROJECT_SOURCE_DIR}/docs/lua/include/luafilesystem/src
    ${LUA_SRC_DIR}
)

set_target_properties(luafilesystem PROPERTIES OUTPUT_NAME "lfs")

target_link_libraries(luafilesystem PRIVATE luadll)

# ===========================================
# Documentation Generation (ldoc)
# ===========================================

# Source files to be scanned for documentation comments
file(GLOB_RECURSE DOC_SOURCES
        CONFIGURE_DEPENDS
        "${PROJECT_SOURCE_DIR}/src/*.cpp"
        "${PROJECT_SOURCE_DIR}/src/*.h"
)

# The main output file that ldoc will generate. This serves as the primary
# output for the custom command.
set(DOCS_MAIN_OUTPUT_FILE "${CMAKE_CURRENT_SOURCE_DIR}/html/index.html")

# Configure Lua paths required by the ldoc script.
# LUA_PATH points to ldoc and its dependencies (penlight).
set(LUA_MODULE_PATH
        "${PROJECT_SOURCE_DIR}/docs/lua/ldoc/?.lua;"
        "${PROJECT_SOURCE_DIR}/docs/lua/penlight/lua/?.lua;"
        "${PROJECT_SOURCE_DIR}/docs/lua/penlight/lua/?/init.lua"
)
# LUA_CPATH tells ldoc where to find the compiled twdll.dll so it can be
# introspected for its Lua API.
set(LUA_CMODULE_PATH
        "$<TARGET_FILE_DIR:twdll>/?.dll;"
        "$<TARGET_FILE_DIR:luafilesystem>/?.dll"
)

# Custom command that runs ldoc to generate the documentation.
add_custom_command(
        OUTPUT ${DOCS_MAIN_OUTPUT_FILE}
        COMMAND ${CMAKE_COMMAND} -E env "LUA_PATH=${LUA_MODULE_PATH}" "LUA_CPATH=${LUA_CMODULE_PATH}"
        $<TARGET_FILE:lua_for_docs>
        "${PROJECT_SOURCE_DIR}/docs/lua/ldoc/ldoc.lua" .
        DEPENDS
        lua_for_docs # We need the lua interpreter
        twdll        # We need the compiled DLL for introspection
        ${DOC_SOURCES}
        WORKING_DIRECTORY ${PROJECT_SOURCE_DIR}
        COMMENT "Generating Lua documentation with LDoc..."
        VERBATIM
)

# A custom target to provide a simple way to build the documentation (e.g., `cmake --build . --target docs`)
add_custom_target(docs
        DEPENDS ${DOCS_MAIN_OUTPUT_FILE}
)
set_target_properties(docs PROPERTIES FOLDER "Docs")
