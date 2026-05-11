#include "log.h"
#include <cstdio>
#include <cstdarg>
#include <fstream>
#include <mutex>
#include <windows.h>
#include <share.h>

static std::mutex g_log_mutex;
static const char* LOG_FILE = "twdll.log";

void init_logger() {
    std::lock_guard<std::mutex> lock(g_log_mutex);
    // Clear the log file on startup
    FILE* f = _fsopen(LOG_FILE, "w", _SH_DENYNO);
    if (f) {
        fclose(f);
    }
}

void Log(const char* format, ...) {
    std::lock_guard<std::mutex> lock(g_log_mutex);
    
    FILE* f = _fsopen(LOG_FILE, "a", _SH_DENYNO);
    if (f) {
        va_list args;
        va_start(args, format);
        vfprintf(f, format, args);
        fprintf(f, "\n");
        va_end(args);
        fclose(f);
    }
}

void Log(const std::string& message) {
    Log("%s", message.c_str());
}
