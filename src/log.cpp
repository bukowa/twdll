#include "log.h"
#include <spdlog/sinks/basic_file_sink.h>

void init_logger() {
    auto logger = spdlog::basic_logger_mt("file_logger", "libtwdll.log");
    spdlog::set_default_logger(logger);
#ifdef _DEBUG
    spdlog::set_pattern("[%Y-%m-%d %H:%M:%S.%e] [%l] [%s:%# %!] %v");
#else
    spdlog::set_pattern("[%Y-%m-%d %H:%M:%S.%e] [%l] %v");
#endif
}