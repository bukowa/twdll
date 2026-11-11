package analyze

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"twdll/cli/internal/workspace"
)

func TestFindIDAExecutable(t *testing.T) {
	// Empty dir returns error
	if _, err := FindIDAExecutable("", false); err == nil {
		t.Fatal("expected error on empty idaDir")
	}

	// Missing dir returns error
	if _, err := FindIDAExecutable(filepath.Join(os.TempDir(), "nonexistent_ida_dir_test"), false); err == nil {
		t.Fatal("expected error on nonexistent idaDir")
	}

	// Create temp dir with dummy idat.exe
	tmp := t.TempDir()
	binName := "idat.exe"
	if runtime.GOOS != "windows" {
		binName = "idat"
	}
	dummyPath := filepath.Join(tmp, binName)
	if err := os.WriteFile(dummyPath, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := FindIDAExecutable(tmp, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != dummyPath {
		t.Fatalf("expected %s, got %s", dummyPath, found)
	}
}

func TestFindGhidraHeadlessScript(t *testing.T) {
	// Missing tools dir returns error
	if _, err := FindGhidraHeadlessScript(t.TempDir()); err == nil {
		t.Fatal("expected error on empty tools dir")
	}

	// Create temp tools dir with ghidra/support/analyzeHeadless.bat
	tmp := t.TempDir()
	supportDir := filepath.Join(tmp, "ghidra", "support")
	if err := os.MkdirAll(supportDir, 0o755); err != nil {
		t.Fatal(err)
	}
	scriptName := "analyzeHeadless.bat"
	if runtime.GOOS != "windows" {
		scriptName = "analyzeHeadless"
	}
	scriptPath := filepath.Join(supportDir, scriptName)
	if err := os.WriteFile(scriptPath, []byte("@echo off"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := FindGhidraHeadlessScript(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != scriptPath {
		t.Fatalf("expected %s, got %s", scriptPath, found)
	}
}

func TestBuildPlan(t *testing.T) {
	tmpRoot := t.TempDir()
	paths := workspace.NewPaths(tmpRoot)
	if err := paths.EnsureBaseDirs(); err != nil {
		t.Fatal(err)
	}

	// Stage a dummy binary for attila_32
	attilaBinDir := paths.BinaryDir(workspace.TargetAttila32)
	if err := os.MkdirAll(attilaBinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dummyBin := paths.BinaryPath(workspace.TargetAttila32)
	if err := os.WriteFile(dummyBin, []byte("dummy_game_binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := workspace.Config{
		DefaultGame: workspace.GameAttila,
		Decompiler:  workspace.DecompilerIDA,
		Games: workspace.GamesConfig{
			Attila: "dummy_attila_path",
		},
	}

	state := &workspace.State{
		SchemaVersion: 1,
		Tools:         map[workspace.ToolID]workspace.ToolState{},
		Binaries:      map[workspace.TargetID]workspace.BinaryState{},
	}

	// 1. Initial plan: NeedsRun should be true
	opts := Options{}
	items, err := BuildPlan(cfg, state, paths, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !items[0].NeedsRun {
		t.Fatal("expected item to need run")
	}

	// 2. Mark target analyzed in state and create database marker file
	binSHA := items[0].BinarySHA256
	state.Binaries[workspace.TargetAttila32] = workspace.BinaryState{
		BinarySHA256: binSHA,
		Analyzed: workspace.AnalyzedState{
			IDA: binSHA,
		},
	}
	dbDir := paths.DatabaseDir(workspace.DecompilerIDA, workspace.TargetAttila32)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := paths.DatabaseMarkerPath(workspace.DecompilerIDA, workspace.TargetAttila32)
	if err := os.WriteFile(marker, []byte("dummy_idb"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-build plan without force: should be skipped
	items, err = BuildPlan(cfg, state, paths, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items[0].NeedsRun {
		t.Fatal("expected item to be skipped (up to date)")
	}

	// Re-build plan with force: should need run
	items, err = BuildPlan(cfg, state, paths, Options{Force: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !items[0].NeedsRun {
		t.Fatal("expected item to need run when Force=true")
	}
}
