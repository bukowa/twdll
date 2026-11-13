package cmd

import (
	"context"
	"fmt"
	"os"
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

	engines, err := resolveCleanEngines(opts.Decompiler)
	if err != nil {
		return err
	}
	targets, err := resolveCleanTargets(opts.Targets)
	if err != nil {
		return err
	}

	state, _, err := workspace.LoadState(paths.StateFile)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	cleared := 0
	for _, tid := range targets {
		for _, engine := range engines {
			dbDir := paths.DatabaseDir(engine, tid)
			logFile := paths.AnalyzeLogPath(engine, tid)
			removed := false

			if _, sErr := os.Stat(dbDir); sErr == nil {
				if rErr := os.RemoveAll(dbDir); rErr == nil {
					removed = true
				}
			}
			if _, sErr := os.Stat(logFile); sErr == nil {
				if rErr := os.Remove(logFile); rErr == nil {
					removed = true
				}
			}
			if binState, ok := state.Binaries[tid]; ok && binState.Analyzed.Get(engine) != "" {
				binState.Analyzed.Set(engine, "")
				state.Binaries[tid] = binState
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

	if cleared > 0 {
		if err := workspace.SaveState(paths.StateFile, state); err != nil {
			return fmt.Errorf("save state: %w", err)
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
