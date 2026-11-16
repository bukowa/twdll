package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"twdll/cli/internal/binaries"
	"twdll/cli/internal/term"
	"twdll/cli/internal/toolchain"
	"twdll/cli/internal/workspace"
)

type InitOptions struct {
	Clear       bool
	NoInput     bool
	JSON        bool
	Games       []string
	DefaultGame string
	Decompiler  string
	IDADir      string
	AttilaDir   string
	Rome2Dir    string
	ToBDir      string
	Attila64Src string
}

type InitReport struct {
	Initialized    bool                 `json:"initialized"`
	ConfigPath     string               `json:"config_path"`
	DataDir        string               `json:"data_dir"`
	DefaultGame    string               `json:"default_game"`
	Decompiler     string               `json:"decompiler"`
	IDADir         string               `json:"ida_dir,omitempty"`
	EnabledGames   []workspace.GameID   `json:"enabled_games"`
	EnabledTargets []workspace.TargetID `json:"enabled_targets"`
	Binaries       []BinaryStatus       `json:"binaries"`
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
	discoveredSteam := discoverSteamGames(steam)
	if cfg.IDADir == "" {
		cfg.IDADir = toolchain.AutoDetectIDADir()
	}

	enabledGames, err := resolveEnabledGames(&cfg, opts, discoveredSteam, prompter)
	if err != nil {
		return err
	}

	if prompter != nil {
		if err := runInteractiveWizard(&cfg, opts, enabledGames, prompter, ui); err != nil {
			return err
		}
		if err := workspace.SaveConfig(paths.Config, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}

	if err := pruneDisabledGameBinaries(paths, &state, enabledGames); err != nil {
		return err
	}

	if err := stepInstallBuildTools(ctx, paths, &state, ui); err != nil {
		return err
	}
	if err := workspace.SaveState(paths.StateFile, state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if err := stepStageGameBinaries(paths, &cfg, &state, opts, enabledGames, prompter, ui); err != nil {
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
		if opts.JSON {
			report := buildInitReport(paths, cfg, state, enabledGames)
			data, mErr := json.MarshalIndent(report, "", "  ")
			if mErr != nil {
				return fmt.Errorf("encode json init report: %w", mErr)
			}
			ui.WriteData("%s\n", string(data))
		}
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

func discoverSteamGames(steam *binaries.SteamDir) map[workspace.GameID]string {
	found := make(map[workspace.GameID]string, 3)
	if steam == nil {
		return found
	}
	for _, g := range workspace.AllGames() {
		if dir, ok := steam.FindGameDir(g); ok {
			found[g] = dir
		}
	}
	return found
}

func parseInitGames(raw []string) ([]workspace.GameID, error) {
	seen := make(map[workspace.GameID]bool, 3)
	var out []workspace.GameID
	for _, entry := range raw {
		for _, tok := range strings.Split(entry, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			g, ok := workspace.ParseGameID(tok)
			if !ok {
				return nil, &term.DiagnosticError{
					Message:  fmt.Sprintf("invalid game %q in --games", tok),
					Expected: "comma-separated subset of: attila, rome2, tob",
					Help:     []string{"example: twdll init --games attila,rome2"},
				}
			}
			if !seen[g] {
				seen[g] = true
				out = append(out, g)
			}
		}
	}
	if len(out) == 0 {
		return nil, &term.DiagnosticError{
			Message:  "--games cannot be empty",
			Expected: "at least one game from: attila, rome2, tob",
			Help:     []string{"example: twdll init --games attila"},
		}
	}
	return out, nil
}

func resolveEnabledGames(
	cfg *workspace.Config,
	opts InitOptions,
	discovered map[workspace.GameID]string,
	prompter term.Prompter,
) ([]workspace.GameID, error) {
	explicitDirs := map[workspace.GameID]string{
		workspace.GameAttila: opts.AttilaDir,
		workspace.GameRome2:  opts.Rome2Dir,
		workspace.GameToB:    opts.ToBDir,
	}

	var enabled []workspace.GameID
	switch {
	case len(opts.Games) > 0:
		parsed, err := parseInitGames(opts.Games)
		if err != nil {
			return nil, err
		}
		enabledSet := make(map[workspace.GameID]bool, len(parsed))
		for _, g := range parsed {
			enabledSet[g] = true
		}
		for _, g := range workspace.AllGames() {
			if explicitDirs[g] != "" && !enabledSet[g] {
				return nil, &term.DiagnosticError{
					Message:  fmt.Sprintf("conflicting flags: --%s-dir was set while %q was excluded from --games", g, g),
					Expected: fmt.Sprintf("include %q in --games or omit --%s-dir", g, g),
					Help:     []string{"example: twdll init --games attila,rome2"},
				}
			}
		}
		enabled = parsed

	case prompter != nil:
		anyConfigured := false
		for _, g := range workspace.AllGames() {
			if cfg.Games.Get(g) != "" {
				anyConfigured = true
				break
			}
		}
		defaultGame := cfg.DefaultGame
		if !defaultGame.IsValid() {
			defaultGame = workspace.GameAttila
		}
		allGames := workspace.AllGames()
		multiOpts := make([]term.MultiSelectOption, 0, len(allGames))
		for _, g := range allGames {
			hint := "not found"
			if cfg.Games.Get(g) != "" {
				hint = "configured"
			} else if discovered[g] != "" {
				hint = "found on Steam"
			}
			checked := false
			if anyConfigured {
				checked = cfg.Games.Get(g) != ""
			} else {
				checked = g == defaultGame
			}
			multiOpts = append(multiOpts, term.MultiSelectOption{
				Label:    fmt.Sprintf("%-6s — %s (%s)", g, g.DisplayName(), hint),
				Selected: checked,
			})
		}
		indices, err := prompter.MultiSelect("Select 32-bit target games to enable (attila_64 is always required)", multiOpts, 1)
		if err != nil {
			return nil, err
		}
		for _, idx := range indices {
			enabled = append(enabled, allGames[idx])
		}

	default:
		for _, g := range workspace.AllGames() {
			if cfg.Games.Get(g) != "" {
				enabled = append(enabled, g)
			}
		}
		if len(enabled) == 0 {
			def := cfg.DefaultGame
			if !def.IsValid() {
				def = workspace.GameAttila
			}
			enabled = []workspace.GameID{def}
		}
	}

	enabledSet := make(map[workspace.GameID]bool, len(enabled))
	for _, g := range enabled {
		enabledSet[g] = true
		if cfg.Games.Get(g) == "" && discovered[g] != "" {
			cfg.Games.Set(g, discovered[g])
		}
	}
	for _, g := range workspace.AllGames() {
		if !enabledSet[g] {
			cfg.Games.Set(g, "")
		}
	}

	explicitSelection := len(opts.Games) > 0 || prompter != nil
	if explicitSelection && !enabledSet[cfg.DefaultGame] {
		if opts.DefaultGame != "" {
			return nil, &term.DiagnosticError{
				Message:  fmt.Sprintf("--default-game %q is not in enabled games", cfg.DefaultGame),
				Expected: "default_game must be one of the enabled 32-bit games",
				Help:     []string{"include it in --games or change --default-game"},
			}
		}
		cfg.DefaultGame = enabled[0]
	}

	return enabled, nil
}

func pruneDisabledGameBinaries(paths workspace.Paths, state *workspace.State, enabled []workspace.GameID) error {
	enabledSet := make(map[workspace.GameID]bool, len(enabled))
	for _, g := range enabled {
		enabledSet[g] = true
	}
	for _, g := range workspace.AllGames() {
		if enabledSet[g] {
			continue
		}
		tid := g.Target32()
		delete(state.Binaries, tid)
		binDir := paths.BinaryDir(tid)
		if err := os.RemoveAll(binDir); err != nil {
			return fmt.Errorf("remove disabled binary dir %s: %w", binDir, err)
		}
	}
	return nil
}

func buildInitReport(paths workspace.Paths, cfg workspace.Config, state workspace.State, enabledGames []workspace.GameID) InitReport {
	targets := make([]workspace.TargetID, 0, 1+len(enabledGames))
	targets = append(targets, workspace.TargetAttila64)
	for _, g := range enabledGames {
		targets = append(targets, g.Target32())
	}
	bins := make([]BinaryStatus, 0, len(targets))
	for _, tid := range targets {
		bins = append(bins, inspectTargetBinary(paths, state, tid))
	}
	return InitReport{
		Initialized:    true,
		ConfigPath:     filepath.ToSlash(paths.Config),
		DataDir:        filepath.ToSlash(paths.DataDir),
		DefaultGame:    string(cfg.DefaultGame),
		Decompiler:     string(cfg.Decompiler),
		IDADir:         filepath.ToSlash(cfg.IDADir),
		EnabledGames:   enabledGames,
		EnabledTargets: targets,
		Binaries:       bins,
	}
}

func runInteractiveWizard(cfg *workspace.Config, opts InitOptions, enabledGames []workspace.GameID, prompter term.Prompter, ui *term.UI) error {
	if opts.DefaultGame == "" {
		if len(enabledGames) == 1 {
			cfg.DefaultGame = enabledGames[0]
		} else {
			choices := make([]string, 0, len(enabledGames))
			defIdx := 0
			for i, g := range enabledGames {
				choices = append(choices, string(g))
				if g == cfg.DefaultGame {
					defIdx = i
				}
			}
			idx, err := prompter.Select("Select default Total War game", choices, defIdx)
			if err != nil {
				return err
			}
			cfg.DefaultGame = workspace.GameID(choices[idx])
		}
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
