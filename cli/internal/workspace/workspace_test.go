package workspace

import (
	"path/filepath"
	"testing"
)

func TestPaths(t *testing.T) {
	tmp := t.TempDir()
	p := NewPaths(tmp)

	if p.Config != filepath.Join(tmp, "twdll.toml") {
		t.Errorf("unexpected Config path: %s", p.Config)
	}
	if p.StateFile != filepath.Join(tmp, ".twdll", "state.json") {
		t.Errorf("unexpected StateFile path: %s", p.StateFile)
	}

	attila64Bin := p.BinaryPath(TargetAttila64)
	expectedBin := filepath.Join(tmp, ".twdll", "binaries", "attila_64", "Attila")
	if attila64Bin != expectedBin {
		t.Errorf("got %s, want %s", attila64Bin, expectedBin)
	}

	if err := p.EnsureBaseDirs(); err != nil {
		t.Fatalf("EnsureBaseDirs failed: %v", err)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "twdll.toml")

	// Missing file returns defaults
	cfg, exists, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig missing error: %v", err)
	}
	if exists {
		t.Errorf("expected exists=false for missing file")
	}
	if cfg.DefaultGame != GameAttila || cfg.Decompiler != DecompilerGhidra {
		t.Errorf("unexpected default config: %+v", cfg)
	}

	// Save modified config
	cfg.DefaultGame = GameRome2
	cfg.Decompiler = DecompilerIDA
	cfg.IDADir = "C:/IDA"
	cfg.Games.Attila = "C:/Games/Attila"
	cfg.Games.Rome2 = "C:/Games/Rome2"

	if err := SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, exists, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig saved error: %v", err)
	}
	if !exists {
		t.Errorf("expected exists=true for saved file")
	}
	if loaded.DefaultGame != GameRome2 || loaded.Decompiler != DecompilerIDA {
		t.Errorf("loaded config mismatch: %+v", loaded)
	}
	if loaded.Games.Attila != "C:/Games/Attila" || loaded.Games.Rome2 != "C:/Games/Rome2" {
		t.Errorf("loaded games mismatch: %+v", loaded.Games)
	}
}

func TestStateRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state.json")

	// Missing file returns empty state
	st, exists, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState missing error: %v", err)
	}
	if exists {
		t.Errorf("expected exists=false for missing file")
	}
	if st.SchemaVersion != CurrentStateSchemaVersion {
		t.Errorf("expected SchemaVersion=%d, got %d", CurrentStateSchemaVersion, st.SchemaVersion)
	}

	// Update state
	st.SetTool(ToolCMake, ToolState{Version: "3.31.5", SHA256: "abc123cmake"})
	st.SetBinary(TargetAttila64, BinaryState{
		SourcePath:   "C:/Steam/Attila",
		BinarySHA256: "hash64",
	})
	st.SetAnalyzed(TargetAttila64, DecompilerGhidra, "hash64")

	if err := SaveState(statePath, st); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded, exists, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState saved error: %v", err)
	}
	if !exists {
		t.Errorf("expected exists=true for saved file")
	}

	bin := loaded.Binaries[TargetAttila64]
	if !bin.IsAnalyzed(DecompilerGhidra) {
		t.Errorf("expected IsAnalyzed(Ghidra) = true")
	}
	if bin.IsAnalyzed(DecompilerIDA) {
		t.Errorf("expected IsAnalyzed(IDA) = false")
	}
	if loaded.Tools[ToolCMake].Version != "3.31.5" {
		t.Errorf("tool version mismatch: %+v", loaded.Tools[ToolCMake])
	}
}
