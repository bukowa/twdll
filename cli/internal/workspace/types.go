package workspace

// GameID identifies one of the three supported 32-bit Windows Warscape games.
type GameID string

const (
	GameAttila GameID = "attila"
	GameRome2  GameID = "rome2"
	GameToB    GameID = "tob"
)

// AllGames returns all supported 32-bit games in deterministic order.
func AllGames() []GameID {
	return []GameID{GameAttila, GameRome2, GameToB}
}

// IsValid reports whether g is a recognized GameID.
func (g GameID) IsValid() bool {
	switch g {
	case GameAttila, GameRome2, GameToB:
		return true
	default:
		return false
	}
}

// Target32 returns the corresponding 32-bit binary TargetID for this game.
func (g GameID) Target32() TargetID {
	switch g {
	case GameAttila:
		return TargetAttila32
	case GameRome2:
		return TargetRome2_32
	case GameToB:
		return TargetToB32
	default:
		return ""
	}
}

// TargetID identifies an isolated binary target inside .twdll/binaries/ and .twdll/db/.
type TargetID string

const (
	TargetAttila64 TargetID = "attila_64"
	TargetAttila32 TargetID = "attila_32"
	TargetRome2_32 TargetID = "rome2_32"
	TargetToB32    TargetID = "tob_32"
)

// AllTargets returns all binary targets in deterministic order (reference binary first).
func AllTargets() []TargetID {
	return []TargetID{TargetAttila64, TargetAttila32, TargetRome2_32, TargetToB32}
}

// IsValid reports whether t is a recognized TargetID.
func (t TargetID) IsValid() bool {
	switch t {
	case TargetAttila64, TargetAttila32, TargetRome2_32, TargetToB32:
		return true
	default:
		return false
	}
}

// BinaryFileName returns the canonical filename stored under .twdll/binaries/<target>/.
func (t TargetID) BinaryFileName() string {
	if t == TargetAttila64 {
		return "Attila"
	}
	return "empire.retail.dll"
}

// DecompilerID identifies the configured headless decompiler backend.
type DecompilerID string

const (
	DecompilerGhidra DecompilerID = "ghidra"
	DecompilerIDA    DecompilerID = "ida"
)

// IsValid reports whether d is a recognized DecompilerID.
func (d DecompilerID) IsValid() bool {
	return d == DecompilerGhidra || d == DecompilerIDA
}

// ToolID identifies a portable tool managed under .twdll/tools/<tool>/.
type ToolID string

const (
	ToolGo     ToolID = "go"
	ToolCMake  ToolID = "cmake"
	ToolNinja  ToolID = "ninja"
	ToolLLVM   ToolID = "llvm"
	ToolXwin   ToolID = "xwin"
	ToolRPFM   ToolID = "rpfm"
	ToolJDK    ToolID = "jdk"
	ToolGhidra ToolID = "ghidra"
)

// Config represents the user-editable twdll.toml file at the repository root.
type Config struct {
	DefaultGame GameID       `toml:"default_game"`
	Decompiler  DecompilerID `toml:"decompiler"`
	IDADir      string       `toml:"ida_dir"`
	Games       GamesConfig  `toml:"games"`
}

// GamesConfig stores the installation directories of owned 32-bit games.
type GamesConfig struct {
	Attila string `toml:"attila"`
	Rome2  string `toml:"rome2"`
	ToB    string `toml:"tob"`
}

// Get returns the configured directory for g.
func (c GamesConfig) Get(g GameID) string {
	switch g {
	case GameAttila:
		return c.Attila
	case GameRome2:
		return c.Rome2
	case GameToB:
		return c.ToB
	default:
		return ""
	}
}

// Set updates the configured directory for g.
func (c *GamesConfig) Set(g GameID, dir string) {
	switch g {
	case GameAttila:
		c.Attila = dir
	case GameRome2:
		c.Rome2 = dir
	case GameToB:
		c.ToB = dir
	}
}

// State represents the machine-managed .twdll/state.json manifest.
type State struct {
	SchemaVersion int                      `json:"schema_version"`
	Tools         map[ToolID]ToolState     `json:"tools"`
	Binaries      map[TargetID]BinaryState `json:"binaries"`
}

// ToolState records the installed version and archive digest of a portable tool.
type ToolState struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// BinaryState records the provenance, digest, and analysis completion state of a target binary.
type BinaryState struct {
	SourcePath   string        `json:"source_path"`
	BinarySHA256 string        `json:"binary_sha256"`
	Analyzed     AnalyzedState `json:"analyzed_sha256"`
}

// AnalyzedState stores per-decompiler binary SHA-256 digests for completed databases.
type AnalyzedState struct {
	Ghidra string `json:"ghidra,omitempty"`
	IDA    string `json:"ida,omitempty"`
}

// Get returns the analyzed binary SHA-256 for the given decompiler.
func (a AnalyzedState) Get(d DecompilerID) string {
	switch d {
	case DecompilerGhidra:
		return a.Ghidra
	case DecompilerIDA:
		return a.IDA
	default:
		return ""
	}
}

// Set updates the analyzed binary SHA-256 for the given decompiler.
func (a *AnalyzedState) Set(d DecompilerID, sha256 string) {
	switch d {
	case DecompilerGhidra:
		a.Ghidra = sha256
	case DecompilerIDA:
		a.IDA = sha256
	}
}

// IsAnalyzed reports whether the database for d was built from the current BinarySHA256.
func (b BinaryState) IsAnalyzed(d DecompilerID) bool {
	return b.BinarySHA256 != "" && b.Analyzed.Get(d) == b.BinarySHA256
}
