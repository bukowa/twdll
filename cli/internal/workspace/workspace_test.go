package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_UsesCustomConfigAndDataDirPrecedence(t *testing.T) {
	// Given a repository root with a custom dev config setting data_dir = ".twdll-sandbox"
	t.Setenv("TWDLL_ROOT", "")
	t.Setenv("TWDLL_WRAPPER_ROOT", "")
	t.Setenv("TWDLL_CONFIG", "")
	t.Setenv("TWDLL_DATA_DIR", "")

	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "cli", "go.mod"), []byte("module twdll/cli\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	devCfgPath := filepath.Join(repoRoot, "twdll.dev.toml")
	err := SaveConfig(devCfgPath, Config{
		DataDir:     ".twdll-sandbox",
		DefaultGame: GameRome2,
		Decompiler:  DecompilerIDA,
	})
	if err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	subDir := filepath.Join(repoRoot, "src", "attila")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// When Resolve runs from a nested subdirectory and then with an explicit --data-dir override
	layout, err := Resolve(ResolveSpec{
		ConfigOverride: devCfgPath,
		WorkingDir:     subDir,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	flagLayout, err := Resolve(ResolveSpec{
		ConfigOverride:  devCfgPath,
		DataDirOverride: ".twdll-flag",
		WorkingDir:      repoRoot,
	})
	if err != nil {
		t.Fatalf("Resolve with flag override: %v", err)
	}

	// Then Root resolves upward, config.data_dir isolates DataDir, and --data-dir outranks config.data_dir
	wantDataDir := filepath.Join(repoRoot, ".twdll-sandbox")
	if layout.Paths.Root != repoRoot {
		t.Fatalf("Root = %q, want %q", layout.Paths.Root, repoRoot)
	}
	if layout.Paths.DataDir != wantDataDir {
		t.Fatalf("DataDir = %q, want %q", layout.Paths.DataDir, wantDataDir)
	}
	if layout.Paths.DBDir != filepath.Join(wantDataDir, "db") {
		t.Fatalf("DBDir = %q, want inside sandbox", layout.Paths.DBDir)
	}
	if layout.Paths.BinDir != filepath.Join(repoRoot, ".twdll", "bin") {
		t.Fatalf("BinDir = %q, want shared .twdll/bin", layout.Paths.BinDir)
	}
	if flagLayout.Paths.DataDir != filepath.Join(repoRoot, ".twdll-flag") {
		t.Fatalf("DataDirOverride ignored: got %q", flagLayout.Paths.DataDir)
	}
}

func TestSaveConfig_OmitsDefaultDataDirKey(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "twdll.toml")
	if err := SaveConfig(cfgPath, DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "data_dir") {
		t.Fatalf("default twdll.toml must omit empty data_dir, got:\n%s", string(raw))
	}
}

func TestState_UpsertBinary_PreservesAnalyzedWhenHashMatches_AndClearsOnDrift(t *testing.T) {
	// Given a state with completed Ghidra and IDA analyses for a known SHA-256
	st := NewState()
	st.UpsertBinary(TargetAttila32, `G:\Steam\Attila\empire.retail.dll`, "aaa111")
	st.SetAnalyzed(TargetAttila32, DecompilerGhidra, "aaa111")
	st.SetAnalyzed(TargetAttila32, DecompilerIDA, "aaa111")

	// When UpsertBinary runs with the same digest, then with a modified digest
	st.UpsertBinary(TargetAttila32, `G:/Steam/Attila/empire.retail.dll`, "AAA111")
	sameHash := st.Binaries[TargetAttila32]

	st.UpsertBinary(TargetAttila32, `G:/Steam/Attila/empire.retail.dll`, "bbb222")
	driftedHash := st.Binaries[TargetAttila32]

	// Then matching digests keep Analyzed hashes intact while changed digests clear stale hashes
	if !sameHash.IsAnalyzed(DecompilerGhidra) || !sameHash.IsAnalyzed(DecompilerIDA) {
		t.Fatalf("UpsertBinary wiped Analyzed state on matching SHA: %+v", sameHash)
	}
	if driftedHash.IsAnalyzed(DecompilerGhidra) || driftedHash.Analyzed.Ghidra != "" {
		t.Fatalf("UpsertBinary retained stale Analyzed state after SHA change: %+v", driftedHash)
	}
}

func TestPaths_HasValidDatabase_AcceptsZeroByteGhidraGPRWithRepAndRequiresNonEmptyIDA(t *testing.T) {
	// Given a workspace with a 0-byte Ghidra .gpr + .rep directory and a 0-byte vs populated IDA .i64
	root := t.TempDir()
	p := NewPaths(root, "", filepath.Join(root, ".twdll"))

	ghDir := p.DatabaseDir(DecompilerGhidra, TargetAttila32)
	if err := os.MkdirAll(filepath.Join(ghDir, "attila_32.rep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ghDir, "attila_32.gpr"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	idaDir := p.DatabaseDir(DecompilerIDA, TargetAttila32)
	if err := os.MkdirAll(idaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	idaFile := filepath.Join(idaDir, "attila_32.i64")
	if err := os.WriteFile(idaFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// When HasValidDatabase checks Ghidra (0-byte .gpr + .rep) and empty vs non-empty IDA .i64
	ghValid := p.HasValidDatabase(DecompilerGhidra, TargetAttila32)
	idaEmptyValid := p.HasValidDatabase(DecompilerIDA, TargetAttila32)
	if err := os.WriteFile(idaFile, []byte("idb"), 0o644); err != nil {
		t.Fatal(err)
	}
	idaNonEmptyValid := p.HasValidDatabase(DecompilerIDA, TargetAttila32)

	// Then Ghidra is valid despite 0-byte .gpr, empty IDA is rejected, and non-empty IDA is accepted
	if !ghValid {
		t.Fatalf("expected 0-byte Ghidra .gpr + .rep directory to be recognized as valid")
	}
	if idaEmptyValid {
		t.Fatalf("expected 0-byte IDA .i64 to be rejected")
	}
	if !idaNonEmptyValid {
		t.Fatalf("expected populated IDA .i64 to be accepted")
	}
}
