package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"twdll/cli/internal/binaries"
	"twdll/cli/internal/toolchain"
	"twdll/cli/internal/ui"
	"twdll/cli/internal/workspace"
)

type InitOptions struct {
	Clear       bool
	NoInput     bool
	DefaultGame string // "attila" | "rome2" | "tob"
	Decompiler  string // "ghidra" | "ida"
	IDADir      string
	AttilaDir   string
	Rome2Dir    string
	ToBDir      string
	Attila64Src string
}

// RunInit executes the 5-step idempotent setup flow for toolchains, games, and decompiler.
func RunInit(ctx context.Context, paths workspace.Paths, opts InitOptions, prompter ui.Prompter, printer *ui.Printer) error {
	if opts.Clear {
		if printer != nil {
			printer.Info("Clearing existing workspace state and tools...")
		}
		_ = os.Remove(paths.Config)
		_ = os.Remove(paths.StateFile)

		// Clear tools but preserve the local Go toolchain so twdll can always compile itself
		if entries, err := os.ReadDir(paths.ToolsDir); err == nil {
			for _, e := range entries {
				if e.Name() != "go" {
					_ = os.RemoveAll(filepath.Join(paths.ToolsDir, e.Name()))
				}
			}
		}

		_ = os.RemoveAll(paths.BinsDir)
		_ = os.RemoveAll(paths.DBDir)
		_ = os.RemoveAll(paths.LogsDir)
		if printer != nil {
			printer.Success("Workspace cleared. Starting fresh initialization.")
			printer.Info("")
		}
	}

	if err := paths.EnsureBaseDirs(); err != nil {
		return fmt.Errorf("ensure .twdll base dirs: %w", err)
	}

	cfg, _, err := workspace.LoadConfig(paths.Config)
	if err != nil || opts.Clear {
		cfg = workspace.DefaultConfig()
	}

	state, _, err := workspace.LoadState(paths.StateFile)
	if err != nil || opts.Clear {
		state = workspace.NewState()
	}

	// Apply option overrides to config
	if opts.DefaultGame != "" {
		cfg.DefaultGame = workspace.GameID(opts.DefaultGame)
	}
	if opts.Decompiler != "" {
		cfg.Decompiler = workspace.DecompilerID(opts.Decompiler)
	}
	if opts.IDADir != "" {
		cfg.IDADir = opts.IDADir
	} else if cfg.IDADir == "" {
		cfg.IDADir = toolchain.AutoDetectIDADir()
	}

	// Interactive prompts if running in TTY and input not disabled
	if prompter != nil && !opts.NoInput {
		gameChoices := []string{"attila", "rome2", "tob"}
		defaultIdx := 0
		for i, g := range gameChoices {
			if g == string(cfg.DefaultGame) {
				defaultIdx = i
				break
			}
		}
		if selected, err := prompter.Select("Select default game", gameChoices, defaultIdx); err == nil {
			cfg.DefaultGame = workspace.GameID(gameChoices[selected])
		}
		printer.Info("")

		decompChoices := []string{"ghidra", "ida"}
		defaultDecompIdx := 0
		for i, d := range decompChoices {
			if d == string(cfg.Decompiler) {
				defaultDecompIdx = i
				break
			}
		}
		if selected, err := prompter.Select("Select decompiler backend", decompChoices, defaultDecompIdx); err == nil {
			cfg.Decompiler = workspace.DecompilerID(decompChoices[selected])
		}
		printer.Info("")

		if cfg.Decompiler == workspace.DecompilerIDA {
			if cfg.IDADir == "" {
				cfg.IDADir = toolchain.AutoDetectIDADir()
			}
			for {
				ans, err := prompter.Input("Path to IDA Pro installation directory", cfg.IDADir)
				if err != nil {
					break
				}
				if ans == "" {
					ans = cfg.IDADir
				}
				if err := toolchain.ValidateIDADir(ans); err != nil {
					printer.Error(fmt.Errorf("invalid IDA Pro directory: %w (please re-enter)", err))
					continue
				}
				cfg.IDADir = ans
				break
			}
			printer.Info("")
		}
	}

	printer.Info("==================================================")
	printer.Info("  twdll Initialization — Stage 1 Setup Pipeline")
	printer.Info("==================================================")
	printer.Info("")

	// ----------------------------------------------------
	// STEP 1: Portable Build & Packaging Toolchain
	// ----------------------------------------------------
	printer.Info("[Step 1/5] Setting up portable build toolchain...")
	manifest := toolchain.CurrentPlatformManifest()

	// Install core build tools
	buildToolIDs := []workspace.ToolID{
		workspace.ToolCMake,
		workspace.ToolNinja,
		workspace.ToolXwin,
		workspace.ToolRPFM,
	}

	for _, toolID := range buildToolIDs {
		tool, ok := manifest[toolID]
		if !ok {
			continue
		}
		if err := toolchain.InstallTool(ctx, paths, tool, printer); err != nil {
			return fmt.Errorf("install tool %s: %w", tool.Name, err)
		}
	}

	// Provision xwin 32-bit CRT and Windows SDK
	if err := toolchain.ProvisionXWin(ctx, paths, printer); err != nil {
		return fmt.Errorf("provision xwin sdk: %w", err)
	}
	printer.Success("[Step 1/5] Build toolchain ready.")
	printer.Info("")

	// ----------------------------------------------------
	// STEP 2: 32-bit Target Games
	// ----------------------------------------------------
	printer.Info("[Step 2/5] Locating and verifying 32-bit target games...")
	steamDir, _ := binaries.Locate()

	gameSetups := []struct {
		gameID   workspace.GameID
		targetID workspace.TargetID
		dirOpt   string
	}{
		{workspace.GameAttila, workspace.TargetAttila32, opts.AttilaDir},
		{workspace.GameRome2, workspace.TargetRome2_32, opts.Rome2Dir},
		{workspace.GameToB, workspace.TargetToB32, opts.ToBDir},
	}

	gamesConfigured := 0
	for _, gs := range gameSetups {
		gameDir := gs.dirOpt
		if gameDir == "" {
			gameDir = cfg.Games.Get(gs.gameID)
		}
		if gameDir == "" && steamDir != nil {
			if found, ok := steamDir.FindGameDir(gs.gameID); ok {
				gameDir = found
			}
		}

		if gameDir == "" {
			continue
		}

		dllSource := filepath.Join(gameDir, "empire.retail.dll")
		if _, err := os.Stat(dllSource); err != nil {
			continue
		}

		peInfo, err := binaries.VerifyPE32(dllSource, gs.targetID)
		if err != nil {
			continue
		}

		destDLL := filepath.Join(paths.BinaryDir(gs.targetID), "empire.retail.dll")
		if err := copyFile(dllSource, destDLL); err != nil {
			return fmt.Errorf("copy %s dll to workspace: %w", gs.gameID, err)
		}

		cfg.Games.Set(gs.gameID, gameDir)
		state.SetBinary(gs.targetID, workspace.BinaryState{
			SourcePath:   dllSource,
			BinarySHA256: peInfo.SHA256,
		})
		gamesConfigured++
		printer.Success("Verified & staged %s 32-bit (SHA: %s...)", gs.gameID, peInfo.SHA256[:12])
	}

	if gamesConfigured == 0 {
		return &ui.DiagnosticError{
			Message:  "no Total War game installations found",
			Expected: "valid installation directory for Attila, Rome II, or Thrones of Britannia containing empire.retail.dll",
			Help: []string{
				"pass game directory via flag: twdll init --attila-dir \"<path-to-attila>\"",
				"or configure path in twdll.toml under [games]",
			},
		}
	}
	printer.Success("[Step 2/5] 32-bit game binaries verified.")
	printer.Info("")

	// ----------------------------------------------------
	// STEP 3: Reference Binary (attila_64)
	// ----------------------------------------------------
	printer.Info("[Step 3/5] Verifying 64-bit Linux DWARF reference binary (attila_64)...")
	destAttila64 := filepath.Join(paths.BinaryDir(workspace.TargetAttila64), "Attila")

	attila64Found := false
	if _, err := os.Stat(destAttila64); err == nil {
		if elfInfo, err := binaries.VerifyELF64DWARF(destAttila64); err == nil {
			attila64Found = true
			state.SetBinary(workspace.TargetAttila64, workspace.BinaryState{
				SourcePath:   destAttila64,
				BinarySHA256: elfInfo.SHA256,
			})
			printer.Info("Existing attila_64 verified in workspace (SHA: %s...).", elfInfo.SHA256[:12])
		}
	}

	if !attila64Found {
		sourcePath := opts.Attila64Src
		if sourcePath == "" && steamDir != nil {
			if depot, ok := steamDir.FindAttila64Depot(); ok {
				sourcePath = depot
			}
		}

		if sourcePath != "" {
			if stat, err := os.Stat(sourcePath); err == nil && !stat.IsDir() {
				if elfInfo, err := binaries.VerifyELF64DWARF(sourcePath); err == nil {
					if err := copyFile(sourcePath, destAttila64); err == nil {
						attila64Found = true
						state.SetBinary(workspace.TargetAttila64, workspace.BinaryState{
							SourcePath:   sourcePath,
							BinarySHA256: elfInfo.SHA256,
						})
						printer.Success("Verified & staged attila_64 from %s (SHA: %s...)", sourcePath, elfInfo.SHA256[:12])
					}
				}
			}
		}
	}

	if !attila64Found {
		// Trigger Steam console download
		_ = binaries.TriggerDepotDownload()

		return &ui.DiagnosticError{
			Message:  "missing reference binary (Attila 64-bit Linux DWARF)",
			Expected: "Linux 64-bit Attila executable with DWARF debug symbols",
			Help: []string{
				fmt.Sprintf("Steam download command copied to clipboard: %s", binaries.DepotCommand()),
				"Steam console opened (steam://nav/console) — paste and run the command to download the depot",
				"Once finished, re-run `twdll init` or pass `--attila-64 <path>`",
			},
		}
	}
	printer.Success("[Step 3/5] Reference binary attila_64 verified.")
	printer.Info("")

	// ----------------------------------------------------
	// STEP 4: Decompiler Toolchain Setup
	// ----------------------------------------------------
	printer.Info("[Step 4/5] Setting up decompiler (%s)...", cfg.Decompiler)
	if cfg.Decompiler == workspace.DecompilerGhidra {
		// Install OpenJDK
		if jdk, ok := manifest[workspace.ToolJDK]; ok {
			if err := toolchain.InstallTool(ctx, paths, jdk, printer); err != nil {
				return fmt.Errorf("install openjdk: %w", err)
			}
		}
		// Install Ghidra
		if ghidra, ok := manifest[workspace.ToolGhidra]; ok {
			if err := toolchain.InstallTool(ctx, paths, ghidra, printer); err != nil {
				return fmt.Errorf("install ghidra: %w", err)
			}
		}
		printer.Success("[Step 4/5] Ghidra decompiler toolchain ready.")
	} else if cfg.Decompiler == workspace.DecompilerIDA {
		if err := toolchain.ValidateIDADir(cfg.IDADir); err != nil {
			return &ui.DiagnosticError{
				Message:  fmt.Sprintf("invalid IDA Pro directory %q", cfg.IDADir),
				Expected: "directory containing idat.exe or idat64.exe",
				Help: []string{
					"pass IDA directory via flag: twdll init --ida-dir \"C:\\Program Files\\IDA Pro 9.0\"",
					"or configure ida_dir in twdll.toml",
				},
			}
		}
		printer.Success("[Step 4/5] IDA Pro directory verified at %s.", cfg.IDADir)
	}
	printer.Info("")

	// ----------------------------------------------------
	// STEP 5: Save State & Report
	// ----------------------------------------------------
	printer.Info("[Step 5/5] Saving configuration and state...")
	if err := workspace.SaveConfig(paths.Config, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := workspace.SaveState(paths.StateFile, state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	printer.Success("==================================================")
	printer.Success("  Initialization complete! Workspace ready.")
	printer.Success("==================================================")
	printer.Info("Next steps:")
	printer.Info("  - Run `twdll status` to inspect workspace components.")
	printer.Info("  - Run `twdll analyze` to build headless decompiler databases.")

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
