package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"twdll/cli/internal/cmd"
	"twdll/cli/internal/term"
	"twdll/cli/internal/toolchain"
	"twdll/cli/internal/workspace"
)

func TestExecute_StatusJSON_RoutesStrictlyToStdoutWithIsolatedSandbox(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "twdll.sandbox.toml")
	if err := workspace.SaveConfig(cfgPath, workspace.Config{
		DataDir:     ".twdll-sandbox",
		DefaultGame: workspace.GameAttila,
		Decompiler:  workspace.DecompilerGhidra,
	}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"status", "--root", root, "--config", cfgPath, "--json",
	}, term.Streams{
		Out:       &stdout,
		Err:       &stderr,
		ColorMode: term.ColorNever,
	})

	if code != 0 {
		t.Fatalf("Execute exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected 0 bytes on stderr for status --json, got %q", stderr.String())
	}

	var report cmd.StatusReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nraw:\n%s", err, stdout.String())
	}
	if !report.Initialized {
		t.Fatalf("expected report.Initialized = true")
	}
	if !strings.HasSuffix(report.DataDir, ".twdll-sandbox") {
		t.Fatalf("report.DataDir = %q, want suffix .twdll-sandbox", report.DataDir)
	}
}

func TestExecute_DataDirFlagOverridesConfigDataDir(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "twdll.sandbox.toml")
	if err := workspace.SaveConfig(cfgPath, workspace.Config{
		DataDir:     ".twdll-from-config",
		DefaultGame: workspace.GameAttila,
		Decompiler:  workspace.DecompilerGhidra,
	}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"--root", root,
		"--config", cfgPath,
		"--data-dir", ".twdll-override",
		"status", "--json",
	}, term.Streams{
		Out:       &stdout,
		Err:       &stderr,
		ColorMode: term.ColorNever,
	})

	if code != 0 {
		t.Fatalf("Execute exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected 0 bytes on stderr, got %q", stderr.String())
	}

	var report cmd.StatusReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nraw:\n%s", err, stdout.String())
	}
	if !strings.HasSuffix(report.DataDir, ".twdll-override") {
		t.Fatalf("report.DataDir = %q, want suffix .twdll-override", report.DataDir)
	}
}

func TestExecute_HelpAndVersion_ReturnZeroWithoutOsExit(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Execute(context.Background(), []string{"--help"}, term.Streams{
			Out:       &stdout,
			Err:       &stderr,
			ColorMode: term.ColorNever,
		})
		if code != 0 {
			t.Fatalf("--help exit code = %d, want 0", code)
		}
		if stderr.Len() != 0 {
			t.Fatalf("expected 0 bytes on stderr for --help, got %q", stderr.String())
		}
		helpText := stdout.String()
		for _, sub := range []string{"status", "init", "analyze", "clean"} {
			if !strings.Contains(helpText, sub) {
				t.Fatalf("expected --help output to mention command %q:\n%s", sub, helpText)
			}
		}
		for _, forbidden := range []string{"extract", "search", "show"} {
			if strings.Contains(helpText, "\n  "+forbidden) {
				t.Fatalf("unexpected non-Stage-1 command %q in --help:\n%s", forbidden, helpText)
			}
		}
	})

	t.Run("version", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Execute(context.Background(), []string{"--version"}, term.Streams{
			Out:       &stdout,
			Err:       &stderr,
			ColorMode: term.ColorNever,
		})
		if code != 0 {
			t.Fatalf("--version exit code = %d, want 0", code)
		}
		if stderr.Len() != 0 {
			t.Fatalf("expected 0 bytes on stderr for --version, got %q", stderr.String())
		}
		if !strings.Contains(stdout.String(), Version) {
			t.Fatalf("--version stdout = %q, want substring %q", stdout.String(), Version)
		}
	})
}

func TestExecute_UnknownCommandAndInvalidFlag_WriteErrorsStrictlyToStderr(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "unregistered command extract", args: []string{"extract"}},
		{name: "invalid flag on status", args: []string{"status", "--bogus-flag"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Execute(context.Background(), tc.args, term.Streams{
				Out:       &stdout,
				Err:       &stderr,
				ColorMode: term.ColorNever,
			})
			if code == 0 {
				t.Fatalf("expected non-zero exit code for %v, got 0", tc.args)
			}
			if stdout.Len() != 0 {
				t.Fatalf("expected 0 bytes on stdout on error, got %q", stdout.String())
			}
			if stderr.Len() == 0 {
				t.Fatalf("expected diagnostic error message on stderr, got empty output")
			}
		})
	}
}

func TestExecute_InitNoInput_PreservesSavedConfigDecompilerAndDefaultGame(t *testing.T) {
	root := t.TempDir()

	emptySteam := filepath.Join(root, "EmptySteam")
	if err := os.MkdirAll(filepath.Join(emptySteam, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptySteam, "steamapps", "libraryfolders.vdf"), []byte(`"libraryfolders" {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEAM_DIR", emptySteam)

	fakeIDADir := filepath.Join(root, "FakeIDA")
	if err := os.MkdirAll(fakeIDADir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeIDADir, "idat64.exe"), []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}

	fakeAttilaDir := filepath.Join(root, "FakeAttila")
	if err := os.MkdirAll(fakeAttilaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestPE32(t, filepath.Join(fakeAttilaDir, "empire.retail.dll"))

	cfgPath := filepath.Join(root, "twdll.sandbox.toml")
	initialCfg := workspace.Config{
		DataDir:     ".twdll-init-sandbox",
		DefaultGame: workspace.GameRome2,
		Decompiler:  workspace.DecompilerIDA,
		IDADir:      fakeIDADir,
		Games: workspace.GamesConfig{
			Attila: fakeAttilaDir,
		},
	}
	if err := workspace.SaveConfig(cfgPath, initialCfg); err != nil {
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

	preseedBuildTools(t, layout.Paths)
	attila64Dest := layout.Paths.BinaryPath(workspace.TargetAttila64)
	if err := os.MkdirAll(filepath.Dir(attila64Dest), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestELF64DWARF(t, attila64Dest)

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"init", "--root", root, "--config", cfgPath, "--no-input",
	}, term.Streams{
		Out:       &stdout,
		Err:       &stderr,
		ColorMode: term.ColorNever,
	})

	if code != 0 {
		t.Fatalf("init exit code = %d, want 0\nstderr:\n%s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected 0 bytes on stdout for init, got %q", stdout.String())
	}

	afterCfg, exists, err := workspace.LoadConfig(cfgPath)
	if err != nil || !exists {
		t.Fatalf("LoadConfig(%s): exists=%v err=%v", cfgPath, exists, err)
	}
	if afterCfg.Decompiler != workspace.DecompilerIDA {
		t.Fatalf("Decompiler = %q, want %q (omitted CLI flag clobbered saved config)", afterCfg.Decompiler, workspace.DecompilerIDA)
	}
	if afterCfg.DefaultGame != workspace.GameRome2 {
		t.Fatalf("DefaultGame = %q, want %q (omitted CLI flag clobbered saved config)", afterCfg.DefaultGame, workspace.GameRome2)
	}
}

func preseedBuildTools(t *testing.T, paths workspace.Paths) {
	t.Helper()
	manifest := toolchain.HostManifest()
	for _, id := range toolchain.BuildToolOrder {
		def, ok := manifest[id]
		if !ok {
			continue
		}
		marker := filepath.Join(paths.ToolDir(def.ID), filepath.FromSlash(def.MarkerRel))
		if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(marker, []byte("stub"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	xwinLib := filepath.Join(paths.ToolDir(workspace.ToolXwin), "sdk", "crt", "lib", "x86", "libcmt.lib")
	if err := os.MkdirAll(filepath.Dir(xwinLib), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(xwinLib, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestPE32(t *testing.T, path string) {
	t.Helper()
	buf := make([]byte, 0x100)
	buf[0] = 'M'
	buf[1] = 'Z'
	binary.LittleEndian.PutUint32(buf[0x3c:], 0x80)
	copy(buf[0x80:], []byte{'P', 'E', 0, 0})
	binary.LittleEndian.PutUint16(buf[0x84:], 0x014c)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestELF64DWARF(t *testing.T, path string) {
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
	buf[4] = 2
	buf[5] = 1
	buf[6] = 1
	binary.LittleEndian.PutUint16(buf[16:], 2)
	binary.LittleEndian.PutUint16(buf[18:], 62)
	binary.LittleEndian.PutUint32(buf[20:], 1)
	binary.LittleEndian.PutUint64(buf[40:], shOff)
	binary.LittleEndian.PutUint16(buf[52:], ehdrSize)
	binary.LittleEndian.PutUint16(buf[58:], shentSize)
	binary.LittleEndian.PutUint16(buf[60:], shNum)
	binary.LittleEndian.PutUint16(buf[62:], 1)

	copy(buf[shstrOff:], shstrtab)
	copy(buf[debugOff:], debugInfo)

	s1 := buf[int(shOff)+shentSize:]
	binary.LittleEndian.PutUint32(s1[0:], 1)
	binary.LittleEndian.PutUint32(s1[4:], 3)
	binary.LittleEndian.PutUint64(s1[24:], shstrOff)
	binary.LittleEndian.PutUint64(s1[32:], uint64(len(shstrtab)))

	s2 := buf[int(shOff)+2*shentSize:]
	binary.LittleEndian.PutUint32(s2[0:], 11)
	binary.LittleEndian.PutUint32(s2[4:], 1)
	binary.LittleEndian.PutUint64(s2[24:], debugOff)
	binary.LittleEndian.PutUint64(s2[32:], uint64(len(debugInfo)))

	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}
