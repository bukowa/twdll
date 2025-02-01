cmake_minimum_required(VERSION 3.29)

# Set the project name and language
project(twdll C)

set(CMAKE_C_STANDARD 99)

# Set Visual Studio-specific flags (optional, depending on the version)
set(CMAKE_GENERATOR_PLATFORM x64)  # Set to x86 if needed

# Option to specify Lua version (default 5.1)
option(LUA_VERSION "Lua version to use (e.g., 5.1, 5.4)" "5.1")

# Set Lua include and source directories based on version
if(LUA_VERSION STREQUAL "5.1")
    set(LUA_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/include/lua-ver-5.1/src")
elseif(LUA_VERSION STREQUAL "5.1.1")
set(LUA_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/include/lua-ver-5.1.1/src")
elseif(LUA_VERSION STREQUAL "5.1.2")
    set(LUA_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/include/lua-ver-5.1.2/src")
elseif(LUA_VERSION STREQUAL "5.1.3")
    set(LUA_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/include/lua-ver-5.1.3/src")
elseif(LUA_VERSION STREQUAL "5.1.4")
    set(LUA_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/include/lua-ver-5.1.4/src")
elseif(LUA_VERSION STREQUAL "5.1.5")
    set(LUA_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/include/lua-ver-5.1.5/src")
else()
    message(FATAL_ERROR "Unsupported Lua version: ${LUA_VERSION}")
endif()

message("${LUA_SRC_DIR}")

set(LUA_INCLUDE_DIR "${LUA_SRC_DIR}")
set(LUA_LIB_DIR "${LUA_SRC_DIR}")

# Add Lua source files
file(GLOB LUA_SOURCES "${LUA_SRC_DIR}/*.c")

# Add Lua as a static library (compiled with CL)
add_library(lua STATIC ${LUA_SOURCES})

# Include Lua headers
include_directories(${LUA_INCLUDE_DIR})

# Set the Lua library for dynamic linking at runtime (optional - usually not needed with static linking)
# link_directories(${LUA_LIB_DIR})  # Remove this line for static linking

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
#    target_compile_options(lua PRIVATE "/Wall" "/W3")
#    target_compile_options(twdll PRIVATE "/Wall" "/W3")
endif()

# For multi-configuration generators (e.g., Visual Studio)
if(CMAKE_CONFIGURATION_TYPES MATCHES "Release")
    target_compile_options(lua PRIVATE "/O2" "/DNDEBUG")
    target_compile_options(twdll PRIVATE "/O2" "/DNDEBUG")
endif()

# For multi-configuration generators (e.g., Visual Studio)
if(CMAKE_CONFIGURATION_TYPES MATCHES "Debug")
    # Debugging options for MSVC
    target_compile_options(lua PRIVATE "/Od" "/Z7" "/D_DEBUG")  # Disable optimizations, add debug info
    target_compile_options(twdll PRIVATE "/Od" "/Z7" "/D_DEBUG")
endif()

# Custom command to build and copy the DLL
add_custom_command(
        TARGET twdll POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy
        $<TARGET_FILE:twdll>  # Path to the built DLL
        "C:\\Games\\Total War - Rome 2"  # Destination directory
)

# Define the MOD_FILE_NAME and IMG_FILE_NAME variables
set(MOD_FILE_NAME "libtwdll.pack")
#set(IMG_FILE_NAME "libtwdll.png")

# Get the current working directory
get_filename_component(CURRENT_DIR ${CMAKE_CURRENT_SOURCE_DIR} REALPATH)

# Get the current build directory (where TARGET_FILE resides)
get_filename_component(BUILD_DIR ${CMAKE_BINARY_DIR} REALPATH)

add_custom_target(update_mod
        COMMAND rpfm_cli --game rome_2 pack create --pack-path=${MOD_FILE_NAME}
        COMMAND rpfm_cli --game rome_2 pack add --pack-path=${MOD_FILE_NAME} -F "${CURRENT_DIR}/pack;"
        COMMAND ${CMAKE_COMMAND} -E copy ${MOD_FILE_NAME} "C:\\Games\\Total War - Rome 2\\data"
        BYPRODUCTS ${MOD_FILE_NAME}
)

# Ensure `twdll` depends on `update_mod`
add_dependencies(twdll update_mod)