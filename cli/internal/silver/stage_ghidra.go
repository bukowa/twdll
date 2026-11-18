package silver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"twdll/cli/internal/analyze"
	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

var ghidraOwnerAttrRe = regexp.MustCompile(`<STATE\s+NAME="OWNER"\s+TYPE="string"\s+VALUE="[^"]*"\s*/>`)

func currentOSUsername() string {
	if u, err := user.Current(); err == nil && strings.TrimSpace(u.Username) != "" {
		name := strings.TrimSpace(u.Username)
		if idx := strings.LastIndexAny(name, `\/`); idx >= 0 {
			name = name[idx+1:]
		}
		if name != "" {
			return name
		}
	}
	for _, k := range []string{"USERNAME", "USER"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return "twdll"
}

func hasDotPathSegment(dir string) bool {
	if dir == "" {
		return false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	for _, seg := range strings.Split(filepath.ToSlash(abs), "/") {
		if len(seg) > 0 && seg != "." && seg != ".." && strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func stageGhidraReadOnly(paths workspace.Paths, target workspace.TargetID, tmpBaseDir string) (stageDir string, cleanup func(), err error) {
	return stageGhidraDir(paths, target, tmpBaseDir, false)
}

func stageGhidraDir(paths workspace.Paths, target workspace.TargetID, tmpBaseDir string, writable bool) (stageDir string, cleanup func(), err error) {
	srcDir := filepath.Join(paths.DBDir, "ghidra", string(target))
	srcGPR := filepath.Join(srcDir, string(target)+".gpr")
	srcRep := filepath.Join(srcDir, string(target)+".rep")
	if info, statErr := os.Stat(srcGPR); statErr != nil || info.IsDir() {
		return "", nil, fmt.Errorf("Ghidra .gpr not found at %s: %w", filepath.ToSlash(srcGPR), statErr)
	}
	if info, statErr := os.Stat(srcRep); statErr != nil || !info.IsDir() {
		return "", nil, fmt.Errorf("Ghidra .rep not found at %s: %w", filepath.ToSlash(srcRep), statErr)
	}

	// Ghidra's ProjectLocator rejects any path segment starting with '.' (e.g. ".twdll" or ".tmp").
	if hasDotPathSegment(tmpBaseDir) {
		tmpBaseDir = ""
	}
	if tmpBaseDir != "" {
		if err := os.MkdirAll(tmpBaseDir, 0o755); err != nil {
			return "", nil, fmt.Errorf("ensure tmp base dir %s: %w", tmpBaseDir, err)
		}
	}
	stageDir, err = os.MkdirTemp(tmpBaseDir, "twdll_gh_ro_"+string(target)+"_*")
	if err != nil {
		return "", nil, fmt.Errorf("create ghidra ro stage dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(stageDir) }

	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	if err := os.MkdirAll(filepath.Join(stageDir, "env", "home"), 0o755); err != nil {
		return "", nil, err
	}
	if err := workspace.CopyFileAtomic(srcGPR, filepath.Join(stageDir, string(target)+".gpr")); err != nil {
		return "", nil, fmt.Errorf("copy .gpr: %w", err)
	}

	dstRep := filepath.Join(stageDir, string(target)+".rep")
	dstDBSubdir := filepath.Join(dstRep, "idata", "00", "~00000000.db")
	for _, d := range []string{
		dstDBSubdir,
		filepath.Join(dstRep, "user"),
		filepath.Join(dstRep, "versioned"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", nil, err
		}
	}

	// Copy project.prp with current OS username so readOnly open never fails on OWNER mismatch.
	srcProjectPRP := filepath.Join(srcRep, "project.prp")
	prpData, err := os.ReadFile(srcProjectPRP)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", srcProjectPRP, err)
	}
	ownerLine := fmt.Sprintf(`<STATE NAME="OWNER" TYPE="string" VALUE="%s" />`, currentOSUsername())
	prpRewritten := ghidraOwnerAttrRe.ReplaceAllLiteralString(string(prpData), ownerLine)
	if err := os.WriteFile(filepath.Join(dstRep, "project.prp"), []byte(prpRewritten), 0o644); err != nil {
		return "", nil, fmt.Errorf("write staged project.prp: %w", err)
	}

	// Copy small metadata index/property files.
	for _, rel := range []string{
		filepath.Join("idata", "~index.dat"),
		filepath.Join("idata", "00", "00000000.prp"),
		filepath.Join("user", "~index.dat"),
		filepath.Join("versioned", "~index.dat"),
	} {
		srcFile := filepath.Join(srcRep, rel)
		if info, err := os.Stat(srcFile); err == nil && !info.IsDir() {
			if err := workspace.CopyFileAtomic(srcFile, filepath.Join(dstRep, rel)); err != nil {
				return "", nil, fmt.Errorf("copy metadata %s: %w", rel, err)
			}
		}
	}

	// Hardlink db.*.gbf files when read-only (0 MB extra disk, 0 ms copy) with CopyFileAtomic fallback;
	// copy instead of hardlinking when writable enrichers are active so .twdll/db/ can never be mutated.
	srcDBSubdir := filepath.Join(srcRep, "idata", "00", "~00000000.db")
	entries, err := os.ReadDir(srcDBSubdir)
	if err != nil {
		return "", nil, fmt.Errorf("read ghidra db dir %s: %w", srcDBSubdir, err)
	}
	linkedAny := false
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "~") {
			continue
		}
		srcFile := filepath.Join(srcDBSubdir, entry.Name())
		dstFile := filepath.Join(dstDBSubdir, entry.Name())
		if writable {
			if cpErr := workspace.CopyFileAtomic(srcFile, dstFile); cpErr != nil {
				return "", nil, fmt.Errorf("copy ghidra payload %s: %w", entry.Name(), cpErr)
			}
		} else if linkErr := os.Link(srcFile, dstFile); linkErr != nil {
			if cpErr := workspace.CopyFileAtomic(srcFile, dstFile); cpErr != nil {
				return "", nil, fmt.Errorf("stage ghidra payload %s (link: %v, copy: %w)", entry.Name(), linkErr, cpErr)
			}
		}
		linkedAny = true
	}
	if !linkedAny {
		return "", nil, fmt.Errorf("no database payload found in %s", srcDBSubdir)
	}

	success = true
	return stageDir, cleanup, nil
}

func stageGhidraEnrichers(scriptDir string, envMap map[string]string, enrichers []EnrichScript) ([]string, error) {
	seen := map[string]bool{
		"twdllsilverexport.java": true,
		"twdlldecompexport.java": true,
	}
	var preArgs []string
	for _, sc := range enrichers {
		name := filepath.Base(strings.TrimSpace(sc.Filename))
		if name == "" || name == "." || name == string(filepath.Separator) || name == "/" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("ghidra enricher filename %q collides with reserved or duplicate script", name)
		}
		seen[key] = true
		if err := os.WriteFile(filepath.Join(scriptDir, name), []byte(sc.Body), 0o644); err != nil {
			return nil, fmt.Errorf("write ghidra enricher %s: %w", name, err)
		}
		for k, v := range sc.Env {
			envMap[k] = v
		}
		preArgs = append(preArgs, "-preScript", name)
	}
	return preArgs, nil
}

func runGhidraSilverExport(ctx context.Context, opts Options, enrichers []EnrichScript, asmOutPath, stringsOutPath string) ([]SilverFunc, error) {
	headlessPath, jdkDir, err := analyze.ResolveGhidraToolchain(opts.Paths)
	if err != nil {
		return nil, err
	}

	writable := len(enrichers) > 0
	stageDir, cleanup, err := stageGhidraDir(opts.Paths, opts.Target, resolveTmpBase(opts.Paths, opts.TmpBaseDir), writable)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	scriptDir := filepath.Join(stageDir, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(scriptDir, "TwdllSilverExport.java"), []byte(twdllSilverExportJava), 0o644); err != nil {
		return nil, err
	}

	outPath := filepath.Join(stageDir, "silver.jsonl")
	totalPath := filepath.Join(stageDir, "silver.total")
	logPath := filepath.Join(stageDir, "ghidra_silver.log")

	mem := analyze.ProbeSystemMemory()
	maxMem := strings.TrimSpace(opts.MaxMem)
	if maxMem == "" {
		maxMem = analyze.ComputeGhidraMaxMem(mem, opts.Target, false, 1)
	}

	envMap := map[string]string{
		"JAVA_HOME":                    jdkDir,
		"GHIDRA_HEADLESS_MAXMEM":       maxMem,
		"GHIDRA_HEADLESS_JAVA_OPTIONS": analyze.BuildGhidraJavaOptions(stageDir, runtime.NumCPU()),
		"PATH":                         filepath.Join(jdkDir, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TWDLL_SILVER_OUT":             outPath,
		"TWDLL_SILVER_TOTAL_OUT":       totalPath,
	}
	if strings.TrimSpace(asmOutPath) != "" {
		envMap["TWDLL_SILVER_ASM_OUT"] = asmOutPath
	}
	if strings.TrimSpace(stringsOutPath) != "" {
		envMap["TWDLL_SILVER_STRINGS_OUT"] = stringsOutPath
	}
	preArgs, err := stageGhidraEnrichers(scriptDir, envMap, enrichers)
	if err != nil {
		return nil, err
	}

	env, err := analyze.PrepareStageEnv(stageDir, true, envMap)
	if err != nil {
		return nil, err
	}

	args := []string{
		stageDir,
		string(opts.Target),
		"-process", opts.Target.BinaryFileName(),
	}
	if !writable {
		args = append(args, "-readOnly")
	}
	args = append(args, "-noanalysis", "-scriptPath", scriptDir)
	args = append(args, preArgs...)
	args = append(args, "-postScript", "TwdllSilverExport.java")

	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, headlessPath, args...)
	cmd.Dir = stageDir
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	stopWatch := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		watchJSONLLineProgress(ctx, outPath, totalPath, stopWatch, opts.OnProgress)
	}()

	_, runErr := analyze.RunObservedCommand(cmd, nil)
	close(stopWatch)
	<-watchDone
	_ = logFile.Close()
	if runErr != nil {
		tail := readLogTail(logPath, 20)
		return nil, fmt.Errorf("ghidra silver export failed (%w)\n%s", runErr, tail)
	}

	funcs, parseErr := parseSilverJSONL(outPath)
	if parseErr != nil {
		tail := readLogTail(logPath, 25)
		if tail != "" {
			return nil, fmt.Errorf("%w\n%s", parseErr, tail)
		}
		return nil, parseErr
	}
	return funcs, nil
}

func runGhidraDecompExport(
	ctx context.Context,
	opts Options,
	enrichers []EnrichScript,
	pending []extractdb.FuncSpec,
	outCh chan<- extractdb.DecompRecord,
) (uint64, error) {
	headlessPath, jdkDir, err := analyze.ResolveGhidraToolchain(opts.Paths)
	if err != nil {
		return 0, err
	}

	writable := len(enrichers) > 0
	stageDir, cleanup, err := stageGhidraDir(opts.Paths, opts.Target, resolveTmpBase(opts.Paths, opts.TmpBaseDir), writable)
	if err != nil {
		return 0, err
	}
	defer cleanup()

	scriptDir := filepath.Join(stageDir, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(scriptDir, "TwdllDecompExport.java"), []byte(twdllDecompExportJava), 0o644); err != nil {
		return 0, err
	}

	inPath := filepath.Join(stageDir, "targets.tsv")
	outPath := filepath.Join(stageDir, "decomp.jsonl")
	logPath := filepath.Join(stageDir, "ghidra_decomp.log")

	ordered := make([]extractdb.FuncSpec, len(pending))
	copy(ordered, pending)
	shards := BuildWorkerShards(ordered, 1)
	if len(shards) > 0 {
		ordered = shards[0]
	}
	if err := writeDecompTargetsTSV(inPath, ordered); err != nil {
		return 0, err
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = max(2, min(8, runtime.NumCPU()/2))
	}
	timeoutSecs := resolveDecompTimeoutSecs(opts)

	mem := analyze.ProbeSystemMemory()
	maxMem := strings.TrimSpace(opts.MaxMem)
	if maxMem == "" {
		maxMem = analyze.ComputeGhidraMaxMem(mem, opts.Target, false, 1)
	}

	envMap := map[string]string{
		"JAVA_HOME":                    jdkDir,
		"GHIDRA_HEADLESS_MAXMEM":       maxMem,
		"GHIDRA_HEADLESS_JAVA_OPTIONS": analyze.BuildGhidraJavaOptions(stageDir, runtime.NumCPU()),
		"PATH":                         filepath.Join(jdkDir, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TWDLL_DECOMP_IN":              inPath,
		"TWDLL_DECOMP_OUT":             outPath,
		"TWDLL_DECOMP_WORKERS":         strconv.Itoa(workers),
		"TWDLL_DECOMP_TIMEOUT_SECS":    strconv.Itoa(timeoutSecs),
	}
	preArgs, err := stageGhidraEnrichers(scriptDir, envMap, enrichers)
	if err != nil {
		return 0, err
	}

	env, err := analyze.PrepareStageEnv(stageDir, true, envMap)
	if err != nil {
		return 0, err
	}

	args := []string{
		stageDir,
		string(opts.Target),
		"-process", opts.Target.BinaryFileName(),
	}
	if !writable {
		args = append(args, "-readOnly")
	}
	args = append(args, "-noanalysis", "-scriptPath", scriptDir)
	args = append(args, preArgs...)
	args = append(args, "-postScript", "TwdllDecompExport.java")

	logFile, err := os.Create(logPath)
	if err != nil {
		return 0, err
	}
	cmd := exec.CommandContext(ctx, headlessPath, args...)
	cmd.Dir = stageDir
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if opts.OnStatus != nil {
		opts.OnStatus("starting ghidra")
	}

	var peakMem atomic.Uint64
	stopTail := make(chan struct{})
	tailDone := make(chan error, 1)
	var doneCount atomic.Int64
	total := len(ordered)

	go func() {
		tailDone <- tailDecompJSONL(ctx, outPath, stopTail, func(frame decompFrameJSON) {
			if frame.Event == "phase" {
				if opts.OnStatus != nil && strings.TrimSpace(frame.Phase) != "" {
					opts.OnStatus(strings.TrimSpace(frame.Phase))
				}
				return
			}
			if frame.Event != "result" && frame.Event != "" {
				return
			}
			if !sendDecompRecord(ctx, outCh, frame.toDecompRecord()) {
				return
			}
			if opts.OnProgress != nil {
				opts.OnProgress(int(doneCount.Add(1)), total)
			}
		})
	}()

	metrics, runErr := analyze.RunObservedCommand(cmd, func(s analyze.LiveProcessStats) {
		for {
			cur := peakMem.Load()
			if s.PeakMemoryBytes <= cur {
				break
			}
			if peakMem.CompareAndSwap(cur, s.PeakMemoryBytes) {
				break
			}
		}
	})
	_ = logFile.Close()
	close(stopTail)
	tailErr := <-tailDone

	if metrics.PeakJobMemoryBytes > peakMem.Load() {
		peakMem.Store(metrics.PeakJobMemoryBytes)
	}
	if runErr != nil {
		tail := readLogTail(logPath, 25)
		return peakMem.Load(), fmt.Errorf("ghidra decomp failed (%w)\n%s", runErr, tail)
	}
	if tailErr != nil {
		return peakMem.Load(), tailErr
	}
	return peakMem.Load(), nil
}

func writeDecompTargetsTSV(path string, specs []extractdb.FuncSpec) error {
	var sb strings.Builder
	sb.Grow(len(specs) * 48)
	for _, s := range specs {
		cleanName := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(s.Name)
		sb.WriteString("0x")
		sb.WriteString(strconv.FormatUint(s.EA, 16))
		sb.WriteByte('\t')
		sb.WriteString(strconv.FormatUint(s.Size, 10))
		sb.WriteByte('\t')
		sb.WriteString(cleanName)
		sb.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func parseSilverJSONL(path string) ([]SilverFunc, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open silver jsonl %s: %w", path, err)
	}
	defer f.Close()

	var funcs []SilverFunc
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 256*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec silverJSONLRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("decode silver jsonl record: %w", err)
		}
		funcs = append(funcs, rec.toSilverFunc())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan silver jsonl %s: %w", path, err)
	}
	return funcs, nil
}

func tailDecompJSONL(
	ctx context.Context,
	path string,
	stop <-chan struct{},
	onFrame func(decompFrameJSON),
) error {
	tailer := &jsonlTailer{path: path}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return tailer.Drain(onFrame)
		case <-ctx.Done():
			return tailer.Drain(onFrame)
		case <-ticker.C:
			if err := tailer.Drain(onFrame); err != nil {
				return err
			}
		}
	}
}

func readLogTail(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
