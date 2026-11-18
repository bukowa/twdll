@echo off
setlocal enabledelayedexpansion

set "ROOT=%~dp0"
set "TWDLL_WRAPPER_ROOT=%ROOT:~0,-1%"
set "BIN=%ROOT%.twdll\bin\twdll.exe"

if exist "%BIN%" (
    if "%TWDLL_NO_REBUILD%"=="1" goto :run
    powershell -NoProfile -ExecutionPolicy Bypass -Command "$bt=[System.IO.File]::GetLastWriteTimeUtc($env:BIN);$cd=[System.IO.Path]::Combine($env:TWDLL_WRAPPER_ROOT,'cli');$s=New-Object System.Collections.Generic.Stack[string];$s.Push($cd);while($s.Count -gt 0){$d=$s.Pop();foreach($sub in [System.IO.Directory]::EnumerateDirectories($d)){$n=[System.IO.Path]::GetFileName($sub);if($n -ne '.git' -and $n -ne 'testdata'){$s.Push($sub)}};foreach($f in [System.IO.Directory]::EnumerateFiles($d)){$n=[System.IO.Path]::GetFileName($f);if($n.EndsWith('_test.go')){continue};if($n -eq 'go.mod' -or $n -eq 'go.sum' -or $n -match '\.(go|java|idc|py)$'){if([System.IO.File]::GetLastWriteTimeUtc($f) -gt $bt){exit 1}}}};exit 0" >nul 2>nul
    if !errorlevel! equ 0 goto :run
)

if defined TWDLL_TOOLS_DIR (
    if exist "%TWDLL_TOOLS_DIR%\go\bin\go.exe" (
        set "PORTABLE_GO=%TWDLL_TOOLS_DIR%\go\bin\go.exe"
        goto :build
    )
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
"%PORTABLE_GO%" build -buildvcs=false -o "%BIN%" .
set "BUILD_ERR=!errorlevel!"
popd
if !BUILD_ERR! neq 0 (
    echo [twdll] CLI compilation failed. >&2
    exit /b !BUILD_ERR!
)

:run
"%BIN%" %*
exit /b !errorlevel!
