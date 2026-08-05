# Total War Script Extender (twdll)

![Build](https://github.com/bukowa/twdll/actions/workflows/build.yml/badge.svg)
![CodeQL](https://github.com/bukowa/twdll/actions/workflows/codeql-analysis.yml/badge.svg)

Run custom C++ code inside Total War: Attila's Lua engine. (Rome 2 support is frozen.)

## Installation

1. Download latest release from [Releases](https://github.com/bukowa/twdll/releases)
2. Extract `twdll_attila.dll` to your game folder
3. The DLL only activates when called by a mod

## Usage

Load the DLL in your Lua script:

```lua
twdll = package.loadlib("twdll_attila.dll", "luaopen_twdll")()

-- Use the API
twdll.core.Log("Hello from C++!")
```

## Docs: 
- [Attila](https://bukowa.github.io/twdll/nightly/attila/)
- [Rome 2 (frozen)](https://bukowa.github.io/twdll/nightly/rome2/)

> **Rome 2 support is totally frozen.** No code, build, or docs changes are made for Rome 2 —
> the `src/rome2/`, `tests/rome2/`, and `docs/rome2/` trees are preserved as-is. The Rome 2 docs
> below are stale and will stay that way until an incoming game update lands, invalidating all
> internal offsets, and structures are re-verified. Active development targets Attila only.

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
