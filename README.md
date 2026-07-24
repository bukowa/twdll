# Total War Script Extender (twdll)

![Build](https://github.com/bukowa/twdll/actions/workflows/build.yml/badge.svg)
![CodeQL](https://github.com/bukowa/twdll/actions/workflows/codeql-analysis.yml/badge.svg)

Run custom C++ code inside Total War: Rome II and Attila's Lua engine.

## Installation

1. Download latest release from [Releases](https://github.com/bukowa/twdll/releases)
2. Extract `twdll_rome2.dll` or `twdll_attila.dll` to your game folder
3. The DLL only activates when called by a mod

## Usage

Load the DLL in your Lua script:

```lua
-- For Rome 2
twdll = package.loadlib("twdll_rome2.dll", "luaopen_twdll")()

-- For Attila
twdll = package.loadlib("twdll_attila.dll", "luaopen_twdll")()

-- Use the API
twdll.Log("Hello from C++!")
```

## Docs: 
- [Rome 2](https://bukowa.github.io/twdll/rome2/)
- [Attila](https://bukowa.github.io/twdll/attila/)

## Development

### Requirements
- Visual Studio 2022 (C++)
- CMake 3.29+

### Build
```sh
# Rome 2
cmake --preset rome2
cmake --build --preset rome2

# Attila
cmake --preset attila
cmake --build --preset attila
```

### Project Structure
```
src/
├── main.cpp              # DLL entry point
├── common/               # Shared utilities
├── rome2/                # Rome 2 specific code
└── attila/               # Attila specific code
```

## Release

1. Update `CHANGELOG.md`
2. Commit and push to master
3. Create git tag: `git tag v1.0.0 && git push origin v1.0.0`
4. GitHub Actions builds and releases automatically
