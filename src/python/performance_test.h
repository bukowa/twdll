#pragma once

#include "lua/lua_api.h"
#include <Python.h>

void register_performance_test_functions(lua_State* L);
void RunPerformanceTests(lua_State* L, char* results_buffer, size_t buffer_size);
void ShowBenchmarkWindow(lua_State* L);
PyMODINIT_FUNC PyInit_engine(void);
