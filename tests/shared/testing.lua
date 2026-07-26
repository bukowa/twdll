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
        -- TEST 3: faction:SetFactionLeader
        -- Behaviour (from FACTION::new_faction_leader source):
        --   old_character = nil  → no succession event fired, rest works normally
        --   old_character given  → fires faction_succession event
        --     new leader is m_regent     → fires faction_succession_regency instead
        --     heir_coming_of_age = true  → fires faction_succession_heir_comes_of_age
        --   always: new leader m_heir set to 0, political party allegiance changed
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 3: faction:SetFactionLeader ---")
        do
            local f     = game:model():world():faction_by_key(faction)
            local chars = f:character_list()
            local total = chars:num_items()

            -- collect generals
            local generals = {}
            for i = 0, total - 1 do
                local c = chars:item_at(i)
                if c:character_type("general") then
                    generals[#generals + 1] = c
                end
            end

            if #generals < 2 then
                twdll.core.Log("[TEST] SetFactionLeader: SKIPPED (need at least 2 generals)")
            else
                -- helper: find a general that is not the current leader
                local function pick_non_leader()
                    local leader_cqi = f:faction_leader():cqi()
                    for _, c in ipairs(generals) do
                        if c:cqi() ~= leader_cqi then return c end
                    end
                end

                -- case 1: silent swap (old = nil, no event)
                local new1 = pick_non_leader()
                local old1_cqi = f:faction_leader():cqi()
                f:SetFactionLeader(new1)
                local after1 = f:faction_leader():cqi()
                if after1 == new1:cqi() then
                    twdll.core.Log(string.format("[TEST] SetFactionLeader silent: OK (old=%d new=%d)", old1_cqi, after1))
                else
                    twdll.core.Log(string.format("[TEST] SetFactionLeader silent: FAILED (expected=%d got=%d)", new1:cqi(), after1))
                end

                -- case 2: normal succession (old provided, fires faction_succession)
                local new2 = pick_non_leader()
                local old2 = f:faction_leader()
                f:SetFactionLeader(new2, old2)
                local after2 = f:faction_leader():cqi()
                if after2 == new2:cqi() then
                    twdll.core.Log(string.format("[TEST] SetFactionLeader succession: OK (old=%d new=%d)", old2:cqi(), after2))
                else
                    twdll.core.Log(string.format("[TEST] SetFactionLeader succession: FAILED (expected=%d got=%d)", new2:cqi(), after2))
                end

                -- case 3: heir coming of age (fires faction_succession_heir_comes_of_age)
                local new3 = pick_non_leader()
                local old3 = f:faction_leader()
                f:SetFactionLeader(new3, old3, true)
                local after3 = f:faction_leader():cqi()
                if after3 == new3:cqi() then
                    twdll.core.Log(string.format("[TEST] SetFactionLeader heir_coming_of_age: OK (old=%d new=%d)", old3:cqi(), after3))
                else
                    twdll.core.Log(string.format("[TEST] SetFactionLeader heir_coming_of_age: FAILED (expected=%d got=%d)", new3:cqi(), after3))
                end
            end
        end

        -- ======================================================
        -- TEST 4: faction GetGold / SetGold
        -- Attila: Hunni start with 15000 gold.
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 4: faction GetGold/SetGold ---")
        do
            local f            = game:model():world():faction_by_key(faction)
            local gold_initial = f:GetGold()
            twdll.core.Log("[TEST] GetGold initial = " .. tostring(gold_initial))

            if twdll.core.GameBuild() == "Attila" and gold_initial == 15000 then
                twdll.core.Log("[TEST] GetGold initial: OK (15000)")
            else
                twdll.core.Log("[TEST] GetGold initial: got " .. tostring(gold_initial))
            end

            f:SetGold(99999)
            local gold_after = f:GetGold()
            twdll.core.Log("[TEST] GetGold after SetGold(99999) = " .. tostring(gold_after))
            if gold_after == 99999 then
                twdll.core.Log("[TEST] SetGold: OK")
            else
                twdll.core.Log("[TEST] SetGold: FAILED (expected 99999, got " .. tostring(gold_after) .. ")")
            end
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
