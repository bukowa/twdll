-- Default log file path
local default_log_file_path = "log_lib_logging.txt"

-- Define logging levels
local levels = {
    DISABLED = 0,
    DEBUG = 1,
    INFO = 2,
    WARN = 3,
    ERROR = 4
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

    return self
end

-- Internal method to write a log entry to the file
function Logger:_write_log_entry(level, text)
    -- Open the log file in append mode
    local logfile, err = io.open(self.log_file_path, "a")
    if not logfile then
        error("Failed to open log file: " .. (err or "Unknown error"))
    end

    -- Add extra space for clean log message
    if #level == 4 then
        level = level .. " "
    end

    -- Format log entry with timestamp and logger name
    local timestamp = os.date("[%Y-%m-%d %H:%M:%S]")
    local log_entry = string.format("%s [%s] %s: %s", timestamp, level, self.name, text)

    -- Write the log entry and close the file
    logfile:write(log_entry .. "\n")
    logfile:close()
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

function Logger:require(moduleName)
    self:debug("Loading module: " .. moduleName)
    local success, result = pcall(require, moduleName)
    if not success then
        self:error("Error loading module '" .. moduleName .. "': " .. tostring(result))
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

-- Module return
return {
    new_logger = function(name, log_file_path, log_level)
        return Logger.new(name, log_file_path, log_level)
    end,
    levels = levels -- Expose logging levels for external use
}
