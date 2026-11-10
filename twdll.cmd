:; @() { true; }
@ goto startbatch

# ==============================================================================
# POSIX SHELL SECTION (Linux / macOS / WSL)
# (Placeholder: Linux testing is deferred until Windows pipeline is complete)
# ==============================================================================
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="$SCRIPT_DIR/.twdll/bin/twdll"

if [ -f "$BIN" ]; then
    exec "$BIN" "$@"
fi

PORTABLE_GO="$SCRIPT_DIR/.twdll/tools/go/bin/go"
if [ ! -f "$PORTABLE_GO" ]; then
    GO_VER="$(awk '$1 == "go" { print $2 }' "$SCRIPT_DIR/cli/go.mod" 2>/dev/null)"
    [ -z "$GO_VER" ] && GO_VER="1.27.3"
    echo "[twdll] Bootstrapping portable Go $GO_VER toolchain into .twdll/tools/go..." >&2
    mkdir -p "$SCRIPT_DIR/.twdll/tools"
    GO_TAR="$SCRIPT_DIR/.twdll/tools/go.tar.gz"
    curl -fL --progress-bar -o "$GO_TAR" "https://go.dev/dl/go$GO_VER.linux-amd64.tar.gz" || exit 1
    tar -xzf "$GO_TAR" -C "$SCRIPT_DIR/.twdll/tools"
    rm -f "$GO_TAR"
fi

mkdir -p "$SCRIPT_DIR/.twdll/bin"
(cd "$SCRIPT_DIR/cli" && "$PORTABLE_GO" build -buildvcs=false -o "$BIN" .)
exec "$BIN" "$@"

# ==============================================================================
# WINDOWS BATCH SECTION (cmd.exe / PowerShell)
# ==============================================================================
:startbatch
@echo off
setlocal enabledelayedexpansion

set "ROOT=%~dp0"
set "BIN=%ROOT%.twdll\bin\twdll.exe"

if not exist "%BIN%" goto :ensure_go

powershell -NoProfile -ExecutionPolicy Bypass -Command "if (-not (Test-Path '%BIN%')) { exit 1 }; $binTime = (Get-Item '%BIN%').LastWriteTimeUtc; if (@(Get-ChildItem -Path '%ROOT%cli' -Recurse -File | Where-Object { $_.LastWriteTimeUtc -gt $binTime }).Count -gt 0) { exit 1 } else { exit 0 }" >nul 2>nul
if !errorlevel! neq 0 goto :ensure_go
goto :run

:ensure_go
set "PORTABLE_GO=%ROOT%.twdll\tools\go\bin\go.exe"
if exist "%PORTABLE_GO%" (
    set "GO_BIN=%PORTABLE_GO%"
    goto :build
)

:: Extract exact Go version required from cli\go.mod
set "GO_VER=1.27.3"
for /f "tokens=1,2" %%A in (%ROOT%cli\go.mod) do (
    if "%%A"=="go" set "GO_VER=%%B"
)

:: Download pinned portable Go toolchain
echo [twdll] Bootstrapping portable Go !GO_VER! toolchain into .twdll\tools\go... >&2
mkdir "%ROOT%.twdll\tools" >nul 2>nul
set "GO_ZIP=%ROOT%.twdll\tools\go.zip"
curl.exe -fL --progress-bar -o "%GO_ZIP%" "https://go.dev/dl/go!GO_VER!.windows-amd64.zip"
if !errorlevel! neq 0 (
    echo [twdll] Failed to download Go !GO_VER! toolchain from https://go.dev/dl/go!GO_VER!.windows-amd64.zip. >&2
    exit /b 1
)

tar.exe -xf "%GO_ZIP%" -C "%ROOT%.twdll\tools"
del "%GO_ZIP%" >nul 2>nul

if not exist "%PORTABLE_GO%" (
    echo [twdll] Error: extracted Go binary not found at %PORTABLE_GO% >&2
    exit /b 1
)
set "GO_BIN=%PORTABLE_GO%"

:build
echo [twdll] Compiling CLI executable... >&2
mkdir "%ROOT%.twdll\bin" >nul 2>nul
cd /d "%ROOT%cli"
"%GO_BIN%" build -buildvcs=false -o "%BIN%" .
set "BUILD_STATUS=!errorlevel!"
cd /d "%ROOT%"
if !BUILD_STATUS! neq 0 (
    echo [twdll] Compilation failed. >&2
    exit /b !BUILD_STATUS!
)

:run
"%BIN%" %*
exit /b %errorlevel%
