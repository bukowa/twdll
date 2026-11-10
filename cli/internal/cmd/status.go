package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"twdll/cli/internal/binaries"
	"twdll/cli/internal/toolchain"
	"twdll/cli/internal/ui"
	"twdll/cli/internal/workspace"
)

type ToolStatus struct {
	ID        workspace.ToolID `json:"id"`
	Name      string           `json:"name"`
	Version   string           `json:"version"`
	Installed bool             `json:"installed"`
	Required  bool             `json:"required"`
	Path      string           `json:"path,omitempty"`
}

type BinaryStatus struct {
	Target        workspace.TargetID `json:"target"`
	Name          string             `json:"name"`
	Present       bool               `json:"present"`
	SHA256        string             `json:"sha256,omitempty"`
	KnownOfficial bool               `json:"known_official"`
	SourcePath    string             `json:"source_path,omitempty"`
	Path          string             `json:"path,omitempty"`
}

type DBStatus struct {
	Target   workspace.TargetID `json:"target"`
	Engine   string             `json:"engine"` // "ghidra" | "ida"
	Analyzed bool               `json:"analyzed"`
	Path     string             `json:"path,omitempty"`
}

type DecompilerEngineStatus struct {
	Engine    string `json:"engine"` // "ida" | "ghidra"
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	Installed bool   `json:"installed"`
	Valid     bool   `json:"valid"`
	Path      string `json:"path,omitempty"`
	Details   string `json:"details,omitempty"`
}

type StatusReport struct {
	Initialized bool                     `json:"initialized"`
	DefaultGame string                   `json:"default_game"`
	Decompiler  string                   `json:"decompiler"`
	IDADir      string                   `json:"ida_dir,omitempty"`
	IDAValid    bool                     `json:"ida_valid,omitempty"`
	Tools       []ToolStatus             `json:"tools"`
	Decompilers []DecompilerEngineStatus `json:"decompilers"`
	Binaries    []BinaryStatus           `json:"binaries"`
	Databases   []DBStatus               `json:"databases"`
}

// RunStatus inspects workspace readiness, installed tools, raw binaries, and decompiler DB states.
func RunStatus(ctx context.Context, paths workspace.Paths, jsonOut bool, printer *ui.Printer) (*StatusReport, error) {
	cfg, exists, err := workspace.LoadConfig(paths.Config)
	isInit := exists && (err == nil)
	if err != nil || !exists {
		cfg = workspace.DefaultConfig()
	}

	state, _, _ := workspace.LoadState(paths.StateFile)

	report := &StatusReport{
		Initialized: isInit,
		DefaultGame: string(cfg.DefaultGame),
		Decompiler:  string(cfg.Decompiler),
		IDADir:      cfg.IDADir,
		IDAValid:    cfg.IDADir != "" && toolchain.ValidateIDADir(cfg.IDADir) == nil,
		Tools:       make([]ToolStatus, 0),
		Decompilers: make([]DecompilerEngineStatus, 0),
		Binaries:    make([]BinaryStatus, 0),
		Databases:   make([]DBStatus, 0),
	}

	// 1. Core build toolchain inventory
	manifest := toolchain.CurrentPlatformManifest()
	for _, toolID := range toolchain.StandardToolOrder {
		tool, ok := manifest[toolID]
		if !ok {
			continue
		}
		installed := toolchain.IsToolInstalled(paths, tool)
		toolDir := paths.ToolDir(tool.ID)

		report.Tools = append(report.Tools, ToolStatus{
			ID:        tool.ID,
			Name:      tool.Name,
			Version:   tool.Version,
			Installed: installed,
			Required:  true,
			Path:      toolDir,
		})
	}

	// 2. Decompiler engines inventory
	// IDA Pro
	report.Decompilers = append(report.Decompilers, DecompilerEngineStatus{
		Engine:    "ida",
		Name:      "IDA Pro",
		Active:    cfg.Decompiler == workspace.DecompilerIDA,
		Installed: report.IDAValid,
		Valid:     report.IDAValid,
		Path:      cfg.IDADir,
	})

	// Ghidra
	ghidraDef, hasGhidra := manifest[workspace.ToolGhidra]
	jdkDef, hasJDK := manifest[workspace.ToolJDK]
	ghidraInstalled := hasGhidra && toolchain.IsToolInstalled(paths, ghidraDef)
	jdkInstalled := hasJDK && toolchain.IsToolInstalled(paths, jdkDef)
	ghidraValid := ghidraInstalled && jdkInstalled
	ghidraDetails := ""
	if cfg.Decompiler == workspace.DecompilerGhidra && !jdkInstalled {
		ghidraDetails = "Missing Adoptium OpenJDK runtime"
	}
	report.Decompilers = append(report.Decompilers, DecompilerEngineStatus{
		Engine:    "ghidra",
		Name:      "Ghidra",
		Active:    cfg.Decompiler == workspace.DecompilerGhidra,
		Installed: ghidraInstalled,
		Valid:     ghidraValid,
		Path:      paths.ToolDir(workspace.ToolGhidra),
		Details:   ghidraDetails,
	})

	// 2. Binaries inventory
	targets := []struct {
		target   workspace.TargetID
		name     string
		fileName string
	}{
		{workspace.TargetAttila64, "Attila Linux 64-bit DWARF", "Attila"},
		{workspace.TargetAttila32, "Total War: Attila 32-bit", "empire.retail.dll"},
		{workspace.TargetRome2_32, "Total War: Rome II 32-bit", "empire.retail.dll"},
		{workspace.TargetToB32, "Total War Saga: ToB 32-bit", "empire.retail.dll"},
	}

	for _, t := range targets {
		binDir := paths.BinaryDir(t.target)
		fullPath := filepath.Join(binDir, t.fileName)
		present := false
		sha := ""
		sourcePath := ""

		if stat, err := os.Stat(fullPath); err == nil && !stat.IsDir() {
			present = true
			if saved, ok := state.Binaries[t.target]; ok && saved.BinarySHA256 != "" {
				sha = saved.BinarySHA256
				sourcePath = saved.SourcePath
			} else {
				if computed, err := binaries.ComputeFileSHA256(fullPath); err == nil {
					sha = computed
				}
			}
		}

		known := false
		if sha != "" {
			if officialMap, ok := binaries.KnownOfficialHashes[t.target]; ok {
				for _, h := range officialMap {
					if h != "" && strings.EqualFold(h, sha) {
						known = true
						break
					}
				}
			}
		}

		report.Binaries = append(report.Binaries, BinaryStatus{
			Target:        t.target,
			Name:          t.name,
			Present:       present,
			SHA256:        sha,
			KnownOfficial: known,
			SourcePath:    sourcePath,
			Path:          fullPath,
		})

		// 3. Database status per target
		dbDir := filepath.Join(paths.DBDir, string(cfg.Decompiler), string(t.target))
		analyzed := false
		if saved, ok := state.Binaries[t.target]; ok && sha != "" {
			analyzedSHA := saved.Analyzed.Get(cfg.Decompiler)
			if analyzedSHA != "" && strings.EqualFold(analyzedSHA, sha) {
				analyzed = true
			}
		}

		report.Databases = append(report.Databases, DBStatus{
			Target:   t.target,
			Engine:   string(cfg.Decompiler),
			Analyzed: analyzed,
			Path:     dbDir,
		})
	}

	if jsonOut {
		enc := json.NewEncoder(printer.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return nil, fmt.Errorf("encode json status: %w", err)
		}
		return report, nil
	}

	// Human-readable output formatting
	renderHumanStatus(report, printer)
	return report, nil
}

func renderHumanStatus(report *StatusReport, printer *ui.Printer) {
	printer.Info("%s", printer.Bold("Workspace Configuration:"))
	if report.Initialized {
		printer.Info("  Status:       %s", printer.Green("Initialized"))
		printer.Info("  Default Game: %s", report.DefaultGame)
		printer.Info("  Decompiler:   %s", report.Decompiler)
	} else {
		printer.Info("  Status:       %s %s", printer.Yellow("Not initialized"), printer.Dim("(run `twdll init` to set up)"))
	}
	printer.Info("")

	printer.Info("%s", printer.Bold("Build Toolchain (.twdll/tools/):"))
	for _, t := range report.Tools {
		status := printer.Green("[x] Installed")
		if !t.Installed {
			status = printer.Red("[ ] Missing  ")
		}
		printer.Info("  %s %-18s %s", status, t.Name, printer.Dim("("+t.Version+")"))
	}
	printer.Info("")

	printer.Info("%s", printer.Bold("Decompiler Engines:"))
	for _, d := range report.Decompilers {
		status := printer.Green("[x] Active   ")
		if !d.Active {
			status = printer.Dim("[-] Inactive ")
		}

		readyTag := ""
		pathStr := ""
		cleanPath := filepath.ToSlash(d.Path)
		if d.Valid {
			readyTag = " " + printer.Green("[ready]")
			pathStr = printer.Dim("(" + cleanPath + ")")
		} else if d.Installed {
			readyTag = " " + printer.Yellow("[needs setup]")
			pathStr = printer.Dim("(" + cleanPath + ")")
		} else {
			if d.Engine == "ghidra" {
				pathStr = printer.Dim("(not installed)")
			} else {
				pathStr = printer.Dim("(not configured)")
			}
		}

		printer.Info("  %s %-12s %s%s", status, d.Name, pathStr, readyTag)
		if d.Active && d.Details != "" {
			printer.Info("    %s", printer.Yellow("Note: "+d.Details))
		}
	}
	printer.Info("")

	printer.Info("%s", printer.Bold("Game Binaries (.twdll/binaries/):"))
	for _, b := range report.Binaries {
		status := printer.Green("[x] Present")
		if !b.Present {
			status = printer.Red("[ ] Missing")
		}
		tag := ""
		if b.SHA256 != "" && len(b.SHA256) >= 12 {
			if b.KnownOfficial {
				tag = fmt.Sprintf(" %s %s", printer.Dim("(SHA: "+b.SHA256[:12]+"...)"), printer.Green("[official]"))
			} else {
				tag = fmt.Sprintf(" %s", printer.Dim("(SHA: "+b.SHA256[:12]+"...)"))
			}
		}
		printer.Info("  %s %-28s%s", status, b.Name, tag)
	}
	printer.Info("")

	printer.Info("%s", printer.Bold(fmt.Sprintf("Decompiler Databases (.twdll/db/%s/):", report.Decompiler)))
	for _, db := range report.Databases {
		status := printer.Green("[x] Analyzed      ")
		if !db.Analyzed {
			status = printer.Yellow("[ ] Needs Analysis")
		}
		printer.Info("  %s %-20s", status, db.Target)
	}
}
