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
	t.Setenv("TWDLL_TOOLS_DIR", "")

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
	if layout.Paths.ExtractedDir != filepath.Join(wantDataDir, "extracted") {
		t.Fatalf("ExtractedDir = %q, want inside sandbox", layout.Paths.ExtractedDir)
	}
	if layout.Paths.BronzeDBPath(TargetAttila32) != filepath.Join(wantDataDir, "extracted", "attila_32", "bronze.sqlite") {
		t.Fatalf("BronzeDBPath = %q, want inside sandbox", layout.Paths.BronzeDBPath(TargetAttila32))
	}
	if layout.Paths.SilverDBPath(TargetAttila32, DecompilerIDA) != filepath.Join(wantDataDir, "extracted", "attila_32", "silver_ida.sqlite") {
		t.Fatalf("SilverDBPath(ida) = %q, want inside sandbox", layout.Paths.SilverDBPath(TargetAttila32, DecompilerIDA))
	}
	if layout.Paths.SilverDBPath(TargetAttila32, DecompilerGhidra) != filepath.Join(wantDataDir, "extracted", "attila_32", "silver_ghidra.sqlite") {
		t.Fatalf("SilverDBPath(ghidra) = %q, want inside sandbox", layout.Paths.SilverDBPath(TargetAttila32, DecompilerGhidra))
	}
	if layout.Paths.TargetManifestPath(TargetAttila32) != filepath.Join(wantDataDir, "extracted", "attila_32", "manifest.json") {
		t.Fatalf("TargetManifestPath = %q, want inside sandbox", layout.Paths.TargetManifestPath(TargetAttila32))
	}
	if layout.Paths.DerivedDBPath() != filepath.Join(wantDataDir, "extracted", "derived.sqlite") {
		t.Fatalf("DerivedDBPath = %q, want inside sandbox", layout.Paths.DerivedDBPath())
	}
	if layout.Paths.BinDir != filepath.Join(repoRoot, ".twdll", "bin") {
		t.Fatalf("BinDir = %q, want shared .twdll/bin", layout.Paths.BinDir)
	}
	if flagLayout.Paths.DataDir != filepath.Join(repoRoot, ".twdll-flag") {
		t.Fatalf("DataDirOverride ignored: got %q", flagLayout.Paths.DataDir)
	}
	if flagLayout.Paths.ExtractedDir != filepath.Join(repoRoot, ".twdll-flag", "extracted") {
		t.Fatalf("flagLayout.Paths.ExtractedDir = %q, want inside flag sandbox", flagLayout.Paths.ExtractedDir)
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

	// And when BinarySHA256 was reset by init --clear, UpsertBinary preserves each engine's Analyzed digest independently
	st.Binaries[TargetAttila32] = BinaryState{
		Analyzed: AnalyzedState{
			Ghidra: "aaa111",
			IDA:    "ccc333",
		},
	}
	st.UpsertBinary(TargetAttila32, `G:/Steam/Attila/empire.retail.dll`, "AAA111")
	afterClearUpsert := st.Binaries[TargetAttila32]
	if !afterClearUpsert.IsAnalyzed(DecompilerGhidra) || afterClearUpsert.Analyzed.Ghidra != "aaa111" {
		t.Fatalf("expected Ghidra Analyzed hash preserved when BinarySHA256 was empty, got %+v", afterClearUpsert)
	}
	if afterClearUpsert.IsAnalyzed(DecompilerIDA) || afterClearUpsert.Analyzed.IDA != "" {
		t.Fatalf("expected mismatched IDA Analyzed hash cleared, got %+v", afterClearUpsert)
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

func TestResolve_ToolsDirOverrideAndEnv_PrecedenceAndExternalGuards(t *testing.T) {
	t.Setenv("TWDLL_ROOT", "")
	t.Setenv("TWDLL_WRAPPER_ROOT", "")
	t.Setenv("TWDLL_CONFIG", "")
	t.Setenv("TWDLL_DATA_DIR", "")
	t.Setenv("TWDLL_TOOLS_DIR", "")

	repoRoot := t.TempDir()
	cwd := filepath.Join(repoRoot, "sub", "cwd")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Default layout has HasExternalToolsDir() == false
	defLayout, err := Resolve(ResolveSpec{
		RootOverride: repoRoot,
		WorkingDir:   cwd,
	})
	if err != nil {
		t.Fatal(err)
	}
	if defLayout.Paths.HasExternalToolsDir() {
		t.Fatalf("expected default Paths.HasExternalToolsDir() = false, got true (%s)", defLayout.Paths.ToolsDir)
	}

	// 2. Relative TWDLL_TOOLS_DIR resolves against cwd (not repoRoot)
	t.Setenv("TWDLL_TOOLS_DIR", "../rel-tools")
	relLayout, err := Resolve(ResolveSpec{
		RootOverride: repoRoot,
		WorkingDir:   cwd,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRel := filepath.Clean(filepath.Join(cwd, "..", "rel-tools"))
	if relLayout.Paths.ToolsDir != wantRel {
		t.Fatalf("relative TWDLL_TOOLS_DIR = %q, want %q", relLayout.Paths.ToolsDir, wantRel)
	}
	if !relLayout.Paths.HasExternalToolsDir() {
		t.Fatalf("expected HasExternalToolsDir() = true for relative external tools dir")
	}

	// 3. Absolute TWDLL_TOOLS_DIR and ToolsDirOverride precedence
	absEnvTools := filepath.Join(t.TempDir(), "env-tools")
	overrideTools := filepath.Join(t.TempDir(), "override-tools")
	if err := os.MkdirAll(overrideTools, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWDLL_TOOLS_DIR", absEnvTools)

	envLayout, err := Resolve(ResolveSpec{
		RootOverride: repoRoot,
		WorkingDir:   cwd,
	})
	if err != nil {
		t.Fatal(err)
	}
	if envLayout.Paths.ToolsDir != filepath.Clean(absEnvTools) {
		t.Fatalf("abs TWDLL_TOOLS_DIR = %q, want %q", envLayout.Paths.ToolsDir, filepath.Clean(absEnvTools))
	}

	overrideLayout, err := Resolve(ResolveSpec{
		RootOverride:     repoRoot,
		ToolsDirOverride: overrideTools,
		WorkingDir:       cwd,
	})
	if err != nil {
		t.Fatal(err)
	}
	if overrideLayout.Paths.ToolsDir != filepath.Clean(overrideTools) {
		t.Fatalf("ToolsDirOverride = %q, want %q", overrideLayout.Paths.ToolsDir, filepath.Clean(overrideTools))
	}
	if !overrideLayout.Paths.HasExternalToolsDir() {
		t.Fatalf("expected HasExternalToolsDir() = true for ToolsDirOverride")
	}

	// 4. EnsureInitDirs() only creates BinsDir (without DBDir, LogsDir, or <DataDir>/tools when external ToolsDir is configured),
	//    and EnsureDirs() creates BinsDir, DBDir, and LogsDir without creating <DataDir>/tools.
	if err := overrideLayout.Paths.EnsureInitDirs(); err != nil {
		t.Fatalf("EnsureInitDirs with existing external tools dir: %v", err)
	}
	if info, statErr := os.Stat(overrideLayout.Paths.BinsDir); statErr != nil || !info.IsDir() {
		t.Fatalf("expected EnsureInitDirs to create %s, err=%v", overrideLayout.Paths.BinsDir, statErr)
	}
	for _, untouched := range []string{overrideLayout.Paths.DBDir, overrideLayout.Paths.LogsDir} {
		if _, statErr := os.Stat(untouched); !os.IsNotExist(statErr) {
			t.Fatalf("EnsureInitDirs must not create %s", untouched)
		}
	}
	if err := overrideLayout.Paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs with existing external tools dir: %v", err)
	}
	for _, d := range []string{overrideLayout.Paths.BinsDir, overrideLayout.Paths.DBDir, overrideLayout.Paths.LogsDir} {
		if info, statErr := os.Stat(d); statErr != nil || !info.IsDir() {
			t.Fatalf("expected EnsureDirs to create %s, err=%v", d, statErr)
		}
	}
	defaultToolsInSandbox := filepath.Join(overrideLayout.Paths.DataDir, "tools")
	if _, statErr := os.Stat(defaultToolsInSandbox); !os.IsNotExist(statErr) {
		t.Fatalf("EnsureDirs must not create %s when external ToolsDir is configured", defaultToolsInSandbox)
	}
}

func TestExpandTargets_GlobsAndConfiguredFiltering(t *testing.T) {
	root := t.TempDir()
	p := NewPaths(root, filepath.Join(root, "twdll.toml"), filepath.Join(root, ".twdll"))
	layout := Layout{
		Paths: p,
		Config: Config{
			Games: GamesConfig{
				Attila: `C:\Games\Attila`,
			},
		},
	}
	if err := os.MkdirAll(filepath.Dir(p.BinaryPath(TargetRome2_32)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.BinaryPath(TargetRome2_32), []byte("dll"), 0o644); err != nil {
		t.Fatal(err)
	}

	configured := ConfiguredTargets(layout)
	wantCfg := []TargetID{TargetAttila64, TargetAttila32, TargetRome2_32}
	if len(configured) != len(wantCfg) {
		t.Fatalf("ConfiguredTargets = %v, want %v", configured, wantCfg)
	}
	for i := range wantCfg {
		if configured[i] != wantCfg[i] {
			t.Fatalf("ConfiguredTargets[%d] = %q, want %q", i, configured[i], wantCfg[i])
		}
	}

	cases := []struct {
		name string
		raw  []string
		want []TargetID
	}{
		{name: "empty defaults to candidates", raw: nil, want: wantCfg},
		{name: "star_32 excludes unconfigured tob_32", raw: []string{"*_32"}, want: []TargetID{TargetAttila32, TargetRome2_32}},
		{name: "attila_star case-insensitive", raw: []string{"ATTILA_*"}, want: []TargetID{TargetAttila64, TargetAttila32}},
		{name: "dedup across overlapping globs and explicit", raw: []string{"attila_*", "attila_32", "*_32"}, want: []TargetID{TargetAttila64, TargetAttila32, TargetRome2_32}},
		{name: "explicit unconfigured target still resolves", raw: []string{"tob_32"}, want: []TargetID{TargetToB32}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpandTargets(tc.raw, configured)
			if err != nil {
				t.Fatalf("ExpandTargets(%v): %v", tc.raw, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ExpandTargets(%v) = %v, want %v", tc.raw, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("ExpandTargets(%v)[%d] = %q, want %q", tc.raw, i, got[i], tc.want[i])
				}
			}
		})
	}

	for _, bad := range [][]string{
		{"tob_*"},
		{"foo_*"},
		{"bogus"},
		{"["},
	} {
		if _, err := ExpandTargets(bad, configured); err == nil {
			t.Fatalf("expected error for ExpandTargets(%v)", bad)
		}
	}
}

func TestAnalyzedState_HasAnyAndIsEmpty(t *testing.T) {
	var empty AnalyzedState
	if empty.HasAny() || !empty.IsEmpty() {
		t.Fatalf("zero AnalyzedState should be empty: %+v", empty)
	}

	onlyGhidra := AnalyzedState{Ghidra: "aaa"}
	if !onlyGhidra.HasAny() || onlyGhidra.IsEmpty() {
		t.Fatalf("AnalyzedState with Ghidra should not be empty: %+v", onlyGhidra)
	}

	onlyIDA := AnalyzedState{IDA: "bbb"}
	if !onlyIDA.HasAny() || onlyIDA.IsEmpty() {
		t.Fatalf("AnalyzedState with IDA should not be empty: %+v", onlyIDA)
	}

	both := AnalyzedState{Ghidra: "aaa", IDA: "bbb"}
	if !both.HasAny() || both.IsEmpty() {
		t.Fatalf("AnalyzedState with both engines should not be empty: %+v", both)
	}
}

func TestState_ClearAnalyzedAndPruneToAnalyzed(t *testing.T) {
	var nilMapState State
	if nilMapState.ClearAnalyzed(TargetAttila32, DecompilerIDA) {
		t.Fatalf("ClearAnalyzed on nil Binaries map must return false")
	}

	st := NewState()
	st.SetTool(ToolGhidra, ToolState{Version: "12.0.4", SHA256: "toolhash"})
	st.UpsertBinary(TargetAttila32, "src.dll", "binhash")
	st.SetAnalyzed(TargetAttila32, DecompilerGhidra, "binhash")
	st.SetAnalyzed(TargetAttila32, DecompilerIDA, "binhash")
	st.UpsertBinary(TargetRome2_32, "rome2.dll", "rome2hash")
	st.Binaries[TargetToB32] = BinaryState{
		Analyzed: AnalyzedState{IDA: "tobhash"},
	}

	// Clearing missing target or already-empty engine returns false.
	if st.ClearAnalyzed(TargetAttila64, DecompilerGhidra) {
		t.Fatalf("ClearAnalyzed on missing target must return false")
	}
	if st.ClearAnalyzed(TargetRome2_32, DecompilerIDA) {
		t.Fatalf("ClearAnalyzed on unanalyzed engine must return false")
	}

	// Clearing one engine when staging metadata exists preserves the binary entry.
	if !st.ClearAnalyzed(TargetAttila32, DecompilerIDA) {
		t.Fatalf("expected ClearAnalyzed to return true for attila_32 IDA")
	}
	if st.Binaries[TargetAttila32].Analyzed.IDA != "" || st.Binaries[TargetAttila32].Analyzed.Ghidra != "binhash" {
		t.Fatalf("unexpected attila_32 state after clearing IDA: %+v", st.Binaries[TargetAttila32])
	}
	if !st.ClearAnalyzed(TargetAttila32, DecompilerGhidra) {
		t.Fatalf("expected ClearAnalyzed to return true for attila_32 Ghidra")
	}
	if _, exists := st.Binaries[TargetAttila32]; !exists {
		t.Fatalf("expected attila_32 entry preserved while SourcePath/BinarySHA256 are non-empty")
	}

	// Clearing the last analyzed hash when staging metadata is empty deletes the map entry.
	if !st.ClearAnalyzed(TargetToB32, DecompilerIDA) {
		t.Fatalf("expected ClearAnalyzed to return true for tob_32 IDA")
	}
	if _, exists := st.Binaries[TargetToB32]; exists {
		t.Fatalf("expected tob_32 entry deleted when staging metadata and analyzed hashes are all empty")
	}

	// PruneToAnalyzed drops tools, drops targets without analyzed hashes, and strips staging metadata.
	st.SetAnalyzed(TargetAttila32, DecompilerGhidra, "binhash")
	pruned := st.PruneToAnalyzed()
	if len(pruned.Tools) != 0 {
		t.Fatalf("expected pruned state Tools to be empty, got %+v", pruned.Tools)
	}
	if len(pruned.Binaries) != 1 {
		t.Fatalf("expected 1 pruned binary entry, got %+v", pruned.Binaries)
	}
	got := pruned.Binaries[TargetAttila32]
	if got.SourcePath != "" || got.BinarySHA256 != "" || got.Analyzed.Ghidra != "binhash" || got.Analyzed.IDA != "" {
		t.Fatalf("unexpected pruned binary entry: %+v", got)
	}
}
