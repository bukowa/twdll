-- Default log file path
local default_log_file_path = "log_lib_logging.txt"

-- Define logging levels
local levels = {
    DISABLED = -1,
    INTERNAL = 0,
    DEBUG = 1,
    INFO = 2,
    WARN = 3,
    ERROR = 4,
    CRITICAL = 5,
    TRACE = 6,
}

-- Logger class-like structure
local Logger = {}
Logger.__index = Logger

-- Constructor for creating a new logger instance
function Logger.new(name, log_file_path, log_level)
    local self = setmetatable({}, Logger)

    self.name = name or "Unnamed Logger"  -- Default to "Unnamed Logger" if no name is provided
    self.log_file_path = log_file_path or default_log_file_path
    self.log_level = levels[log_level] or levels.INFO -- Default to INFO if an invalid level is provided

    -- Internal Libraries
    self.lib_pl__pretty = self:require('script._lib.penlight.pretty')
    return self
end


function Logger:pretty_table(table)
    self:raw(self.lib_pl__pretty.write(table))
end

-- Internal method to write directly to the log file
function Logger:_write_to_file(text)
    local logfile, err = io.open(self.log_file_path, "a")
    if not logfile then
        error("Failed to open log file: " .. (err or "Unknown error"))
    end
    logfile:write(text .. "\n")
    logfile:close()
end

-- Internal method to write a log entry with level and formatting
function Logger:_write_log_entry(level, text)
    if #level == 4 then
        level = level .. " "  -- Add extra space for alignment
    end
    local timestamp = os.date("[%Y-%m-%d %H:%M:%S]")
    local log_entry = string.format("%s [%s] %s: %s", timestamp, level, self.name, text)
    self:_write_to_file(log_entry)
end

-- Public method to write raw text to the log file
function Logger:raw(text)
    self:_write_to_file(text)
end

-- Generic log function
function Logger:log(level, text)
    text = tostring(text or "")
    level = tostring(level or "INFO")

    -- Skip logging if the logger is disabled or the level is below the current threshold
    if self.log_level == levels.DISABLED or levels[level] < self.log_level then
        return
    end

    -- Write the log entry
    self:_write_log_entry(level, text)
end

-- Convenience methods for specific log levels
function Logger:debug(text)
    self:log("DEBUG", text)
end

function Logger:info(text)
    self:log("INFO", text)
end

function Logger:warn(text)
    self:log("WARN", text)
end

function Logger:error(text)
    self:log("ERROR", text)
end

function Logger:internal(text)
    self:log("INTERNAL", text)
end

function Logger:critical(text)
    self:log("CRITICAL", text)
end

function Logger:trace(text)
    self:log("TRACE", text)
end

-- Safely execute a function and log errors if it fails
function Logger:pcall(func, ...)
    -- Skip error logging if the logger is disabled
    if self.log_level == levels.DISABLED then
        return pcall(func, ...)
    end

    local success, result = pcall(func, ...)
    if not success then
        self:error("Error during execution: " .. tostring(result))
    end
    return success, result
end

function Logger:require(moduleName, is_critical_module)
    self:debug("Loading module: '" .. moduleName .. "'")
    local success, result = pcall(require, moduleName)
    if not success then

        if is_critical_module then
            self:critical("Critical error loading module: '" .. moduleName .. "'")
            -- Quit
            if CliExecute ~= nil then CliExecute('quit') end
        else
            self:error("Error loading module: '" .. moduleName .. "': " .. tostring(result))
        end

        return nil
    end
    self:debug("Module loaded: " .. moduleName)
    return result
end

-- Set the logging level dynamically
function Logger:set_level(level)
    if levels[level] then
        self.log_level = levels[level]
    else
        error("Invalid log level: " .. tostring(level))
    end
end

function Logger:start_trace(mask, write_log_entry, callback)
    mask = (mask == nil) and "c" or mask
    write_log_entry = (write_log_entry == nil) and true or write_log_entry

    debug.sethook(function(event)
        local info = debug.getinfo(3, 'nSluf')

        -- Add event to info table
        if info == nil then
            info = {
                ["event"] = event
            }
        else
            info['event'] = event
        end

        local message = string.format(
                "Event: %s | Function: %s | Source: %s | Line: %d",
                event,
                info.name or "unknown",
                info.short_src or "unknown",
                info.currentline or -1
        )
        if write_log_entry then
            self:trace(message)
        end
        if callback then
            self:pcall(callback, event, info)
        end
    end, mask)
end

-- Method to stop the trace hook
function Logger:stop_trace()
    -- Remove the hook by passing nil
    debug.sethook(nil)
end

-- Log events based on a filter function, with always-filtered events
function Logger:log_events(events, filter_func)

    -- Define events to always filter out
    local always_filtered = {
        ["_PACKAGE"] = true,
        ["_M"] = true,
        ["_NAME"] = true,
    }

    -- Load scripting module
    local scripting = self:require('lua_scripts.episodicscripting')

    -- Loop through all events and apply the filter
    for event, _ in pairs(events) do

        -- Skip always filtered events
        if not always_filtered[event] and filter_func(event) then

            -- Attempt to register the event callback with error handling
            local success, err = pcall(function()
                scripting.AddEventCallBack(event, function()
                    self:info("Event fired: " .. event)
                end)
            end)

            -- If callback registration fails, log the error
            if not success then
                self:error("Failed to register logging for event: " .. event .. " Error: " .. err)
            else
                self:internal("Successfully registered logging for event: " .. event)  -- Log again when the event is fired
            end
        end
    end
end

-- Log all events
function Logger:log_events_all()
    -- Import all events
    local all_events = self:require('lua_scripts.events')
    -- Log events
    self:log_events(all_events, function(e)
        return true
    end)
end

-- Log all events excluding specified events
function Logger:log_events_all_excluding(excluded_events_list)
    -- Import all events
    local all_events = self:require('lua_scripts.events')

    -- Convert list to a lookup table for quick filtering
    local excluded_events = {}
    for _, event in ipairs(excluded_events_list) do
        excluded_events[event] = true
    end

    -- Create a filter function that excludes the specified events
    local function filter_func(event)
        return not excluded_events[event]  -- Only log events not in the excluded list
    end

    -- Log events with the custom filter
    self:log_events(all_events, filter_func)
end

-- Module return
return {
    new_logger = function(name, log_file_path, log_level)
        return Logger.new(name, log_file_path, log_level)
    end,
    levels = levels -- Expose logging levels for external use
}
