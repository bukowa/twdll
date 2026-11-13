package cmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"twdll/cli/internal/term"
	"twdll/cli/internal/toolchain"
	"twdll/cli/internal/workspace"
)

func TestRunStatus_RoutesJSONStrictlyToStdout_WithZeroStderrNoise(t *testing.T) {
	// Given a custom sandbox workspace resolved via --config with data_dir
	root := t.TempDir()
	cfgPath := filepath.Join(root, "twdll.sandbox.toml")
	if err := workspace.SaveConfig(cfgPath, workspace.Config{
		DataDir:     ".twdll-sandbox",
		DefaultGame: workspace.GameAttila,
		Decompiler:  workspace.DecompilerGhidra,
	}); err != nil {
		t.Fatal(err)
	}
	layout, err := workspace.Resolve(workspace.ResolveSpec{
		RootOverride:   root,
		ConfigOverride: cfgPath,
		WorkingDir:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{Out: &stdout, Err: &stderr, ColorMode: term.ColorNever})

	// When status --json executes
	report, err := RunStatus(context.Background(), StatusRequest{
		Layout: layout,
		JSON:   true,
		UI:     ui,
	})
	if err != nil {
		t.Fatalf("RunStatus: %v", err)
	}

	// Then stdout parses as valid JSON matching the sandbox DataDir and stderr is completely empty
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr on status --json, got: %q", stderr.String())
	}
	var decoded StatusReport
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON on stdout: %v\nraw: %s", err, stdout.String())
	}
	if !report.Initialized || !decoded.Initialized {
		t.Fatalf("expected initialized=true")
	}
	if !strings.HasSuffix(decoded.DataDir, ".twdll-sandbox") {
		t.Fatalf("DataDir = %q, want .twdll-sandbox suffix", decoded.DataDir)
	}
}

func TestRunInit_PreservesAnalyzedHashesAndNeverCallsSteamInNonInteractiveMode(t *testing.T) {
	// Given an initialized sandbox with pre-installed tool markers, valid PE32/ELF64 binaries, and existing Analyzed hashes
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-test")

	gameDir := filepath.Join(root, "FakeAttila")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dllPath := filepath.Join(gameDir, "empire.retail.dll")
	writeMinimalPE32(t, dllPath)
	dllSHA, err := workspace.ComputeFileSHA256(dllPath)
	if err != nil {
		t.Fatal(err)
	}

	elfPath := filepath.Join(root, "Attila64")
	writeMinimalELF64DWARF(t, elfPath)
	elfSHA, err := workspace.ComputeFileSHA256(elfPath)
	if err != nil {
		t.Fatal(err)
	}

	seedState := workspace.NewState()
	seedState.UpsertBinary(workspace.TargetAttila32, dllPath, dllSHA)
	seedState.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerGhidra, dllSHA)
	seedState.UpsertBinary(workspace.TargetAttila64, elfPath, elfSHA)
	seedState.SetAnalyzed(workspace.TargetAttila64, workspace.DecompilerGhidra, elfSHA)
	if err := workspace.SaveState(layout.Paths.StateFile, seedState); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{Out: &stdout, Err: &stderr, ColorMode: term.ColorNever})
	steamCalled := false

	// When RunInit executes in non-interactive mode (--no-input)
	err = RunInit(context.Background(), InitRequest{
		Layout: layout,
		Options: InitOptions{
			NoInput:     true,
			AttilaDir:   gameDir,
			Attila64Src: elfPath,
		},
		OpenSteamCmd: func() error {
			steamCalled = true
			return nil
		},
		UI: ui,
	})
	if err != nil {
		t.Fatalf("RunInit failed: %v\nstderr:\n%s", err, stderr.String())
	}

	// Then stdout stays completely empty, Analyzed hashes survive intact, and state.Tools is populated
	if stdout.Len() != 0 {
		t.Fatalf("init must emit logs to stderr only, got stdout: %q", stdout.String())
	}
	if steamCalled {
		t.Fatalf("OpenSteamCmd must never be called when binary is resolved or non-interactive")
	}

	afterState, _, err := workspace.LoadState(layout.Paths.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if !afterState.Binaries[workspace.TargetAttila32].IsAnalyzed(workspace.DecompilerGhidra) {
		t.Fatalf("re-running init wiped attila_32 Analyzed hash: %+v", afterState.Binaries[workspace.TargetAttila32])
	}
	if !afterState.Binaries[workspace.TargetAttila64].IsAnalyzed(workspace.DecompilerGhidra) {
		t.Fatalf("re-running init wiped attila_64 Analyzed hash: %+v", afterState.Binaries[workspace.TargetAttila64])
	}
	if len(afterState.Tools) == 0 {
		t.Fatalf("expected state.Tools to be populated by EnsureTool")
	}
}

func TestRunInit_MissingAttila64InAgentMode_FailsFastWithoutSteamSideEffects(t *testing.T) {
	// Given a sandbox with a valid 32-bit game binary but missing attila_64 and an empty STEAM_DIR
	root := t.TempDir()
	emptySteam := filepath.Join(root, "EmptySteam")
	if err := os.MkdirAll(filepath.Join(emptySteam, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptySteam, "steamapps", "libraryfolders.vdf"), []byte(`"libraryfolders" {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEAM_DIR", emptySteam)

	layout := newPreseededToolchainSandbox(t, root, ".twdll-agent")
	gameDir := filepath.Join(root, "FakeAttila")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMinimalPE32(t, filepath.Join(gameDir, "empire.retail.dll"))

	steamOpened := false
	ui := term.NewUI(term.Streams{ColorMode: term.ColorNever})

	// When RunInit runs without a Prompter (simulating an AI agent / non-TTY pipe)
	err := RunInit(context.Background(), InitRequest{
		Layout:  layout,
		Options: InitOptions{NoInput: true, AttilaDir: gameDir},
		OpenSteamCmd: func() error {
			steamOpened = true
			return nil
		},
		UI: ui,
	})

	// Then it returns a DiagnosticError without triggering GUI/clipboard side effects
	var diag *term.DiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError, got %T (%v)", err, err)
	}
	if steamOpened {
		t.Fatalf("non-interactive agent mode must NEVER invoke OpenSteamDepotConsole")
	}
}

func TestRunClean_PreservesSandboxIsolationAndClearsState(t *testing.T) {
	// Given a sandbox with an IDA database marker and analyzed state
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-clean")

	prodDB := filepath.Join(root, ".twdll", "db", "ida", "attila_32", "attila_32.i64")
	if err := os.MkdirAll(filepath.Dir(prodDB), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prodDB, []byte("production-db"), 0o644); err != nil {
		t.Fatal(err)
	}

	sandboxMarker := layout.Paths.DatabaseMarkerPath(workspace.DecompilerIDA, workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Dir(sandboxMarker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sandboxMarker, []byte("sandbox-db"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := workspace.NewState()
	st.UpsertBinary(workspace.TargetAttila32, "src.dll", "hash123")
	st.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerIDA, "hash123")
	if err := workspace.SaveState(layout.Paths.StateFile, st); err != nil {
		t.Fatal(err)
	}

	// When RunClean runs against the sandbox layout
	err := RunClean(context.Background(), CleanRequest{
		Layout:  layout,
		Options: CleanOptions{Decompiler: "ida", Targets: []string{"attila_32"}},
		UI:      term.NewUI(term.Streams{ColorMode: term.ColorNever}),
	})
	if err != nil {
		t.Fatalf("RunClean: %v", err)
	}

	// Then the sandbox DB is removed, state is cleared, and production .twdll/db is untouched
	if _, err := os.Stat(sandboxMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected sandbox DB to be deleted")
	}
	if _, err := os.Stat(prodDB); err != nil {
		t.Fatalf("production .twdll DB must remain untouched: %v", err)
	}
	after, _, _ := workspace.LoadState(layout.Paths.StateFile)
	if after.Binaries[workspace.TargetAttila32].Analyzed.IDA != "" {
		t.Fatalf("expected IDA analyzed hash cleared, got %q", after.Binaries[workspace.TargetAttila32].Analyzed.IDA)
	}
}

func newPreseededToolchainSandbox(t *testing.T, root, dataDir string) workspace.Layout {
	t.Helper()
	cfgPath := filepath.Join(root, "twdll.test.toml")
	cfg := workspace.Config{
		DataDir:     dataDir,
		DefaultGame: workspace.GameAttila,
		Decompiler:  workspace.DecompilerGhidra,
	}
	if err := workspace.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	layout, err := workspace.Resolve(workspace.ResolveSpec{
		RootOverride:   root,
		ConfigOverride: cfgPath,
		WorkingDir:     root,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, def := range toolchain.HostManifest() {
		marker := filepath.Join(layout.Paths.ToolDir(def.ID), filepath.FromSlash(def.MarkerRel))
		if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(marker, []byte("stub"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	xwinLib := filepath.Join(layout.Paths.ToolDir(workspace.ToolXwin), "sdk", "crt", "lib", "x86", "libcmt.lib")
	if err := os.MkdirAll(filepath.Dir(xwinLib), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(xwinLib, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	return layout
}

func writeMinimalPE32(t *testing.T, path string) {
	t.Helper()
	buf := make([]byte, 0x100)
	buf[0] = 'M'
	buf[1] = 'Z'
	binary.LittleEndian.PutUint32(buf[0x3c:], 0x80)
	copy(buf[0x80:], []byte{'P', 'E', 0, 0})
	binary.LittleEndian.PutUint16(buf[0x84:], 0x014c) // IMAGE_FILE_MACHINE_I386
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeMinimalELF64DWARF(t *testing.T, path string) {
	t.Helper()
	shstrtab := []byte("\x00.shstrtab\x00.debug_info\x00")
	debugInfo := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	const ehdrSize = 64
	const shentSize = 64
	const shNum = 3

	shstrOff := uint64(ehdrSize)
	debugOff := shstrOff + uint64(len(shstrtab))
	shOff := (debugOff + uint64(len(debugInfo)) + 7) &^ 7

	buf := make([]byte, int(shOff)+shNum*shentSize)
	copy(buf[0:4], []byte{0x7f, 'E', 'L', 'F'})
	buf[4] = 2                                         // ELFCLASS64
	buf[5] = 1                                         // ELFDATA2LSB
	buf[6] = 1                                         // EV_CURRENT
	binary.LittleEndian.PutUint16(buf[16:], 2)         // ET_EXEC
	binary.LittleEndian.PutUint16(buf[18:], 62)        // EM_X86_64
	binary.LittleEndian.PutUint32(buf[20:], 1)         // EV_CURRENT
	binary.LittleEndian.PutUint64(buf[40:], shOff)     // e_shoff
	binary.LittleEndian.PutUint16(buf[52:], ehdrSize)  // e_ehsize
	binary.LittleEndian.PutUint16(buf[58:], shentSize) // e_shentsize
	binary.LittleEndian.PutUint16(buf[60:], shNum)     // e_shnum
	binary.LittleEndian.PutUint16(buf[62:], 1)         // e_shstrndx

	copy(buf[shstrOff:], shstrtab)
	copy(buf[debugOff:], debugInfo)

	// Section 1: .shstrtab (name offset 1, SHT_STRTAB = 3)
	s1 := buf[int(shOff)+shentSize:]
	binary.LittleEndian.PutUint32(s1[0:], 1)
	binary.LittleEndian.PutUint32(s1[4:], 3)
	binary.LittleEndian.PutUint64(s1[24:], shstrOff)
	binary.LittleEndian.PutUint64(s1[32:], uint64(len(shstrtab)))

	// Section 2: .debug_info (name offset 11, SHT_PROGBITS = 1)
	s2 := buf[int(shOff)+2*shentSize:]
	binary.LittleEndian.PutUint32(s2[0:], 11)
	binary.LittleEndian.PutUint32(s2[4:], 1)
	binary.LittleEndian.PutUint64(s2[24:], debugOff)
	binary.LittleEndian.PutUint64(s2[32:], uint64(len(debugInfo)))

	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}
