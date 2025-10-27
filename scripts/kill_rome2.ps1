# scripts/kill_rome2.ps1 - Self-elevating version

# 1. Check if the script is already running with Admin privileges.
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    # 2. If not admin, re-launch this same script with elevated rights.
    Write-Host "Requesting Administrator privileges to terminate Rome2.exe..."
    $scriptPath = $MyInvocation.MyCommand.Path
    Start-Process powershell.exe -Verb RunAs -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$scriptPath`""
    # Exit the current, non-admin script.
    exit
}

# 3. This part of the script only runs if we have admin rights.
# We can now remove -ErrorAction SilentlyContinue because we expect it to work.
Write-Host "[Admin] Attempting to terminate Rome2.exe..."
Stop-Process -Name 'Rome2' -Force -ErrorAction SilentlyContinue # Keep SilentlyContinue here in case the process isn't running
Write-Host "[Admin] Waiting for Rome2.exe process to fully exit..."
Wait-Process -Name 'Rome2' -ErrorAction SilentlyContinue # Keep this too
Write-Host "[Admin] Rome2.exe process confirmed terminated."