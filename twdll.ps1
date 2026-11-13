#!/usr/bin/env pwsh
$ErrorActionPreference = 'Stop'

$ScriptDir = $PSScriptRoot
if (-not $ScriptDir) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
}
$env:TWDLL_WRAPPER_ROOT = $ScriptDir

$onWindows = ($env:OS -eq 'Windows_NT') -or ([System.Environment]::OSVersion.Platform -eq 'Win32NT')
$onMacOS = $false
if (-not $onWindows) {
    try {
        $onMacOS = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::OSX)
    } catch {
        $onMacOS = $false
    }
}

$exeSuffix = if ($onWindows) { '.exe' } else { '' }
$BinDir = Join-Path (Join-Path $ScriptDir '.twdll') 'bin'
$Bin = Join-Path $BinDir ("twdll" + $exeSuffix)

if (Test-Path -LiteralPath $Bin -PathType Leaf) {
    & $Bin @args
    exit $LASTEXITCODE
}

$ToolsDir = Join-Path (Join-Path $ScriptDir '.twdll') 'tools'
$PortableGo = Join-Path (Join-Path (Join-Path $ToolsDir 'go') 'bin') ("go" + $exeSuffix)

if (-not (Test-Path -LiteralPath $PortableGo -PathType Leaf)) {
    $GoVer = '1.27.3'
    $GoMod = Join-Path (Join-Path $ScriptDir 'cli') 'go.mod'
    if (Test-Path -LiteralPath $GoMod -PathType Leaf) {
        foreach ($line in [System.IO.File]::ReadAllLines($GoMod)) {
            if ($line -match '^\s*go\s+(\S+)') {
                $GoVer = $Matches[1]
                break
            }
        }
    }

    $osName = if ($onWindows) { 'windows' } elseif ($onMacOS) { 'darwin' } else { 'linux' }
    $archName = 'amd64'
    try {
        $osArch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
        if ($osArch -eq 'arm64') {
            $archName = 'arm64'
        }
    } catch {
        if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') {
            $archName = 'arm64'
        }
    }

    [Console]::Error.WriteLine("[twdll] Bootstrapping portable Go $GoVer ($osName/$archName) into .twdll/tools/go...")
    [void][System.IO.Directory]::CreateDirectory($ToolsDir)

    $ext = if ($onWindows) { 'zip' } else { 'tar.gz' }
    $ArchivePath = Join-Path $ToolsDir ("go." + $ext)
    $Url = "https://go.dev/dl/go$GoVer.$osName-$archName.$ext"

    $curlCmd = if ($onWindows -and (Test-Path "$env:SystemRoot\System32\curl.exe")) { "$env:SystemRoot\System32\curl.exe" } else { 'curl' }
    & $curlCmd -fL --progress-bar -o $ArchivePath $Url
    if ($LASTEXITCODE -ne 0) {
        [Console]::Error.WriteLine("[twdll] Failed to download portable Go $GoVer.")
        exit 1
    }

    $tarCmd = if ($onWindows -and (Test-Path "$env:SystemRoot\System32\tar.exe")) { "$env:SystemRoot\System32\tar.exe" } else { 'tar' }
    if ($onWindows) {
        & $tarCmd -xf $ArchivePath -C $ToolsDir
    } else {
        & $tarCmd -xzf $ArchivePath -C $ToolsDir
    }
    $tarExit = $LASTEXITCODE
    Remove-Item -LiteralPath $ArchivePath -Force -ErrorAction SilentlyContinue
    if ($tarExit -ne 0) {
        [Console]::Error.WriteLine("[twdll] Failed to extract portable Go archive.")
        exit 1
    }

    if (-not (Test-Path -LiteralPath $PortableGo -PathType Leaf)) {
        [Console]::Error.WriteLine("[twdll] Portable Go binary missing at $PortableGo.")
        exit 1
    }
}

[Console]::Error.WriteLine("[twdll] Compiling CLI executable...")
[void][System.IO.Directory]::CreateDirectory($BinDir)
$CliDir = Join-Path $ScriptDir 'cli'
Push-Location -LiteralPath $CliDir
try {
    $env:TWDLL_NO_REBUILD = '1'
    & $PortableGo build -buildvcs=false -o $Bin .
    $buildExit = $LASTEXITCODE
} finally {
    Pop-Location
}
if ($buildExit -ne 0) {
    [Console]::Error.WriteLine("[twdll] CLI compilation failed.")
    exit $buildExit
}

$env:TWDLL_NO_REBUILD = '1'
& $Bin @args
exit $LASTEXITCODE
