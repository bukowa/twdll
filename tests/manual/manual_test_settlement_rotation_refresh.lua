twdll = _G.script_env.twdll

local scripting_string = "lua_scripts.episodicscripting"
local scripting = require(scripting_string)
local game = scripting.game_interface

local region_key = "att_reg_sarmatia_europaea_olbia"
local reg = game:model():world():region_manager():region_by_key(region_key)
local settlement = reg and reg:settlement()

if settlement and not settlement:is_null_interface() then
    local slot_list = settlement:slot_list()
    local num_slots = slot_list:num_items()

    twdll.core.Log(string.format("[LUA] Rotating settlement '%s' (%d slots)...", region_key, num_slots))

    for i = 0, num_slots - 1 do
        local slot = slot_list:item_at(i)
        local current_rot = slot:GetBuildingRotation()


        local new_rot = math.random(0, 5)

        slot:SetBuildingRotation(new_rot)
        twdll.core.Log(string.format("[LUA]   Slot %d: rotacja %s -> %d", i, tostring(current_rot), new_rot))
    end


    twdll.campaign_ui.RefreshSettlements()
    twdll.core.Log("[LUA] Settlement display refreshed successfully.")
else
    twdll.core.Log(string.format("[LUA] ERROR: Settlement '%s' not found.", region_key))
end