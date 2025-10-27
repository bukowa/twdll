# scripts/run_game.ps1
# This script disables the mod popup and then launches the game.

# Define parameters that will be passed from the CMake command.
param(
    [Parameter(Mandatory=$true)]
    [string]$PopupScriptPath,

    [Parameter(Mandatory=$true)]
    [string]$GameLaunchDir
)

# Validate that the paths exist before trying to use them.
if (-not (Test-Path -Path $PopupScriptPath)) {
    Write-Error "Popup script not found at the provided path: $PopupScriptPath"
    exit 1 # Exit with an error code to fail the build.
}
if (-not (Test-Path -Path $GameLaunchDir)) {
    Write-Error "Game launch directory does not exist: $GameLaunchDir"
    exit 1
}

Write-Host "Executing script to disable outdated mods popup..."
Start-Process -FilePath $PopupScriptPath

Write-Host "Launching Rome2.exe from directory: $GameLaunchDir"
Start-Process -FilePath 'Rome2.exe' -WorkingDirectory $GameLaunchDir