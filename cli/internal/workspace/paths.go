package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type ResolveSpec struct {
	RootOverride    string
	ConfigOverride  string
	DataDirOverride string
	WorkingDir      string
	ExecutablePath  string
}

type Layout struct {
	Paths        Paths
	Config       Config
	ConfigExists bool
}

type Paths struct {
	Root      string
	Config    string
	DataDir   string
	BinDir    string
	ToolsDir  string
	BinsDir   string
	DBDir     string
	LogsDir   string
	StateFile string
}

// Resolve applies DataDir precedence: --data-dir > TWDLL_DATA_DIR > config.data_dir > <root>/.twdll.
func Resolve(spec ResolveSpec) (Layout, error) {
	cwd := spec.WorkingDir
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}

	root := ResolveRepoRoot(spec)

	cfgPath := firstNonEmpty(spec.ConfigOverride, os.Getenv("TWDLL_CONFIG"))
	if cfgPath == "" {
		cfgPath = filepath.Join(root, "twdll.toml")
	} else if !filepath.IsAbs(cfgPath) && cwd != "" {
		cfgPath = filepath.Join(cwd, cfgPath)
	}
	cfgPath = filepath.Clean(cfgPath)

	cfg, exists, err := LoadConfig(cfgPath)
	if err != nil {
		return Layout{}, err
	}

	rawDataDir := firstNonEmpty(spec.DataDirOverride, os.Getenv("TWDLL_DATA_DIR"), cfg.DataDir)
	var dataDir string
	switch {
	case rawDataDir == "":
		dataDir = filepath.Join(root, ".twdll")
	case filepath.IsAbs(rawDataDir):
		dataDir = filepath.Clean(rawDataDir)
	default:
		dataDir = filepath.Clean(filepath.Join(root, filepath.FromSlash(rawDataDir)))
	}

	return Layout{
		Paths:        NewPaths(root, cfgPath, dataDir),
		Config:       cfg,
		ConfigExists: exists,
	}, nil
}

func ResolveRepoRoot(spec ResolveSpec) string {
	if explicit := firstNonEmpty(spec.RootOverride, os.Getenv("TWDLL_ROOT"), os.Getenv("TWDLL_WRAPPER_ROOT")); explicit != "" {
		if abs, err := filepath.Abs(explicit); err == nil {
			return filepath.Clean(abs)
		}
		return filepath.Clean(explicit)
	}

	exePath := spec.ExecutablePath
	if exePath == "" {
		exePath, _ = os.Executable()
	}
	if exePath != "" {
		if absExe, err := filepath.Abs(exePath); err == nil {
			binDir := filepath.Dir(absExe)
			dotDir := filepath.Dir(binDir)
			candidateRoot := filepath.Dir(dotDir)
			if strings.EqualFold(filepath.Base(binDir), "bin") && isRepoRoot(candidateRoot) {
				return filepath.Clean(candidateRoot)
			}
		}
	}

	cwd := spec.WorkingDir
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if cwd != "" {
		if absCwd, err := filepath.Abs(cwd); err == nil {
			for cur := absCwd; ; {
				if isRepoRoot(cur) {
					return filepath.Clean(cur)
				}
				parent := filepath.Dir(cur)
				if parent == cur {
					break
				}
				cur = parent
			}
			return filepath.Clean(absCwd)
		}
	}
	return "."
}

func NewPaths(repoRoot, configPath, dataDir string) Paths {
	cleanRoot := filepath.Clean(repoRoot)
	if configPath == "" {
		configPath = filepath.Join(cleanRoot, "twdll.toml")
	}
	if dataDir == "" {
		dataDir = filepath.Join(cleanRoot, ".twdll")
	}
	cleanData := filepath.Clean(dataDir)
	return Paths{
		Root:      cleanRoot,
		Config:    filepath.Clean(configPath),
		DataDir:   cleanData,
		BinDir:    filepath.Join(cleanRoot, ".twdll", "bin"),
		ToolsDir:  filepath.Join(cleanData, "tools"),
		BinsDir:   filepath.Join(cleanData, "binaries"),
		DBDir:     filepath.Join(cleanData, "db"),
		LogsDir:   filepath.Join(cleanData, "logs"),
		StateFile: filepath.Join(cleanData, "state.json"),
	}
}

func (p Paths) EnsureDirs() error {
	for _, dir := range []string{p.ToolsDir, p.BinsDir, p.DBDir, p.LogsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p Paths) ToolDir(id ToolID) string    { return filepath.Join(p.ToolsDir, string(id)) }
func (p Paths) BinaryDir(t TargetID) string { return filepath.Join(p.BinsDir, string(t)) }
func (p Paths) BinaryPath(t TargetID) string {
	return filepath.Join(p.BinaryDir(t), t.BinaryFileName())
}
func (p Paths) DatabaseDir(d DecompilerID, t TargetID) string {
	return filepath.Join(p.DBDir, string(d), string(t))
}

func (p Paths) DatabaseMarkerPath(d DecompilerID, t TargetID) string {
	dir := p.DatabaseDir(d, t)
	switch d {
	case DecompilerGhidra:
		return filepath.Join(dir, string(t)+".gpr")
	case DecompilerIDA:
		idbPath := filepath.Join(dir, string(t)+".idb")
		if info, err := os.Stat(idbPath); err == nil && !info.IsDir() {
			return idbPath
		}
		return filepath.Join(dir, string(t)+".i64")
	default:
		return dir
	}
}

// HasValidDatabase checks on-disk database completeness for a target.
// Headless Ghidra writes a 0-byte <target>.gpr marker alongside a <target>.rep directory,
// whereas headless IDA writes a single non-empty .i64 or .idb file.
func (p Paths) HasValidDatabase(d DecompilerID, t TargetID) bool {
	dir := p.DatabaseDir(d, t)
	switch d {
	case DecompilerGhidra:
		gprInfo, gprErr := os.Stat(filepath.Join(dir, string(t)+".gpr"))
		if gprErr != nil || gprInfo.IsDir() {
			return false
		}
		repInfo, repErr := os.Stat(filepath.Join(dir, string(t)+".rep"))
		return repErr == nil && repInfo.IsDir()
	case DecompilerIDA:
		info, err := os.Stat(p.DatabaseMarkerPath(d, t))
		return err == nil && !info.IsDir() && info.Size() > 0
	default:
		return false
	}
}

func (p Paths) AnalyzeLogPath(d DecompilerID, t TargetID) string {
	return filepath.Join(p.LogsDir, "analyze_"+string(d)+"_"+string(t)+".log")
}

func (p Paths) AnalyzeResourceLogPath() string {
	return filepath.Join(p.LogsDir, "analyze_resources.jsonl")
}

func ExeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func isRepoRoot(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, "cli", "go.mod")); err == nil && !info.IsDir() {
		return true
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
