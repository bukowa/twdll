#include "log.h"
#include "common.h"
#include "spdlog.h"
#include <spdlog/sinks/basic_file_sink.h>

void init_logger() {
    auto logger = spdlog::basic_logger_mt("file_logger", "libtwdll.log");
    spdlog::set_default_logger(logger);
    logger->flush_on(spdlog::level::trace);
#ifdef _DEBUG
    spdlog::set_level(spdlog::level::debug);
    spdlog::set_pattern("[%Y-%m-%d %H:%M:%S.%e] [%l] [%s:%# %!] %v");
#else
    spdlog::set_pattern("[%Y-%m-%d %H:%M:%S.%e] [%l] %v");
#endif
}
