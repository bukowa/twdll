-- mp_testing.lua — Strict deterministic multiplayer test suite

local function log(...)
    local msg = table.concat({...}, " ")
    if twdll and twdll.core and twdll.core.Log then
        twdll.core.Log("[MP_TEST] " .. msg)
    else
        local f = io.open("twdll.log", "a")
        if f then f:write("[MP_TEST] " .. msg .. "\n"); f:close() end
    end
end

log("Loaded mp_testing.lua")

-- Bezpieczny wrapper pcall dla wszystkich listenerów
local function on_event(event_table, name, callback)
    if not event_table then return end
    table.insert(event_table, function(context)
        local ok, err = pcall(callback, context)
        if not ok then
            log("[ERROR] [" .. name .. "] " .. tostring(err))
        end
    end)
end

-- 1. Start kampanii / Pierwszy tick (Globalne limity silnika)
on_event(events.FirstTickAfterWorldCreated, "FirstTickAfterWorldCreated", function(context)
    log("=== FirstTickAfterWorldCreated (MP) ===")
    if twdll and twdll.world then
        twdll.world.SetMaxUnitsInArmy(20)
        twdll.world.SetMaxTraits(15)
        log("Global limits set: MaxUnits=20, MaxTraits=15")
    end
end)

-- 2. Początek tury frakcji (Deterministyczny integer bonus)
on_event(events.FactionTurnStart, "FactionTurnStart", function(context)
    local f = context:faction()
    if f:is_null_interface() then return end

    local name = f:name()
    log("--- [EVENT] FactionTurnStart: " .. tostring(name) .. " ---")

    local cur = f:GetTreasury()
    f:SetTreasury(cur + 250)
    log("  Treasury: " .. tostring(cur) .. " -> " .. tostring(f:GetTreasury()))
end)

-- 3. Początek tury postaci (Deterministyczny stan AP, wpływów, lojalności i oddziału)
on_event(events.CharacterTurnStart, "CharacterTurnStart", function(context)
    local c = context:character()
    if c:is_null_interface() then return end

    local name = c:GetFullName()
    log("[EVENT] CharacterTurnStart: " .. tostring(name))

    -- Punkty ruchu w Attili (skala ~3960 = 100%)
    local cur_ap = c:GetActionPoints()
    c:SetActionPoints(cur_ap + 500)
    c:SetInfluence(c:GetInfluence() + 1)
    c:SetLoyaltyModifier(c:GetLoyaltyModifier() + 1)

    -- Modyfikacja pierwszego oddziału w armii generała
    if c:has_military_force() then
        local mf = c:military_force()
        if not mf:is_null_interface() then
            local ul = mf:unit_list()
            if ul and ul:num_items() > 0 then
                local u0 = ul:item_at(0)
                if u0 and not u0:is_null_interface() then
                    u0:SetNumMen(u0:GetMaxNumMen())
                end
            end
        end
    end
end)

-- 4. Początek tury regionu (Deterministyczny wzrost prowincji)
on_event(events.RegionTurnStart, "RegionTurnStart", function(context)
    local r = context:region()
    if r:is_null_interface() then return end

    log("[EVENT] RegionTurnStart: " .. tostring(r:name()))

    local gr = r:GetGrowthPoints()
    r:SetGrowthPoints(gr + 5)
    log("  Region growth: " .. tostring(gr) .. " -> " .. tostring(r:GetGrowthPoints()))
end)

-- 5. Zakończenie bitwy
on_event(events.BattleCompleted, "BattleCompleted", function(context)
    log("=== [EVENT] BattleCompleted (MP) ===")
    if twdll and twdll.world and twdll.world.SetReinforcementCap then
        twdll.world.SetReinforcementCap(40)
        log("SetReinforcementCap = 40")
    end
end)
