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
	"time"

	"twdll/cli/internal/bronze"
	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/silver"
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
		extractedBronze := paths.BronzeDBPath(workspace.TargetAttila32)
		if err := os.MkdirAll(filepath.Dir(extractedBronze), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(extractedBronze, []byte("bronze-sqlite"), 0o644); err != nil {
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

		// Config, staged binaries, extracted DBs, and init state entries must remain intact
		for _, kept := range []string{paths.Config, binFile, extractedBronze, paths.StateFile} {
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
		extractedFile := paths.BronzeDBPath(workspace.TargetAttila32)
		if err := os.MkdirAll(filepath.Dir(extractedFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(extractedFile, []byte("facts"), 0o644); err != nil {
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
			paths.ExtractedDir,
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
	preservedExtracted := layout.Paths.BronzeDBPath(workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Dir(preservedExtracted), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preservedExtracted, []byte("extracted-binary"), 0o644); err != nil {
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
	for _, preserved := range []string{preservedDB, preservedLog, preservedResLog, preservedExtracted} {
		if _, sErr := os.Stat(preserved); sErr != nil {
			t.Fatalf("expected analyze/extract artifact %s to be preserved by init --clear: %v", preserved, sErr)
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

func TestRunInit_SelectiveGamesAndJSON_PrunesUnselectedGames(t *testing.T) {
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-selective")

	attilaDir := filepath.Join(root, "Games", "Attila")
	rome2Dir := filepath.Join(root, "Games", "Rome2")
	for _, d := range []string{attilaDir, rome2Dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		writeMinimalPE32(t, filepath.Join(d, "empire.retail.dll"))
	}
	elfPath := filepath.Join(root, "Attila64")
	writeMinimalELF64DWARF(t, elfPath)

	// First initialize with both attila and rome2
	err := RunInit(context.Background(), InitRequest{
		Layout: layout,
		Options: InitOptions{
			NoInput:     true,
			Games:       []string{"attila,rome2"},
			AttilaDir:   attilaDir,
			Rome2Dir:    rome2Dir,
			Attila64Src: elfPath,
		},
		UI: term.NewUI(term.Streams{ColorMode: term.ColorNever}),
	})
	if err != nil {
		t.Fatalf("first RunInit failed: %v", err)
	}
	if _, err := os.Stat(layout.Paths.BinaryPath(workspace.TargetAttila32)); err != nil {
		t.Fatalf("expected attila_32 staged initially: %v", err)
	}

	// Re-run selecting only rome2 with --json
	reloadedCfg, _, err := workspace.LoadConfig(layout.Paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	layout.Config = reloadedCfg

	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{Out: &stdout, Err: &stderr, ColorMode: term.ColorNever})
	err = RunInit(context.Background(), InitRequest{
		Layout: layout,
		Options: InitOptions{
			NoInput:     true,
			JSON:        true,
			Games:       []string{"rome2"},
			Attila64Src: elfPath,
		},
		UI: ui,
	})
	if err != nil {
		t.Fatalf("selective RunInit failed: %v\nstderr:\n%s", err, stderr.String())
	}

	var report InitReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON on stdout: %v\nraw: %s", err, stdout.String())
	}
	if report.DefaultGame != "rome2" {
		t.Fatalf("DefaultGame = %q, want rome2", report.DefaultGame)
	}
	if len(report.EnabledGames) != 1 || report.EnabledGames[0] != workspace.GameRome2 {
		t.Fatalf("EnabledGames = %v, want [rome2]", report.EnabledGames)
	}
	if len(report.EnabledTargets) != 2 || report.EnabledTargets[0] != workspace.TargetAttila64 || report.EnabledTargets[1] != workspace.TargetRome2_32 {
		t.Fatalf("EnabledTargets = %v, want [attila_64 rome2_32]", report.EnabledTargets)
	}

	finalCfg, _, err := workspace.LoadConfig(layout.Paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if finalCfg.Games.Attila != "" || finalCfg.Games.Rome2 != filepath.ToSlash(rome2Dir) {
		t.Fatalf("unexpected cfg.Games after pruning: %+v", finalCfg.Games)
	}
	if _, err := os.Stat(layout.Paths.BinaryPath(workspace.TargetAttila32)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected pruned attila_32 binary to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(layout.Paths.BinaryPath(workspace.TargetRome2_32)); err != nil {
		t.Fatalf("expected rome2_32 binary to remain staged: %v", err)
	}
}

func TestRunInit_ConflictingGamesAndDirFlag_ReturnsDiagnosticError(t *testing.T) {
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-conflict")

	err := RunInit(context.Background(), InitRequest{
		Layout: layout,
		Options: InitOptions{
			NoInput:  true,
			Games:    []string{"attila"},
			Rome2Dir: filepath.Join(root, "Rome2"),
		},
		UI: term.NewUI(term.Streams{ColorMode: term.ColorNever}),
	})
	var diag *term.DiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError, got %T (%v)", err, err)
	}
}

func TestRunInit_InteractiveMultiSelect_StagesOnlySelectedGames(t *testing.T) {
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-multi")

	rome2Dir := filepath.Join(root, "Games", "Rome2")
	if err := os.MkdirAll(rome2Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMinimalPE32(t, filepath.Join(rome2Dir, "empire.retail.dll"))
	elfPath := filepath.Join(root, "Attila64")
	writeMinimalELF64DWARF(t, elfPath)

	// Input sequence:
	// 1. MultiSelect games -> "2" (rome2 only)
	// 2. Select decompiler -> "1" (ghidra)
	// 3. Input Rome2 path -> rome2Dir
	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{
		In:        strings.NewReader("2\n1\n" + rome2Dir + "\n"),
		Out:       &stdout,
		Err:       &stderr,
		ColorMode: term.ColorNever,
	})
	prompter := term.NewLinePrompter(ui)

	err := RunInit(context.Background(), InitRequest{
		Layout: layout,
		Options: InitOptions{
			Attila64Src: elfPath,
		},
		Prompter: prompter,
		UI:       ui,
	})
	if err != nil {
		t.Fatalf("interactive RunInit failed: %v\nstderr:\n%s", err, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout without --json, got %q", stdout.String())
	}
	cfg, _, err := workspace.LoadConfig(layout.Paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultGame != workspace.GameRome2 || cfg.Games.Rome2 != filepath.ToSlash(rome2Dir) || cfg.Games.Attila != "" || cfg.Games.ToB != "" {
		t.Fatalf("unexpected interactive config: %+v", cfg)
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

func writeValidTestPE32DLL(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const (
		peOffset     = 0x80
		coffOffset   = peOffset + 4
		optOffset    = coffOffset + 20
		optSize      = 224
		secOffset    = optOffset + optSize
		textFileOff  = 0x200
		textFileSize = 0x200
	)
	buf := make([]byte, textFileOff+textFileSize)
	buf[0] = 'M'
	buf[1] = 'Z'
	binary.LittleEndian.PutUint32(buf[0x3c:], peOffset)
	copy(buf[peOffset:], []byte{'P', 'E', 0, 0})

	// COFF Header
	binary.LittleEndian.PutUint16(buf[coffOffset:], 0x014c) // i386
	binary.LittleEndian.PutUint16(buf[coffOffset+2:], 1)    // NumberOfSections
	binary.LittleEndian.PutUint16(buf[coffOffset+16:], optSize)
	binary.LittleEndian.PutUint16(buf[coffOffset+18:], 0x2102) // EXECUTABLE_IMAGE | 32BIT_MACHINE | DLL

	// OptionalHeader32
	binary.LittleEndian.PutUint16(buf[optOffset:], 0x010b)         // PE32
	binary.LittleEndian.PutUint32(buf[optOffset+16:], 0x1000)      // AddressOfEntryPoint
	binary.LittleEndian.PutUint32(buf[optOffset+28:], 0x10000000)  // ImageBase
	binary.LittleEndian.PutUint32(buf[optOffset+32:], 0x1000)      // SectionAlignment
	binary.LittleEndian.PutUint32(buf[optOffset+36:], 0x200)       // FileAlignment
	binary.LittleEndian.PutUint16(buf[optOffset+40:], 6)           // MajorOSVersion
	binary.LittleEndian.PutUint16(buf[optOffset+48:], 6)           // MajorSubsystemVersion
	binary.LittleEndian.PutUint32(buf[optOffset+56:], 0x2000)      // SizeOfImage
	binary.LittleEndian.PutUint32(buf[optOffset+60:], textFileOff) // SizeOfHeaders
	binary.LittleEndian.PutUint16(buf[optOffset+68:], 2)           // WINDOWS_GUI
	binary.LittleEndian.PutUint32(buf[optOffset+92:], 16)          // NumberOfRvaAndSizes

	// Section Header 0: .text
	copy(buf[secOffset:], []byte(".text\x00\x00\x00"))
	binary.LittleEndian.PutUint32(buf[secOffset+8:], 0x10)
	binary.LittleEndian.PutUint32(buf[secOffset+12:], 0x1000)
	binary.LittleEndian.PutUint32(buf[secOffset+16:], textFileSize)
	binary.LittleEndian.PutUint32(buf[secOffset+20:], textFileOff)
	binary.LittleEndian.PutUint32(buf[secOffset+36:], 0x60000020) // CODE | EXECUTE | READ

	// Tiny cdecl/simple function body: push ebp; mov ebp, esp; pop ebp; ret
	copy(buf[textFileOff:], []byte{0x55, 0x8b, 0xec, 0x5d, 0xc3})

	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunExtract_NoDecomp_ExtractsBronzeAndSkipsWhenUpToDate(t *testing.T) {
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-test")
	writeValidTestPE32DLL(t, layout.Paths.BinaryPath(workspace.TargetAttila32))

	var stdout1, stderr1 bytes.Buffer
	ui1 := term.NewUI(term.Streams{Out: &stdout1, Err: &stderr1, ColorMode: term.ColorNever})
	rep1, err := RunExtract(context.Background(), ExtractRequest{
		Layout: layout,
		Options: ExtractOptions{
			NoDecomp: true,
			Targets:  []string{"attila_32"},
		},
		UI: ui1,
	})
	if err != nil {
		t.Fatalf("first RunExtract failed: %v\nstderr:\n%s", err, stderr1.String())
	}
	if _, statErr := os.Stat(layout.Paths.BronzeDBPath(workspace.TargetAttila32)); statErr != nil {
		t.Fatalf("expected bronze.sqlite to exist: %v", statErr)
	}
	if len(rep1.Targets) != 1 || !rep1.Targets[0].BronzeRan {
		t.Fatalf("expected BronzeRan=true on first extract, got %+v", rep1)
	}
	if len(rep1.Targets[0].Engines) != 1 || !rep1.Targets[0].Engines[0].SilverSkipped {
		t.Fatalf("expected SilverSkipped=true without analyzed DB, got %+v", rep1.Targets[0].Engines)
	}

	var stdout2, stderr2 bytes.Buffer
	ui2 := term.NewUI(term.Streams{Out: &stdout2, Err: &stderr2, ColorMode: term.ColorNever})
	rep2, err := RunExtract(context.Background(), ExtractRequest{
		Layout: layout,
		Options: ExtractOptions{
			NoDecomp: true,
			Targets:  []string{"attila_32"},
		},
		UI: ui2,
	})
	if err != nil {
		t.Fatalf("second RunExtract failed: %v\nstderr:\n%s", err, stderr2.String())
	}
	if len(rep2.Targets) != 1 || rep2.Targets[0].BronzeRan {
		t.Fatalf("expected BronzeRan=false when bronze.sqlite is already up to date, got %+v", rep2)
	}
}

func TestRunExtract_FullPipelineAndFlags_UsesInjectedRunners(t *testing.T) {
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-test")
	writeValidTestPE32DLL(t, layout.Paths.BinaryPath(workspace.TargetAttila32))

	var silverCalls []silver.Options
	var order []string

	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{Out: &stdout, Err: &stderr, ColorMode: term.ColorNever})
	rep, err := RunExtract(context.Background(), ExtractRequest{
		Layout: layout,
		Options: ExtractOptions{
			Decompiler: "all",
			Jobs:       4,
			MaxMem:     "4G",
			Retry:      true,
			JSON:       true,
			Targets:    []string{"attila_32"},
		},
		UI: ui,
		ExtractBronze: func(_ context.Context, opts bronze.Options) (*bronze.Result, error) {
			order = append(order, "bronze")
			return &bronze.Result{Target: opts.Target, Counts: map[string]int64{"sections": 4}}, nil
		},
		ExtractSilver: func(_ context.Context, opts silver.Options) (*silver.Result, error) {
			order = append(order, "silver:"+string(opts.Engine))
			silverCalls = append(silverCalls, opts)
			return &silver.Result{
				Target:        opts.Target,
				Engine:        opts.Engine,
				FuncsRan:      true,
				FuncCount:     10,
				DecompRan:     true,
				DecompWritten: 5,
				StatusCounts:  map[string]int{extractdb.DecompStatusOK: 4, extractdb.DecompStatusFailed: 1},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunExtract failed: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr on extract --json, got %q", stderr.String())
	}
	wantOrder := []string{"bronze", "silver:ghidra", "silver:ida"}
	if strings.Join(order, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("execution order = %v, want %v", order, wantOrder)
	}
	for _, sc := range silverCalls {
		if sc.Workers != 4 || sc.MaxMem != "4G" || !sc.RetryFailed || !sc.RetryTimeouts || sc.NoDecomp {
			t.Fatalf("unexpected silver.Options forwarded: %+v", sc)
		}
		if !strings.HasSuffix(filepath.ToSlash(sc.SilverDBPath), "/silver_"+string(sc.Engine)+".sqlite") {
			t.Fatalf("unexpected SilverDBPath: %q", sc.SilverDBPath)
		}
	}

	var decoded ExtractReport
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON extract report: %v\nraw:\n%s", err, stdout.String())
	}
	if len(decoded.Targets) != 1 || len(decoded.Targets[0].Engines) != 2 {
		t.Fatalf("unexpected decoded report: %+v", decoded)
	}
	if decoded.Targets[0].Engines[0].Engine != "ghidra" || decoded.Targets[0].Engines[1].Engine != "ida" {
		t.Fatalf("expected raw engine names, got %+v", decoded.Targets[0].Engines)
	}
	if rep.Targets[0].Engines[0].DecompWritten != 5 || rep.Targets[0].Engines[0].FuncCount != 10 {
		t.Fatalf("unexpected engine summary: %+v", rep.Targets[0].Engines[0])
	}
}

func TestRunStatus_ReportsExtractedDatabases(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-test")
	binPath := layout.Paths.BinaryPath(workspace.TargetAttila32)
	writeValidTestPE32DLL(t, binPath)

	sha, err := workspace.ComputeFileSHA256(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bronze.ExtractTarget(ctx, bronze.Options{
		Target:       workspace.TargetAttila32,
		BinaryPath:   binPath,
		BronzeDBPath: layout.Paths.BronzeDBPath(workspace.TargetAttila32),
		ManifestPath: layout.Paths.TargetManifestPath(workspace.TargetAttila32),
	}); err != nil {
		t.Fatal(err)
	}

	store, err := extractdb.OpenSilverStore(ctx, layout.Paths.SilverDBPath(workspace.TargetAttila32, workspace.DecompilerGhidra), "attila_32", "ghidra", sha, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceFuncs(ctx, []extractdb.SilverFuncRow{
		{EA: 0x10001000, Size: 5, Name: "sub_10001000"},
	}, nil, "funcs"); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.ReplaceAsm(ctx, []extractdb.SilverAsmRow{
		{FuncEA: 0x10001000, InsnCount: 1, RawAsm: "10001000: ret"},
	}, ""); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.WriteBatch(ctx, []extractdb.DecompRecord{
		{FuncEA: 0x10001000, FuncSize: 5, FuncName: "sub_10001000", RawC: "void sub_10001000(void) {}", Status: extractdb.DecompStatusOK},
	}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{Out: &stdout, Err: &stderr, ColorMode: term.ColorNever})
	report, err := RunStatus(ctx, StatusRequest{
		Layout: layout,
		JSON:   false,
		UI:     ui,
	})
	if err != nil {
		t.Fatalf("RunStatus failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Extracted Databases:") {
		t.Fatalf("expected human status output to contain Extracted Databases section:\n%s", stdout.String())
	}

	var found *ExtractedStatus
	for i := range report.Extracted {
		if report.Extracted[i].Target == workspace.TargetAttila32 {
			found = &report.Extracted[i]
			break
		}
	}
	if found == nil || !found.BronzePresent || !found.BronzeUpToDate {
		t.Fatalf("expected attila_32 BronzePresent & BronzeUpToDate, got %+v", found)
	}
	ghSilver := found.Silver[string(workspace.DecompilerGhidra)]
	if !ghSilver.Present || !ghSilver.UpToDate || ghSilver.Funcs != 1 || ghSilver.Asm != 1 || ghSilver.OK != 1 {
		t.Fatalf("unexpected ghidra silver status: %+v", ghSilver)
	}
}

func TestRunStatus_BronzeBinarySHAMismatchMarksNotUpToDate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-test")
	binPath := layout.Paths.BinaryPath(workspace.TargetAttila32)
	writeValidTestPE32DLL(t, binPath)

	bronzePath := layout.Paths.BronzeDBPath(workspace.TargetAttila32)
	b, err := extractdb.NewAtomicBuilder(bronzePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Abort() }()
	if err := extractdb.InitBronzeSchema(ctx, b.DB()); err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{
		{extractdb.MetaTarget, "attila_32"},
		{extractdb.MetaBinarySHA256, "stale_sha_value"},
	} {
		if _, err := b.DB().ExecContext(ctx, `INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	report, err := RunStatus(ctx, StatusRequest{
		Layout: layout,
		JSON:   true,
		UI:     term.NewUI(term.Streams{Out: &stdout, Err: &stderr, ColorMode: term.ColorNever}),
	})
	if err != nil {
		t.Fatalf("RunStatus failed: %v", err)
	}
	var found *ExtractedStatus
	for i := range report.Extracted {
		if report.Extracted[i].Target == workspace.TargetAttila32 {
			found = &report.Extracted[i]
			break
		}
	}
	if found == nil || !found.BronzePresent || found.BronzeUpToDate {
		t.Fatalf("expected BronzePresent=true and BronzeUpToDate=false on binary_sha256 mismatch, got %+v", found)
	}
}

func TestRunExtract_AnimatedDashboard_RendersAllTargetsAndFinalOKRows(t *testing.T) {
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-test")
	writeValidTestPE32DLL(t, layout.Paths.BinaryPath(workspace.TargetAttila32))
	writeValidTestPE32DLL(t, layout.Paths.BinaryPath(workspace.TargetRome2_32))

	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{
		Out:         &stdout,
		Err:         &stderr,
		ColorMode:   term.ColorAlways,
		ForceTTYErr: true,
	})
	rep, err := RunExtract(context.Background(), ExtractRequest{
		Layout: layout,
		Options: ExtractOptions{
			Decompiler: "all",
			Targets:    []string{"attila_32", "rome2_32"},
		},
		UI: ui,
		ExtractSilver: func(_ context.Context, opts silver.Options) (*silver.Result, error) {
			if opts.OnPhase != nil {
				opts.OnPhase("decomp")
			}
			if opts.OnProgress != nil {
				opts.OnProgress(6, 12)
				opts.OnProgress(12, 12)
			}
			return &silver.Result{
				Target:          opts.Target,
				Engine:          opts.Engine,
				FuncsRan:        true,
				FuncCount:       12,
				AsmCount:        12,
				DecompRan:       true,
				DecompWritten:   12,
				StatusCounts:    map[string]int{extractdb.DecompStatusOK: 12},
				PeakJobMemBytes: 256 * 1024 * 1024,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunExtract failed: %v\nstderr:\n%s", err, stderr.String())
	}
	if len(rep.Targets) != 2 {
		t.Fatalf("len(Targets) = %d, want 2", len(rep.Targets))
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout without --json, got %q", stdout.String())
	}
	errOut := stderr.String()
	for _, want := range []string{"Extracting (ghidra, ida)", "queued", "attila_32", "rome2_32", "bronze (format)", "silver (ghidra)", "silver (ida)", "12 fn, 12 asm | ok=12 | pk 256 MiB", "[ok]"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("expected live dashboard stderr to contain %q\ngot:\n%s", want, errOut)
		}
	}
}

func TestExtractObserver_RendersCountOnlyAndPercentProgress(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{
		Out:         &stdout,
		Err:         &stderr,
		ColorMode:   term.ColorAlways,
		ForceTTYErr: true,
	})
	obs := newExtractObserver(ui, []workspace.DecompilerID{workspace.DecompilerGhidra, workspace.DecompilerIDA}, ExtractOptions{}, []plannedExtractTarget{
		{target: workspace.TargetAttila32},
	})
	ghidraKey := extractRowKey{target: workspace.TargetAttila32, step: "ghidra"}
	idaKey := extractRowKey{target: workspace.TargetAttila32, step: "ida"}
	obs.RowStarted(ghidraKey, "funcs+asm...")
	obs.RowProgress(ghidraKey, "funcs+asm", 4200, 0)
	obs.mu.Lock()
	obs.renderLocked()
	obs.mu.Unlock()
	if !strings.Contains(stderr.String(), "funcs+asm: 4200 fn") {
		t.Fatalf("expected count-only progress line, got:\n%s", stderr.String())
	}

	stderr.Reset()
	obs.RowProgress(ghidraKey, "decomp", 50, 100)
	obs.mu.Lock()
	rs := obs.states[ghidraKey]
	now := time.Now()
	rs.samples = []progressSample{
		{at: now.Add(-5 * time.Second), done: 0},
		{at: now, done: 50},
	}
	rs.etaSmooth = 0
	obs.renderLocked()
	obs.mu.Unlock()
	if !strings.Contains(stderr.String(), "decomp: 50/100 (50.0%, 50 left, ~5s)") {
		t.Fatalf("expected percentage/left/ETA progress line, got:\n%s", stderr.String())
	}

	// Simulate tail slowdown (only 10 items in last 10s -> 1/s -> 40 left = ~40s, not anchored to early burst).
	stderr.Reset()
	obs.mu.Lock()
	rs.samples = []progressSample{
		{at: now.Add(-10 * time.Second), done: 50},
		{at: now, done: 60},
	}
	rs.done = 60
	rs.etaSmooth = 0
	obs.renderLocked()
	obs.mu.Unlock()
	if !strings.Contains(stderr.String(), "decomp: 60/100 (60.0%, 40 left, ~40s)") {
		t.Fatalf("expected sliding-window ETA to adapt to tail slowdown, got:\n%s", stderr.String())
	}

	// Simulate byte-weighted ETA when 75% of functions processed but only 20% of bytes (200/1000 bytes in 10s -> 40s left).
	stderr.Reset()
	obs.RowBytes(ghidraKey, 200, 1000)
	obs.mu.Lock()
	rs.done = 75
	rs.byteSamples = []progressSample{
		{at: now.Add(-10 * time.Second), done: 0},
		{at: now, done: 200},
	}
	rs.etaSmooth = 0
	obs.renderLocked()
	obs.mu.Unlock()
	if !strings.Contains(stderr.String(), "decomp: 75/100 (75.0%, 25 left, ~40s)") {
		t.Fatalf("expected byte-weighted ETA line, got:\n%s", stderr.String())
	}

	// Simulate ghidra finished and ida starting: both rows stay visible in the dashboard.
	stderr.Reset()
	obs.RowFinished(ghidraKey, "86295 fn, 86295 asm | ok=86291 timeout=4 | pk 4.1 GiB")
	obs.RowStarted(idaKey, "decomp...")
	obs.RowBytes(idaKey, 0, 1000)
	obs.RowProgress(idaKey, "decomp", 0, 100)
	obs.mu.Lock()
	irs := obs.states[idaKey]
	if irs.totalBytes != 1000 {
		obs.mu.Unlock()
		t.Fatalf("expected totalBytes preserved across phase ellipsis transition, got %d", irs.totalBytes)
	}
	obs.mu.Unlock()

	obs.RowProgress(idaKey, "decomp", 50, 100)
	obs.RowBytes(idaKey, 200, 1000)
	obs.mu.Lock()
	irs.byteSamples = []progressSample{
		{at: now.Add(-10 * time.Second), done: 0},
		{at: now, done: 200},
	}
	irs.etaSmooth = 0
	obs.mu.Unlock()
	// Stale out-of-order lower progress count (e.g. 49 after 50) must be ignored without clearing ETA samples.
	obs.RowProgress(idaKey, "decomp", 49, 100)
	obs.mu.Lock()
	obs.renderLocked()
	obs.mu.Unlock()
	out := stderr.String()
	if !strings.Contains(out, "silver (ghidra)") || !strings.Contains(out, "ok=86291 timeout=4") {
		t.Fatalf("expected finished ghidra row to stay visible, got:\n%s", out)
	}
	if !strings.Contains(out, "silver (ida)") || !strings.Contains(out, "decomp: 50/100 (50.0%, 50 left, ~40s)") {
		t.Fatalf("expected active ida row with preserved done and byte ETA, got:\n%s", out)
	}
}

func TestExtractObserver_ResumeAndStatusTransitions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{
		Out:         &stdout,
		Err:         &stderr,
		ColorMode:   term.ColorAlways,
		ForceTTYErr: true,
	})
	planned := []plannedExtractTarget{
		{target: workspace.TargetAttila64},
	}
	obs := newExtractObserver(ui, []workspace.DecompilerID{workspace.DecompilerGhidra}, ExtractOptions{}, planned)
	ghKey := extractRowKey{target: workspace.TargetAttila64, step: string(workspace.DecompilerGhidra)}

	obs.RowStarted(ghKey, "starting...")
	obs.RowPhase(ghKey, "decomp...")
	obs.RowResume(ghKey, 900, 100)
	obs.RowBytes(ghKey, 0, 1000)
	obs.RowProgress(ghKey, "decomp", 0, 100)
	obs.RowStatus(ghKey, "loading types")

	stderr.Reset()
	obs.mu.Lock()
	obs.renderLocked()
	obs.mu.Unlock()
	if !strings.Contains(stderr.String(), "loading types: 900/1000 (90.0%, 100 left)") {
		t.Fatalf("expected loading types with cached/total progress, got:\n%s", stderr.String())
	}

	obs.RowStatus(ghKey, "decomp")
	obs.RowProgress(ghKey, "decomp", 25, 100)
	obs.RowBytes(ghKey, 500, 1000)
	now := time.Now()
	obs.mu.Lock()
	rs := obs.states[ghKey]
	rs.byteSamples = []progressSample{
		{at: now.Add(-10 * time.Second), done: 0},
		{at: now, done: 500},
	}
	rs.etaSmooth = 0
	stderr.Reset()
	obs.renderLocked()
	obs.mu.Unlock()
	if !strings.Contains(stderr.String(), "decomp: 925/1000 (92.5%, 75 left, ~10s)") {
		t.Fatalf("expected overall resumed progress with pending-only ETA, got:\n%s", stderr.String())
	}
}

func TestRunExtract_RejectsStaleDatabaseWhenAnalyzedSHA256Differs(t *testing.T) {
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-test")
	binPath := layout.Paths.BinaryPath(workspace.TargetAttila32)
	writeValidTestPE32DLL(t, binPath)

	dbDir := layout.Paths.DatabaseDir(workspace.DecompilerGhidra, workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Join(dbDir, "attila_32.rep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dbDir, "attila_32.gpr"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !layout.Paths.HasValidDatabase(workspace.DecompilerGhidra, workspace.TargetAttila32) {
		t.Fatalf("expected valid on-disk Ghidra database in %s", dbDir)
	}

	st := workspace.NewState()
	st.UpsertBinary(workspace.TargetAttila32, binPath, "old-sha")
	st.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerGhidra, "old-sha")
	if err := workspace.SaveState(layout.Paths.StateFile, st); err != nil {
		t.Fatal(err)
	}

	_, err := RunExtract(context.Background(), ExtractRequest{
		Layout: layout,
		Options: ExtractOptions{
			Targets: []string{"attila_32"},
		},
		UI: term.NewUI(term.Streams{ColorMode: term.ColorNever}),
	})
	var diag *term.DiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError for stale DB, got %T (%v)", err, err)
	}
	if !strings.Contains(diag.Message, "is stale (binary changed since last analyze)") {
		t.Fatalf("unexpected diagnostic message: %q", diag.Message)
	}

	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{Out: &stdout, Err: &stderr, ColorMode: term.ColorNever})
	rep, err := RunExtract(context.Background(), ExtractRequest{
		Layout: layout,
		Options: ExtractOptions{
			NoDecomp: true,
			Targets:  []string{"attila_32"},
		},
		UI: ui,
	})
	if err != nil {
		t.Fatalf("RunExtract --no-decomp with stale DB failed: %v", err)
	}
	if len(rep.Targets) != 1 || len(rep.Targets[0].Engines) != 1 {
		t.Fatalf("unexpected report targets/engines: %+v", rep)
	}
	eng := rep.Targets[0].Engines[0]
	if !eng.SilverSkipped || eng.FuncsRan {
		t.Fatalf("expected SilverSkipped=true and FuncsRan=false for stale DB, got %+v", eng)
	}
	if !strings.Contains(stderr.String(), "decompiler database missing or stale") {
		t.Fatalf("expected stderr to warn about missing or stale DB, got:\n%s", stderr.String())
	}
}

func TestRunExtract_ForwardsSplitRetryAndTimeoutFlags(t *testing.T) {
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-test")
	writeValidTestPE32DLL(t, layout.Paths.BinaryPath(workspace.TargetAttila32))

	t.Run("retry-timeouts and timeout-secs", func(t *testing.T) {
		var got silver.Options
		_, err := RunExtract(context.Background(), ExtractRequest{
			Layout: layout,
			Options: ExtractOptions{
				TimeoutSecs:   60,
				RetryTimeouts: true,
				Targets:       []string{"attila_32"},
			},
			UI: term.NewUI(term.Streams{ColorMode: term.ColorNever}),
			ExtractSilver: func(_ context.Context, opts silver.Options) (*silver.Result, error) {
				got = opts
				return &silver.Result{Target: opts.Target, Engine: opts.Engine}, nil
			},
		})
		if err != nil {
			t.Fatalf("RunExtract failed: %v", err)
		}
		if got.TimeoutSecs != 60 || got.NoTimeout || !got.RetryTimeouts || got.RetryFailed {
			t.Fatalf("unexpected silver.Options: TimeoutSecs=%d NoTimeout=%v RetryTimeouts=%v RetryFailed=%v",
				got.TimeoutSecs, got.NoTimeout, got.RetryTimeouts, got.RetryFailed)
		}
	})

	t.Run("retry-failed and no-timeout", func(t *testing.T) {
		var got silver.Options
		_, err := RunExtract(context.Background(), ExtractRequest{
			Layout: layout,
			Options: ExtractOptions{
				NoTimeout:   true,
				RetryFailed: true,
				Targets:     []string{"attila_32"},
			},
			UI: term.NewUI(term.Streams{ColorMode: term.ColorNever}),
			ExtractSilver: func(_ context.Context, opts silver.Options) (*silver.Result, error) {
				got = opts
				return &silver.Result{Target: opts.Target, Engine: opts.Engine}, nil
			},
		})
		if err != nil {
			t.Fatalf("RunExtract failed: %v", err)
		}
		if !got.NoTimeout || !got.RetryFailed || got.RetryTimeouts {
			t.Fatalf("unexpected silver.Options: NoTimeout=%v RetryFailed=%v RetryTimeouts=%v",
				got.NoTimeout, got.RetryFailed, got.RetryTimeouts)
		}
	})
}

func TestRunExtract_And_Clean_ExpandTargetGlobs(t *testing.T) {
	root := t.TempDir()
	layout := newPreseededToolchainSandbox(t, root, ".twdll-test")
	attila64Path := layout.Paths.BinaryPath(workspace.TargetAttila64)
	if err := os.MkdirAll(filepath.Dir(attila64Path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMinimalELF64DWARF(t, attila64Path)
	writeValidTestPE32DLL(t, layout.Paths.BinaryPath(workspace.TargetAttila32))
	writeValidTestPE32DLL(t, layout.Paths.BinaryPath(workspace.TargetRome2_32))

	t.Run("extract *_32 expands configured 32-bit targets", func(t *testing.T) {
		rep, err := RunExtract(context.Background(), ExtractRequest{
			Layout: layout,
			Options: ExtractOptions{
				NoDecomp: true,
				Targets:  []string{"*_32"},
			},
			UI: term.NewUI(term.Streams{ColorMode: term.ColorNever}),
			ExtractSilver: func(_ context.Context, opts silver.Options) (*silver.Result, error) {
				return &silver.Result{Target: opts.Target, Engine: opts.Engine}, nil
			},
		})
		if err != nil {
			t.Fatalf("RunExtract(*_32): %v", err)
		}
		if len(rep.Targets) != 2 || rep.Targets[0].Target != workspace.TargetAttila32 || rep.Targets[1].Target != workspace.TargetRome2_32 {
			t.Fatalf("RunExtract(*_32) targets = %+v, want [attila_32, rome2_32]", rep.Targets)
		}
	})

	t.Run("extract attila_* expands attila_64 and attila_32", func(t *testing.T) {
		rep, err := RunExtract(context.Background(), ExtractRequest{
			Layout: layout,
			Options: ExtractOptions{
				NoDecomp: true,
				Targets:  []string{"attila_*"},
			},
			UI: term.NewUI(term.Streams{ColorMode: term.ColorNever}),
			ExtractSilver: func(_ context.Context, opts silver.Options) (*silver.Result, error) {
				return &silver.Result{Target: opts.Target, Engine: opts.Engine}, nil
			},
		})
		if err != nil {
			t.Fatalf("RunExtract(attila_*): %v", err)
		}
		if len(rep.Targets) != 2 || rep.Targets[0].Target != workspace.TargetAttila64 || rep.Targets[1].Target != workspace.TargetAttila32 {
			t.Fatalf("RunExtract(attila_*) targets = %+v, want [attila_64, attila_32]", rep.Targets)
		}
	})

	t.Run("clean *_32 removes only 32-bit DBs and preserves attila_64", func(t *testing.T) {
		for _, tid := range []workspace.TargetID{workspace.TargetAttila64, workspace.TargetAttila32, workspace.TargetRome2_32} {
			dbDir := layout.Paths.DatabaseDir(workspace.DecompilerGhidra, tid)
			if err := os.MkdirAll(filepath.Join(dbDir, string(tid)+".rep"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dbDir, string(tid)+".gpr"), []byte("ok"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := RunClean(context.Background(), CleanRequest{
			Layout:  layout,
			Options: CleanOptions{Decompiler: "ghidra", Targets: []string{"*_32"}},
			UI:      term.NewUI(term.Streams{ColorMode: term.ColorNever}),
		}); err != nil {
			t.Fatalf("RunClean(*_32): %v", err)
		}
		if !layout.Paths.HasValidDatabase(workspace.DecompilerGhidra, workspace.TargetAttila64) {
			t.Fatalf("expected attila_64 DB preserved after clean *_32")
		}
		if layout.Paths.HasValidDatabase(workspace.DecompilerGhidra, workspace.TargetAttila32) {
			t.Fatalf("expected attila_32 DB removed after clean *_32")
		}
		if layout.Paths.HasValidDatabase(workspace.DecompilerGhidra, workspace.TargetRome2_32) {
			t.Fatalf("expected rome2_32 DB removed after clean *_32")
		}
	})
}
