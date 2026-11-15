[CmdletBinding()]
param(
    [ValidateSet('mini', 'attila_32', 'full')]
    [string]$Mode = 'attila_32',
    [switch]$Full,
    [switch]$Wait,
    [switch]$InternalWorker,
    [string]$RunDir
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($Full) {
    $Mode = 'full'
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$toolsDir = if ($env:TWDLL_TOOLS_DIR -and $env:TWDLL_TOOLS_DIR.Trim() -ne '') {
    [System.IO.Path]::GetFullPath($env:TWDLL_TOOLS_DIR)
} else {
    Join-Path $repoRoot '.twdll\tools'
}
$liveBinsDir = if ($env:TWDLL_E2E_LIVE_BINS_DIR -and $env:TWDLL_E2E_LIVE_BINS_DIR.Trim() -ne '') {
    [System.IO.Path]::GetFullPath($env:TWDLL_E2E_LIVE_BINS_DIR)
} else {
    Join-Path $repoRoot '.twdll\binaries'
}

$baseTemp = if ($env:LOCALAPPDATA -and $env:LOCALAPPDATA.Trim() -ne '') {
    Join-Path $env:LOCALAPPDATA 'Temp\twdll'
} else {
    Join-Path ([System.IO.Path]::GetTempPath()) 'twdll'
}
$runsRoot = Join-Path $baseTemp 'twdll-e2e-runs'
$lockPath = Join-Path $baseTemp 'twdll-e2e.lock'
$latestPath = Join-Path $runsRoot 'latest.txt'

function Get-TargetsForMode([string]$m) {
    switch ($m) {
        'mini'      { return @('attila_32', 'attila_64') }
        'attila_32' { return @('attila_32') }
        'full'      { return @('attila_32', 'attila_64') }
        default     { throw "Unknown E2E mode: $m" }
    }
}

function Get-GitShortSha([string]$root) {
    try {
        $sha = (& git -C $root rev-parse --short HEAD 2>$null)
        if ($LASTEXITCODE -eq 0 -and $sha) {
            return $sha.Trim()
        }
    } catch {}
    return 'nogit'
}

function Write-StatusJson([string]$path, [System.Collections.IDictionary]$payload) {
    $json = $payload | ConvertTo-Json -Depth 5
    $tmp = "$path.tmp.$PID"
    [System.IO.File]::WriteAllText($tmp, $json, [System.Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $tmp -Destination $path -Force
}

function Invoke-E2EWorker([string]$WorkerRunDir, [string]$WorkerMode) {
    $statusPath = Join-Path $WorkerRunDir 'status.json'
    $logFile = Join-Path $WorkerRunDir 'run.log'
    $binDir = Join-Path $WorkerRunDir 'bin'
    $sandboxDir = Join-Path $WorkerRunDir 'sandbox'
    $cliExe = Join-Path $binDir 'twdll.exe'
    $testExe = Join-Path $binDir 'e2e.test.exe'
    $runId = Split-Path -Leaf $WorkerRunDir
    $gitSha = ($runId -split '-')[-1]
    $targets = Get-TargetsForMode $WorkerMode

    $lockStream = $null
    $startedAt = (Get-Date).ToUniversalTime()
    try {
        $lockStream = [System.IO.File]::Open(
            $lockPath,
            [System.IO.FileMode]::OpenOrCreate,
            [System.IO.FileAccess]::ReadWrite,
            [System.IO.FileShare]::None
        )

        try {
            (Get-Process -Id $PID).PriorityClass = [System.Diagnostics.ProcessPriorityClass]::BelowNormal
        } catch {}

        Write-StatusJson -path $statusPath -payload ([ordered]@{
            run_id           = $runId
            mode             = $WorkerMode
            targets          = $targets
            git_sha          = $gitSha
            pid              = $PID
            state            = 'running'
            exit_code        = $null
            started_at       = $startedAt.ToString('o')
            finished_at      = $null
            duration_sec     = 0.0
            run_dir          = $WorkerRunDir
            log_file         = $logFile
            sandbox_retained = $true
        })

        $env:TWDLL_E2E_CLI_BIN = $cliExe
        $env:TWDLL_E2E_SANDBOX_DIR = $sandboxDir
        $env:TWDLL_TOOLS_DIR = $toolsDir
        $env:TWDLL_E2E_LIVE_BINS_DIR = $liveBinsDir
        $env:TWDLL_NO_REBUILD = '1'
        Remove-Item Env:TWDLL_ROOT -ErrorAction SilentlyContinue
        Remove-Item Env:TWDLL_WRAPPER_ROOT -ErrorAction SilentlyContinue
        Remove-Item Env:TWDLL_CONFIG -ErrorAction SilentlyContinue
        Remove-Item Env:TWDLL_DATA_DIR -ErrorAction SilentlyContinue

        switch ($WorkerMode) {
            'mini' {
                $env:TWDLL_E2E_SOAK = '0'
                $env:TWDLL_E2E_TARGETS = ''
                $testRunRegex = '^TestE2E_MiniWarscape_RealDecompilers$'
                $testTimeout = '10m'
            }
            'attila_32' {
                $env:TWDLL_E2E_SOAK = '1'
                $env:TWDLL_E2E_TARGETS = 'attila_32'
                $testRunRegex = '^TestE2E_RealBinaries_Soak$'
                $testTimeout = '60m'
            }
            'full' {
                $env:TWDLL_E2E_SOAK = '1'
                $env:TWDLL_E2E_TARGETS = 'attila_32,attila_64'
                $testRunRegex = '^TestE2E_RealBinaries_Soak$'
                $testTimeout = '180m'
            }
        }

        $cmdLine = '""{0}" -test.v -test.run "{1}" -test.timeout {2} > "{3}" 2>&1"' -f $testExe, $testRunRegex, $testTimeout, $logFile
        Push-Location (Join-Path $repoRoot 'cli')
        try {
            & cmd.exe /d /s /c $cmdLine
            $exitCode = $LASTEXITCODE
        } finally {
            Pop-Location
        }

        $sandboxResLog = Join-Path $sandboxDir 'data\logs\analyze_resources.jsonl'
        $archivedResLog = Join-Path $WorkerRunDir 'analyze_resources.jsonl'
        if (Test-Path -LiteralPath $sandboxResLog) {
            Copy-Item -LiteralPath $sandboxResLog -Destination $archivedResLog -Force
        }

        $sandboxRetained = $true
        if ($exitCode -eq 0) {
            if (Test-Path -LiteralPath $sandboxDir) {
                Remove-Item -LiteralPath $sandboxDir -Recurse -Force -ErrorAction SilentlyContinue
            }
            $sandboxRetained = [bool](Test-Path -LiteralPath $sandboxDir)
        }

        $finishedAt = (Get-Date).ToUniversalTime()
        $durationSec = [math]::Round(($finishedAt - $startedAt).TotalSeconds, 2)
        $finalState = if ($exitCode -eq 0) { 'passed' } else { 'failed' }

        Write-StatusJson -path $statusPath -payload ([ordered]@{
            run_id           = $runId
            mode             = $WorkerMode
            targets          = $targets
            git_sha          = $gitSha
            pid              = $PID
            state            = $finalState
            exit_code        = $exitCode
            started_at       = $startedAt.ToString('o')
            finished_at      = $finishedAt.ToString('o')
            duration_sec     = $durationSec
            run_dir          = $WorkerRunDir
            log_file         = $logFile
            sandbox_retained = $sandboxRetained
        })

        return $exitCode
    } catch {
        $errText = ($_ | Out-String)
        [System.IO.File]::AppendAllText($logFile, "`n[worker exception]`n$errText`n")
        $finishedAt = (Get-Date).ToUniversalTime()
        $durationSec = [math]::Round(($finishedAt - $startedAt).TotalSeconds, 2)
        Write-StatusJson -path $statusPath -payload ([ordered]@{
            run_id           = $runId
            mode             = $WorkerMode
            targets          = $targets
            git_sha          = $gitSha
            pid              = $PID
            state            = 'failed'
            exit_code        = 1
            started_at       = $startedAt.ToString('o')
            finished_at      = $finishedAt.ToString('o')
            duration_sec     = $durationSec
            run_dir          = $WorkerRunDir
            log_file         = $logFile
            sandbox_retained = [bool](Test-Path -LiteralPath $sandboxDir)
        })
        return 1
    } finally {
        if ($null -ne $lockStream) {
            $lockStream.Dispose()
        }
    }
}

if ($InternalWorker) {
    if ([string]::IsNullOrWhiteSpace($RunDir)) {
        throw "-InternalWorker requires -RunDir"
    }
    $code = Invoke-E2EWorker -WorkerRunDir $RunDir -WorkerMode $Mode
    exit $code
}

# Foreground orchestration phase
New-Item -ItemType Directory -Path $runsRoot -Force | Out-Null

# 1. Probe exclusive lockfile before compiling or spawning
try {
    $probeStream = [System.IO.File]::Open(
        $lockPath,
        [System.IO.FileMode]::OpenOrCreate,
        [System.IO.FileAccess]::ReadWrite,
        [System.IO.FileShare]::None
    )
    $probeStream.Dispose()
} catch {
    throw "Another twdll E2E run is already active (lockfile held: $lockPath). Check $latestPath for the active run directory."
}

$gitSha = Get-GitShortSha $repoRoot
$timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$runId = "$timestamp-$gitSha"
$resolvedRunDir = Join-Path $runsRoot $runId
$binDir = Join-Path $resolvedRunDir 'bin'
$sandboxDir = Join-Path $resolvedRunDir 'sandbox'

New-Item -ItemType Directory -Path $binDir -Force | Out-Null
New-Item -ItemType Directory -Path $sandboxDir -Force | Out-Null

$goExe = Join-Path $toolsDir 'go\bin\go.exe'
if (-not (Test-Path -LiteralPath $goExe)) {
    throw "Portable Go compiler not found at $goExe"
}

$cliExe = Join-Path $binDir 'twdll.exe'
$testExe = Join-Path $binDir 'e2e.test.exe'

Push-Location (Join-Path $repoRoot 'cli')
try {
    & $goExe build -buildvcs=false -o $cliExe .
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to snapshot-compile twdll.exe (exit $LASTEXITCODE)"
    }
    & $goExe test -tags=e2e -c -o $testExe ./internal/e2e
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to snapshot-compile e2e.test.exe (exit $LASTEXITCODE)"
    }
} finally {
    Pop-Location
}

$statusPath = Join-Path $resolvedRunDir 'status.json'
$logFile = Join-Path $resolvedRunDir 'run.log'
$targets = Get-TargetsForMode $Mode
$nowIso = (Get-Date).ToUniversalTime().ToString('o')

Write-StatusJson -path $statusPath -payload ([ordered]@{
    run_id           = $runId
    mode             = $Mode
    targets          = $targets
    git_sha          = $gitSha
    pid              = $null
    state            = 'starting'
    exit_code        = $null
    started_at       = $nowIso
    finished_at      = $null
    duration_sec     = 0.0
    run_dir          = $resolvedRunDir
    log_file         = $logFile
    sandbox_retained = $true
})

[System.IO.File]::WriteAllText($latestPath, $resolvedRunDir, [System.Text.UTF8Encoding]::new($false))

if ($Wait) {
    $code = Invoke-E2EWorker -WorkerRunDir $resolvedRunDir -WorkerMode $Mode
    Write-Output (Get-Content -LiteralPath $statusPath -Raw)
    if ($code -ne 0) {
        exit $code
    }
    exit 0
}

$hostExe = (Get-Process -Id $PID).Path
$workerProc = Start-Process -FilePath $hostExe -ArgumentList @(
    '-NoProfile',
    '-ExecutionPolicy', 'Bypass',
    '-WindowStyle', 'Hidden',
    '-File', $PSCommandPath,
    '-InternalWorker',
    '-RunDir', $resolvedRunDir,
    '-Mode', $Mode
) -WorkingDirectory $repoRoot -WindowStyle Hidden -PassThru

Write-Output ([ordered]@{
    run_id      = $runId
    mode        = $Mode
    targets     = $targets
    worker_pid  = $workerProc.Id
    run_dir     = $resolvedRunDir
    status_file = $statusPath
    log_file    = $logFile
    latest_file = $latestPath
} | ConvertTo-Json -Depth 4)
