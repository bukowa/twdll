--[[
Import all the lua scripts
--]]

local triggers = require "data.lua_scripts.export_triggers"
local ancillaries  = require "data.lua_scripts.export_ancillaries"
local historic_characters = require "data.lua_scripts.export_historic_characters"
local missions = require "data.lua_scripts.export_missions"
local encyclopedia = require "data.lua_scripts.export_encyclopedia"
local experience = require "data.lua_scripts.export_experience"
local political = require "data.lua_scripts.export_political_triggers"

events = triggers.events
--[[
print(events, #events)

for n,v in ipairs(events.historical_events) do print(n, v) end
--]]

--
logG = require('script._lib.lib_logging').new_logger("all_scriptedG", "all_scriptedG.log", "TRACE")
logR = require('script._lib.lib_logging').new_logger("all_scriptedR", "all_scriptedR.log", "TRACE")
logE = require('script._lib.lib_logging').new_logger("all_scriptedE", "all_scriptedE.log", "TRACE")

logG:pretty_table(_G)
logR:pretty_table(debug.getregistry())
logE:pretty_table(getfenv())