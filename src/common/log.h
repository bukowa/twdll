#pragma once

#include <string>

// Logs a formatted message to twdll.log
void Log(const char* format, ...);

// Logs a simple string
void Log(const std::string& message);
