set(LUA_SRC_DIR "${PROJECT_SOURCE_DIR}/docs/lua/include/lua/src")
file(GLOB LUA_CORE_SOURCES "${LUA_SRC_DIR}/*.c")
list(REMOVE_ITEM LUA_CORE_SOURCES 
    "${LUA_SRC_DIR}/lua.c" 
    "${LUA_SRC_DIR}/luac.c"
)

# 1. Build the Lua core as a single Shared Library (DLL) to prevent crashes.
add_library(lua_dll SHARED ${LUA_CORE_SOURCES})
target_include_directories(lua_dll PUBLIC ${LUA_SRC_DIR})
target_compile_definitions(lua_dll PRIVATE LUA_BUILD_AS_DLL PUBLIC LUA_LIB)
set_target_properties(lua_dll PROPERTIES OUTPUT_NAME "lua")

# 2. Build modules linking against the shared DLL.
add_library(luafilesystem MODULE
    "${PROJECT_SOURCE_DIR}/docs/lua/include/luafilesystem/src/lfs.c"
)
set_target_properties(luafilesystem PROPERTIES PREFIX "" OUTPUT_NAME "lfs")
target_link_libraries(luafilesystem PRIVATE lua_dll)

# 3. Build the executable linking against the shared DLL.
add_executable(lua_for_docs "${LUA_SRC_DIR}/lua.c")
target_link_libraries(lua_for_docs PRIVATE lua_dll)

# 4. Collect ONLY the source files that need documentation.
set(ALL_DOC_SOURCES "")
foreach(file ${TWDLL_DOC_SOURCES})
    list(APPEND ALL_DOC_SOURCES "${PROJECT_SOURCE_DIR}/${file}")
endforeach()
file(GLOB_RECURSE DOC_HEADERS "${PROJECT_SOURCE_DIR}/src/*.h")
list(APPEND ALL_DOC_SOURCES ${DOC_HEADERS})

set(LUA_MODULE_PATH
    "${PROJECT_SOURCE_DIR}/docs/lua/ldoc/?.lua;"
    "${PROJECT_SOURCE_DIR}/docs/lua/penlight/lua/?.lua;"
    "${PROJECT_SOURCE_DIR}/docs/lua/penlight/lua/?/init.lua"
)
set(LUA_CMODULE_PATH "$<TARGET_FILE_DIR:luafilesystem>/?.dll")

set(DOCS_OUTPUT_FILE "${PROJECT_SOURCE_DIR}/docs/lua/doc/index.html")

add_custom_command(
    OUTPUT      ${DOCS_OUTPUT_FILE}
    COMMAND     ${CMAKE_COMMAND} -E env 
                "LUA_PATH=${LUA_MODULE_PATH}" 
                "LUA_CPATH=${LUA_CMODULE_PATH}"
                $<TARGET_FILE:lua_for_docs>
                "${PROJECT_SOURCE_DIR}/docs/lua/ldoc/ldoc.lua"
                -c "${PROJECT_SOURCE_DIR}/docs/lua/config.ld"
                # THIS IS THE FIX: Pass the specific list of files, not the whole directory.
                ${ALL_DOC_SOURCES}
    # Set the working directory TO where config.ld is.
    WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/docs/lua"
    DEPENDS     lua_for_docs luafilesystem ${ALL_DOC_SOURCES}
    VERBATIM
)

add_custom_target(docs DEPENDS ${DOCS_OUTPUT_FILE})

set_target_properties(lua_dll luafilesystem lua_for_docs docs PROPERTIES FOLDER "Docs")