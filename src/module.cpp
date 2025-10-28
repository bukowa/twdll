#include "module.h"
#include <filesystem>

// Define the global variables that are declared in the header
std::string g_ModuleRootPath;
std::string g_PythonRootPath;

void module_initialize(HMODULE hModule) {
    {
        char buffer[MAX_PATH];
        GetModuleFileNameA(hModule, buffer, MAX_PATH);

        std::filesystem::path modulePath(buffer);
        g_ModuleRootPath = modulePath.parent_path().string();
        g_PythonRootPath = (modulePath.parent_path() / "python").string();

        SetDllDirectoryA(g_PythonRootPath.c_str());
    }
}