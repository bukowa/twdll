local game = nil
local faction = nil
local function run_twdll_tests()
    local f = io.open("twdll.log", "a")
    if f then
        f:write("\n===================================\n")
        f:write("[TEST] FirstTickAfterWorldCreated triggered!\n")
        f:write("===================================\n")
        f:close()
    end

    -- Using the flat global structure and PascalCase as defined in your C++ code
    if type(twdll) == "table" and type(twdll.Log) == "function" then
        twdll.Log("[TEST] twdll is loaded. Starting unit tests...")

        -- Grab the first unit from the Roman faction leader
        local char = game:model():world():faction_by_key(faction):faction_leader()
        local unit = char:military_force():unit_list():item_at(0)

        -- Use the flat global twdll_unit and PascalCase methods
        local max_men = twdll_unit.GetMaxNumberOfMan(unit)
        local initial_men = twdll_unit.GetNumberOfMan(unit)
        local initial_percentage = unit:percentage_proportion_of_full_strength()

        twdll.Log("[TEST] Unit Initial State - Men: " ..
        tostring(initial_men) .. "/" .. tostring(max_men) .. " (" .. tostring(initial_percentage) .. "%)")

        -- Modify the unit
        twdll_unit.SetNumberOfMan(unit, 20)

        local new_men = twdll_unit.GetNumberOfMan(unit)
        local new_percentage = unit:percentage_proportion_of_full_strength()

        twdll.Log("[TEST] Unit Modified State - Men: " ..
        tostring(new_men) .. "/" .. tostring(max_men) .. " (" .. tostring(new_percentage) .. "%)")

        if new_men == 20 then
            twdll.Log("[TEST] Final Result: SUCCESS")
        else
            twdll.Log("[TEST] Final Result: FAILED (twdll_unit failed to modify unit memory!)")
        end
    else
        local f2 = io.open("twdll.log", "a")
        if f2 then
            f2:write("[TEST] Final Result: FAILED (twdll.Log not found in global state. Testing aborted.)\n")
            f2:close()
        end
    end
end

-- Register the test suite to execute only after the world is initialized
local _, err = pcall(function()
    table.insert(events.FirstTickAfterWorldCreated, function()

        local scripting_string = nil
        if twdll.GameBuild() == "Rome2" then
            scripting_string = 'lua_scripts.EpisodicScripting'
            faction = 'rom_rome'
        elseif twdll.GameBuild() == "Attila" then
            scripting_string = "lua_scripts.episodicscripting"
            faction = 'att_fact_hunni'
        end

        local scripting = require(scripting_string)
        game = scripting.game_interface

        local ok, err = pcall(run_twdll_tests)
        twdll.Log(err)
    end)
end)

if err then
    twdll.Log("Added event had error: " .. tostring(err))
end
