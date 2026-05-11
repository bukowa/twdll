#pragma once

#include <string>

// Initializes the logger (e.g., clears the log file)
void init_logger();

// Logs a formatted message to twdll.log
void Log(const char* format, ...);

// Logs a simple string
void Log(const std::string& message);
