package cmd

import (
	"context"
	"fmt"

	"twdll/cli/internal/analyze"
	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

type CleanOptions struct {
	Yes bool
}

// CleanStage represents one pipeline stage's workspace cleaner.
type CleanStage struct {
	Name  string
	Clean func(ctx context.Context, paths workspace.Paths, ui *term.UI) (bool, error)
}

type CleanRequest struct {
	Layout   workspace.Layout
	Options  CleanOptions
	Prompter term.Prompter
	UI       *term.UI
	Stages   []CleanStage
}

// DefaultCleanStages returns the registered pipeline cleaners in reverse
// dependency order (downstream outputs first, init artifacts last).
func DefaultCleanStages() []CleanStage {
	return []CleanStage{
		{
			Name: "extract",
			Clean: func(_ context.Context, paths workspace.Paths, _ *term.UI) (bool, error) {
				return extractdb.CleanWorkspace(paths)
			},
		},
		{
			Name: "analyze",
			Clean: func(_ context.Context, paths workspace.Paths, ui *term.UI) (bool, error) {
				return analyze.CleanWorkspace(paths, ui)
			},
		},
		{
			Name: "init",
			Clean: func(_ context.Context, paths workspace.Paths, ui *term.UI) (bool, error) {
				return cleanInitWorkspace(paths, ui)
			},
		},
	}
}

func RunClean(ctx context.Context, req CleanRequest) error {
	ui := req.UI
	if !req.Options.Yes {
		if req.Prompter == nil {
			return &term.DiagnosticError{
				Message:  "clean wipes the entire workspace and requires confirmation",
				Expected: "`--yes` (`-y`) flag or interactive confirmation",
				Help: []string{
					"Run `twdll clean --yes` (`-y`) to wipe all workspace artifacts without prompting.",
					"To reset a single stage, run `twdll init --clear`, `twdll analyze --clear`, or `twdll extract --clear`.",
				},
			}
		}
		ok, err := req.Prompter.Confirm(
			"Wipe entire workspace (config, binaries, local tools, decompiler databases, extracted SQLite DBs, and logs)?",
			false,
		)
		if err != nil {
			return err
		}
		if !ok {
			if ui != nil {
				ui.Info("Clean cancelled.")
			}
			return nil
		}
	}

	stages := req.Stages
	if len(stages) == 0 {
		stages = DefaultCleanStages()
	}

	hadArtifacts := false
	for _, stage := range stages {
		if stage.Clean == nil {
			continue
		}
		removed, err := stage.Clean(ctx, req.Layout.Paths, ui)
		if err != nil {
			return fmt.Errorf("clean %s: %w", stage.Name, err)
		}
		if removed {
			hadArtifacts = true
		}
	}

	if ui != nil {
		if hadArtifacts {
			ui.Success("Wiped workspace configuration, binaries, local tools, databases, and logs.")
		} else {
			ui.Info("Workspace is already clean.")
		}
	}
	return nil
}
