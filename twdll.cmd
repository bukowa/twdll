@echo off
setlocal EnableDelayedExpansion

set "REPO_ROOT=%~dp0"
if "%REPO_ROOT:~-1%"=="\" set "REPO_ROOT=%REPO_ROOT:~0,-1%"
set "UV_ROOT=%REPO_ROOT%\.twdll\uv"
set "UV_BIN_DIR=%UV_ROOT%\bin"
set "UV_BIN=%UV_BIN_DIR%\uv.exe"
set "UV_VERSION=0.13.9"

if not exist "%UV_BIN%" (
    if not exist "%UV_BIN_DIR%" mkdir "%UV_BIN_DIR%"
    set "UV_URL=https://github.com/astral-sh/uv/releases/download/%UV_VERSION%/uv-x86_64-pc-windows-msvc.zip"
    set "TMP_ZIP=%UV_ROOT%\uv.zip"
    powershell -NoProfile -ExecutionPolicy Bypass -Command ^
        "$ProgressPreference = 'SilentlyContinue'; Invoke-WebRequest -Uri '!UV_URL!' -OutFile '!TMP_ZIP!'; Expand-Archive -Path '!TMP_ZIP!' -DestinationPath '!UV_BIN_DIR!' -Force; Remove-Item '!TMP_ZIP!' -Force"
    if errorlevel 1 (
        echo Failed to bootstrap portable uv. >&2
        exit /b 1
    )
)

set "UV_CACHE_DIR=%UV_ROOT%\cache"
set "UV_PYTHON_INSTALL_DIR=%UV_ROOT%\python"
set "UV_PROJECT_ENVIRONMENT=%UV_ROOT%\venv"
set "UV_NO_CONFIG=1"

"%UV_BIN%" run --quiet --project "%REPO_ROOT%\cli" twdll %*
exit /b %ERRORLEVEL%
