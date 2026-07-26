-- test_attila.lua — twdll tests for Total War: Attila.
-- Returns a run() function called by testing.lua inside FirstTickAfterWorldCreated.

local FACTION = "att_fact_hunni"

return function(game)
    twdll = _G.script_env.twdll
    local function get_faction() return game:model():world():faction_by_key(FACTION) end

    -- ======================================================
    -- TEST 1: Singleton hooks
    -- ======================================================
    twdll.core.Log("[TEST] --- Test 1: Singleton hooks ---")
    do
        local world_ptr     = twdll.world.GetMemoryAddress()
        local ui_ptr        = twdll.campaign_ui.GetMemoryAddress()
        local faction_count = twdll.world.GetFactionCount()
        local engine_count  = game:model():world():faction_list():num_items()

        twdll.core.Log("[TEST] g_world       = " .. tostring(world_ptr))
        twdll.core.Log("[TEST] g_campaign_ui = " .. tostring(ui_ptr))
        twdll.core.Log("[TEST] FactionCount  = " .. tostring(faction_count))

        if world_ptr ~= nil then
            twdll.core.Log("[TEST] g_world: OK")
        else
            twdll.core.Log("[TEST] g_world: FAILED")
        end

        if ui_ptr ~= nil then
            twdll.core.Log("[TEST] g_campaign_ui: OK")
        else
            twdll.core.Log("[TEST] g_campaign_ui: FAILED")
        end

        if faction_count == engine_count and faction_count ~= nil and faction_count > 0 then
            twdll.core.Log("[TEST] GetFactionCount: OK (" .. tostring(faction_count) .. ")")
        else
            twdll.core.Log("[TEST] GetFactionCount: FAILED (hook=" .. tostring(faction_count) .. " engine=" .. tostring(engine_count) .. ")")
        end
    end

    -- ======================================================
    -- TEST 2: unit GetNumberOfMan / SetNumberOfMan
    -- ======================================================
    twdll.core.Log("[TEST] --- Test 2: unit read/write ---")
    do
        local unit    = get_faction():faction_leader():military_force():unit_list():item_at(0)
        local max_men = twdll.unit.GetMaxNumberOfMan(unit)

        twdll.unit.SetNumberOfMan(unit, 20)
        local new_men = twdll.unit.GetNumberOfMan(unit)

        twdll.core.Log("[TEST] SetNumberOfMan(20): got " .. tostring(new_men) .. " / max=" .. tostring(max_men))
        if new_men == 20 then
            twdll.core.Log("[TEST] unit read/write: OK")
        else
            twdll.core.Log("[TEST] unit read/write: FAILED (expected 20, got " .. tostring(new_men) .. ")")
        end
    end

    -- ======================================================
    -- TEST 3: faction:SetFactionLeader
    -- ======================================================
    twdll.core.Log("[TEST] --- Test 3: SetFactionLeader ---")
    do
        local f       = get_faction()
        local chars   = f:character_list()
        local generals = {}
        for i = 0, chars:num_items() - 1 do
            local c = chars:item_at(i)
            if c:character_type("general") then generals[#generals + 1] = c end
        end

        if #generals < 2 then
            twdll.core.Log("[TEST] SetFactionLeader: SKIPPED (need at least 2 generals)")
        else
            local function pick_non_leader()
                local lid = f:faction_leader():cqi()
                for _, c in ipairs(generals) do if c:cqi() ~= lid then return c end end
            end

            local new1 = pick_non_leader()
            f:SetFactionLeader(new1)
            if f:faction_leader():cqi() == new1:cqi() then
                twdll.core.Log("[TEST] SetFactionLeader silent: OK")
            else
                twdll.core.Log("[TEST] SetFactionLeader silent: FAILED")
            end

            local new2, old2 = pick_non_leader(), f:faction_leader()
            f:SetFactionLeader(new2, old2)
            if f:faction_leader():cqi() == new2:cqi() then
                twdll.core.Log("[TEST] SetFactionLeader succession: OK")
            else
                twdll.core.Log("[TEST] SetFactionLeader succession: FAILED")
            end

            local new3, old3 = pick_non_leader(), f:faction_leader()
            f:SetFactionLeader(new3, old3, true)
            if f:faction_leader():cqi() == new3:cqi() then
                twdll.core.Log("[TEST] SetFactionLeader heir_coming_of_age: OK")
            else
                twdll.core.Log("[TEST] SetFactionLeader heir_coming_of_age: FAILED")
            end
        end
    end

    -- ======================================================
    -- TEST 4: faction GetGold / SetGold
    -- Hunni start with 15000 gold.
    -- ======================================================
    twdll.core.Log("[TEST] --- Test 4: faction GetGold/SetGold ---")
    do
        local f            = get_faction()
        local gold_initial = f:GetGold()
        twdll.core.Log("[TEST] GetGold initial = " .. tostring(gold_initial))

        if gold_initial == 15000 then
            twdll.core.Log("[TEST] GetGold initial: OK (15000)")
        else
            twdll.core.Log("[TEST] GetGold initial: UNEXPECTED (expected 15000, got " .. tostring(gold_initial) .. ")")
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
end
