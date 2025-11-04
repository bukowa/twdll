set(LUA_SRC_DIR "${PROJECT_SOURCE_DIR}/docs/lua/include/lua/src")
file(GLOB LUA_CORE_SOURCES "${LUA_SRC_DIR}/*.c")
list(REMOVE_ITEM LUA_CORE_SOURCES "${LUA_SRC_DIR}/lua.c" "${LUA_SRC_DIR}/luac.c")

add_library(lua_dll SHARED ${LUA_CORE_SOURCES})
target_include_directories(lua_dll PUBLIC ${LUA_SRC_DIR})
target_compile_definitions(lua_dll PRIVATE LUA_BUILD_AS_DLL PUBLIC LUA_LIB)
set_target_properties(lua_dll PROPERTIES OUTPUT_NAME "lua")

add_library(luafilesystem MODULE "${PROJECT_SOURCE_DIR}/docs/lua/include/luafilesystem/src/lfs.c")
set_target_properties(luafilesystem PROPERTIES PREFIX "" OUTPUT_NAME "lfs")
target_link_libraries(luafilesystem PRIVATE lua_dll)

add_executable(lua_for_docs "${LUA_SRC_DIR}/lua.c")
target_link_libraries(lua_for_docs PRIVATE lua_dll)

set(LUA_MODULE_PATH
    "${PROJECT_SOURCE_DIR}/docs/lua/ldoc/?.lua;"
    "${PROJECT_SOURCE_DIR}/docs/lua/penlight/lua/?.lua;"
    "${PROJECT_SOURCE_DIR}/docs/lua/penlight/lua/?/init.lua"
)
set(LUA_CMODULE_PATH "$<TARGET_FILE_DIR:luafilesystem>/?.dll")
set(DOCS_OUTPUT_FILE "${PROJECT_SOURCE_DIR}/docs/lua/doc/index.html")
set(CONFIG_LD_SOURCE "${PROJECT_SOURCE_DIR}/docs/lua/config.ld")
set(CONFIG_LD_GENERATED "${PROJECT_BINARY_DIR}/config.ld.generated")

set(lua_formatted_files "")
foreach(source_file IN LISTS TWDLL_DOC_SOURCES)
    list(APPEND lua_formatted_files "    \"${source_file}\"")
endforeach()
string(JOIN ",\n" lua_file_list_string "${lua_formatted_files}")

file(READ ${CONFIG_LD_SOURCE} ORIGINAL_CONFIG_CONTENT)
string(REPLACE "file = {}" "file = {\n${lua_file_list_string}\n}" MODIFIED_CONFIG_CONTENT "${ORIGINAL_CONFIG_CONTENT}")
file(WRITE ${CONFIG_LD_GENERATED} "${MODIFIED_CONFIG_CONTENT}")

add_custom_command(
    OUTPUT      ${DOCS_OUTPUT_FILE}
    COMMAND     ${CMAKE_COMMAND} -E env 
                "LUA_PATH=${LUA_MODULE_PATH}" 
                "LUA_CPATH=${LUA_CMODULE_PATH}"
                $<TARGET_FILE:lua_for_docs>
                "${PROJECT_SOURCE_DIR}/docs/lua/ldoc/ldoc.lua"
                -c "${CONFIG_LD_GENERATED}"
                src
    DEPENDS     lua_for_docs luafilesystem ${CONFIG_LD_GENERATED}
    WORKING_DIRECTORY ${PROJECT_SOURCE_DIR}
    VERBATIM
)

add_custom_target(docs DEPENDS ${DOCS_OUTPUT_FILE})

set_target_properties(lua_dll luafilesystem lua_for_docs docs PROPERTIES FOLDER "Docs")