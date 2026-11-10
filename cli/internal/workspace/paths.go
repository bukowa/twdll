package workspace

import (
	"os"
	"path/filepath"
	"runtime"
)

// Paths resolves all canonical workspace paths relative to the repository root.
type Paths struct {
	Root      string // <root>
	Config    string // <root>/twdll.toml
	DotDir    string // <root>/.twdll
	BinDir    string // <root>/.twdll/bin
	ToolsDir  string // <root>/.twdll/tools
	BinsDir   string // <root>/.twdll/binaries
	DBDir     string // <root>/.twdll/db
	LogsDir   string // <root>/.twdll/logs
	StateFile string // <root>/.twdll/state.json
}

// NewPaths constructs a Paths struct rooted at repoRoot.
func NewPaths(repoRoot string) Paths {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		absRoot = repoRoot
	}
	dot := filepath.Join(absRoot, ".twdll")
	return Paths{
		Root:      absRoot,
		Config:    filepath.Join(absRoot, "twdll.toml"),
		DotDir:    dot,
		BinDir:    filepath.Join(dot, "bin"),
		ToolsDir:  filepath.Join(dot, "tools"),
		BinsDir:   filepath.Join(dot, "binaries"),
		DBDir:     filepath.Join(dot, "db"),
		LogsDir:   filepath.Join(dot, "logs"),
		StateFile: filepath.Join(dot, "state.json"),
	}
}

// EnsureBaseDirs creates the top-level .twdll directory structure if missing.
func (p Paths) EnsureBaseDirs() error {
	for _, dir := range []string{p.BinDir, p.ToolsDir, p.BinsDir, p.DBDir, p.LogsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// ToolDir returns .twdll/tools/<id>.
func (p Paths) ToolDir(id ToolID) string {
	return filepath.Join(p.ToolsDir, string(id))
}

// BinaryDir returns .twdll/binaries/<target>.
func (p Paths) BinaryDir(t TargetID) string {
	return filepath.Join(p.BinsDir, string(t))
}

// BinaryPath returns the full path to the isolated binary inside .twdll/binaries/<target>/.
func (p Paths) BinaryPath(t TargetID) string {
	return filepath.Join(p.BinaryDir(t), t.BinaryFileName())
}

// DatabaseDir returns .twdll/db/<decompiler>/<target>.
func (p Paths) DatabaseDir(d DecompilerID, t TargetID) string {
	return filepath.Join(p.DBDir, string(d), string(t))
}

// DatabaseMarkerPath returns the primary database file that must exist after a completed analysis.
func (p Paths) DatabaseMarkerPath(d DecompilerID, t TargetID) string {
	dir := p.DatabaseDir(d, t)
	switch d {
	case DecompilerGhidra:
		// Headless Ghidra creates <dir>/<target>.gpr
		return filepath.Join(dir, string(t)+".gpr")
	case DecompilerIDA:
		// IDA 8.x/9.x creates .i64 (or .idb on legacy 32-bit idat)
		return filepath.Join(dir, t.BinaryFileName()+".i64")
	default:
		return dir
	}
}

// AnalyzeLogPath returns .twdll/logs/analyze_<decompiler>_<target>.log.
func (p Paths) AnalyzeLogPath(d DecompilerID, t TargetID) string {
	return filepath.Join(p.LogsDir, "analyze_"+string(d)+"_"+string(t)+".log")
}

// ExeSuffix returns ".exe" on Windows and "" on other platforms.
func ExeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
