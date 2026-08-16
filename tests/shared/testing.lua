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
                local ok = src_unit:ConvertUnit("att_merc_ger_agathyrsi_warriors")
                if ok ~= true then
                    twdll.core.Log("[TEST] ConvertUnit: FAILED (returned " .. tostring(ok) .. ")")
                    report("ConvertUnit", false)
                else
                    local unit_after = char:military_force():unit_list():item_at(src_index)
                    local men_after = unit_after:GetNumMen()
                    local key_after = unit_after:unit_key()
                    twdll.core.Log("[TEST] ConvertUnit: men " .. tostring(men_before) ..
                        " -> " .. tostring(men_after) .. ", key '" .. tostring(key_after) .. "'")
                    if men_after == men_before and key_after == "att_merc_ger_agathyrsi_warriors" then
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
                        local parties = cand:GetPoliticalParties()
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

                    -- GetPoliticalParties: exactly the two known parties
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
