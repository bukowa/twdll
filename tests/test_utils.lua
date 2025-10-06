-- A simple, non-intrusive assertion and test running framework for in-game use.

if not package.loaded["twdll"] then
    local _lib, err = package.loadlib("twdll.dll", "luaopen_twdll")
    if not _lib then
        error("FATAL: Failed to load twdll.dll: " .. tostring(err))
    end
    _lib()
end

local TestUtils = {}

function TestUtils.Log(message)
    if twdll and twdll.Log then
        twdll.Log(message)
    else
        print(message)
    end
end

--- Runs all functions in a given table that start with "test_".
-- @tparam table test_suite A table containing the test functions.
-- @tparam function get_subject_factory A function that returns a new test subject.
function TestUtils.run_tests(test_suite, get_subject_factory)
    local passed_count = 0
    local failed_count = 0
    local subject_name = test_suite.name or "Unknown Suite"

    TestUtils.Log("\n----- Running Test Suite: " .. subject_name .. " -----")

    for name, func in pairs(test_suite) do
        if name:match("^test_") and type(func) == "function" then
            TestUtils.Log("-> Executing: " .. name)

            -- FIX #1: Track assertion status for the CURRENT test.
            -- This variable will be flipped to `false` by our custom Assert function if any check fails.
            local test_passed_assertions = true

            -- FIX #2: Create a custom Assert function for each test run.
            -- This makes the test API cleaner and allows us to track failures.
            -- It will only log failures, reducing log spam.
            local function Assert(condition, message)
                if not condition then
                    TestUtils.Log("  [FAIL] " .. message)
                    test_passed_assertions = false
                end
                -- No more "[PASS]" logs for cleaner output.
            end

            local subject = get_subject_factory()
            if not subject then
                TestUtils.Log("  [FATAL] Could not get a test subject for: " .. name)
                failed_count = failed_count + 1
            else
                -- FIX #3: Call the test correctly and check both crash status AND assertion results.
                -- `pcall(func, test_suite, Assert, subject)`
                --   - `func`: The test function (e.g., UnitSuite:test_number_of_man_property)
                --   - `test_suite`: This becomes `self` inside the function because of the ':' syntax. CORRECT!
                --   - `Assert`: This is our custom assert function. CORRECT!
                --   - `subject`: This is the test subject (the unit). CORRECT!
                local status, err = pcall(func, test_suite, Assert, subject)

                if status and test_passed_assertions then
                    -- To pass, the test must NOT crash (status=true) AND all its assertions must have passed.
                    passed_count = passed_count + 1
                    TestUtils.Log("  [PASS]") -- A single pass message for the whole test function.
                else
                    failed_count = failed_count + 1
                    if not status then
                        -- The test crashed with a Lua error.
                        TestUtils.Log("  [ERROR] Test crashed: " .. tostring(err))
                    end
                    -- If it failed due to an assertion, the [FAIL] message was already logged by Assert().
                end
            end
        end
    end

    TestUtils.Log("----- Suite Complete: " .. subject_name .. " -----")
    TestUtils.Log("  > Summary: " .. passed_count .. " passed, " .. failed_count .. " failed.")
    TestUtils.Log("---------------------------------------")
end

return TestUtils