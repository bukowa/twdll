consul.console.clear()
package.loaded['tests.test_utils'] = nil

local test_files = {
    'tests.test_unit_script_interface',
    'tests.test_character_script_interface',
    --'tests.test_battle_unit_script_interface'
}

-- for each file clear from loaded packages
for _, file in ipairs(test_files) do
    if package.loaded[file] then
        package.loaded[file] = nil
    end
end

-- now require running the test
for _, file in ipairs(test_files) do
    require(file)
end
