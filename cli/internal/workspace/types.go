package workspace

import "strings"

type GameID string

const (
	GameAttila GameID = "attila"
	GameRome2  GameID = "rome2"
	GameToB    GameID = "tob"
)

func AllGames() []GameID {
	return []GameID{GameAttila, GameRome2, GameToB}
}

func ParseGameID(raw string) (GameID, bool) {
	g := GameID(strings.ToLower(strings.TrimSpace(raw)))
	return g, g.IsValid()
}

func (g GameID) IsValid() bool {
	switch g {
	case GameAttila, GameRome2, GameToB:
		return true
	default:
		return false
	}
}

func (g GameID) DisplayName() string {
	switch g {
	case GameAttila:
		return "Total War: Attila"
	case GameRome2:
		return "Total War: Rome II"
	case GameToB:
		return "Total War Saga: Thrones of Britannia"
	default:
		return string(g)
	}
}

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

type TargetID string

const (
	TargetAttila64 TargetID = "attila_64"
	TargetAttila32 TargetID = "attila_32"
	TargetRome2_32 TargetID = "rome2_32"
	TargetToB32    TargetID = "tob_32"
)

func AllTargets() []TargetID {
	return []TargetID{TargetAttila64, TargetAttila32, TargetRome2_32, TargetToB32}
}

func ParseTargetID(raw string) (TargetID, bool) {
	t := TargetID(strings.ToLower(strings.TrimSpace(raw)))
	return t, t.IsValid()
}

func (t TargetID) IsValid() bool {
	switch t {
	case TargetAttila64, TargetAttila32, TargetRome2_32, TargetToB32:
		return true
	default:
		return false
	}
}

func (t TargetID) Is64Bit() bool {
	return t == TargetAttila64
}

func (t TargetID) BinaryFileName() string {
	if t == TargetAttila64 {
		return "Attila"
	}
	return "empire.retail.dll"
}

func (t TargetID) DisplayName() string {
	switch t {
	case TargetAttila64:
		return "Attila Linux 64-bit DWARF"
	case TargetAttila32:
		return "Total War: Attila 32-bit"
	case TargetRome2_32:
		return "Total War: Rome II 32-bit"
	case TargetToB32:
		return "Total War Saga: ToB 32-bit"
	default:
		return string(t)
	}
}

type DecompilerID string

const (
	DecompilerGhidra DecompilerID = "ghidra"
	DecompilerIDA    DecompilerID = "ida"
)

func AllDecompilers() []DecompilerID {
	return []DecompilerID{DecompilerGhidra, DecompilerIDA}
}

func ParseDecompilerID(raw string) (DecompilerID, bool) {
	d := DecompilerID(strings.ToLower(strings.TrimSpace(raw)))
	return d, d.IsValid()
}

func (d DecompilerID) IsValid() bool {
	return d == DecompilerGhidra || d == DecompilerIDA
}

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

// DataDir uses omitempty so the default twdll.toml remains free of internal paths,
// while custom configs (--config twdll.dev.toml) can isolate data_dir = ".twdll-dev".
type Config struct {
	DataDir     string       `toml:"data_dir,omitempty"`
	DefaultGame GameID       `toml:"default_game"`
	Decompiler  DecompilerID `toml:"decompiler"`
	IDADir      string       `toml:"ida_dir"`
	Games       GamesConfig  `toml:"games"`
}

type GamesConfig struct {
	Attila string `toml:"attila"`
	Rome2  string `toml:"rome2"`
	ToB    string `toml:"tob"`
}

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

type State struct {
	SchemaVersion int                      `json:"schema_version"`
	Tools         map[ToolID]ToolState     `json:"tools"`
	Binaries      map[TargetID]BinaryState `json:"binaries"`
}

type ToolState struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type BinaryState struct {
	SourcePath   string        `json:"source_path"`
	BinarySHA256 string        `json:"binary_sha256"`
	Analyzed     AnalyzedState `json:"analyzed_sha256"`
}

type AnalyzedState struct {
	Ghidra string `json:"ghidra,omitempty"`
	IDA    string `json:"ida,omitempty"`
}

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

func (a *AnalyzedState) Set(d DecompilerID, sha256 string) {
	switch d {
	case DecompilerGhidra:
		a.Ghidra = sha256
	case DecompilerIDA:
		a.IDA = sha256
	}
}

func (b BinaryState) IsAnalyzed(d DecompilerID) bool {
	return b.BinarySHA256 != "" && strings.EqualFold(b.Analyzed.Get(d), b.BinarySHA256)
}
