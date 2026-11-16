package analyze

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

type ghidraToolchain struct {
	headlessPath string
	jdkDir       string
}

type ghidraJobSpec struct {
	toolchain      ghidraToolchain
	item           PlanItem
	opts           Options
	mem            SystemMemory
	concurrentJobs int
	onLogLine      func(string)
	onSample       func(LiveProcessStats)
}

const tuneReferenceAnalysisJava = `import ghidra.app.script.GhidraScript;
import java.util.Map;

public class TuneReferenceAnalysis extends GhidraScript {
    private static final String[] REQUIRED_ANALYZERS = new String[] {
        "DWARF.Import Data Types",
        "DWARF.Import Functions",
        "DWARF.Create Function Signatures"
    };

    private static final String[] HEAVY_ANALYZERS = new String[] {
        "Decompiler Switch Analysis",
        "Decompiler Parameter ID",
        "Call Convention ID",
        "Non-Returning Functions - Discovered",
        "Stack",
        "Shared Return Calls",
        "Aggressive Instruction Finder",
        "Create Address Tables",
        "Create Address Tables - One Time",
        "GCC Exception Handlers",
        "Variadic Function Signature Override",
        "x86 Constant Reference Analyzer",
        "DWARF.Import Local Variable Info",
        "DWARF.Import Source Line Info",
        "DWARF.Output Source Info",
        "DWARF.Output DWARF DIE Info",
        "DWARF.Output Storage Info",
        "DWARF.Add Inlined Functions Comments",
        "DWARF.Add Lexical Block Comments"
    };

    @Override
    public void run() throws Exception {
        if (currentProgram == null) {
            return;
        }
        Map<String, String> options = getCurrentAnalysisOptionsAndValues(currentProgram);
        for (String name : REQUIRED_ANALYZERS) {
            if (options.containsKey(name)) {
                setAnalysisOption(currentProgram, name, "true");
            }
        }
        for (String name : HEAVY_ANALYZERS) {
            if (options.containsKey(name)) {
                setAnalysisOption(currentProgram, name, "false");
            }
        }
    }
}
`

func resolveGhidraToolchain(paths workspace.Paths) (ghidraToolchain, error) {
	launcherName := "analyzeHeadless"
	if runtime.GOOS == "windows" {
		launcherName = "analyzeHeadless.bat"
	}
	headlessPath := filepath.Join(paths.ToolDir(workspace.ToolGhidra), "support", launcherName)
	if info, err := os.Stat(headlessPath); err != nil || info.IsDir() {
		return ghidraToolchain{}, &term.DiagnosticError{
			Message:  fmt.Sprintf("Ghidra headless launcher not found at %s", filepath.ToSlash(headlessPath)),
			Expected: "installed Ghidra toolchain in workspace",
			Help:     []string{"Run `twdll init --decompiler ghidra` to download and install Ghidra and OpenJDK."},
		}
	}

	jdkDir := paths.ToolDir(workspace.ToolJDK)
	javaPath := filepath.Join(jdkDir, "bin", "java"+workspace.ExeSuffix())
	if info, err := os.Stat(javaPath); err != nil || info.IsDir() {
		return ghidraToolchain{}, &term.DiagnosticError{
			Message:  fmt.Sprintf("Java runtime not found at %s", filepath.ToSlash(javaPath)),
			Expected: "installed Adoptium OpenJDK toolchain in workspace",
			Help:     []string{"Run `twdll init --decompiler ghidra` to download and install Ghidra and OpenJDK."},
		}
	}

	return ghidraToolchain{
		headlessPath: headlessPath,
		jdkDir:       jdkDir,
	}, nil
}

func buildGhidraArgs(stageDir string, target workspace.TargetID, binaryPath, scriptDir string) []string {
	args := []string{stageDir, string(target), "-import", binaryPath}
	if scriptDir != "" {
		args = append(args, "-scriptPath", scriptDir, "-preScript", "TuneReferenceAnalysis.java")
	}
	return append(args, "-overwrite")
}

func buildGhidraJavaOptions(stageDir string, numCPU int) string {
	gcThreads := clampInt(numCPU/2, 2, 4)
	opts := fmt.Sprintf("-XX:+UseG1GC -XX:+UseStringDeduplication -XX:ParallelGCThreads=%d", gcThreads)
	if !strings.ContainsAny(stageDir, " \t\"") {
		tmpDir := filepath.Join(stageDir, "env", "tmp")
		homeDir := filepath.Join(stageDir, "env", "home")
		opts += fmt.Sprintf(" -Djava.io.tmpdir=%s -Duser.home=%s", tmpDir, homeDir)
	}
	return opts
}

func runGhidraTarget(ctx context.Context, spec ghidraJobSpec) (targetRunResult, error) {
	target := spec.item.Target
	stageDir, err := os.MkdirTemp("", "twdll_ghidra_"+string(target)+"_*")
	if err != nil {
		return targetRunResult{}, fmt.Errorf("create Ghidra staging dir for %s: %w", target, err)
	}
	defer func() { _ = os.RemoveAll(stageDir) }()

	if err := os.MkdirAll(filepath.Join(stageDir, "env", "home"), 0o755); err != nil {
		return targetRunResult{}, fmt.Errorf("create Ghidra home dir for %s: %w", target, err)
	}

	stripHeavy := target == workspace.TargetAttila64 && !spec.opts.FullAnalysis
	scriptDir := ""
	if stripHeavy {
		scriptDir = filepath.Join(stageDir, "scripts")
		if err := os.MkdirAll(scriptDir, 0o755); err != nil {
			return targetRunResult{}, fmt.Errorf("create Ghidra script dir for %s: %w", target, err)
		}
		scriptFile := filepath.Join(scriptDir, "TuneReferenceAnalysis.java")
		if err := os.WriteFile(scriptFile, []byte(tuneReferenceAnalysisJava), 0o644); err != nil {
			return targetRunResult{}, fmt.Errorf("write TuneReferenceAnalysis.java for %s: %w", target, err)
		}
	}

	maxMem := strings.TrimSpace(spec.opts.MaxMem)
	if maxMem == "" {
		maxMem = ComputeGhidraMaxMem(spec.mem, target, spec.opts.FullAnalysis, spec.concurrentJobs)
	}

	jdkBin := filepath.Join(spec.toolchain.jdkDir, "bin")
	pathVal := jdkBin
	if existingPath := os.Getenv("PATH"); existingPath != "" {
		pathVal = jdkBin + string(os.PathListSeparator) + existingPath
	}

	env, err := prepareStageEnv(stageDir, true, map[string]string{
		"JAVA_HOME":                    spec.toolchain.jdkDir,
		"GHIDRA_HEADLESS_MAXMEM":       maxMem,
		"GHIDRA_HEADLESS_JAVA_OPTIONS": buildGhidraJavaOptions(stageDir, runtime.NumCPU()),
		"PATH":                         pathVal,
	})
	if err != nil {
		return targetRunResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(spec.item.LogPath), 0o755); err != nil {
		return targetRunResult{}, fmt.Errorf("ensure log dir for %s: %w", target, err)
	}
	logFile, err := os.OpenFile(spec.item.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return targetRunResult{}, fmt.Errorf("open log file %s: %w", spec.item.LogPath, err)
	}

	args := buildGhidraArgs(stageDir, target, spec.item.BinaryPath, scriptDir)
	cmd := exec.CommandContext(ctx, spec.toolchain.headlessPath, args...)
	cmd.Dir = stageDir
	cmd.Env = env
	var sink *lineSink
	var logWriter io.Writer = logFile
	if spec.onLogLine != nil {
		sink = newLineSink(spec.onLogLine)
		logWriter = io.MultiWriter(logFile, sink)
	}
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	metrics, runErr := runObservedCommand(cmd, spec.onSample)
	res := targetRunResult{
		Metrics:      metrics,
		GhidraMaxMem: maxMem,
		Started:      true,
	}
	if sink != nil {
		sink.Flush()
	}
	_ = logFile.Close()
	if runErr != nil {
		if ctx.Err() != nil {
			return res, fmt.Errorf("Ghidra analysis cancelled for %s: %w", target, ctx.Err())
		}
		return res, fmt.Errorf("Ghidra headless analysis failed for %s (%w); see %s", target, runErr, filepath.ToSlash(spec.item.LogPath))
	}

	stagedGPR := filepath.Join(stageDir, string(target)+".gpr")
	if info, err := os.Stat(stagedGPR); err != nil || info.IsDir() {
		return res, fmt.Errorf("Ghidra exited 0 for %s but project marker %s was not created (see %s)", target, stagedGPR, filepath.ToSlash(spec.item.LogPath))
	}
	stagedRep := filepath.Join(stageDir, string(target)+".rep")
	if info, err := os.Stat(stagedRep); err != nil || !info.IsDir() {
		return res, fmt.Errorf("Ghidra exited 0 for %s but project repository %s was not created (see %s)", target, stagedRep, filepath.ToSlash(spec.item.LogPath))
	}

	destDir := spec.item.DatabaseDir
	if err := os.RemoveAll(destDir); err != nil {
		return res, fmt.Errorf("clear existing Ghidra database %s: %w", destDir, err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return res, fmt.Errorf("create Ghidra database dir %s: %w", destDir, err)
	}
	if err := moveFile(stagedGPR, filepath.Join(destDir, string(target)+".gpr")); err != nil {
		return res, fmt.Errorf("install Ghidra .gpr for %s: %w", target, err)
	}
	if err := moveDir(stagedRep, filepath.Join(destDir, string(target)+".rep")); err != nil {
		return res, fmt.Errorf("install Ghidra .rep for %s: %w", target, err)
	}
	return res, nil
}
