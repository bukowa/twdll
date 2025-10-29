# scripts/kill_game.ps1
# This script terminates existing Rome2.exe processes, waiting for completion.

# 1. Check if the script is already running with Admin privileges.
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    # 2. If not admin, re-launch this same script with elevated rights and WAIT for it to finish.
    Write-Host "Requesting Administrator privileges to terminate Rome2.exe..."
    $scriptPath = $MyInvocation.MyCommand.Path
    
    # THE FIX IS HERE: Added the -Wait parameter
    Start-Process powershell.exe -Verb RunAs -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$scriptPath`"" -Wait
    
    # Exit the current, non-admin script *after* the admin one has finished.
    exit
}

# 3. This part of the script only runs if we have admin rights.
$processName = 'Rome2'
Write-Host "[Admin] Attempting to terminate all instances of $processName.exe..."

$processes = Get-Process -Name $processName -ErrorAction SilentlyContinue
if ($null -eq $processes) {
    Write-Host "[Admin] No $processName.exe processes found running."
} else {
    $processes | ForEach-Object {
        Write-Host "[Admin] Stopping process with ID $($_.Id) and name $($_.ProcessName)..."
        Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue
    }

    $timeoutSeconds = 10
    $checkIntervalSeconds = 1
    $elapsedTime = 0

    Write-Host "[Admin] Waiting for $processName.exe process(es) to fully exit (timeout: ${timeoutSeconds}s)..."

    while ($elapsedTime -lt $timeoutSeconds) {
        if (-not (Get-Process -Name $processName -ErrorAction SilentlyContinue)) {
            Write-Host "[Admin] All $processName.exe processes confirmed terminated."
            # Set a success exit code for advanced build systems
            exit 0
        }
        Start-Sleep -Seconds $checkIntervalSeconds
        $elapsedTime += $checkIntervalSeconds
    }

    if ($elapsedTime -ge $timeoutSeconds) {
        Write-Warning "[Admin] $processName.exe process(es) did not terminate within ${timeoutSeconds} seconds. It might still be running."
        # Set a failure exit code
        exit 1
    }
}