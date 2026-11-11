package analyze

import (
	"fmt"
	"os"
	"strings"

	"twdll/cli/internal/binaries"
	"twdll/cli/internal/workspace"
)

// BuildPlan resolves all targets to analyze and checks if they can be skipped.
func BuildPlan(cfg workspace.Config, state *workspace.State, paths workspace.Paths, opts Options) ([]Item, error) {
	var targetIDs []workspace.TargetID

	if len(opts.Targets) > 0 {
		for _, raw := range opts.Targets {
			tid := workspace.TargetID(strings.ToLower(strings.TrimSpace(raw)))
			if !tid.IsValid() {
				return nil, fmt.Errorf("unknown target: %q (valid targets: %v)", raw, workspace.AllTargets())
			}
			targetIDs = append(targetIDs, tid)
		}
	} else {
		// Default to all 32-bit games configured, plus attila_64 if present
		for _, g := range workspace.AllGames() {
			if cfg.Games.Get(g) != "" {
				targetIDs = append(targetIDs, g.Target32())
			}
		}
		// Also include attila_64 if present
		if _, err := os.Stat(paths.BinaryPath(workspace.TargetAttila64)); err == nil {
			targetIDs = append([]workspace.TargetID{workspace.TargetAttila64}, targetIDs...)
		}
	}

	var items []Item
	for _, tid := range targetIDs {
		binPath := paths.BinaryPath(tid)
		stat, err := os.Stat(binPath)
		if err != nil || stat.IsDir() {
			return nil, fmt.Errorf("binary for %s not found at %s (run `twdll init` to acquire)", tid, binPath)
		}

		// Compute or retrieve binary SHA256
		binSHA := ""
		if saved, ok := state.Binaries[tid]; ok && saved.BinarySHA256 != "" {
			binSHA = saved.BinarySHA256
		} else {
			computed, err := binaries.ComputeFileSHA256(binPath)
			if err != nil {
				return nil, fmt.Errorf("compute sha256 for %s: %w", tid, err)
			}
			binSHA = computed
		}

		decompiler := cfg.Decompiler
		if opts.Decompiler != "" {
			d := workspace.DecompilerID(strings.ToLower(strings.TrimSpace(opts.Decompiler)))
			if !d.IsValid() {
				return nil, fmt.Errorf("unknown decompiler: %q (valid: ghidra, ida)", opts.Decompiler)
			}
			decompiler = d
		}

		outDir := paths.DatabaseDir(decompiler, tid)
		marker := paths.DatabaseMarkerPath(decompiler, tid)
		logFile := paths.AnalyzeLogPath(decompiler, tid)

		needsRun := true
		skipReason := ""

		if !opts.Force && !opts.Clean {
			if saved, ok := state.Binaries[tid]; ok {
				if saved.BinarySHA256 != "" && saved.Analyzed.Get(decompiler) == binSHA {
					if _, err := os.Stat(marker); err == nil {
						needsRun = false
						skipReason = "already analyzed (hash match)"
					}
				}
			}
		}

		items = append(items, Item{
			Target:       tid,
			Decompiler:   decompiler,
			BinaryPath:   binPath,
			BinarySHA256: binSHA,
			OutputDir:    outDir,
			MarkerFile:   marker,
			LogFile:      logFile,
			NeedsRun:     needsRun,
			SkipReason:   skipReason,
		})
	}

	return items, nil
}
