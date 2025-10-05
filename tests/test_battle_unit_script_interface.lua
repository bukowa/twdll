-- tests/battle_unit_script_interface.test.lua

local TestUtils = require("tests.test_utils")

-- Define the test suite for the Battle Unit interface
local BattleUnitSuite = { name = "twdll_battle_unit" }

--- This is the placeholder function for this test suite.
-- It will be called before each test to get a fresh test subject.
-- @return userdata A valid battle unit object for testing.
local function get_test_subject()
    -- TODO: Fill this in to return a real battle unit object from the game.
    error("get_test_subject() is not implemented for the Battle Unit suite.")
    return nil
end

--- Tests reading the fatigue property.
function BattleUnitSuite:test_get_fatigue(Assert, unit)
    local value = twdll_battle_unit.GetFatigue(unit)
    Assert(type(value) == "number", "GetFatigue should return a number.")
end

--- Tests reading the charge bonus property.
function BattleUnitSuite:test_get_charge_bonus(Assert, unit)
    local value = twdll_battle_unit.GetChargeBonus(unit)
    Assert(type(value) == "number", "GetChargeBonus should return a number.")
end

--- Tests reading the melee attack property.
function BattleUnitSuite:test_get_melee_attack(Assert, unit)
    local value = twdll_battle_unit.GetMeleeAttack(unit)
    Assert(type(value) == "number", "GetMeleeAttack should return a number.")
end

--- Tests reading the base morale property.
function BattleUnitSuite:test_get_base_morale(Assert, unit)
    local value = twdll_battle_unit.GetBaseMorale(unit)
    Assert(type(value) == "number", "GetBaseMorale should return a number.")
end

--- Tests reading the example float property.
function BattleUnitSuite:test_get_some_float_value(Assert, unit)
    local value = twdll_battle_unit.GetSomeFloatValue(unit)
    Assert(type(value) == "number", "GetSomeFloatValue should return a number.")
end

-- Run all tests in this suite
TestUtils.run_tests(BattleUnitSuite, get_test_subject)
