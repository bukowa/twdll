# Total War: Rome II Script Extender (twdll)

`twdll` is a mod for *Total War: Rome II* that extends the game's scripting capabilities, allowing for more complex and powerful mods. What started as a proof-of-concept for calling the game's Lua functions from a C++ DLL has evolved into a foundational script extender.

This project provides a framework for modders to add new functionality to the game by exposing more of the game's internal workings to the Lua scripting environment.

## Installation

## Compatibility Note

**Important:** This mod is currently built against game version **2.4.0.19534** and is **not compatible with the Steam version** of *Total War: Rome II*. Attempting to use it with an incompatible version may lead to crashes or unexpected behavior.

Development primarily focuses on the standalone (non-Steam) version of the game due to easier mod loading and overall development workflow. To make the mod compatible with the Steam version, primarily updating signatures would be required.

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

## Development

This section is for those who wish to contribute to the development of `twdll` itself or build it from the source code.

### Development Environment

This project is configured to work with Visual Studio Code. For the best development experience, it is recommended to install the following extensions:

-   [**clangd**](https://marketplace.visualstudio.com/items?itemName=llvm-vs-code-extensions.vscode-clangd): For C++ language support. The repository is pre-configured to work with it.
-   [**CMake Tools**](https://marketplace.visualstudio.com/items?itemName=ms-vscode.cmake-tools): For easier CMake integration within VS Code.


### Building from Source

This project is built with CMake and primarily targets Visual Studio tools for a 32-bit build environment. For a detailed reference on how to build the project, including specific tool versions, please refer to the `.github/workflows/build.yml` file.

#### Prerequisites

-   **Visual Studio 2022 Community Edition:** With the "Desktop development with C++" workload installed.
-   **Windows 10/11 SDK:** A recent version compatible with Visual Studio 2022.
-   **CMake:**
-   **Ninja:**
-   **RPFM CLI:** You can download it from the [RPFM releases page](https://github.com/Frodo45127/rpfm/releases).

#### Steps

1.  **Clone the repository:**

    ```sh
    git clone https://github.com/bukowa/twdll.git
    cd twdll
    ```

2.  **Configure the project with CMake:**

    ```sh
    cmake --preset vs2022-win32-ninja
    ```

3.  **Build the project:**

    ```sh
    cmake --build --preset vs2022-win32-ninja
    ```

    The build process will generate `twdll.dll` and other release files in the `build/release` directory.

### Technical Notes

This project was built against a specific version of Lua used by the game.

- Requires **Lua 5.1.2** for proper functionality. Using other versions might lead to instability or crashes.
- The game's method for table insertion in Lua (`events[event][#events[event]+1] = func`) is not compatible with standard Lua VMs. This project works around this by using `table.insert`.

### Reverse Engineering with ReClass.NET

The memory offsets and structures used in this project were identified using [ReClass.NET](https://github.com/ReClassNET/ReClass.NET). The ReClass files containing the structure definitions can be found in the `tools/` directory of this repository.

You can download a compatible version of ReClass.NET from [here](https://github.com/bukowa/ReClass.NET/releases/tag/1.0.0). This tool is essential for anyone looking to contribute to the project by finding new offsets or understanding the existing ones.

To inspect a game object, you can use the `GetMemoryAddress` function provided by the script extender to get its address and then examine it in ReClass.NET.