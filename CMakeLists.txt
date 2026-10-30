cmake_minimum_required(VERSION 3.29)
project(libtwdll LANGUAGES C CXX RC)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# ── MinHook ───────────────────────────────────────────────────────────────────
add_subdirectory(vendor/minhook EXCLUDE_FROM_ALL)

if(NOT MSVC)
    message(FATAL_ERROR "MSVC (Visual Studio) compiler required.")
endif()

# ── Game selection ────────────────────────────────────────────────────────────
set(TW_GAME "rome2" CACHE STRING "Target game: rome2 | attila")
set_property(CACHE TW_GAME PROPERTY STRINGS rome2 attila)
message(STATUS "Building for game: ${TW_GAME}")

# ── Source files ──────────────────────────────────────────────────────────────
# Common sources shared across all games
set(TWDLL_COMMON_SOURCES
    src/main.cpp
    src/twdll.rc
    src/common/log.cpp
    src/common/lua_api.cpp
    src/common/lua_core.cpp
    src/common/lua_log.cpp
    src/common/game_api.cpp
    src/common/signature_scanner.cpp
)

# Game-specific sources (struct layouts, Lua signatures)
file(GLOB_RECURSE TWDLL_GAME_SOURCES CONFIGURE_DEPENDS "src/${TW_GAME}/*.cpp")

# ── Library target ────────────────────────────────────────────────────────────
add_library(twdll SHARED
    ${TWDLL_COMMON_SOURCES}
    ${TWDLL_GAME_SOURCES}
)

# ── Git Commit SHA ────────────────────────────────────────────────────────────
execute_process(
    COMMAND git rev-parse HEAD
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    OUTPUT_VARIABLE TWDLL_BUILD_SHA
    OUTPUT_STRIP_TRAILING_WHITESPACE
    ERROR_QUIET
)
if(NOT TWDLL_BUILD_SHA)
    set(TWDLL_BUILD_SHA "unknown")
endif()

target_compile_definitions(twdll PRIVATE
    TWDLL_BUILD_SHA="${TWDLL_BUILD_SHA}"
)

target_include_directories(twdll PRIVATE
    src
    vendor/plog/include
)
target_link_libraries(twdll PRIVATE minhook)

# Add Windows SDK and compiler standard library paths
if(MSVC)
    target_include_directories(twdll PRIVATE
        "$ENV{INCLUDE}"
    )
    # Also need LIB path for linking
    string(REPLACE ";" "\\;" LIB_PATHS "$ENV{LIB}")
    target_link_directories(twdll PRIVATE ${LIB_PATHS})
endif()

set_target_properties(twdll PROPERTIES
    OUTPUT_NAME              "twdll"
    PDB_NAME                 "twdll"
    RUNTIME_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}"
    LIBRARY_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}"
    ARCHIVE_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}"
    PDB_OUTPUT_DIRECTORY     "${CMAKE_BINARY_DIR}"
    DEBUG_INFORMATION_RELEASE TRUE
)

target_compile_options(twdll PRIVATE /Zi /utf-8 /W4)
target_link_options(twdll PRIVATE /DEBUG)
target_compile_definitions(twdll PRIVATE _CRT_SECURE_NO_WARNINGS)

# ── Game preprocessor define ──────────────────────────────────────────────────
if(TW_GAME STREQUAL "attila")
    target_compile_definitions(twdll PRIVATE TW_ATTILA)
elseif(TW_GAME STREQUAL "rome2")
    target_compile_definitions(twdll PRIVATE TW_ROME2)
endif()

# ── Test Dependencies (vendor/tests) ──────────────────────────────────────────
set(LUA51_TEST_SOURCES
    vendor/tests/lua51/src/lapi.c
    vendor/tests/lua51/src/lcode.c
    vendor/tests/lua51/src/ldebug.c
    vendor/tests/lua51/src/ldo.c
    vendor/tests/lua51/src/ldump.c
    vendor/tests/lua51/src/lfunc.c
    vendor/tests/lua51/src/lgc.c
    vendor/tests/lua51/src/llex.c
    vendor/tests/lua51/src/lmem.c
    vendor/tests/lua51/src/lobject.c
    vendor/tests/lua51/src/lopcodes.c
    vendor/tests/lua51/src/lparser.c
    vendor/tests/lua51/src/lstate.c
    vendor/tests/lua51/src/lstring.c
    vendor/tests/lua51/src/ltable.c
    vendor/tests/lua51/src/ltm.c
    vendor/tests/lua51/src/lundump.c
    vendor/tests/lua51/src/lvm.c
    vendor/tests/lua51/src/lzio.c
    vendor/tests/lua51/src/lauxlib.c
    vendor/tests/lua51/src/lbaselib.c
    vendor/tests/lua51/src/ldblib.c
    vendor/tests/lua51/src/liolib.c
    vendor/tests/lua51/src/linit.c
    vendor/tests/lua51/src/lmathlib.c
    vendor/tests/lua51/src/loadlib.c
    vendor/tests/lua51/src/loslib.c
    vendor/tests/lua51/src/lstrlib.c
    vendor/tests/lua51/src/ltablib.c
)
add_library(lua51_test STATIC ${LUA51_TEST_SOURCES})
target_include_directories(lua51_test PUBLIC vendor/tests/lua51/src)
target_compile_definitions(lua51_test PRIVATE _CRT_SECURE_NO_WARNINGS)
if(MSVC)
    target_compile_options(lua51_test PRIVATE /W3 /wd4996 /wd4244 /wd4305)
endif()

# ── Unit Tests (doctest) ──────────────────────────────────────────────────────
enable_testing()
add_executable(twdll_unit_tests
    tests/unit/main.cpp
    tests/unit/common/log_test.cpp
    src/common/log.cpp
)
target_include_directories(twdll_unit_tests PRIVATE
    src
    vendor
    vendor/plog/include
    vendor/tests/doctest
    vendor/tests/lua51/src
)
target_link_libraries(twdll_unit_tests PRIVATE lua51_test)
if(MSVC)
    target_include_directories(twdll_unit_tests PRIVATE "$ENV{INCLUDE}")
    string(REPLACE ";" "\\;" LIB_PATHS "$ENV{LIB}")
    target_link_directories(twdll_unit_tests PRIVATE ${LIB_PATHS})
endif()
target_compile_options(twdll_unit_tests PRIVATE /Zi /utf-8 /W4)
target_compile_definitions(twdll_unit_tests PRIVATE
    _CRT_SECURE_NO_WARNINGS
    BUILD_TESTING_LUA
)

# ── Code Coverage ─────────────────────────────────────────────────────────────
option(ENABLE_COVERAGE "Enable code coverage instrumentation" OFF)

if(ENABLE_COVERAGE)
    if(MSVC)
        target_link_options(twdll_unit_tests PRIVATE /PROFILE)
    else()
        target_compile_options(twdll_unit_tests PRIVATE --coverage)
        target_link_options(twdll_unit_tests PRIVATE --coverage)
    endif()
endif()

add_test(NAME unit_tests COMMAND twdll_unit_tests)

add_custom_target(tw-unit-test
    COMMAND ${CMAKE_CTEST_COMMAND} --output-on-failure
    WORKING_DIRECTORY ${CMAKE_BINARY_DIR}
    DEPENDS twdll_unit_tests
    COMMENT "Running C++ unit tests (ctest)"
)

# ── Task targets (delegate to tools/twdll.ps1) ────────────────────────────────
# Logic lives in the script; these targets exist for IDE integration.
set(TW_PS1 "${CMAKE_SOURCE_DIR}/tools/twdll.ps1")

if(WIN32)
    add_custom_target(test-coverage
        DEPENDS twdll_unit_tests
        COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" coverage "${TW_GAME}"
        COMMENT "Running unit tests with code coverage (${TW_GAME})"
        VERBATIM USES_TERMINAL
    )
else()
    find_program(GCOVR_TOOL gcovr)
    find_program(LLVM_COV_TOOL llvm-cov)
    if(GCOVR_TOOL)
        add_custom_target(test-coverage
            COMMAND $<TARGET_FILE:twdll_unit_tests>
            COMMAND ${GCOVR_TOOL} --root "${CMAKE_SOURCE_DIR}" --print-summary
            WORKING_DIRECTORY "${CMAKE_BINARY_DIR}"
            DEPENDS twdll_unit_tests
            COMMENT "Running unit tests with gcovr coverage"
            USES_TERMINAL
        )
    elseif(LLVM_COV_TOOL)
        add_custom_target(test-coverage
            COMMAND $<TARGET_FILE:twdll_unit_tests>
            WORKING_DIRECTORY "${CMAKE_BINARY_DIR}"
            DEPENDS twdll_unit_tests
            COMMENT "Running unit tests (llvm-cov ready)"
            USES_TERMINAL
        )
    else()
        add_custom_target(test-coverage
            COMMAND ${CMAKE_CTEST_COMMAND} --output-on-failure
            WORKING_DIRECTORY "${CMAKE_BINARY_DIR}"
            DEPENDS twdll_unit_tests
            COMMENT "Running unit tests"
            USES_TERMINAL
        )
    endif()
endif()

add_custom_target(tw-coverage DEPENDS test-coverage)

# ── Task targets (delegate to tools/twdll.ps1) ────────────────────────────────
# Logic lives in the script; these targets exist for IDE integration.
set(TW_PS1 "${CMAKE_SOURCE_DIR}/tools/twdll.ps1")

add_custom_target(tw-pack
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" pack "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Building mod pack (${TW_GAME})"
    VERBATIM
)

add_custom_target(tw-install
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" install "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Installing twdll (${TW_GAME})"
    VERBATIM
)

add_custom_target(tw-run
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" run "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Running game (${TW_GAME})"
    VERBATIM
)

add_custom_target(tw-test
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" test "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Running tests (${TW_GAME})"
    VERBATIM USES_TERMINAL
)

add_custom_target(tw-test-keep
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" test-keep "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Running tests, keeping game alive (${TW_GAME})"
    VERBATIM USES_TERMINAL
)

add_custom_target(tw-test-keep-nosavereload
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" test-keep-nosavereload "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Running tests without save/reload, keeping game alive (${TW_GAME})"
    VERBATIM USES_TERMINAL
)

add_custom_target(tw-tdd
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" tdd "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Running The Dawnless Days campaign (${TW_GAME})"
    VERBATIM USES_TERMINAL
)

add_custom_target(tw-mp-install
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" install-mp "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Installing twdll to multiplayer instances (${TW_GAME})"
    VERBATIM
)

add_custom_target(tw-mp-run
    DEPENDS twdll
    COMMAND powershell -ExecutionPolicy Bypass -File "${TW_PS1}" run-mp "${TW_GAME}" "$<TARGET_FILE:twdll>"
    COMMENT "Running multiplayer instances (${TW_GAME})"
    VERBATIM USES_TERMINAL
)

# ── Documentation targets (defer to docs/CMakeLists.txt) ────────────────────
add_subdirectory(docs)

add_custom_target(tw-docs
    DEPENDS docs
    COMMENT "Building documentation (${TW_GAME})"
)
