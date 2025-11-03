# Pobiera ścieżkę do C/C++ nagłówków Lua z głównego projektu
# include_directories(${LUA_SRC_DIR}) # This line is commented out as we are using target_include_directories instead

# Tworzy bibliotekę 'lfs' (LuaFileSystem) jako bibliotekę współdzieloną (DLL)
add_library(lfs SHARED src/lfs.c)

target_include_directories(lfs PRIVATE ${LUA_SRC_DIR})

# --- TUTAJ JEST NAPRAWA ---
# Zamiast linkować do nieistniejącej biblioteki obiektów 'lua',
# linkujemy do nowej biblioteki współdzielonej 'luadll', którą zdefiniowaliśmy wcześniej.
# To gwarantuje, że lfs.dll będzie używać tej samej, jednej instancji Lua.
target_link_libraries(lfs PRIVATE luadll)

# Definiuje makro preprocesora potrzebne dla zgodności z Twoją wersją Lua.
target_compile_definitions(lfs PRIVATE -DLUA_COMPAT_ALL)