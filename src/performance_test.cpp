#include "performance_test.h"

#include <chrono>
#include <sstream>
#include "imgui.h"
#include "log.h"
#include "game_lua_api.h"

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
    return Py_BuildValue("");
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
// 4. The Benchmark Runner (UPDATED with Game Logic Simulation)
// =================================================================================

void RunPerformanceTests(lua_State* L, char* results_buffer, size_t buffer_size) {
    std::stringstream ss;
    ss << "Running benchmarks...\n\n";

    // --- Test 1: Game Logic Simulation ---
    // This test simulates a more realistic workload than a simple PI calculation.
    // It creates 100 "cities", each with 5 "buildings".
    // It then iterates through all of them, calculating a "score" based on
    // population, building types, and levels.
    // This measures data creation, iteration, dictionary/table lookups,
    // string comparisons, and conditional logic.
    const int num_sim_runs = 500; // Run the simulation this many times to get a measurable duration.
    ss << "--- Test 1: Game Logic Simulation (" << num_sim_runs << " runs) ---\n";
    ss << "Simulates iterating over 100 cities with 5 buildings each.\n";
    
    // The Python version of the simulation script
    const char* python_logic_script = R"(
def simulate_game_turn():
    cities = []
    building_types = ['market', 'barracks', 'library', 'granary', 'walls']
    for i in range(100):
        city = {
            'name': f'City_{i}',
            'population': 10000 + (i * 150) % 7500,
            'is_capital': (i == 0),
            'buildings': []
        }
        for j in range(5):
            building = {
                'type': building_types[j % 5],
                'level': 1 + (i + j) % 4
            }
            city['buildings'].append(building)
        cities.append(city)

    total_score = 0
    for city in cities:
        score = city['population'] // 100
        if city['is_capital']:
            score += 500
        
        for building in city['buildings']:
            if building['type'] == 'market':
                score += building['level'] * 50
            elif building['type'] == 'barracks':
                score += building['level'] * 20
            elif building['type'] == 'library':
                score += building['level'] * 30
            else:
                score += building['level'] * 10
        total_score += score
    return total_score

for _ in range(500):
    simulate_game_turn()
)";

    auto start = std::chrono::high_resolution_clock::now();
    PyRun_SimpleString(python_logic_script);
    auto end = std::chrono::high_resolution_clock::now();
    std::chrono::duration<double, std::milli> python_duration = end - start;
    ss << "Python (Embedded):  " << python_duration.count() << " ms\n";

    // The Lua version of the simulation script
    const char* lua_logic_script = R"(
local function simulate_game_turn()
    local cities = {}
    local building_types = {'market', 'barracks', 'library', 'granary', 'walls'}
    for i = 0, 99 do
        local city = {
            name = "City_" .. i,
            population = 10000 + (i * 150) % 7500,
            is_capital = (i == 0),
            buildings = {}
        }
        for j = 1, 5 do
            local building = {
                type = building_types[j],
                level = 1 + (i + j) % 4
            }
            table.insert(city.buildings, building)
        end
        table.insert(cities, city)
    end

    local total_score = 0
    for _, city in ipairs(cities) do
        local score = math.floor(city.population / 100)
        if city.is_capital then
            score = score + 500
        end

        for _, building in ipairs(city.buildings) do
            if building.type == 'market' then
                score = score + building.level * 50
            elseif building.type == 'barracks' then
                score = score + building.level * 20
            elseif building.type == 'library' then
                score = score + building.level * 30
            else
                score = score + building.level * 10
            end
        end
        total_score = total_score + score
    end
    return total_score
end

for i = 1, 500 do
    simulate_game_turn()
end
)";

    start = std::chrono::high_resolution_clock::now();
    RunGameLuaString(L, lua_logic_script);
    end = std::chrono::high_resolution_clock::now();
    std::chrono::duration<double, std::milli> lua_duration = end - start;
    ss << "Lua (Game's Internal): " << lua_duration.count() << " ms\n\n";


    // --- Test 2: C++ Function Calls (FFI Overhead) ---
    // This test remains vital as it measures raw communication speed.
    const int ffi_call_iterations = 2'000'000;
    ss << "--- Test 2: C++ Function Calls (" << ffi_call_iterations << " iterations) ---\n";
    ss << "Measures the raw cost of calling C++ from the script.\n";

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