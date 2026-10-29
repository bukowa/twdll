#include "log.h"
#include <plog/Log.h>
#include <plog/Init.h>
#include <plog/Appenders/RollingFileAppender.h>
#include <plog/Formatters/MessageOnlyFormatter.h>
#include <cstdio>
#include <cstdarg>
#include <string>

namespace {
class RawConverter {
public:
    static std::string header(const plog::util::nstring&) {
        return "";
    }
#if PLOG_CHAR_IS_UTF8
    static const std::string& convert(const plog::util::nstring& str) {
        return str;
    }
#else
    static std::string convert(const plog::util::nstring& str) {
        return plog::util::toNarrow(str, plog::codePage::kUTF8);
    }
#endif
};
} // namespace

static const char* LOG_FILE = "twdll.log";
static plog::RollingFileAppender<plog::MessageOnlyFormatter, plog::NativeEOLConverter<RawConverter>> g_file_appender(LOG_FILE);

static void ensure_plog_initialized() {
    static const bool s_initialized = []() {
        plog::init(plog::none, &g_file_appender);
        return true;
    }();
    (void)s_initialized;
}

void set_logging_enabled(bool enabled) {
    ensure_plog_initialized();
    plog::get()->setMaxSeverity(enabled ? plog::info : plog::none);
    if (!enabled) {
        g_file_appender.setFileName(LOG_FILE);
    }
}

bool is_logging_enabled() {
    ensure_plog_initialized();
    return plog::get()->getMaxSeverity() != plog::none;
}

void Log(const char* format, ...) {
    if (!format || !is_logging_enabled()) {
        return;
    }

    va_list args;
    va_start(args, format);
    va_list args_copy;
    va_copy(args_copy, args);
    int len = vsnprintf(nullptr, 0, format, args_copy);
    va_end(args_copy);

    if (len < 0) {
        va_end(args);
        return;
    }

    std::string buffer(len, '\0');
    if (len > 0) {
        vsnprintf(&buffer[0], len + 1, format, args);
    }
    va_end(args);

    PLOG_INFO << buffer;
}

void Log(const std::string& message) {
    if (!is_logging_enabled()) {
        return;
    }
    PLOG_INFO << message;
}
