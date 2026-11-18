package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"twdll/cli/internal/binaries"
	"twdll/cli/internal/extractdb"
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

type ExtractedSilverStatus struct {
	Present  bool   `json:"present"`
	UpToDate bool   `json:"up_to_date"`
	Locked   bool   `json:"locked,omitempty"`
	Error    string `json:"error,omitempty"`
	Funcs    int    `json:"funcs"`
	Asm      int    `json:"asm,omitempty"`
	OK       int    `json:"ok"`
	Failed   int    `json:"failed"`
	Timeout  int    `json:"timeout"`
	Skipped  int    `json:"skipped"`
	Path     string `json:"path,omitempty"`
}

type ExtractedStatus struct {
	Target          workspace.TargetID               `json:"target"`
	BronzePresent   bool                             `json:"bronze_present"`
	BronzeUpToDate  bool                             `json:"bronze_up_to_date"`
	BronzeLocked    bool                             `json:"bronze_locked,omitempty"`
	BronzeError     string                           `json:"bronze_error,omitempty"`
	BronzeSections  int                              `json:"bronze_sections,omitempty"`
	BronzeStrings   int                              `json:"bronze_strings,omitempty"`
	BronzeDwarfFunc int                              `json:"bronze_dwarf_funcs,omitempty"`
	BronzePath      string                           `json:"bronze_path,omitempty"`
	Silver          map[string]ExtractedSilverStatus `json:"silver,omitempty"`
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
	Extracted       []ExtractedStatus  `json:"extracted"`
}

func RunStatus(ctx context.Context, req StatusRequest) (*StatusReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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
		Extracted:       make([]ExtractedStatus, 0, 4),
	}

	for _, tid := range workspace.AllTargets() {
		binStat := inspectTargetBinary(paths, state, tid)
		report.Binaries = append(report.Binaries, binStat)
		for _, engine := range workspace.AllDecompilers() {
			report.Databases = append(report.Databases, inspectTargetDB(paths, state, tid, engine, binStat.SHA256))
		}
		report.Extracted = append(report.Extracted, inspectTargetExtracted(ctx, paths, tid, binStat.SHA256))
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

	b.WriteByte('\n')
	renderExtractedTable(&b, st, r.Extracted)

	ui.WriteData("%s", b.String())
}

func renderExtractedTable(b *strings.Builder, st term.Style, extracted []ExtractedStatus) {
	fmt.Fprintf(b, "%s\n", st.Bold("Extracted Databases:"))
	headers := [4]string{"Target", "Bronze", "Ghidra Silver", "IDA Silver"}
	minWidths := [3]int{16, 24, 24}

	rows := make([][4]statusCell, 0, len(extracted))
	for _, ext := range extracted {
		target := string(ext.Target)
		rows = append(rows, [4]statusCell{
			{text: target, visibleLen: utf8.RuneCountInString(target)},
			buildBronzeCell(st, ext),
			buildSilverCell(st, ext.Silver[string(workspace.DecompilerGhidra)]),
			buildSilverCell(st, ext.Silver[string(workspace.DecompilerIDA)]),
		})
	}

	var widths [3]int
	for col := 0; col < 3; col++ {
		w := utf8.RuneCountInString(headers[col]) + 3
		if minWidths[col] > w {
			w = minWidths[col]
		}
		for _, row := range rows {
			if row[col].visibleLen+3 > w {
				w = row[col].visibleLen + 3
			}
		}
		widths[col] = w
	}

	fmt.Fprintf(b, "  %s%s%s%s\n",
		st.Bold(padRight(headers[0], widths[0])),
		st.Bold(padRight(headers[1], widths[1])),
		st.Bold(padRight(headers[2], widths[2])),
		st.Bold(headers[3]),
	)
	for _, row := range rows {
		fmt.Fprintf(b, "  %s%s%s%s\n",
			padCell(row[0].text, row[0].visibleLen, widths[0]),
			padCell(row[1].text, row[1].visibleLen, widths[1]),
			padCell(row[2].text, row[2].visibleLen, widths[2]),
			row[3].text,
		)
	}
}

func inspectTargetExtracted(ctx context.Context, paths workspace.Paths, tid workspace.TargetID, currentSHA string) ExtractedStatus {
	bronzePath := paths.BronzeDBPath(tid)
	out := ExtractedStatus{
		Target:     tid,
		BronzePath: filepath.ToSlash(bronzePath),
		Silver:     make(map[string]ExtractedSilverStatus, 2),
	}

	b := extractdb.InspectBronzeStatus(ctx, bronzePath)
	out.BronzePresent = b.Present
	out.BronzeLocked = b.Locked
	out.BronzeError = b.Error
	out.BronzeSections = b.Sections
	out.BronzeStrings = b.Strings
	out.BronzeDwarfFunc = b.DwarfFuncs
	if b.Present && !b.Locked && b.Error == "" {
		out.BronzeUpToDate = currentSHA != "" && strings.EqualFold(strings.TrimSpace(b.BinarySHA256), strings.TrimSpace(currentSHA))
	}

	for _, engine := range workspace.AllDecompilers() {
		silverPath := paths.SilverDBPath(tid, engine)
		s := extractdb.InspectSilverStatus(ctx, silverPath)
		ss := ExtractedSilverStatus{
			Present: s.Present,
			Locked:  s.Locked,
			Error:   s.Error,
			Funcs:   s.Funcs,
			Asm:     s.Asm,
			OK:      s.OK,
			Failed:  s.Failed,
			Timeout: s.Timeout,
			Skipped: s.Skipped,
			Path:    filepath.ToSlash(silverPath),
		}
		if s.Present && !s.Locked && s.Error == "" {
			ss.UpToDate = s.Engine == string(engine) &&
				currentSHA != "" && strings.EqualFold(strings.TrimSpace(s.BinarySHA256), strings.TrimSpace(currentSHA))
		}
		out.Silver[string(engine)] = ss
	}
	return out
}

type styledSpan struct {
	plain  string
	styled string
}

type statusCell struct {
	text       string
	visibleLen int
}

func plainSpan(s string) styledSpan {
	return styledSpan{plain: s, styled: s}
}

func colorSpan(s string, fn func(string) string) styledSpan {
	return styledSpan{plain: s, styled: fn(s)}
}

func buildCell(spans ...styledSpan) statusCell {
	var text strings.Builder
	visibleLen := 0
	for _, sp := range spans {
		text.WriteString(sp.styled)
		visibleLen += utf8.RuneCountInString(sp.plain)
	}
	return statusCell{
		text:       text.String(),
		visibleLen: visibleLen,
	}
}

func buildStatusErrorCell(st term.Style, errMsg string) statusCell {
	raw := "[!] Error"
	if errMsg == "timeout" {
		raw = "[!] Timeout"
	}
	return buildCell(colorSpan(raw, st.Yellow))
}

func formatBronzeCell(st term.Style, ext ExtractedStatus, width int) string {
	cell := buildBronzeCell(st, ext)
	return padCell(cell.text, cell.visibleLen, width)
}

func buildBronzeCell(st term.Style, ext ExtractedStatus) statusCell {
	if !ext.BronzePresent {
		return buildCell(colorSpan("[-] None", st.Dim))
	}
	if ext.BronzeLocked {
		return buildCell(colorSpan("[!] Locked", st.Yellow))
	}
	if ext.BronzeError != "" {
		return buildStatusErrorCell(st, ext.BronzeError)
	}

	strNum := formatInt(ext.BronzeStrings)
	secondNum := formatInt(ext.BronzeSections)
	secondUnit := "sections"
	if ext.BronzeDwarfFunc > 0 {
		secondNum = formatInt(ext.BronzeDwarfFunc)
		secondUnit = "dwarf"
	}

	if !ext.BronzeUpToDate {
		return buildCell(
			colorSpan(fmt.Sprintf("[!] %s str, %s %s (stale)", strNum, secondNum, secondUnit), st.Yellow),
		)
	}
	return buildCell(
		colorSpan("[x]", st.Green),
		plainSpan(" "+strNum+" "),
		colorSpan("str,", st.Dim),
		plainSpan(" "+secondNum+" "),
		colorSpan(secondUnit, st.Dim),
	)
}

func formatSilverCell(st term.Style, ss ExtractedSilverStatus, width int) string {
	cell := buildSilverCell(st, ss)
	return padCell(cell.text, cell.visibleLen, width)
}

func buildSilverCell(st term.Style, ss ExtractedSilverStatus) statusCell {
	if !ss.Present {
		return buildCell(colorSpan("[-] None", st.Dim))
	}
	if ss.Locked {
		return buildCell(colorSpan("[!] Locked", st.Yellow))
	}
	if ss.Error != "" {
		return buildStatusErrorCell(st, ss.Error)
	}

	denom := ss.Funcs
	if denom == 0 {
		denom = ss.OK + ss.Failed + ss.Timeout + ss.Skipped
	}

	if !ss.UpToDate {
		switch {
		case denom > 0:
			return buildCell(colorSpan(fmt.Sprintf("[!] %s/%s fn (stale)", formatInt(ss.OK), formatInt(denom)), st.Yellow))
		case ss.OK > 0:
			return buildCell(colorSpan(fmt.Sprintf("[!] %s ok (stale)", formatInt(ss.OK)), st.Yellow))
		default:
			return buildCell(colorSpan(fmt.Sprintf("[!] %s fn (stale)", formatInt(ss.Funcs)), st.Yellow))
		}
	}

	switch {
	case ss.OK > 0 && denom > 0:
		return buildCell(
			colorSpan("[x]", st.Green),
			plainSpan(" "+formatInt(ss.OK)+"/"+formatInt(denom)+" "),
			colorSpan("fn", st.Dim),
		)
	case ss.OK > 0:
		return buildCell(
			colorSpan("[x]", st.Green),
			plainSpan(" "+formatInt(ss.OK)+" "),
			colorSpan("ok", st.Dim),
		)
	case ss.Funcs > 0 && ss.Failed == 0 && ss.Timeout == 0 && ss.Skipped == 0:
		return buildCell(
			colorSpan("[x]", st.Green),
			plainSpan(" "+formatInt(ss.Funcs)+" "),
			colorSpan("fn (no decomp)", st.Dim),
		)
	case ss.Funcs > 0:
		return buildCell(colorSpan(fmt.Sprintf("[!] 0/%s fn", formatInt(denom)), st.Yellow))
	default:
		return buildCell(colorSpan("[!] 0 fn", st.Yellow))
	}
}

func formatInt(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		if len(s) > rem {
			b.WriteByte(',')
		}
	}
	for i := rem; i < len(s); i += 3 {
		if i > rem {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func padCell(styled string, rawLen, width int) string {
	if width <= rawLen {
		return styled
	}
	return styled + strings.Repeat(" ", width-rawLen)
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
