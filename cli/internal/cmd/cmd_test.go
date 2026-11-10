package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"twdll/cli/internal/toolchain"
	"twdll/cli/internal/ui"
	"twdll/cli/internal/workspace"
)

func TestStatusUninitialized(t *testing.T) {
	tmp := t.TempDir()
	paths := workspace.NewPaths(tmp)

	var buf bytes.Buffer
	printer := &ui.Printer{
		Out: &buf,
		Err: &buf,
	}

	report, err := RunStatus(context.Background(), paths, false, printer)
	if err != nil {
		t.Fatalf("RunStatus error: %v", err)
	}

	if report.Initialized {
		t.Errorf("expected initialized = false")
	}

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("Not initialized")) {
		t.Errorf("output missing 'Not initialized': %s", output)
	}
}

func TestStatusJSONOutput(t *testing.T) {
	tmp := t.TempDir()
	paths := workspace.NewPaths(tmp)

	// Write mock config
	cfg := workspace.DefaultConfig()
	cfg.DefaultGame = workspace.GameAttila
	_ = workspace.SaveConfig(paths.Config, cfg)

	var buf bytes.Buffer
	printer := &ui.Printer{
		Out: &buf,
		Err: &buf,
	}

	report, err := RunStatus(context.Background(), paths, true, printer)
	if err != nil {
		t.Fatalf("RunStatus JSON error: %v", err)
	}

	if !report.Initialized {
		t.Errorf("expected initialized = true")
	}

	var parsed StatusReport
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed unmarshaling JSON output: %v\nOutput: %s", err, buf.String())
	}

	if parsed.DefaultGame != "attila" {
		t.Errorf("expected default_game 'attila', got %q", parsed.DefaultGame)
	}
}

func TestInitMissingGamesDiagnostic(t *testing.T) {
	tmp := t.TempDir()
	paths := workspace.NewPaths(tmp)
	_ = paths.EnsureBaseDirs()

	// Isolate Steam to an empty library containing no games
	emptySteam := filepath.Join(tmp, "empty_steam")
	_ = os.MkdirAll(filepath.Join(emptySteam, "steamapps"), 0o755)
	_ = os.WriteFile(filepath.Join(emptySteam, "steamapps", "libraryfolders.vdf"), []byte(`"libraryfolders" { "0" { "path" "`+filepath.ToSlash(emptySteam)+`" } }`), 0o644)
	t.Setenv("STEAM_DIR", emptySteam)

	// Mock all tools as installed so step 1 passes offline
	manifest := toolchain.CurrentPlatformManifest()
	for _, tool := range manifest {
		marker := filepath.Join(paths.ToolDir(tool.ID), filepath.FromSlash(tool.MarkerRel))
		_ = os.MkdirAll(filepath.Dir(marker), 0o755)
		_ = os.WriteFile(marker, []byte("mock"), 0o755)
	}
	sdkMarker := filepath.Join(paths.ToolDir(workspace.ToolXwin), "sdk", "crt", "lib", "x86", "libcmt.lib")
	_ = os.MkdirAll(filepath.Dir(sdkMarker), 0o755)
	_ = os.WriteFile(sdkMarker, []byte("mock"), 0o644)

	var buf bytes.Buffer
	printer := &ui.Printer{
		Out: &buf,
		Err: &buf,
	}

	opts := InitOptions{
		NoInput:     true,
		DefaultGame: "attila",
		Decompiler:  "ghidra",
	}

	// In an isolated temp dir without configured game paths, it should return a DiagnosticError for missing games
	err := RunInit(context.Background(), paths, opts, nil, printer)
	if err == nil {
		t.Fatalf("expected error for missing games")
	}

	diag, ok := err.(*ui.DiagnosticError)
	if !ok {
		t.Fatalf("expected *ui.DiagnosticError, got: %T (%v)", err, err)
	}

	if diag.Message != "no Total War game installations found" {
		t.Errorf("unexpected error message: %s", diag.Message)
	}
}

func TestCopyFile(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "sub", "dst.txt")

	_ = os.WriteFile(src, []byte("test_data"), 0o644)
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst failed: %v", err)
	}
	if string(content) != "test_data" {
		t.Errorf("got %q, want 'test_data'", string(content))
	}
}

func TestInitClearWorkspace(t *testing.T) {
	tmp := t.TempDir()
	paths := workspace.NewPaths(tmp)
	_ = paths.EnsureBaseDirs()

	// Write dummy files
	_ = os.WriteFile(paths.Config, []byte("mock config"), 0o644)
	_ = os.WriteFile(paths.StateFile, []byte("mock state"), 0o644)
	dummyTool := filepath.Join(paths.ToolsDir, "dummy_tool.txt")
	_ = os.WriteFile(dummyTool, []byte("mock tool"), 0o644)

	var buf bytes.Buffer
	printer := &ui.Printer{
		Out: &buf,
		Err: &buf,
	}

	opts := InitOptions{
		Clear:   true,
		NoInput: true,
	}

	_ = RunInit(context.Background(), paths, opts, nil, printer)

	if _, err := os.Stat(paths.Config); !os.IsNotExist(err) {
		t.Errorf("expected config to be deleted")
	}
	if _, err := os.Stat(dummyTool); !os.IsNotExist(err) {
		t.Errorf("expected dummy tool file to be deleted")
	}
}
