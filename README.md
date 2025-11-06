# Total War: Rome II Script Extender (twdll)

`twdll` is a mod for *Total War: Rome II* that extends the game's scripting capabilities, allowing for more complex and powerful mods. What started as a proof-of-concept for calling the game's Lua functions from a C++ DLL has evolved into a foundational script extender.

This project provides a framework for modders to add new functionality to the game by exposing more of the game's internal workings to the Lua scripting environment, as well as integrating ImGui UI and Python into the game's event loop.

## Installation

## Compatibility Note

**Important:** This mod is currently built against game version **2.4.0.20027** and is compatible with the Steam version of *Total War: Rome II*.

There are two ways to install `twdll`:

### Stable Release (Recommended)

1.  Go to the [**releases page**](https://github.com/bukowa/twdll/releases).
2.  Download the latest `libtwdll-vX.X.X.zip` file.
3.  Extract the contents of the zip file directly into your *Total War: Rome II* game directory (e.g., `C:/Program Files (x86)/Steam/steamapps/common/Total War Rome II`).

### Nightly Build (Development Version)

For the latest features and bugfixes, you can use the nightly build.

1.  Go to the [**nightly release page**](https://github.com/bukowa/twdll/releases/tag/nightly).
2.  Download the `libtwdll-nightly.zip` file.
3.  Extract the contents of the zip file directly into your *Total War: Rome II* game directory.

This will place `twdll.dll` in the game's root directory and the `twdll.pack` file in the `data` subdirectory.

## Usage for Modding

To use the script extender in your own mods, you need to load the `twdll.dll` library from your Lua script. Add the following code to your script:

```lua
-- Load the script extender library
package.loadlib("twdll.dll", "luaopen_twdll")()
```

After loading the library, you will have access to the new functions provided by the script extender.

### Scripting Documentation

For a complete list of available functions and how to use them in your own Lua scripts, please refer to the official documentation:

**[https://bukowa.github.io/twdll/](https://bukowa.github.io/twdll/)**

### Examples

Practical examples of how to use the script extender's functions can be found in the `examples/` directory. The `example.lua` file provides a starting point for understanding how to integrate these functions into your own mods.

## Features

*   **Lua Scripting:** Extends the game's Lua scripting capabilities, allowing for more complex and powerful mods.
*   **ImGui Integration:** Provides an ImGui backend for creating in-game user interfaces.
*   **Python Integration:** Allows running Python scripts in the game's context.
*   **C++ Framework:** Offers a C++ framework for adding new functionality to the game.
*   **Memory Patching:** Includes MinHook for function hooking and memory patching.

## Project Structure

*   `src/`: Contains the C++ source code for the script extender.
*   `include/`: Contains the header files for the project, including dependencies like ImGui, MinHook, and Python.
*   `scripts/`: Contains various scripts for building, running, and managing the project.
*   `tools/`: Contains tools used for development, such as `ldoc` for documentation generation and `ReClass.NET` for reverse engineering.
*   `examples/`: Contains example Lua scripts demonstrating how to use the script extender.
*   `pack/`: Contains the files that are packed into the `twdll.pack` file.
*   `luadoc/`: Contains the generated Lua documentation.

## Development

This section is for those who wish to contribute to the development of `twdll` itself or build it from the source code.

### Development Environment

This project is configured to work with Visual Studio Code. For the best development experience, it is recommended to install the following extensions:

-   [**clangd**](https://marketplace.visualstudio.com/items?itemName=llvm-vs-code-extensions.vscode-clangd): For C++ language support. The repository is pre-configured to work with it.
-   [**CMake Tools**](https://marketplace.visualstudio.com/items?itemName=ms-vscode.cmake-tools): For easier CMake integration within VS Code.


### Building from Source

This project is built with CMake and primarily targets Visual Studio tools for a 32-bit build environment. For a detailed reference on how to build the project, including specific tool versions, please refer to the `.github/workflows/build.yml` file.

#### Prerequisites

-   **Visual Studio 2022 Community Edition:** With the "Desktop development with C++" workload installed. The continuous integration build uses MSVC toolset version `14.44.35207` and Windows SDK version `10.0.26100.0`.
-   **CMake:** Version 3.29 or higher.
-   **Ninja:** A recent version.
-   **RPFM CLI:** Version `v4.3.14` is used in the build workflow. You can download it from the [RPFM releases page](https://github.com/Frodo45127/rpfm/releases).

#### Steps

1.  **Configure the project with CMake:**
    ```sh
    cmake --preset vs2022-install
    ```

2.  **Build the project:**
    ```sh
    cmake --build --preset vs2022-install
    ```

### Reverse Engineering with ReClass.NET

The memory offsets and structures used in this project were identified using [ReClass.NET](https://github.com/ReClassNET/ReClass.NET). The ReClass files containing the structure definitions can be found in the `tools/` directory of this repository.

You can download a compatible version of ReClass.NET from [here](https://github.com/bukowa/ReClass.NET/releases/tag/1.0.0). This tool is essential for anyone looking to contribute to the project by finding new offsets or understanding the existing ones.

To inspect a game object, you can use the `GetMemoryAddress` function provided by the script extender to get its address and then examine it in ReClass.NET.