# Running Two Instances of *Total War: Attila* on One PC (LAN Multiplayer & twdll Testing)

This guide explains how to configure and run two isolated instances of *Total War: Attila* side-by-side on a single Windows machine using **Sandboxie-Plus** and the **Goldberg LAN Steam Emulator**, integrated with the **twdll** automated multiplayer test runner.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Host Windows PC                          │
├──────────────────────────────┬──────────────────────────────┤
│ Instance 1 (Host / Player 1) │ Instance 2 (Player 2)        │
│ • Directory: Total War Attila│ • Directory: Attila - Copy   │
│ • Sandboxie Box: TDD         │ • Sandboxie Box: TDD_Copy    │
│ • Steam ID: ...001           │ • Steam ID: ...002           │
│ • Log Tag: [G1] (Cyan)       │ • Log Tag: [G2] (Yellow)     │
└──────────────────────────────┴──────────────────────────────┘
```

---

## Step 1: Prepare Game Folders & Goldberg Emulator

1. **Duplicate the Game:** Create two separate copies of your *Total War: Attila* directory:
   * Instance 1: `%USERPROFILE%\Documents\TDD\Total War Attila`
   * Instance 2: `%USERPROFILE%\Documents\TDD\Total War Attila - Copy`
2. **Install Goldberg Emulator into BOTH folders:**
   * Copy the 32-bit **`steam_api.dll`** from the Goldberg emulator archive into both game folders (overwrite the original file).
   * In both folders, create a text file named **`steam_appid.txt`** containing `325610`.
   * In both folders, create an empty text file named **`local_save.txt`** directly next to `Attila.exe`.

---

## Step 2: Sandboxie-Plus Configuration

1. **Create Two Sandboxes in Sandboxie-Plus:**
   * Name Box 1: `TDD` (or `Attila1`)
   * Name Box 2: `TDD_Copy` (or `Attila2`)
   * **Box Type:** Select **Standard Isolation Box (Yellow Icon)** — do *not* use *Privacy Enhanced*.
2. **Unlock IPC & Sockets:**
   * Right-click Sandbox 1 $\rightarrow$ **Sandbox Options** $\rightarrow$ **Edit ini Section** (at the bottom).
   * Paste the following configuration lines at the end and click **Apply**:
     ```ini
     OpenIpcPath=\RPC Control\*
     OpenIpcPath=\BaseNamedObjects\*
     OpenWinClass=*
     DropAdminRights=n
     ```
   * Repeat this exact step for Sandbox 2.
3. **Assign Forced Folders:**
   * In Sandbox 1 Options $\rightarrow$ **Program Control** $\rightarrow$ **Forced Programs** $\rightarrow$ click **Force Folder** and select the **entire first game directory** (`Total War Attila`).
   * In Sandbox 2 Options $\rightarrow$ **Program Control** $\rightarrow$ **Forced Programs** $\rightarrow$ click **Force Folder** and select the **entire second game directory** (`Total War Attila - Copy`).

---

## Step 3: Generate & Configure Player Profiles

1. Launch `Attila.exe` from both folders once so the emulator generates the initial configuration files, then close both games.
2. When the Sandboxie **File Recovery** prompt appears, click **RECOVER** (to persist the generated `settings/` folder into your actual game directory).
3. **Edit the configuration files in `settings/`:**
   * **Folder 1 (`Total War Attila\settings\`):**
     * `account_name.txt` $\rightarrow$ `Player1`
     * `user_steam_id.txt` $\rightarrow$ `76561198000000001`
   * **Folder 2 (`Total War Attila - Copy\settings\`):**
     * `account_name.txt` $\rightarrow$ `Player2`
     * `user_steam_id.txt` $\rightarrow$ `76561198000000002`
   * Ensure `language.txt` is identical in both folders (`english` or `polish`).

---

## Step 4: Synchronize Engine & Graphics Settings (Windowed Mode)

1. Launch the first game, go to **Options $\rightarrow$ Graphics**, and enable **Windowed Mode** so both game windows can run side-by-side.
2. Open Sandbox 1 AppData:
   `C:\Sandbox\<User>\TDD\user\current\AppData\Roaming\The Creative Assembly\Attila\scripts\`
3. Copy **`preferences.script.txt`**.
4. Paste and overwrite it into Sandbox 2 AppData:
   `C:\Sandbox\<User>\TDD_Copy\user\current\AppData\Roaming\The Creative Assembly\Attila\scripts\`

---

## Step 5: twdll Automated Workflow & Live Log Tailing

The `twdll` repository provides built-in CMake targets and PowerShell commands to build, package, deploy, and monitor both instances:

### 1. Configure Paths in `tools/paths.ps1`
Ensure `$MpGamePaths` and `$MpAppDataPaths` match your sandbox setup:
```powershell
$MpGamePaths = @(
    (Join-Path $env:USERPROFILE "Documents\TDD\Total War Attila"),
    (Join-Path $env:USERPROFILE "Documents\TDD\Total War Attila - Copy")
)

$MpAppDataPaths = @(
    (Join-Path $env:SystemDrive "Sandbox\$env:USERNAME\TDD\user\current\AppData\Roaming\The Creative Assembly\Attila"),
    (Join-Path $env:SystemDrive "Sandbox\$env:USERNAME\TDD_Copy\user\current\AppData\Roaming\The Creative Assembly\Attila")
)
```

### 2. Automated Commands

* **Build & Deploy to Both Instances:**
  ```powershell
  cmake --build build/attila --target tw-mp-install
  ```
  *(Rebuilds `twdll.dll`, packages `twdll_test_attila.pack` with RPFM, copies all files to both game folders, and updates `user.script.txt` in both sandboxes).*

* **Deploy, Launch Both Instances & Stream Live Color-Coded Logs:**
  ```powershell
  cmake --build build/attila --target tw-mp-run
  ```
  *(Launches both games side-by-side, applies the Windows 11 CPU0 affinity fix to both processes, and streams live color-coded logs: `[G1]` in Cyan for Host and `[G2]` in Yellow for Player 2).*

* **Live Stream Logs (when games are already running):**
  ```powershell
  .\tools\twdll.ps1 tail-mp attila
  ```
