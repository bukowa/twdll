cmake_minimum_required(VERSION 3.29)

# Set the project name and language
project(twdll C)

set(CMAKE_C_STANDARD 99)

# Set Visual Studio-specific flags (optional, depending on the version)
set(CMAKE_GENERATOR_PLATFORM x64)  # Set to x86 if needed

# Set Lua include and source directories
set(LUA_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/lua-5.1.5/src")
set(LUA_INCLUDE_DIR "${LUA_SRC_DIR}")
set(LUA_LIB_DIR "${LUA_SRC_DIR}")

# Add Lua source files
file(GLOB LUA_SOURCES "${LUA_SRC_DIR}/*.c")

# Add Lua as a static library (compiled with CL)
add_library(lua STATIC ${LUA_SOURCES})

# Include Lua headers
include_directories(${LUA_INCLUDE_DIR})

# Set the Lua library for dynamic linking at runtime (optional)
link_directories(${LUA_LIB_DIR})

# Build your own DLL (twdll)
add_library(twdll SHARED library.c)

# Link your DLL to Lua (this is the Lua static library we just built)
target_link_libraries(twdll lua ${CMAKE_DL_LIBS})

# Output settings for DLL
set_target_properties(twdll PROPERTIES
        OUTPUT_NAME "libtwdll"  # The name of the DLL without any extension
        PREFIX ""              # Prevents the toolchain from adding a prefix (like msys-)
        SUFFIX ".dll"          # Ensures the correct extension
        LIBRARY_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/bin"  # Outputs to 'bin' folder
)

# Set custom build steps to ensure Lua is compiled before your project
add_dependencies(twdll lua)

# If you want to ensure CL-specific configuration, you can set specific flags
if(MSVC)
    target_compile_options(lua PRIVATE "/Wall" "/W3")
    target_compile_options(twdll PRIVATE "/Wall" "/W3")
endif()

# Custom command to copy the built DLL after build
add_custom_command(
        TARGET twdll POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy
        $<TARGET_FILE:twdll>  # Path to the built DLL
        "C:\\Program Files (x86)\\Steam\\steamapps\\common\\Total War Rome II"  # Destination directory
)
