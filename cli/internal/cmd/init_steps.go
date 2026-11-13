package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"twdll/cli/internal/binaries"
	"twdll/cli/internal/term"
	"twdll/cli/internal/toolchain"
	"twdll/cli/internal/workspace"
)

func stepInstallBuildTools(ctx context.Context, paths workspace.Paths, state *workspace.State, ui *term.UI) error {
	if ui != nil {
		ui.Step(1, 4, "Setting up portable build toolchain")
	}
	manifest := toolchain.HostManifest()
	env := toolchain.InstallEnv{Paths: paths, State: state, UI: ui}

	for _, id := range toolchain.BuildToolOrder {
		def, ok := manifest[id]
		if !ok {
			continue
		}
		if err := toolchain.EnsureTool(ctx, env, def); err != nil {
			return fmt.Errorf("install %s: %w", def.Name, err)
		}
	}
	if _, hasXwin := manifest[workspace.ToolXwin]; hasXwin {
		if err := toolchain.ProvisionXWin(ctx, paths, ui); err != nil {
			return fmt.Errorf("provision xwin sdk: %w", err)
		}
	}
	return nil
}

func stepStageGameBinaries(
	paths workspace.Paths,
	cfg *workspace.Config,
	state *workspace.State,
	opts InitOptions,
	prompter term.Prompter,
	ui *term.UI,
) error {
	if ui != nil {
		ui.Step(2, 4, "Verifying 32-bit target game binaries")
	}

	explicitFlags := map[workspace.GameID]string{
		workspace.GameAttila: opts.AttilaDir,
		workspace.GameRome2:  opts.Rome2Dir,
		workspace.GameToB:    opts.ToBDir,
	}

	stagedCount := 0
	for _, g := range workspace.AllGames() {
		dir := cfg.Games.Get(g)
		if dir == "" {
			continue
		}
		target := g.Target32()
		dllPath := filepath.Join(filepath.FromSlash(dir), "empire.retail.dll")
		sp := ui.StartSpinner(fmt.Sprintf("Verifying %s", target))
		peInfo, err := binaries.VerifyPE32(dllPath, target)
		if err != nil {
			sp.Stop()
			if explicitFlags[g] != "" {
				return &term.DiagnosticError{
					Message:  fmt.Sprintf("invalid %s installation directory %q: %v", g, dir, err),
					Expected: "directory containing 32-bit x86 empire.retail.dll",
					Help:     []string{fmt.Sprintf("pass a valid path via --%s-dir <path>", g)},
				}
			}
			continue
		}
		sp.SetText(fmt.Sprintf("Staging %s", target))
		stageErr := stageVerifiedFile(dllPath, paths.BinaryPath(target), peInfo.SHA256)
		sp.Stop()
		if stageErr != nil {
			return fmt.Errorf("stage %s binary: %w", g, stageErr)
		}
		state.UpsertBinary(target, dllPath, peInfo.SHA256)
		stagedCount++
		if ui != nil {
			ui.Success("Staged %s (%s...)", target, peInfo.SHA256[:12])
		}
	}

	if stagedCount == 0 && prompter != nil {
		targetGame := cfg.DefaultGame
		if !targetGame.IsValid() {
			targetGame = workspace.GameAttila
		}
		for {
			dir, err := prompter.Input(fmt.Sprintf("Path to %s installation folder", targetGame.DisplayName()), "")
			if err != nil {
				break
			}
			dllPath := filepath.Join(filepath.FromSlash(dir), "empire.retail.dll")
			sp := ui.StartSpinner(fmt.Sprintf("Verifying %s", targetGame.Target32()))
			peInfo, vErr := binaries.VerifyPE32(dllPath, targetGame.Target32())
			if vErr != nil {
				sp.Stop()
				if ui != nil {
					ui.Warn("Could not verify empire.retail.dll in %q: %v", dir, vErr)
				}
				continue
			}
			sp.SetText(fmt.Sprintf("Staging %s", targetGame.Target32()))
			cErr := stageVerifiedFile(dllPath, paths.BinaryPath(targetGame.Target32()), peInfo.SHA256)
			sp.Stop()
			if cErr != nil {
				return cErr
			}
			cfg.Games.Set(targetGame, dir)
			state.UpsertBinary(targetGame.Target32(), dllPath, peInfo.SHA256)
			stagedCount++
			if ui != nil {
				ui.Success("Staged %s (%s...)", targetGame.Target32(), peInfo.SHA256[:12])
			}
			break
		}
	}

	if stagedCount == 0 {
		return &term.DiagnosticError{
			Message:  "no Total War game installations found",
			Expected: "at least one valid installation directory for Attila, Rome II, or Thrones of Britannia containing empire.retail.dll",
			Help: []string{
				"pass game path explicitly: twdll init --attila-dir \"<path-to-attila>\"",
				"or set [games] paths in twdll.toml",
			},
		}
	}
	return nil
}

type refBinaryRequest struct {
	paths     workspace.Paths
	state     *workspace.State
	srcFlag   string
	steam     *binaries.SteamDir
	prompter  term.Prompter
	openSteam func() error
	ui        *term.UI
}

func stepStageAttila64(req refBinaryRequest) error {
	if req.ui != nil {
		req.ui.Step(3, 4, "Verifying 64-bit Linux DWARF reference binary (attila_64)")
	}
	dest := req.paths.BinaryPath(workspace.TargetAttila64)

	if req.srcFlag == "" {
		sp := req.ui.StartSpinner("Verifying existing attila_64 (ELF64/DWARF)")
		info, err := binaries.VerifyELF64DWARF(dest)
		sp.Stop()
		if err == nil {
			sourcePath := dest
			if prev, ok := req.state.Binaries[workspace.TargetAttila64]; ok && prev.SourcePath != "" {
				sourcePath = prev.SourcePath
			}
			req.state.UpsertBinary(workspace.TargetAttila64, sourcePath, info.SHA256)
			if req.ui != nil {
				req.ui.Success("Verified existing attila_64 (%s...)", info.SHA256[:12])
			}
			return nil
		}
	}

	candidate := req.srcFlag
	if candidate == "" && req.steam != nil {
		if depot, ok := req.steam.FindAttila64Depot(); ok {
			candidate = depot
		}
	}
	if candidate != "" {
		sp := req.ui.StartSpinner("Verifying attila_64 (ELF64/DWARF)")
		info, err := binaries.VerifyELF64DWARF(candidate)
		if err != nil {
			sp.Stop()
			if req.srcFlag != "" {
				return &term.DiagnosticError{
					Message:  fmt.Sprintf("invalid attila_64 binary %q: %v", candidate, err),
					Expected: "64-bit x86-64 Linux Attila ELF containing .debug_info DWARF symbols",
					Help:     []string{"pass a valid unstripped Linux Attila ELF via --attila-64 <path>"},
				}
			}
		} else {
			sp.SetText("Staging attila_64")
			stageErr := stageVerifiedFile(candidate, dest, info.SHA256)
			sp.Stop()
			if stageErr != nil {
				return fmt.Errorf("stage attila_64: %w", stageErr)
			}
			req.state.UpsertBinary(workspace.TargetAttila64, candidate, info.SHA256)
			if req.ui != nil {
				req.ui.Success("Staged attila_64 (%s...)", info.SHA256[:12])
			}
			return nil
		}
	}

	if req.prompter != nil {
		if err := req.openSteam(); err == nil && req.ui != nil {
			req.ui.Info("Copied `%s` to clipboard and opened Steam Console.", binaries.DepotCommand())
			req.ui.Info("Paste the command into Steam Console, wait for 'Depot download complete', then press Enter below.")
		}
		defaultDepot := ""
		if req.steam != nil {
			defaultDepot = filepath.ToSlash(filepath.Join(
				filepath.FromSlash(req.steam.Path),
				"steamapps", "content", "app_"+binaries.AttilaAppID, "depot_"+binaries.AttilaDepotID, "Attila",
			))
		}
		for {
			entered, pErr := req.prompter.Input("Path to downloaded Linux Attila binary (Enter to check Steam depot)", defaultDepot)
			if pErr != nil {
				break
			}
			if entered == "" {
				continue
			}
			sp := req.ui.StartSpinner("Verifying attila_64 (ELF64/DWARF)")
			info, vErr := binaries.VerifyELF64DWARF(filepath.FromSlash(entered))
			if vErr != nil {
				sp.Stop()
				if req.ui != nil {
					req.ui.Warn("Binary not ready at %q: %v", entered, vErr)
				}
				continue
			}
			sp.SetText("Staging attila_64")
			cErr := stageVerifiedFile(filepath.FromSlash(entered), dest, info.SHA256)
			sp.Stop()
			if cErr != nil {
				return cErr
			}
			req.state.UpsertBinary(workspace.TargetAttila64, entered, info.SHA256)
			if req.ui != nil {
				req.ui.Success("Staged attila_64 (%s...)", info.SHA256[:12])
			}
			return nil
		}
	}

	return &term.DiagnosticError{
		Message:  "missing reference binary (Attila 64-bit Linux DWARF)",
		Expected: "Linux 64-bit Attila ELF executable with .debug_info DWARF symbols",
		Help: []string{
			fmt.Sprintf("run `%s` in Steam Console (steam://nav/console)", binaries.DepotCommand()),
			"once finished, re-run `twdll init` or pass `--attila-64 <path>`",
		},
	}
}

func stepSetupDecompiler(
	ctx context.Context,
	paths workspace.Paths,
	cfg *workspace.Config,
	state *workspace.State,
	ui *term.UI,
) error {
	if ui != nil {
		ui.Step(4, 4, fmt.Sprintf("Setting up decompiler toolchain (%s)", cfg.Decompiler))
	}
	switch cfg.Decompiler {
	case workspace.DecompilerGhidra:
		manifest := toolchain.HostManifest()
		env := toolchain.InstallEnv{Paths: paths, State: state, UI: ui}
		for _, id := range []workspace.ToolID{workspace.ToolJDK, workspace.ToolGhidra} {
			if def, ok := manifest[id]; ok {
				if err := toolchain.EnsureTool(ctx, env, def); err != nil {
					return fmt.Errorf("install %s: %w", def.Name, err)
				}
			}
		}
		return nil

	case workspace.DecompilerIDA:
		if err := toolchain.ValidateIDADir(cfg.IDADir); err != nil {
			return &term.DiagnosticError{
				Message:  fmt.Sprintf("invalid IDA Pro directory %q: %v", cfg.IDADir, err),
				Expected: "directory containing idat64.exe / idat.exe",
				Help: []string{
					"pass IDA path explicitly: twdll init --decompiler ida --ida-dir \"C:/Program Files/IDA Professional 9.5\"",
					"or switch to portable Ghidra: twdll init --decompiler ghidra",
				},
			}
		}
		if ui != nil {
			ui.Success("Verified IDA Pro at %s", filepath.ToSlash(cfg.IDADir))
		}
		return nil

	default:
		return fmt.Errorf("unsupported decompiler %q", cfg.Decompiler)
	}
}

func stageVerifiedFile(src, dst, expectedSHA string) error {
	cleanSrc, _ := filepath.Abs(src)
	cleanDst, _ := filepath.Abs(dst)
	if strings.EqualFold(cleanSrc, cleanDst) {
		return nil
	}
	if info, err := os.Stat(dst); err == nil && !info.IsDir() {
		if existingSHA, hErr := workspace.ComputeFileSHA256(dst); hErr == nil && strings.EqualFold(existingSHA, expectedSHA) {
			return nil
		}
	}
	return workspace.CopyFileAtomic(src, dst)
}
