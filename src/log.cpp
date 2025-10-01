#include <cstdio>
#include "log.h"

void Log(const char* message) {
    FILE* file = nullptr;
    fopen_s(&file, "libtwdll.log", "a"); // append mode
    if (!file) return;

    fprintf(file, "[libtwdll] %s\n", message);
    fclose(file);
}
