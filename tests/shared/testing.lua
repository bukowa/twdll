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
                twdll.core.Log(string.format("[TEST] picked character cqi=%d (first with a military force)", c:cqi()))
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

                    -- GetPoliticalPartyList: exactly the two known parties + alias check
                    local list_ok = type(parties) == "userdata"
                        and parties:num_items() == 2
                        and not parties:is_empty()
                        and parties:item_at(-1) == nil
                        and parties:item_at(999) == nil
                        and f:GetPoliticalParties():num_items() == 2
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
                        local empty_list = empty_faction:GetPoliticalParties()
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
        if hun_faction and type(hun_faction.GetPoliticalParties) == "function" then
            local parties = hun_faction:GetPoliticalParties()
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
            twdll.core.Log("[TEST] Political Parties: SKIPPED (GetPoliticalParties not available)")
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

            if char_party and hun_faction and type(hun_faction.GetPoliticalParties) == "function" then
                local parties = hun_faction:GetPoliticalParties()
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

                    local list_valid = is_ud and num_items > 0 and not religions:is_empty() and region:GetReligions():num_items() == num_items
                    report("Region GetReligionList valid", list_valid)
                    report("Region religion list is_empty false", religions:is_empty() == false)

                    local sum_proportion = 0.0
                    local found_majority = false
                    local majority_key = region:majority_religion()
                    local aliases_ok = true

                    for i = 0, num_items - 1 do
                        local rel = religions:item_at(i)
                        if rel ~= nil then
                            local r_key = rel:GetKey()
                            local r_prop = rel:GetProportion()
                            local r_icon = rel:GetIconPath()

                            -- Test aliases (lowercase)
                            if rel:key() ~= r_key or rel:proportion() ~= r_prop or rel:icon_path() ~= r_icon then
                                aliases_ok = false
                            end

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

                    report("Region religion methods and aliases match", aliases_ok)

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
        -- SUMMARY
        -- ======================================================
        twdll.core.Log("[TEST] ===== TEST SUMMARY =====")
        twdll.core.Log(string.format("[TEST] PASSED: %d   FAILED: %d   SKIPPED: %d", passed, failed, skipped))
        if failed == 0 then
            twdll.core.Log("[TEST] ===== ALL TESTS PASSED =====")
            twdll.core.Log("[TEST] Final Result: SUCCESS")
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
