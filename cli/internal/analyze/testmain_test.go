package analyze

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"twdll/cli/internal/workspace"
)

const fakeDecompilerMainSrc = `package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	wd, _ := os.Getwd()
	for _, envKey := range []string{"TEMP", "TMP"} {
		v := os.Getenv(envKey)
		if v == "" || strings.Contains(filepath.ToSlash(v), "/.twdll") {
			fmt.Fprintf(os.Stderr, "unisolated %s=%q\n", envKey, v)
			os.Exit(3)
		}
		_ = os.WriteFile(filepath.Join(v, "probe.tmp"), []byte("isolated"), 0o644)
	}
	_ = os.WriteFile(filepath.Join(wd, "sidecar.id0"), []byte("temp"), 0o644)

	if len(os.Args) > 1 && os.Args[1] == "-c" {
		runFakeIDA(os.Args[1:])
		return
	}
	for _, envKey := range []string{"APPDATA", "LOCALAPPDATA"} {
		v := os.Getenv(envKey)
		if v == "" || strings.Contains(filepath.ToSlash(v), "/.twdll") {
			fmt.Fprintf(os.Stderr, "unisolated Ghidra %s=%q\n", envKey, v)
			os.Exit(3)
		}
		_ = os.WriteFile(filepath.Join(v, "probe.tmp"), []byte("isolated"), 0o644)
	}
	runFakeGhidra(os.Args[1:])
}

func runFakeGhidra(args []string) {
	stageDir, project, binPath := args[0], args[1], args[3]
	for _, part := range strings.Split(filepath.ToSlash(stageDir), "/") {
		if strings.HasPrefix(part, ".") {
			fmt.Fprintf(os.Stderr, "dot component in Ghidra stage path: %s\n", stageDir)
			os.Exit(4)
		}
	}
	if os.Getenv("GHIDRA_HEADLESS_MAXMEM") == "" {
		os.Exit(5)
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "-scriptPath" && i+1 < len(args) {
			data, err := os.ReadFile(filepath.Join(args[i+1], "TuneReferenceAnalysis.java"))
			if err != nil || !strings.Contains(string(data), "Decompiler Parameter ID") {
				os.Exit(6)
			}
		}
	}
	binData, err := os.ReadFile(binPath)
	if err != nil || strings.HasPrefix(string(binData), "FAIL") {
		fmt.Fprintln(os.Stderr, "simulated Ghidra failure")
		os.Exit(1)
	}
	if strings.Contains(string(binData), "SLOW32") {
		time.Sleep(80 * time.Millisecond)
	}
	fmt.Printf("analyzed %s maxmem=%s\n", project, os.Getenv("GHIDRA_HEADLESS_MAXMEM"))
	_ = os.WriteFile(filepath.Join(stageDir, project+".gpr"), nil, 0o644)
	_ = os.MkdirAll(filepath.Join(stageDir, project+".rep"), 0o755)
	_ = os.WriteFile(filepath.Join(stageDir, project+".rep", "project.prp"), []byte("rep"), 0o644)
}

func runFakeIDA(args []string) {
	var scriptPath, dbPath, logPath, binPath string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "-S"):
			scriptPath = strings.TrimPrefix(a, "-S")
		case strings.HasPrefix(a, "-o"):
			dbPath = strings.TrimPrefix(a, "-o")
		case strings.HasPrefix(a, "-L"):
			logPath = strings.TrimPrefix(a, "-L")
		case !strings.HasPrefix(a, "-"):
			binPath = a
		}
	}
	idc, err := os.ReadFile(scriptPath)
	if err != nil || !strings.Contains(string(idc), "qexit(0);") {
		os.Exit(6)
	}
	_ = os.WriteFile(logPath, []byte("ida batch log\n"), 0o644)
	binData, err := os.ReadFile(binPath)
	if err != nil || strings.HasPrefix(string(binData), "FAIL") {
		os.Exit(1)
	}
	if strings.Contains(string(binData), "SLOW32") {
		time.Sleep(80 * time.Millisecond)
	}
	_ = os.WriteFile(dbPath, []byte("IDA64-DATABASE"), 0o644)
}
`

var compiledFakeHelper string

func TestMain(m *testing.M) {
	srcDir, err := os.MkdirTemp("", "twdll_fake_decompiler_*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create helper dir: %v\n", err)
		os.Exit(1)
	}
	srcFile := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(srcFile, []byte(fakeDecompilerMainSrc), 0o644); err != nil {
		_ = os.RemoveAll(srcDir)
		fmt.Fprintf(os.Stderr, "write helper source: %v\n", err)
		os.Exit(1)
	}
	compiledFakeHelper = filepath.Join(srcDir, "fake_decompiler"+workspace.ExeSuffix())
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go"+workspace.ExeSuffix())
	if out, err := exec.Command(goBin, "build", "-o", compiledFakeHelper, srcFile).CombinedOutput(); err != nil {
		_ = os.RemoveAll(srcDir)
		fmt.Fprintf(os.Stderr, "compile fake helper: %v\n%s\n", err, string(out))
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(srcDir)
	os.Exit(code)
}

func installFakeGhidraAndJDK(t *testing.T, paths workspace.Paths) {
	t.Helper()
	helperBytes, err := os.ReadFile(compiledFakeHelper)
	if err != nil {
		t.Fatal(err)
	}
	javaPath := filepath.Join(paths.ToolDir(workspace.ToolJDK), "bin", "java"+workspace.ExeSuffix())
	if err := os.MkdirAll(filepath.Dir(javaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(javaPath, helperBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	supportDir := filepath.Join(paths.ToolDir(workspace.ToolGhidra), "support")
	if err := os.MkdirAll(supportDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		bat := "@echo off\r\n\"%JAVA_HOME%\\bin\\java.exe\" %*\r\nexit /b %ERRORLEVEL%\r\n"
		if err := os.WriteFile(filepath.Join(supportDir, "analyzeHeadless.bat"), []byte(bat), 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	sh := "#!/bin/sh\nexec \"$JAVA_HOME/bin/java\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(supportDir, "analyzeHeadless"), []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
}

func installFakeIDA(t *testing.T, idaDir string) {
	t.Helper()
	data, err := os.ReadFile(compiledFakeHelper)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(idaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"idat" + workspace.ExeSuffix(), "idat64" + workspace.ExeSuffix()} {
		if err := os.WriteFile(filepath.Join(idaDir, name), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func assertCleanWorkspace(t *testing.T, paths workspace.Paths) {
	t.Helper()
	entries, err := os.ReadDir(paths.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"binaries": true, "db": true, "logs": true, "tools": true, "state.json": true}
	for _, e := range entries {
		if !allowed[e.Name()] {
			t.Fatalf("unexpected entry %q leaked into %s", e.Name(), paths.DataDir)
		}
	}
	_ = filepath.WalkDir(paths.BinsDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() != "Attila" && d.Name() != "empire.retail.dll" {
			t.Fatalf("sidecar file leaked into binaries dir: %s", path)
		}
		return nil
	})
}
