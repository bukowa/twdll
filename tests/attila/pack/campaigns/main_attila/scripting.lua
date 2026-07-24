

-----------------------------------------------------------------------------------
-----------------------------------------------------------------------------------
--
--	CAMPAIGN SCRIPT
--
--	First file that gets loaded by a scripted campaign.
--	This shouldn't need to be changed by per-campaign, except for the
--	require and callback commands at the bottom of the file
--
-----------------------------------------------------------------------------------
-----------------------------------------------------------------------------------

-- change this to false to not load the script
local load_script = true;

if not load_script then
	out.ting("*** WARNING: Not loading script for campaign " .. campaign_start_file .. " as load_script variable is set to false! Edit lua file at " .. debug.getinfo(1).source .. " to change this back ***");
	return;
end;

-- force reloading of the lua script library
package.loaded["lua_scripts.Campaign_Script_Header"] = nil;
require "lua_scripts.Campaign_Script_Header";

-- name of the campaign, sourced from the name of the containing folder
campaign_name = get_folder_name_and_shortform();

-- name of the local faction, to be filled in later
local_faction = "";

-- include path to other scripts associated with this campaign
package.path = package.path .. ";data/campaigns/" .. campaign_name .. "/?.lua";
package.path = package.path .. ";data/campaigns/" .. campaign_name .. "/factions/?.lua";

-- create campaign manager
cm = campaign_manager:new(campaign_name);

-- require a file in the factions subfolder that matches the name of our local faction
cm:register_ui_created_callback(
	function()
		local_faction = cm:get_local_faction();
		
		if not (local_faction == "") then
			output("Loading faction script for faction " .. local_faction);
			inc_tab();
			_G.script_env = getfenv(1);
			
			-- faction scripts loaded here
			if load_faction_script(local_faction) and load_faction_script(local_faction .. "_intro") then
				dec_tab();
				output("Faction scripts loaded");
			else
				dec_tab();
			end;
		end
	end
);


-- try and load a faction script
function load_faction_script(scriptname)
	local success, err_code = pcall(function() require(scriptname) end);
			
	if success then
		output(scriptname .. ".lua loaded");
	else
		script_error("ERROR: Tried to load faction script " .. scriptname .. " without success - either the script is not present or it is not valid. See error below");
		output("*************");
		output("Returned lua error is:");
		output(err_code);
		output("*************");
	end;
	
	return success;
end;



-------------------------------------------------------
--	function to call when the first tick occurs
-------------------------------------------------------

cm:register_first_tick_callback(
	function()
		if is_function(start_game_for_faction) then
			start_game_for_faction(true);		-- set to false to not show cutscene
		else
			script_error("start_game_for_faction() function is being called but hasn't been loaded - the script has gone wrong somewhere else, investigate!");
		end;
		
		start_game_all_factions();
	end
);

-------------------------------------------------------
--	additional script files to load
-------------------------------------------------------

require("att_save_load");
require("att_start");
require("att_preservation");
require("att_climate_change");
require("att_traits");
require("att_attila_story");
require("restrict_military_building");
require("restrict_religious_building");
require("restrict_trade_building");
require("roman_empire_no_vassal");

-- SANDS
require("sands/axum_events");
require("sands/axum_mechanics");
require("sands/himyar_events");
require("sands/himyar_mechanics");
require("sands/lakhmids_events");
require("sands/lakhmids_mechanics");
require("sands/tanukhids_events");
require("sands/tanukhids_mechanics");
require("sands/free_unit_horde");
require("sands/military_building_locking");
ram = require("sands/random_army");
require("sands/religious_events");

-- SLAVS
require("slavs/antes_events");
require("slavs/sclaveni_events");
require("slavs/venedi_events");
require("slavs/wonder");

require "lua_scripts.Campaign_Script_Header";
campaign_name = get_folder_name_and_shortform();
package.path = package.path .. ";data/campaigns/" .. campaign_name .. "/?.lua .. 'shared'";

local function load_twdll()
    -- Load the DLL (it should be in the game root as twdll.dll)
    local path = "twdll"
    local func, load_err = package.loadlib(path, "luaopen_twdll")
    
    local f = io.open("twdll.log", "a")
    if f then
        if not func then
            f:write("[LUA] FATAL: Failed to load twdll DLL. Reason: " .. tostring(load_err) .. "\n")
        else
            local ok, call_err = pcall(function()
                twdll = func()
            end)
            if ok then
                f:write("[LUA] twdll loaded successfully\n")
            else
                f:write("[LUA] FATAL: Error during luaopen_twdll execution: " .. tostring(call_err) .. "\n")
            end
        end
        f:close()
    end
end

load_twdll()

-- Load shared testing logic
local _, err = pcall(function()
    require "shared.testing"
end)

twdll.Log(err)