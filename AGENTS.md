**DO NOT BE OBSEQUIOUS**

**twdll** is a 32-bit C++ Lua script extender and self-contained reverse-engineering pipeline for Warscape titles (`rome2`, `attila`, `tob`), driven by a native Go CLI (`./twdll` / `twdll.cmd`).
Canonical architecture and stage roadmap: `docs/cli_re_pipeline_spec.md`.

## Active Stage Boundaries

Development follows three strictly sequential stages — never implement or stub future stages early:
- **Stage 1 — Bootstrap, `status`, `init`, `analyze`, `clean` (`cli/`):** Complete.
- **Stage 2 — RE Extraction & Search (`cli/` -> `.twdll/extracted/`):** **ACTIVE.** Agree on `.twdll/extracted/` schema and `extract` / `search` / `show` command contracts with the user before writing implementation code.
- **Stage 3 — Multi-Game C++ Rewrite & Build/Test CLI (`src/`, `CMakeLists.txt`, `tests/`, LDoc `docs/{CMakeLists.txt,attila,rome2,lua}`):** **FROZEN.** Do not modify C++ sources, CMake files, Lua test scripts, LDoc API docs, or wire `build`/`pack`/`install`/`run`/`test`/`docs` commands until Stage 2 is complete. (Preserved Warscape/Lua domain rules live in `docs/cli_re_pipeline_spec.md` §6 and `git show c9a35e9:AGENTS.md` / tag `archive/legacy-agents-v1`.)

## Operating Principles

- **User Authority & Conflict Flagging:** The user authored these instructions; they outrank generic defaults. If any instruction conflicts with another rule, agent role, or actual code reality, act sensibly and flag the conflict briefly at the end of the turn with a concrete fix proposal.
- **Simple, Iterative Execution:** Failure is free; iteration beats overthinking. Prefer the simplest clean solution, verify against real code and tests, and stop once solved. Avoid repetitive tool-call loops or defensive re-reads.
- **Design Gate vs Implementation Autonomy:** Present concise proposals and get user confirmation before introducing new CLI commands, storage schemas, or architectural changes. Once a task or blueprint is approved (including orchestrator -> subagent delegation), edit files and run fast local builds/unit tests directly without per-file diff pauses.
- **English Only:** All code comments, documentation, diagnostics, and log messages must be written in English.

## Plans, Research & Documentation

- **Default Scratchpad (`.agents/plans/`, `.agents/research/` — gitignored):** Write working execution plans, checklists, raw benchmarks, and exploratory spikes here by default. Prune or leave completed/superseded task plans local — never commit checklists or raw dumps.
- **Tracked Engineering Docs (`docs/plans/`, `docs/research/` — Git-tracked, English-only):** Promote artifacts to `docs/` **only** with user confirmation:
  - `docs/plans/<slug>.md`: Active cross-stage or multi-session proposals (use semantic kebab-case slugs and a `Status:` header; fold into canonical specs and delete the plan file once implemented).
  - `docs/research/<slug>.md`: Distilled, host-independent Warscape RE findings, ABI/DWARF rules, and durable technical tradeoffs (no local paths or raw logs).

## Workspace & Data Safety

- **Protect `.twdll/` and `twdll.toml`:** `.twdll/` stores ~9 GB of verified game binaries, portable toolchains, and analyzed Ghidra/IDA databases. Never run destructive commands (`twdll clean`, `twdll analyze --force`, or manual deletion inside `.twdll/` / `twdll.toml`) against the live workspace without explicit user permission.
- **Isolated Tests Only:** Automated tests must never mutate the repo's real `twdll.toml` or `.twdll/` directory; always isolate via `t.TempDir()`, `--config`, and `--data-dir`.

## Git Hygiene

- **Explicit Request Only:** Never create commits, tags, amends, or pushes unless explicitly requested by the user (no silent auto-commits).
- **Conventional Commits:** Use `<type>(<scope>): <short lowercase description>` (`feat`, `fix`, `refactor`, `test`, `docs`, `chore` — e.g., `feat(cli): add dwarf symbol extractor`).
