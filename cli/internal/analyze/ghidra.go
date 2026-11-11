package analyze

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"twdll/cli/internal/ui"
)

// FindGhidraHeadlessScript locates analyzeHeadless.bat on Windows or analyzeHeadless on Linux.
func FindGhidraHeadlessScript(toolsDir string) (string, error) {
	scriptName := "analyzeHeadless"
	if runtime.GOOS == "windows" {
		scriptName = "analyzeHeadless.bat"
	}
	path := filepath.Join(toolsDir, "ghidra", "support", scriptName)
	if stat, err := os.Stat(path); err != nil || stat.IsDir() {
		return "", fmt.Errorf("ghidra headless script not found at %s (run `twdll init` to install)", path)
	}
	return path, nil
}

// RunGhidraHeadless executes Ghidra headless analysis on a target binary.
func RunGhidraHeadless(ctx context.Context, toolsDir string, item Item, maxMem string, slot *ui.TaskSlot, printer *ui.Printer) error {
	scriptPath, err := FindGhidraHeadlessScript(toolsDir)
	if err != nil {
		return err
	}

	jdkDir := filepath.Join(toolsDir, "jdk")
	javaExe := filepath.Join(jdkDir, "bin", "java")
	if runtime.GOOS == "windows" {
		javaExe += ".exe"
	}
	if _, err := os.Stat(javaExe); err != nil {
		return fmt.Errorf("jdk not found at %s (run `twdll init` to install)", jdkDir)
	}

	if maxMem == "" {
		maxMem = "8G"
	}

	// Ghidra forbids any path component starting with '.' (e.g. .twdll).
	// To ensure 100% cross-platform reliability across Windows, Linux, and WSL,
	// we stage the project in a dot-free temporary directory, then atomically
	// move the verified .gpr and .rep into item.OutputDir on clean exit code 0.
	stageDir, err := os.MkdirTemp("", "twdll_ghidra_"+string(item.Target)+"_*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stageDir)

	if err := os.MkdirAll(item.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(item.LogFile), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	logOut, err := os.Create(item.LogFile)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer logOut.Close()

	startTime := time.Now()
	cleanLog := filepath.ToSlash(item.LogFile)
	if slot == nil && printer != nil {
		printer.Info("  [ghidra] Analyzing %s (heap: %s, logs: %s)...", printer.Bold(string(item.Target)), maxMem, printer.Dim(cleanLog))
	}

	// analyzeHeadless <output_dir> <project_name> -import <binary_path> -overwrite
	args := []string{
		stageDir,
		string(item.Target),
		"-import",
		item.BinaryPath,
		"-overwrite",
	}

	cmd := exec.CommandContext(ctx, scriptPath, args...)
	cmd.Stdout = logOut
	cmd.Stderr = logOut

	// Set JAVA_HOME and memory environment
	env := os.Environ()
	env = append(env, "JAVA_HOME="+jdkDir)
	env = append(env, "GHIDRA_HEADLESS_MAXMEM="+maxMem)
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ghidra: %w", err)
	}

	if slot != nil && cmd.Process != nil {
		slot.SetPID(cmd.Process.Pid)
	}

	runErr := cmd.Wait()
	if runErr != nil {
		return fmt.Errorf("ghidra headless analysis failed for %s (see %s): %w", item.Target, item.LogFile, runErr)
	}

	// Move staged files into destination
	stagedGpr := filepath.Join(stageDir, string(item.Target)+".gpr")
	stagedRep := filepath.Join(stageDir, string(item.Target)+".rep")

	if _, err := os.Stat(stagedGpr); err != nil {
		return fmt.Errorf("ghidra finished with exit 0 but project file is missing: %s", stagedGpr)
	}
	if repStat, err := os.Stat(stagedRep); err != nil || !repStat.IsDir() {
		return fmt.Errorf("ghidra finished with exit 0 but project repository folder is missing: %s", stagedRep)
	}

	_ = os.RemoveAll(item.OutputDir)
	if err := os.MkdirAll(item.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}

	destGpr := filepath.Join(item.OutputDir, string(item.Target)+".gpr")
	destRep := filepath.Join(item.OutputDir, string(item.Target)+".rep")

	if err := os.Rename(stagedGpr, destGpr); err != nil {
		return fmt.Errorf("move project file to destination: %w", err)
	}
	if err := os.Rename(stagedRep, destRep); err != nil {
		return fmt.Errorf("move project repository to destination: %w", err)
	}

	elapsed := time.Since(startTime).Round(time.Second)
	finishMsg := fmt.Sprintf("  [ghidra] %s finished in %s", printer.Green(string(item.Target)), elapsed)
	if slot != nil {
		if slot.Total > 1 {
			finishMsg = fmt.Sprintf("  [%d/%d] [ghidra] %s finished in %s", slot.Index, slot.Total, printer.Green(string(item.Target)), elapsed)
		}
		slot.Finish(finishMsg)
	} else if printer != nil {
		printer.Info("%s", finishMsg)
	}
	return nil
}
