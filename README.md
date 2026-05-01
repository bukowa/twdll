# Total War: Rome II Script Extender (twdll)

This project lets you run custom C++ code inside the Rome II Lua engine. It’s useful for modders who want to do things that the standard game scripts can't handle.

## How it works
The game engine has its own Lua state. `twdll` uses **Signature Scanning** to find the internal Lua functions in the game's memory at runtime. This allows the DLL to register a custom `twdll` table that your scripts can use.

The project already includes example functions for modifying units, characters, and factions, which you can use as a base for your own features.

## Installation
1. Go to the [**Releases Page**](https://github.com/bukowa/twdll/releases).
2. Download the latest `libtwdll.zip`.
3. Extract everything into your main game folder (where `Rome2.exe` is).
4. The DLL is passive; it only starts working if a mod calls for it.

## For Modders
To use it in your mod, make sure `twdll.dll` is in the game root directory, then load the library at the start of your script:

```lua
-- Load the library
package.loadlib("twdll.dll", "luaopen_twdll")()

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
> You must edit `CMakePresets.json` to point to your actual game and tool paths before building.

### Automated Workflow
We have CMake presets that handle everything:
- `run_alone_tail`: Builds everything, starts the game, and shows you the log in your IDE.
- `run_alone_test_tail`: Runs our automated test suite and tells you if it passed.

### Building Documentation
We use **LDoc** to generate the API reference from the C++ comments. To build it manually:
```sh
# This is usually part of the release build
cmake --build --preset vs2022-release
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