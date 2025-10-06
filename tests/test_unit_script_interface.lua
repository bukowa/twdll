-- tests/unit_script_interface.test.lua

local TestUtils = require("tests.test_utils")

-- Define the test suite for the Unit interface
local UnitSuite = { name = "twdll_unit" }

-- It will be called before each test to get a fresh test subject.
-- @return userdata A valid unit object for testing.
local function get_test_subject()
    local chars = consul._game():model():world():faction_by_key('inv_rome'):character_list()
    for i = 0, chars:num_items() - 1 do
        if chars:item_at(i):cqi() == 6 then
            return chars:item_at(i):military_force():unit_list():item_at(0)
        end
    end
    error("get_test_subject() is not implemented for the Unit suite.")
    return nil
end

--- Tests the Get/Set NumberOfMan functions.
function UnitSuite:test_number_of_man_property(Assert, unit)
    local initial_value = twdll_unit.GetNumberOfMan(unit)
    local new_value = initial_value + 10

    twdll_unit.SetNumberOfMan(unit, new_value)
    local updated_value = twdll_unit.GetNumberOfMan(unit)

    Assert(updated_value == new_value, "SetNumberOfMan should update the unit's number of man.")
end

--- Tests the Get/Set Max NumberOfMan functions.
function UnitSuite:test_max_number_of_man_property(Assert, unit)
    local initial_value = twdll_unit.GetMaxNumberOfMan(unit)
    local new_value = initial_value + 10

    twdll_unit.SetMaxNumberOfMan(unit, new_value)
    local updated_value = twdll_unit.GetMaxNumberOfMan(unit)

    Assert(updated_value == new_value, "SetMaxNumberOfMan should update the unit's max number of man.")
end

--- Tests the Get/Set Movement Points functions.
function UnitSuite:test_movement_points_property(Assert, unit)
    local initial_value = twdll_unit.GetMovementPoints(unit)
    local new_value = initial_value - 100

    twdll_unit.SetMovementPoints(unit, new_value)
    local updated_value = twdll_unit.GetMovementPoints(unit)

    Assert(updated_value == new_value, "SetMovementPoints should update the unit's movement points.")
end

-- Run all tests in this suite
TestUtils.run_tests(UnitSuite, get_test_subject)
