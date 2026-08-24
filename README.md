# Total War Script Extender (twdll)

![Build](https://github.com/bukowa/twdll/actions/workflows/build.yml/badge.svg)
![CodeQL](https://github.com/bukowa/twdll/actions/workflows/codeql-analysis.yml/badge.svg)
[![Attestations](https://img.shields.io/badge/Sigstore-Attested-success?logo=github)](https://github.com/bukowa/twdll/attestations)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Discord](https://img.shields.io/badge/Discord-Join%20Chat-5865F2?logo=discord&logoColor=white)](https://discord.gg/6vm2M94vhX)

Run custom C++ code inside Total War: Attila's Lua engine. (Rome 2 support is frozen.)

## Installation

1. Download the latest development build: [**`libtwdll-nightly.zip`**](https://github.com/bukowa/twdll/releases/download/nightly/libtwdll-nightly.zip) (or check all releases on the [Dev Build page](https://github.com/bukowa/twdll/releases/tag/nightly))
2. Extract `twdll_attila.dll` into your Total War: Attila game root directory (where `Attila.exe` is located)
3. The DLL is active and ready to be loaded by your mod script

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

## Disclaimer: AI-Assisted & Vibe-Coded

> [!NOTE]
> This project is heavily **vibe-coded** with extensive AI assistance. We believe in complete transparency about how this software is built.
>
> While every effort is made to inspect, review, and test reverse-engineered offsets, memory layouts, and C++ hooks against real game binaries, unintended bugs, edge cases, and side effects can and will occur.
>
> - **Bug reports, code reviews, and PRs are warmly welcome!** If you spot an issue, an incorrect offset, or a cleaner way to implement something, your contributions are greatly appreciated.
> - **Freedom to Fork:** `twdll` is licensed under the **GNU General Public License v3.0 (GPL-3.0)**. You have the full right and freedom to fork, modify, maintain, or rebuild this project for your own needs.

## Development

### Building on Windows

#### Requirements
- **Visual Studio 2022** (Desktop development with C++, 32-bit / x86 toolset)
- **CMake** 3.29+

#### Build
```sh
# Configure & build for Attila (active)
cmake --preset attila
cmake --build --preset attila
```

---

### Cross-compiling on Linux

You can build the 32-bit Windows DLL on Linux using Clang and MSVC headers/libraries packaged via `xwin`.

#### Requirements
- **Clang** (`clang-cl`, `lld-link`)
- **Ninja**
- **CMake** 3.29+
- **[xwin](https://github.com/Jake-Shadle/xwin)** (to fetch MSVC CRT + Windows SDK)

#### Build
```sh
# 1. Download MSVC CRT + Windows SDK (one-time setup, defaults to ~/xwin)
xwin --accept-license --arch x86 splat --output ~/xwin

# 2. Configure & build for Attila
cmake -S . -B build -G Ninja \
  --toolchain cmake/clang-cross-x86.cmake \
  -DCMAKE_BUILD_TYPE=Release \
  -DTW_GAME=attila

cmake --build build --target twdll
```
> Set `XWIN_ROOT` environment variable if you splatted `xwin` somewhere other than `~/xwin`.

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

## Credits & Acknowledgements

This project would not have been possible without the immense work, research, and support of many individuals, modding teams, and tool creators across the Total War and reverse-engineering communities.

Special thanks to **Jake Armitage**, **Valerius**, **Irishbandito**, **Divide et Impera (DeI)**, **Medieval Kingdoms 1212 AD (MK1212)**, **The Dawnless Days (TDD)**, **Para Bellum**, **M2TWEOP Maintainers**, **Frodo (RPFM)**, **Taw (etwng)**, **daniu (PFM)**, **Da Modding Den**, **TW Modders Agora**, **Creative Assembly**, **Hex-Rays**, **NSA Ghidra**, **Tsuda Kageyu (MinHook)**, and the entire Total War modding community.

See [**`CREDITS.md`**](CREDITS.md) for the full list of acknowledgements and the [**GitHub Contributors Graph**](https://github.com/bukowa/twdll/graphs/contributors).

## Security & Verification

All `twdll` binaries are built on public GitHub Actions runners and cryptographically signed using **GitHub Artifact Attestations (Sigstore)** with published SHA-256 checksums.

### Verify Provenance & Integrity
You can verify that your downloaded binary matches the exact open-source commit built by GitHub:
```bash
gh attestation verify libtwdll-nightly.zip --repo bukowa/twdll
```

- **Public Attestation Log:** [github.com/bukowa/twdll/attestations](https://github.com/bukowa/twdll/attestations)
- **Multi-AV Reports:** Automated VirusTotal scans are linked on each [Release page](https://github.com/bukowa/twdll/releases).
