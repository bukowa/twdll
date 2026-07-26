#pragma once

struct TW_GameSigInfo {
    const char* name;
    void**      target;
    const char* sig;
};

extern const TW_GameSigInfo g_game_signatures[];

void initialize_game_api();
