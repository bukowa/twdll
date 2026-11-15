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
	// Given a sandbox with IDA and Ghidra databases, logs, analyze_resources.jsonl, and analyzed state
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-clean")

	prodDB := filepath.Join(root, ".twdll", "db", "ida", "attila_32", "attila_32.i64")
	if err := os.MkdirAll(filepath.Dir(prodDB), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prodDB, []byte("production-db"), 0o644); err != nil {
		t.Fatal(err)
	}

	idaMarker := layout.Paths.DatabaseMarkerPath(workspace.DecompilerIDA, workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Dir(idaMarker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(idaMarker, []byte("sandbox-ida-db"), 0o644); err != nil {
		t.Fatal(err)
	}
	idaLog := layout.Paths.AnalyzeLogPath(workspace.DecompilerIDA, workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Dir(idaLog), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(idaLog, []byte("ida-log"), 0o644); err != nil {
		t.Fatal(err)
	}

	ghidraMarker := layout.Paths.DatabaseMarkerPath(workspace.DecompilerGhidra, workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Dir(ghidraMarker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ghidraMarker, []byte("sandbox-ghidra-db"), 0o644); err != nil {
		t.Fatal(err)
	}
	resLog := layout.Paths.AnalyzeResourceLogPath()
	if err := os.WriteFile(resLog, []byte("{\"status\":\"ok\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := workspace.NewState()
	st.UpsertBinary(workspace.TargetAttila32, "src.dll", "hash123")
	st.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerIDA, "hash123")
	st.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerGhidra, "hash123")
	if err := workspace.SaveState(layout.Paths.StateFile, st); err != nil {
		t.Fatal(err)
	}

	// When scoped RunClean (-d ida attila_32) runs against the sandbox layout
	err := RunClean(context.Background(), CleanRequest{
		Layout:  layout,
		Options: CleanOptions{Decompiler: "ida", Targets: []string{"attila_32"}},
		UI:      term.NewUI(term.Streams{ColorMode: term.ColorNever}),
	})
	if err != nil {
		t.Fatalf("RunClean: %v", err)
	}

	// Then only the ida/attila_32 DB, log, and Analyzed.IDA are removed; production DB, ghidra DB, and analyze_resources.jsonl remain intact
	if _, err := os.Stat(idaMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected sandbox IDA DB to be deleted")
	}
	if _, err := os.Stat(idaLog); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected sandbox IDA log to be deleted")
	}
	if _, err := os.Stat(ghidraMarker); err != nil {
		t.Fatalf("expected sibling Ghidra DB to remain untouched: %v", err)
	}
	if _, err := os.Stat(resLog); err != nil {
		t.Fatalf("expected analyze_resources.jsonl to remain untouched on scoped clean: %v", err)
	}
	if _, err := os.Stat(prodDB); err != nil {
		t.Fatalf("production .twdll DB must remain untouched: %v", err)
	}
	after, _, _ := workspace.LoadState(layout.Paths.StateFile)
	if after.Binaries[workspace.TargetAttila32].Analyzed.IDA != "" {
		t.Fatalf("expected IDA analyzed hash cleared, got %q", after.Binaries[workspace.TargetAttila32].Analyzed.IDA)
	}
	if after.Binaries[workspace.TargetAttila32].Analyzed.Ghidra != "hash123" {
		t.Fatalf("expected Ghidra analyzed hash preserved, got %q", after.Binaries[workspace.TargetAttila32].Analyzed.Ghidra)
	}
}

func TestRunClean_UnscopedAndAllAndContradictoryFlags(t *testing.T) {
	t.Run("contradictory flags fail fast without deleting anything", func(t *testing.T) {
		root := t.TempDir()
		layout := newPreseededToolchainSandbox(t, root, ".twdll-contradict")
		dbMarker := layout.Paths.DatabaseMarkerPath(workspace.DecompilerIDA, workspace.TargetAttila32)
		if err := os.MkdirAll(filepath.Dir(dbMarker), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dbMarker, []byte("keep-me"), 0o644); err != nil {
			t.Fatal(err)
		}

		for _, opts := range []CleanOptions{
			{All: true, Decompiler: "ida"},
			{All: true, Targets: []string{"attila_32"}},
		} {
			err := RunClean(context.Background(), CleanRequest{
				Layout:  layout,
				Options: opts,
				UI:      term.NewUI(term.Streams{ColorMode: term.ColorNever}),
			})
			var diag *term.DiagnosticError
			if !errors.As(err, &diag) {
				t.Fatalf("expected *term.DiagnosticError for %+v, got %T (%v)", opts, err, err)
			}
			if _, sErr := os.Stat(dbMarker); sErr != nil {
				t.Fatalf("contradictory clean must not delete %s: %v", dbMarker, sErr)
			}
			if _, sErr := os.Stat(layout.Paths.Config); sErr != nil {
				t.Fatalf("contradictory clean must not delete %s: %v", layout.Paths.Config, sErr)
			}
		}
	})

	t.Run("unscoped clean removes all DBs logs and resource log while preserving init artifacts", func(t *testing.T) {
		root := t.TempDir()
		layout := newPreseededToolchainSandbox(t, root, ".twdll-unscoped")
		paths := layout.Paths

		binFile := paths.BinaryPath(workspace.TargetAttila32)
		if err := os.MkdirAll(filepath.Dir(binFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binFile, []byte("staged-pe32"), 0o644); err != nil {
			t.Fatal(err)
		}

		for _, eng := range workspace.AllDecompilers() {
			marker := paths.DatabaseMarkerPath(eng, workspace.TargetAttila32)
			if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(marker, []byte("db"), 0o644); err != nil {
				t.Fatal(err)
			}
			logPath := paths.AnalyzeLogPath(eng, workspace.TargetAttila32)
			if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(logPath, []byte("log"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		resLog := paths.AnalyzeResourceLogPath()
		if err := os.WriteFile(resLog, []byte("{\"status\":\"ok\"}\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		st := workspace.NewState()
		st.SetTool(workspace.ToolGhidra, workspace.ToolState{Version: "12.0.4", SHA256: "toolhash"})
		st.UpsertBinary(workspace.TargetAttila32, "C:/Games/Attila/empire.retail.dll", "binhash")
		st.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerGhidra, "binhash")
		st.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerIDA, "binhash")
		if err := workspace.SaveState(paths.StateFile, st); err != nil {
			t.Fatal(err)
		}

		err := RunClean(context.Background(), CleanRequest{
			Layout:  layout,
			Options: CleanOptions{},
			UI:      term.NewUI(term.Streams{ColorMode: term.ColorNever}),
		})
		if err != nil {
			t.Fatalf("unscoped RunClean: %v", err)
		}

		for _, removed := range []string{
			paths.DatabaseDir(workspace.DecompilerGhidra, workspace.TargetAttila32),
			paths.DatabaseDir(workspace.DecompilerIDA, workspace.TargetAttila32),
			paths.AnalyzeLogPath(workspace.DecompilerGhidra, workspace.TargetAttila32),
			paths.AnalyzeLogPath(workspace.DecompilerIDA, workspace.TargetAttila32),
			resLog,
		} {
			if _, sErr := os.Stat(removed); !errors.Is(sErr, os.ErrNotExist) {
				t.Fatalf("expected %s to be deleted by unscoped clean, stat err = %v", removed, sErr)
			}
		}

		// Config, staged binaries, and init state entries must remain intact
		for _, kept := range []string{paths.Config, binFile, paths.StateFile} {
			if _, sErr := os.Stat(kept); sErr != nil {
				t.Fatalf("expected %s to be preserved by unscoped clean: %v", kept, sErr)
			}
		}
		after, _, err := workspace.LoadState(paths.StateFile)
		if err != nil {
			t.Fatal(err)
		}
		if after.Tools[workspace.ToolGhidra].Version != "12.0.4" {
			t.Fatalf("expected state.Tools preserved, got %+v", after.Tools)
		}
		b := after.Binaries[workspace.TargetAttila32]
		if b.SourcePath != "C:/Games/Attila/empire.retail.dll" || b.BinarySHA256 != "binhash" {
			t.Fatalf("expected binary staging metadata preserved, got %+v", b)
		}
		if b.Analyzed.Ghidra != "" || b.Analyzed.IDA != "" {
			t.Fatalf("expected Analyzed hashes cleared, got %+v", b.Analyzed)
		}
	})

	t.Run("clean --all wipes workspace while preserving .twdll/bin go toolchain and external TWDLL_TOOLS_DIR", func(t *testing.T) {
		// Case A: internal ToolsDir with go toolchain and .twdll/bin
		root := t.TempDir()
		layout := newPreseededToolchainSandbox(t, root, ".twdll")
		paths := layout.Paths

		cliExe := filepath.Join(paths.BinDir, "twdll.exe")
		if err := os.MkdirAll(filepath.Dir(cliExe), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cliExe, []byte("cli-bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		goExe := filepath.Join(paths.ToolDir(workspace.ToolGo), "bin", "go.exe")
		if err := os.MkdirAll(filepath.Dir(goExe), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goExe, []byte("go-bin"), 0o755); err != nil {
			t.Fatal(err)
		}

		binFile := paths.BinaryPath(workspace.TargetAttila32)
		if err := os.MkdirAll(filepath.Dir(binFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binFile, []byte("bin"), 0o644); err != nil {
			t.Fatal(err)
		}
		dbFile := paths.DatabaseMarkerPath(workspace.DecompilerIDA, workspace.TargetAttila32)
		if err := os.MkdirAll(filepath.Dir(dbFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dbFile, []byte("db"), 0o644); err != nil {
			t.Fatal(err)
		}
		logFile := paths.AnalyzeResourceLogPath()
		if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(logFile, []byte("log"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := workspace.SaveState(paths.StateFile, workspace.NewState()); err != nil {
			t.Fatal(err)
		}

		err := RunClean(context.Background(), CleanRequest{
			Layout:  layout,
			Options: CleanOptions{All: true},
			UI:      term.NewUI(term.Streams{ColorMode: term.ColorNever}),
		})
		if err != nil {
			t.Fatalf("RunClean(--all): %v", err)
		}

		for _, removed := range []string{
			paths.Config,
			paths.StateFile,
			paths.BinsDir,
			paths.DBDir,
			paths.LogsDir,
			paths.ToolDir(workspace.ToolGhidra),
			paths.ToolDir(workspace.ToolLLVM),
		} {
			if _, sErr := os.Stat(removed); !errors.Is(sErr, os.ErrNotExist) {
				t.Fatalf("expected %s to be deleted by clean --all, stat err = %v", removed, sErr)
			}
		}
		for _, preserved := range []string{cliExe, goExe} {
			if _, sErr := os.Stat(preserved); sErr != nil {
				t.Fatalf("expected %s to be preserved by clean --all: %v", preserved, sErr)
			}
		}

		// Case B: external TWDLL_TOOLS_DIR must remain completely untouched
		extRoot := t.TempDir()
		extTools := filepath.Join(t.TempDir(), "ext-tools")
		t.Setenv("TWDLL_TOOLS_DIR", extTools)
		extLayout := newPreseededToolchainSandbox(t, extRoot, ".twdll-ext")
		extSentinel := filepath.Join(extTools, "ghidra", "sentinel.txt")
		if err := os.WriteFile(extSentinel, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := RunClean(context.Background(), CleanRequest{
			Layout:  extLayout,
			Options: CleanOptions{All: true},
			UI:      term.NewUI(term.Streams{ColorMode: term.ColorNever}),
		}); err != nil {
			t.Fatalf("RunClean(--all) with external tools: %v", err)
		}
		if _, sErr := os.Stat(extSentinel); sErr != nil {
			t.Fatalf("external tools sentinel deleted by clean --all: %v", sErr)
		}
	})
}

func TestRunInit_Clear_PreservesExternalToolsDir(t *testing.T) {
	root := t.TempDir()
	extTools := filepath.Join(t.TempDir(), "shared-tools")
	t.Setenv("TWDLL_TOOLS_DIR", extTools)

	layout := newPreseededToolchainSandbox(t, root, ".twdll-ext-clear")
	if !layout.Paths.HasExternalToolsDir() {
		t.Fatalf("expected HasExternalToolsDir() = true for %q", layout.Paths.ToolsDir)
	}

	sentinel := filepath.Join(extTools, "ghidra", "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("shared-tools-sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}

	staleBin := filepath.Join(layout.Paths.BinsDir, "rome2_32", "stale.dll")
	if err := os.MkdirAll(filepath.Dir(staleBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staleBin, []byte("stale-bin"), 0o644); err != nil {
		t.Fatal(err)
	}

	preservedDB := filepath.Join(layout.Paths.DBDir, "ida", "attila_32", "attila_32.i64")
	if err := os.MkdirAll(filepath.Dir(preservedDB), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preservedDB, []byte("analyzed-db"), 0o644); err != nil {
		t.Fatal(err)
	}
	preservedLog := layout.Paths.AnalyzeLogPath(workspace.DecompilerIDA, workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Dir(preservedLog), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preservedLog, []byte("analyzed-log"), 0o644); err != nil {
		t.Fatal(err)
	}
	preservedResLog := layout.Paths.AnalyzeResourceLogPath()
	if err := os.WriteFile(preservedResLog, []byte("{\"status\":\"ok\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

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
	seedState.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerIDA, dllSHA)
	seedState.UpsertBinary(workspace.TargetAttila64, elfPath, elfSHA)
	seedState.SetAnalyzed(workspace.TargetAttila64, workspace.DecompilerGhidra, elfSHA)
	seedState.UpsertBinary(workspace.TargetRome2_32, "stale/rome2.dll", "stalehash")
	if err := workspace.SaveState(layout.Paths.StateFile, seedState); err != nil {
		t.Fatal(err)
	}

	err = RunInit(context.Background(), InitRequest{
		Layout: layout,
		Options: InitOptions{
			Clear:       true,
			NoInput:     true,
			AttilaDir:   gameDir,
			Attila64Src: elfPath,
		},
		UI: term.NewUI(term.Streams{ColorMode: term.ColorNever}),
	})
	if err != nil {
		t.Fatalf("RunInit(--clear) with external tools dir failed: %v", err)
	}

	data, rErr := os.ReadFile(sentinel)
	if rErr != nil || string(data) != "shared-tools-sentinel" {
		t.Fatalf("external tools sentinel was modified or deleted: err=%v data=%q", rErr, string(data))
	}
	if _, sErr := os.Stat(staleBin); !errors.Is(sErr, os.ErrNotExist) {
		t.Fatalf("expected stale BinsDir artifact to be removed by init --clear, stat err = %v", sErr)
	}
	for _, preserved := range []string{preservedDB, preservedLog, preservedResLog} {
		if _, sErr := os.Stat(preserved); sErr != nil {
			t.Fatalf("expected analyze artifact %s to be preserved by init --clear: %v", preserved, sErr)
		}
	}

	afterState, _, err := workspace.LoadState(layout.Paths.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if !afterState.Binaries[workspace.TargetAttila32].IsAnalyzed(workspace.DecompilerIDA) {
		t.Fatalf("expected attila_32 IDA Analyzed state preserved after init --clear re-staging same binary: %+v", afterState.Binaries[workspace.TargetAttila32])
	}
	if !afterState.Binaries[workspace.TargetAttila64].IsAnalyzed(workspace.DecompilerGhidra) {
		t.Fatalf("expected attila_64 Ghidra Analyzed state preserved after init --clear re-staging same binary: %+v", afterState.Binaries[workspace.TargetAttila64])
	}
	if _, ok := afterState.Binaries[workspace.TargetRome2_32]; ok {
		t.Fatalf("expected unanalyzed stale rome2_32 binary entry to be cleared: %+v", afterState.Binaries[workspace.TargetRome2_32])
	}
}

func newPreseededToolchainSandbox(t *testing.T, root, dataDir string) workspace.Layout {
	t.Helper()
	emptySteam := filepath.Join(root, "EmptySteam")
	if err := os.MkdirAll(filepath.Join(emptySteam, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptySteam, "steamapps", "libraryfolders.vdf"), []byte(`"libraryfolders" {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEAM_DIR", emptySteam)

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
