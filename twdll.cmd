@echo off
setlocal enabledelayedexpansion

set "ROOT=%~dp0"
set "TWDLL_WRAPPER_ROOT=%ROOT:~0,-1%"
set "BIN=%ROOT%.twdll\bin\twdll.exe"

if exist "%BIN%" (
    "%BIN%" %*
    exit /b !errorlevel!
)

set "PORTABLE_GO=%ROOT%.twdll\tools\go\bin\go.exe"
if exist "%PORTABLE_GO%" goto :build

set "GO_VER=1.27.3"
for /f "usebackq tokens=1,2" %%A in ("%ROOT%cli\go.mod") do (
    if "%%A"=="go" set "GO_VER=%%B"
)

echo [twdll] Bootstrapping portable Go !GO_VER! into .twdll\tools\go... >&2
mkdir "%ROOT%.twdll\tools" >nul 2>nul
set "GO_ZIP=%ROOT%.twdll\tools\go.zip"
"%SystemRoot%\System32\curl.exe" -fL --progress-bar -o "%GO_ZIP%" "https://go.dev/dl/go!GO_VER!.windows-amd64.zip"
if !errorlevel! neq 0 (
    echo [twdll] Failed to download portable Go !GO_VER!. >&2
    exit /b 1
)

"%SystemRoot%\System32\tar.exe" -xf "%GO_ZIP%" -C "%ROOT%.twdll\tools"
if !errorlevel! neq 0 (
    del "%GO_ZIP%" >nul 2>nul
    echo [twdll] Failed to extract portable Go archive. >&2
    exit /b 1
)
del "%GO_ZIP%" >nul 2>nul

if not exist "%PORTABLE_GO%" (
    echo [twdll] Portable Go binary missing at %PORTABLE_GO%. >&2
    exit /b 1
)

:build
echo [twdll] Compiling CLI executable... >&2
mkdir "%ROOT%.twdll\bin" >nul 2>nul
pushd "%ROOT%cli"
set "TWDLL_NO_REBUILD=1"
"%PORTABLE_GO%" build -buildvcs=false -o "%BIN%" .
set "BUILD_ERR=!errorlevel!"
popd
if !BUILD_ERR! neq 0 (
    echo [twdll] CLI compilation failed. >&2
    exit /b !BUILD_ERR!
)

set "TWDLL_NO_REBUILD=1"
"%BIN%" %*
exit /b !errorlevel!
