# Fetch Lua C/C++ headers path from parent project
# include_directories(${LUA_SRC_DIR})

# Build 'lfs' (LuaFileSystem) as a shared library (DLL)
add_library(lfs SHARED src/lfs.c)

target_include_directories(lfs PRIVATE ${LUA_SRC_DIR})

# Link to shared 'luadll' library to share the single Lua state instance
target_link_libraries(lfs PRIVATE luadll)

# Preprocessor definition for Lua compatibility
target_compile_definitions(lfs PRIVATE -DLUA_COMPAT_ALL)