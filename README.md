# Total War Script Extender (twdll)


This project lets you run custom C++ code inside the native Lua engine of **Total War: Rome II** and **Total War: Attila**.


## How it works
The game engine has its own Lua state. `twdll` uses **Signature Scanning** to find the internal Lua functions in the game's memory at runtime. This allows the DLL to register a custom `twdll` table that your scripts can use.

The project already includes example functions for modifying units, characters, and factions, which you can use as a base for your own features.

## Installation
1. Go to the [**Releases Page**](https://github.com/bukowa/twdll/releases).
2. Download the latest `libtwdll.zip`.
3. Extract everything into your main game folder (where `Rome2.exe` is).
4. The DLL is passive; it only starts working if a mod calls for it.

> [!NOTE]
> The release zip contains one DLL per game (`twdll_rome2.dll`, `twdll_attila.dll`). Place the correct one in your game root.


## For Modders
Make sure the correct `twdll_<game>.dll` is in the game root directory, then load it at the start of your script:

```lua
-- Rome 2
package.loadlib("twdll_rome2.dll", "luaopen_twdll")()

-- Attila
package.loadlib("twdll_attila.dll", "luaopen_twdll")()

-- Use it
twdll.Log("Hello from C++!")
```

Check out the [API Documentation](https://bukowa.github.io/twdll/) for all the available functions.

## Development
We recommend using **CLion** for development. It's built with CMake and is easy to set up.

### Prerequisites
- **Visual Studio 2022** (with C++ components)
- **CMake** 3.29+
- **RPFM CLI** (to build the pack files)

> [!IMPORTANT]
> On first run, `tools/twdll.ps1` will auto-create `tools/paths.ps1` from the example and open it for editing. Fill in your game install paths, then re-run.

### Build System

Configure and build for a specific game using presets:

```sh
cmake --preset rome2
cmake --build --preset rome2

cmake --preset attila
cmake --build --preset attila
```

Outputs: `build/rome2/twdll.dll` and `build/attila/twdll.dll`.

### Task Runner (`tools/twdll.ps1`)

All install/run/test operations use the task script — CMake just compiles:

```powershell
.\tools\twdll.ps1 install rome2          # copy DLL + pack to game dir
.\tools\twdll.ps1 run rome2              # install + launch
.\tools\twdll.ps1 run rome2 -Steam       # install + launch via Steam
.\tools\twdll.ps1 test rome2             # full automated test cycle
.\tools\twdll.ps1 help                   # all commands
```

The IDE targets `tw-install`, `tw-run`, `tw-test` also exist as wrappers for the same script if you prefer clicking buttons in CLion/VS.

### Building Documentation
We use **LDoc** to generate the API reference from the C++ comments. To build it manually:
```sh
cmake --build --preset rome2-release --target docs
```
The documentation will be generated in the `docs/` folder.

## How to Release
Official releases are automated via GitHub Actions. To release a new version:

1.  **Update `CHANGELOG.md`**: Add a new entry for the version (e.g., `## [0.5.0] - 2026-05-01`).
    *   You can validate your changes locally using: `python .github/scripts/validate_changelog.py`
2.  **Commit and Push**: Push your changes to the `master` branch.
3.  **Tag the Version**: Create a git tag starting with `v` and push it:
    ```sh
    git tag v0.5.0
    git push origin v0.5.0
    ```
4.  **Verification**: The GitHub Action will automatically build the DLL, package the mod, extract the changelog notes, and create the official GitHub Release.