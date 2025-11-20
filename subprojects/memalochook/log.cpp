// log.cpp
#include "log.h"
#include "spdlog/sinks/basic_file_sink.h"

std::shared_ptr<spdlog::logger> g_logger;

void InitializeLogging() {
    try {
        g_logger = spdlog::basic_logger_mt("memalochook_logger", "memalochook_log.txt", true);
        spdlog::set_default_logger(g_logger);
        spdlog::flush_on(spdlog::level::info);
        spdlog::info("Logging initialized.");
    }
    catch (const spdlog::spdlog_ex& ex) {
        // Logging to file failed, maybe try to log to debug output
    }
}

void ShutdownLogging() {
    spdlog::info("Logging shutdown.");
    spdlog::shutdown();
}
