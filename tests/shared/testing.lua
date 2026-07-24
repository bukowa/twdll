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
    if type(twdll) == "table" and type(twdll.core) == "table" and type(twdll.core.Log) == "function" then
        twdll.core.Log("[TEST] twdll is loaded. Starting unit tests...")

        -- ======================================================
        -- TEST 1: Verify WORLD and CAMPAIGN_UI singleton hooks
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 1: Singleton hooks ---")

        local world_ptr     = twdll.world.GetMemoryAddress()
        local ui_ptr        = twdll.campaign_ui.GetMemoryAddress()
        local faction_count = twdll.world.GetFactionCount()

        twdll.core.Log("[TEST] g_world       = " .. tostring(world_ptr))
        twdll.core.Log("[TEST] g_campaign_ui = " .. tostring(ui_ptr))
        twdll.core.Log("[TEST] FactionCount  = " .. tostring(faction_count))

        if world_ptr ~= nil and world_ptr ~= 0 then
            twdll.core.Log("[TEST] g_world hook: OK")
        else
            twdll.core.Log("[TEST] g_world hook: FAILED (nil or zero!)")
        end

        if ui_ptr ~= nil and ui_ptr ~= 0 then
            twdll.core.Log("[TEST] g_campaign_ui hook: OK")
        else
            twdll.core.Log("[TEST] g_campaign_ui hook: FAILED (nil or zero!)")
        end

        local engine_faction_count = game:model():world():faction_list():num_items()
        
        twdll.core.Log(string.format("[TEST] Faction count verification: hook=%s, engine=%s", tostring(faction_count), tostring(engine_faction_count)))
        
        if faction_count == engine_faction_count and faction_count ~= nil and faction_count > 0 then
            twdll.core.Log("[TEST] g_world hook validation: PASSED")
        else
            twdll.core.Log("[TEST] g_world hook validation: FAILED")
        end



        -- ======================================================
        -- TEST 2: twdll_unit read/write
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 2: twdll_unit read/write ---")

        local char = game:model():world():faction_by_key(faction):faction_leader()
        local unit = char:military_force():unit_list():item_at(0)

        local max_men = twdll.unit.GetMaxNumberOfMan(unit)
        local initial_men = twdll.unit.GetNumberOfMan(unit)
        local initial_percentage = unit:percentage_proportion_of_full_strength()

        twdll.core.Log("[TEST] Unit Initial State - Men: " ..
            tostring(initial_men) .. "/" .. tostring(max_men) .. " (" .. tostring(initial_percentage) .. "%)")

        twdll.unit.SetNumberOfMan(unit, 20)

        local new_men = twdll.unit.GetNumberOfMan(unit)
        local new_percentage = unit:percentage_proportion_of_full_strength()

        twdll.core.Log("[TEST] Unit Modified State - Men: " ..
            tostring(new_men) .. "/" .. tostring(max_men) .. " (" .. tostring(new_percentage) .. "%)")

        if new_men == 20 then
            twdll.core.Log("[TEST] twdll_unit: OK")
        else
            twdll.core.Log("[TEST] twdll_unit: FAILED (expected 20, got " .. tostring(new_men) .. ")")
        end

        -- ======================================================
        -- SUMMARY
        -- ======================================================
        twdll.core.Log("[TEST] ===== ALL TESTS DONE =====")

    else
        local f2 = io.open("twdll.log", "a")
        if f2 then
            f2:write("[TEST] FAILED: twdll.Log not found in global state. Testing aborted.\n")
            f2:close()
        end
    end
end

-- Register the test suite to execute only after the world is initialized
local _, err = pcall(function()
    table.insert(events.FirstTickAfterWorldCreated, function()

        local scripting_string = nil
        if twdll.core.GameBuild() == "Rome2" then
            scripting_string = 'lua_scripts.EpisodicScripting'
            faction = 'rom_rome'
        elseif twdll.core.GameBuild() == "Attila" then
            scripting_string = "lua_scripts.episodicscripting"
            faction = 'att_fact_hunni'
        end

        local scripting = require(scripting_string)
        game = scripting.game_interface

        local ok, err = pcall(run_twdll_tests)
        twdll.core.Log(err)
    end)
end)

if err then
    twdll.core.Log("Added event had error: " .. tostring(err))
end
