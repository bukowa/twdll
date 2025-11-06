# KROK 1: Stwórz puste listy na "naprawione" ścieżki.
set(TEST_SOURCES "")
set(TEST_INCLUDE_DIRS "")

# KROK 2: Przejdź w pętli po listach z głównego pliku i dodaj prefix.
# CMAKE_SOURCE_DIR to absolutna ścieżka do głównego folderu projektu.
foreach(FILE IN LISTS TWDLL_DOC_SOURCES TWDLL_NON_DOC_SOURCES)
    list(APPEND TEST_SOURCES "${CMAKE_SOURCE_DIR}/${FILE}")
endforeach()

foreach(DIR IN LISTS TWDLL_INCLUDE_DIRS)
    list(APPEND TEST_INCLUDE_DIRS "${CMAKE_SOURCE_DIR}/${DIR}")
endforeach()

# KROK 3: Użyj "naprawionych" list do zbudowania celu testowego.
file(GLOB TEST_CPP_FILES "${CMAKE_CURRENT_SOURCE_DIR}/test_*.cpp")

add_executable(twdll_tests
    ${TEST_CPP_FILES}
    ${TEST_SOURCES}
)

# Build Lua as a static library for testing purposes only
add_library(lua_test_static STATIC
    ${CMAKE_SOURCE_DIR}/include/lua/src/lapi.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lauxlib.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lbaselib.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lcode.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/ldblib.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/ldebug.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/ldo.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/ldump.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lfunc.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lgc.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/linit.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/liolib.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/llex.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lmem.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lobject.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lopcodes.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/loslib.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lparser.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lstate.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lstring.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lstrlib.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/ltable.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/ltablib.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/ltm.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lua.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lundump.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lvm.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lzio.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/lmathlib.c
    ${CMAKE_SOURCE_DIR}/include/lua/src/loadlib.c
)

target_include_directories(lua_test_static PUBLIC
    ${CMAKE_SOURCE_DIR}/include/lua/src
)

# Używamy naszych "naprawionych" list.
target_include_directories(twdll_tests PRIVATE
    ${TEST_INCLUDE_DIRS}
    ${CMAKE_SOURCE_DIR}/include/lua/src
    ${CMAKE_SOURCE_DIR}/src
)

# Używamy zmiennych z głównego pliku, które nie są ścieżkami.
target_link_libraries(twdll_tests PRIVATE
    GTest::gtest_main
    lua_test_static
    "${CMAKE_SOURCE_DIR}/include/minhook/lib/libMinHook.x86.lib"
)
# Jeśli masz TWDLL_LINK_LIBS, dodaj je tutaj też.
# target_link_libraries(twdll_tests PRIVATE ${TWDLL_LINK_LIBS} GTest::gtest_main)

# Dodaj swoje makro.
target_compile_definitions(twdll_tests PRIVATE BUILD_TESTING_LUA)

# Zarejestruj test.
add_test(NAME TwdllUnitTests COMMAND twdll_tests)

target_compile_options(twdll_tests PRIVATE "$<$<CXX_COMPILER_ID:MSVC>:/utf-8>")