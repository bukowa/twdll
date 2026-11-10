package binaries

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"twdll/cli/internal/workspace"
)

func TestParseVDF(t *testing.T) {
	vdfContent := `
// This is a test comment
"AppState"
{
	"appid"		"325610"
	"name"		"Total War: ATTILA"
	"installdir"		"Total War Attila"
	"StateFlags"		"4"
	"UserConfig"
	{
		"language"		"english"
	}
}
`
	root, err := ParseVDF(vdfContent)
	if err != nil {
		t.Fatalf("ParseVDF failed: %v", err)
	}

	appState := root.Get("appstate")
	if appState == nil {
		t.Fatalf("missing appstate node")
	}

	if got := appState.GetString("installdir", ""); got != "Total War Attila" {
		t.Errorf("got installdir %q, want 'Total War Attila'", got)
	}
	if got := appState.GetInt("stateflags", 0); got != 4 {
		t.Errorf("got stateflags %d, want 4", got)
	}
	userConfig := appState.Get("userconfig")
	if userConfig == nil || userConfig.GetString("language", "") != "english" {
		t.Errorf("failed to parse nested section: %+v", userConfig)
	}
}

func TestSteamDir_Libraries(t *testing.T) {
	tmp := t.TempDir()
	steamRoot := filepath.Join(tmp, "Steam")
	secLib := filepath.Join(tmp, "SteamLibrary")
	_ = os.MkdirAll(filepath.Join(steamRoot, "steamapps"), 0o755)
	_ = os.MkdirAll(filepath.Join(secLib, "steamapps"), 0o755)

	// Create appmanifests
	_ = os.WriteFile(filepath.Join(steamRoot, "steamapps", "appmanifest_214950.acf"), []byte(`"AppState" { "appid" "214950" }`), 0o644)
	_ = os.WriteFile(filepath.Join(secLib, "steamapps", "appmanifest_325610.acf"), []byte(`"AppState" { "appid" "325610" }`), 0o644)

	vdfContent := `
"libraryfolders"
{
	"0"
	{
		"path"		"` + strings.ReplaceAll(steamRoot, `\`, `\\`) + `"
	}
	"1"
	{
		"path"		"` + strings.ReplaceAll(secLib, `\`, `\\`) + `"
	}
}
`
	_ = os.WriteFile(filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"), []byte(vdfContent), 0o644)

	steamDir := &SteamDir{Path: steamRoot}
	libs, err := steamDir.Libraries()
	if err != nil {
		t.Fatalf("Libraries() failed: %v", err)
	}
	if len(libs) != 2 {
		t.Fatalf("expected 2 libraries, got %d", len(libs))
	}

	// Guaranteed order from steamlocate-rs: library 0 then library 1
	if libs[0].Path != filepath.Clean(steamRoot) {
		t.Errorf("libs[0].Path = %q, want %q", libs[0].Path, steamRoot)
	}
	if libs[1].Path != filepath.Clean(secLib) {
		t.Errorf("libs[1].Path = %q, want %q", libs[1].Path, secLib)
	}

	// Verify app discovered in secLib
	app, lib, err := steamDir.FindApp(325610)
	if err != nil || app == nil || lib == nil {
		t.Fatalf("FindApp(325610) failed: %v", err)
	}
	if app.AppID != 325610 {
		t.Errorf("expected AppID 325610, got %d", app.AppID)
	}
	if lib.Path != filepath.Clean(secLib) {
		t.Errorf("expected lib path %q, got %q", secLib, lib.Path)
	}
}

func TestFindGameDirGroundTruth(t *testing.T) {
	tmp := t.TempDir()
	steamRoot := filepath.Join(tmp, "Steam")
	secLib := filepath.Join(tmp, "SecondaryDrive", "SteamLibrary")
	_ = os.MkdirAll(filepath.Join(steamRoot, "steamapps"), 0o755)
	_ = os.MkdirAll(filepath.Join(secLib, "steamapps"), 0o755)

	// Create installed game in secondary drive
	gameDir := filepath.Join(secLib, "steamapps", "common", "Total War Attila")
	_ = os.MkdirAll(gameDir, 0o755)
	dllPath := filepath.Join(gameDir, "empire.retail.dll")
	_ = os.WriteFile(dllPath, []byte("fake_pe"), 0o644)

	// Write appmanifest_325610.acf with StateFlags = 4
	acfContent := `
"AppState"
{
	"appid"		"325610"
	"installdir"		"Total War Attila"
	"StateFlags"		"4"
}
`
	_ = os.WriteFile(filepath.Join(secLib, "steamapps", "appmanifest_325610.acf"), []byte(acfContent), 0o644)

	vdfContent := `
"libraryfolders"
{
	"0"
	{
		"path"		"` + strings.ReplaceAll(steamRoot, `\`, `\\`) + `"
	}
	"1"
	{
		"path"		"` + strings.ReplaceAll(secLib, `\`, `\\`) + `"
	}
}
`
	_ = os.WriteFile(filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"), []byte(vdfContent), 0o644)

	steamDir := &SteamDir{Path: steamRoot}
	foundDir, ok := steamDir.FindGameDir(workspace.GameAttila)
	if !ok {
		t.Fatalf("FindGameDir failed to locate Attila")
	}
	expected := filepath.ToSlash(gameDir)
	if foundDir != expected {
		t.Errorf("got %q, want %q", foundDir, expected)
	}
}

func TestComputeFileSHA256(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(f, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	hash, err := ComputeFileSHA256(f)
	if err != nil {
		t.Fatalf("ComputeFileSHA256 error: %v", err)
	}
	expected := "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
	if hash != expected {
		t.Errorf("got %s, want %s", hash, expected)
	}
}

func TestVerifyInvalidFiles(t *testing.T) {
	tmp := t.TempDir()
	junk := filepath.Join(tmp, "junk.bin")
	if err := os.WriteFile(junk, []byte("NOT_A_PE_OR_ELF"), 0o644); err != nil {
		t.Fatalf("write junk: %v", err)
	}

	if _, err := VerifyPE32(junk, "test"); err == nil {
		t.Errorf("expected error for non-PE file")
	}

	if _, err := VerifyELF64DWARF(junk); err == nil {
		t.Errorf("expected error for non-ELF file")
	}
}

func TestLiveSteamLocate(t *testing.T) {
	steam, err := Locate()
	if err != nil {
		t.Skipf("no live steam on this machine: %v", err)
	}
	t.Logf("Found live Steam at: %s", steam.Path)

	libs, err := steam.Libraries()
	if err != nil {
		t.Fatalf("Libraries() failed: %v", err)
	}
	t.Logf("Discovered %d libraries:", len(libs))
	for _, lib := range libs {
		t.Logf("  - Library: %s (%d apps)", lib.Path, len(lib.AppIDs))
	}

	attilaDir, ok := steam.FindGameDir(workspace.GameAttila)
	if ok {
		t.Logf("Found Attila at: %s", attilaDir)
	} else {
		t.Logf("Attila not installed or not found")
	}
}
