package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

func cleanInitWorkspace(paths workspace.Paths, ui *term.UI) (bool, error) {
	removed := false
	for _, p := range []string{paths.Config, paths.StateFile, paths.BinsDir} {
		if _, err := os.Stat(p); err == nil {
			removed = true
			break
		}
	}
	if !removed && !paths.HasExternalToolsDir() {
		if entries, err := os.ReadDir(paths.ToolsDir); err == nil {
			for _, e := range entries {
				if e.Name() != string(workspace.ToolGo) {
					removed = true
					break
				}
			}
		}
	}

	if _, err := clearInitArtifactsQuiet(paths, ui, false); err != nil {
		return false, err
	}
	if !paths.HasExternalToolsDir() {
		_ = os.Remove(paths.ToolsDir)
	}
	_ = os.Remove(paths.DataDir)
	return removed, nil
}

func clearInitArtifacts(paths workspace.Paths, ui *term.UI) (workspace.State, error) {
	return clearInitArtifactsQuiet(paths, ui, true)
}

func clearInitArtifactsQuiet(paths workspace.Paths, ui *term.UI, showProgress bool) (workspace.State, error) {
	if showProgress && ui != nil {
		ui.Info("Clearing initialized configuration, binaries, and local tools in %s...", filepath.ToSlash(paths.DataDir))
		sp := ui.StartSpinner("Clearing initialized workspace artifacts")
		defer sp.Stop()
	}

	prevState, _, err := workspace.LoadState(paths.StateFile)
	if err != nil {
		if ui != nil {
			ui.Warn("Ignoring unreadable state file %s during --clear: %v", filepath.ToSlash(paths.StateFile), err)
		}
		prevState = workspace.NewState()
	}

	if err := removeFileIfExists(paths.Config); err != nil {
		return workspace.State{}, fmt.Errorf("remove config %s: %w", paths.Config, err)
	}
	if err := cleanLocalTools(paths); err != nil {
		return workspace.State{}, fmt.Errorf("clean local tools in %s: %w", paths.ToolsDir, err)
	}
	if err := os.RemoveAll(paths.BinsDir); err != nil {
		return workspace.State{}, fmt.Errorf("remove binaries dir %s: %w", paths.BinsDir, err)
	}

	nextState := prevState.PruneToAnalyzed()

	if len(nextState.Binaries) == 0 {
		if err := removeFileIfExists(paths.StateFile); err != nil {
			return workspace.State{}, fmt.Errorf("remove state %s: %w", paths.StateFile, err)
		}
	} else {
		if err := workspace.SaveState(paths.StateFile, nextState); err != nil {
			return workspace.State{}, fmt.Errorf("save state %s: %w", paths.StateFile, err)
		}
	}
	return nextState, nil
}

func removeFileIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func cleanLocalTools(paths workspace.Paths) error {
	if paths.HasExternalToolsDir() {
		return nil
	}
	entries, err := os.ReadDir(paths.ToolsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.Name() == string(workspace.ToolGo) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(paths.ToolsDir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
