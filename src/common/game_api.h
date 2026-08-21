#pragma once
#include <windows.h>

struct TW_GameSigInfo {
    const char* name;
    void**      target;
    const char* sig;
};

extern const TW_GameSigInfo g_game_signatures[];
extern HMODULE g_empire_module;

void initialize_game_api();
