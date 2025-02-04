cmake_minimum_required(VERSION 3.29)

# ========================
# Set C Standard and Project Info
# ========================

set(CMAKE_C_STANDARD 99)
project(libtwdll C)

# ========================
# Lua Configuration
# ========================

# Set the Lua version and directory containing Lua source files
set(LUA_SRC_DIR "${CMAKE_CURRENT_SOURCE_DIR}/include/lua-ver-${LUA_VERSION}/src")

# Check if Lua source directory exists, otherwise throw an error
if(NOT EXISTS "${LUA_SRC_DIR}")
    message(FATAL_ERROR "Lua version ${LUA_VERSION} not found in ${LUA_SRC_DIR}")
endif()

# Print the Lua Version
message(STATUS "LUA_VERSION: " ${LUA_VERSION})

# Collect Lua source files (*.c files in the Lua source directory)
file(GLOB LUA_SOURCES "${LUA_SRC_DIR}/*.c")

# Add Lua source directory to include path
include_directories(${LUA_SRC_DIR})

# Add Lua as a static library to the project
add_library(lua STATIC ${LUA_SOURCES})

# ========================
# DLL Setup (twdll)
# ========================

# Build the shared DLL (twdll) from the library source file (library.c)
add_library(twdll SHARED library.c)

# Set the output name of the DLL to be "libtwdll"
set_target_properties(twdll PROPERTIES OUTPUT_NAME "libtwdll")

# Set a dependency between the twdll and lua libraries to ensure Lua is built before twdll
add_dependencies(twdll lua)

# Link twdll to the Lua static library
target_link_libraries(twdll lua)

# ========================
# Lua Setup (lua.exe)
# ========================
add_executable(lua.exe "${LUA_SRC_DIR}/lua.c")
target_link_libraries(lua.exe PRIVATE lua)
set_target_properties(lua.exe PROPERTIES OUTPUT_NAME "lua")

# ========================
# Lua Setup (luac.exe)
# ========================
add_executable(luac.exe "${LUA_SRC_DIR}/luac.c")
target_link_libraries(luac.exe PRIVATE lua)
set_target_properties(luac.exe PROPERTIES OUTPUT_NAME "luac")

# ========================
# Compiler Settings
# ========================

# Compiler settings based on the build type (Release or Debug)
if(CMAKE_BUILD_TYPE STREQUAL "Release")
    target_compile_options(lua PRIVATE "/O2" "/Z7" "/DNDEBUG")
    target_compile_options(twdll PRIVATE "/O2" "/Z7" "/DNDEBUG")
elseif(CMAKE_BUILD_TYPE STREQUAL "Debug")
    target_compile_options(lua PRIVATE "/Od" "/Z7" "/D_DEBUG /Gz")
    target_compile_options(twdll PRIVATE "/Od" "/Z7" "/D_DEBUG /Gz")
endif()

# ========================
# Custom Post-Build Command (Copy DLL)
# ========================

# After building twdll, copy the DLL to the specified target directory
add_custom_command(
        TARGET twdll POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy
        $<TARGET_FILE:twdll>  # Path to the built DLL
        "C:\\Games\\Total War - Rome 2"  # Destination directory
        COMMENT "Copying libtwdll DLL to target directory..."
)

# ========================
# Mod File Setup
# ========================

# Define the path where the mod file will be generated
set(MOD_FILE_PATH "${CMAKE_BINARY_DIR}/libtwdll.pack")

# Get the absolute path of the current source directory
get_filename_component(CURRENT_DIR ${CMAKE_CURRENT_SOURCE_DIR} REALPATH)

# Specify the directory containing files for the mod (these files will be included in the mod)
set(MOD_WATCH_DIR "${CURRENT_DIR}/pack")

# Collect all files in the MOD_WATCH_DIR recursively (files that will be added to the mod)
set(MOD_FILES
        "${MOD_WATCH_DIR}/lua_scripts/all_scripted.lua"
        "${MOD_WATCH_DIR}/script/_lib/lib_logging.lua"
        "${MOD_WATCH_DIR}/script/_lib/lib_twdll.lua")

# ========================
# Mod File Generation Command
# ========================

# Add a custom command to generate the mod pack
# This command runs if any files in MOD_FILES change
add_custom_command(
        OUTPUT ${MOD_FILE_PATH}  # The output mod pack file
        COMMAND rpfm_cli --game rome_2 pack create --pack-path=${MOD_FILE_PATH}  # Create the mod pack
        COMMAND rpfm_cli --game rome_2 pack add --pack-path=${MOD_FILE_PATH} -F "${MOD_WATCH_DIR};"  # Add files to the mod pack
        COMMAND ${CMAKE_COMMAND} -E copy ${MOD_FILE_PATH} "C:\\Games\\Total War - Rome 2\\data"  # Copy the mod pack to the game directory
        DEPENDS ${MOD_FILES}  # The command runs only if any of the files in MOD_FILES change
        COMMENT "Generating mod pack file..."
)

# ========================
# Custom Target for Mod File Creation
# ========================

# Create a custom target that will trigger the mod generation command
add_custom_target(create_mod_pack
        DEPENDS ${MOD_FILE_PATH}  # Ensure this target runs only if the MOD file is up-to-date
)

# Ensure the twdll target depends on create_mod_pack so it builds after the mod file is ready
add_dependencies(twdll create_mod_pack)

# ========================
# Output Directory Setup
# ========================

# Define the directory where the mod file and DLL will be copied
set(MOD_TARGET_DIR "${CMAKE_BINARY_DIR}/mod")

# Ensure the target directory exists (create it if it doesn't)
file(MAKE_DIRECTORY ${MOD_TARGET_DIR})

# ========================
# Copy Files to Output Directory
# ========================

# After building twdll, copy both the mod file and the DLL to the output directory
add_custom_command(
        TARGET twdll POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy ${MOD_FILE_PATH} ${MOD_TARGET_DIR}  # Copy the mod pack
        COMMAND ${CMAKE_COMMAND} -E copy $<TARGET_FILE:twdll> ${MOD_TARGET_DIR}  # Copy the built DLL
        COMMENT "Copying MOD file and libtwdll DLL to output directory..."
)
