# --- CMakeLists.txt dla luafilesystem (wersja dla Twojego projektu) ---

cmake_minimum_required(VERSION 3.10)
project(LuaFileSystem C)

# Znajduje pliki źródłowe .c w folderze src
file(GLOB LFS_SOURCES "src/*.c")

# Tworzy bibliotekę .dll (SHARED)
add_library(lfs SHARED ${LFS_SOURCES})

# Definicja preprocesora wymagana do poprawnego budowania lfs.dll w Windows
target_compile_definitions(lfs PRIVATE LFS_LIB)

# --- NAJWAŻNIEJSZE ZMIANY ---

# 1. Dodajemy ścieżkę do nagłówków Lua, korzystając z Twojego targetu `lua`
#    CMake automatycznie przekaże include directories z targetu `lua`.
# 2. Linkujemy z obiektami Lua, które już skompilowałeś w głównym projekcie.
#    To jest dokładnie ten sam mechanizm, którego używasz dla `luasocket`.
target_link_libraries(lfs PRIVATE $<TARGET_OBJECTS:lua>)