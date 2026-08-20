# Total War Script Extender (twdll)

![Build](https://github.com/bukowa/twdll/actions/workflows/build.yml/badge.svg)
![CodeQL](https://github.com/bukowa/twdll/actions/workflows/codeql-analysis.yml/badge.svg)
[![Attestations](https://img.shields.io/badge/Sigstore-Attested-success?logo=github)](https://github.com/bukowa/twdll/attestations)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Discord](https://img.shields.io/badge/Discord-Join%20Chat-5865F2?logo=discord&logoColor=white)](https://discord.gg/6vm2M94vhX)

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

> **Where to load twdll**
>
> Load the DLL from the campaign script — `campaigns/<campaign>/scripting.lua` — exactly as the
> test suite does (`tests/attila/pack/campaigns/main_attila/scripting.lua`). On first load twdll
> hooks game singletons (world, campaign UI, campaign model), and those objects are only
> constructed once a campaign world is created. Calling singleton-dependent functions any earlier
> (e.g. from the main menu or a global script) can return `nil` or crash.
>
> Not every function requires this: plain utilities (`twdll.core.Log`, `twdll.core.GameBuild`) and
> functions that read fixed module offsets work regardless of where they are called. This is how
> the current hook architecture works, not a permanent contract — it may change in the future.

## Documentation
- [Online API Documentation](https://bukowa.github.io/twdll/)

> **Rome 2 support is frozen.** No code, build, or docs changes are actively made for Rome 2 —
> the `src/rome2/`, `tests/rome2/`, and `docs/rome2/` trees are preserved as-is. Active development,
> releases, and documentation target Attila only.

## Development

### Requirements
- Visual Studio 2022 (C++), 32-bit (x86) toolset
- CMake 3.29+
- Linux cross-compile: `clang-cl`, `lld-link`, Ninja, and [xwin](https://github.com/Jake-Shadle/xwin).

### Build

## Windows
```sh
# Attila (active)
cmake --preset attila
cmake --build --preset attila

# Rome 2 (frozen)
# cmake --preset rome2
# cmake --build --preset rome2
```

## Linux (cross-compile)
```sh
# Install MSVC CRT + Windows SDK once (default location ~/xwin)
xwin --accept-license --arch x86 splat --output ~/xwin

# Attila (active)
cmake -S . -B build -G Ninja \
  --toolchain cmake/clang-cross-x86.cmake \
  -DCMAKE_BUILD_TYPE=Release \
  -DTW_GAME=attila
cmake --build build --target twdll
```
Set `XWIN_ROOT` if you splatted somewhere other than `~/xwin`.

### Project Structure
```
src/
├── main.cpp              # DLL entry point
├── common/               # Shared utilities
├── attila/               # Attila specific code (active)
└── rome2/                # Rome 2 specific code (frozen)
```

## Contributing

twdll works by injecting C++ into Attila's Lua engine: it finds game functions and
structs by byte-signature scanning and fixed offsets, then hooks them at runtime. Most
contribution work is reverse-engineering the game to map out what to hook.

### The Linux build is your best reverse-engineering aid

> The **Steam Linux build of Attila is 64-bit and ships with DWARF debug symbols**
> built in. The Windows binary this project targets is 32-bit and stripped, so it is
> far harder to read. If you want to contribute:
>
> 1. Download the Linux version of Attila from Steam.
> 2. Load it in IDA/Ghidra (or a DWARF-aware tool) to read real function names,
>    class layouts and vtables instead of guessing from disassembly.
> 3. Map what you learn from the 64-bit Linux symbols back onto the 32-bit Windows
>    binary — addresses differ, but the class structures and logic carry over.

### Adding a Lua API

1. Register the module in `src/main.cpp`.
2. Add an LDoc comment (`@function`, `@tparam`, `@treturn`) for every exposed function.
3. List the source in `TWDLL_DOC_SOURCES` in `docs/CMakeLists.txt`.
4. Add test cases in `tests/shared/testing.lua`.
5. Regenerate docs and commit them (see `AGENTS.md` → Documentation).

Questions and ideas go in [Issues](https://github.com/bukowa/twdll/issues) or on [Discord](https://discord.gg/6vm2M94vhX).

## Release

1. Update `CHANGELOG.md`
2. Commit and push to master
3. Create git tag: `git tag v1.0.0 && git push origin v1.0.0`
4. GitHub Actions builds and releases automatically

## Security & Verification

All `twdll` binaries are built on public GitHub Actions runners and cryptographically signed using **GitHub Artifact Attestations (Sigstore)** with published SHA-256 checksums.

### Verify Provenance & Integrity
You can verify that your downloaded binary matches the exact open-source commit built by GitHub:
```bash
gh attestation verify libtwdll-nightly.zip --repo bukowa/twdll
```

- **Public Attestation Log:** [github.com/bukowa/twdll/attestations](https://github.com/bukowa/twdll/attestations)
- **Multi-AV Reports:** Automated VirusTotal scans are linked on each [Release page](https://github.com/bukowa/twdll/releases).
