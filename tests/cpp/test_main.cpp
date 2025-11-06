#include "gtest/gtest.h"

// Dołączamy nagłówek, który zawiera naszą abstrakcję Lua.
// To zadziała, ponieważ ścieżki include są poprawnie ustawione w CMake.
#include "lua/lua_api.h"

// === Test 1: Sprawdzenie, czy możemy stworzyć prawdziwy stan Lua ===
// Ten test się skompiluje i zadziała TYLKO, jeśli BUILD_TESTING_LUA jest aktywne
// i prawdziwe nagłówki Lua są dołączone.
TEST(LuaAbstractions, CanCreateAndDestroyRealLuaState) {

}

// Główna funkcja, która uruchamia wszystkie testy zdefiniowane powyżej.
int main(int argc, char **argv) {
    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}