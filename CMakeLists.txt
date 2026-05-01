cmake_minimum_required(VERSION 3.29)

# ========================
# Project Config
# ========================
project(libtwdll LANGUAGES C CXX)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_MESSAGE_CONTEXT_SHOW ON)

# ========================
# Game Configuration
# ========================
set(GAME_EXE "Rome2.exe")
set(STEAM_APP_ID "214950")
set(INSTALL_ALONE_DIR "D:/Games/Total War - Rome 2 Steam")
set(INSTALL_STEAM_DIR "C:/Program Files (x86)/Steam/steamapps/common/Total War Rome II")
set(INSTALL_USER_SCRIPT "$ENV{APPDATA}/The Creative Assembly/Rome2/scripts")

# ========================
# Compiler Settings
# ========================
if(NOT MSVC)
    message(FATAL_ERROR "You must use the MSVC (Visual Studio) compiler!")
endif()

add_compile_definitions(_CRT_SECURE_NO_WARNINGS)

set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
set(CMAKE_CXX_FLAGS_RELEASE "/O2 /Zi /DNDEBUG /W4")
set(CMAKE_CXX_FLAGS_DEBUG "/Od /Zi /D_DEBUG /W4")

# ========================
# Lib Setup (twdll.dll)
# ========================

set(TWDLL_DOC_SOURCES
    src/unit.cpp
    src/battle_unit.cpp
    src/character.cpp
    src/faction.cpp
    src/military_force.cpp
)

set(TWDLL_NON_DOC_SOURCES
    src/lua/lua_api.cpp
    src/lua/lua_api.h
    src/main.cpp
    src/log.cpp
    src/script_utils.cpp
    src/signature_scanner.cpp
)

add_library(twdll SHARED
    ${TWDLL_DOC_SOURCES}
    ${TWDLL_NON_DOC_SOURCES}
)

target_include_directories(twdll PRIVATE src)

# Dynamic Steam Build Detection
# If Rome2.dll exists in the standalone directory, it's the old retail version (no steam flag).
# Otherwise, default to STEAM_BUILD (works for both legitimate Steam and torrented Steam versions).
if(EXISTS "${INSTALL_ALONE_DIR}/Rome2.dll")
    message(STATUS "Rome2.dll detected in standalone dir! Building WITHOUT STEAM_BUILD flag.")
    add_custom_command(TARGET twdll PRE_BUILD
        COMMAND ${CMAKE_COMMAND} -E echo "=================================================="
        COMMAND ${CMAKE_COMMAND} -E echo " BUILDING TWDLL (RETAIL BUILD - NO STEAM FLAG)"
        COMMAND ${CMAKE_COMMAND} -E echo "=================================================="
    )
else()
    message(STATUS "Rome2.dll not found in standalone dir. Defaulting to STEAM_BUILD flag.")
    target_compile_definitions(twdll PUBLIC STEAM_BUILD)
    add_custom_command(TARGET twdll PRE_BUILD
        COMMAND ${CMAKE_COMMAND} -E echo "=================================================="
        COMMAND ${CMAKE_COMMAND} -E echo " BUILDING TWDLL (STEAM_BUILD FLAG ENABLED)"
        COMMAND ${CMAKE_COMMAND} -E echo "=================================================="
    )
endif()

target_compile_options(twdll PRIVATE
    "/Zi"
    "/utf-8"
)

set_target_properties(twdll PROPERTIES
    OUTPUT_NAME "twdll"
    PDB_NAME "twdll"
    PDB_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}"
    DEBUG_INFORMATION_RELEASE TRUE
)

target_link_options(twdll PRIVATE "/DEBUG")

# ================================
# Mod Setup (twdll.pack)
# ================================
set(MOD_DIR "${CMAKE_CURRENT_SOURCE_DIR}/src/pack")
set(MOD_PACK "${CMAKE_BINARY_DIR}/twdll.pack")
file(GLOB_RECURSE MOD_FILES CONFIGURE_DEPENDS "${MOD_DIR}/*.*")

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

# ================================
# Test Mod Setup (twdll_test.pack)
# ================================
set(TEST_MOD_DIR "${CMAKE_CURRENT_SOURCE_DIR}/tests/pack")
set(TEST_MOD_PACK "${CMAKE_BINARY_DIR}/twdll_test.pack")
file(GLOB_RECURSE TEST_MOD_FILES CONFIGURE_DEPENDS "${TEST_MOD_DIR}/*.*")

add_custom_target(
        build_test_pack ALL
        DEPENDS ${TEST_MOD_PACK}
)
add_custom_command(
        OUTPUT ${TEST_MOD_PACK}
        COMMAND rpfm_cli --game rome_2 pack create --pack-path=${TEST_MOD_PACK}
        COMMAND rpfm_cli --game rome_2 pack add --pack-path=${TEST_MOD_PACK} -F "${TEST_MOD_DIR};"
        DEPENDS ${TEST_MOD_FILES}
        COMMENT "Building test mod .pack file"
)

# ======================================
# Release Setup
# ======================================
set(RELEASE_DIR "${CMAKE_BINARY_DIR}/release")
file(MAKE_DIRECTORY ${RELEASE_DIR})

add_custom_target(
    build_release ALL
    COMMENT "Assembling release package in ${RELEASE_DIR}"
    COMMAND ${CMAKE_COMMAND} -E copy 
            ${MOD_PACK} 
            ${RELEASE_DIR}/data/twdll.pack

    COMMAND ${CMAKE_COMMAND} -E copy 
            $<TARGET_FILE:twdll> 
            ${RELEASE_DIR}/twdll.dll

    COMMAND ${CMAKE_COMMAND} -E copy_directory
            "${CMAKE_CURRENT_SOURCE_DIR}/docs/lua/doc"
            ${RELEASE_DIR}/twdlldocs
    DEPENDS
        twdll
        docs
        ${MOD_PACK}
    VERBATIM
)

# ===========================================
# Install & Run Targets
# ===========================================
# Generate a PowerShell script to safely tail the log file and auto-exit when the game closes
set(TAIL_SCRIPT "${CMAKE_BINARY_DIR}/tail_log.ps1")
file(WRITE "${TAIL_SCRIPT}" "
Param(\$LogFile)
if (!(Test-Path \$LogFile)) { Start-Sleep -Seconds 1 }
if (!(Test-Path \$LogFile)) { Write-Host 'Log file not found.'; exit }

Write-Host '-- Tailing log in real-time (Event-Driven)...'
Get-Content -Path \$LogFile -Wait -Tail 0 | ForEach-Object {
    Write-Host \$_
    
    if (\$_ -match '\\[TEST\\] Final Result: SUCCESS') {
        Write-Host 'Tests Passed! Auto-closing game...' -ForegroundColor Green
        Stop-Process -Name 'Rome2' -Force -ErrorAction SilentlyContinue
        exit 0
    }
    
    if (\$_ -match '\\[TEST\\] Final Result: FAILED') {
        Write-Host 'Tests Failed! Leaving game open for debugging.' -ForegroundColor Red
        exit 1
    }
}
")

# Custom target to kill the game process
add_custom_target(kill_game
    COMMAND ${CMAKE_COMMAND} -E echo "-- Checking if Rome2 is running..."
    COMMAND powershell -Command "Stop-Process -Name 'Rome2' -Force -ErrorAction SilentlyContinue; while (Get-Process -Name 'Rome2' -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 200 }"
    COMMAND ${CMAKE_COMMAND} -E echo "-- Rome2 process killed."
    COMMENT "Killing Game Process"
    VERBATIM
)

# Macro to generate DRY targets for both Steam and Standalone
macro(create_game_targets PREFIX TARGET_DIR IS_STEAM)
    
    # 1. Install Target
    add_custom_target(install_${PREFIX}_mod
        DEPENDS twdll build_mod_pack kill_game
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Install] Creating data directories at ${TARGET_DIR}..."
        COMMAND ${CMAKE_COMMAND} -E make_directory "${TARGET_DIR}/data"
        
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Install] Copying twdll.pack..."
        COMMAND ${CMAKE_COMMAND} -E copy "${MOD_PACK}" "${TARGET_DIR}/data/"
        
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Install] Copying twdll.dll..."
        COMMAND ${CMAKE_COMMAND} -E copy "$<TARGET_FILE:twdll>" "${TARGET_DIR}/"
        
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Install] Updating user.script.txt..."
        COMMAND ${CMAKE_COMMAND} -E make_directory "${INSTALL_USER_SCRIPT}"
        COMMAND powershell -Command "Set-Content -Path '${INSTALL_USER_SCRIPT}/user.script.txt' -Value 'mod \"twdll.pack\";'"
        COMMENT "Installing Mod (${PREFIX})"
        VERBATIM
    )

    # 1.5. Test Install Target
    add_custom_target(install_${PREFIX}_test
        DEPENDS install_${PREFIX}_mod build_test_pack
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Install Test] Copying pack files..."
        COMMAND ${CMAKE_COMMAND} -E copy "${TEST_MOD_PACK}" "${TARGET_DIR}/data/"
        COMMAND ${CMAKE_COMMAND} -E copy "${CMAKE_CURRENT_SOURCE_DIR}/tests/consulscriptum.pack" "${TARGET_DIR}/data/"
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Install Test] Copying save game..."
        COMMAND ${CMAKE_COMMAND} -E make_directory "$ENV{APPDATA}/The Creative Assembly/Rome2/save_games"
        COMMAND ${CMAKE_COMMAND} -E copy_if_different "${CMAKE_CURRENT_SOURCE_DIR}/tests/tests.save" "$ENV{APPDATA}/The Creative Assembly/Rome2/save_games/tests.save"
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Install Test] Injecting campaign_load into user.script.txt..."
        COMMAND powershell -Command "Set-Content -Path '${INSTALL_USER_SCRIPT}/user.script.txt' -Value 'mod \"twdll_test.pack\";'; Add-Content -Path '${INSTALL_USER_SCRIPT}/user.script.txt' -Value 'mod \"twdll.pack\";'; Add-Content -Path '${INSTALL_USER_SCRIPT}/user.script.txt' -Value 'mod \"consulscriptum.pack\";'; Add-Content -Path '${INSTALL_USER_SCRIPT}/user.script.txt' -Value 'game_startup_mode campaign_load \"tests.save\";'"
        COMMENT "Installing Test Mod (${PREFIX})"
        VERBATIM
    )

    # 2. Run Target (Launch Game Only)
    if(${IS_STEAM})
        set(LAUNCH_CMD powershell -Command "start steam://rungameid/${STEAM_APP_ID}")
    else()
        set(LAUNCH_CMD powershell -WindowStyle Hidden -Command "Start-Process '${GAME_EXE}' -WorkingDirectory '${TARGET_DIR}'")
    endif()

    add_custom_target(run_${PREFIX}
        DEPENDS install_${PREFIX}_mod
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Run] Clearing previous log file..."
        COMMAND powershell -Command "Set-Content -Path '${TARGET_DIR}/twdll.log' -Value ''"
        
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Run] Bypassing outdated mod popup..."
        COMMAND reg add "HKEY_CURRENT_USER\\SOFTWARE\\The Creative Assembly\\Rome2" /v last_game_version /t REG_DWORD /d 0x00020004 /f
        
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Run] Launching game..."
        COMMAND ${LAUNCH_CMD}
        COMMENT "Running Game (${PREFIX})"
        VERBATIM
    )

    # 3. Tail Target (Wait and Stream Logs)
    add_custom_target(run_${PREFIX}_tail
        DEPENDS run_${PREFIX}
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Tail] Connected to ${TARGET_DIR}/twdll.log"
        COMMAND powershell -ExecutionPolicy Bypass -File "${TAIL_SCRIPT}" "${TARGET_DIR}/twdll.log"
        COMMENT "Tailing Logs (${PREFIX})"
        VERBATIM
        USES_TERMINAL
    )

    # 4. Run Test Target
    add_custom_target(run_${PREFIX}_test
        DEPENDS install_${PREFIX}_test
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Run Test] Clearing previous log file..."
        COMMAND powershell -Command "Set-Content -Path '${TARGET_DIR}/twdll.log' -Value ''"
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Run Test] Bypassing outdated mods popup..."
        COMMAND reg add "HKEY_CURRENT_USER\\SOFTWARE\\The Creative Assembly\\Rome2" /v last_game_version /t REG_DWORD /d 0x00020004 /f
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Run Test] Launching Rome2.exe..."
        COMMAND ${LAUNCH_CMD}
        COMMENT "Running Game (${PREFIX})"
        VERBATIM
    )

    # 5. Run Test Target & Tail
    add_custom_target(run_${PREFIX}_test_tail
        DEPENDS run_${PREFIX}_test
        COMMAND ${CMAKE_COMMAND} -E echo "-- [Tail] Connected to ${TARGET_DIR}/twdll.log (Test Mode)"
        COMMAND powershell -ExecutionPolicy Bypass -File "${TAIL_SCRIPT}" "${TARGET_DIR}/twdll.log"
        COMMENT "Tailing Logs (${PREFIX})"
        VERBATIM
        USES_TERMINAL
    )
endmacro()

# Generate targets for Standalone (alone) and Steam (steam)
create_game_targets("alone" "${INSTALL_ALONE_DIR}" FALSE)
create_game_targets("steam" "${INSTALL_STEAM_DIR}" TRUE)

# ===========================================
# Documentation Generation (ldoc)
# ===========================================
add_subdirectory(docs/lua)
