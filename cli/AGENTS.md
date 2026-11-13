# Go CLI Rules (`cli/`, `twdll`, `twdll.cmd`)

## Stage 1 Architecture Invariants (Do Not Regress)

- **Single Native Go Runtime:** Keep dependencies minimal; never introduce Python/`uv` or external runtime requirements into the CLI. Wrapper scripts (`./twdll`, `twdll.cmd`) must stay zero-overhead when `.twdll/bin/twdll(.exe)` is up to date.
- **Unified `Options` -> `Prompter` -> `Run` Pipeline:** Populate a plain `Options` struct from flags + `twdll.toml`. Invoke an interactive `Prompter` *only* when `stdin` is a TTY (`go-isatty`) and `--no-input` is false. Core `Run*` functions must never read `stdin`; missing inputs in `!TTY` fail fast with a non-zero exit code and actionable `stderr` help. Never build divergent TUI-wizard vs headless-heuristic codepaths.
- **Stream Separation (`Out` vs `Err`) & `!TTY` Purity:** Write primary command data (e.g., `status --json`) solely to injected `Out` (`stdout`) and route all prompts, progress, warnings, and errors to injected `Err` (`stderr`). When `!TTY` or `--color=never`, emit zero ANSI escapes, zero `\r` spinners, and trigger **zero host GUI/OS side effects** (no blocking console reads, no clipboard writes, no `steam://` URL launches).
- **Explicit Workspace Paths (No `os.Getwd()` Sprawl):** Thread resolved `--root`, `--config`, and `--data-dir` paths explicitly through workspace structs rather than calling `os.Getwd()` inside domain packages. `twdll.toml` stores only user choices and external game/IDA paths — never internal `.twdll/` paths.
- **Non-Destructive `state.json`:** Preserve existing target keys on partial updates and write `analyzed_sha256` atomically only after a clean exit code `0`.
- **Fast Hermetic Tests:** Verify changes with `go test ./...` inside `cli/` (falling back to `..\.twdll\tools\go\bin\go.exe test ./...` if `go` is not on `PATH`). Assert `Out` and `Err` buffers separately using `t.TempDir()`.

## Stage 2 Extraction & Search Guardrails

- **Schema & Command Agreement First:** Confirm `.twdll/extracted/` storage layout and CLI flags with the user before implementing `extract`, `search`, or `show`.
- **Native APIs Only:** Read DWARF (Go `debug/dwarf` / `debug/elf`), Ghidra, and IDA APIs directly — never write ad-hoc C++ string lexers or regex syntax parsers.
- **Incremental Vertical Slices:** Build one small extraction/query slice at a time, inspect real extracted records from `.twdll/db/` with the user, then prune or extend.
- **Zero Speculative Bloat:** Do not add cross-binary fuzzy matching hashes, speculative heuristics, or extra tables/indexes before concrete data proves they are needed.
