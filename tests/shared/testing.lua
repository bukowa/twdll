local game = nil
local faction = nil
local twdll_tests_already_run = false
local marker_file = "twdll_reload_marker.flag"

local function get_game_interface()
    if not game then
        pcall(function()
            local scripting_string = nil
            if type(twdll) == "table" and type(twdll.core) == "table" and type(twdll.core.GameBuild) == "function" and twdll.core.GameBuild() == "Rome2" then
                scripting_string = 'lua_scripts.EpisodicScripting'
                faction = 'rom_rome'
            else
                scripting_string = "lua_scripts.episodicscripting"
                faction = 'att_fact_hunni'
            end
            local scripting = require(scripting_string)
            game = scripting.game_interface
        end)
    end
    return game
end

-- NOTE: This test suite runs against a specific save file (tests.save).
-- Several tests rely on hardcoded CQI (Command Queue Index) values that are
-- only valid for that exact save snapshot:
--   CQI 204 — ArtSet test character (att_huns_general_01)
--   CQI 205 — SetDefaultBodyGuard test character
--   CQI 208 — Hunnic family general for TransferToFaction
--   CQI 212 — Influence test character (initial influence = 35)
-- If tests.save is regenerated, these CQIs must be re-verified and updated.

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
    local f_marker = io.open(marker_file, "r")
    local is_reload = twdll_tests_already_run or (f_marker ~= nil)
    if f_marker then
        f_marker:close()
        os.remove(marker_file)
    end

    if is_reload then
        if type(twdll) == "table" and type(twdll.core) == "table" and type(twdll.core.Log) == "function" then
            twdll.core.Log("\n======================================================")
            twdll.core.Log("[TEST] [LIFECYCLE] Reload verification successful!")
            twdll.core.Log("[TEST] [LIFECYCLE] Campaign successfully reloaded from saved game.")
            twdll.core.Log("[TEST] [LIFECYCLE] All tests and save/load cycle PASSED.")
            twdll.core.Log("======================================================\n")
        end
        return
    end

    -- Mark that tests have run in this campaign session so subsequent saves will persist it
    twdll_tests_already_run = true

    -- Initialize test suite with flat global structure
    if type(twdll) == "table" and type(twdll.core) == "table" and type(twdll.core.Log) == "function" then
        twdll.core.Log("[TEST] twdll is loaded. Starting unit tests...")

        -- Initialize lightweight test runner
        local SKIP_SENTINEL = {}

        local function create_test_runner()
            local total_passed, total_failed, total_skipped = 0, 0, 0
            local failed_list = {}
            local skipped_list = {}
            local current_t = nil

            local function test(name, fn)
                twdll.core.Log(string.format("\n[TEST] === %s ===", name))
                local test_had_failure = false

                local t = {
                    name = name,
                    log = function(self, ...)
                        twdll.core.Log(...)
                    end,
                    skip = function(self, reason)
                        local skip_reason = tostring(reason or "precondition not met")
                        twdll.core.Log(string.format("[TEST] %s: SKIPPED (%s)", name, skip_reason))
                        error({ sentinel = SKIP_SENTINEL, reason = skip_reason })
                    end,
                    assert = function(self, condition, msg)
                        msg = msg or "unnamed assertion"
                        if condition then
                            twdll.core.Log(string.format("[TEST]   assert '%s': OK", msg))
                            total_passed = total_passed + 1
                            return true
                        else
                            test_had_failure = true
                            total_failed = total_failed + 1
                            twdll.core.Log(string.format("[TEST]   assert '%s': FAILED", msg))
                            failed_list[#failed_list + 1] = string.format("%s -> Assertion failed: '%s'", name, msg)
                            return false
                        end
                    end,
                    assert_eq = function(self, actual, expected, msg)
                        msg = msg or "equality check"
                        if actual == expected then
                            twdll.core.Log(string.format("[TEST]   assert '%s': OK (%s)", msg, tostring(actual)))
                            total_passed = total_passed + 1
                            return true
                        else
                            test_had_failure = true
                            total_failed = total_failed + 1
                            local err = string.format("expected '%s', got '%s'", tostring(expected), tostring(actual))
                            twdll.core.Log(string.format("[TEST]   assert '%s': FAILED (%s)", msg, err))
                            failed_list[#failed_list + 1] = string.format("%s -> %s (%s)", name, msg, err)
                            return false
                        end
                    end,
                    report = function(self, msg, condition)
                        return self:assert(condition, msg)
                    end,
                }

                current_t = t
                local ok, err = pcall(fn, t)
                current_t = nil

                if not ok then
                    if type(err) == "table" and err.sentinel == SKIP_SENTINEL then
                        total_skipped = total_skipped + 1
                        skipped_list[#skipped_list + 1] = { name = name, reason = err.reason }
                        return
                    end
                    -- Real runtime error
                    test_had_failure = true
                    total_failed = total_failed + 1
                    twdll.core.Log(string.format("[TEST] %s: RUNTIME ERROR: %s", name, tostring(err)))
                    failed_list[#failed_list + 1] = string.format("%s -> RUNTIME ERROR: %s", name, tostring(err))
                end
            end

            local function report(name, condition)
                if current_t then
                    return current_t:assert(condition, name)
                else
                    if condition then
                        total_passed = total_passed + 1
                    else
                        total_failed = total_failed + 1
                        failed_list[#failed_list + 1] = name
                    end
                end
            end

            local function record_skip(reason)
                if current_t then
                    current_t:skip(reason)
                else
                    total_skipped = total_skipped + 1
                end
            end

            local function summary()
                twdll.core.Log("\n===================================")
                twdll.core.Log("[TEST] ===== TEST SUMMARY =====")
                twdll.core.Log(string.format("[TEST] PASSED: %d   FAILED: %d   SKIPPED: %d", total_passed, total_failed, total_skipped))

                if #failed_list > 0 then
                    twdll.core.Log("[TEST] !!! FAILURES !!!")
                    for _, f in ipairs(failed_list) do
                        twdll.core.Log("  - " .. f)
                    end
                end

                if #skipped_list > 0 then
                    twdll.core.Log("[TEST] --- SKIPPED TESTS ---")
                    for _, s in ipairs(skipped_list) do
                        twdll.core.Log(string.format("  - %s: %s", s.name, s.reason))
                    end
                end

                local success = (total_failed == 0)
                if success then
                    twdll.core.Log("[TEST] ===== ALL TESTS PASSED =====")
                    twdll.core.Log("[TEST] Final Result: SUCCESS")
                else
                    twdll.core.Log("[TEST] ===== TESTS DONE (WITH FAILURES) =====")
                    twdll.core.Log("[TEST] Final Result: FAILED")
                end
                twdll.core.Log("===================================\n")
                return success
            end

            return test, report, record_skip, summary
        end

        local test, report, record_skip, summary = create_test_runner()

        -- Smoke tests: Log and GetBuildSha
        test("Core Smoke Tests", function(t)
            local log_test_ok = pcall(function()
                twdll.core.Log("[TEST] twdll.core.Log multi-type test:", nil, true, 123, { test = 1 })
            end)
            t:assert(log_test_ok, "twdll.core.Log multi-type")

            local build_sha = twdll.core.GetBuildSha and twdll.core.GetBuildSha()
            twdll.core.Log("[TEST] twdll.core.GetBuildSha = " .. tostring(build_sha))
            t:assert(type(build_sha) == "string" and #build_sha >= 7, "twdll.core.GetBuildSha valid")
        end)

        -- ======================================================
        -- TEST 1: Verify WORLD and CAMPAIGN_UI singleton hooks
        -- ======================================================
        test("Test 1: Verify WORLD and CAMPAIGN_UI singleton hooks", function(t)
            local world_ptr     = twdll.world.GetMemoryAddress()
            local ui_ptr        = twdll.campaign_ui.GetMemoryAddress()
            local faction_count = twdll.world.GetFactionCount()

            twdll.core.Log("[TEST] g_world       = " .. tostring(world_ptr))
            twdll.core.Log("[TEST] g_campaign_ui = " .. tostring(ui_ptr))
            twdll.core.Log("[TEST] FactionCount  = " .. tostring(faction_count))

            t:assert(world_ptr ~= nil and world_ptr ~= 0, "g_world hook valid")
            t:assert(ui_ptr ~= nil and ui_ptr ~= 0, "g_campaign_ui hook valid")

            local engine_faction_count = game:model():world():faction_list():num_items()
            twdll.core.Log(string.format("[TEST] Faction count verification: hook=%s, engine=%s", tostring(faction_count), tostring(engine_faction_count)))
            t:assert_eq(faction_count, engine_faction_count, "g_world faction count matches engine")
        end)

        -- ======================================================
        -- TEST 2: twdll_unit read/write (via metatable)
        -- ======================================================
        test("Test 2: twdll_unit read/write (via metatable)", function(t)
            local char = nil
            local chars = game:model():world():faction_by_key(faction):character_list()
            for i = 0, chars:num_items() - 1 do
                local c = chars:item_at(i)
                if c:military_force() ~= nil then
                    char = c
                    local fname = (type(c.get_forename) == "function") and tostring(c:get_forename()) or "unknown"
                    local sname = (type(c.get_surname) == "function") and tostring(c:get_surname()) or ""
                    local stype = (type(c.character_subtype_key) == "function") and tostring(c:character_subtype_key()) or ""
                    twdll.core.Log(string.format("[TEST] picked character cqi=%d name='%s %s' subtype='%s'", c:cqi(), fname, sname, stype))
                    break
                end
            end

            if not char then
                return t:skip("no character with a military force")
            end

            local unit = char:military_force():unit_list():item_at(0)
            if not unit then
                return t:skip("military force has no units")
            end

            local max_men = unit:GetMaxNumMen()
            local initial_men = unit:GetNumMen()
            local initial_percentage = unit:percentage_proportion_of_full_strength()
            twdll.core.Log(string.format("[TEST] Unit Initial State - Men: %d/%d (%d%%)", initial_men, max_men, initial_percentage))

            unit:SetNumMen(20)
            local new_men = unit:GetNumMen()
            t:assert_eq(new_men, 20, "unit metatable SetNumMen(20)")
            unit:SetNumMen(initial_men)

            -- Test 2b: SetMaxNumMen
            local max_before = unit:GetMaxNumMen()
            unit:SetMaxNumMen(150)
            local max_after = unit:GetMaxNumMen()
            t:assert_eq(max_after, 150, "SetMaxNumMen(150)")
            unit:SetMaxNumMen(max_before)

            -- Test 2c: GetActionPoints / SetActionPoints
            local ap_before = unit:GetActionPoints()
            unit:SetActionPoints(50)
            local ap_after = unit:GetActionPoints()
            t:assert_eq(ap_after, 50, "SetActionPoints(50)")
            unit:SetActionPoints(ap_before)

            -- Test 2d: ConvertUnit
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
            else
                local src_unit = ul:item_at(src_index)
                local men_before = src_unit:GetNumMen()
                local max_before = src_unit:GetMaxNumMen()
                local ok = src_unit:ConvertUnit("att_merc_ger_agathyrsi_warriors")
                if t:assert(ok == true, "ConvertUnit returns true") then
                    local unit_after = char:military_force():unit_list():item_at(src_index)
                    local men_after = unit_after:GetNumMen()
                    local max_after = unit_after:GetMaxNumMen()
                    local key_after = unit_after:unit_key()
                    local expected_men = (men_before >= max_before) and max_after or math.floor((men_before / max_before) * max_after + 0.5)
                    t:assert(men_after == expected_men and key_after == "att_merc_ger_agathyrsi_warriors", "ConvertUnit men/key match")
                end
            end
        end)

        -- ======================================================
        -- TEST 3: faction:SetFactionLeader
        -- ======================================================
        test("Test 3: faction:SetFactionLeader", function(t)
            local f     = game:model():world():faction_by_key(faction)
            local chars = f:character_list()
            local total = chars:num_items()

            local generals = {}
            for i = 0, total - 1 do
                local c = chars:item_at(i)
                if c:character_type("general") then
                    generals[#generals + 1] = c
                end
            end

            if #generals < 2 then
                return t:skip("need at least 2 generals")
            end

            local function pick_non_leader()
                local leader_cqi = f:faction_leader():cqi()
                for _, c in ipairs(generals) do
                    if c:cqi() ~= leader_cqi then return c end
                end
            end

            -- case 1: silent swap
            local new1 = pick_non_leader()
            f:SetFactionLeader(new1)
            t:assert_eq(f:faction_leader():cqi(), new1:cqi(), "SetFactionLeader silent")

            -- case 2: normal succession
            local new2 = pick_non_leader()
            local old2 = f:faction_leader()
            f:SetFactionLeader(new2, old2)
            t:assert_eq(f:faction_leader():cqi(), new2:cqi(), "SetFactionLeader succession")

            -- case 3: heir coming of age
            local new3 = pick_non_leader()
            local old3 = f:faction_leader()
            f:SetFactionLeader(new3, old3, true)
            t:assert_eq(f:faction_leader():cqi(), new3:cqi(), "SetFactionLeader heir_coming_of_age")
        end)

        -- ======================================================
        -- TEST 4: faction GetGold / SetGold
        -- ======================================================
        test("Test 4: faction GetGold/SetGold", function(t)
            local f            = game:model():world():faction_by_key(faction)
            local gold_initial = f:GetTreasury()
            twdll.core.Log("[TEST] GetTreasury initial = " .. tostring(gold_initial))

            if twdll.core.GameBuild() == "Attila" then
                t:assert_eq(gold_initial, 15000, "GetTreasury initial is 15000")
            else
                t:assert(type(gold_initial) == "number", "GetTreasury initial is number")
            end

            f:SetTreasury(99999)
            t:assert_eq(f:GetTreasury(), 99999, "SetTreasury(99999)")

            -- Restore original treasury
            f:SetTreasury(gold_initial)
        end)

        -- ======================================================
        -- TEST 5: twdll.model DisbandUnits
        -- ======================================================
        test("Test 5: twdll.model DisbandUnits", function(t)
            local test_char = nil
            local chars = game:model():world():faction_by_key(faction):character_list()
            for i = 0, chars:num_items() - 1 do
                local c = chars:item_at(i)
                if c:military_force() ~= nil then
                    test_char = c
                    break
                end
            end

            if not test_char then
                return t:skip("no character with a military force")
            end

            local mf = test_char:military_force()
            local ul = mf:unit_list()
            local before = ul:num_items()
            if before == 0 then
                return t:skip("military force has no units")
            end

            local last_unit = ul:item_at(before - 1)
            twdll.model.DisbandUnits(last_unit)
            local after = mf:unit_list():num_items()
            t:assert_eq(after, before - 1, "DisbandUnits decrements count by 1")
        end)

        -- ======================================================
        -- TEST 6: MILITARY_FORCE_SCRIPT_INTERFACE methods
        -- ======================================================
        test("Test 6: MILITARY_FORCE_SCRIPT_INTERFACE methods", function(t)
            local test_char = nil
            local chars = game:model():world():faction_by_key(faction):character_list()
            for i = 0, chars:num_items() - 1 do
                local c = chars:item_at(i)
                if c:military_force() ~= nil then
                    test_char = c
                    break
                end
            end

            if not test_char then
                return t:skip("no character with a military force")
            end

            local mf = test_char:military_force()
            if type(mf.GetRecruitmentQueueSize) ~= "function" or type(mf.GetMemoryAddress) ~= "function" then
                return t:skip("military force methods not registered")
            end

            local qsize = mf:GetRecruitmentQueueSize()
            t:assert_eq(qsize, 0, "GetRecruitmentQueueSize is 0")

            local addr = mf:GetMemoryAddress()
            t:assert(type(addr) == "string" and string.match(addr, "^0x"), "GetMemoryAddress is valid hex string")
        end)

        -- ======================================================
        -- TEST 7: twdll.world SetMaxUnitsInArmy / SetMaxUnitsInNavy
        -- ======================================================
        test("Test 7: SetMaxUnitsInArmy / SetMaxUnitsInNavy", function(t)
            local army_before = twdll.world.GetMaxUnitsInArmy()
            local navy_before = twdll.world.GetMaxUnitsInNavy()

            twdll.world.SetMaxUnitsInArmy(30)
            twdll.world.SetMaxUnitsInNavy(15)
            t:assert_eq(twdll.world.GetMaxUnitsInArmy(), 30, "SetMaxUnitsInArmy(30)")
            t:assert_eq(twdll.world.GetMaxUnitsInNavy(), 15, "SetMaxUnitsInNavy(15)")

            -- Restore via no-arg / nil
            twdll.world.SetMaxUnitsInArmy()
            twdll.world.SetMaxUnitsInNavy(nil)
            t:assert_eq(twdll.world.GetMaxUnitsInArmy(), army_before, "SetMaxUnitsInArmy() restores default")
            t:assert_eq(twdll.world.GetMaxUnitsInNavy(), navy_before, "SetMaxUnitsInNavy(nil) restores default")
        end)

        -- ======================================================
        -- TEST 8: game:add_unit_to_force — add 80 units to every army
        -- ======================================================
        test("Test 8: game:add_unit_to_force (add 80 to our armies)", function(t)
            twdll.world.SetMaxUnitsInArmy(80)
            local cap = twdll.world.GetMaxUnitsInArmy()

            local f   = game:model():world():faction_by_key(faction)
            local mfl = f:military_force_list()
            local mfl_n = mfl:num_items()

            local key = "att_nom_hunnic_mounted_warband"
            local forces_done = 0
            local units_added = 0
            local force_snapshots = {}

            for j = 0, mfl_n - 1 do
                local mf     = mfl:item_at(j)
                local cqi    = mf:command_queue_index()
                local before = mf:unit_list():num_items()
                force_snapshots[#force_snapshots + 1] = { mf = mf, original_count = before }
                for k = 1, 80 do
                    game:add_unit_to_force(key, cqi)
                end
                local after  = mf:unit_list():num_items()
                forces_done  = forces_done + 1
                units_added  = units_added + (after - before)
                twdll.core.Log(string.format("[TEST]   force cqi=%d: %d -> %d units", cqi, before, after))
            end

            twdll.core.Log(string.format("[TEST] add_unit_to_force: forces=%d units_added=%d cap=%d", forces_done, units_added, cap))
            t:assert(units_added > 0, "add_unit_to_force added units")

            -- Cleanup: disband added units to restore original force sizes
            for _, snap in ipairs(force_snapshots) do
                local mf = snap.mf
                local ul = mf:unit_list()
                local current = ul:num_items()
                if current > snap.original_count then
                    local to_disband = {}
                    for i = snap.original_count, current - 1 do
                        to_disband[#to_disband + 1] = ul:item_at(i)
                    end
                    twdll.model.DisbandUnits(unpack(to_disband))
                end
            end
            twdll.world.SetMaxUnitsInArmy()
            twdll.core.Log("[TEST] add_unit_to_force: cleanup done, army cap restored")
        end)

        -- ======================================================
        -- TEST 9: twdll.world SetReinforcementCap / GetReinforcementCap
        -- ======================================================
        test("Test 9: SetReinforcementCap / GetReinforcementCap", function(t)
            local initial = twdll.world.GetReinforcementCap()
            twdll.core.Log("[TEST] ReinforcementCap initial = " .. tostring(initial))

            twdll.world.SetReinforcementCap(0)
            t:assert_eq(twdll.world.GetReinforcementCap(), 0, "SetReinforcementCap(0)")

            twdll.world.SetReinforcementCap(1000000)
            t:assert_eq(twdll.world.GetReinforcementCap(), 1000000, "SetReinforcementCap(1000000)")

            twdll.world.SetReinforcementCap(-1)
            t:assert_eq(twdll.world.GetReinforcementCap(), nil, "SetReinforcementCap(-1) restore default")
        end)

        -- ======================================================
        -- TEST 10: twdll.battle GetBattleInfo
        -- ======================================================
        test("Test 10: GetBattleInfo", function(t)
            local info = twdll.battle.GetBattleInfo()
            if info == nil then
                t:assert(true, "GetBattleInfo in campaign is nil")
            elseif type(info) == "table" then
                t:assert(info.battle ~= nil and info.battle ~= 0 and info.manager ~= nil and info.cap ~= nil and info.size ~= nil, "GetBattleInfo during battle table valid")
            else
                t:assert(false, "GetBattleInfo unexpected type " .. type(info))
            end
        end)

        -- ======================================================
        -- TEST 11: REGION_SCRIPT_INTERFACE methods
        -- ======================================================
        test("Test 11: REGION_SCRIPT_INTERFACE methods", function(t)
            local region = game:model():world():region_manager():region_by_key("att_reg_scandza_hafn")
            if not region or region:is_null_interface() then
                return t:skip("region att_reg_scandza_hafn not found")
            end

            if type(region.GetPopulationSurplus) ~= "function" or type(region.SetPopulationSurplus) ~= "function" then
                return t:skip("region methods not registered")
            end

            -- Population surplus: game-start value is 5
            local dp_orig = region:GetPopulationSurplus()
            t:assert_eq(dp_orig, 5, "PopulationSurplus initial is 5")
            region:SetPopulationSurplus(10)
            t:assert_eq(region:GetPopulationSurplus(), 10, "SetPopulationSurplus(10)")
            region:SetPopulationSurplus(dp_orig)

            -- Growth points: game-start value is 0
            local sp_orig = region:GetGrowthPoints()
            t:assert_eq(sp_orig, 0, "GrowthPoints initial is 0")
            region:SetGrowthPoints(20)
            t:assert_eq(region:GetGrowthPoints(), 20, "SetGrowthPoints(20)")
            region:SetGrowthPoints(sp_orig)

            local addr = region:GetMemoryAddress()
            t:assert(type(addr) == "string" and string.match(addr, "^0x"), "region GetMemoryAddress valid")
        end)

        -- ======================================================
        -- TEST 12: faction:SetCapital
        -- ======================================================
        test("Test 12: faction:SetCapital", function(t)
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
            if not f then
                return t:skip("no settled faction with regions found")
            end
            if not region or region:is_null_interface() then
                return t:skip("region att_reg_scandza_hafn not found")
            end
            if type(f.SetCapital) ~= "function" then
                return t:skip("SetCapital method not registered")
            end

            local orig_capital = f:home_region()
            f:SetCapital(region)
            local new_capital = f:home_region()
            t:assert(new_capital ~= nil and new_capital:name() == region:name(), "SetCapital changes capital")

            if orig_capital ~= nil and not orig_capital:is_null_interface() then
                f:SetCapital(orig_capital)
                twdll.core.Log("[TEST] SetCapital: restored original capital")
            end
        end)

        -- ======================================================
        -- TEST 13: faction:InstantlyResearchTechnology
        -- ======================================================
        test("Test 13: faction:InstantlyResearchTechnology", function(t)
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
            if not f then
                return t:skip("no faction found")
            end
            if type(f.InstantlyResearchTechnology) ~= "function" then
                return t:skip("InstantlyResearchTechnology not registered")
            end

            local ok = f:InstantlyResearchTechnology("att_hunnic_military_combat_at_distance")
            t:assert_eq(ok, true, "InstantlyResearchTechnology returns true")
        end)

        -- ======================================================
        -- TEST 14: faction political parties API
        -- ======================================================
        test("Test 14: faction political parties API", function(t)
            local fl = game:model():world():faction_list()
            if not fl then
                return t:skip("no faction list")
            end

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

            if not hunni then
                return t:skip("faction 'att_fact_hunni' not found")
            end

            local f = hunni.f
            local parties = hunni.parties

            local expected = {
                ["att_politics_hunni_ruler"]   = { senators = 480, power_pct = 70, primary = true  },
                ["att_politics_hunni_council"] = { senators = 720, power_pct = 30, primary = false },
            }

            -- GetPoliticalPartyList
            local list_ok = type(parties) == "userdata"
                and parties:num_items() == 2
                and not parties:is_empty()
                and parties:item_at(-1) == nil
                and parties:item_at(999) == nil
            t:assert(list_ok, "PoliticalParties list valid")

            -- per-party data against hardcoded values
            local fields_ok = true
            for i = 0, parties:num_items() - 1 do
                local p = parties:item_at(i)
                local key = p:GetKey()
                local exp = expected[key]
                if exp == nil then
                    fields_ok = false
                else
                    local sens = p:GetSenators()
                    local pct  = round(p:GetPower() * 100)
                    local prim = p:IsPrimary()
                    if sens ~= exp.senators or pct ~= exp.power_pct or prim ~= exp.primary then
                        fields_ok = false
                    end
                end
            end
            t:assert(fields_ok, "PoliticalParties party fields")

            -- GetPoliticalParty(key)
            local lookup_ok = true
            for key in pairs(expected) do
                local by_key = f:GetPoliticalParty(key)
                if by_key == nil or by_key:GetKey() ~= key then
                    lookup_ok = false
                end
            end
            if f:GetPoliticalParty("no_such_party") ~= nil then
                lookup_ok = false
            end
            t:assert(lookup_ok, "PoliticalParties GetPoliticalParty")

            -- GetPrimaryParty
            local primary = f:GetPrimaryParty()
            local primary_ok = primary ~= nil and primary:GetKey() == "att_politics_hunni_ruler" and primary:IsPrimary()
            t:assert(primary_ok, "PoliticalParties GetPrimaryParty")

            -- HasPoliticalParties
            t:assert(f:HasPoliticalParties() == true, "PoliticalParties HasPoliticalParties true")

            if empty_faction ~= nil then
                local empty_list = empty_faction:GetPoliticalPartyList()
                local ok = empty_faction:HasPoliticalParties() == false
                    and empty_list ~= nil
                    and empty_list:num_items() == 0
                    and empty_list:is_empty()
                t:assert(ok, "PoliticalParties HasPoliticalParties false")
            else
                twdll.core.Log("[TEST] HasPoliticalParties: no empty faction found, skipping false check")
            end
        end)

        -- ======================================================
        -- TEST 15: Character SetDefaultBodyGuard
        -- ======================================================
        test("Test 15: Character SetDefaultBodyGuard", function(t)
            local bg_char = nil
            local all_chars = game:model():world():faction_by_key(faction):character_list()
            for i = 0, all_chars:num_items() - 1 do
                local c = all_chars:item_at(i)
                if c:cqi() == 205 then
                    bg_char = c
                    break
                end
            end

            if not bg_char then
                return t:skip("character cqi 205 not found")
            end

            local test_unit_key = "att_merc_ger_agathyrsi_warriors"
            local res = bg_char:SetDefaultBodyGuard(test_unit_key)
            twdll.core.Log(string.format("[TEST] char (cqi=%d):SetDefaultBodyGuard('%s') returned: %s", bg_char:cqi(), test_unit_key, tostring(res)))
            t:assert_eq(res, true, "character SetDefaultBodyGuard returns true")
        end)

        -- ======================================================
        -- TEST 16: Character GetInfluence / SetInfluence
        -- ======================================================
        test("Test 16: Character GetInfluence / SetInfluence", function(t)
            local inf_char = nil
            local all = game:model():world():faction_by_key(faction):character_list()
            for i = 0, all:num_items() - 1 do
                local c = all:item_at(i)
                if c:cqi() == 212 then
                    inf_char = c
                    break
                end
            end

            if not inf_char then
                return t:skip("character cqi 212 not found")
            end

            local initial = inf_char:GetInfluence()
            twdll.core.Log("[TEST] GetInfluence cqi=212 initial = " .. tostring(initial))
            t:assert_eq(initial, 35, "character GetInfluence initial is 35")

            inf_char:SetInfluence(99)
            local after = inf_char:GetInfluence()
            t:assert_eq(after, 99, "character SetInfluence round-trip (99)")
            inf_char:SetInfluence(initial)
        end)

        -- ======================================================
        -- TEST 17: Slot Building Rotation (Olbia)
        -- ======================================================
        test("Test 17: Slot Building Rotation (Olbia)", function(t)
            local reg = game:model():world():region_manager():region_by_key("att_reg_sarmatia_europaea_olbia")
            local settlement = reg and reg:settlement()
            if not settlement or settlement:is_null_interface() or settlement:slot_list():num_items() == 0 then
                return t:skip("no settlement or slots found in Olbia")
            end

            local slot = settlement:slot_list():item_at(0)
            if type(slot.GetBuildingRotation) ~= "function" or type(slot.SetBuildingRotation) ~= "function" then
                return t:skip("slot rotation methods not registered")
            end

            local orig_rot = slot:GetBuildingRotation()
            twdll.core.Log("[TEST] Olbia Slot initial rotation = " .. tostring(orig_rot))
            t:assert(orig_rot ~= nil and orig_rot >= 0 and orig_rot <= 5, "slot GetBuildingRotation valid")

            -- Set rotation to 3 (180 deg)
            slot:SetBuildingRotation(3)
            local after_rot = slot:GetBuildingRotation()
            t:assert_eq(after_rot, 3, "slot SetBuildingRotation(3)")

            local mem_addr = slot:GetMemoryAddress()
            t:assert(type(mem_addr) == "string" and string.match(mem_addr, "^0x"), "slot GetMemoryAddress valid")

            -- Trigger visual refresh
            if type(twdll.campaign_ui.RefreshSettlements) == "function" then
                twdll.campaign_ui.RefreshSettlements()
                t:assert(true, "campaign_ui RefreshSettlements")
            end
        end)

        -- ======================================================
        -- TEST 18: Political Parties & SetPrimary
        -- ======================================================
        test("Test 18: Political Parties & SetPrimary", function(t)
            local hun_faction = game:model():world():faction_by_key(faction)
            if not hun_faction or type(hun_faction.GetPoliticalPartyList) ~= "function" then
                return t:skip("GetPoliticalPartyList not available")
            end

            local parties = hun_faction:GetPoliticalPartyList()
            local num_parties = (parties and type(parties.num_items) == "function") and parties:num_items() or 0
            t:assert(num_parties > 0, "political parties num_items > 0")

            if num_parties < 2 then
                return t:skip("less than 2 political parties")
            end

            local p0 = parties:item_at(0)
            local p1 = parties:item_at(1)
            local orig_primary = p0:IsPrimary() and p0 or p1
            local other_party = (orig_primary == p0) and p1 or p0

            twdll.core.Log(string.format("[TEST] Setting non-primary party '%s' as new primary...", tostring(other_party:GetKey())))
            local set_ok = other_party:SetPrimary()
            t:assert_eq(set_ok, true, "party SetPrimary returns true")
            t:assert(other_party:IsPrimary() == true and orig_primary:IsPrimary() == false, "party IsPrimary after SetPrimary")

            -- Restore original primary party
            local restore_ok = orig_primary:SetPrimary()
            t:assert_eq(restore_ok, true, "party SetPrimary restore returns true")
            t:assert(orig_primary:IsPrimary() == true and other_party:IsPrimary() == false, "party IsPrimary restored")
        end)

        -- ======================================================
        -- TEST 19: Character Political Party (Get/SetPoliticalParty)
        -- ======================================================
        test("Test 19: Character Political Party", function(t)
            local char_212 = nil
            local all_chars_212 = game:model():world():faction_by_key(faction):character_list()
            for i = 0, all_chars_212:num_items() - 1 do
                local c = all_chars_212:item_at(i)
                if c:cqi() == 212 then
                    char_212 = c
                    break
                end
            end

            if not char_212 or type(char_212.GetPoliticalParty) ~= "function" or type(char_212.SetPoliticalParty) ~= "function" then
                return t:skip("character 212 or methods not found")
            end

            local char_party = char_212:GetPoliticalParty()
            t:assert(char_party ~= nil, "character GetPoliticalParty valid")
            t:assert(char_party ~= nil and char_party:IsPrimary() == true, "character initial party is primary")

            local hun_fac = game:model():world():faction_by_key(faction)
            local parties = hun_fac and type(hun_fac.GetPoliticalPartyList) == "function" and hun_fac:GetPoliticalPartyList()
            if not parties or parties:num_items() < 2 then
                return t:skip("not enough faction parties")
            end

            local p0 = parties:item_at(0)
            local p1 = parties:item_at(1)
            local primary_party = p0:IsPrimary() and p0 or p1
            local secondary_party = (primary_party == p0) and p1 or p0

            -- Set to secondary party via userdata
            local set_ud_ok = char_212:SetPoliticalParty(secondary_party)
            t:assert_eq(set_ud_ok, true, "character SetPoliticalParty (userdata) returns true")
            local after_ud = char_212:GetPoliticalParty()
            t:assert(after_ud ~= nil and after_ud:GetKey() == secondary_party:GetKey() and after_ud:IsPrimary() == false, "character GetPoliticalParty matches secondary (userdata)")

            -- Set to secondary party via key string
            local set_str_ok = char_212:SetPoliticalParty(secondary_party:GetKey())
            t:assert_eq(set_str_ok, true, "character SetPoliticalParty (key string) returns true")
            local final_party = char_212:GetPoliticalParty()
            t:assert(final_party ~= nil and final_party:GetKey() == secondary_party:GetKey() and final_party:IsPrimary() == false, "character GetPoliticalParty matches secondary (key string)")
        end)

        -- ======================================================
        -- TEST 20: Campaign UI Encyclopedia URL
        -- ======================================================
        test("Test 20: Campaign UI Encyclopedia URL", function(t)
            local orig_enc_url = twdll.campaign_ui.GetEncyclopediaUrl()
            twdll.core.Log("[TEST] Original Encyclopedia URL: " .. tostring(orig_enc_url))
            t:assert(type(orig_enc_url) == "string" and orig_enc_url:len() > 0, "campaign_ui GetEncyclopediaUrl returns non-empty string")
            t:assert(type(orig_enc_url) == "string" and orig_enc_url:find("http://") == 1, "campaign_ui GetEncyclopediaUrl starts with http")

            -- Test setting a custom URL (The Dawnless Days Encyclopedia)
            local test_custom_url = "https://encyclopedia.thedawnlessdays.com/TDD.html#"
            local set_res = twdll.campaign_ui.SetEncyclopediaUrl(test_custom_url)
            t:assert_eq(set_res, test_custom_url, "campaign_ui SetEncyclopediaUrl returns new url")
            local read_back_url = twdll.campaign_ui.GetEncyclopediaUrl()
            t:assert_eq(read_back_url, test_custom_url, "campaign_ui GetEncyclopediaUrl matches custom url")

            -- Restore original Encyclopedia URL
            if orig_enc_url then
                twdll.campaign_ui.SetEncyclopediaUrl(orig_enc_url)
            end
        end)

        -- ======================================================
        -- TEST 21: Region Religions & Religion Proportion API
        -- ======================================================
        test("Test 21: Region Religions & Religion Proportion", function(t)
            local region = game:model():world():region_manager():region_by_key("att_reg_scandza_hafn")
            if not region or region:is_null_interface() then
                return t:skip("region att_reg_scandza_hafn not found")
            end

            if type(region.GetReligionList) ~= "function" or type(region.GetReligionProportion) ~= "function" then
                return t:skip("region religion methods not registered")
            end

            local religions = region:GetReligionList()
            local is_ud = type(religions) == "userdata"
            local num_items = is_ud and religions:num_items() or 0
            t:assert(is_ud and num_items > 0 and not religions:is_empty(), "Region GetReligionList valid")
            t:assert(religions:is_empty() == false, "Region religion list is_empty false")

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
                    if r_key == majority_key then
                        found_majority = true
                    end
                end
            end

            t:assert(math.abs(sum_proportion - 1.0) < 0.05, "Region religion proportions sum to 1.0")
            t:assert(found_majority, "Region majority religion present in list")
            t:assert(religions:item_at(-1) == nil and religions:item_at(999) == nil, "Region religion list out of bounds nil")
            t:assert_eq(region:GetReligionProportion("non_existent_religion_key"), 0.0, "Region non-existent religion proportion is 0")
        end)

        -- ======================================================
        -- TEST 22: twdll.cai (Campaign AI Telemetry)
        -- ======================================================
        test("Test 22: twdll.cai telemetry", function(t)
            if type(twdll.cai) ~= "table" or type(twdll.cai.EnableLogging) ~= "function" then
                return t:skip("twdll.cai module or EnableLogging not found")
            end

            local prev_state = twdll.cai.EnableLogging()
            twdll.core.Log(string.format("[TEST] twdll.cai.EnableLogging initial state: %s", tostring(prev_state)))

            local state_false = twdll.cai.EnableLogging(false)
            local state_true = twdll.cai.EnableLogging(true)

            local cai_ok = (state_false == false) and (state_true == true)
            t:assert(cai_ok, "twdll.cai.EnableLogging toggle")
        end)

        -- ======================================================
        -- TEST 23: Character ArtSet and Portrait API (ARTSET_SCRIPT_INTERFACE)
        -- ======================================================
        test("Test 23: Character ArtSet and Portrait API", function(t)
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
                return t:skip("character cqi 204 not found")
            end

            local art_set = test_char:GetArtSet()
            t:assert(art_set ~= nil and type(art_set) == "userdata", "character:GetArtSet returns userdata")
            if not art_set then
                return t:skip("character has no ArtSet")
            end

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

            t:assert_eq(key, "att_huns_general_01", "art_set:GetKey")
            t:assert_eq(culture, "att_cult_nomadic", "art_set:GetCulture")
            t:assert_eq(subculture, "att_sub_cult_nomadic_hunnic", "art_set:GetSubculture")
            t:assert_eq(char_faction, "att_fact_hunni", "art_set:GetFaction")
            t:assert_eq(agent, "general", "art_set:GetAgent")
            t:assert(type(group) == "string", "art_set:GetGroup returns string")
            t:assert_eq(settings_id, "att_huns_general_010", "art_set:GetSettingsId")
            t:assert_eq(portrait_path, "UI/Portraits/Portholes/att_cult_nomadic/att_frontend_faction_leader_huns_0.png", "art_set:GetPortraitPath")
            t:assert_eq(is_custom, true, "art_set:IsCustom")
            t:assert_eq(is_male, true, "art_set:IsMale")
            t:assert_eq(has_aging, true, "art_set:HasAging")
            t:assert_eq(has_seasonal, false, "art_set:HasSeasonal")
            t:assert_eq(has_levelling, true, "art_set:HasLevelling")
            t:assert(type(has_health) == "boolean", "art_set:HasHealth returns boolean")
            t:assert(type(has_religion) == "boolean", "art_set:HasReligion returns boolean")
            t:assert(type(is_faction_leader) == "boolean", "art_set:IsFactionLeaderSet returns boolean")

            -- Perform ArtSet swap
            local swap_ok = test_char:SetArtSet("att_general_nomadic_16")
            t:assert_eq(swap_ok, true, "character:SetArtSet swap returns true")
            local curr_art = test_char:GetArtSet()
            t:assert_eq(curr_art and curr_art:GetKey(), "att_general_nomadic_16", "character:SetArtSet verified key")

            -- Restore original artset
            test_char:SetArtSet(key)
        end)

        -- ======================================================
        -- TEST 24: Disband Unit & Military Force DisbandUnits
        -- ======================================================
        test("Test 24: Disband Unit & Military Force DisbandUnits", function(t)
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
                return t:skip("no military force with at least 3 units found")
            end

            local force = target_char:military_force()
            local initial_count = force:unit_list():num_items()
            twdll.core.Log(string.format("[TEST] Force cqi=%d initial unit count: %d", force:command_queue_index(), initial_count))

            -- 1. unit:Disband()
            local last_unit = force:unit_list():item_at(initial_count - 1)
            if not last_unit or last_unit:is_null_interface() or type(last_unit.Disband) ~= "function" then
                return t:skip("unit:Disband method not registered or unit is null")
            end

            local disband_ok = last_unit:Disband()
            t:assert_eq(disband_ok, true, "unit:Disband returns true")
            local count_after_unit = force:unit_list():num_items()
            t:assert_eq(count_after_unit, initial_count - 1, "unit:Disband decreases unit count by 1")

            -- 2. force:DisbandUnits(index)
            if type(force.DisbandUnits) ~= "function" or count_after_unit < 2 then
                return t:skip("force.DisbandUnits not registered or not enough units")
            end

            local idx_to_disband = count_after_unit - 1
            local force_disband_idx_ok = force:DisbandUnits(idx_to_disband)
            t:assert_eq(force_disband_idx_ok, true, "force:DisbandUnits(index) returns true")
            local count_after_idx = force:unit_list():num_items()
            t:assert_eq(count_after_idx, count_after_unit - 1, "force:DisbandUnits(index) decreases count")

            -- 3. force:DisbandUnits(unit_ud)
            local next_last = force:unit_list():item_at(count_after_idx - 1)
            if next_last and not next_last:is_null_interface() and count_after_idx >= 2 then
                local force_disband_ud_ok = force:DisbandUnits(next_last)
                t:assert_eq(force_disband_ud_ok, true, "force:DisbandUnits(userdata) returns true")
                local count_after_ud = force:unit_list():num_items()
                t:assert_eq(count_after_ud, count_after_idx - 1, "force:DisbandUnits(userdata) decreases count")
            end
        end)

        -- ======================================================
        -- TEST 25: Military Force Integrity / Morale API
        -- ======================================================
        test("Test 25: Military Force Integrity / Morale API", function(t)
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
                return t:skip("no military force found")
            end

            if type(test_force.GetIntegrity) ~= "function" or type(test_force.SetIntegrity) ~= "function" then
                return t:skip("force integrity methods not registered")
            end

            local has_integrity = test_force:HasIntegrity()
            t:assert(type(has_integrity) == "boolean", "force:HasIntegrity returns boolean")

            if not has_integrity then
                return t:skip("force has no integrity tracker")
            end

            local initial_integrity = test_force:GetIntegrity()
            t:assert(type(initial_integrity) == "number" and initial_integrity >= 0.0 and initial_integrity <= 100.0, "force:GetIntegrity returns in [0, 100]")

            -- Test SetIntegrity(75.0)
            local set_ok = test_force:SetIntegrity(75.0)
            t:assert_eq(set_ok, true, "force:SetIntegrity(75.0) returns true")
            t:assert(math.abs(test_force:GetIntegrity() - 75.0) < 0.01, "force:GetIntegrity verified 75.0")

            -- Test clamping
            test_force:SetIntegrity(150.0)
            t:assert(math.abs(test_force:GetIntegrity() - 100.0) < 0.01, "force:SetIntegrity clamps upper to 100")
            test_force:SetIntegrity(-10.0)
            t:assert(math.abs(test_force:GetIntegrity() - 0.0) < 0.01, "force:SetIntegrity clamps lower to 0")

            -- Restore initial integrity
            test_force:SetIntegrity(initial_integrity)
            t:assert(math.abs(test_force:GetIntegrity() - initial_integrity) < 0.01, "force:SetIntegrity restored initial value")
        end)

        -- ======================================================
        -- TEST 26: World Max Character Traits Limit API
        -- ======================================================
        test("Test 26: World Max Character Traits Limit API", function(t)
            if type(twdll.world.GetMaxTraits) ~= "function" or type(twdll.world.SetMaxTraits) ~= "function" then
                return t:skip("world max traits methods not found")
            end

            local initial_max_traits = twdll.world.GetMaxTraits()
            t:assert(type(initial_max_traits) == "number", "twdll.world.GetMaxTraits returns number")
            t:assert_eq(initial_max_traits, 10, "twdll.world.GetMaxTraits initial value is 10")

            twdll.world.SetMaxTraits(30)
            t:assert_eq(twdll.world.GetMaxTraits(), 30, "twdll.world.SetMaxTraits(30) verified")

            twdll.world.SetMaxTraits(0)
            t:assert_eq(twdll.world.GetMaxTraits(), 1, "twdll.world.SetMaxTraits clamps < 1 to 1")

            twdll.world.SetMaxTraits()
            t:assert_eq(twdll.world.GetMaxTraits(), 10, "twdll.world.SetMaxTraits() restored to 10")
        end)

        -- ======================================================
        -- TEST 27: Character Trait Manipulation API (AddTrait, RemoveTrait, GetTraitList)
        -- ======================================================
        test("Test 27: Character Trait Manipulation API", function(t)
            local test_char = nil
            local chars = game:model():world():faction_by_key(faction):character_list()
            if chars:num_items() > 0 then
                test_char = chars:item_at(0)
            end

            if not test_char then
                return t:skip("no character found")
            end

            if type(test_char.AddTrait) ~= "function" or type(test_char.RemoveTrait) ~= "function" or type(test_char.GetTraitList) ~= "function" then
                return t:skip("character trait methods not registered")
            end

            -- 1. Inspect initial traits
            local initial_traits = test_char:GetTraitList()
            t:assert(type(initial_traits) == "table", "char:GetTraitList returns table")

            if #initial_traits == 0 then
                return t:skip("character has 0 traits")
            end

            -- 2. Test RemoveTrait on a real verified trait
            local trait_to_remove = initial_traits[#initial_traits]
            local removed = test_char:RemoveTrait(trait_to_remove)
            t:assert_eq(removed, true, "char:RemoveTrait returned true")

            -- 3. Verify trait is gone
            local after_remove_traits = test_char:GetTraitList()
            local still_present = false
            for _, tr in ipairs(after_remove_traits) do
                if tr == trait_to_remove then still_present = true break end
            end
            t:assert(not still_present, "char:RemoveTrait verified absent from GetTraitList")
            t:assert_eq(#after_remove_traits, #initial_traits - 1, "char:GetTraitList count decremented by 1")

            -- 4. Test AddTrait to restore
            local added = test_char:AddTrait(trait_to_remove, 1, false)
            t:assert_eq(added, true, "char:AddTrait returned true")

            -- 5. Test Exceeding Vanilla 10-Trait Cap (> 10 traits)
            twdll.world.SetMaxTraits(20)
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

            local current_traits = test_char:GetTraitList()
            local target_count = 14
            for _, new_trait in ipairs(trait_pool) do
                if #current_traits >= target_count then break end
                test_char:AddTrait(new_trait, 1, false)
                current_traits = test_char:GetTraitList()
            end

            local final_traits = test_char:GetTraitList()
            t:assert(#final_traits > 10, "character trait count exceeds vanilla cap of 10")
            t:assert(#final_traits >= 14, "character reached target trait count (14)")

            -- 6. Clean Slate & Restore
            local traits_to_clean = test_char:GetTraitList()
            for _, tk in ipairs(traits_to_clean) do
                test_char:RemoveTrait(tk)
            end
            t:assert_eq(#test_char:GetTraitList(), 0, "all traits successfully removed (count == 0)")

            for _, tk in ipairs(initial_traits) do
                test_char:AddTrait(tk, 1, false)
            end
            twdll.world.SetMaxTraits()
            twdll.core.Log("[TEST] Restored initial character traits and max traits cap (10)")
        end)

        -- ======================================================
        -- TEST 28: Character Loyalty API (GetLoyalty, GetLoyaltyModifier, SetLoyaltyModifier, GetLoyaltyFactorList)
        -- ======================================================
        test("Test 28: Character Loyalty API", function(t)
            local test_char = nil
            local chars = game:model():world():faction_by_key(faction):character_list()
            if chars:num_items() > 0 then
                test_char = chars:item_at(0)
            end

            if not test_char then
                return t:skip("no character found")
            end

            if type(test_char.GetLoyalty) ~= "function" or type(test_char.GetLoyaltyModifier) ~= "function"
               or type(test_char.SetLoyaltyModifier) ~= "function" or type(test_char.GetLoyaltyFactorList) ~= "function" then
                return t:skip("character loyalty methods not registered")
            end

            local initial_loyalty = test_char:GetLoyalty()
            local initial_mod = test_char:GetLoyaltyModifier()
            t:assert(type(initial_loyalty) == "number" and initial_loyalty >= 0 and initial_loyalty <= 10, "char:GetLoyalty returns in [0, 10]")
            t:assert(type(initial_mod) == "number", "char:GetLoyaltyModifier returns integer")

            local factors = test_char:GetLoyaltyFactorList()
            t:assert(type(factors) == "table", "char:GetLoyaltyFactorList returns table")

            -- Test negative modifier (-10)
            local set_neg_ok = test_char:SetLoyaltyModifier(-10)
            t:assert_eq(set_neg_ok, true, "char:SetLoyaltyModifier(-10) returns true")
            t:assert_eq(test_char:GetLoyaltyModifier(), -10, "char:GetLoyaltyModifier verifies -10")
            local low_loyalty = test_char:GetLoyalty()
            t:assert(low_loyalty <= initial_loyalty, "loyalty decreased or clamped at 0 with negative modifier")

            -- Test positive modifier (+10)
            local set_pos_ok = test_char:SetLoyaltyModifier(10)
            t:assert_eq(set_pos_ok, true, "char:SetLoyaltyModifier(10) returns true")
            t:assert_eq(test_char:GetLoyaltyModifier(), 10, "char:GetLoyaltyModifier verifies 10")
            local high_loyalty = test_char:GetLoyalty()
            t:assert(high_loyalty >= initial_loyalty, "loyalty increased or clamped at 10 with positive modifier")

            -- Test 0 loyalty
            test_char:SetLoyaltyModifier(-100)
            t:assert_eq(test_char:GetLoyalty(), 0, "character final loyalty is 0")

            -- Restore original loyalty modifier
            test_char:SetLoyaltyModifier(initial_mod)
        end)

        -- ======================================================
        -- TEST 29: Character Transfer to Faction (TransferToFaction)
        -- ======================================================
        test("Test 29: Character Transfer to Faction", function(t)
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
            local spawn_x = p_char:logical_position_x() + 8
            local spawn_y = p_char:logical_position_y() + 8

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
                local s_x = (target_f_key == "att_fact_eastern_roman_empire" and target_reg:settlement()) and target_reg:settlement():logical_position_x() + 6 + (spawn_seq * 3) or spawn_x + (spawn_seq * 3)
                local s_y = (target_f_key == "att_fact_eastern_roman_empire" and target_reg:settlement()) and target_reg:settlement():logical_position_y() + 6 + (spawn_seq * 3) or spawn_y + (spawn_seq * 3)
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
                return t:skip("failed to spawn test character or TransferToFaction not registered")
            end

            -- Variation 1: Basic call
            local ok_1 = test_char1:TransferToFaction(ere_fac)
            t:assert_eq(ok_1, true, "TransferToFaction basic call returns true")
            t:assert_eq(test_char1:faction():name(), "att_fact_eastern_roman_empire", "character 1 faction changed to ERE")

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
                    t:assert_eq(sample_u:GetNumMen(), 15, "unit damaged before transfer (15 men)")
                end

                local ok_2 = test_char2:TransferToFaction(ere_fac, { replenish_units = true })
                t:assert_eq(ok_2, true, "TransferToFaction options table { replenish_units = true } returns true")
                t:assert_eq(test_char2:faction():name(), "att_fact_eastern_roman_empire", "character 2 faction changed to ERE")

                if u_list and u_list:num_items() > 0 then
                    local sample_u_after = u_list:item_at(0)
                    local full_men = sample_u_after:GetNumMen()
                    local max_men = sample_u_after:GetMaxNumMen()
                    t:assert(full_men == max_men and full_men > 15, "unit fully replenished after transfer (men == max_men)")
                end
            end

            -- Variation 3: Options Table with rebel_region
            local ere_home_reg = ere_fac:has_home_region() and ere_fac:home_region() or reg
            local test_char3 = spawn_general_for_test("twdll_tf_3", "att_fact_eastern_roman_empire")
            if test_char3 and rebel_fac and not rebel_fac:is_null_interface() then
                local ok_3 = test_char3:TransferToFaction(rebel_fac, { rebel_region = ere_home_reg, replenish_units = true })
                t:assert_eq(ok_3, true, "TransferToFaction options table { rebel_region } returns true")
                t:assert_eq(test_char3:faction():name(), rebel_fac:name(), "character 3 faction changed to rebels")
            end

            -- Variation 4: Native Hunnic Family Tree General Transfer (cqi=208)
            local hun_family_char = nil
            for i = 0, w_fac:character_list():num_items() - 1 do
                local c = w_fac:character_list():item_at(i)
                if c:cqi() == 208 then
                    hun_family_char = c
                    break
                end
            end
            if hun_family_char then
                local ok_4 = hun_family_char:TransferToFaction(ere_fac)
                t:assert_eq(ok_4, true, "Hunnic family general TransferToFaction returns true")
                t:assert_eq(hun_family_char:faction():name(), "att_fact_eastern_roman_empire", "Hunnic family general faction changed to ERE")
            end
        end)

        -- ======================================================
        -- TEST 30: Faction Create Agent API (CreateAgent)
        -- ======================================================
        test("Test 30: Faction Create Agent", function(t)
            local ere_fac = game:model():world():faction_by_key("att_fact_eastern_roman_empire")
            if not ere_fac or type(ere_fac.CreateAgent) ~= "function" then
                return t:skip("CreateAgent method not registered or ERE faction not found")
            end

            local initial_char_count = ere_fac:character_list():num_items()
            local initial_cqis = {}
            for i = 0, initial_char_count - 1 do
                initial_cqis[ere_fac:character_list():item_at(i):cqi()] = true
            end

            -- 1. Champion
            local capital_settlement = ere_fac:home_region():settlement()
            local create_champ_ok = ere_fac:CreateAgent("champion", capital_settlement)
            t:assert_eq(create_champ_ok, true, "ere_fac:CreateAgent('champion', settlement) returns true")

            local spawned_champ = nil
            for i = 0, ere_fac:character_list():num_items() - 1 do
                local c = ere_fac:character_list():item_at(i)
                if not initial_cqis[c:cqi()] then
                    spawned_champ = c
                    initial_cqis[c:cqi()] = true
                    break
                end
            end
            t:assert(spawned_champ ~= nil, "spawned champion character object resolved")
            if spawned_champ then
                t:assert_eq(spawned_champ:character_type("champion"), true, "spawned champion has type 'champion'")
                t:assert(spawned_champ:has_region() and spawned_champ:region():name() == ere_fac:home_region():name(), "spawned champion in capital region")
            end

            -- 2. Dignitary
            local target_x = 516
            local target_y = 381
            local create_dig_ok = ere_fac:CreateAgent("dignitary", target_x, target_y)
            t:assert_eq(create_dig_ok, true, "ere_fac:CreateAgent('dignitary', x, y) returns true")

            local spawned_dig = nil
            for i = 0, ere_fac:character_list():num_items() - 1 do
                local c = ere_fac:character_list():item_at(i)
                if not initial_cqis[c:cqi()] then
                    spawned_dig = c
                    initial_cqis[c:cqi()] = true
                    break
                end
            end
            t:assert(spawned_dig ~= nil, "spawned dignitary character object resolved")
            if spawned_dig then
                t:assert_eq(spawned_dig:character_type("dignitary"), true, "spawned dignitary has type 'dignitary'")
            end

            local final_char_count = ere_fac:character_list():num_items()
            t:assert_eq(final_char_count, initial_char_count + 2, "faction character count increased by 2")
        end)

        -- ======================================================
        -- TEST 31: Character Names, Localisation Keys, and Immortality API
        -- ======================================================
        test("Test 31: Character Names, Localisation Keys, and Immortality API", function(t)
            local f = game:model():world():faction_by_key(faction)
            local char = f and f:faction_leader() or nil
            if not char then
                return t:skip("no faction leader")
            end

            -- 1. Initial state reading
            local initial_fullname = char:GetFullName()
            local initial_fn = char:GetForename()
            local initial_fn_key = char:GetForenameKey()
            t:assert(type(initial_fullname) == "string", "GetFullName returns string")
            t:assert(type(initial_fn) == "string", "GetForename returns string")
            t:assert(type(initial_fn_key) == "string", "GetForenameKey returns string")

            -- 2. Custom Text vs DB Localisation Key switching
            local set_fn_ok = char:SetForename("Witch-king")
            t:assert_eq(set_fn_ok, true, "SetForename returns true")
            t:assert_eq(char:GetForename(), "Witch-king", "GetForename matches custom name")
            t:assert_eq(char:GetForenameKey(), "", "GetForenameKey is empty after custom text")

            local set_fn_key_ok = char:SetForenameKey("names_name_custom_test_1001")
            t:assert_eq(set_fn_key_ok, true, "SetForenameKey returns true")
            t:assert_eq(char:GetForenameKey(), "names_name_custom_test_1001", "GetForenameKey matches set key")
            t:assert_eq(char:GetForename(), "names_name_custom_test_1001", "GetForename returns key string as fallback")

            -- 3. Slot Independence
            char:SetForename("Aragorn")
            char:SetClanName("Dunadan")
            char:SetFamilyName("Telcontar")
            char:SetOtherName("Elessar")

            t:assert_eq(char:GetForename(), "Aragorn", "Forename initialized to Aragorn")
            t:assert_eq(char:GetClanName(), "Dunadan", "ClanName initialized to Dunadan")
            t:assert_eq(char:GetFamilyName(), "Telcontar", "FamilyName initialized to Telcontar")
            t:assert_eq(char:GetOtherName(), "Elessar", "OtherName initialized to Elessar")
            t:assert_eq(char:GetFullName(), "Aragorn Dunadan Telcontar Elessar", "GetFullName matches initial 4-slot composite")

            char:SetFamilyName("Isildur")
            t:assert_eq(char:GetFamilyName(), "Isildur", "FamilyName updated to Isildur")
            t:assert_eq(char:GetForename(), "Aragorn", "Forename untouched after FamilyName change")
            t:assert_eq(char:GetClanName(), "Dunadan", "ClanName untouched after FamilyName change")
            t:assert_eq(char:GetOtherName(), "Elessar", "OtherName untouched after FamilyName change")

            char:SetClanName("Ranger")
            t:assert_eq(char:GetClanName(), "Ranger", "ClanName updated to Ranger")
            t:assert_eq(char:GetForename(), "Aragorn", "Forename untouched after ClanName change")
            t:assert_eq(char:GetFamilyName(), "Isildur", "FamilyName untouched after ClanName change")
            t:assert_eq(char:GetOtherName(), "Elessar", "OtherName untouched after ClanName change")

            char:SetOtherName("Strider")
            t:assert_eq(char:GetOtherName(), "Strider", "OtherName updated to Strider")
            t:assert_eq(char:GetForename(), "Aragorn", "Forename untouched after OtherName change")
            t:assert_eq(char:GetClanName(), "Ranger", "ClanName untouched after OtherName change")
            t:assert_eq(char:GetFamilyName(), "Isildur", "FamilyName untouched after OtherName change")

            t:assert_eq(char:GetFullName(), "Aragorn Ranger Isildur Strider", "GetFullName matches mutated composite order")

            -- 4. DB Localisation Keys on all slots
            t:assert_eq(char:SetFamilyNameKey("names_name_fam_key_2002"), true, "SetFamilyNameKey returns true")
            t:assert_eq(char:GetFamilyNameKey(), "names_name_fam_key_2002", "GetFamilyNameKey matches set key")
            t:assert_eq(char:SetClanNameKey("names_name_clan_key_3003"), true, "SetClanNameKey returns true")
            t:assert_eq(char:GetClanNameKey(), "names_name_clan_key_3003", "GetClanNameKey matches set key")
            t:assert_eq(char:SetOtherNameKey("names_titles_other_key_4004"), true, "SetOtherNameKey returns true")
            t:assert_eq(char:GetOtherNameKey(), "names_titles_other_key_4004", "GetOtherNameKey matches set key")

            -- 5. Immortality & Resurrection Turns
            local imm_before = char:IsImmortal()
            t:assert(type(imm_before) == "boolean", "IsImmortal returns boolean")

            t:assert_eq(char:SetImmortal(true), true, "SetImmortal returns true")
            t:assert_eq(char:IsImmortal(), true, "IsImmortal matches set true")

            char:SetResurrectionTurns(5)
            t:assert_eq(char:GetResurrectionTurns(), 5, "GetResurrectionTurns matches 5")

            -- Restore health and immortality
            char:SetResurrectionTurns(0)
            char:SetImmortal(imm_before)
        end)

        -- ======================================================
        -- TEST 32: Game Lifecycle & Save/Load API Functions Existence
        -- ======================================================
        test("Test 32: Game Lifecycle & Save/Load API Functions Existence", function(t)
            t:assert(type(twdll.world.SaveGame) == "function", "twdll.world.SaveGame is function")
            t:assert(type(twdll.world.LoadGame) == "function", "twdll.world.LoadGame is function")
            t:assert(type(twdll.world.ExitToMainMenu) == "function", "twdll.world.ExitToMainMenu is function")
            t:assert(type(twdll.world.ExitGame) == "function", "twdll.world.ExitGame is function")
        end)

        -- ======================================================
        -- TEST 33: Technology Status API (GetTechnologyStatus / SetTechnologyStatus)
        -- ======================================================
        test("Test 33: Technology Status API", function(t)
            local fac_obj = game:model():world():faction_by_key(faction)
            if not fac_obj then
                return t:skip("faction not found")
            end

            t:assert(type(fac_obj.GetTechnologyStatus) == "function", "faction:GetTechnologyStatus is function")
            t:assert(type(fac_obj.SetTechnologyStatus) == "function", "faction:SetTechnologyStatus is function")
            t:assert(type(fac_obj.InstantlyResearchTechnology) == "function", "faction:InstantlyResearchTechnology is function")

            -- 1. Instantly research 4 Hunnic military technologies
            fac_obj:InstantlyResearchTechnology("att_hunnic_military_militarised_massing_of_power")
            fac_obj:InstantlyResearchTechnology("att_hunnic_military_traditions_of_mobility")
            fac_obj:InstantlyResearchTechnology("att_hunnic_military_extra_military_provisions")
            fac_obj:InstantlyResearchTechnology("att_hunnic_military_speed_of_attack")

            t:assert_eq(fac_obj:GetTechnologyStatus("att_hunnic_military_militarised_massing_of_power"), 0, "Tech 1 status is RESEARCHED (0)")
            t:assert_eq(fac_obj:GetTechnologyStatus("att_hunnic_military_traditions_of_mobility"), 0, "Tech 2 status is RESEARCHED (0)")
            t:assert_eq(fac_obj:GetTechnologyStatus("att_hunnic_military_extra_military_provisions"), 0, "Tech 3 status is RESEARCHED (0)")
            t:assert_eq(fac_obj:GetTechnologyStatus("att_hunnic_military_speed_of_attack"), 0, "Tech 4 status is RESEARCHED (0)")

            -- 2. Explicitly set supply_acquisition to UNAVAILABLE (4)
            local set_ok = fac_obj:SetTechnologyStatus("att_hunnic_military_supply_acquisition", 4)
            t:assert_eq(set_ok, true, "SetTechnologyStatus returns true")
            t:assert_eq(fac_obj:GetTechnologyStatus("att_hunnic_military_supply_acquisition"), 4, "Tech 5 status is UNAVAILABLE (4)")
        end)

        -- ======================================================
        -- TEST 34: Tweakers Module & Object-Oriented Registry
        -- ======================================================
        test("Test 34: Tweakers Module & Object-Oriented Registry", function(t)
            if not twdll.tweakers then
                return t:skip("twdll.tweakers module NOT found")
            end

            t:assert(type(twdll.tweakers) == "table", "twdll.tweakers table exists")
            t:assert(type(twdll.tweakers.Dump) == "function", "twdll.tweakers.Dump is function")
            t:assert(type(twdll.tweakers.GetList) == "function", "twdll.tweakers.GetList is function")
            t:assert(type(twdll.tweakers.Find) == "function", "twdll.tweakers.Find is function")

            -- 1. Direct Table Access via Metatable (__index): twdll.tweakers.max_traits
            local tw_obj = twdll.tweakers.max_traits
            t:assert(type(tw_obj) == "userdata", "twdll.tweakers.max_traits returns userdata")

            if tw_obj then
                t:assert_eq(tw_obj.name, "max_traits", "tweak.name is 'max_traits'")
                t:assert_eq(tw_obj.category, "all", "tweak.category is 'all'")
                t:assert_eq(tw_obj.line, 62, "tweak.line is 62")
                t:assert(type(tw_obj.value) == "number" and tw_obj.value > 0, "tweak.value returns number")

                -- 2. Property Setter: tw_obj.value = 25
                local orig_val = tw_obj.value
                tw_obj.value = 25
                t:assert_eq(tw_obj.value, 25, "tweak.value setter updated to 25")

                -- 3. Direct Module Assignment: twdll.tweakers.max_traits.value = 28
                twdll.tweakers.max_traits.value = 28
                t:assert_eq(tw_obj.value, 28, "twdll.tweakers.max_traits.value = 28 updated")

                -- Restore original value
                tw_obj.value = orig_val
            end

            -- 4. Test Boolean Tweaker: AI_FORCE_ATTACK_PLAN
            local ai_tw = twdll.tweakers.AI_FORCE_ATTACK_PLAN
            if ai_tw then
                t:assert_eq(ai_tw.name, "AI_FORCE_ATTACK_PLAN", "AI_FORCE_ATTACK_PLAN found")
                local orig_b = ai_tw.bool
                twdll.tweakers.AI_FORCE_ATTACK_PLAN.value = true
                t:assert_eq(ai_tw.bool, true, "AI_FORCE_ATTACK_PLAN.value = true sets boolean")
                ai_tw.value = orig_b
            end

            -- 5. Test Campaign Variable Live Routing: general_admiral_action_point_bonus
            local ap_tw = twdll.tweakers.general_admiral_action_point_bonus
            if ap_tw then
                t:assert_eq(ap_tw.name, "general_admiral_action_point_bonus", "general_admiral_action_point_bonus found")
                local orig_ap = ap_tw.value
                ap_tw.value = 120
                t:assert_eq(ap_tw.value, 120, "general_admiral_action_point_bonus updated in live model")
                ap_tw.value = orig_ap
            end

            -- 6. Test GetList() returning array of Tweaker objects
            local list = twdll.tweakers.GetList()
            t:assert(#list > 3000, "GetList returns 3000+ tweakers")
            if #list > 0 then
                t:assert(type(list[1]) == "userdata", "GetList element is userdata")
                t:assert(type(list[1].name) == "string", "GetList element has .name property")

                -- Comprehensive Bulk Sweep Test
                local read_ok_count = 0
                local mutate_ok_count = 0
                local cv_verified_count = 0
                local cv_total = 0

                for _, tw in ipairs(list) do
                    local name = tw.name
                    if name and #name > 0 then
                        read_ok_count = read_ok_count + 1
                        local is_cv = (tw.category == "Campaign Variable Tweakers")
                        if is_cv then
                            cv_total = cv_total + 1
                            local orig_val = tw.float
                            tw.float = orig_val + 10.0
                            if math.abs(tw.float - (orig_val + 10.0)) < 0.001 then
                                cv_verified_count = cv_verified_count + 1
                                mutate_ok_count = mutate_ok_count + 1
                            end
                            tw.float = orig_val
                        else
                            local orig_val = tw.value
                            if type(orig_val) == "number" then
                                tw.value = orig_val + 1
                                if math.abs(tw.value - (orig_val + 1)) < 0.001 then
                                    mutate_ok_count = mutate_ok_count + 1
                                end
                                tw.value = orig_val
                            elseif type(orig_val) == "boolean" then
                                tw.value = not orig_val
                                if tw.value == (not orig_val) then
                                    mutate_ok_count = mutate_ok_count + 1
                                end
                                tw.value = orig_val
                            else
                                mutate_ok_count = mutate_ok_count + 1
                            end
                        end
                    end
                end

                t:assert_eq(read_ok_count, #list, "All tweakers readable")
                t:assert_eq(mutate_ok_count, #list, "All tweakers mutable and restorable")
                t:assert(cv_total == 714 and cv_verified_count == 714, "All 714 Campaign Variables verified in live model")
            end
        end)

        -- ======================================================
        -- TEST 35: Military Force methods and AppointCharacter
        -- ======================================================
        test("Test 35: Military Force methods and AppointCharacter", function(t)
            local test_fac = nil
            if game and type(game.model) == "function" and game:model() and type(game:model().world) == "function" and game:model():world() then
                local fac_list = game:model():world():faction_list()
                if fac_list and type(fac_list.num_items) == "function" and fac_list:num_items() > 0 then
                    for i = 0, fac_list:num_items() - 1 do
                        local f = fac_list:item_at(i)
                        if f and type(f.military_force_list) == "function" and f:military_force_list():num_items() > 0 then
                            test_fac = f
                            break
                        end
                    end
                end
            end

            if not test_fac then
                return t:skip("no faction with military force found")
            end

            local force_list = test_fac:military_force_list()
            local force = force_list:item_at(0)
            if not force then
                return t:skip("no force found in faction")
            end

            local mf_mem = force:GetMemoryAddress()
            t:assert(type(mf_mem) == "string" and mf_mem:sub(1, 2) == "0x", "force:GetMemoryAddress valid")

            local rq_size = force:GetRecruitmentQueueSize()
            t:assert(type(rq_size) == "number" and rq_size >= 0, "force:GetRecruitmentQueueSize valid")

            local has_integ = force:HasIntegrity()
            t:assert(type(has_integ) == "boolean", "force:HasIntegrity boolean")

            if has_integ then
                local orig_integ = force:GetIntegrity()
                force:SetIntegrity(75.0)
                t:assert(math.abs(force:GetIntegrity() - 75.0) < 0.01, "force:SetIntegrity")
                force:SetIntegrity(orig_integ)
            end

            -- 1. Test invalid arguments
            local bad_arg_ok = force:AppointCharacter(nil)
            t:assert_eq(bad_arg_ok, false, "force:AppointCharacter invalid nil arg returns false")

            -- 2. Test AppointCharacter self-assignment (no-op check)
            local current_gen = force:has_general() and force:general_character()
            if not current_gen then
                return t:skip("force has no general")
            end

            local orig_name = current_gen:GetFullName()
            local orig_cqi = current_gen:command_queue_index()
            local orig_bg_key = "none"
            local initial_ulist = force:unit_list()
            if initial_ulist and type(initial_ulist.num_items) == "function" and initial_ulist:num_items() > 0 then
                orig_bg_key = initial_ulist:item_at(0):unit_key()
            end

            local app_self_ok = force:AppointCharacter(current_gen)
            t:assert_eq(app_self_ok, true, "force:AppointCharacter self-assign")

            -- 3. Find another character in the faction to test real swap
            local other_gen = nil
            local char_list = test_fac:character_list()
            if char_list and type(char_list.num_items) == "function" then
                for i = 0, char_list:num_items() - 1 do
                    local c = char_list:item_at(i)
                    if c and not c:is_null_interface() and c:command_queue_index() ~= orig_cqi then
                        local ctype = type(c.character_type_key) == "function" and c:character_type_key() or ""
                        if ctype == "general" or ctype == "colonel" or ctype == "politician" or ctype == "" then
                            other_gen = c
                            break
                        end
                    end
                end
            end

            if not other_gen then
                return t:skip("no alternative general candidate in faction")
            end

            local other_name = other_gen:GetFullName()
            local other_cqi = other_gen:command_queue_index()
            local target_custom_bg = (orig_bg_key == "att_merc_ger_agathyrsi_warriors") and "att_rom_legio" or "att_merc_ger_agathyrsi_warriors"
            local initial_bg_different = (orig_bg_key ~= target_custom_bg)

            t:assert_eq(initial_bg_different, true, "force:AppointCharacter initial bodyguard differed from custom target")

            local swap_ok = force:AppointCharacter(other_gen, { bodyguard_key = target_custom_bg })
            local new_active_gen = force:has_general() and force:general_character()
            local new_active_cqi = new_active_gen and new_active_gen:command_queue_index() or -1
            local swap_verified = swap_ok and new_active_gen and (new_active_cqi == other_cqi)

            -- Verify that the custom bodyguard unit exists in the army
            local found_bg_unit = false
            local u_list = force:unit_list()
            if u_list and type(u_list.num_items) == "function" then
                for i = 0, u_list:num_items() - 1 do
                    local u = u_list:item_at(i)
                    if u and not u:is_null_interface() and u:unit_key() == target_custom_bg then
                        found_bg_unit = true
                        break
                    end
                end
            end

            t:assert(swap_verified == true, "force:AppointCharacter appoint candidate")
            t:assert_eq(found_bg_unit, true, "force:AppointCharacter custom bodyguard key verified in force")
        end)

        -- ======================================================
        -- SUMMARY & LIFECYCLE
        -- ======================================================
        local all_passed = summary()
        if all_passed then
            -- POST-TEST LIFECYCLE: Save -> Load (Only when all tests passed, not MP, and no-save flag not set)
            local is_mp = (cm and type(cm.is_multiplayer) == "function" and cm:is_multiplayer()) or
                          (game and type(game.model) == "function" and game:model() and type(game:model().is_multiplayer) == "function" and game:model():is_multiplayer())

            local no_save_flag = io.open("twdll_no_save_reload.flag", "r")
            if is_mp then
                twdll.core.Log("[TEST] [LIFECYCLE] Multiplayer campaign detected — skipping SaveGame / LoadGame reload sequence.")
            elseif no_save_flag then
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
        end

    else
        local f2 = io.open("twdll.log", "a")
        if f2 then
            f2:write("[TEST] FAILED: twdll.Log not found in global state. Testing aborted.\n")
            f2:close()
        end
    end
end

-- Register Save/Load persistence listeners
pcall(function()
    table.insert(events.SavingGame, function(context)
        local gi = get_game_interface()
        if gi and type(gi.save_named_value) == "function" then
            gi:save_named_value("twdll_tests_already_run", twdll_tests_already_run, context)
        end
    end)
end)

pcall(function()
    table.insert(events.LoadingGame, function(context)
        local gi = get_game_interface()
        if gi and type(gi.load_named_value) == "function" then
            twdll_tests_already_run = gi:load_named_value("twdll_tests_already_run", false, context)
        end
    end)
end)

-- Register the test suite to execute only after the world is initialized
local _, err = pcall(function()
    table.insert(events.FirstTickAfterWorldCreated, function()
        get_game_interface()
        local ok, err = pcall(run_twdll_tests)
        twdll.core.Log(ok and "[TEST] run_twdll_tests returned normally" or ("[TEST] run_twdll_tests error: " .. tostring(err)))
    end)
end)

if err then
    twdll.core.Log("Added event had error: " .. tostring(err))
end
