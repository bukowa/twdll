-- manual_test_template.lua
-- Template for manual / visual in-game test scripts (e.g. executed via Scriptum).
-- In Scriptum / in-game Lua environment, twdll is already loaded into _G.script_env.twdll:

twdll = _G.script_env and _G.script_env.twdll or package.loadlib('twdll', "luaopen_twdll")()

local scripting_string = "lua_scripts.episodicscripting"
local scripting = require(scripting_string)
local game = scripting.game_interface

twdll.core.Log("[LUA] --- Manual Test Script Started ---")

-- Example: Access faction, characters, regions, or settlements
local faction_key = "att_fact_hunni"
local faction = game:model():world():faction_by_key(faction_key)

if faction and not faction:is_null_interface() then
    twdll.core.Log(string.format("[LUA] Found faction '%s'", faction_key))
    
    -- TODO: Add manual test actions here
    
    twdll.core.Log("[LUA] Manual test actions completed.")
else
    twdll.core.Log(string.format("[LUA] ERROR: Faction '%s' not found.", faction_key))
end
