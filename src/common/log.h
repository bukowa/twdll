#pragma once

#include <string>
#include <optional>

// Severity levels matching plog::Severity values
enum class LogLevel {
    None = 0,
    Fatal = 1,
    Error = 2,
    Warn = 3,
    Info = 4,
    Debug = 5,
    Trace = 6
};

// String <-> LogLevel parsing and formatting
LogLevel parse_log_level(const char* str);
const char* log_level_to_string(LogLevel level);

// Master switch controlling whether logging to twdll.log is active (disabled by default)
void set_logging_enabled(bool enabled);
bool is_logging_enabled();

// Root log level (default for child channels that don't have an override)
void set_root_log_level(LogLevel level);
LogLevel get_root_log_level();

// Channel configuration and inheritance
void set_channel_log_level(const char* channel, std::optional<LogLevel> level);
LogLevel get_channel_effective_level(const char* channel);
bool is_channel_level_enabled(const char* channel, LogLevel level);

// Multi-channel logging functions
void LogChannel(const char* channel, LogLevel level, const char* message);
void LogChannel(const char* channel, LogLevel level, const std::string& message);

// C++ engine logging helpers (Channel: "CORE")
void LogInfo(const char* format, ...);
void LogWarn(const char* format, ...);
void LogError(const char* format, ...);
void LogDebug(const char* format, ...);
void LogTrace(const char* format, ...);
void LogFatal(const char* format, ...);

// Legacy compatibility: logs [INFO] [CORE]
void Log(const char* format, ...);
void Log(const std::string& message);

// Flushes the log file buffers to physical disk
void FlushLog();

// Resets root and channel levels back to initial defaults and clears all overrides
void reset_log_registry();
