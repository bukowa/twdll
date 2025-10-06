# Total War: Rome II Script Extender (twdll)

`twdll` is a mod for *Total War: Rome II* that extends the game's scripting capabilities, allowing for more complex and powerful mods. What started as a proof-of-concept for calling the game's Lua functions from a C++ DLL has evolved into a foundational script extender.

This project provides a framework for modders to add new functionality to the game by exposing more of the game's internal workings to the Lua scripting environment.

## Documentation

The documentation for the available script interfaces can be found here: https://bukowa.github.io/twdll/

## Features

*   **Extends Scripting Interfaces:** Introduces new Lua script interfaces to interact with game elements that were previously inaccessible.
*   **C++-Lua Interoperability:** Allows calling game functions from C++ and exposing C++ functions to the game's Lua environment.
*   **Function Hooking:** Uses MinHook to safely intercept and extend game functions.

## Getting Started

### For Players

1.  Download the latest release.
2.  Copy `twdll.dll` and `lua.dll` to your *Total War: Rome II* installation directory (e.g., `C:/Program Files (x86)/Steam/steamapps/common/Total War Rome II`).
3.  The mod should be active the next time you launch the game.

### For Modders

You can use the extended script interfaces provided by this mod in your own Lua scripts. Check the [documentation](https://bukowa.github.io/twdll/) for details on the available functions.

## Building from Source

This project is built with CMake.

1.  Clone the repository.
2.  Configure the project with CMake.
3.  Build the `twdll` target.

The build process will generate `twdll.dll`.

## Technical Notes

This project was built against a specific version of Lua used by the game.

- Requires **Lua 5.1.2** for proper functionality. Using other versions might lead to instability or crashes.
- The game's method for table insertion in Lua (`events[event][#events[event]+1] = func`) is not compatible with standard Lua VMs. This project works around this by using `table.insert`.
