cmake_minimum_required(VERSION 3.30)

project(twdll C)

set(CMAKE_C_STANDARD 99)
set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -m32")

# Set Lua's include directory (where lua.h, lauxlib.h, lualib.h are located)
include_directories("luabuiltforrome")

# Set Lua's lib directory (where lua.lib is located)
link_directories("luabuiltforrome")

# Build the shared library (DLL)
add_library(twdll SHARED library.c)

target_link_libraries(twdll ${CMAKE_DL_LIBS} lua)
