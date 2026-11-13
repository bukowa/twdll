package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

type Options struct {
	Decompiler   string
	Clean        bool
	Force        bool
	Jobs         int
	MaxMem       string
	FullAnalysis bool
	Targets      []string
}

type Request struct {
	Layout  workspace.Layout
	Options Options
	UI      *term.UI
}

type PlanInput struct {
	Layout  workspace.Layout
	State   workspace.State
	Options Options
}

type PlanItem struct {
	Target       workspace.TargetID
	BinaryPath   string
	BinarySHA256 string
	SourcePath   string
	DatabaseDir  string
	LogPath      string
	NeedsRun     bool
	Reason       string
}

func BuildPlan(in PlanInput) (workspace.DecompilerID, []PlanItem, error) {
	if !in.Layout.ConfigExists {
		return "", nil, &term.DiagnosticError{
			Message:  fmt.Sprintf("workspace is not initialized (missing %s)", filepath.ToSlash(in.Layout.Paths.Config)),
			Expected: "an initialized twdll workspace configuration",
			Help:     []string{"Run `twdll init` to configure games, stage binaries, and install decompiler tools."},
		}
	}

	engine, err := resolveDecompiler(in.Options.Decompiler, in.Layout.Config.Decompiler)
	if err != nil {
		return "", nil, err
	}

	targets, err := resolveTargets(in.Layout, in.Options.Targets)
	if err != nil {
		return "", nil, err
	}

	paths := in.Layout.Paths
	items := make([]PlanItem, 0, len(targets))
	for _, tid := range targets {
		binPath := paths.BinaryPath(tid)
		info, sErr := os.Stat(binPath)
		if sErr != nil || info.IsDir() {
			return "", nil, &term.DiagnosticError{
				Message:  fmt.Sprintf("staged binary for %s not found at %s", tid, filepath.ToSlash(binPath)),
				Expected: fmt.Sprintf("staged %s binary in workspace", tid.BinaryFileName()),
				Help:     []string{"Run `twdll init` to verify and stage game binaries."},
			}
		}

		currentSHA, hErr := workspace.ComputeFileSHA256(binPath)
		if hErr != nil {
			return "", nil, fmt.Errorf("hash binary for %s: %w", tid, hErr)
		}

		saved := in.State.Binaries[tid]
		shaMatches := currentSHA != "" && strings.EqualFold(saved.Analyzed.Get(engine), currentSHA)
		validDB := paths.HasValidDatabase(engine, tid)
		upToDate := !in.Options.Force && !in.Options.Clean && shaMatches && validDB

		items = append(items, PlanItem{
			Target:       tid,
			BinaryPath:   binPath,
			BinarySHA256: currentSHA,
			SourcePath:   saved.SourcePath,
			DatabaseDir:  paths.DatabaseDir(engine, tid),
			LogPath:      paths.AnalyzeLogPath(engine, tid),
			NeedsRun:     !upToDate,
			Reason:       describePlanReason(in.Options, shaMatches, validDB),
		})
	}

	return engine, items, nil
}

func resolveDecompiler(override string, configured workspace.DecompilerID) (workspace.DecompilerID, error) {
	raw := strings.TrimSpace(override)
	if raw == "" {
		raw = string(configured)
	}
	engine, ok := workspace.ParseDecompilerID(raw)
	if !ok {
		return "", &term.DiagnosticError{
			Message:  fmt.Sprintf("unknown decompiler %q", raw),
			Expected: "ghidra or ida",
			Help:     []string{"Pass `--decompiler ghidra` or `--decompiler ida`, or set `decompiler` in twdll.toml."},
		}
	}
	return engine, nil
}

func resolveTargets(layout workspace.Layout, rawTargets []string) ([]workspace.TargetID, error) {
	if len(rawTargets) > 0 {
		seen := make(map[workspace.TargetID]bool, len(rawTargets))
		out := make([]workspace.TargetID, 0, len(rawTargets))
		for _, raw := range rawTargets {
			tid, ok := workspace.ParseTargetID(raw)
			if !ok {
				return nil, &term.DiagnosticError{
					Message:  fmt.Sprintf("unknown target %q", raw),
					Expected: "one of: attila_64, attila_32, rome2_32, tob_32",
					Help:     []string{"Specify valid target IDs or omit positional arguments to analyze all configured targets."},
				}
			}
			if !seen[tid] {
				seen[tid] = true
				out = append(out, tid)
			}
		}
		return out, nil
	}

	out := []workspace.TargetID{workspace.TargetAttila64}
	for _, game := range workspace.AllGames() {
		tid := game.Target32()
		configured := strings.TrimSpace(layout.Config.Games.Get(game)) != ""
		info, err := os.Stat(layout.Paths.BinaryPath(tid))
		present := err == nil && !info.IsDir()
		if configured || present {
			out = append(out, tid)
		}
	}
	return out, nil
}

func describePlanReason(opts Options, shaMatches, validDB bool) string {
	switch {
	case opts.Clean:
		return "clean requested"
	case opts.Force:
		return "force requested"
	case !validDB:
		return "database missing"
	case !shaMatches:
		return "binary changed or unanalyzed"
	default:
		return "up to date"
	}
}
