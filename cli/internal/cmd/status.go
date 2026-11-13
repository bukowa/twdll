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

type StatusRequest struct {
	Layout workspace.Layout
	JSON   bool
	UI     *term.UI
}

type ToolStatus struct {
	ID        workspace.ToolID `json:"id"`
	Name      string           `json:"name"`
	Version   string           `json:"version"`
	Installed bool             `json:"installed"`
	Path      string           `json:"path,omitempty"`
}

type DecompilerStatus struct {
	Engine    string `json:"engine"`
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	Installed bool   `json:"installed"`
	Valid     bool   `json:"valid"`
	Path      string `json:"path,omitempty"`
	Details   string `json:"details,omitempty"`
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
	Engine   string             `json:"engine"`
	Analyzed bool               `json:"analyzed"`
	Path     string             `json:"path,omitempty"`
}

type StatusReport struct {
	Initialized     bool               `json:"initialized"`
	ConfigPath      string             `json:"config_path"`
	DataDir         string             `json:"data_dir"`
	DefaultGame     string             `json:"default_game"`
	Decompiler      string             `json:"decompiler"`
	IDADir          string             `json:"ida_dir,omitempty"`
	IDAValid        bool               `json:"ida_valid,omitempty"`
	XWinProvisioned bool               `json:"xwin_provisioned"`
	Tools           []ToolStatus       `json:"tools"`
	Decompilers     []DecompilerStatus `json:"decompilers"`
	Binaries        []BinaryStatus     `json:"binaries"`
	Databases       []DBStatus         `json:"databases"`
}

func RunStatus(_ context.Context, req StatusRequest) (*StatusReport, error) {
	paths := req.Layout.Paths
	cfg := req.Layout.Config
	state, _, _ := workspace.LoadState(paths.StateFile)

	idaValid := cfg.IDADir != "" && toolchain.ValidateIDADir(cfg.IDADir) == nil
	xwinReady := toolchain.IsXWinProvisioned(paths)

	report := &StatusReport{
		Initialized:     req.Layout.ConfigExists,
		ConfigPath:      filepath.ToSlash(paths.Config),
		DataDir:         filepath.ToSlash(paths.DataDir),
		DefaultGame:     string(cfg.DefaultGame),
		Decompiler:      string(cfg.Decompiler),
		IDADir:          filepath.ToSlash(cfg.IDADir),
		IDAValid:        idaValid,
		XWinProvisioned: xwinReady,
		Tools:           collectToolStatuses(paths, xwinReady),
		Decompilers:     collectDecompilerStatuses(paths, cfg, idaValid),
		Binaries:        make([]BinaryStatus, 0, 4),
		Databases:       make([]DBStatus, 0, 8),
	}

	for _, tid := range workspace.AllTargets() {
		binStat := inspectTargetBinary(paths, state, tid)
		report.Binaries = append(report.Binaries, binStat)
		for _, engine := range workspace.AllDecompilers() {
			report.Databases = append(report.Databases, inspectTargetDB(paths, state, tid, engine, binStat.SHA256))
		}
	}

	if req.UI != nil {
		if req.JSON {
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("encode json status: %w", err)
			}
			req.UI.WriteData("%s\n", string(data))
		} else {
			renderHumanStatus(report, req.UI)
		}
	}
	return report, nil
}

func collectToolStatuses(paths workspace.Paths, xwinReady bool) []ToolStatus {
	manifest := toolchain.HostManifest()
	out := make([]ToolStatus, 0, len(toolchain.BuildToolOrder))
	for _, id := range toolchain.BuildToolOrder {
		def, ok := manifest[id]
		if !ok {
			continue
		}
		installed := toolchain.IsInstalled(paths, def)
		if id == workspace.ToolXwin && installed && !xwinReady {
			installed = false
		}
		out = append(out, ToolStatus{
			ID:        def.ID,
			Name:      def.Name,
			Version:   def.Version,
			Installed: installed,
			Path:      filepath.ToSlash(paths.ToolDir(def.ID)),
		})
	}
	return out
}

func collectDecompilerStatuses(paths workspace.Paths, cfg workspace.Config, idaValid bool) []DecompilerStatus {
	manifest := toolchain.HostManifest()
	ghidraDef, hasGhidra := manifest[workspace.ToolGhidra]
	jdkDef, hasJDK := manifest[workspace.ToolJDK]
	ghidraInstalled := hasGhidra && toolchain.IsInstalled(paths, ghidraDef)
	jdkInstalled := hasJDK && toolchain.IsInstalled(paths, jdkDef)

	ghidraDetails := ""
	if ghidraInstalled && !jdkInstalled {
		ghidraDetails = "Missing Adoptium OpenJDK 25 runtime"
	}

	return []DecompilerStatus{
		{
			Engine:    string(workspace.DecompilerIDA),
			Name:      "IDA Pro",
			Active:    cfg.Decompiler == workspace.DecompilerIDA,
			Installed: idaValid,
			Valid:     idaValid,
			Path:      filepath.ToSlash(cfg.IDADir),
		},
		{
			Engine:    string(workspace.DecompilerGhidra),
			Name:      "Ghidra",
			Active:    cfg.Decompiler == workspace.DecompilerGhidra,
			Installed: ghidraInstalled,
			Valid:     ghidraInstalled && jdkInstalled,
			Path:      filepath.ToSlash(paths.ToolDir(workspace.ToolGhidra)),
			Details:   ghidraDetails,
		},
	}
}

func inspectTargetBinary(paths workspace.Paths, state workspace.State, tid workspace.TargetID) BinaryStatus {
	fullPath := paths.BinaryPath(tid)
	present := false
	sha := ""
	sourcePath := ""

	if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
		present = true
		if saved, ok := state.Binaries[tid]; ok && saved.BinarySHA256 != "" {
			sha = saved.BinarySHA256
			sourcePath = saved.SourcePath
		} else if computed, hErr := workspace.ComputeFileSHA256(fullPath); hErr == nil {
			sha = computed
		}
	}

	return BinaryStatus{
		Target:        tid,
		Name:          tid.DisplayName(),
		Present:       present,
		SHA256:        sha,
		KnownOfficial: sha != "" && binaries.IsKnownOfficialHash(tid, sha),
		SourcePath:    filepath.ToSlash(sourcePath),
		Path:          filepath.ToSlash(fullPath),
	}
}

func inspectTargetDB(paths workspace.Paths, state workspace.State, tid workspace.TargetID, engine workspace.DecompilerID, currentSHA string) DBStatus {
	analyzed := false
	if saved, ok := state.Binaries[tid]; ok && currentSHA != "" {
		if strings.EqualFold(saved.Analyzed.Get(engine), currentSHA) && paths.HasValidDatabase(engine, tid) {
			analyzed = true
		}
	}
	return DBStatus{
		Target:   tid,
		Engine:   string(engine),
		Analyzed: analyzed,
		Path:     filepath.ToSlash(paths.DatabaseDir(engine, tid)),
	}
}

func renderHumanStatus(r *StatusReport, ui *term.UI) {
	st := ui.OutStyle()
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n", st.Bold("Workspace Configuration:"))
	if r.Initialized {
		fmt.Fprintf(&b, "  Status:       %s\n", st.Green("Initialized"))
	} else {
		fmt.Fprintf(&b, "  Status:       %s %s\n", st.Yellow("Not initialized"), st.Dim("(run `twdll init` to set up)"))
	}
	fmt.Fprintf(&b, "  Config:       %s\n", st.Dim(r.ConfigPath))
	fmt.Fprintf(&b, "  Data Dir:     %s\n", st.Dim(r.DataDir))
	fmt.Fprintf(&b, "  Default Game: %s\n", r.DefaultGame)
	fmt.Fprintf(&b, "  Decompiler:   %s\n\n", r.Decompiler)

	fmt.Fprintf(&b, "%s\n", st.Bold("Build Toolchain:"))
	for _, t := range r.Tools {
		badge := st.Red("[ ] Missing  ")
		if t.Installed {
			badge = st.Green("[x] Installed")
		}
		fmt.Fprintf(&b, "  %s %-18s %s\n", badge, t.Name, st.Dim("("+t.Version+")"))
	}
	b.WriteByte('\n')

	fmt.Fprintf(&b, "%s\n", st.Bold("Decompiler Engines:"))
	for _, d := range r.Decompilers {
		role := st.Dim("[-] Inactive ")
		if d.Active {
			role = st.Green("[x] Active   ")
		}
		tag := st.Dim("(not configured)")
		if d.Valid {
			tag = st.Dim("("+d.Path+")") + " " + st.Green("[ready]")
		} else if d.Installed {
			tag = st.Dim("("+d.Path+")") + " " + st.Yellow("[needs setup]")
		} else if d.Engine == string(workspace.DecompilerGhidra) {
			tag = st.Dim("(not installed)")
		}
		fmt.Fprintf(&b, "  %s %-12s %s\n", role, d.Name, tag)
		if d.Active && d.Details != "" {
			fmt.Fprintf(&b, "    %s\n", st.Yellow("Note: "+d.Details))
		}
	}
	b.WriteByte('\n')

	fmt.Fprintf(&b, "%s\n", st.Bold("Game Binaries:"))
	for _, bin := range r.Binaries {
		badge := st.Red("[ ] Missing")
		if bin.Present {
			badge = st.Green("[x] Present")
		}
		suffix := ""
		if len(bin.SHA256) >= 12 {
			suffix = " " + st.Dim("(SHA: "+bin.SHA256[:12]+"...)")
			if bin.KnownOfficial {
				suffix += " " + st.Green("[official]")
			}
		}
		fmt.Fprintf(&b, "  %s %-28s%s\n", badge, bin.Name, suffix)
	}
	b.WriteByte('\n')

	fmt.Fprintf(&b, "%s\n", st.Bold("Decompiler Databases:"))
	ghLabel := "Ghidra"
	idaLabel := "IDA"
	if r.Decompiler == string(workspace.DecompilerGhidra) {
		ghLabel += " (active)"
	} else if r.Decompiler == string(workspace.DecompilerIDA) {
		idaLabel += " (active)"
	}
	fmt.Fprintf(&b, "  %s%s%s\n", st.Bold(padRight("Target", 16)), st.Bold(padRight(ghLabel, 24)), st.Bold(idaLabel))

	for _, bin := range r.Binaries {
		var ghOK, idaOK bool
		for _, db := range r.Databases {
			if db.Target == bin.Target && db.Engine == string(workspace.DecompilerGhidra) {
				ghOK = db.Analyzed
			}
			if db.Target == bin.Target && db.Engine == string(workspace.DecompilerIDA) {
				idaOK = db.Analyzed
			}
		}
		ghCell := formatDBCell(st, ghOK, r.Decompiler == string(workspace.DecompilerGhidra), 24)
		idaCell := formatDBCell(st, idaOK, r.Decompiler == string(workspace.DecompilerIDA), 0)
		fmt.Fprintf(&b, "  %s%s%s\n", padRight(string(bin.Target), 16), ghCell, idaCell)
	}

	ui.WriteData("%s", b.String())
}

func formatDBCell(st term.Style, analyzed, isActive bool, width int) string {
	raw := "[-] Not Analyzed"
	styled := st.Dim(raw)
	if analyzed {
		raw = "[x] Analyzed"
		styled = st.Green(raw)
	} else if isActive {
		raw = "[ ] Needs Analysis"
		styled = st.Yellow(raw)
	}
	if width <= len(raw) {
		return styled
	}
	return styled + strings.Repeat(" ", width-len(raw))
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
