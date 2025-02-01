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

local libtwdll = require "script._lib.lib_twdll"
libtwdll.registerConsoleOnEvent("NewSession")
