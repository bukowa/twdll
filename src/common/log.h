#pragma once

#include <string>

// Logs a formatted message to <namespace>.log (twdll.log by default)
void Log(const char* format, ...);

// Logs a simple string
void Log(const std::string& message);
