package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"twdll/cli/internal/ui"
	"twdll/cli/internal/workspace"
)

// CleanOptions configures database wiping.
type CleanOptions struct {
	Decompiler string   // "ghidra", "ida", or "" / "all"
	All        bool     // clean all targets
	Targets    []string // specific targets (e.g. "attila_64")
}

// RunClean removes database artifacts, temporary files, logs, and resets state for requested targets.
func RunClean(ctx context.Context, paths workspace.Paths, opts CleanOptions, printer *ui.Printer) error {
	state, _, err := workspace.LoadState(paths.StateFile)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var engines []workspace.DecompilerID
	decLower := strings.ToLower(strings.TrimSpace(opts.Decompiler))
	switch decLower {
	case "ida":
		engines = []workspace.DecompilerID{workspace.DecompilerIDA}
	case "ghidra":
		engines = []workspace.DecompilerID{workspace.DecompilerGhidra}
	case "", "all":
		engines = []workspace.DecompilerID{workspace.DecompilerGhidra, workspace.DecompilerIDA}
	default:
		return fmt.Errorf("unknown decompiler: %q (valid: ghidra, ida, all)", opts.Decompiler)
	}

	var targets []workspace.TargetID
	if len(opts.Targets) > 0 {
		for _, raw := range opts.Targets {
			tid := workspace.TargetID(strings.ToLower(strings.TrimSpace(raw)))
			if !tid.IsValid() {
				return fmt.Errorf("unknown target: %q (valid targets: %v)", raw, workspace.AllTargets())
			}
			targets = append(targets, tid)
		}
	} else if opts.All {
		targets = workspace.AllTargets()
	} else {
		// If neither targets nor --all is specified, default to all targets
		targets = workspace.AllTargets()
	}

	clearedCount := 0
	for _, target := range targets {
		for _, engine := range engines {
			dbDir := paths.DatabaseDir(engine, target)
			logFile := paths.AnalyzeLogPath(engine, target)

			removedAny := false

			if stat, err := os.Stat(dbDir); err == nil {
				if err := os.RemoveAll(dbDir); err == nil {
					removedAny = true
				}
				_ = stat
			}

			if _, err := os.Stat(logFile); err == nil {
				if err := os.Remove(logFile); err == nil {
					removedAny = true
				}
			}

			// Clear state record
			if binState, ok := state.Binaries[target]; ok {
				if binState.Analyzed.Get(engine) != "" {
					binState.Analyzed.Set(engine, "")
					state.SetBinary(target, binState)
					removedAny = true
				}
			}

			if removedAny {
				clearedCount++
				if printer != nil {
					printer.Info("  [clean] Removed %s database for %s", printer.Bold(string(engine)), printer.Bold(string(target)))
				}
			}
		}
	}

	if err := workspace.SaveState(paths.StateFile, state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if printer != nil {
		if clearedCount == 0 {
			printer.Info("[clean] No databases needed cleaning.")
		} else {
			printer.Info("%s Cleared %d database(s). Run `twdll analyze` to rebuild.", printer.Green("[clean] Done!"), clearedCount)
		}
	}

	return nil
}
