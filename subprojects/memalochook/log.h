// log.h
#pragma once

#include "spdlog.h"

void InitializeLogging();
void ShutdownLogging();

extern std::shared_ptr<spdlog::logger> g_logger;
