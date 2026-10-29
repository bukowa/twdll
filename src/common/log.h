#pragma once

#include <string>

// Controls whether logging to twdll.log is active (disabled by default)
void set_logging_enabled(bool enabled);
bool is_logging_enabled();

// Logs a formatted message to twdll.log
void Log(const char* format, ...);

// Logs a simple string
void Log(const std::string& message);
