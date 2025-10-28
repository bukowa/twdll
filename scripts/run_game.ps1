# scripts/run_game.ps1
# This script disables the mod popup and then launches the game.

# Define parameters that will be passed from the CMake command.
param(
    [Parameter(Mandatory=$true)]
    [string]$GameLaunchDir
)

# Validate that the paths exist before trying to use them.
if (-not (Test-Path -Path $GameLaunchDir)) {
    Write-Error "Game launch directory does not exist: $GameLaunchDir"
    exit 1
}

Write-Host "Disabling outdated mods popup..."
Set-ItemProperty -Path "HKCU:\SOFTWARE\The Creative Assembly\Rome2" -Name "last_game_version" -Value 0x00020004 -Force

Write-Host "Launching Rome2.exe from directory: $GameLaunchDir..."
Start-Process -FilePath 'Rome2.exe' -WorkingDirectory $GameLaunchDir