package analyze

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"twdll/cli/internal/term"
	"twdll/cli/internal/toolchain"
	"twdll/cli/internal/workspace"
)

type idaJobSpec struct {
	exePath   string
	item      PlanItem
	opts      Options
	onLogLine func(string)
	onSample  func(LiveProcessStats)
}

func resolveIDAExecutable(idaDir string, target workspace.TargetID) (string, error) {
	found, err := toolchain.FindIDAExecutable(idaDir, target.Is64Bit())
	if err != nil {
		return "", &term.DiagnosticError{
			Message:  fmt.Sprintf("IDA Pro executable not found for %s (%v)", target, err),
			Expected: "a valid IDA Pro installation directory with idat/ida",
			Help:     []string{"Run `twdll init --decompiler ida --ida-dir <path>` or switch with `twdll analyze --decompiler ghidra`."},
		}
	}
	return filepath.FromSlash(found), nil
}

func buildIDABatchScript(stripHeavy bool) string {
	afLine := "set_inf_attr(INF_AF, get_inf_attr(INF_AF) | AF_DODATA | AF_FINAL);"
	if stripHeavy {
		afLine = "set_inf_attr(INF_AF, (get_inf_attr(INF_AF) & ~(AF_STKARG | AF_REGARG)) | AF_DODATA | AF_FINAL);"
	}
	return "#include <idc.idc>\n\nstatic main() {\n  " +
		afLine + "\n  " +
		"auto_mark_range(0, BADADDR, AU_FINAL);\n  " +
		"auto_wait();\n  " +
		"qexit(0);\n" +
		"}\n"
}

func buildIDAArgs(scriptPath, stagedDBPath, logPath, binaryPath string) []string {
	return []string{
		"-c",
		"-A",
		"-S" + scriptPath,
		"-o" + stagedDBPath,
		"-L" + logPath,
		binaryPath,
	}
}

func runIDATarget(ctx context.Context, spec idaJobSpec) (targetRunResult, error) {
	target := spec.item.Target
	stageDir, err := os.MkdirTemp("", "twdll_ida_"+string(target)+"_*")
	if err != nil {
		return targetRunResult{}, fmt.Errorf("create IDA staging dir for %s: %w", target, err)
	}
	defer func() { _ = os.RemoveAll(stageDir) }()

	stripHeavy := target == workspace.TargetAttila64 && !spec.opts.FullAnalysis
	scriptPath := filepath.Join(stageDir, "analyze_batch.idc")
	if err := os.WriteFile(scriptPath, []byte(buildIDABatchScript(stripHeavy)), 0o644); err != nil {
		return targetRunResult{}, fmt.Errorf("write analyze_batch.idc for %s: %w", target, err)
	}

	stagedDBPath := filepath.Join(stageDir, string(target)+".i64")
	if err := os.MkdirAll(filepath.Dir(spec.item.LogPath), 0o755); err != nil {
		return targetRunResult{}, fmt.Errorf("ensure log dir for %s: %w", target, err)
	}
	_ = os.Remove(spec.item.LogPath)

	env, err := prepareStageEnv(stageDir, false, nil)
	if err != nil {
		return targetRunResult{}, err
	}

	args := buildIDAArgs(scriptPath, stagedDBPath, spec.item.LogPath, spec.item.BinaryPath)
	cmd := exec.CommandContext(ctx, spec.exePath, args...)
	cmd.Dir = stageDir
	cmd.Env = env
	var consoleBuf bytes.Buffer
	cmd.Stdout = &consoleBuf
	cmd.Stderr = &consoleBuf

	var flushTail func()
	if spec.onLogLine != nil {
		flushTail = tailLogFile(ctx.Done(), spec.item.LogPath, spec.onLogLine)
	}
	metrics, runErr := runObservedCommand(ctx, cmd, spec.onSample)
	res := targetRunResult{
		Metrics: metrics,
		Started: true,
	}
	if flushTail != nil {
		flushTail()
	}
	if consoleBuf.Len() > 0 {
		if info, sErr := os.Stat(spec.item.LogPath); sErr != nil || info.Size() == 0 {
			_ = os.WriteFile(spec.item.LogPath, consoleBuf.Bytes(), 0o644)
		}
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return res, fmt.Errorf("IDA analysis cancelled for %s: %w", target, ctx.Err())
		}
		return res, fmt.Errorf("IDA headless analysis failed for %s (%w); see %s", target, runErr, filepath.ToSlash(spec.item.LogPath))
	}

	info, err := os.Stat(stagedDBPath)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return res, fmt.Errorf("IDA exited 0 for %s but produced no valid database at %s (see %s)", target, stagedDBPath, filepath.ToSlash(spec.item.LogPath))
	}

	destDir := spec.item.DatabaseDir
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return res, fmt.Errorf("create IDA database dir %s: %w", destDir, err)
	}
	_ = os.Remove(filepath.Join(destDir, string(target)+".idb"))
	finalDBPath := filepath.Join(destDir, string(target)+".i64")
	if err := moveFile(stagedDBPath, finalDBPath); err != nil {
		return res, fmt.Errorf("install IDA database for %s: %w", target, err)
	}
	return res, nil
}
