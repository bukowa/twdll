@echo off
reg add "HKEY_CURRENT_USER\SOFTWARE\The Creative Assembly\Rome2" /v last_game_version /t REG_DWORD /d 0x00020004 /f
