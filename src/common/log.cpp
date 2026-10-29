#include "log.h"
#include <plog/Log.h>
#include <plog/Init.h>
#include <plog/Appenders/RollingFileAppender.h>
#include <plog/Util.h>
#include <iomanip>
#include <cstdio>
#include <cstdarg>
#include <cstring>
#include <string>
#include <unordered_map>
#include <mutex>
#include <atomic>
#include <algorithm>
#include <cctype>

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

class TwdllFormatter {
public:
    static plog::util::nstring header() {
        return "";
    }

    static plog::util::nstring format(const plog::Record& record) {
        tm t;
        plog::util::localtime_s(&t, &record.getTime().time);

        plog::util::nostringstream ss;
        ss << t.tm_year + 1900 << "-"
           << std::setfill(PLOG_NSTR('0')) << std::setw(2) << t.tm_mon + 1 << PLOG_NSTR("-")
           << std::setfill(PLOG_NSTR('0')) << std::setw(2) << t.tm_mday << PLOG_NSTR(" ")
           << std::setfill(PLOG_NSTR('0')) << std::setw(2) << t.tm_hour << PLOG_NSTR(":")
           << std::setfill(PLOG_NSTR('0')) << std::setw(2) << t.tm_min << PLOG_NSTR(":")
           << std::setfill(PLOG_NSTR('0')) << std::setw(2) << t.tm_sec << PLOG_NSTR(".")
           << std::setfill(PLOG_NSTR('0')) << std::setw(3) << static_cast<int>(record.getTime().millitm) << PLOG_NSTR(" ");

        const char* sev_str = plog::severityToString(record.getSeverity());
        ss << PLOG_NSTR("[") << sev_str << PLOG_NSTR("]");
        if (std::strlen(sev_str) <= 4) {
            ss << PLOG_NSTR("  ");
        } else {
            ss << PLOG_NSTR(" ");
        }

        ss << record.getMessage() << PLOG_NSTR("\n");
        return ss.str();
    }
};

std::string to_lower(const char* str) {
    if (!str) return "";
    std::string s(str);
    for (char& c : s) {
        c = static_cast<char>(std::tolower(static_cast<unsigned char>(c)));
    }
    return s;
}

static inline void to_upper_buf(const char* src, char* dst, size_t dst_size) {
    if (!src || dst_size == 0) {
        if (dst_size > 0) dst[0] = '\0';
        return;
    }
    size_t i = 0;
    for (; src[i] != '\0' && i + 1 < dst_size; ++i) {
        dst[i] = static_cast<char>(std::toupper(static_cast<unsigned char>(src[i])));
    }
    dst[i] = '\0';
}

static std::mutex g_registry_mutex;
static std::atomic<LogLevel> g_root_level{LogLevel::Info};
static std::atomic<LogLevel> g_core_level{LogLevel::Info};
static std::unordered_map<std::string, std::optional<LogLevel>> g_channel_overrides;

} // namespace

static const char* LOG_FILE = "twdll.log";
static plog::RollingFileAppender<TwdllFormatter, plog::NativeEOLConverter<RawConverter>> g_file_appender(LOG_FILE);

LogLevel parse_log_level(const char* str) {
    if (!str) return LogLevel::Info;
    std::string s = to_lower(str);
    if (s == "none" || s == "off") return LogLevel::None;
    if (s == "fatal") return LogLevel::Fatal;
    if (s == "error" || s == "err") return LogLevel::Error;
    if (s == "warn" || s == "warning") return LogLevel::Warn;
    if (s == "info") return LogLevel::Info;
    if (s == "debug") return LogLevel::Debug;
    if (s == "trace" || s == "verbose") return LogLevel::Trace;
    return LogLevel::Info;
}

const char* log_level_to_string(LogLevel level) {
    switch (level) {
        case LogLevel::None: return "none";
        case LogLevel::Fatal: return "fatal";
        case LogLevel::Error: return "error";
        case LogLevel::Warn: return "warn";
        case LogLevel::Info: return "info";
        case LogLevel::Debug: return "debug";
        case LogLevel::Trace: return "trace";
        default: return "info";
    }
}

static plog::Severity to_plog_severity(LogLevel level) {
    switch (level) {
        case LogLevel::None: return plog::none;
        case LogLevel::Fatal: return plog::fatal;
        case LogLevel::Error: return plog::error;
        case LogLevel::Warn: return plog::warning;
        case LogLevel::Info: return plog::info;
        case LogLevel::Debug: return plog::debug;
        case LogLevel::Trace: return plog::verbose;
        default: return plog::info;
    }
}

static void ensure_plog_initialized() {
    static const bool s_initialized = []() {
        plog::init(plog::none, &g_file_appender);
        return true;
    }();
    (void)s_initialized;
}

void set_logging_enabled(bool enabled) {
    ensure_plog_initialized();
    plog::get()->setMaxSeverity(enabled ? plog::verbose : plog::none);
    if (!enabled) {
        g_file_appender.setFileName(LOG_FILE);
    }
}

bool is_logging_enabled() {
    ensure_plog_initialized();
    return plog::get()->getMaxSeverity() != plog::none;
}

void set_root_log_level(LogLevel level) {
    g_root_level.store(level, std::memory_order_relaxed);
}

LogLevel get_root_log_level() {
    return g_root_level.load(std::memory_order_relaxed);
}

void set_channel_log_level(const char* channel, std::optional<LogLevel> level) {
    if (!channel) return;
    char ch_buf[64];
    to_upper_buf(channel, ch_buf, sizeof(ch_buf));

    if (strcmp(ch_buf, "CORE") == 0) {
        g_core_level.store(level.value_or(LogLevel::Info), std::memory_order_relaxed);
        return;
    }

    std::lock_guard<std::mutex> lock(g_registry_mutex);
    if (level.has_value()) {
        g_channel_overrides[ch_buf] = level;
    } else {
        g_channel_overrides.erase(ch_buf);
    }
}

LogLevel get_channel_effective_level(const char* channel) {
    if (!channel) return g_root_level.load(std::memory_order_relaxed);

    char ch_buf[64];
    to_upper_buf(channel, ch_buf, sizeof(ch_buf));

    if (strcmp(ch_buf, "CORE") == 0) {
        return g_core_level.load(std::memory_order_relaxed);
    }

    std::lock_guard<std::mutex> lock(g_registry_mutex);
    if (g_channel_overrides.empty()) {
        return g_root_level.load(std::memory_order_relaxed);
    }
    auto it = g_channel_overrides.find(ch_buf);
    if (it != g_channel_overrides.end() && it->second.has_value()) {
        return it->second.value();
    }
    return g_root_level.load(std::memory_order_relaxed);
}

void reset_log_registry() {
    std::lock_guard<std::mutex> lock(g_registry_mutex);
    g_root_level.store(LogLevel::Info, std::memory_order_relaxed);
    g_core_level.store(LogLevel::Info, std::memory_order_relaxed);
    g_channel_overrides.clear();
}

bool is_channel_level_enabled(const char* channel, LogLevel level) {
    if (!is_logging_enabled() || level == LogLevel::None) {
        return false;
    }
    LogLevel effective = get_channel_effective_level(channel);
    if (effective == LogLevel::None) {
        return false;
    }
    return static_cast<int>(level) <= static_cast<int>(effective);
}

void LogChannel(const char* channel, LogLevel level, const char* message) {
    if (!message || !is_channel_level_enabled(channel, level)) {
        return;
    }
    const char* ch = channel ? channel : "MAIN";
    std::string formatted = "[" + std::string(ch) + "] " + message;
    PLOG_(0, to_plog_severity(level)) << formatted;
}

void LogChannel(const char* channel, LogLevel level, const std::string& message) {
    LogChannel(channel, level, message.c_str());
}

static void LogV(const char* channel, LogLevel level, const char* format, va_list args) {
    if (!format || !is_channel_level_enabled(channel, level)) {
        return;
    }
    va_list args_copy;
    va_copy(args_copy, args);
    int len = vsnprintf(nullptr, 0, format, args_copy);
    va_end(args_copy);
    if (len < 0) return;

    std::string buffer(len, '\0');
    if (len > 0) {
        vsnprintf(&buffer[0], len + 1, format, args);
    }
    LogChannel(channel, level, buffer.c_str());
}

#define DEFINE_LOG_HELPER(fn_name, level) \
void fn_name(const char* format, ...) { \
    if (!format) return; \
    va_list args; \
    va_start(args, format); \
    LogV("CORE", level, format, args); \
    va_end(args); \
}

DEFINE_LOG_HELPER(LogInfo, LogLevel::Info)
DEFINE_LOG_HELPER(LogWarn, LogLevel::Warn)
DEFINE_LOG_HELPER(LogError, LogLevel::Error)
DEFINE_LOG_HELPER(LogDebug, LogLevel::Debug)
DEFINE_LOG_HELPER(LogTrace, LogLevel::Trace)
DEFINE_LOG_HELPER(LogFatal, LogLevel::Fatal)
DEFINE_LOG_HELPER(Log, LogLevel::Info)

void Log(const std::string& message) {
    LogChannel("CORE", LogLevel::Info, message);
}

void FlushLog() {
    // Plog writes directly to OS file descriptor with unbuffered _write on each call.
}
