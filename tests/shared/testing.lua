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

    -- ======================================================
    -- RELOAD CHECK: If we are running after LoadGame, verify and finish test run
    -- ======================================================
    local marker_file = "twdll_reload_marker.flag"
    local f_marker = io.open(marker_file, "r")
    if f_marker then
        f_marker:close()
        os.remove(marker_file)
        if type(twdll) == "table" and type(twdll.core) == "table" and type(twdll.core.Log) == "function" then
            twdll.core.Log("\n======================================================")
            twdll.core.Log("[TEST] [LIFECYCLE] Reload verification successful!")
            twdll.core.Log("[TEST] [LIFECYCLE] Campaign successfully reloaded from saved game.")
            twdll.core.Log("[TEST] [LIFECYCLE] All tests and save/load cycle PASSED.")
            twdll.core.Log("======================================================\n")
        end
        return
    end

    -- Using the flat global structure and PascalCase as defined in your C++ code
    if type(twdll) == "table" and type(twdll.core) == "table" and type(twdll.core.Log) == "function" then
        twdll.core.Log("[TEST] twdll is loaded. Starting unit tests...")

        -- Aggregate test outcomes across the whole suite.
        local passed, failed, skipped = 0, 0, 0
        local failed_names = {}
        local function report(name, condition)
            if condition then passed = passed + 1
            else failed = failed + 1; failed_names[#failed_names + 1] = name end
        end
        local function record_skip() skipped = skipped + 1 end

        -- Verify twdll.core.Log handles arbitrary types (nil, boolean, number, table) without errors
        local log_test_ok = pcall(function()
            twdll.core.Log("[TEST] twdll.core.Log multi-type test:", nil, true, 123, { test = 1 })
        end)
        if log_test_ok then
            twdll.core.Log("[TEST] twdll.core.Log multi-type: OK")
            report("twdll.core.Log multi-type", true)
        else
            report("twdll.core.Log multi-type", false)
        end

        -- Verify twdll.core.GetBuildSha returns a valid string
        local build_sha = twdll.core.GetBuildSha and twdll.core.GetBuildSha()
        twdll.core.Log("[TEST] twdll.core.GetBuildSha = " .. tostring(build_sha))
        if type(build_sha) == "string" and #build_sha >= 7 then
            twdll.core.Log("[TEST] twdll.core.GetBuildSha: OK (" .. build_sha .. ")")
            report("twdll.core.GetBuildSha", true)
        else
            twdll.core.Log("[TEST] twdll.core.GetBuildSha: FAILED")
            report("twdll.core.GetBuildSha", false)
        end

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
            report("g_world hook", true)
        else
            twdll.core.Log("[TEST] g_world hook: FAILED (nil or zero!)")
            report("g_world hook", false)
        end

        if ui_ptr ~= nil and ui_ptr ~= 0 then
            twdll.core.Log("[TEST] g_campaign_ui hook: OK")
            report("g_campaign_ui hook", true)
        else
            twdll.core.Log("[TEST] g_campaign_ui hook: FAILED (nil or zero!)")
            report("g_campaign_ui hook", false)
        end

        local engine_faction_count = game:model():world():faction_list():num_items()
        
        twdll.core.Log(string.format("[TEST] Faction count verification: hook=%s, engine=%s", tostring(faction_count), tostring(engine_faction_count)))
        
        if faction_count == engine_faction_count and faction_count ~= nil and faction_count > 0 then
            twdll.core.Log("[TEST] g_world hook validation: PASSED")
            report("g_world hook validation", true)
        else
            twdll.core.Log("[TEST] g_world hook validation: FAILED")
            report("g_world hook validation", false)
        end



        -- ======================================================
        -- TEST 2: twdll_unit read/write (via metatable)
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 2: twdll_unit read/write (via metatable) ---")

        -- Iterate all faction characters and take the first one that has a
        -- military force (the faction leader may not have one).
        local char = nil
        local chars = game:model():world():faction_by_key(faction):character_list()
        for i = 0, chars:num_items() - 1 do
            local c = chars:item_at(i)
            if c:military_force() ~= nil then
                char = c
                local fname = (type(c.get_forename) == "function") and tostring(c:get_forename()) or "unknown"
                local sname = (type(c.get_surname) == "function") and tostring(c:get_surname()) or ""
                local stype = (type(c.character_subtype_key) == "function") and tostring(c:character_subtype_key()) or ""
                twdll.core.Log(string.format("[TEST] picked character cqi=%d name='%s %s' subtype='%s' (first with a military force)", c:cqi(), fname, sname, stype))
                break
            end
        end

        if char == nil then
            twdll.core.Log("[TEST] unit metatable: SKIPPED (no character with a military force)")
            record_skip()
        else
            local unit = char:military_force():unit_list():item_at(0)

            local max_men = unit:GetMaxNumMen()
            local initial_men = unit:GetNumMen()
            local initial_percentage = unit:percentage_proportion_of_full_strength()

            twdll.core.Log("[TEST] Unit Initial State - Men: " ..
                tostring(initial_men) .. "/" .. tostring(max_men) .. " (" .. tostring(initial_percentage) .. "%)")

            unit:SetNumMen(20)

            local new_men = unit:GetNumMen()
            local new_percentage = unit:percentage_proportion_of_full_strength()

            twdll.core.Log("[TEST] Unit Modified State - Men: " ..
                tostring(new_men) .. "/" .. tostring(max_men) .. " (" .. tostring(new_percentage) .. "%)")

            if new_men == 20 then
                twdll.core.Log("[TEST] unit metatable: OK")
                report("unit metatable", true)
            else
                twdll.core.Log("[TEST] unit metatable: FAILED (expected 20, got " .. tostring(new_men) .. ")")
                report("unit metatable", false)
            end

            twdll.core.Log("[TEST] --- Test 2b: SetMaxNumMen ---")
            local max_before = unit:GetMaxNumMen()
            twdll.core.Log("[TEST] MaxNumMen initial = " .. tostring(max_before))
            unit:SetMaxNumMen(150)
            local max_after = unit:GetMaxNumMen()
            twdll.core.Log("[TEST] MaxNumMen after  = " .. tostring(max_after))
            if max_after == 150 then
                twdll.core.Log("[TEST] SetMaxNumMen: OK")
                report("SetMaxNumMen", true)
            else
                twdll.core.Log("[TEST] SetMaxNumMen: FAILED (expected 150, got " .. tostring(max_after) .. ")")
                report("SetMaxNumMen", false)
            end

            twdll.core.Log("[TEST] --- Test 2c: GetActionPoints / SetActionPoints ---")
            local ap_before = unit:GetActionPoints()
            twdll.core.Log("[TEST] ActionPoints initial = " .. tostring(ap_before))
            unit:SetActionPoints(50)
            local ap_after = unit:GetActionPoints()
            twdll.core.Log("[TEST] ActionPoints after  = " .. tostring(ap_after))
            if ap_after == 50 then
                twdll.core.Log("[TEST] SetActionPoints: OK")
                report("SetActionPoints", true)
            else
                twdll.core.Log("[TEST] SetActionPoints: FAILED (expected 50, got " .. tostring(ap_after) .. ")")
                report("SetActionPoints", false)
            end

            twdll.core.Log("[TEST] --- Test 2d: ConvertUnit ---")
            local ul = char:military_force():unit_list()
            local src_index = -1
            for i = 0, ul:num_items() - 1 do
                if ul:item_at(i):unit_key() == "att_nom_steppe_chieftain" then
                    src_index = i
                    break
                end
            end
            if src_index < 0 then
                twdll.core.Log("[TEST] ConvertUnit: SKIPPED (no att_nom_steppe_chieftain in force)")
                record_skip()
            else
                local src_unit = ul:item_at(src_index)
                local men_before = src_unit:GetNumMen()
                local max_before = src_unit:GetMaxNumMen()
                local ok = src_unit:ConvertUnit("att_merc_ger_agathyrsi_warriors")
                if ok ~= true then
                    twdll.core.Log("[TEST] ConvertUnit: FAILED (returned " .. tostring(ok) .. ")")
                    report("ConvertUnit", false)
                else
                    local unit_after = char:military_force():unit_list():item_at(src_index)
                    local men_after = unit_after:GetNumMen()
                    local max_after = unit_after:GetMaxNumMen()
                    local key_after = unit_after:unit_key()
                    local expected_men = (men_before >= max_before) and max_after or math.floor((men_before / max_before) * max_after + 0.5)
                    twdll.core.Log("[TEST] ConvertUnit: men " .. tostring(men_before) .. "/" .. tostring(max_before) ..
                        " -> " .. tostring(men_after) .. "/" .. tostring(max_after) .. " (expected " .. tostring(expected_men) .. "), key '" .. tostring(key_after) .. "'")
                    if men_after == expected_men and key_after == "att_merc_ger_agathyrsi_warriors" then
                        twdll.core.Log("[TEST] ConvertUnit: OK")
                        report("ConvertUnit", true)
                    else
                        twdll.core.Log("[TEST] ConvertUnit: FAILED (men/key mismatch)")
                        report("ConvertUnit", false)
                    end
                end
            end
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
                record_skip()
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
                    report("SetFactionLeader silent", true)
                else
                    twdll.core.Log(string.format("[TEST] SetFactionLeader silent: FAILED (expected=%d got=%d)", new1:cqi(), after1))
                    report("SetFactionLeader silent", false)
                end

                -- case 2: normal succession (old provided, fires faction_succession)
                local new2 = pick_non_leader()
                local old2 = f:faction_leader()
                f:SetFactionLeader(new2, old2)
                local after2 = f:faction_leader():cqi()
                if after2 == new2:cqi() then
                    twdll.core.Log(string.format("[TEST] SetFactionLeader succession: OK (old=%d new=%d)", old2:cqi(), after2))
                    report("SetFactionLeader succession", true)
                else
                    twdll.core.Log(string.format("[TEST] SetFactionLeader succession: FAILED (expected=%d got=%d)", new2:cqi(), after2))
                    report("SetFactionLeader succession", false)
                end

                -- case 3: heir coming of age (fires faction_succession_heir_comes_of_age)
                local new3 = pick_non_leader()
                local old3 = f:faction_leader()
                f:SetFactionLeader(new3, old3, true)
                local after3 = f:faction_leader():cqi()
                if after3 == new3:cqi() then
                    twdll.core.Log(string.format("[TEST] SetFactionLeader heir_coming_of_age: OK (old=%d new=%d)", old3:cqi(), after3))
                    report("SetFactionLeader heir_coming_of_age", true)
                else
                    twdll.core.Log(string.format("[TEST] SetFactionLeader heir_coming_of_age: FAILED (expected=%d got=%d)", new3:cqi(), after3))
                    report("SetFactionLeader heir_coming_of_age", false)
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
            local gold_initial = f:GetTreasury()
            twdll.core.Log("[TEST] GetTreasury initial = " .. tostring(gold_initial))

            if twdll.core.GameBuild() == "Attila" and gold_initial == 15000 then
                twdll.core.Log("[TEST] GetTreasury initial: OK (15000)")
                report("GetTreasury initial", true)
            else
                twdll.core.Log("[TEST] GetTreasury initial: FAILED (expected 15000 on Attila, got " .. tostring(gold_initial) .. ")")
                report("GetTreasury initial", false)
            end

            f:SetTreasury(99999)
            local gold_after = f:GetTreasury()
            twdll.core.Log("[TEST] GetTreasury after SetTreasury(99999) = " .. tostring(gold_after))
            if gold_after == 99999 then
                twdll.core.Log("[TEST] SetTreasury: OK")
                report("SetTreasury", true)
            else
                twdll.core.Log("[TEST] SetTreasury: FAILED (expected 99999, got " .. tostring(gold_after) .. ")")
                report("SetTreasury", false)
            end
        end

        -- ======================================================
        -- TEST 5: twdll.model DisbandUnits
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 5: twdll.model DisbandUnits ---")
        do
            if char == nil then
                twdll.core.Log("[TEST] DisbandUnits: SKIPPED (no character with a military force)")
                record_skip()
            else
                local mf   = char:military_force()
                local ul   = mf:unit_list()
                local before = ul:num_items()

                twdll.core.Log("[TEST] DisbandUnits: units before = " .. tostring(before))

                if before > 0 then
                    local last_unit = ul:item_at(before - 1)
                    twdll.model.DisbandUnits(last_unit)
                    local after = mf:unit_list():num_items()
                    twdll.core.Log("[TEST] DisbandUnits: units after  = " .. tostring(after))
                    if after == before - 1 then
                        twdll.core.Log("[TEST] DisbandUnits: OK")
                        report("DisbandUnits", true)
                    else
                        twdll.core.Log("[TEST] DisbandUnits: FAILED (expected " .. tostring(before - 1) .. ", got " .. tostring(after) .. ")")
                        report("DisbandUnits", false)
                    end
                else
                    twdll.core.Log("[TEST] DisbandUnits: SKIPPED (military force has no units)")
                    record_skip()
                end
            end
        end

        -- ======================================================
        -- TEST 5b: MILITARY_FORCE_SCRIPT_INTERFACE methods
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 5b: MILITARY_FORCE_SCRIPT_INTERFACE methods ---")
        do
            if char == nil then
                twdll.core.Log("[TEST] MILITARY_FORCE_SCRIPT_INTERFACE: SKIPPED (no character with a military force)")
                record_skip()
            else
                local mf = char:military_force()
                if type(mf.GetRecruitmentQueueSize) == "function" then
                    local qsize = mf:GetRecruitmentQueueSize()
                    twdll.core.Log("[TEST] MILITARY_FORCE_SCRIPT_INTERFACE: recruitment queue size = " .. tostring(qsize))
                    if qsize == 0 then
                        twdll.core.Log("[TEST] GetRecruitmentQueueSize: OK")
                        report("GetRecruitmentQueueSize", true)
                    else
                        twdll.core.Log("[TEST] GetRecruitmentQueueSize: FAILED (expected 0, got " .. tostring(qsize) .. ")")
                        report("GetRecruitmentQueueSize", false)
                    end
                    local addr = mf:GetMemoryAddress()
                    if type(addr) == "string" and string.match(addr, "^0x") then
                        twdll.core.Log("[TEST] GetMemoryAddress: OK")
                        report("GetMemoryAddress", true)
                    else
                        twdll.core.Log("[TEST] GetMemoryAddress: FAILED (got " .. tostring(addr) .. ")")
                        report("GetMemoryAddress", false)
                    end
                else
                    twdll.core.Log("[TEST] MILITARY_FORCE_SCRIPT_INTERFACE: FAILED (methods not registered)")
                    report("GetRecruitmentQueueSize", false)
                    report("GetMemoryAddress", false)
                end
            end
        end

        -- ======================================================
        -- TEST 6: twdll.world SetMaxUnitsInArmy / SetMaxUnitsInNavy
        -- Attila default: army=20, navy=20
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 6: SetMaxUnitsInArmy / SetMaxUnitsInNavy ---")
        do
            local army_before = twdll.world.GetMaxUnitsInArmy()
            local navy_before = twdll.world.GetMaxUnitsInNavy()
            twdll.core.Log("[TEST] MaxUnitsInArmy initial = " .. tostring(army_before))
            twdll.core.Log("[TEST] MaxUnitsInNavy initial = " .. tostring(navy_before))

            twdll.world.SetMaxUnitsInArmy(30)
            twdll.world.SetMaxUnitsInNavy(15)

            local army_after = twdll.world.GetMaxUnitsInArmy()
            local navy_after = twdll.world.GetMaxUnitsInNavy()
            twdll.core.Log("[TEST] MaxUnitsInArmy after Set(30) = " .. tostring(army_after))
            twdll.core.Log("[TEST] MaxUnitsInNavy after Set(15) = " .. tostring(navy_after))

            if army_after == 30 then
                twdll.core.Log("[TEST] SetMaxUnitsInArmy: OK")
                report("SetMaxUnitsInArmy", true)
            else
                twdll.core.Log("[TEST] SetMaxUnitsInArmy: FAILED (expected 30, got " .. tostring(army_after) .. ")")
                report("SetMaxUnitsInArmy", false)
            end

            if navy_after == 15 then
                twdll.core.Log("[TEST] SetMaxUnitsInNavy: OK")
                report("SetMaxUnitsInNavy", true)
            else
                twdll.core.Log("[TEST] SetMaxUnitsInNavy: FAILED (expected 15, got " .. tostring(navy_after) .. ")")
                report("SetMaxUnitsInNavy", false)
            end

            -- Test restore via no-arg / nil
            twdll.world.SetMaxUnitsInArmy()
            twdll.world.SetMaxUnitsInNavy(nil)
            local army_restored = twdll.world.GetMaxUnitsInArmy()
            local navy_restored = twdll.world.GetMaxUnitsInNavy()
            report("SetMaxUnitsInArmy() restores default", army_restored == army_before)
            report("SetMaxUnitsInNavy(nil) restores default", navy_restored == navy_before)
        end

        -- ======================================================
        -- TEST 7: game:add_unit_to_force — add 80 units to every army
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 7: game:add_unit_to_force (add 80 to our armies) ---")
        do
            twdll.world.SetMaxUnitsInArmy(80)
            local cap = twdll.world.GetMaxUnitsInArmy()

            local f   = game:model():world():faction_by_key(faction)
            local mfl = f:military_force_list()
            local mfl_n = mfl:num_items()

            local key = "att_nom_hunnic_mounted_warband"
            local forces_done = 0
            local units_added = 0

            for j = 0, mfl_n - 1 do
                local mf     = mfl:item_at(j)
                local cqi    = mf:command_queue_index()
                local before = mf:unit_list():num_items()
                for k = 1, 80 do
                    game:add_unit_to_force(key, cqi)
                end
                local after  = mf:unit_list():num_items()
                forces_done  = forces_done + 1
                units_added  = units_added + (after - before)
                twdll.core.Log(string.format("[TEST]   force cqi=%d: %d -> %d units", cqi, before, after))
            end

            twdll.core.Log(string.format("[TEST] add_unit_to_force: forces=%d units_added=%d cap=%d", forces_done, units_added, cap))

            if units_added > 0 then
                twdll.core.Log("[TEST] add_unit_to_force: OK (units added)")
                report("add_unit_to_force", true)
            else
                twdll.core.Log("[TEST] add_unit_to_force: FAILED (no units added)")
                report("add_unit_to_force", false)
            end
        end

        -- ======================================================
        -- TEST 8: twdll.world SetReinforcementCap / GetReinforcementCap
        -- -1 restores the game default (nil from the getter).
        -- Note: Attila's Lua stores numbers as 32-bit floats, so values must
        -- be exactly representable (<= 2^24); 1000000 is "effectively unlimited".
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 8: SetReinforcementCap / GetReinforcementCap ---")
        do
            local initial = twdll.world.GetReinforcementCap()
            twdll.core.Log("[TEST] ReinforcementCap initial = " .. tostring(initial))

            twdll.world.SetReinforcementCap(0)
            local v0 = twdll.world.GetReinforcementCap()
            if v0 == 0 then
                twdll.core.Log("[TEST] SetReinforcementCap(0): OK")
                report("SetReinforcementCap(0)", true)
            else
                twdll.core.Log("[TEST] SetReinforcementCap(0): FAILED (expected 0, got " .. tostring(v0) .. ")")
                report("SetReinforcementCap(0)", false)
            end

            twdll.world.SetReinforcementCap(1000000)
            local vmax = twdll.world.GetReinforcementCap()
            if vmax == 1000000 then
                twdll.core.Log("[TEST] SetReinforcementCap(1000000): OK")
                report("SetReinforcementCap(1000000)", true)
            else
                twdll.core.Log("[TEST] SetReinforcementCap(1000000): FAILED (expected 1000000, got " .. tostring(vmax) .. ")")
                report("SetReinforcementCap(1000000)", false)
            end

            twdll.world.SetReinforcementCap(-1)
            local vdef = twdll.world.GetReinforcementCap()
            if vdef == nil then
                twdll.core.Log("[TEST] SetReinforcementCap(-1) restore: OK")
                report("SetReinforcementCap(-1) restore", true)
            else
                twdll.core.Log("[TEST] SetReinforcementCap(-1) restore: FAILED (expected nil, got " .. tostring(vdef) .. ")")
                report("SetReinforcementCap(-1) restore", false)
            end
        end

        -- ======================================================
        -- TEST 9: twdll.battle GetBattleInfo
        -- Expected nil while the test suite runs (campaign, no active battle).
        -- Live battle layout verification happens in twdll.log via the
        -- BATTLE ctor/dtor hooks (user plays a battle with tw-test).
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 9: GetBattleInfo ---")
        do
            local info = twdll.battle.GetBattleInfo()
            if info == nil then
                twdll.core.Log("[TEST] GetBattleInfo in campaign: OK (nil)")
                report("GetBattleInfo campaign nil", true)
            elseif type(info) == "table" then
                twdll.core.Log(string.format(
                    "[TEST] GetBattleInfo during battle: battle=%s manager=%s cap=%s size=%s",
                    tostring(info.battle), tostring(info.manager), tostring(info.cap), tostring(info.size)))
                if info.battle ~= nil and info.battle ~= 0 and info.manager ~= nil and info.cap ~= nil and info.size ~= nil then
                    twdll.core.Log("[TEST] GetBattleInfo during battle: OK")
                    report("GetBattleInfo battle table", true)
                else
                    twdll.core.Log("[TEST] GetBattleInfo during battle: FAILED (missing fields)")
                    report("GetBattleInfo battle table", false)
                end
            else
                twdll.core.Log("[TEST] GetBattleInfo: FAILED (unexpected type " .. type(info) .. ")")
                report("GetBattleInfo type", false)
            end
        end

        -- ======================================================
        -- TEST 10: REGION_SCRIPT_INTERFACE methods
        -- GetPopulationSurplus / SetPopulationSurplus /
        -- GetGrowthPoints / SetGrowthPoints
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 10: REGION_SCRIPT_INTERFACE methods ---")
        do
            local region = game:model():world():region_manager():region_by_key("att_reg_scandza_hafn")
            if region == nil or region:is_null_interface() then
                twdll.core.Log("[TEST] REGION_SCRIPT_INTERFACE: SKIPPED (region att_reg_scandza_hafn not found)")
                record_skip()
            else
                if type(region.GetPopulationSurplus) ~= "function" then
                    twdll.core.Log("[TEST] REGION_SCRIPT_INTERFACE: FAILED (methods not registered)")
                    report("region methods registered", false)
                else
                    report("region methods registered", true)

                    -- Population surplus: game-start value is 5
                    local dp_orig = region:GetPopulationSurplus()
                    twdll.core.Log("[TEST] PopulationSurplus initial = " .. tostring(dp_orig))
                    if dp_orig == 5 then
                        twdll.core.Log("[TEST] PopulationSurplus initial: OK (5)")
                        report("PopulationSurplus initial", true)
                    else
                        twdll.core.Log("[TEST] PopulationSurplus initial: FAILED (expected 5, got " .. tostring(dp_orig) .. ")")
                        report("PopulationSurplus initial", false)
                    end
                    region:SetPopulationSurplus(10)
                    local dp_after = region:GetPopulationSurplus()
                    twdll.core.Log("[TEST] PopulationSurplus after Set(10) = " .. tostring(dp_after))
                    if dp_after == 10 then
                        twdll.core.Log("[TEST] SetPopulationSurplus: OK")
                        report("SetPopulationSurplus", true)
                    else
                        twdll.core.Log(string.format("[TEST] SetPopulationSurplus: FAILED (expected 10, got %d)", dp_after))
                        report("SetPopulationSurplus", false)
                    end
                    region:SetPopulationSurplus(dp_orig)

                    -- Growth points: game-start value is 0
                    local sp_orig = region:GetGrowthPoints()
                    twdll.core.Log("[TEST] GrowthPoints initial = " .. tostring(sp_orig))
                    if sp_orig == 0 then
                        twdll.core.Log("[TEST] GrowthPoints initial: OK (0)")
                        report("GrowthPoints initial", true)
                    else
                        twdll.core.Log("[TEST] GrowthPoints initial: FAILED (expected 0, got " .. tostring(sp_orig) .. ")")
                        report("GrowthPoints initial", false)
                    end
                    region:SetGrowthPoints(20)
                    local sp_after = region:GetGrowthPoints()
                    twdll.core.Log("[TEST] GrowthPoints after Set(20) = " .. tostring(sp_after))
                    if sp_after == 20 then
                        twdll.core.Log("[TEST] SetGrowthPoints: OK")
                        report("SetGrowthPoints", true)
                    else
                        twdll.core.Log(string.format("[TEST] SetGrowthPoints: FAILED (expected 20, got %d)", sp_after))
                        report("SetGrowthPoints", false)
                    end
                    region:SetGrowthPoints(sp_orig)

                    local addr = region:GetMemoryAddress()
                    if type(addr) == "string" and string.match(addr, "^0x") then
                        twdll.core.Log("[TEST] region GetMemoryAddress: OK")
                        report("region GetMemoryAddress", true)
                    else
                        twdll.core.Log("[TEST] region GetMemoryAddress: FAILED (got " .. tostring(addr) .. ")")
                        report("region GetMemoryAddress", false)
                    end
                end
            end
        end

        -- ======================================================
        -- TEST 11: faction:SetCapital
        -- Uses a settled (non-horde) faction so it actually has a capital.
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 11: faction:SetCapital ---")
        do
            local f = nil
            local fl = game:model():world():faction_list()
            if fl ~= nil then
                for i = 0, fl:num_items() - 1 do
                    local cand = fl:item_at(i)
                    if cand ~= nil and not cand:is_null_interface()
                        and not cand:is_horde()
                        and cand:region_list() ~= nil and cand:region_list():num_items() > 0 then
                        f = cand
                        break
                    end
                end
            end
            local region = game:model():world():region_manager():region_by_key("att_reg_scandza_hafn")
            if f == nil then
                twdll.core.Log("[TEST] SetCapital: SKIPPED (no settled faction with regions found)")
                record_skip()
            elseif region == nil or region:is_null_interface() then
                twdll.core.Log("[TEST] SetCapital: SKIPPED (region att_reg_scandza_hafn not found)")
                record_skip()
            elseif type(f.SetCapital) ~= "function" then
                twdll.core.Log("[TEST] SetCapital: FAILED (method not registered)")
                report("SetCapital registered", false)
            else
                report("SetCapital registered", true)
                local orig_capital = f:home_region()
                twdll.core.Log("[TEST] SetCapital: faction = " .. tostring(f:name())
                    .. ", original capital = " .. tostring(orig_capital ~= nil and orig_capital:name() or "nil"))
                f:SetCapital(region)
                local new_capital = f:home_region()
                if new_capital ~= nil and new_capital:name() == region:name() then
                    twdll.core.Log("[TEST] SetCapital: OK (new capital = " .. tostring(new_capital:name()) .. ")")
                    report("SetCapital", true)
                else
                    twdll.core.Log("[TEST] SetCapital: FAILED (expected " .. tostring(region:name())
                        .. ", got " .. tostring(new_capital ~= nil and new_capital:name() or "nil") .. ")")
                    report("SetCapital", false)
                end
                if orig_capital ~= nil and not orig_capital:is_null_interface() then
                    f:SetCapital(orig_capital)
                    twdll.core.Log("[TEST] SetCapital: restored original capital")
                end
            end
        end

        -- ======================================================
        -- TEST 12: faction:InstantlyResearchTechnology
        -- Uses the game's native per-tech instant completion (events,
        -- achievements, unit upgrades + effect/availability refresh).
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 12: faction:InstantlyResearchTechnology ---")
        do
            local f = nil
            local fl = game:model():world():faction_list()
            if fl ~= nil then
                for i = 0, fl:num_items() - 1 do
                    local cand = fl:item_at(i)
                    if cand ~= nil and not cand:is_null_interface() then
                        f = cand
                        break
                    end
                end
            end
            if f == nil then
                twdll.core.Log("[TEST] InstantlyResearchTechnology: SKIPPED (no faction found)")
                record_skip()
            elseif type(f.InstantlyResearchTechnology) ~= "function" then
                twdll.core.Log("[TEST] InstantlyResearchTechnology: FAILED (method not registered)")
                report("InstantlyResearchTechnology registered", false)
            else
                report("InstantlyResearchTechnology registered", true)
                twdll.core.Log("[TEST] InstantlyResearchTechnology: faction = " .. tostring(f:name()))
                -- A real base-game technology key (verified present in data.pack).
                local ok = f:InstantlyResearchTechnology("att_hunnic_military_combat_at_distance")
                twdll.core.Log("[TEST] InstantlyResearchTechnology: result = " .. tostring(ok))
                if ok == true then
                    twdll.core.Log("[TEST] InstantlyResearchTechnology: OK (native path completed)")
                    report("InstantlyResearchTechnology", true)
                else
                    twdll.core.Log("[TEST] InstantlyResearchTechnology: FAILED (returned " .. tostring(ok) .. ")")
                    report("InstantlyResearchTechnology", false)
                end
            end
        end

        -- ======================================================
        -- TEST 13: faction political parties API
        -- Full coverage of the politics surface with hardcoded expected
        -- values (manually verified in-game against att_fact_hunni):
        --   faction:GetPoliticalParties()   -> POLITICAL_PARTY_LIST_SCRIPT_INTERFACE
        --   faction:GetPoliticalParty(key)  -> single party or nil
        --   faction:GetPrimaryParty()       -> primary party or nil
        --   faction:HasPoliticalParties()   -> boolean
        --   party:GetKey() / GetSenators() / GetPower() / IsPrimary()
        -- Power is compared as a whole percent: round(power * 100).
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 13: faction political parties API ---")
        do
            local fl = game:model():world():faction_list()
            if fl == nil then
                twdll.core.Log("[TEST] PoliticalParties: SKIPPED (no faction list)")
                record_skip()
            else
                -- find the known politics faction and the first faction with
                -- zero parties (for the HasPoliticalParties false check)
                local hunni, empty_faction = nil, nil
                for i = 0, fl:num_items() - 1 do
                    local cand = fl:item_at(i)
                    if cand ~= nil and not cand:is_null_interface() then
                        local parties = cand:GetPoliticalPartyList()
                        local n = type(parties) == "userdata" and parties:num_items() or 0
                        if cand:name() == "att_fact_hunni" then
                            hunni = { f = cand, parties = parties, n = n }
                        end
                        if empty_faction == nil and n == 0 then
                            empty_faction = cand
                        end
                    end
                end

                local function round(x) return math.floor(x + 0.5) end

                if hunni == nil then
                    twdll.core.Log("[TEST] PoliticalParties: FAILED (faction 'att_fact_hunni' not found)")
                    report("PoliticalParties list", false)
                    report("PoliticalParties party fields", false)
                    report("PoliticalParties GetPoliticalParty", false)
                    report("PoliticalParties GetPrimaryParty", false)
                    report("PoliticalParties HasPoliticalParties true", false)
                    report("PoliticalParties HasPoliticalParties false", false)
                else
                    local f = hunni.f
                    local parties = hunni.parties
                    twdll.core.Log(string.format(
                        "[TEST] PoliticalParties: faction '%s' has %d parties",
                        tostring(f:name()), hunni.n))

                    -- expected values (verified in-game)
                    local expected = {
                        ["att_politics_hunni_ruler"]   = { senators = 480, power_pct = 70, primary = true  },
                        ["att_politics_hunni_council"] = { senators = 720, power_pct = 30, primary = false },
                    }

                    -- GetPoliticalPartyList: exactly the two known parties
                    local list_ok = type(parties) == "userdata"
                        and parties:num_items() == 2
                        and not parties:is_empty()
                        and parties:item_at(-1) == nil
                        and parties:item_at(999) == nil
                    report("PoliticalParties list", list_ok)

                    -- per-party data against hardcoded values
                    local fields_ok, field_names = true, {}
                    for i = 0, parties:num_items() - 1 do
                        local p = parties:item_at(i)
                        local key = p:GetKey()
                        local exp = expected[key]
                        if exp == nil then
                            fields_ok = false
                            table.insert(field_names, "unknown key '" .. tostring(key) .. "'")
                        else
                            local sens = p:GetSenators()
                            local pct  = round(p:GetPower() * 100)
                            local prim = p:IsPrimary()
                            if sens ~= exp.senators or pct ~= exp.power_pct or prim ~= exp.primary then
                                fields_ok = false
                                table.insert(field_names, string.format(
                                    "'%s' got(sen=%s,pct=%s,prim=%s) want(sen=%s,pct=%s,prim=%s)",
                                    key, tostring(sens), tostring(pct), tostring(prim),
                                    exp.senators, exp.power_pct, tostring(exp.primary)))
                            end
                            twdll.core.Log(string.format(
                                "[TEST]   party '%s' senators=%d power=%d%% primary=%s",
                                key, sens, pct, tostring(prim)))
                        end
                    end
                    if not fields_ok then
                        twdll.core.Log("[TEST] party fields mismatch: " .. table.concat(field_names, "; "))
                    end
                    report("PoliticalParties party fields", fields_ok)

                    -- GetPoliticalParty(key): every expected key retrievable, unknown key nil
                    local lookup_ok = true
                    for key in pairs(expected) do
                        local by_key = f:GetPoliticalParty(key)
                        if by_key == nil or by_key:GetKey() ~= key then
                            lookup_ok = false
                            twdll.core.Log(string.format(
                                "[TEST] GetPoliticalParty: FAILED for key '%s'", key))
                        end
                    end
                    if f:GetPoliticalParty("no_such_party") ~= nil then
                        lookup_ok = false
                        twdll.core.Log("[TEST] GetPoliticalParty: FAILED (unknown key returned a party)")
                    end
                    report("PoliticalParties GetPoliticalParty", lookup_ok)

                    -- GetPrimaryParty: must equal the ruler party
                    local primary = f:GetPrimaryParty()
                    local primary_ok = primary ~= nil and primary:GetKey() == "att_politics_hunni_ruler"
                        and primary:IsPrimary()
                    if not primary_ok then
                        twdll.core.Log(string.format(
                            "[TEST] GetPrimaryParty: FAILED primary=%s",
                            tostring(primary and primary:GetKey() or "<nil>")))
                    end
                    report("PoliticalParties GetPrimaryParty", primary_ok)

                    -- HasPoliticalParties: true on the politics faction
                    report("PoliticalParties HasPoliticalParties true", f:HasPoliticalParties() == true)

                    -- HasPoliticalParties: false on a faction with no parties, if one exists
                    if empty_faction ~= nil then
                        local empty_list = empty_faction:GetPoliticalPartyList()
                        local ok = empty_faction:HasPoliticalParties() == false
                            and empty_list ~= nil
                            and empty_list:num_items() == 0
                            and empty_list:is_empty()
                        if not ok then
                            twdll.core.Log(string.format(
                                "[TEST] HasPoliticalParties: FAILED (faction '%s' has no parties but returned true)",
                                tostring(empty_faction:name())))
                        end
                        report("PoliticalParties HasPoliticalParties false", ok)
                    else
                        twdll.core.Log("[TEST] HasPoliticalParties: no empty faction found, SKIPPING false check")
                        record_skip()
                    end
                end
            end
        end

        -- ======================================================
        -- TEST: Character SetDefaultBodyGuard
        -- ======================================================
        twdll.core.Log("[TEST] --- Test: Character SetDefaultBodyGuard ---")
        local bg_char = nil
        local all_chars = game:model():world():faction_by_key(faction):character_list()
        for i = 0, all_chars:num_items() - 1 do
            local c = all_chars:item_at(i)
            if c:cqi() == 205 then
                bg_char = c
                break
            end
        end

        if bg_char ~= nil then
            local test_unit_key = "att_merc_ger_agathyrsi_warriors"
            local res = bg_char:SetDefaultBodyGuard(test_unit_key)
            twdll.core.Log(string.format("[TEST] char (cqi=%d):SetDefaultBodyGuard('%s') returned: %s", bg_char:cqi(), test_unit_key, tostring(res)))
            report("character SetDefaultBodyGuard", res == true)
        else
            twdll.core.Log("[TEST] character SetDefaultBodyGuard: SKIPPED (character cqi 205 not found)")
            record_skip()
        end

        -- ======================================================
        -- TEST: Character GetInfluence / SetInfluence
        -- Character cqi=212 is expected to have influence=35 at game start.
        -- ======================================================
        twdll.core.Log("[TEST] --- Test: Character GetInfluence / SetInfluence ---")
        do
            local inf_char = nil
            local all = game:model():world():faction_by_key(faction):character_list()
            for i = 0, all:num_items() - 1 do
                local c = all:item_at(i)
                if c:cqi() == 212 then
                    inf_char = c
                    break
                end
            end

            if inf_char == nil then
                twdll.core.Log("[TEST] character influence: SKIPPED (cqi 212 not found)")
                record_skip()
                record_skip()
            else
                -- verify expected initial value
                local initial = inf_char:GetInfluence()
                twdll.core.Log("[TEST] GetInfluence cqi=212 initial = " .. tostring(initial))
                if initial == 35 then
                    twdll.core.Log("[TEST] GetInfluence initial: OK (35)")
                    report("character GetInfluence initial", true)
                else
                    twdll.core.Log("[TEST] GetInfluence initial: FAILED (expected 35, got " .. tostring(initial) .. ")")
                    report("character GetInfluence initial", false)
                end

                -- round-trip
                inf_char:SetInfluence(99)
                local after = inf_char:GetInfluence()
                twdll.core.Log("[TEST] GetInfluence after SetInfluence(99) = " .. tostring(after))
                if after == 99 then
                    twdll.core.Log("[TEST] SetInfluence round-trip: OK")
                    report("character SetInfluence round-trip", true)
                else
                    twdll.core.Log("[TEST] SetInfluence round-trip: FAILED (expected 99, got " .. tostring(after) .. ")")
                    report("character SetInfluence round-trip", false)
                end
                inf_char:SetInfluence(initial)
            end
        end

        -- ======================================================
        -- TEST 14: Slot Building Rotation (Olbia)
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 14: Slot Building Rotation (Olbia) ---")
        do
            local reg = game:model():world():region_manager():region_by_key("att_reg_sarmatia_europaea_olbia")
            local settlement = reg and reg:settlement()
            if settlement and not settlement:is_null_interface() and settlement:slot_list():num_items() > 0 then
                local slot = settlement:slot_list():item_at(0)
                if type(slot.GetBuildingRotation) ~= "function" or type(slot.SetBuildingRotation) ~= "function" then
                    twdll.core.Log("[TEST] Olbia Slot Building Rotation: FAILED (methods not registered)")
                    report("slot methods registered", false)
                else
                    report("slot methods registered", true)
                    local orig_rot = slot:GetBuildingRotation()
                    twdll.core.Log("[TEST] Olbia Slot initial rotation = " .. tostring(orig_rot))
                    local rot_valid = (orig_rot ~= nil and orig_rot >= 0 and orig_rot <= 5)
                    report("slot GetBuildingRotation valid", rot_valid)

                    -- Set rotation to 3 (180 deg) and keep it (do not restore)
                    slot:SetBuildingRotation(3)
                    local after_rot = slot:GetBuildingRotation()
                    twdll.core.Log("[TEST] Olbia Slot rotation after Set(3) = " .. tostring(after_rot))
                    report("slot SetBuildingRotation", after_rot == 3)

                    local mem_addr = slot:GetMemoryAddress()
                    local addr_ok = (type(mem_addr) == "string" and string.match(mem_addr, "^0x"))
                    twdll.core.Log("[TEST] slot GetMemoryAddress = " .. tostring(mem_addr))
                    report("slot GetMemoryAddress", addr_ok)

                    -- Trigger visual refresh
                    if type(twdll.campaign_ui.RefreshSettlements) == "function" then
                        twdll.campaign_ui.RefreshSettlements()
                        report("campaign_ui RefreshSettlements", true)
                    end
                end
            else
                twdll.core.Log("[TEST] Olbia Slot Building Rotation: SKIPPED (no settlement/slots found)")
                record_skip()
            end
        end

        -- ======================================================
        -- TEST: Political Parties & SetPrimary
        -- ======================================================
        twdll.core.Log("[TEST] --- Test: Political Parties & SetPrimary ---")
        local hun_faction = game:model():world():faction_by_key("att_fact_hunni")
        if hun_faction and type(hun_faction.GetPoliticalPartyList) == "function" then
            local parties = hun_faction:GetPoliticalPartyList()
            local num_parties = (parties and type(parties.num_items) == "function") and parties:num_items() or 0
            twdll.core.Log(string.format("[TEST] hunni political parties count = %d", num_parties))
            report("political parties num_items > 0", num_parties > 0)

            if num_parties >= 2 then
                local p0 = parties:item_at(0)
                local p1 = parties:item_at(1)
                twdll.core.Log(string.format("[TEST] Party 0: key=%s, is_primary=%s, senators=%s, power=%s",
                    tostring(p0:GetKey()), tostring(p0:IsPrimary()), tostring(p0:GetSenators()), tostring(p0:GetPower())))
                twdll.core.Log(string.format("[TEST] Party 1: key=%s, is_primary=%s, senators=%s, power=%s",
                    tostring(p1:GetKey()), tostring(p1:IsPrimary()), tostring(p1:GetSenators()), tostring(p1:GetPower())))

                local orig_primary = p0:IsPrimary() and p0 or p1
                local other_party = (orig_primary == p0) and p1 or p0

                twdll.core.Log(string.format("[TEST] Setting non-primary party '%s' as new primary...", tostring(other_party:GetKey())))
                local set_ok = other_party:SetPrimary()
                report("party SetPrimary return true", set_ok == true)
                report("party IsPrimary after SetPrimary", other_party:IsPrimary() == true and orig_primary:IsPrimary() == false)
                twdll.core.Log(string.format("[TEST] New primary is now: '%s' (IsPrimary=%s), old primary IsPrimary=%s",
                    tostring(other_party:GetKey()), tostring(other_party:IsPrimary()), tostring(orig_primary:IsPrimary())))

                -- Restore original primary party
                local restore_ok = orig_primary:SetPrimary()
                report("party SetPrimary restore return true", restore_ok == true)
                report("party IsPrimary restored", orig_primary:IsPrimary() == true and other_party:IsPrimary() == false)
                twdll.core.Log(string.format("[TEST] Primary restored to: '%s' (IsPrimary=%s)",
                    tostring(orig_primary:GetKey()), tostring(orig_primary:IsPrimary())))
            else
                twdll.core.Log("[TEST] Political Parties SetPrimary: SKIPPED (less than 2 parties)")
                record_skip()
            end
        else
            twdll.core.Log("[TEST] Political Parties: SKIPPED (GetPoliticalPartyList not available)")
            record_skip()
        end

        -- ======================================================
        -- TEST: Character Political Party (Get/SetPoliticalParty)
        -- Character cqi=212 starts in the primary/ruler party and
        -- is changed to the secondary (non-primary) party at the end.
        -- ======================================================
        twdll.core.Log("[TEST] --- Test: Character Political Party ---")
        local char_212 = nil
        local all_chars_212 = game:model():world():faction_by_key(faction):character_list()
        for i = 0, all_chars_212:num_items() - 1 do
            local c = all_chars_212:item_at(i)
            if c:cqi() == 212 then
                char_212 = c
                break
            end
        end

        if char_212 and type(char_212.GetPoliticalParty) == "function" and type(char_212.SetPoliticalParty) == "function" then
            local char_party = char_212:GetPoliticalParty()
            twdll.core.Log(string.format("[TEST] Character cqi=212 initial political party: %s (IsPrimary=%s)",
                char_party and char_party:GetKey() or "nil",
                tostring(char_party and char_party:IsPrimary())))
            report("character GetPoliticalParty valid", char_party ~= nil)
            report("character initial party is primary", char_party ~= nil and char_party:IsPrimary() == true)

            if char_party and hun_faction and type(hun_faction.GetPoliticalPartyList) == "function" then
                local parties = hun_faction:GetPoliticalPartyList()
                if parties and parties:num_items() >= 2 then
                    local p0 = parties:item_at(0)
                    local p1 = parties:item_at(1)
                    local primary_party = p0:IsPrimary() and p0 or p1
                    local secondary_party = (primary_party == p0) and p1 or p0

                    -- Set to secondary party via userdata
                    local set_ud_ok = char_212:SetPoliticalParty(secondary_party)
                    report("character SetPoliticalParty (userdata) return true", set_ud_ok == true)
                    local after_ud = char_212:GetPoliticalParty()
                    report("character GetPoliticalParty matches secondary after SetPoliticalParty (userdata)",
                        after_ud ~= nil and after_ud:GetKey() == secondary_party:GetKey() and after_ud:IsPrimary() == false)

                    -- Set to secondary party via key string and keep it as secondary
                    local set_str_ok = char_212:SetPoliticalParty(secondary_party:GetKey())
                    report("character SetPoliticalParty (key string) return true", set_str_ok == true)
                    local final_party = char_212:GetPoliticalParty()
                    report("character GetPoliticalParty matches secondary after SetPoliticalParty (key string)",
                        final_party ~= nil and final_party:GetKey() == secondary_party:GetKey() and final_party:IsPrimary() == false)
                    twdll.core.Log(string.format("[TEST] Character cqi=212 final political party: %s (IsPrimary=%s)",
                        final_party and final_party:GetKey() or "nil",
                        tostring(final_party and final_party:IsPrimary())))
                else
                    twdll.core.Log("[TEST] Character SetPoliticalParty: SKIPPED (not enough faction parties)")
                    record_skip()
                end
            else
                twdll.core.Log("[TEST] Character SetPoliticalParty: SKIPPED (char_party is nil)")
                record_skip()
            end
        else
            twdll.core.Log("[TEST] Character Political Party: SKIPPED (character 212 or methods not found)")
            record_skip()
        end

        -- ======================================================
        -- TEST 32: Campaign UI Encyclopedia URL
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 32: Campaign UI Encyclopedia URL ---")
        local orig_enc_url = twdll.campaign_ui.GetEncyclopediaUrl()
        twdll.core.Log("[TEST] Original Encyclopedia URL: " .. tostring(orig_enc_url))
        report("campaign_ui GetEncyclopediaUrl returns non-empty string", type(orig_enc_url) == "string" and orig_enc_url:len() > 0)
        report("campaign_ui GetEncyclopediaUrl starts with http", type(orig_enc_url) == "string" and orig_enc_url:find("http://") == 1)

        -- Test setting a custom URL (The Dawnless Days Encyclopedia)
        local test_custom_url = "https://encyclopedia.thedawnlessdays.com/TDD.html#"
        local set_res = twdll.campaign_ui.SetEncyclopediaUrl(test_custom_url)
        report("campaign_ui SetEncyclopediaUrl returns new url", set_res == test_custom_url)
        local read_back_url = twdll.campaign_ui.GetEncyclopediaUrl()
        report("campaign_ui GetEncyclopediaUrl matches custom url", read_back_url == test_custom_url)

        -- ======================================================
        -- TEST: Region Religions & Religion Proportion API
        -- ======================================================
        twdll.core.Log("[TEST] --- Test: Region Religions & Religion Proportion ---")
        do
            local region = game:model():world():region_manager():region_by_key("att_reg_scandza_hafn")
            if region == nil or region:is_null_interface() then
                twdll.core.Log("[TEST] Region Religions: SKIPPED (region att_reg_scandza_hafn not found)")
                record_skip()
            else
                if type(region.GetReligionList) ~= "function" or type(region.GetReligionProportion) ~= "function" then
                    twdll.core.Log("[TEST] Region Religions: FAILED (methods not registered)")
                    report("region religion methods registered", false)
                else
                    report("region religion methods registered", true)

                    local religions = region:GetReligionList()
                    local is_ud = type(religions) == "userdata"
                    local num_items = is_ud and religions:num_items() or 0
                    twdll.core.Log(string.format("[TEST] Region att_reg_scandza_hafn religions count: %d", num_items))

                    local list_valid = is_ud and num_items > 0 and not religions:is_empty()
                    report("Region GetReligionList valid", list_valid)
                    report("Region religion list is_empty false", religions:is_empty() == false)

                    local sum_proportion = 0.0
                    local found_majority = false
                    local majority_key = region:majority_religion()

                    for i = 0, num_items - 1 do
                        local rel = religions:item_at(i)
                        if rel ~= nil then
                            local r_key = rel:GetKey()
                            local r_prop = rel:GetProportion()
                            local r_icon = rel:GetIconPath()

                            sum_proportion = sum_proportion + r_prop
                            twdll.core.Log(string.format("[TEST]   religion[%d]: key='%s', proportion=%.4f, icon='%s'", i, tostring(r_key), r_prop, tostring(r_icon)))
                            if r_key == majority_key then
                                found_majority = true
                            end

                            local direct_prop = region:GetReligionProportion(r_key)
                            if math.abs(direct_prop - r_prop) > 0.001 then
                                twdll.core.Log(string.format("[TEST]   mismatch: GetReligionProportion('%s') = %.4f vs rel:GetProportion() = %.4f", r_key, direct_prop, r_prop))
                            end
                        end
                    end

                    local sum_ok = math.abs(sum_proportion - 1.0) < 0.05
                    twdll.core.Log(string.format("[TEST] Total religion proportion sum: %.4f (sum_ok=%s)", sum_proportion, tostring(sum_ok)))
                    report("Region religion proportions sum to 1.0", sum_ok)
                    report("Region majority religion present in list", found_majority)

                    report("Region religion list out of bounds nil", religions:item_at(-1) == nil and religions:item_at(999) == nil)

                    local fake_prop = region:GetReligionProportion("non_existent_religion_key")
                    report("Region non-existent religion proportion is 0", fake_prop == 0.0)
                end
            end
        end

        -- ======================================================
        -- TEST 18: twdll.cai (Campaign AI Telemetry)
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 18: twdll.cai telemetry ---")
        if type(twdll.cai) == "table" and type(twdll.cai.EnableLogging) == "function" then
            local prev_state = twdll.cai.EnableLogging()
            twdll.core.Log(string.format("[TEST] twdll.cai.EnableLogging initial state: %s", tostring(prev_state)))

            local state_false = twdll.cai.EnableLogging(false)
            local state_true = twdll.cai.EnableLogging(true)

            local cai_ok = (state_false == false) and (state_true == true)
            report("twdll.cai.EnableLogging toggle", cai_ok)
            twdll.core.Log(string.format("[TEST] twdll.cai telemetry toggle: %s", cai_ok and "OK" or "FAILED"))
        else
            twdll.core.Log("[TEST] twdll.cai module: FAILED (missing table or EnableLogging function)")
            report("twdll.cai module", false)
        end

        -- ======================================================
        -- TEST 19: Character ArtSet and Portrait API (ARTSET_SCRIPT_INTERFACE)
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 19: Character ArtSet and Portrait API ---")
        local test_char = nil
        local char_list = game:model():world():faction_by_key(faction):character_list()
        for i = 0, char_list:num_items() - 1 do
            local c = char_list:item_at(i)
            if c:cqi() == 204 then
                test_char = c
                break
            end
        end

        if not test_char then
            twdll.core.Log("[TEST] Character ArtSet: SKIPPED (character cqi 204 not found)")
            record_skip()
        else
            local art_set = test_char:GetArtSet()
            twdll.core.Log(string.format("[TEST] character cqi=%d GetArtSet()=%s",
                test_char:cqi(), tostring(art_set)))

            local art_set_ok = (art_set ~= nil) and (type(art_set) == "userdata")
            report("character:GetArtSet returns userdata", art_set_ok)

            if art_set then
                local key = art_set:GetKey()
                local culture = art_set:GetCulture()
                local subculture = art_set:GetSubculture()
                local char_faction = art_set:GetFaction()
                local agent = art_set:GetAgent()
                local group = art_set:GetGroup()
                local portrait_path = art_set:GetPortraitPath()
                local settings_id = art_set:GetSettingsId()
                local is_custom = art_set:IsCustom()
                local is_male = art_set:IsMale()
                local has_aging = art_set:HasAging()
                local has_seasonal = art_set:HasSeasonal()
                local has_levelling = art_set:HasLevelling()
                local has_health = art_set:HasHealth()
                local has_religion = art_set:HasReligion()
                local is_faction_leader = art_set:IsFactionLeaderSet()

                twdll.core.Log(string.format("[TEST] ArtSet Key: '%s' Group: '%s'", tostring(key), tostring(group)))
                twdll.core.Log(string.format("[TEST] ArtSet Culture: '%s' Subculture: '%s' Faction: '%s' Agent: '%s'",
                    tostring(culture), tostring(subculture), tostring(char_faction), tostring(agent)))
                twdll.core.Log(string.format("[TEST] Portrait Path: '%s'", tostring(portrait_path)))
                twdll.core.Log(string.format("[TEST] Settings ID: '%s'", tostring(settings_id)))
                twdll.core.Log(string.format("[TEST] Flags: custom=%s male=%s aging=%s seasonal=%s levelling=%s health=%s religion=%s fl=%s",
                    tostring(is_custom), tostring(is_male), tostring(has_aging), tostring(has_seasonal),
                    tostring(has_levelling), tostring(has_health), tostring(has_religion), tostring(is_faction_leader)))

                report("art_set:GetKey", key == "att_huns_general_01")
                report("art_set:GetCulture", culture == "att_cult_nomadic")
                report("art_set:GetSubculture", subculture == "att_sub_cult_nomadic_hunnic")
                report("art_set:GetFaction", char_faction == "att_fact_hunni")
                report("art_set:GetAgent", agent == "general")
                report("art_set:GetGroup returns string", type(group) == "string")
                report("art_set:GetSettingsId", settings_id == "att_huns_general_010")
                report("art_set:GetPortraitPath", portrait_path == "UI/Portraits/Portholes/att_cult_nomadic/att_frontend_faction_leader_huns_0.png")
                report("art_set:IsCustom", is_custom == true)
                report("art_set:IsMale", is_male == true)
                report("art_set:HasAging", has_aging == true)
                report("art_set:HasSeasonal", has_seasonal == false)
                report("art_set:HasLevelling", has_levelling == true)
                report("art_set:HasHealth returns boolean", type(has_health) == "boolean")
                report("art_set:HasReligion returns boolean", type(has_religion) == "boolean")
                report("art_set:IsFactionLeaderSet returns boolean", type(is_faction_leader) == "boolean")

                -- Find distinct valid ArtSets from other male generals in the faction
                local candidate_keys = {}
                local seen_keys = {}
                seen_keys[key] = true
                local char_list = hun_faction:character_list()
                if char_list then
                    for i = 0, char_list:num_items() - 1 do
                        local cand = char_list:item_at(i)
                        if cand and not cand:is_null_interface() and cand:cqi() ~= test_char:cqi() then
                            local cand_art = cand:GetArtSet()
                            if cand_art and cand_art:GetKey() ~= "" and cand_art:IsMale() and cand_art:GetAgent() == "general" then
                                local k = cand_art:GetKey()
                                if not seen_keys[k] then
                                    seen_keys[k] = true
                                    table.insert(candidate_keys, k)
                                end
                            end
                        end
                    end
                end
                if #candidate_keys == 0 then
                    table.insert(candidate_keys, "att_general_nomadic_16")
                end

                -- Perform multiple sequential ArtSet swaps to stress-test stability and live updates
                for idx, target_key in ipairs(candidate_keys) do
                    local swap_ok = test_char:SetArtSet(target_key)
                    twdll.core.Log(string.format("[TEST] Swap %d: SetArtSet('%s') -> %s", idx, target_key, tostring(swap_ok)))
                    report(string.format("character:SetArtSet swap %d (%s)", idx, target_key), swap_ok == true)

                    local curr_art = test_char:GetArtSet()
                    local curr_key = curr_art and curr_art:GetKey() or nil
                    local curr_portrait = curr_art and curr_art:GetPortraitPath() or nil
                    local curr_culture = curr_art and curr_art:GetCulture() or nil
                    twdll.core.Log(string.format("[TEST]   -> active key: '%s' | culture: '%s' | portrait: '%s'",
                        tostring(curr_key), tostring(curr_culture), tostring(curr_portrait)))
                    report(string.format("character:SetArtSet verified key swap %d", idx), curr_key == target_key)
                end
            end
        end

        -- ======================================================
        -- TEST 20: Disband Unit & Military Force DisbandUnits
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 20: Disband Unit & Military Force DisbandUnits ---")
        do
            local target_char = nil
            local chars = game:model():world():faction_by_key(faction):character_list()
            for i = 0, chars:num_items() - 1 do
                local c = chars:item_at(i)
                if c:military_force() and not c:military_force():is_null_interface() then
                    local ul = c:military_force():unit_list()
                    if ul and ul:num_items() >= 3 then
                        target_char = c
                        break
                    end
                end
            end

            if not target_char then
                twdll.core.Log("[TEST] Disband tests: SKIPPED (no military force with at least 3 units found)")
                record_skip()
            else
                local force = target_char:military_force()
                local initial_count = force:unit_list():num_items()
                twdll.core.Log(string.format("[TEST] Force cqi=%d initial unit count: %d", force:command_queue_index(), initial_count))

                -- Test 20a: unit:Disband() on the last unit in the force
                local last_unit = force:unit_list():item_at(initial_count - 1)
                local last_unit_ok = (last_unit ~= nil and not last_unit:is_null_interface())
                if last_unit_ok and type(last_unit.Disband) == "function" then
                    local disband_ok = last_unit:Disband()
                    twdll.core.Log(string.format("[TEST] unit:Disband() -> %s", tostring(disband_ok)))
                    report("unit:Disband returns true", disband_ok == true)

                    local count_after_unit = force:unit_list():num_items()
                    report("unit:Disband decreases unit count by 1", count_after_unit == initial_count - 1)

                    -- Test 20b: force:DisbandUnits(index) using integer index (last unit)
                    if type(force.DisbandUnits) == "function" and count_after_unit >= 2 then
                        local idx_to_disband = count_after_unit - 1
                        local force_disband_idx_ok = force:DisbandUnits(idx_to_disband)
                        twdll.core.Log(string.format("[TEST] force:DisbandUnits(%d) -> %s", idx_to_disband, tostring(force_disband_idx_ok)))
                        report("force:DisbandUnits(index) returns true", force_disband_idx_ok == true)

                        local count_after_idx = force:unit_list():num_items()
                        report("force:DisbandUnits(index) decreases unit count", count_after_idx == count_after_unit - 1)

                        -- Test 20c: force:DisbandUnits(unit_ud) using userdata
                        local next_last = force:unit_list():item_at(count_after_idx - 1)
                        if next_last and not next_last:is_null_interface() and count_after_idx >= 2 then
                            local force_disband_ud_ok = force:DisbandUnits(next_last)
                            twdll.core.Log(string.format("[TEST] force:DisbandUnits(unit_ud) -> %s", tostring(force_disband_ud_ok)))
                            report("force:DisbandUnits(userdata) returns true", force_disband_ud_ok == true)

                            local count_after_ud = force:unit_list():num_items()
                            report("force:DisbandUnits(userdata) decreases unit count", count_after_ud == count_after_idx - 1)
                        else
                            record_skip()
                        end
                    else
                        twdll.core.Log("[TEST] force:DisbandUnits(index): FAILED (method not found or not enough units)")
                        report("force:DisbandUnits method registered", false)
                    end
                else
                    twdll.core.Log("[TEST] unit:Disband: FAILED (method not found or null unit)")
                    report("unit:Disband method registered", false)
                end
            end
        end

        -- ======================================================
        -- TEST 21: Military Force Integrity / Morale API
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 21: Military Force Integrity / Morale API ---")
        do
            local test_force = nil
            local chars = game:model():world():faction_by_key(faction):character_list()
            for i = 0, chars:num_items() - 1 do
                local c = chars:item_at(i)
                if c:military_force() and not c:military_force():is_null_interface() then
                    test_force = c:military_force()
                    break
                end
            end

            if not test_force then
                twdll.core.Log("[TEST] Military Force Integrity: SKIPPED (no military force found)")
                record_skip()
            else
                if type(test_force.GetIntegrity) ~= "function" or type(test_force.SetIntegrity) ~= "function" then
                    twdll.core.Log("[TEST] Military Force Integrity: FAILED (methods not registered)")
                    report("force integrity methods registered", false)
                else
                    report("force integrity methods registered", true)

                    local has_integrity = test_force:HasIntegrity()
                    twdll.core.Log(string.format("[TEST] Force cqi=%d HasIntegrity() = %s", test_force:command_queue_index(), tostring(has_integrity)))
                    report("force:HasIntegrity returns boolean", type(has_integrity) == "boolean")

                    if has_integrity then
                        local initial_integrity = test_force:GetIntegrity()
                        twdll.core.Log(string.format("[TEST] Initial integrity = %s", tostring(initial_integrity)))
                        report("force:GetIntegrity returns number in [0, 100]", type(initial_integrity) == "number" and initial_integrity >= 0.0 and initial_integrity <= 100.0)

                        -- Test setting integrity (e.g. 75.0)
                        local set_ok = test_force:SetIntegrity(75.0)
                        twdll.core.Log(string.format("[TEST] SetIntegrity(75.0) -> %s", tostring(set_ok)))
                        report("force:SetIntegrity(75.0) returns true", set_ok == true)

                        local after_set = test_force:GetIntegrity()
                        twdll.core.Log(string.format("[TEST] Integrity after SetIntegrity(75.0) = %s", tostring(after_set)))
                        report("force:GetIntegrity verified 75.0", math.abs(after_set - 75.0) < 0.01)

                        -- Test clamping (over 100 and below 0)
                        test_force:SetIntegrity(150.0)
                        local clamped_max = test_force:GetIntegrity()
                        report("force:SetIntegrity clamps upper to 100", math.abs(clamped_max - 100.0) < 0.01)

                        test_force:SetIntegrity(-10.0)
                        local clamped_min = test_force:GetIntegrity()
                        report("force:SetIntegrity clamps lower to 0", math.abs(clamped_min - 0.0) < 0.01)

                        -- Restore initial integrity
                        test_force:SetIntegrity(initial_integrity)
                        local restored_val = test_force:GetIntegrity()
                        report("force:SetIntegrity restored initial value", math.abs(restored_val - initial_integrity) < 0.01)
                    else
                        twdll.core.Log("[TEST] Force has no integrity tracker: SKIPPING value tests")
                        record_skip()
                    end
                end
            end
        end

        -- ======================================================
        -- TEST 22: World Max Character Traits Limit API
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 22: World Max Character Traits Limit API ---")
        do
            if type(twdll.world.GetMaxTraits) ~= "function" or type(twdll.world.SetMaxTraits) ~= "function" then
                twdll.core.Log("[TEST] Max Traits API: FAILED (methods not found)")
                report("world max traits methods registered", false)
            else
                report("world max traits methods registered", true)

                local initial_max_traits = twdll.world.GetMaxTraits()
                twdll.core.Log(string.format("[TEST] Initial GetMaxTraits() = %s", tostring(initial_max_traits)))
                report("twdll.world.GetMaxTraits returns number", type(initial_max_traits) == "number")
                report("twdll.world.GetMaxTraits initial value is 10", initial_max_traits == 10)

                -- Test setting to 30
                twdll.world.SetMaxTraits(30)
                local modified_traits = twdll.world.GetMaxTraits()
                twdll.core.Log(string.format("[TEST] GetMaxTraits() after SetMaxTraits(30) = %s", tostring(modified_traits)))
                report("twdll.world.SetMaxTraits(30) verified", modified_traits == 30)

                -- Test minimum clamp
                twdll.world.SetMaxTraits(0)
                local clamped_traits = twdll.world.GetMaxTraits()
                report("twdll.world.SetMaxTraits clamps < 1 to 1", clamped_traits == 1)

                -- Restore default 10 via no-arg
                twdll.world.SetMaxTraits()
                local restored_traits = twdll.world.GetMaxTraits()
                report("twdll.world.SetMaxTraits() restored to 10", restored_traits == 10)
            end
        end

        -- ======================================================
        -- TEST 23: Character Trait Manipulation API (AddTrait, RemoveTrait, GetTraitList)
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 23: Character Trait Manipulation API ---")
        do
            local test_char = char
            if not test_char then
                local chars = game:model():world():faction_by_key(faction):character_list()
                if chars:num_items() > 0 then
                    test_char = chars:item_at(0)
                end
            end

            if not test_char then
                twdll.core.Log("[TEST] Character Trait API: SKIPPED (no character found)")
                record_skip()
            else
                if type(test_char.AddTrait) ~= "function" or type(test_char.RemoveTrait) ~= "function" or type(test_char.GetTraitList) ~= "function" then
                    twdll.core.Log("[TEST] Character Trait API: FAILED (methods not found on character)")
                    report("character trait methods registered", false)
                else
                    report("character trait methods registered", true)

                    local fname = (type(test_char.get_forename) == "function") and tostring(test_char:get_forename()) or "unknown"
                    local sname = (type(test_char.get_surname) == "function") and tostring(test_char:get_surname()) or ""
                    twdll.core.Log(string.format("[TEST] Testing traits on character cqi=%d ('%s %s')", test_char:cqi(), fname, sname))

                    -- 1. Inspect initial traits
                    local initial_traits = test_char:GetTraitList()
                    twdll.core.Log(string.format("[TEST] Initial trait count via GetTraitList: %d", #initial_traits))
                    report("char:GetTraitList returns table", type(initial_traits) == "table")
                    for i, t in ipairs(initial_traits) do
                        twdll.core.Log(string.format("[TEST]   Trait [%d]: %s", i, tostring(t)))
                    end

                    if #initial_traits > 0 then
                        -- 2. Test RemoveTrait on a real verified trait
                        local trait_to_remove = initial_traits[#initial_traits]
                        twdll.core.Log(string.format("[TEST] Selected trait to remove: %s", tostring(trait_to_remove)))
                        local removed = test_char:RemoveTrait(trait_to_remove)
                        twdll.core.Log(string.format("[TEST] char:RemoveTrait('%s') result = %s", tostring(trait_to_remove), tostring(removed)))
                        report("char:RemoveTrait returned true", removed == true)

                        -- 3. Verify trait is gone from GetTraitList
                        local after_remove_traits = test_char:GetTraitList()
                        local still_present = false
                        for _, t in ipairs(after_remove_traits) do
                            if t == trait_to_remove then still_present = true break end
                        end
                        report("char:RemoveTrait verified absent from GetTraitList", not still_present)
                        report("char:GetTraitList count decremented by 1", #after_remove_traits == #initial_traits - 1)

                        -- 4. Test AddTrait to restore the verified trait
                        local added = test_char:AddTrait(trait_to_remove, 1, false)
                        twdll.core.Log(string.format("[TEST] char:AddTrait('%s') result = %s", tostring(trait_to_remove), tostring(added)))
                        report("char:AddTrait returned true", added == true)

                        -- ======================================================
                        -- 5. Test Exceeding Vanilla 10-Trait Cap (> 10 traits)
                        -- ======================================================
                        twdll.core.Log("[TEST] --- Testing Trait Limit Increase (> 10 traits) ---")
                        twdll.world.SetMaxTraits(20)

                        -- Harvest valid trait keys from characters across the campaign world
                        local trait_pool = {}
                        local seen_traits = {}
                        for _, tk in ipairs(test_char:GetTraitList()) do
                            seen_traits[tk] = true
                        end

                        local all_factions = game:model():world():faction_list()
                        for fi = 0, all_factions:num_items() - 1 do
                            local f_obj = all_factions:item_at(fi)
                            local fc_list = f_obj:character_list()
                            for ci = 0, fc_list:num_items() - 1 do
                                local fc = fc_list:item_at(ci)
                                local ctraits = fc:GetTraitList()
                                for _, tk in ipairs(ctraits) do
                                    if not seen_traits[tk] then
                                        seen_traits[tk] = true
                                        table.insert(trait_pool, tk)
                                    end
                                end
                            end
                            if #trait_pool >= 20 then break end
                        end

                        twdll.core.Log(string.format("[TEST] Harvested %d valid candidate traits from campaign world", #trait_pool))

                        -- Add traits until we exceed 10 (target: 14 traits)
                        local current_traits = test_char:GetTraitList()
                        local target_count = 14
                        for _, new_trait in ipairs(trait_pool) do
                            if #current_traits >= target_count then break end
                            test_char:AddTrait(new_trait, 1, false)
                            current_traits = test_char:GetTraitList()
                        end

                        local final_traits = test_char:GetTraitList()
                        twdll.core.Log(string.format("[TEST] Character trait count after expansion: %d (exceeds vanilla 10 cap!)", #final_traits))
                        report("character trait count exceeds vanilla cap of 10", #final_traits > 10)
                        report("character reached target trait count (14)", #final_traits >= 14)

                        twdll.core.Log("[TEST] === Character Final Trait List (Visual Inspection) ===")
                        for i, t in ipairs(final_traits) do
                            twdll.core.Log(string.format("[TEST]   [%02d] %s", i, tostring(t)))
                        end

                        -- ======================================================
                        -- 6. Remove ALL traits (Visual verification: character will have 0 traits in game)
                        -- ======================================================
                        twdll.core.Log("[TEST] --- Removing ALL traits from character (Clean Slate Test) ---")
                        local traits_to_clean = test_char:GetTraitList()
                        twdll.core.Log(string.format("[TEST] Purging all %d traits from character...", #traits_to_clean))
                        for _, tk in ipairs(traits_to_clean) do
                            test_char:RemoveTrait(tk)
                        end

                        local empty_traits = test_char:GetTraitList()
                        twdll.core.Log(string.format("[TEST] Character trait count after total purge: %d", #empty_traits))
                        report("all traits successfully removed (count == 0)", #empty_traits == 0)
                    else
                        twdll.core.Log("[TEST] Character has 0 traits: SKIPPING remove/add cycle")
                        record_skip()
                    end
                end
            end
        end

        -- ======================================================
        -- TEST 24: Character Loyalty API (GetLoyalty, GetLoyaltyModifier, SetLoyaltyModifier, GetLoyaltyFactorList)
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 24: Character Loyalty API ---")
        do
            local test_char = char
            if not test_char then
                local chars = game:model():world():faction_by_key(faction):character_list()
                if chars:num_items() > 0 then
                    test_char = chars:item_at(0)
                end
            end

            if not test_char then
                twdll.core.Log("[TEST] Character Loyalty API: SKIPPED (no character found)")
                record_skip()
            else
                if type(test_char.GetLoyalty) ~= "function" or type(test_char.GetLoyaltyModifier) ~= "function"
                   or type(test_char.SetLoyaltyModifier) ~= "function" or type(test_char.GetLoyaltyFactorList) ~= "function" then
                    twdll.core.Log("[TEST] Character Loyalty API: FAILED (methods not registered)")
                    report("character loyalty methods registered", false)
                else
                    report("character loyalty methods registered", true)

                    local initial_loyalty = test_char:GetLoyalty()
                    local initial_mod = test_char:GetLoyaltyModifier()
                    twdll.core.Log(string.format("[TEST] Character cqi=%d initial loyalty = %d/10, modifier = %+d",
                        test_char:cqi(), initial_loyalty, initial_mod))
                    report("char:GetLoyalty returns integer in [0, 10]", type(initial_loyalty) == "number" and initial_loyalty >= 0 and initial_loyalty <= 10)
                    report("char:GetLoyaltyModifier returns integer", type(initial_mod) == "number")

                    -- Inspect loyalty factors
                    local factors = test_char:GetLoyaltyFactorList()
                    twdll.core.Log("[TEST] === Active Loyalty Factors ===")
                    local factor_count = 0
                    for k, v in pairs(factors) do
                        factor_count = factor_count + 1
                        twdll.core.Log(string.format("[TEST]   Factor '%s' = %+d", tostring(k), v))
                    end
                    twdll.core.Log(string.format("[TEST] Total active loyalty factors: %d", factor_count))
                    report("char:GetLoyaltyFactorList returns table", type(factors) == "table")

                    -- Test setting negative modifier (-10)
                    local set_neg_ok = test_char:SetLoyaltyModifier(-10)
                    report("char:SetLoyaltyModifier(-10) returns true", set_neg_ok == true)
                    report("char:GetLoyaltyModifier verifies -10", test_char:GetLoyaltyModifier() == -10)
                    local low_loyalty = test_char:GetLoyalty()
                    twdll.core.Log(string.format("[TEST] Loyalty after modifier -10 = %d/10", low_loyalty))
                    report("loyalty decreased or clamped at 0 with negative modifier", low_loyalty <= initial_loyalty)

                    -- Test setting positive modifier (+10)
                    local set_pos_ok = test_char:SetLoyaltyModifier(10)
                    report("char:SetLoyaltyModifier(10) returns true", set_pos_ok == true)
                    report("char:GetLoyaltyModifier verifies 10", test_char:GetLoyaltyModifier() == 10)
                    local high_loyalty = test_char:GetLoyalty()
                    twdll.core.Log(string.format("[TEST] Loyalty after modifier +10 = %d/10", high_loyalty))
                    report("loyalty increased or clamped at 10 with positive modifier", high_loyalty >= initial_loyalty)

                    -- Finally, set character loyalty to 0 (-100 modifier) to observe in-game state
                    twdll.core.Log(string.format("[TEST] Priming character cqi=%d to 0 loyalty (modifier = -100)...", test_char:cqi()))
                    test_char:SetLoyaltyModifier(-100)
                    local zero_loyalty = test_char:GetLoyalty()
                    twdll.core.Log(string.format("[TEST] Final character loyalty = %d/10 (modifier = %d)", zero_loyalty, test_char:GetLoyaltyModifier()))
                    report("character final loyalty is 0", zero_loyalty == 0)
                end
            end
        end

        -- ======================================================
        -- TEST 25: Character Transfer to Faction (TransferToFaction)
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 25: Character Transfer to Faction ---")
        do
            local w_fac = game:model():world():faction_by_key(faction)
            local ere_fac = game:model():world():faction_by_key("att_fact_eastern_roman_empire")
            local rebel_fac = nil
            local all_facs = game:model():world():faction_list()
            for i = 0, all_facs:num_items() - 1 do
                local f = all_facs:item_at(i)
                local fn = f:name()
                if string.find(fn, "separatist") or string.find(fn, "rebel") then
                    rebel_fac = f
                    break
                end
            end
            if not rebel_fac or rebel_fac:is_null_interface() then
                rebel_fac = game:model():world():faction_by_key("att_fact_separatists_ere")
            end

            local p_char = w_fac:character_list():item_at(0)
            local reg = p_char:has_region() and p_char:region() or (ere_fac:has_home_region() and ere_fac:home_region() or ere_fac:region_list():item_at(0))
            local reg_name = reg:name()
            local spawn_x = p_char:logical_position_x() + 2
            local spawn_y = p_char:logical_position_y() + 2

            local spawn_seq = 0
            local function spawn_general_for_test(id_tag, target_fac_key)
                spawn_seq = spawn_seq + 1
                local target_f_key = target_fac_key or faction
                local fac_obj = game:model():world():faction_by_key(target_f_key)
                if not fac_obj or fac_obj:is_null_interface() then return nil end
                local pre_chars = {}
                for i = 0, fac_obj:character_list():num_items() - 1 do
                    pre_chars[fac_obj:character_list():item_at(i):cqi()] = true
                end
                local tag = id_tag or ("twdll_gen_" .. tostring(spawn_seq))
                local u_key = (target_f_key == "att_fact_eastern_roman_empire") and "att_rom_legio" or "att_merc_ger_agathyrsi_warriors"
                local target_reg = (target_f_key == "att_fact_eastern_roman_empire" and ere_fac:has_home_region()) and ere_fac:home_region() or reg
                local r_name = target_reg:name()
                local s_x = (target_f_key == "att_fact_eastern_roman_empire" and target_reg:settlement()) and target_reg:settlement():logical_position_x() + 2 + spawn_seq or spawn_x + spawn_seq
                local s_y = (target_f_key == "att_fact_eastern_roman_empire" and target_reg:settlement()) and target_reg:settlement():logical_position_y() + 2 + spawn_seq or spawn_y + spawn_seq
                game:create_force(target_f_key, u_key, r_name, s_x, s_y, tag, false)
                for i = 0, fac_obj:character_list():num_items() - 1 do
                    local c = fac_obj:character_list():item_at(i)
                    if not pre_chars[c:cqi()] and c:has_military_force() then
                        return c
                    end
                end
                -- Fallback to another non-leader general if create_force was queued asynchronously
                for i = 0, fac_obj:character_list():num_items() - 1 do
                    local c = fac_obj:character_list():item_at(i)
                    if not c:is_faction_leader() and c:has_military_force() then
                        return c
                    end
                end
                return nil
            end

            local test_char1 = spawn_general_for_test("twdll_tf_1")
            if not test_char1 or type(test_char1.TransferToFaction) ~= "function" then
                twdll.core.Log("[TEST] TransferToFaction: FAILED (failed to spawn test character or method not registered)")
                report("character TransferToFaction method registered", false)
            else
                report("character TransferToFaction method registered", true)

                -- Variation 1: Basic call without options table
                local ok_1 = test_char1:TransferToFaction(ere_fac)
                twdll.core.Log(string.format("[TEST] TransferToFaction basic call: %s", tostring(ok_1)))
                report("TransferToFaction basic call returns true", ok_1 == true)
                report("character 1 faction changed to ERE", test_char1:faction():name() == "att_fact_eastern_roman_empire")

                -- Variation 2: Options Table with replenish_units = true
                local test_char2 = spawn_general_for_test("twdll_tf_2")
                if test_char2 then
                    local force = test_char2:military_force()
                    local u_list = force and force:unit_list()
                    if u_list and u_list:num_items() > 0 then
                        for ui = 0, u_list:num_items() - 1 do
                            local u = u_list:item_at(ui)
                            if type(u.SetNumMen) == "function" then
                                u:SetNumMen(15)
                            end
                        end
                        local sample_u = u_list:item_at(0)
                        twdll.core.Log(string.format("[TEST] Force before transfer damaged: unit[0] men=%d/%d", sample_u:GetNumMen(), sample_u:GetMaxNumMen()))
                        report("unit damaged before transfer (15 men)", sample_u:GetNumMen() == 15)
                    end

                    local ok_2 = test_char2:TransferToFaction(ere_fac, { replenish_units = true })
                    twdll.core.Log(string.format("[TEST] TransferToFaction options table { replenish_units = true }: %s", tostring(ok_2)))
                    report("TransferToFaction options table { replenish_units = true } returns true", ok_2 == true)
                    report("character 2 faction changed to ERE", test_char2:faction():name() == "att_fact_eastern_roman_empire")

                    if u_list and u_list:num_items() > 0 then
                        local sample_u_after = u_list:item_at(0)
                        local full_men = sample_u_after:GetNumMen()
                        local max_men = sample_u_after:GetMaxNumMen()
                        twdll.core.Log(string.format("[TEST] Force after transfer replenished: unit[0] men=%d/%d", full_men, max_men))
                        report("unit fully replenished after transfer (men == max_men)", full_men == max_men and full_men > 15)
                    end
                end

                -- Variation 3: Options Table with rebel_region (regional rebel army creation on settled ERE force)
                local ere_home_reg = ere_fac:has_home_region() and ere_fac:home_region() or reg
                local test_char3 = spawn_general_for_test("twdll_tf_3", "att_fact_eastern_roman_empire")
                if test_char3 and rebel_fac and not rebel_fac:is_null_interface() then
                    local ok_3 = test_char3:TransferToFaction(rebel_fac, { rebel_region = ere_home_reg, replenish_units = true })
                    twdll.core.Log(string.format("[TEST] TransferToFaction options table { rebel_region, replenish_units }: %s", tostring(ok_3)))
                    report("TransferToFaction options table { rebel_region } returns true", ok_3 == true)
                    report("character 3 faction changed to rebels", test_char3:faction():name() == rebel_fac:name())
                end

                -- Variation 4: Native Hunnic Family Tree General Transfer (Specific general cqi=208)
                local hun_family_char = nil
                for i = 0, w_fac:character_list():num_items() - 1 do
                    local c = w_fac:character_list():item_at(i)
                    if c:cqi() == 208 then
                        hun_family_char = c
                        break
                    end
                end
                if hun_family_char then
                    local cqi = hun_family_char:cqi()
                    twdll.core.Log(string.format("[TEST] Transferring native Hunnic starting general (cqi=%d) to ERE...", cqi))
                    local ok_4 = hun_family_char:TransferToFaction(ere_fac)
                    twdll.core.Log(string.format("[TEST] Hunnic family general (cqi=%d) TransferToFaction result: %s", cqi, tostring(ok_4)))
                    report("Hunnic family general TransferToFaction returns true", ok_4 == true)
                    report("Hunnic family general faction changed to ERE", hun_family_char:faction():name() == "att_fact_eastern_roman_empire")
                end
            end
        end

        -- ======================================================
        -- TEST 26: Faction Create Agent API (CreateAgent)
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 26: Faction Create Agent ---")
        do
            local ere_fac = game:model():world():faction_by_key("att_fact_eastern_roman_empire")
            if type(ere_fac.CreateAgent) ~= "function" then
                twdll.core.Log("[TEST] CreateAgent: FAILED (method not registered)")
                report("faction CreateAgent method registered", false)
            else
                report("faction CreateAgent method registered", true)

                local initial_char_count = ere_fac:character_list():num_items()
                twdll.core.Log(string.format("[TEST] Initial ERE character count: %d", initial_char_count))

                local initial_cqis = {}
                for i = 0, initial_char_count - 1 do
                    initial_cqis[ere_fac:character_list():item_at(i):cqi()] = true
                end

                -- 1. Test spawning a champion in ERE capital settlement
                local capital_settlement = ere_fac:home_region():settlement()
                local create_champ_ok = ere_fac:CreateAgent("champion", capital_settlement)
                twdll.core.Log(string.format("[TEST] ere_fac:CreateAgent('champion', settlement) -> %s", tostring(create_champ_ok)))
                report("ere_fac:CreateAgent('champion', settlement) returns true", create_champ_ok == true)

                local spawned_champ = nil
                for i = 0, ere_fac:character_list():num_items() - 1 do
                    local c = ere_fac:character_list():item_at(i)
                    if not initial_cqis[c:cqi()] then
                        spawned_champ = c
                        initial_cqis[c:cqi()] = true
                        break
                    end
                end

                report("spawned champion character object resolved", spawned_champ ~= nil)
                if spawned_champ then
                    twdll.core.Log(string.format("[TEST] Spawned Champion: cqi=%d, is_champion=%s, pos=(%d, %d), region='%s'",
                        spawned_champ:cqi(), tostring(spawned_champ:character_type("champion")),
                        spawned_champ:logical_position_x(), spawned_champ:logical_position_y(),
                        spawned_champ:has_region() and spawned_champ:region():name() or "none"))
                    report("spawned champion has type 'champion'", spawned_champ:character_type("champion") == true)
                    report("spawned champion in capital region", spawned_champ:has_region() and spawned_champ:region():name() == ere_fac:home_region():name())
                end

                -- 2. Test spawning a dignitary using map coordinates (x, y)
                local target_x = 516
                local target_y = 381
                local create_dig_ok = ere_fac:CreateAgent("dignitary", target_x, target_y)
                twdll.core.Log(string.format("[TEST] ere_fac:CreateAgent('dignitary', %d, %d) -> %s", target_x, target_y, tostring(create_dig_ok)))
                report("ere_fac:CreateAgent('dignitary', x, y) returns true", create_dig_ok == true)

                local spawned_dig = nil
                for i = 0, ere_fac:character_list():num_items() - 1 do
                    local c = ere_fac:character_list():item_at(i)
                    if not initial_cqis[c:cqi()] then
                        spawned_dig = c
                        initial_cqis[c:cqi()] = true
                        break
                    end
                end

                report("spawned dignitary character object resolved", spawned_dig ~= nil)
                if spawned_dig then
                    twdll.core.Log(string.format("[TEST] Spawned Dignitary: cqi=%d, is_dignitary=%s, pos=(%d, %d), region='%s'",
                        spawned_dig:cqi(), tostring(spawned_dig:character_type("dignitary")),
                        spawned_dig:logical_position_x(), spawned_dig:logical_position_y(),
                        spawned_dig:has_region() and spawned_dig:region():name() or "none"))
                    report("spawned dignitary has type 'dignitary'", spawned_dig:character_type("dignitary") == true)
                end

                local final_char_count = ere_fac:character_list():num_items()
                twdll.core.Log(string.format("[TEST] Final ERE Character count: %d (started with %d)", final_char_count, initial_char_count))
                report("faction character count increased by 2", final_char_count == initial_char_count + 2)
            end
        end

        -- ======================================================
        -- TEST: Character Names, Localisation Keys, and Immortality API
        -- ======================================================
        twdll.core.Log("[TEST] --- Test: Character Names, Localisation Keys, and Immortality API ---")
        do
            local f = game:model():world():faction_by_key(faction)
            local char = f and f:faction_leader() or nil
            if not char then
                twdll.core.Log("[TEST] Character Names: SKIPPED (no faction leader)")
                record_skip()
            else
                -- 1. Verify method registrations
                report("GetForenameKey method registered", type(char.GetForenameKey) == "function")
                report("SetForenameKey method registered", type(char.SetForenameKey) == "function")
                report("GetFamilyNameKey method registered", type(char.GetFamilyNameKey) == "function")
                report("SetFamilyNameKey method registered", type(char.SetFamilyNameKey) == "function")
                report("GetClanNameKey method registered", type(char.GetClanNameKey) == "function")
                report("SetClanNameKey method registered", type(char.SetClanNameKey) == "function")
                report("GetOtherNameKey method registered", type(char.GetOtherNameKey) == "function")
                report("SetOtherNameKey method registered", type(char.SetOtherNameKey) == "function")

                -- 2. Initial state reading
                local initial_fullname = char:GetFullName()
                local initial_fn = char:GetForename()
                local initial_fn_key = char:GetForenameKey()
                twdll.core.Log(string.format("[TEST] Initial FullName: '%s', Forename: '%s', ForenameKey: '%s'",
                    tostring(initial_fullname), tostring(initial_fn), tostring(initial_fn_key)))
                report("GetFullName returns string", type(initial_fullname) == "string")
                report("GetForename returns string", type(initial_fn) == "string")
                report("GetForenameKey returns string", type(initial_fn_key) == "string")

                -- 3. Cross-Test: Custom Text vs DB Localisation Key switching
                -- 3a. Setting custom forename clears DB key
                local set_fn_ok = char:SetForename("Witch-king")
                local fn_after_custom = char:GetForename()
                local fn_key_after_custom = char:GetForenameKey()
                twdll.core.Log(string.format("[TEST] SetForename('Witch-king') -> Forename: '%s', ForenameKey: '%s'",
                    tostring(fn_after_custom), tostring(fn_key_after_custom)))
                report("SetForename returns true", set_fn_ok == true)
                report("GetForename matches custom name", fn_after_custom == "Witch-king")
                report("GetForenameKey is empty after custom text", fn_key_after_custom == "")

                -- 3b. Setting DB key clears custom text and switches mode
                local set_fn_key_ok = char:SetForenameKey("names_name_custom_test_1001")
                local fn_key_after_key = char:GetForenameKey()
                local fn_after_key = char:GetForename()
                twdll.core.Log(string.format("[TEST] SetForenameKey('names_name_custom_test_1001') -> ForenameKey: '%s', Forename: '%s'",
                    tostring(fn_key_after_key), tostring(fn_after_key)))
                report("SetForenameKey returns true", set_fn_key_ok == true)
                report("GetForenameKey matches set key", fn_key_after_key == "names_name_custom_test_1001")
                report("GetForename returns key string as fallback", fn_after_key == "names_name_custom_test_1001")

                -- 4. Cross-Test: Slot Independence (modifying one slot never mutates other slots)
                -- 4a. Initialize all 4 slots with distinct names
                char:SetForename("Aragorn")
                char:SetClanName("Dunadan")
                char:SetFamilyName("Telcontar")
                char:SetOtherName("Elessar")

                report("Forename initialized to Aragorn", char:GetForename() == "Aragorn")
                report("ClanName initialized to Dunadan", char:GetClanName() == "Dunadan")
                report("FamilyName initialized to Telcontar", char:GetFamilyName() == "Telcontar")
                report("OtherName initialized to Elessar", char:GetOtherName() == "Elessar")
                report("GetFullName matches initial 4-slot composite", char:GetFullName() == "Aragorn Dunadan Telcontar Elessar")

                -- 4b. Mutate ONLY FamilyName -> verify Forename, ClanName, OtherName are unchanged
                char:SetFamilyName("Isildur")
                report("FamilyName updated to Isildur", char:GetFamilyName() == "Isildur")
                report("Forename untouched after FamilyName change", char:GetForename() == "Aragorn")
                report("ClanName untouched after FamilyName change", char:GetClanName() == "Dunadan")
                report("OtherName untouched after FamilyName change", char:GetOtherName() == "Elessar")

                -- 4c. Mutate ONLY ClanName -> verify Forename, FamilyName, OtherName are unchanged
                char:SetClanName("Ranger")
                report("ClanName updated to Ranger", char:GetClanName() == "Ranger")
                report("Forename untouched after ClanName change", char:GetForename() == "Aragorn")
                report("FamilyName untouched after ClanName change", char:GetFamilyName() == "Isildur")
                report("OtherName untouched after ClanName change", char:GetOtherName() == "Elessar")

                -- 4d. Mutate ONLY OtherName -> verify Forename, ClanName, FamilyName are unchanged
                char:SetOtherName("Strider")
                report("OtherName updated to Strider", char:GetOtherName() == "Strider")
                report("Forename untouched after OtherName change", char:GetForename() == "Aragorn")
                report("ClanName untouched after OtherName change", char:GetClanName() == "Ranger")
                report("FamilyName untouched after OtherName change", char:GetFamilyName() == "Isildur")

                -- 4e. Verify Composite Full Name after targeted slot mutations
                local composite_full = char:GetFullName()
                twdll.core.Log(string.format("[TEST] Composite FullName: '%s'", tostring(composite_full)))
                report("GetFullName matches mutated composite order", composite_full == "Aragorn Ranger Isildur Strider")

                -- 5. Cross-Test: DB Localisation Keys on all slots
                local set_fam_key_ok = char:SetFamilyNameKey("names_name_fam_key_2002")
                local set_clan_key_ok = char:SetClanNameKey("names_name_clan_key_3003")
                local set_on_key_ok = char:SetOtherNameKey("names_titles_other_key_4004")

                report("SetFamilyNameKey returns true", set_fam_key_ok == true)
                report("GetFamilyNameKey matches set key", char:GetFamilyNameKey() == "names_name_fam_key_2002")
                report("SetClanNameKey returns true", set_clan_key_ok == true)
                report("GetClanNameKey matches set key", char:GetClanNameKey() == "names_name_clan_key_3003")
                report("SetOtherNameKey returns true", set_on_key_ok == true)
                report("GetOtherNameKey matches set key", char:GetOtherNameKey() == "names_titles_other_key_4004")

                -- 6. Immortality & Resurrection Turns
                local imm_before = char:IsImmortal()
                twdll.core.Log(string.format("[TEST] Initial IsImmortal: %s", tostring(imm_before)))
                report("IsImmortal returns boolean", type(imm_before) == "boolean")

                local set_imm_ok = char:SetImmortal(true)
                local imm_after = char:IsImmortal()
                twdll.core.Log(string.format("[TEST] SetImmortal(true) -> %s (IsImmortal: %s)", tostring(set_imm_ok), tostring(imm_after)))
                report("SetImmortal returns true", set_imm_ok == true)
                report("IsImmortal matches set true", imm_after == true)

                char:SetResurrectionTurns(5)
                local res_turns = char:GetResurrectionTurns()
                twdll.core.Log(string.format("[TEST] SetResurrectionTurns(5) -> GetResurrectionTurns: %d", res_turns))
                report("GetResurrectionTurns matches 5", res_turns == 5)

                -- Restore health and immortality
                char:SetResurrectionTurns(0)
                char:SetImmortal(imm_before)
            end
        end

        -- ======================================================
        -- TEST 29: Game Lifecycle & Save/Load API Functions Existence
        -- ======================================================
        twdll.core.Log("[TEST] --- Test 29: Game Lifecycle & Save/Load API Functions Existence ---")
        do
            report("twdll.world.SaveGame is function", type(twdll.world.SaveGame) == "function")
            report("twdll.world.LoadGame is function", type(twdll.world.LoadGame) == "function")
            report("twdll.world.ExitToMainMenu is function", type(twdll.world.ExitToMainMenu) == "function")
            report("twdll.world.ExitGame is function", type(twdll.world.ExitGame) == "function")
        end

        -- ======================================================
        -- TEST 30: Technology Status API (GetTechnologyStatus / SetTechnologyStatus)
        -- ======================================================
        do
            twdll.core.Log("[TEST] ----- Test 30: Technology Status API -----")
            local fac_obj = game:model():world():faction_by_key(faction)
            if fac_obj then
                report("faction:GetTechnologyStatus is function", type(fac_obj.GetTechnologyStatus) == "function")
                report("faction:SetTechnologyStatus is function", type(fac_obj.SetTechnologyStatus) == "function")
                report("faction:InstantlyResearchTechnology is function", type(fac_obj.InstantlyResearchTechnology) == "function")

                -- 1. Instantly research 4 Hunnic military technologies
                fac_obj:InstantlyResearchTechnology("att_hunnic_military_militarised_massing_of_power")
                fac_obj:InstantlyResearchTechnology("att_hunnic_military_traditions_of_mobility")
                fac_obj:InstantlyResearchTechnology("att_hunnic_military_extra_military_provisions")
                fac_obj:InstantlyResearchTechnology("att_hunnic_military_speed_of_attack")

                local s1 = fac_obj:GetTechnologyStatus("att_hunnic_military_militarised_massing_of_power")
                local s2 = fac_obj:GetTechnologyStatus("att_hunnic_military_traditions_of_mobility")
                local s3 = fac_obj:GetTechnologyStatus("att_hunnic_military_extra_military_provisions")
                local s4 = fac_obj:GetTechnologyStatus("att_hunnic_military_speed_of_attack")

                twdll.core.Log(string.format("[TEST] Researched tech statuses: s1=%s s2=%s s3=%s s4=%s", tostring(s1), tostring(s2), tostring(s3), tostring(s4)))
                report("Tech 1 status is RESEARCHED (0)", s1 == 0)
                report("Tech 2 status is RESEARCHED (0)", s2 == 0)
                report("Tech 3 status is RESEARCHED (0)", s3 == 0)
                report("Tech 4 status is RESEARCHED (0)", s4 == 0)

                -- 2. Explicitly set supply_acquisition to UNAVAILABLE (4)
                local set_ok = fac_obj:SetTechnologyStatus("att_hunnic_military_supply_acquisition", 4)
                local s5 = fac_obj:GetTechnologyStatus("att_hunnic_military_supply_acquisition")
                twdll.core.Log(string.format("[TEST] SetTechnologyStatus returned %s, status=%s", tostring(set_ok), tostring(s5)))
                report("SetTechnologyStatus returns true", set_ok == true)
                report("Tech 5 status is UNAVAILABLE (4)", s5 == 4)
            else
                record_skip()
                record_skip()
                record_skip()
                record_skip()
                record_skip()
                record_skip()
                record_skip()
                record_skip()
                record_skip()
            end
        end

        -- ======================================================
        -- SUMMARY
        -- ======================================================
        twdll.core.Log("[TEST] ===== TEST SUMMARY =====")
        twdll.core.Log(string.format("[TEST] PASSED: %d   FAILED: %d   SKIPPED: %d", passed, failed, skipped))
        if failed == 0 then
            twdll.core.Log("[TEST] ===== ALL TESTS PASSED =====")
            twdll.core.Log("[TEST] Final Result: SUCCESS")

            -- ======================================================
            -- POST-TEST LIFECYCLE: Save -> Load (Only when all tests passed and no-save flag not set)
            -- ======================================================
            local no_save_flag = io.open("twdll_no_save_reload.flag", "r")
            if no_save_flag then
                no_save_flag:close()
                twdll.core.Log("[TEST] [LIFECYCLE] twdll_no_save_reload.flag detected — skipping SaveGame / LoadGame reload sequence.")
            else
                local function schedule_callback(func, delay_seconds)
                    if cm and type(cm.callback) == "function" then
                        cm:callback(func, delay_seconds)
                    elseif game and type(game.callback) == "function" then
                        game:callback(func, delay_seconds)
                    else
                        func()
                    end
                end

                schedule_callback(function()
                    twdll.core.Log("[TEST] [LIFECYCLE] All unit tests passed! Starting SaveGame & LoadGame verification...")
                    twdll.core.Log("[TEST] [LIFECYCLE] Step 1: Saving campaign to 'twdll_lifecycle_test' (after 1.0s delay)...")
                    local save_ok = twdll.world.SaveGame("twdll_lifecycle_test")
                    twdll.core.Log(string.format("[TEST] [LIFECYCLE] Save result: %s", tostring(save_ok)))
                    if save_ok then
                        local f_write = io.open(marker_file, "w")
                        if f_write then
                            f_write:write("reload_pending\n")
                            f_write:close()
                        end
                        schedule_callback(function()
                            twdll.core.Log("[TEST] [LIFECYCLE] Step 2: Requesting engine to load 'twdll_lifecycle_test' (after 1.0s save flush)...")
                            twdll.world.LoadGame("twdll_lifecycle_test")
                        end, 1.0)
                    else
                        twdll.core.Log("[TEST] [LIFECYCLE] FAILED: SaveGame returned false!")
                    end
                end, 1.0)
            end
        else
            twdll.core.Log("[TEST] !!! THERE WERE FAILURES !!!")
            for _, n in ipairs(failed_names) do
                twdll.core.Log("[TEST]   - " .. n)
            end
            twdll.core.Log("[TEST] ===== TESTS DONE (WITH FAILURES) =====")
            twdll.core.Log("[TEST] Final Result: FAILED")
        end
        twdll.world.SetMaxUnitsInArmy(20)

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
        twdll.core.Log(ok and "[TEST] run_twdll_tests returned normally" or ("[TEST] run_twdll_tests error: " .. tostring(err)))
    end)
end)

if err then
    twdll.core.Log("Added event had error: " .. tostring(err))
end
