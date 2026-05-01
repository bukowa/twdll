--[[
Import all the vanilla lua scripts first
--]]
local triggers = require "data.lua_scripts.export_triggers"
local ancillaries  = require "data.lua_scripts.export_ancillaries"
local historic_characters = require "data.lua_scripts.export_historic_characters"
local missions = require "data.lua_scripts.export_missions"
local encyclopedia = require "data.lua_scripts.export_encyclopedia"
local experience = require "data.lua_scripts.export_experience"
local political = require "data.lua_scripts.export_political_triggers"

events = triggers.events

-- Ensure logging to a file if something goes wrong
local __write = function(line)
    local f = io.open('consul.log', 'a')
    f:write(line)
    f:close()
end

__write('Starting consul\n')
-- Add the consul path to the package path
-- Warning: This means that if the game directory
-- contains a consul directory, it will be prioritized.
package.path = package.path .. ";consul/?.lua"

-- Load the consul module
__write('Loading consul\n')
local ok, result = pcall(require, 'consul')
if not ok then
    __write('Failed to load consul: ' .. tostring(result) .. '\n')
else
    __write('Loaded consul\n')
    local success, err = pcall(consul.setup)
    if not success then
        __write('Setup error: ' .. tostring(err) .. '\n')
    else
        __write('Setup successful\n')
    end
end

-- Load the twdll mod using package.loadlib
local ok, err = pcall(function()
    package.loadlib("twdll.dll", "luaopen_twdll")()
end)

if not ok or type(twdll) ~= "table" then
    -- Fallback logging if twdll fails to load
    local f = io.open('twdll_load_error.log', 'a')
    if f then
        f:write('Failed to load twdll: ' .. tostring(err) .. '\n')
        f:close()
    end
else
    -- twdll loaded successfully! We can now use its functions.
    twdll.Log("=========================================")
    twdll.Log("twdll loaded successfully via all_scripted.lua!")
    twdll.Log("=========================================")
end
