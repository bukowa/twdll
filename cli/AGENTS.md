# Go CLI Rules (`cli/`, `twdll`, `twdll.cmd`)

Status: **Stage 1 (including 3-tier E2E verification) Complete; Stage 2 (`extract`, `derive`, `search`, `show`, `verify`) Active.**
See `./README.md` for commands, env vars, and test workflows, and `../docs/cli_re_pipeline_spec.md` §5 for stage tracking.

## Stage 1 Architecture Invariants (Do Not Regress)

- **Single Native Go Runtime:** Keep dependencies minimal; never introduce Python/`uv` or external runtime requirements into the CLI. Wrapper scripts (`./twdll`, `twdll.cmd`) must stay zero-overhead when `.twdll/bin/twdll(.exe)` is up to date.
- **Unified `Options` -> `Prompter` -> `Run` Pipeline:** Populate a plain `Options` struct from flags + `twdll.toml`. Invoke an interactive `Prompter` *only* when `stdin` is a TTY (`go-isatty`) and `--no-input` is false. Core `Run*` functions must never read `stdin`; missing inputs in `!TTY` fail fast with a non-zero exit code and actionable `stderr` help. Never build divergent TUI-wizard vs headless-heuristic codepaths.
- **Stream Separation (`Out` vs `Err`) & `!TTY` Purity:** Write primary command data (e.g., `status --json`) solely to injected `Out` (`stdout`) and route all prompts, progress, warnings, and errors to injected `Err` (`stderr`). When `!TTY` or `--color=never`, emit zero ANSI escapes, zero `\r` spinners, and trigger **zero host GUI/OS side effects** (no blocking console reads, no clipboard writes, no `steam://` URL launches).
- **Explicit Workspace Paths & Read-Only `TWDLL_TOOLS_DIR`:** Thread resolved `--root`, `--config`, and `--data-dir` paths explicitly through workspace structs rather than calling `os.Getwd()` inside domain packages. `twdll.toml` stores only user choices and external game/IDA paths — never internal `.twdll/` paths. When `TWDLL_TOOLS_DIR` points to an external shared toolchain directory (`Paths.HasExternalToolsDir()`), treat it as read-only: `EnsureDirs`, `clean`, and `init --clear` must never create, mutate, or delete it.
- **Non-Destructive `state.json`:** Preserve existing target keys on partial updates and write `analyzed_sha256` atomically only after a clean exit code `0`.

## 3-Tier Test Discipline (See `cli/README.md` for Details)

- **Tier 0 — Fast Hermetic Unit Tests (`<5s`, mandatory on every `cli/` edit):** Run `go vet ./...` and `go test ./...` inside `cli/` (fall back to `..\.twdll\tools\go\bin\go.exe` on Windows or `../.twdll/tools/go/bin/go` on POSIX if `go` is not on `PATH`). Isolate via `t.TempDir()` and assert `Out` and `Err` buffers separately.
- **Tier 1 — Quick Real-Decompiler E2E (`~30–45s`, required for lifecycle/decompiler changes):** Run `go test -v -tags=e2e -timeout 5m ./internal/e2e` inside `cli/` whenever modifying `init`, `analyze`, `clean`, `status`, `workspace`, decompiler drivers/scripts, or PE/ELF/DWARF inspection. Compiles `testdata/mini_warscape.cpp` and verifies live `twdll.toml` and `.twdll/state.json` remain untouched.
- **Tier 2 — Real-Binary Background Soak (`20–75m`, `6–17 GB` RAM):** Never run `TWDLL_E2E_SOAK=1` synchronously inside an interactive agent turn. Launch detached via `pwsh -NoProfile -File cli/scripts/run-e2e-bg.ps1` only when explicitly validating real game binaries, DWARF import, or memory budgets.

## Stage 2 Extraction, Search & Audit Guardrails

- **Database Naming & Roles (`.twdll/extracted/`):**
  - `.twdll/extracted/<target>/binary.sqlite` — Static binary structure, DWARF4 symbols/types (`attila_64`), and x86 instruction graph (`functions`, `strings`, `xrefs`, `call_edges`, `vtables`, `rtti_*`, `lua_regs`).
  - `.twdll/extracted/<target>/decomp_ida.sqlite` & `decomp_ghidra.sqlite` — Per-engine decompiler outputs (`decomp_functions` + `FTS5`, `line_ea_json`, `decomp_vars`).
  - `.twdll/extracted/derived.sqlite` — Algorithmic cross-binary mappings (`attila_64` -> `attila_32` / `rome2_32` / `tob_32`), struct layouts, and candidate field offsets with explicit confidence and evidence chains.
- **Zero Optimism & Continuous Improvement:** Extracted databases are parser, heuristic, and decompiler outputs — never infallible truth. Compiler optimizations (inlining, tail calls, ABI packing), decompiler errors (`in_ECX`, wrong boundaries, variable coalescing), and bugs in our own Go extractors will occur. Always cross-check `binary.sqlite`, `decomp_ida.sqlite`, `decomp_ghidra.sqlite`, and raw `x86asm` disassembly. When a bug or blind spot is found during audit or RE, fix the extractor in `cli/`, add a unit test, and re-extract.
- **Mandatory Post-Implementation Audit Before Stage 3:** After wiring `extract`, `derive`, `search`, `show`, and `verify`, execute a dedicated **Extraction Audit & Hardening Phase** with the user to inspect real Warscape subsystems, compare IDA vs Ghidra vs DWARF vs raw x86 assembly, and fix extractor defects before unlocking Stage 3 (`src/`).
- **Schema & Command Agreement First:** Confirm `.twdll/extracted/` storage layout and CLI flags with the user before implementing `extract`, `derive`, `search`, `show`, or `verify`.
- **Native APIs Only:** Read DWARF (Go `debug/dwarf` / `debug/elf`), Ghidra, and IDA APIs directly — never write ad-hoc C++ string lexers or regex syntax parsers.
- **Incremental Vertical Slices:** Build one small extraction/query slice at a time, inspect real extracted records from `.twdll/db/` with the user, then prune or extend.
- **Zero Speculative Bloat:** Do not add cross-binary fuzzy matching hashes, speculative heuristics, or extra tables/indexes before concrete data proves they are needed.
- **Bulk State vs Curated RE Docs:** `twdll extract` writes machine-generated artifacts to gitignored `.twdll/extracted/`; agreed schemas/CLI contracts belong in `docs/cli_re_pipeline_spec.md`, and curated Warscape RE findings (when approved for Git) belong in `docs/research/`.
