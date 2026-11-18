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

function Test-CliNeedsRebuild {
    param([string]$BinPath, [string]$RepoRoot)
    if (-not (Test-Path -LiteralPath $BinPath -PathType Leaf)) {
        return $true
    }
    if ($env:TWDLL_NO_REBUILD -eq '1') {
        return $false
    }
    $binTime = [System.IO.File]::GetLastWriteTimeUtc($BinPath)
    $cliDir = Join-Path $RepoRoot 'cli'
    $dirs = New-Object System.Collections.Generic.Stack[string]
    $dirs.Push($cliDir)
    while ($dirs.Count -gt 0) {
        $d = $dirs.Pop()
        foreach ($sub in [System.IO.Directory]::EnumerateDirectories($d)) {
            $name = [System.IO.Path]::GetFileName($sub)
            if ($name -ne '.git' -and $name -ne 'testdata') {
                $dirs.Push($sub)
            }
        }
        foreach ($f in [System.IO.Directory]::EnumerateFiles($d)) {
            $name = [System.IO.Path]::GetFileName($f)
            if ($name.EndsWith('_test.go')) { continue }
            if ($name -eq 'go.mod' -or $name -eq 'go.sum' -or $name -match '\.(go|java|idc|py)$') {
                if ([System.IO.File]::GetLastWriteTimeUtc($f) -gt $binTime) {
                    return $true
                }
            }
        }
    }
    return $false
}

if (-not (Test-CliNeedsRebuild -BinPath $Bin -RepoRoot $ScriptDir)) {
    & $Bin @args
    exit $LASTEXITCODE
}

$PortableGo = $null
if ($env:TWDLL_TOOLS_DIR) {
    $externalGo = Join-Path (Join-Path $env:TWDLL_TOOLS_DIR 'go') (Join-Path 'bin' ("go" + $exeSuffix))
    if (Test-Path -LiteralPath $externalGo -PathType Leaf) {
        $PortableGo = $externalGo
    }
}
if (-not $PortableGo) {
    $ToolsDir = Join-Path (Join-Path $ScriptDir '.twdll') 'tools'
    $PortableGo = Join-Path (Join-Path (Join-Path $ToolsDir 'go') 'bin') ("go" + $exeSuffix)
} else {
    $ToolsDir = Join-Path (Join-Path $ScriptDir '.twdll') 'tools'
}

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
    & $PortableGo build -buildvcs=false -o $Bin .
    $buildExit = $LASTEXITCODE
} finally {
    Pop-Location
}
if ($buildExit -ne 0) {
    [Console]::Error.WriteLine("[twdll] CLI compilation failed.")
    exit $buildExit
}

& $Bin @args
exit $LASTEXITCODE
