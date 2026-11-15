package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"twdll/cli/internal/binaries"
	"twdll/cli/internal/term"
	"twdll/cli/internal/toolchain"
	"twdll/cli/internal/workspace"
)

type InitOptions struct {
	Clear       bool
	NoInput     bool
	DefaultGame string
	Decompiler  string
	IDADir      string
	AttilaDir   string
	Rome2Dir    string
	ToBDir      string
	Attila64Src string
}

type InitRequest struct {
	Layout       workspace.Layout
	Options      InitOptions
	Prompter     term.Prompter
	OpenSteamCmd func() error
	UI           *term.UI
}

func RunInit(ctx context.Context, req InitRequest) error {
	paths := req.Layout.Paths
	ui := req.UI
	opts := req.Options

	var prompter term.Prompter
	if !opts.NoInput {
		prompter = req.Prompter
	}

	openSteam := req.OpenSteamCmd
	if openSteam == nil {
		openSteam = binaries.OpenSteamDepotConsole
	}

	preservedDataDir := req.Layout.Config.DataDir
	var (
		state workspace.State
		err   error
	)
	if opts.Clear {
		state, err = clearInitArtifacts(paths, ui)
		if err != nil {
			return err
		}
	} else {
		state, _, err = workspace.LoadState(paths.StateFile)
		if err != nil {
			state = workspace.NewState()
		}
	}

	if err := paths.EnsureInitDirs(); err != nil {
		return fmt.Errorf("create workspace dirs: %w", err)
	}

	cfg := req.Layout.Config
	if opts.Clear {
		cfg = workspace.DefaultConfig()
		cfg.DataDir = preservedDataDir
		if preservedDataDir != "" {
			if err := workspace.SaveConfig(paths.Config, cfg); err != nil {
				return fmt.Errorf("preserve sandbox data_dir config: %w", err)
			}
		}
	}

	steam, _ := binaries.LocateSteam()
	if err := applyExplicitInitFlags(&cfg, opts); err != nil {
		return err
	}
	autoDiscoverDefaults(&cfg, steam)

	if prompter != nil {
		if err := runInteractiveWizard(&cfg, opts, prompter, ui); err != nil {
			return err
		}
		if err := workspace.SaveConfig(paths.Config, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}

	if err := stepInstallBuildTools(ctx, paths, &state, ui); err != nil {
		return err
	}
	if err := workspace.SaveState(paths.StateFile, state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if err := stepStageGameBinaries(paths, &cfg, &state, opts, prompter, ui); err != nil {
		return err
	}
	if err := workspace.SaveConfig(paths.Config, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := workspace.SaveState(paths.StateFile, state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	refReq := refBinaryRequest{
		paths:     paths,
		state:     &state,
		srcFlag:   opts.Attila64Src,
		steam:     steam,
		prompter:  prompter,
		openSteam: openSteam,
		ui:        ui,
	}
	if err := stepStageAttila64(refReq); err != nil {
		return err
	}
	if err := workspace.SaveState(paths.StateFile, state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if err := stepSetupDecompiler(ctx, paths, &cfg, &state, ui); err != nil {
		return err
	}
	if err := workspace.SaveConfig(paths.Config, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := workspace.SaveState(paths.StateFile, state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if ui != nil {
		ui.Success("Workspace initialized at %s (next: run `twdll analyze`)", filepath.ToSlash(paths.DataDir))
	}
	return nil
}

func applyExplicitInitFlags(cfg *workspace.Config, opts InitOptions) error {
	if opts.DefaultGame != "" {
		g, ok := workspace.ParseGameID(opts.DefaultGame)
		if !ok {
			return fmt.Errorf("invalid default game %q (expected attila, rome2, or tob)", opts.DefaultGame)
		}
		cfg.DefaultGame = g
	}
	if opts.Decompiler != "" {
		d, ok := workspace.ParseDecompilerID(opts.Decompiler)
		if !ok {
			return fmt.Errorf("invalid decompiler %q (expected ghidra or ida)", opts.Decompiler)
		}
		cfg.Decompiler = d
	}
	if opts.IDADir != "" {
		cfg.IDADir = opts.IDADir
	}
	if opts.AttilaDir != "" {
		cfg.Games.Set(workspace.GameAttila, opts.AttilaDir)
	}
	if opts.Rome2Dir != "" {
		cfg.Games.Set(workspace.GameRome2, opts.Rome2Dir)
	}
	if opts.ToBDir != "" {
		cfg.Games.Set(workspace.GameToB, opts.ToBDir)
	}
	return nil
}

func autoDiscoverDefaults(cfg *workspace.Config, steam *binaries.SteamDir) {
	if steam != nil {
		for _, g := range workspace.AllGames() {
			if cfg.Games.Get(g) == "" {
				if dir, ok := steam.FindGameDir(g); ok {
					cfg.Games.Set(g, dir)
				}
			}
		}
	}
	if cfg.IDADir == "" {
		cfg.IDADir = toolchain.AutoDetectIDADir()
	}
}

func runInteractiveWizard(cfg *workspace.Config, opts InitOptions, prompter term.Prompter, ui *term.UI) error {
	if opts.DefaultGame == "" {
		choices := []string{"attila", "rome2", "tob"}
		defIdx := 0
		for i, c := range choices {
			if c == string(cfg.DefaultGame) {
				defIdx = i
				break
			}
		}
		idx, err := prompter.Select("Select default Total War game", choices, defIdx)
		if err != nil {
			return err
		}
		cfg.DefaultGame = workspace.GameID(choices[idx])
	}

	if opts.Decompiler == "" {
		choices := []string{"ghidra", "ida"}
		defIdx := 0
		if cfg.Decompiler == workspace.DecompilerIDA {
			defIdx = 1
		}
		idx, err := prompter.Select("Select decompiler backend", choices, defIdx)
		if err != nil {
			return err
		}
		cfg.Decompiler = workspace.DecompilerID(choices[idx])
	}

	if cfg.Decompiler == workspace.DecompilerIDA && opts.IDADir == "" {
		for {
			dir, err := prompter.Input("IDA Pro installation directory", cfg.IDADir)
			if err != nil {
				return err
			}
			if vErr := toolchain.ValidateIDADir(dir); vErr != nil {
				if ui != nil {
					ui.Warn("Invalid IDA directory: %v", vErr)
				}
				continue
			}
			cfg.IDADir = dir
			break
		}
	}
	return nil
}
