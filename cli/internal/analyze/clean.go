package analyze

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

// CleanWorkspace removes all decompiler databases, analyze logs, resource
// telemetry, and analyzed SHA-256 checkpoints while preserving staged binaries
// and init configuration.
func CleanWorkspace(paths workspace.Paths, ui *term.UI) (bool, error) {
	removed := false

	if _, err := os.Stat(paths.DBDir); err == nil {
		if rErr := os.RemoveAll(paths.DBDir); rErr != nil {
			return removed, fmt.Errorf("remove database dir %s: %w", paths.DBDir, rErr)
		}
		removed = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return removed, fmt.Errorf("stat database dir %s: %w", paths.DBDir, err)
	}

	resLog := paths.AnalyzeResourceLogPath()
	ok, err := removeIfExistsCounted(resLog)
	if err != nil {
		return removed, fmt.Errorf("remove resource log %s: %w", resLog, err)
	}
	if ok {
		removed = true
	}

	logMatches, gErr := filepath.Glob(filepath.Join(paths.LogsDir, "analyze_*.log"))
	if gErr != nil {
		return removed, fmt.Errorf("glob analyze logs: %w", gErr)
	}
	for _, lp := range logMatches {
		ok, err := removeIfExistsCounted(lp)
		if err != nil {
			return removed, fmt.Errorf("remove log file %s: %w", lp, err)
		}
		if ok {
			removed = true
		}
	}
	_ = os.Remove(paths.LogsDir)

	state, stateExisted, err := workspace.LoadState(paths.StateFile)
	if err != nil {
		if ui != nil {
			ui.Warn("Ignoring unreadable state file %s during clean: %v", filepath.ToSlash(paths.StateFile), err)
		}
		stateExisted = false
	}
	if stateExisted {
		dirty := false
		for tid := range state.Binaries {
			for _, engine := range workspace.AllDecompilers() {
				if state.ClearAnalyzed(tid, engine) {
					dirty = true
					removed = true
				}
			}
		}
		if dirty {
			if len(state.Tools) == 0 && len(state.Binaries) == 0 {
				if _, err := removeIfExistsCounted(paths.StateFile); err != nil {
					return removed, fmt.Errorf("remove state %s: %w", paths.StateFile, err)
				}
			} else if err := workspace.SaveState(paths.StateFile, state); err != nil {
				return removed, fmt.Errorf("save state: %w", err)
			}
		}
	}

	if removed && ui != nil {
		ui.Info("Cleared analyze databases, logs, and analyzed state checkpoints.")
	}
	return removed, nil
}

// clearPlannedTargets clears the database, analyze log, and analyzed hash for each planned target.
func clearPlannedTargets(paths workspace.Paths, state *workspace.State, engine workspace.DecompilerID, items []PlanItem, ui *term.UI) error {
	dirty := false
	for _, item := range items {
		if err := os.RemoveAll(item.DatabaseDir); err != nil {
			return fmt.Errorf("clear database dir %s: %w", item.DatabaseDir, err)
		}
		if err := os.Remove(item.LogPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear log file %s: %w", item.LogPath, err)
		}
		if state.ClearAnalyzed(item.Target, engine) {
			dirty = true
		}
		if ui != nil {
			ui.Info("Cleared existing %s database and log for %s", engine, item.Target)
		}
	}
	if dirty {
		if err := workspace.SaveState(paths.StateFile, *state); err != nil {
			return fmt.Errorf("save state after clear: %w", err)
		}
	}
	return nil
}

func removeIfExistsCounted(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
