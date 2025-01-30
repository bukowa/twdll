cmake_minimum_required(VERSION 3.30)

project(twdll C)

set(CMAKE_C_STANDARD 99)
set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -m32")

# Set Lua's include directory (where lua.h, lauxlib.h, lualib.h are located)
include_directories("luabuiltforrome")  # Ensure this points to the Lua headers for Rome 2

# Set the Lua library for dynamic linking at runtime (only the headers are needed)
# In this case, you should already have a Lua library being loaded by the game, so we link to it dynamically
link_directories("luabuiltforrome")

# Build the shared library (DLL)
add_library(twdll SHARED library.c)

# Link to Lua dynamically, assuming Lua is loaded by the game (no need for version-specific linkage)
target_link_libraries(twdll ${CMAKE_DL_LIBS} lua)
