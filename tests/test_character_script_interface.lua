-- tests/character_script_interface.test.lua

local TestUtils = require("tests.test_utils")

-- Define the test suite for the Character interface
local CharacterSuite = { name = "twdll_character" }

--- This is the placeholder function for this test suite.
-- It will be called before each test to get a fresh test subject.
-- @return userdata A valid character object for testing.
local function get_test_subject()
    local chars = consul._game():model():world():faction_by_key('inv_rome'):character_list()
    for i = 0, chars:num_items()-1 do
        if chars:item_at(i):cqi() == 6 then
            return chars:item_at(i)
        end
    end
    error("get_test_subject() is not implemented for the Character suite.")
end

--- Tests the Get/Set IntAtOffset functions.
-- WARNING: This test is powerful but dangerous. Ensure the offset is correct.
function CharacterSuite:test_offset_read_write(Assert, character)
    -- TODO: Replace with a known, safe 32-bit integer offset.
    local test_offset = 0xABC
    
    local initial_value = twdll_character.GetIntAtOffset(character, test_offset)
    local new_value = 99 -- An arbitrary integer

    twdll_character.SetIntAtOffset(character, test_offset, new_value)
    local updated_value = twdll_character.GetIntAtOffset(character, test_offset)

    Assert(updated_value == new_value, "SetIntAtOffset should correctly change the value at the specified offset.")

    -- Cleanup: Restore the original value
    twdll_character.SetIntAtOffset(character, test_offset, initial_value)
end

-- Run all tests in this suite
TestUtils.run_tests(CharacterSuite, get_test_subject)
