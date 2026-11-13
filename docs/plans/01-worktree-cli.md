# 01 — Cross-Platform `twdll wt` Worktree CLI

- **Status:** Complete
- **Goal:** Add a cross-platform `twdll wt` command in `cli/` to create, inspect, sync, and safely remove git worktrees sharing heavy `.twdll/` assets without duplicating ~9 GB or risking accidental deletion of shared databases/tools.
- **Locked Decisions:**
  - **Hybrid `.twdll/` layout per worktree:** Real local `.twdll/bin/` (initially empty so first wrapper run compiles cleanly for that branch) and `.twdll/logs/`; OS directory links (`tools/`, `binaries/`, `db/`, `extracted/`) targeting the primary workspace's resolved `DataDir` (`FSCTL_SET_REPARSE_POINT` NTFS junctions on Windows — zero admin/dev-mode privilege needed; `os.Symlink` on Unix).
  - **Config & state isolation:** Seed `<wt>/twdll.toml` via `workspace.SaveConfig` (clearing `cfg.DataDir = ""`) and copy `<wt>/.twdll/state.json` via `workspace.CopyFileAtomic` (no file symlinks/hardlinks).
  - **Shared-data safety guards:** `twdll clean` and `twdll init --clear` must check `fslink.IsLinkOrReparsePoint` and fail fast with `term.DiagnosticError` inside linked worktrees; `twdll wt remove` unlinks junctions/symlinks without traversal before `git worktree remove`.
  - **CLI contract (`twdll wt` / `worktree`):**
    - `twdll wt list [--json]` (alias `ls`)
    - `twdll wt add <name> [--base <ref>] [--path <dir>] [--detach] [--json]` (aliases `new`, `create`; default `<parent>/<repo>.wt/<slug>`)
    - `twdll wt remove <target> [-f|--force] [--delete-branch]` (alias `rm`)
    - `twdll wt sync [<target>] [--all] [-f|--force]` (alias `repair`)
    - `twdll wt path <target>`

## What Exists to Build On
- `cli/internal/workspace/paths.go` — `Resolve`, `Paths`, `isRepoRoot` (checks `cli/go.mod`, already worktree-safe)
- `cli/internal/workspace/store.go` — `SaveConfig`, `CopyFileAtomic`, `WriteFileAtomic`
- `cli/internal/bootstrap/rebuild.go` — `MaybeRebuildAndExec` (expects local `<root>/.twdll/bin/` + `<root>/.twdll/tools/go`)
- `cli/internal/term/` — `UI` (`Out` vs `Err` stream separation), `DiagnosticError`
- `cli/internal/cmd/clean.go`, `cli/internal/cmd/init.go` — `RunClean`, `clearWorkspaceData` (require reparse/symlink guards)

## Phases
- [x] **Phase 0 (Baseline Probe & Design Gate):** Verify `git --version` (`2.54.0.windows.1`) and `git worktree list --porcelain` (confirmed `detached` & `prunable` stanzas in live repo), persist `docs/plans/01-worktree-cli.md`, confirm CLI contract with user.
- [x] **Phase 1 (`cli/internal/fslink/`):** Cross-platform directory links (`EnsureDirLink`, `ReadDirLink`, `RemoveLinkOnly`, `RemoveAllNoFollow`, `SamePath`, `IsLinkOrReparsePoint`) + `t.TempDir()` sentinel tests.
- [x] **Phase 2 (`cli/internal/worktree/`):** `git` porcelain plumbing (`GIT_TERMINAL_PROMPT=0`), provisioning/inspection/safe teardown, transactional `Add`, and hermetic git integration tests in `t.TempDir()`.
- [x] **Phase 3 (`cli/internal/cmd/`, `cli/main.go`):** Shared-data guards in `clean` & `init --clear`, `twdll wt` subcommands + Kong wiring, `cmd` & `main_test.go` buffer tests.

## Verification Oracle
- `cd cli && ..\.\.twdll\tools\go\bin\go.exe vet ./... && ..\.\.twdll\tools\go\bin\go.exe test -count=1 ./...` (or `go vet ./... && go test -race ./...` when available)
- Isolated temp-dir smoke binary (`$OPENCODE_TEMP/twdll-wt-smoke.exe`) exercising `wt add`, `wt list --json`, `wt sync`, `clean` guard, `init --clear` guard, and `wt rm` with sentinel survival check
- Read-only live probe: `./twdll wt list` and `./twdll wt list --json` in repo root
