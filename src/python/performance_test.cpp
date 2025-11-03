#include "performance_test.h"

#include <chrono>
#include <sstream>
#include "imgui.h"
#include "log.h"
#include "lua_api.h"
#include "object.h"

// =================================================================================
// Lua Performance Test Functions
// =================================================================================

// This is our C++ function that does nothing. It has the required signature for Lua.
static int lua_api_do_nothing(lua_State* L) {
    // We don't do anything with the Lua state.
    // We return 0 to tell Lua that this function returns zero values.
    return 0;
}

// This function will register our custom functions into the game's Lua state.
void register_performance_test_functions(lua_State* L) {
    if (!L) {
        Log("ERROR: Cannot register custom Lua functions, lua_State is NULL.");
        return;
    }
    if (!g_game_lua_pushcclosure || !g_game_lua_setfield) {
        Log("ERROR: Cannot register custom Lua functions, required API functions are missing.");
        return;
    }

    Log("Registering custom C++ function 'do_nothing_fast' into game's Lua state...");

    g_game_lua_pushcclosure(L, lua_api_do_nothing, 0);
    g_game_lua_setfield(L, -10002, "do_nothing_fast");

    Log("Custom Lua function registered successfully.");
}

// =================================================================================
// Python Benchmark Setup (Modified for Delay Loading)
// =================================================================================

static void api_do_nothing() {}

static PyObject* py_api_do_nothing(PyObject* self, PyObject* args) {
    api_do_nothing();
    return Py_None;
}

static PyMethodDef EngineMethods[] = {
    {"do_nothing", py_api_do_nothing, METH_NOARGS, ""},
    {NULL, NULL, 0, NULL}
};

static struct PyModuleDef engine_module = {
    PyModuleDef_HEAD_INIT, "engine", NULL, -1, EngineMethods
};

PyMODINIT_FUNC PyInit_engine(void) {
    return PyModule_Create(&engine_module);
}

// =================================================================================
// Helper to run a Lua string using the GAME'S functions via luaB_loadstring
// =================================================================================

void RunGameLuaString(lua_State* L, const char* script) {
    if (!g_game_luaB_loadstring || !g_game_lua_pushcclosure || !g_game_lua_pushstring || !g_game_lua_pcall) {
        Log("ERROR: A required Lua function for RunGameLuaString is missing!");
        return;
    }
    g_game_lua_pushcclosure(L, g_game_luaB_loadstring, 0);
    g_game_lua_pushstring(L, script);
    if (g_game_lua_pcall(L, 1, 1, 0) != 0) {
        Log("ERROR: pcall failed during script compilation phase.");
        return;
    }
    if (g_game_lua_pcall(L, 0, 0, 0) != 0) {
        Log("ERROR: pcall failed during script execution phase.");
        return;
    }
}

// =================================================================================
// The Benchmark Runner
// =================================================================================

void RunPerformanceTests(lua_State* L, char* results_buffer, size_t buffer_size) {
    std::stringstream ss;
    ss << "Running benchmarks...\n\n";

    // --- Test 1: PI Calculation (CPU & Math Intensive) ---
    const int pi_iterations = 5000000;
    ss << "--- Test 1: PI Calculation (" << pi_iterations << " iterations) ---\n";
    ss << "Measures raw CPU and floating-point math performance.\n";

    const char* python_pi_calc =
        "pi = 0.0\n"
        "sign = 1.0\n"
        "for i in range(5000000):\n"
        "    term = 1.0 / (2.0 * i + 1.0)\n"
        "    pi += sign * term\n"
        "    sign = -sign\n"
        "pi *= 4.0\n";
    auto start = std::chrono::high_resolution_clock::now();
    PyRun_SimpleString(python_pi_calc);
    auto end = std::chrono::high_resolution_clock::now();
    std::chrono::duration<double, std::milli> python_duration = end - start;
    ss << "Python (Embedded):  " << python_duration.count() << " ms\n";

    const char* lua_pi_calc =
        "local pi = 0.0\n"
        "local sign = 1.0\n"
        "for i = 0, 4999999 do\n"
        "    local term = 1.0 / (2.0 * i + 1.0)\n"
        "    pi = pi + sign * term\n"
        "    sign = -sign\n"
        "end\n"
        "pi = pi * 4.0\n";
    start = std::chrono::high_resolution_clock::now();
    RunGameLuaString(L, lua_pi_calc);
    end = std::chrono::high_resolution_clock::now();
    std::chrono::duration<double, std::milli> lua_duration = end - start;
    ss << "Lua (Game's Internal): " << lua_duration.count() << " ms\n\n";

    // --- Test 2: C++ Function Calls (FFI Overhead) ---
    const int ffi_call_iterations = 2000000;
    ss << "--- Test 2: C++ Function Calls (" << ffi_call_iterations << " iterations) ---\n";
    ss << "Measures the cost of calling C++ from the script.\n";

    const char* python_ffi_calls = "import engine\nfor i in range(2000000): engine.do_nothing()";
    start = std::chrono::high_resolution_clock::now();
    PyRun_SimpleString(python_ffi_calls);
    end = std::chrono::high_resolution_clock::now();
    python_duration = end - start;
    ss << "Python (Embedded):  " << python_duration.count() << " ms\n";

    const char* lua_ffi_calls = "for i = 1, 2000000 do do_nothing_fast() end";

    start = std::chrono::high_resolution_clock::now();
    RunGameLuaString(L, lua_ffi_calls);
    end = std::chrono::high_resolution_clock::now();
    lua_duration = end - start;
    ss << "Lua (Game's Internal): " << lua_duration.count() << " ms\n\n";

    strncpy_s(results_buffer, buffer_size, ss.str().c_str(), _TRUNCATE);
}

// =================================================================================
// The ImGui Window
// =================================================================================

void ShowBenchmarkWindow(lua_State* L) {
    ImGui::Begin("Performance Benchmark: Python vs. Game's Lua");
    ImGui::Text("Click the button to run a series of performance tests.");
    ImGui::Text("IMPORTANT: Compile in RELEASE mode for accurate results!");
    if (L == nullptr) {
        ImGui::TextColored(ImVec4(1.0f, 0.2f, 0.2f, 1.0f), "ERROR: Game's lua_State* has not been found!");
    } else {
        ImGui::TextColored(ImVec4(0.2f, 1.0f, 0.2f, 1.0f), "SUCCESS: Game's lua_State* is loaded.");
    }
    ImGui::Separator();
    static char results[4096] = "Results will be shown here.";
    if (L == nullptr) ImGui::BeginDisabled();
    if (ImGui::Button("Run Benchmark")) {
        strcpy_s(results, "Running...");
        RunPerformanceTests(L, results, sizeof(results));
    }
    if (L == nullptr) ImGui::EndDisabled();
    ImGui::Separator();
    ImGui::InputTextMultiline("##Results", results, IM_ARRAYSIZE(results), ImVec2(-1.0f, ImGui::GetTextLineHeight() * 15), ImGuiInputTextFlags_ReadOnly);
    ImGui::End();
}