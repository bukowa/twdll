package bootstrap

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func MaybeRebuildAndExec(repoRoot string, args []string) (bool, int) {
	if os.Getenv("TWDLL_NO_REBUILD") == "1" || repoRoot == "" {
		return false, 0
	}

	exePath, err := os.Executable()
	if err != nil {
		return false, 0
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return false, 0
	}

	expectedBinDir := filepath.Clean(filepath.Join(repoRoot, ".twdll", "bin"))
	if !strings.EqualFold(filepath.Clean(filepath.Dir(exePath)), expectedBinDir) {
		return false, 0
	}

	_ = os.Remove(exePath + ".old")

	exeInfo, err := os.Stat(exePath)
	if err != nil {
		return false, 0
	}

	cliDir := filepath.Join(repoRoot, "cli")
	if !hasNewerSources(cliDir, exeInfo.ModTime()) {
		return false, 0
	}

	goExe := resolveGoToolchain(repoRoot)
	if goExe == "" {
		return false, 0
	}

	fmt.Fprintln(os.Stderr, "[twdll] CLI sources changed, rebuilding...")
	tmpExe := exePath + ".tmp"
	_ = os.Remove(tmpExe)

	build := exec.Command(goExe, "build", "-buildvcs=false", "-o", tmpExe, ".")
	build.Dir = cliDir
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		_ = os.Remove(tmpExe)
		fmt.Fprintf(os.Stderr, "error: failed to rebuild CLI: %v\n", err)
		return true, 1
	}

	oldExe := exePath + ".old"
	if err := os.Rename(exePath, oldExe); err != nil {
		_ = os.Remove(tmpExe)
		return false, 0
	}
	if err := os.Rename(tmpExe, exePath); err != nil {
		_ = os.Rename(oldExe, exePath)
		return false, 0
	}

	child := exec.Command(exePath, args...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = append(os.Environ(), "TWDLL_NO_REBUILD=1")
	if err := child.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return true, exitErr.ExitCode()
		}
		return true, 1
	}
	return true, 0
}

func hasNewerSources(cliDir string, binModTime time.Time) bool {
	foundNewer := false
	_ = filepath.WalkDir(cliDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !isWatchedSourceFile(d.Name()) {
			return nil
		}
		info, statErr := d.Info()
		if statErr == nil && info.ModTime().After(binModTime) {
			foundNewer = true
			return fs.SkipAll
		}
		return nil
	})
	return foundNewer
}

func isWatchedSourceFile(name string) bool {
	if name == "go.mod" || name == "go.sum" {
		return true
	}
	if strings.HasSuffix(name, "_test.go") {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".java", ".idc":
		return true
	default:
		return false
	}
}

func resolveGoToolchain(repoRoot string) string {
	if repoRoot != "" {
		portable := portableGoPath(repoRoot)
		if info, err := os.Stat(portable); err == nil && !info.IsDir() {
			return portable
		}
	}
	if sysGo, err := exec.LookPath("go"); err == nil {
		return sysGo
	}
	return ""
}

func portableGoPath(repoRoot string) string {
	name := "go"
	if runtime.GOOS == "windows" {
		name = "go.exe"
	}
	return filepath.Join(repoRoot, ".twdll", "tools", "go", "bin", name)
}
