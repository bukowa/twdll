#pragma once
#include <string>
#include <windows.h>

extern std::string g_ModuleRootPath;
extern std::string g_PythonRootPath;

void module_initialize(HMODULE hModule);