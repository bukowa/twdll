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
    Copy-Item -Force $DllPath (Join-Path $InstallDir ("twdll_" + $Game + ".dll"))
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

    $MarkerFile = Join-Path $InstallDir "twdll_reload_marker.flag"
    if (Test-Path $MarkerFile) {
        Remove-Item -Force $MarkerFile
        Write-Host "Cleaned stale reload marker: $MarkerFile"
    }

    $NoSaveFlag = Join-Path $InstallDir "twdll_no_save_reload.flag"
    if (Test-Path $NoSaveFlag) {
        Remove-Item -Force $NoSaveFlag
        Write-Host "Cleaned no-save flag: $NoSaveFlag"
    }

    $TestPackName = Split-Path $TestPack -Leaf
    Copy-Item -Force $TestPack "$DataDir\"

    # Deploy optional extra pack for per-game testing content
    $ExtraPack = Join-Path $Root ("tests/" + $Game + "/extra.pack")
    $ExtraModLine = ""
    if (Test-Path $ExtraPack) {
        Copy-Item -Force $ExtraPack "$DataDir\"
        $ExtraModLine = "`nmod `"extra.pack`";"
    }

    $GameSave = Join-Path $Root ("tests/" + $Game + "/tests.save")
    if (Test-Path $GameSave) { 
        Copy-Item -Force $GameSave (Join-Path $SaveDir "tests.save") 
    }

    $ConsulPack = if ($Game -eq "rome2") { "consulscriptum.pack" } else { "consulscriptum_attila.pack" }
    $s = "mod `"$TestPackName`";`nmod `"twdll.pack`";`nmod `"$ConsulPack`";$ExtraModLine`ngame_startup_mode campaign_load `"tests.save`";"

    Set-Content -Path (Join-Path $ScriptDir "user.script.txt") -Value $s
    Write-Host "Test env installed for $Game"
}

function Install-Test-NoSaveReload {
    Install-Test
    $NoSaveFlag = Join-Path $InstallDir "twdll_no_save_reload.flag"
    New-Item -ItemType File -Force -Path $NoSaveFlag | Out-Null
    Write-Host "Created no-save-reload flag: $NoSaveFlag"
}

function Install-Tdd {
    Install-Base
    # Build per-game test pack
    Build-Pack $TestPack $TestSrcDir

    $MarkerFile = Join-Path $InstallDir "twdll_reload_marker.flag"
    if (Test-Path $MarkerFile) {
        Remove-Item -Force $MarkerFile
        Write-Host "Cleaned stale reload marker: $MarkerFile"
    }

    $NoSaveFlag = Join-Path $InstallDir "twdll_no_save_reload.flag"
    New-Item -ItemType File -Force -Path $NoSaveFlag | Out-Null
    Write-Host "Created no-save-reload flag: $NoSaveFlag"

    $TestPackName = Split-Path $TestPack -Leaf
    Copy-Item -Force $TestPack "$DataDir\"

    $s = @"
mod "twdll_test_valerius.pack";
mod "startpos_war_of_the_ring.pack";
mod "tdd_pack1_main_1.1.0.9.pack";
mod "tdd_pack2_battles_1.1.0.9.pack";
mod "tdd_pack3_campaign_1.0.0.pack";
mod "tdd_pack4_models_1.1.0_rev.9.pack";
mod "tdd_pack5_buildings_1.1.0_rev.9.pack";
mod "tdd_pack6_weather_1.0.0.pack";
"@
    Set-Content -Path (Join-Path $ScriptDir "user.script.txt") -Value $s
    Write-Host "TDD campaign env installed for $Game"
}

function Fix-CpuAffinity($procName) {
    # Windows 11 freeze workaround: exclude CPU0 from the game's affinity mask.
    $startTime = Get-Date
    while ($true) {
        $procs = @(Get-Process -Name $procName -ErrorAction SilentlyContinue)
        if ($procs.Count -gt 0) {
            try {
                # Build mask with all CPUs except CPU0 (bit 0)
                $sys = [System.Environment]::ProcessorCount
                $fullMask = [int64]([math]::Pow(2, $sys) - 1)
                $newMask  = $fullMask -band (-bnot 1)
                foreach ($proc in $procs) {
                    $proc.ProcessorAffinity = [System.IntPtr]$newMask
                }
                Write-Host "CPU affinity fixed for $($procs.Count) process(es): CPU0 excluded (mask 0x$($newMask.ToString('X')))"
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
        if (((Get-Date) - $startTime).TotalSeconds -gt 45) {
            Write-Error "Timeout waiting for game process ($ProcName) to start"
            exit 1
        }
        Start-Sleep -Milliseconds 500
    }

    # Start tailing from the end so previous runs are not re-printed
    if (Test-Path $LogFile) {
        $lastCount = @(Get-Content -Path $LogFile -ErrorAction SilentlyContinue).Count
    }

    $lastActivity = Get-Date
    while ($true) {
        if (Test-Path $LogFile) {
            $lines = @(Get-Content -Path $LogFile -ErrorAction SilentlyContinue)
            if ($lines.Count -lt $lastCount) { $lastCount = 0 }
            if ($lines.Count -gt $lastCount) {
                $lastActivity = Get-Date
                for ($i = $lastCount; $i -lt $lines.Count; $i++) {
                    $line = $lines[$i]
                    Write-Host $line
                    if ($line -match '\[TEST\] Final Result: SUCCESS' -or $line -match 'Reload verification completed successfully') {
                        Write-Host "=== TEST PASSED ==="
                        Stop-Process -Name $ProcName -Force -ErrorAction SilentlyContinue
                        exit 0
                    }
                    if ($line -match '\[TEST\] Final Result: FAILED' -or $line -match '\[TEST\] run_twdll_tests error:' -or $line -match '\[LUA\] FATAL:') {
                        Write-Host "=== TEST FAILED ==="
                        Stop-Process -Name $ProcName -Force -ErrorAction SilentlyContinue
                        exit 1
                    }
                }
                $lastCount = $lines.Count
            }
        }

        if (((Get-Date) - $lastActivity).TotalSeconds -gt 90) {
            Write-Host "Warning: 90s inactivity timeout waiting for test results"
            Stop-Process -Name $ProcName -Force -ErrorAction SilentlyContinue
            exit 1
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

# Like Tail-Log but never kills the game process - keeps streaming until the
# game closes on its own. Useful for interactive post-test inspection.
function Tail-Log-Keep {
    Write-Host "Tailing $LogFile (keep-alive mode - game will NOT be killed after tests)"
    $lastCount = 0
    $ProcName = if ($Game -eq "rome2") { "Rome2" } else { "Attila" }

    # Wait for process to start
    $startTime = Get-Date
    while (!(Get-Process -Name $ProcName -ErrorAction SilentlyContinue)) {
        if (((Get-Date) - $startTime).TotalSeconds -gt 30) {
            Write-Error "Timeout waiting for game process ($ProcName) to start"
            exit 1
        }
        Start-Sleep -Milliseconds 500
    }

    if (Test-Path $LogFile) {
        $lastCount = @(Get-Content -Path $LogFile -ErrorAction SilentlyContinue).Count
    }

    while ($true) {
        if (Test-Path $LogFile) {
            $lines = @(Get-Content -Path $LogFile -ErrorAction SilentlyContinue)
            if ($lines.Count -lt $lastCount) { $lastCount = 0 }
            if ($lines.Count -gt $lastCount) {
                for ($i = $lastCount; $i -lt $lines.Count; $i++) {
                    $line = $lines[$i]
                    Write-Host $line
                    # Report result but do NOT kill the process
                    if ($line -match '\[TEST\] Final Result: SUCCESS') {
                        Write-Host "PASSED (game kept alive)"
                    }
                    if ($line -match '\[TEST\] Final Result: FAILED') {
                        Write-Host "FAILED (game kept alive)"
                    }
                }
                $lastCount = $lines.Count
            }
        }

        if (!(Get-Process -Name $ProcName -ErrorAction SilentlyContinue)) {
            # Flush remaining log lines
            if (Test-Path $LogFile) {
                $lines = @(Get-Content -Path $LogFile -ErrorAction SilentlyContinue)
                if ($lines.Count -gt $lastCount) {
                    for ($i = $lastCount; $i -lt $lines.Count; $i++) {
                        Write-Host $lines[$i]
                    }
                }
            }
            Write-Host "Game process exited"
            exit 0
        }
        Start-Sleep -Milliseconds 500
    }
}

function Install-Mp {
    Write-Host "Installing Multiplayer environment for $Game..."
    if (!$MpGamePaths -or $MpGamePaths.Count -eq 0) {
        Write-Error "No MpGamePaths configured. Check tools/paths.ps1."
        exit 1
    }
    if (!(Test-Path $DllPath)) { Write-Error "No DLL found at $DllPath"; exit 1 }
    if (!(Test-Path $ModPack)) { Build-Pack $ModPack $ModDir }
    Build-Pack $TestPack $TestSrcDir

    $TestPackName = Split-Path $TestPack -Leaf
    $userScriptContent = "mod `"$TestPackName`";`nmod `"twdll.pack`";`n"

    for ($i = 0; $i -lt $MpGamePaths.Count; $i++) {
        $gPath = $MpGamePaths[$i]
        $data = Join-Path $gPath "data"
        if (!(Test-Path $data)) { New-Item -ItemType Directory -Force -Path $data | Out-Null }

        Write-Host "Deploying to Game Instance $($i+1): $gPath"
        try {
            Copy-Item -Force $DllPath $gPath -ErrorAction Stop
            Copy-Item -Force $DllPath (Join-Path $gPath ("twdll_" + $Game + ".dll")) -ErrorAction Stop
        } catch {
            Write-Warning "Could not overwrite DLL (game process is likely running): $_"
        }
        try {
            Copy-Item -Force $ModPack $data -ErrorAction Stop
            Copy-Item -Force $TestPack $data -ErrorAction Stop
        } catch {
            Write-Warning "Could not overwrite pack files: $_"
        }

        Remove-Item -Force (Join-Path $gPath "twdll_reload_marker.flag") -ErrorAction SilentlyContinue
        Remove-Item -Force (Join-Path $gPath "twdll_no_save_reload.flag") -ErrorAction SilentlyContinue

    }

    if ($MpAppDataPaths) {
        for ($i = 0; $i -lt $MpAppDataPaths.Count; $i++) {
            $appData = $MpAppDataPaths[$i]
            $scriptDir = Join-Path $appData "scripts"
            if (!(Test-Path $scriptDir)) { New-Item -ItemType Directory -Force -Path $scriptDir | Out-Null }

            Write-Host "Setting user.script.txt for Instance $($i+1): $scriptDir"
            Set-Content -Path (Join-Path $scriptDir "user.script.txt") -Value $userScriptContent
        }
    }

    Write-Host "Multiplayer environment successfully installed to all instances."
}

function Launch-Mp {
    Install-Mp
    Write-Host "Launching MP instances..."
    foreach ($gPath in $MpGamePaths) {
        $exePath = Join-Path $gPath $Exe
        if (Test-Path $exePath) {
            Start-Process -FilePath $exePath -WorkingDirectory $gPath
            Write-Host "Launched: $exePath"
        } else {
            Write-Warning "Could not find $exePath"
        }
    }
    $ProcName = if ($Game -eq "rome2") { "Rome2" } else { "Attila" }
    Fix-CpuAffinity $ProcName
}

function Tail-Mp-Log {
    Write-Host "Tailing MP logs from $($MpGamePaths.Count) instances (Ctrl+C to stop)..."
    $ProcName = if ($Game -eq "rome2") { "Rome2" } else { "Attila" }

    $startTime = Get-Date
    while (!@(Get-Process -Name $ProcName -ErrorAction SilentlyContinue).Count) {
        if (((Get-Date) - $startTime).TotalSeconds -gt 30) {
            Write-Error "Timeout waiting for game processes ($ProcName) to start"
            exit 1
        }
        Start-Sleep -Milliseconds 500
    }

    $logFiles = @()
    $lastCounts = @()
    for ($i = 0; $i -lt $MpGamePaths.Count; $i++) {
        $gName = Split-Path $MpGamePaths[$i] -Leaf
        $sandboxName = if ($i -eq 0) { "TDD" } else { "TDD_Copy" }
        $sboxLog = Join-Path $env:SystemDrive "Sandbox\$env:USERNAME\$sandboxName\user\current\Documents\TDD\$gName\twdll.log"
        $directLog = Join-Path $MpGamePaths[$i] "twdll.log"

        $targetLog = if (Test-Path $sboxLog) { $sboxLog } else { $directLog }
        $logFiles += $targetLog
        $initCount = 0
        if (Test-Path $targetLog) {
            $initCount = @(Get-Content -Path $targetLog -ErrorAction SilentlyContinue).Count
        }
        $lastCounts += $initCount
        Write-Host "Monitoring log for Instance $($i+1): $targetLog (initial lines: $initCount)"
    }

    $colors = @("Cyan", "Yellow", "Green", "Magenta")

    while ($true) {
        for ($i = 0; $i -lt $logFiles.Count; $i++) {
            $lf = $logFiles[$i]
            if (!(Test-Path $lf)) {
                $gName = Split-Path $MpGamePaths[$i] -Leaf
                $sandboxName = if ($i -eq 0) { "TDD" } else { "TDD_Copy" }
                $sboxLog = Join-Path $env:SystemDrive "Sandbox\$env:USERNAME\$sandboxName\user\current\Documents\TDD\$gName\twdll.log"
                if (Test-Path $sboxLog) {
                    $lf = $sboxLog
                    $logFiles[$i] = $sboxLog
                }
            }

            $lastCount = $lastCounts[$i]
            if (Test-Path $lf) {
                $lines = @(Get-Content -Path $lf -ErrorAction SilentlyContinue)
                if ($lines.Count -lt $lastCount) { $lastCount = 0 }
                if ($lines.Count -gt $lastCount) {
                    $prefix = "[G$($i+1)]"
                    $color = $colors[$i % $colors.Count]
                    for ($j = $lastCount; $j -lt $lines.Count; $j++) {
                        Write-Host "$prefix $($lines[$j])" -ForegroundColor $color
                    }
                    $lastCounts[$i] = $lines.Count
                }
            }
        }

        if (!@(Get-Process -Name $ProcName -ErrorAction SilentlyContinue).Count) {
            Write-Host "All game processes exited"
            exit 0
        }
        Start-Sleep -Milliseconds 250
    }
}

$c = $Command.ToLower().Trim()
switch ($c) {
    "pack"         { Build-Pack $ModPack $ModDir; Build-Pack $TestPack $TestSrcDir }
    "install"      { Install-Base }
    "install-test" { Install-Test }
    "install-mp"   { Install-Mp }
    "run"          { Install-Base; Launch-Game }
    "run-test"     { Install-Test; Launch-Game }
    "run-mp"       { Launch-Mp; Tail-Mp-Log }
    "mp-run"       { Launch-Mp; Tail-Mp-Log }
    "tail"         { Tail-Log }
    "tail-mp"      { Tail-Mp-Log }
    "test"         { Install-Test; Launch-Game; Tail-Log }
    "test-keep"    { Install-Test; Launch-Game; Tail-Log-Keep }
    "test-keep-nosavereload" { Install-Test-NoSaveReload; Launch-Game; Tail-Log-Keep }
    "install-tdd"  { Install-Tdd }
    "tdd"          { Install-Tdd; Launch-Game; Tail-Log-Keep }
    "help" {
        Write-Host "Usage: .\tools\twdll.ps1 <command> <game> [-Steam]"
        Write-Host "Commands: pack, install, install-test, install-mp, run, run-test, run-mp, tail, tail-mp, test, test-keep, test-keep-nosavereload, install-tdd, tdd"
    }
    Default {
        Write-Error "Unknown command: $Command"
        Write-Host "Usage: .\tools\twdll.ps1 <command> <game> [-Steam]"
        exit 1
    }
}

