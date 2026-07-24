# .SYNOPSIS
#     twdll.ps1 - Task runner for twdll mod development.

param(
    [string] $Command = "help",
    [string] $Game = "rome2",
    [string] $DllPath = "",
    [switch] $Steam
)

$ErrorActionPreference = "Stop"

$Root = Split-Path $PSScriptRoot -Parent
$PathsFile = Join-Path $PSScriptRoot "paths.ps1"

if (!(Test-Path $PathsFile)) {
    $ExampleFile = Join-Path $PSScriptRoot "paths.ps1.example"
    Copy-Item $ExampleFile $PathsFile
    Write-Host "Created tools/paths.ps1 from example - please fill in your game paths, then re-run."
    Start-Process $PathsFile
    exit 0
}
. $PathsFile

if ($Steam) {
    $InstallDir = $SteamPaths[$Game]
}
else {
    $InstallDir = $GamePaths[$Game]
}

if (!$InstallDir) {
    Write-Error "No path configured for game $Game. Check tools/paths.ps1."
    exit 1
}

$Exe = $GameExes[$Game]
$AppId = $SteamAppIds[$Game]
$RpfmGame = $RpfmGames[$Game]

$BuildDir = Join-Path $Root ("build/" + $Game)
$ModDir = Join-Path $Root "src/pack"
$TestSrcDir = Join-Path $Root ("tests/" + $Game + "/pack")
$ModPack = Join-Path $BuildDir "twdll.pack"
$TestPack = Join-Path $BuildDir ("twdll_test_" + $Game + ".pack")
$DataDir = Join-Path $InstallDir "data"
$ScriptBase = Join-Path $env:APPDATA "The Creative Assembly"
$ScriptDir = Join-Path (Join-Path $ScriptBase $Game) "scripts"
$SaveDir = Join-Path (Join-Path $ScriptBase $Game) "save_games"
$LogFile = Join-Path $InstallDir "twdll.log"

if (!$DllPath) {
    $DllPath = Join-Path $BuildDir "twdll.dll"
}

function Build-Pack($p, $src) {
    Write-Host "Building $p from $src"
    # Ensure source dir exists
    if (!(Test-Path $src)) { New-Item -ItemType Directory -Force -Path $src | Out-Null }

    # Copy shared test scripts if this is a test pack
    if ($src -match "tests") {
        $sharedDir = Join-Path $Root "tests/shared"
        $targetShared = Join-Path $src "shared"
        if (!(Test-Path $targetShared)) { New-Item -ItemType Directory -Force -Path $targetShared | Out-Null }
        Copy-Item -Force (Join-Path $sharedDir "*.lua") $targetShared
    }

    if (!$RpfmGame) { Write-Error "RpfmGame not defined for $Game"; exit 1 }

    rpfm_cli --game $RpfmGame pack create --pack-path "$p"
    if ($LASTEXITCODE -ne 0) { Write-Error "RPFM create failed"; exit 1 }
    rpfm_cli --game $RpfmGame pack add    --pack-path "$p" -F "$src;"
    if ($LASTEXITCODE -ne 0) { Write-Error "RPFM add failed"; exit 1 }
}

function Install-Base {
    Write-Host "Installing DLL: $DllPath"
    if (!(Test-Path $DllPath)) { Write-Error "No DLL found at $DllPath"; exit 1 }
    if (!(Test-Path $ModPack)) { Build-Pack $ModPack $ModDir }

    if (!(Test-Path $DataDir)) { New-Item -ItemType Directory -Force -Path $DataDir | Out-Null }
    if (!(Test-Path $ScriptDir)) { New-Item -ItemType Directory -Force -Path $ScriptDir | Out-Null }

    Copy-Item -Force $DllPath $InstallDir
    Copy-Item -Force $ModPack $DataDir

    $content = "mod `"twdll.pack`";"
    Set-Content -Path (Join-Path $ScriptDir "user.script.txt") -Value $content
    Write-Host "Installed"
}

function Install-Test {
    Install-Base
    # Build per-game test pack
    Build-Pack $TestPack $TestSrcDir

    if (!(Test-Path $SaveDir)) { New-Item -ItemType Directory -Force -Path $SaveDir | Out-Null }

    $TestPackName = Split-Path $TestPack -Leaf
    Copy-Item -Force $TestPack "$DataDir\"

    $GameSave = Join-Path $Root ("tests/" + $Game + "/tests.save")
    if (Test-Path $GameSave) { 
        Copy-Item -Force $GameSave (Join-Path $SaveDir "tests.save") 
    }

    $ConsulPack = if ($Game -eq "rome2") { "consulscriptum.pack" } else { "consulscriptum_attila.pack" }
    $s = "mod `"$TestPackName`";`nmod `"twdll.pack`";`nmod `"$ConsulPack`";`ngame_startup_mode campaign_load `"tests.save`";"

    Set-Content -Path (Join-Path $ScriptDir "user.script.txt") -Value $s
    Write-Host "Test env installed for $Game"
}

function Fix-CpuAffinity($procName) {
    # Windows 11 freeze workaround: exclude CPU0 from the game's affinity mask.
    $startTime = Get-Date
    while ($true) {
        $proc = Get-Process -Name $procName -ErrorAction SilentlyContinue
        if ($proc) {
            try {
                # Build mask with all CPUs except CPU0 (bit 0)
                $sys = [System.Environment]::ProcessorCount
                $fullMask = [int64]([math]::Pow(2, $sys) - 1)
                $newMask  = $fullMask -band (-bnot 1)
                $proc.ProcessorAffinity = [System.IntPtr]$newMask
                Write-Host "CPU affinity fixed: CPU0 excluded (mask 0x$($newMask.ToString('X')))"
            } catch {
                Write-Host "Warning: could not set CPU affinity: $_"
            }
            return
        }
        if (((Get-Date) - $startTime).TotalSeconds -gt 30) {
            Write-Host "Warning: timeout waiting for $procName to set affinity"
            return
        }
        Start-Sleep -Milliseconds 200
    }
}

function Launch-Game {
    if (Test-Path $LogFile) { 
        Remove-Item $LogFile -ErrorAction SilentlyContinue 
    }
    $reg = "HKCU:\SOFTWARE\The Creative Assembly\" + $Game
    if (!(Test-Path $reg)) { New-Item -Path $reg -Force | Out-Null }
    Set-ItemProperty -Path $reg -Name "last_game_version" -Value 131076 -Type DWord

    if ($Steam) {
        if (!$AppId) { Write-Error "AppId not defined for $Game"; exit 1 }
        $uri = "steam://rungameid/" + $AppId
        Start-Process $uri
        Write-Host "Launched via Steam"
    }
    else {
        if (!$Exe) { Write-Error "Exe not defined for $Game"; exit 1 }
        Start-Process -FilePath $Exe -WorkingDirectory $InstallDir -WindowStyle Hidden
        Write-Host "Launched"
    }

    $ProcName = if ($Game -eq "rome2") { "Rome2" } else { "Attila" }
    Fix-CpuAffinity $ProcName
}

function Tail-Log {
    Write-Host "Tailing $LogFile"
    $lastCount = 0
    $ProcName = if ($Game -eq "rome2") { "Rome2" } else { "Attila" }
    
    # Wait for process to start (especially for Steam)
    $startTime = Get-Date
    while (!(Get-Process -Name $ProcName -ErrorAction SilentlyContinue)) {
        if (((Get-Date) - $startTime).TotalSeconds -gt 30) {
            Write-Error "Timeout waiting for game process ($ProcName) to start"
            exit 1
        }
        Start-Sleep -Milliseconds 500
    }

    while ($true) {
        if (Test-Path $LogFile) {
            $lines = @(Get-Content -Path $LogFile -ErrorAction SilentlyContinue)
            if ($lines.Count -gt $lastCount) {
                for ($i = $lastCount; $i -lt $lines.Count; $i++) {
                    $line = $lines[$i]
                    Write-Host $line
                    if ($line -match '\[TEST\] Final Result: SUCCESS') {
                        Write-Host "PASSED"
                        Stop-Process -Name $ProcName -Force -ErrorAction SilentlyContinue
                        exit 0
                    }
                    if ($line -match '\[TEST\] Final Result: FAILED') {
                        Write-Host "FAILED"
                        exit 1
                    }
                }
                $lastCount = $lines.Count
            }
        }

        if (!(Get-Process -Name $ProcName -ErrorAction SilentlyContinue)) {
            # Check for log one last time before exiting
            if (Test-Path $LogFile) {
                $lines = @(Get-Content -Path $LogFile -ErrorAction SilentlyContinue)
                if ($lines.Count -gt $lastCount) {
                    for ($i = $lastCount; $i -lt $lines.Count; $i++) {
                        Write-Host $lines[$i]
                    }
                }
            }
            Write-Error "Game process exited"
            exit 1
        }
        Start-Sleep -Milliseconds 500
    }
}

$c = $Command.ToLower().Trim()
switch ($c) {
    "pack"         { Build-Pack $ModPack $ModDir; Build-Pack $TestPack $TestSrcDir }
    "install"      { Install-Base }
    "install-test" { Install-Test }
    "run"          { Install-Base; Launch-Game }
    "run-test"     { Install-Test; Launch-Game }
    "tail"         { Tail-Log }
    "test"         { Install-Test; Launch-Game; Tail-Log }
    "help" {
        Write-Host "Usage: .\tools\twdll.ps1 <command> <game> [-Steam]"
        Write-Host "Commands: pack, install, install-test, run, run-test, tail, test"
    }
    Default {
        Write-Error "Unknown command: $Command"
        Write-Host "Usage: .\tools\twdll.ps1 <command> <game> [-Steam]"
        exit 1
    }
}
