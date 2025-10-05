-- tests/test_utils.lua
-- A simple, non-intrusive assertion and test running framework for in-game use.

-- Load the library to get access to the global logger
if not package.loaded["libtwdll"] then
    local _lib, err = package.loadlib("libtwdll.dll", "luaopen_libtwdll")
    if not _lib then
        error("FATAL: Failed to load libtwdll.dll: " .. tostring(err))
    end
    _lib()
end

local TestUtils = {}

--- Logs a message to the libtwdll.log file.
-- @tparam string message The message to log.
function TestUtils.Log(message)
    -- This relies on a Log function being exposed in your main libtwdll table.
    -- We need to add this to libtwdll.cpp
    if twdll and twdll.Log then
        twdll.Log(message)
    else
        -- Fallback for when running outside the game, prints to console.
        print(message)
    end
end

--- Asserts a condition is true, logging a failure if not.
-- Does not halt execution.
-- @tparam any condition The condition to check (if false or nil, it's a failure).
-- @tparam string message The failure message to log.
-- @treturn boolean True if the assertion passed, false otherwise.
function TestUtils.Assert(condition, message)
    if not condition then
        TestUtils.Log("  [FAIL] " .. message)
        return false
    end
    TestUtils.Log("  [PASS]")
    return true
end

--- Runs all functions in a given table that start with "test_".
-- @tparam table test_suite A table containing the test functions.
-- @tparam function get_subject_factory A function that returns a new test subject (e.g., a unit or character).
function TestUtils.run_tests(test_suite, get_subject_factory)
    local passed_count = 0
    local failed_count = 0
    local subject_name = test_suite.name or "Unknown Suite"

    TestUtils.Log("\n----- Running Test Suite: " .. subject_name .. " -----")

    for name, func in pairs(test_suite) do
        if name:match("^test_") and type(func) == "function" then
            TestUtils.Log("-> Executing: " .. name)
            local subject = get_subject_factory()
            if not subject then
                TestUtils.Log("  [FATAL] Could not get a test subject for: " .. name)
                failed_count = failed_count + 1
            else
                -- Run the test in a protected call to catch errors
                local status, err = pcall(func, TestUtils, subject)
                if status then
                    passed_count = passed_count + 1
                else
                    failed_count = failed_count + 1
                    TestUtils.Log("  [ERROR] Test crashed: " .. tostring(err))
                end
            end
        end
    end

    TestUtils.Log("----- Suite Complete: " .. subject_name .. " -----")
    TestUtils.Log("  > Summary: " .. passed_count .. " passed, " .. failed_count .. " failed.")
    TestUtils.Log("---------------------------------------")
end

return TestUtils
