#include <cstdio>
#include <cstdarg> // Required for variadic arguments
#include "log.h"

void Log(const char* format, ...) {
    FILE* file = nullptr;
    fopen_s(&file, "libtwdll.log", "a"); // append mode
    if (!file) return;

    va_list args;
    va_start(args, format);
    vfprintf(file, format, args);
    va_end(args);
    fprintf(file, "\n"); // Add a newline after each log entry
    fclose(file);
}
