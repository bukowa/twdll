#!/bin/bash

# --- Konfiguracja ---
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
# Używamy ścieżki do folderu build, gdzie CMake tworzy lua.exe
LUA_EXECUTABLE="${SCRIPT_DIR}/cmake-build-clrelease512/lua.exe"
# --- Koniec Konfiguracji ---

if [ ! -f "$LUA_EXECUTABLE" ]; then
    echo "BLAD: Nie znaleziono pliku lua.exe w lokalizacji:"
    echo "$LUA_EXECUTABLE"
    exit 1
fi

echo "Uzywam Lua z: ${LUA_EXECUTABLE}"

# --- Ustawienie ścieżek dla bibliotek Lua ---
SCRIPT_DIR_WIN=$(cygpath -w "$SCRIPT_DIR")

# 1. Ścieżka do modułów LUA (.lua)
export LUA_PATH="${SCRIPT_DIR_WIN}\\tools\\ldoc\\?.lua;${SCRIPT_DIR_WIN}\\tools\\penlight\\lua\\?.lua;${SCRIPT_DIR_WIN}\\tools\\penlight\\lua\\?\\init.lua"
echo "LUA_PATH ustawiony na: ${LUA_PATH}"

# 2. KLUCZOWY BRAKUJĄCY ELEMENT: Ścieżka do modułów C (.dll)
#    Wskazujemy na folder 'tools/bin', gdzie CMake skopiował lfs.dll.
export LUA_CPATH="${SCRIPT_DIR_WIN}\\tools\\bin\\?.dll"
echo "LUA_CPATH ustawiony na: ${LUA_CPATH}"
echo ""

# --- Uruchomienie LDoc ---
"${LUA_EXECUTABLE}" "${SCRIPT_DIR}/tools/ldoc/ldoc.lua" .

echo ""
echo "Dokumentacja wygenerowana!"