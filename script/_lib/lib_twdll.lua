-- Import logging lib
local lib_logging = require "script._lib.lib_logging"

-- Create logger instance with a custom name
local logger = lib_logging.new_logger("lib_twdll.lua", "libtwdll.log", "DEBUG")

--- Loads the library and starts the console.
local function start_console()
    logger:info("Loading library...")
    local ok, libtwdll_or_err = pcall(package.loadlib, "libtwdll.dll", "luaopen_libtwdll")
    if not ok then
        logger:error("Failed to load library: " .. tostring(libtwdll_or_err))
        return
    end

    local libtwdll = libtwdll_or_err

    logger:info("Library loaded successfully. Spawning console...")
    local ok, err = pcall(libtwdll)
    if not ok then
        logger:error("Failed to spawn console: " .. tostring(err))
        return
    end
    logger:info("Console started successfully!")
end

--- Registers the event to trigger library loading.
-- @param event The event name that will trigger the library load.
local function registerConsoleOnEvent(event)
    logger:debug("Registering console to start on event: " .. event)

    local core = require "ui/CoreUtils";
    local scripting = core.Require "lua_scripts.EpisodicScripting";

    logger:debug((function(tbl) local str = "" for k, v in pairs(tbl) do str = str .. tostring(k) .. ": " .. tostring(v) .. "\n" end return str end)(_G))

    scripting.AddEventCallBack(event, function(context)
        logger:debug("Starting console inside event: " .. context.string)
        start_console()
    end)
end

return {
    start_console = start_console,
    registerConsoleOnEvent = registerConsoleOnEvent,
}
