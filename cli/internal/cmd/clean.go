package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

type CleanOptions struct {
	Decompiler string
	All        bool
	Targets    []string
}

type CleanRequest struct {
	Layout  workspace.Layout
	Options CleanOptions
	UI      *term.UI
}

func RunClean(_ context.Context, req CleanRequest) error {
	paths := req.Layout.Paths
	opts := req.Options
	ui := req.UI

	if opts.All && (strings.TrimSpace(opts.Decompiler) != "" || len(opts.Targets) > 0) {
		return &term.DiagnosticError{
			Message:  "--all wipes the entire workspace and cannot be combined with --decompiler or target arguments",
			Expected: "either `twdll clean --all` alone or scoped `twdll clean [-d <engine>] [<targets>...]`",
			Help: []string{
				"Run `twdll clean -d <engine> [<targets>...]` to remove specific decompiler databases and logs.",
				"Run `twdll clean --all` without extra arguments to wipe the entire workspace.",
			},
		}
	}

	if opts.All {
		return runCleanAll(paths, ui)
	}

	engines, err := resolveCleanEngines(opts.Decompiler)
	if err != nil {
		return err
	}
	targets, err := resolveCleanTargets(opts.Targets)
	if err != nil {
		return err
	}
	fullAnalyzeScope := len(engines) == len(workspace.AllDecompilers()) && len(opts.Targets) == 0

	state, stateExisted, err := workspace.LoadState(paths.StateFile)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	cleared := 0
	stateDirty := false
	for _, tid := range targets {
		for _, engine := range engines {
			dbDir := paths.DatabaseDir(engine, tid)
			logFile := paths.AnalyzeLogPath(engine, tid)
			removed := false

			if _, sErr := os.Stat(dbDir); sErr == nil {
				if rErr := os.RemoveAll(dbDir); rErr != nil {
					return fmt.Errorf("remove database dir %s: %w", dbDir, rErr)
				}
				_ = os.Remove(filepath.Dir(dbDir))
				removed = true
			}
			if _, sErr := os.Stat(logFile); sErr == nil {
				if rErr := removeFileIfExists(logFile); rErr != nil {
					return fmt.Errorf("remove log file %s: %w", logFile, rErr)
				}
				removed = true
			}
			if binState, ok := state.Binaries[tid]; ok && binState.Analyzed.Get(engine) != "" {
				binState.Analyzed.Set(engine, "")
				if binState.SourcePath == "" && binState.BinarySHA256 == "" && binState.Analyzed.Ghidra == "" && binState.Analyzed.IDA == "" {
					delete(state.Binaries, tid)
				} else {
					state.Binaries[tid] = binState
				}
				stateDirty = true
				removed = true
			}

			if removed {
				cleared++
				if ui != nil {
					ui.Info("Removed %s database for %s", engine, tid)
				}
			}
		}
	}

	if fullAnalyzeScope {
		resLog := paths.AnalyzeResourceLogPath()
		if _, sErr := os.Stat(resLog); sErr == nil {
			if rErr := removeFileIfExists(resLog); rErr != nil {
				return fmt.Errorf("remove resource log %s: %w", resLog, rErr)
			}
			cleared++
			if ui != nil {
				ui.Info("Removed analysis resource log %s", filepath.ToSlash(resLog))
			}
		}
		for _, engine := range engines {
			_ = os.Remove(filepath.Join(paths.DBDir, string(engine)))
		}
		_ = os.Remove(paths.DBDir)
		_ = os.Remove(paths.LogsDir)
	}

	if stateDirty {
		if stateExisted && len(state.Tools) == 0 && len(state.Binaries) == 0 {
			if err := removeFileIfExists(paths.StateFile); err != nil {
				return fmt.Errorf("remove state: %w", err)
			}
		} else {
			if err := workspace.SaveState(paths.StateFile, state); err != nil {
				return fmt.Errorf("save state: %w", err)
			}
		}
	}

	if ui != nil {
		if cleared == 0 {
			ui.Info("No databases needed cleaning.")
		} else {
			ui.Success("Cleared %d database(s).", cleared)
		}
	}
	return nil
}

func runCleanAll(paths workspace.Paths, ui *term.UI) error {
	hadArtifacts := false
	for _, p := range []string{paths.Config, paths.StateFile, paths.BinsDir, paths.DBDir, paths.ExtractedDir, paths.LogsDir} {
		if _, err := os.Stat(p); err == nil {
			hadArtifacts = true
			break
		}
	}
	if !hadArtifacts && !paths.HasExternalToolsDir() {
		if entries, err := os.ReadDir(paths.ToolsDir); err == nil {
			for _, e := range entries {
				if e.Name() != string(workspace.ToolGo) {
					hadArtifacts = true
					break
				}
			}
		}
	}

	if err := removeFileIfExists(paths.Config); err != nil {
		return fmt.Errorf("remove config %s: %w", paths.Config, err)
	}
	if err := removeFileIfExists(paths.StateFile); err != nil {
		return fmt.Errorf("remove state %s: %w", paths.StateFile, err)
	}
	if err := cleanLocalTools(paths); err != nil {
		return fmt.Errorf("clean local tools in %s: %w", paths.ToolsDir, err)
	}
	if !paths.HasExternalToolsDir() {
		_ = os.Remove(paths.ToolsDir)
	}
	for _, dir := range []string{paths.BinsDir, paths.DBDir, paths.ExtractedDir, paths.LogsDir} {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove %s: %w", dir, err)
		}
	}
	_ = os.Remove(paths.DataDir)

	if ui != nil {
		if hadArtifacts {
			ui.Success("Wiped workspace configuration, binaries, local tools, databases, and logs.")
		} else {
			ui.Info("Workspace is already clean.")
		}
	}
	return nil
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

func clearInitArtifacts(paths workspace.Paths, ui *term.UI) (workspace.State, error) {
	if ui != nil {
		ui.Info("Clearing initialized configuration, binaries, and local tools in %s...", filepath.ToSlash(paths.DataDir))
	}
	sp := ui.StartSpinner("Clearing initialized workspace artifacts")
	defer sp.Stop()

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

	nextState := workspace.NewState()
	for tid, b := range prevState.Binaries {
		if b.Analyzed.Ghidra != "" || b.Analyzed.IDA != "" {
			nextState.Binaries[tid] = workspace.BinaryState{
				Analyzed: b.Analyzed,
			}
		}
	}

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

func resolveCleanEngines(raw string) ([]workspace.DecompilerID, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return workspace.AllDecompilers(), nil
	default:
		d, ok := workspace.ParseDecompilerID(raw)
		if !ok {
			return nil, fmt.Errorf("unknown decompiler %q (expected ghidra, ida, or all)", raw)
		}
		return []workspace.DecompilerID{d}, nil
	}
}

func resolveCleanTargets(rawTargets []string) ([]workspace.TargetID, error) {
	if len(rawTargets) == 0 {
		return workspace.AllTargets(), nil
	}
	seen := make(map[workspace.TargetID]bool, len(rawTargets))
	out := make([]workspace.TargetID, 0, len(rawTargets))
	for _, raw := range rawTargets {
		tid, ok := workspace.ParseTargetID(raw)
		if !ok {
			return nil, fmt.Errorf("unknown target %q (valid: %v)", raw, workspace.AllTargets())
		}
		if !seen[tid] {
			seen[tid] = true
			out = append(out, tid)
		}
	}
	return out, nil
}
