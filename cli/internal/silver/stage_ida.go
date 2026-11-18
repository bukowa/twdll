package silver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"twdll/cli/internal/analyze"
	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

func resolveIDADBPath(paths workspace.Paths, target workspace.TargetID) (string, int64, error) {
	dbPath := filepath.Join(paths.DBDir, "ida", string(target), string(target)+".i64")
	info, err := os.Stat(dbPath)
	if err != nil {
		return "", 0, fmt.Errorf("IDA database not found at %s: %w", filepath.ToSlash(dbPath), err)
	}
	if info.IsDir() || info.Size() == 0 {
		return "", 0, fmt.Errorf("invalid IDA database at %s", filepath.ToSlash(dbPath))
	}
	return dbPath, info.Size(), nil
}

func stageIDAWorkerDir(srcI64Path string, target workspace.TargetID, tmpBaseDir, prefix string) (workerDir, stagedI64 string, cleanup func(), err error) {
	if tmpBaseDir != "" {
		if err := os.MkdirAll(tmpBaseDir, 0o755); err != nil {
			return "", "", nil, fmt.Errorf("ensure tmp base dir %s: %w", tmpBaseDir, err)
		}
	}
	workerDir, err = os.MkdirTemp(tmpBaseDir, prefix+"_"+string(target)+"_*")
	if err != nil {
		return "", "", nil, fmt.Errorf("create ida worker dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(workerDir) }

	idaCfgDir := filepath.Join(workerDir, "idausr", "cfg")
	if err := os.MkdirAll(idaCfgDir, 0o755); err != nil {
		cleanup()
		return "", "", nil, err
	}
	if err := os.WriteFile(filepath.Join(idaCfgDir, "hexrays.cfg"), []byte("MAX_FUNCSIZE = 128\nARG_HINTS_MODE = 0\n"), 0o644); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("stage hexrays.cfg: %w", err)
	}
	stagedI64 = filepath.Join(workerDir, string(target)+".i64")
	if err := workspace.CopyFileAtomic(srcI64Path, stagedI64); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("stage .i64 copy: %w", err)
	}
	return workerDir, stagedI64, cleanup, nil
}

func cleanupIDAUnpackedSidecars(workerDir string, target workspace.TargetID) {
	base := filepath.Join(workerDir, string(target))
	for _, ext := range []string{".id0", ".id1", ".id2", ".nam", ".til"} {
		_ = os.Remove(base + ext)
	}
}

func stageIDAScriptsAndEnv(
	workerDir string,
	exportScriptName string,
	exportScriptBody string,
	enrichers []EnrichScript,
	envMap map[string]string,
) (string, error) {
	if err := os.WriteFile(filepath.Join(workerDir, exportScriptName), []byte(exportScriptBody), 0o644); err != nil {
		return "", err
	}
	if len(enrichers) == 0 {
		return exportScriptName, nil
	}

	var orderedNames []string
	for i, sc := range enrichers {
		base := filepath.Base(strings.TrimSpace(sc.Filename))
		if base == "" || base == "." {
			continue
		}
		name := fmt.Sprintf("enrich_%d_%s", i, base)
		if err := os.WriteFile(filepath.Join(workerDir, name), []byte(sc.Body), 0o644); err != nil {
			return "", fmt.Errorf("write ida enricher %s: %w", name, err)
		}
		for k, v := range sc.Env {
			envMap[k] = v
		}
		orderedNames = append(orderedNames, name)
	}
	if len(orderedNames) == 0 {
		return exportScriptName, nil
	}

	orderedNames = append(orderedNames, exportScriptName)
	var py strings.Builder
	py.WriteString("import os, runpy, traceback, idc\n\n")
	py.WriteString("base_dir = os.path.dirname(os.path.abspath(__file__)) if '__file__' in globals() else os.getcwd()\n")
	py.WriteString("try:\n")
	for _, nm := range orderedNames {
		fmt.Fprintf(&py, "    runpy.run_path(os.path.join(base_dir, %q), run_name='__main__')\n", nm)
	}
	py.WriteString("except SystemExit:\n")
	py.WriteString("    raise\n")
	py.WriteString("except Exception:\n")
	py.WriteString("    traceback.print_exc()\n")
	py.WriteString("    idc.qexit(1)\n")
	entryName := "twdll_ida_run.py"
	if err := os.WriteFile(filepath.Join(workerDir, entryName), []byte(py.String()), 0o644); err != nil {
		return "", err
	}
	return entryName, nil
}

func runIDASilverExport(ctx context.Context, opts Options, enrichers []EnrichScript, asmOutPath, stringsOutPath string) ([]SilverFunc, error) {
	idaExe, err := analyze.ResolveIDAExecutable(opts.IDADir, opts.Target)
	if err != nil {
		return nil, err
	}
	srcI64, _, err := resolveIDADBPath(opts.Paths, opts.Target)
	if err != nil {
		return nil, err
	}

	workerDir, stagedI64, cleanup, err := stageIDAWorkerDir(srcI64, opts.Target, resolveTmpBase(opts.Paths, opts.TmpBaseDir), "twdll_ida_silver")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	outPath := filepath.Join(workerDir, "silver.jsonl")
	totalPath := filepath.Join(workerDir, "silver.total")
	logName := "ida_silver.log"
	envMap := map[string]string{
		"IDAUSR":                 filepath.Join(workerDir, "idausr"),
		"TWDLL_SILVER_OUT":       outPath,
		"TWDLL_SILVER_TOTAL_OUT": totalPath,
	}
	if strings.TrimSpace(asmOutPath) != "" {
		envMap["TWDLL_SILVER_ASM_OUT"] = asmOutPath
	}
	if strings.TrimSpace(stringsOutPath) != "" {
		envMap["TWDLL_SILVER_STRINGS_OUT"] = stringsOutPath
	}
	scriptName, err := stageIDAScriptsAndEnv(workerDir, "export_ida_silver.py", exportIDASilverPy, enrichers, envMap)
	if err != nil {
		return nil, err
	}

	env, err := analyze.PrepareStageEnv(workerDir, false, envMap)
	if err != nil {
		return nil, err
	}

	args := []string{"-A", "-S" + scriptName, "-L" + logName, filepath.Base(stagedI64)}
	cmd := exec.CommandContext(ctx, idaExe, args...)
	cmd.Dir = workerDir
	cmd.Env = env
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	stopWatch := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		watchJSONLLineProgress(ctx, outPath, totalPath, stopWatch, opts.OnProgress)
	}()

	_, runErr := analyze.RunObservedCommand(cmd, nil)
	close(stopWatch)
	<-watchDone
	if runErr != nil {
		tail := readLogTail(filepath.Join(workerDir, logName), 20)
		stdOut := strings.TrimSpace(outBuf.String())
		if tail == "" {
			tail = stdOut
		} else if stdOut != "" {
			tail = tail + "\nstdout/stderr:\n" + stdOut
		}
		return nil, fmt.Errorf("ida silver export failed (%w)\n%s", runErr, tail)
	}
	return parseSilverJSONL(outPath)
}

func resolveIDADecompWorkers(target workspace.TargetID, requested int) int {
	if requested > 0 {
		return requested
	}
	numCPU := runtime.NumCPU()
	if target.Is64Bit() {
		return max(1, min(4, numCPU/3))
	}
	return max(1, min(6, numCPU/2))
}

type idaWorkerConfig struct {
	workerID     int
	idaExe       string
	srcI64       string
	srcI64Size   int64
	target       workspace.TargetID
	engine       workspace.DecompilerID
	timeoutSecs  int
	startupGrace time.Duration
	tmpBaseDir   string
	enrichers    []EnrichScript
	shard        []extractdb.FuncSpec
	outCh        chan<- extractdb.DecompRecord
	onDone       func()
}

func runIDADecompExport(
	ctx context.Context,
	opts Options,
	enrichers []EnrichScript,
	pending []extractdb.FuncSpec,
	outCh chan<- extractdb.DecompRecord,
) (uint64, error) {
	idaExe, err := analyze.ResolveIDAExecutable(opts.IDADir, opts.Target)
	if err != nil {
		return 0, err
	}
	srcI64, srcSize, err := resolveIDADBPath(opts.Paths, opts.Target)
	if err != nil {
		return 0, err
	}

	workers := resolveIDADecompWorkers(opts.Target, opts.Workers)
	shards := BuildWorkerShards(pending, workers)
	if len(shards) == 0 {
		return 0, nil
	}

	timeoutSecs := resolveDecompTimeoutSecs(opts)

	tmpBase := resolveTmpBase(opts.Paths, opts.TmpBaseDir)
	total := len(pending)
	var progMu sync.Mutex
	var doneCount int
	var startedDecomp atomic.Bool
	var peakMem atomic.Uint64
	if opts.OnStatus != nil {
		opts.OnStatus("starting ida")
	}

	g, gCtx := errgroup.WithContext(ctx)
	for wIdx, shard := range shards {
		wIdx := wIdx
		shard := shard
		g.Go(func() error {
			wPeak, wErr := runIDAWorkerShard(gCtx, idaWorkerConfig{
				workerID:    wIdx,
				idaExe:      idaExe,
				srcI64:      srcI64,
				srcI64Size:  srcSize,
				target:      opts.Target,
				engine:      opts.Engine,
				timeoutSecs: timeoutSecs,
				tmpBaseDir:  tmpBase,
				enrichers:   enrichers,
				shard:       shard,
				outCh:       outCh,
				onDone: func() {
					if opts.OnStatus != nil && startedDecomp.CompareAndSwap(false, true) {
						opts.OnStatus("decomp")
					}
					if opts.OnProgress == nil {
						return
					}
					progMu.Lock()
					doneCount++
					n := doneCount
					opts.OnProgress(n, total)
					progMu.Unlock()
				},
			})
			peakMem.Add(wPeak)
			return wErr
		})
	}
	if err := g.Wait(); err != nil {
		return peakMem.Load(), err
	}
	return peakMem.Load(), nil
}

func runIDAWorkerShard(ctx context.Context, cfg idaWorkerConfig) (uint64, error) {
	workerDir, stagedI64, cleanup, err := stageIDAWorkerDir(
		cfg.srcI64,
		cfg.target,
		cfg.tmpBaseDir,
		fmt.Sprintf("twdll_ida_w%d", cfg.workerID),
	)
	if err != nil {
		return 0, err
	}
	defer cleanup()

	extraEnv := make(map[string]string)
	scriptName, err := stageIDAScriptsAndEnv(workerDir, "export_ida_decomp.py", exportIDADecompPy, cfg.enrichers, extraEnv)
	if err != nil {
		return 0, err
	}

	runner := func(attemptCtx context.Context, attempt int, remaining []extractdb.FuncSpec) idaAttemptOutcome {
		return executeIDAAttempt(attemptCtx, workerDir, stagedI64, scriptName, extraEnv, attempt, remaining, cfg)
	}
	return driveIDAWorkerShardWithWatchdog(ctx, workerDir, stagedI64, cfg, runner)
}

type idaAttemptOutcome struct {
	completed int
	peakMem   uint64
	err       error
}

type idaAttemptFunc func(ctx context.Context, attempt int, remaining []extractdb.FuncSpec) idaAttemptOutcome

func driveIDAWorkerShardWithWatchdog(
	ctx context.Context,
	workerDir string,
	stagedI64 string,
	cfg idaWorkerConfig,
	runAttempt idaAttemptFunc,
) (uint64, error) {
	var maxWorkerPeak uint64
	cursor := 0
	attempt := 0
	consecutiveZeroProgress := 0

	for cursor < len(cfg.shard) {
		if ctx.Err() != nil {
			return maxWorkerPeak, ctx.Err()
		}

		// Ensure unpacked sidecars are purged and staged .i64 is clean before every attempt.
		if attempt > 0 {
			cleanupIDAUnpackedSidecars(workerDir, cfg.target)
			if info, statErr := os.Stat(stagedI64); statErr != nil || (cfg.srcI64Size > 0 && info.Size() != cfg.srcI64Size) {
				if cpErr := workspace.CopyFileAtomic(cfg.srcI64, stagedI64); cpErr != nil {
					return maxWorkerPeak, fmt.Errorf("re-stage ida .i64 for worker %d: %w", cfg.workerID, cpErr)
				}
			}
		}

		remaining := cfg.shard[cursor:]
		outcome := runAttempt(ctx, attempt, remaining)
		attempt++
		if outcome.peakMem > maxWorkerPeak {
			maxWorkerPeak = outcome.peakMem
		}

		if ctx.Err() != nil {
			return maxWorkerPeak, ctx.Err()
		}

		if outcome.completed > 0 {
			cursor += outcome.completed
			consecutiveZeroProgress = 0
			continue
		}

		// 0 progress on attempt (e.g. startup crash / fatal IDA error).
		consecutiveZeroProgress++
		if consecutiveZeroProgress >= 2 || (outcome.err != nil && errors.Is(outcome.err, errIDANoProgressFatal)) {
			if outcome.err != nil {
				return maxWorkerPeak, outcome.err
			}
			return maxWorkerPeak, fmt.Errorf("%w: worker %d stopped at cursor %d/%d", errIDANoProgressFatal, cfg.workerID, cursor, len(cfg.shard))
		}
	}
	if cursor < len(cfg.shard) {
		return maxWorkerPeak, fmt.Errorf("%w: worker %d incomplete at cursor %d/%d", errIDANoProgressFatal, cfg.workerID, cursor, len(cfg.shard))
	}
	return maxWorkerPeak, nil
}

var errIDANoProgressFatal = errors.New("ida worker made zero progress")

func executeIDAAttempt(
	ctx context.Context,
	workerDir string,
	stagedI64 string,
	scriptName string,
	extraEnv map[string]string,
	attempt int,
	remaining []extractdb.FuncSpec,
	cfg idaWorkerConfig,
) idaAttemptOutcome {
	inPath := filepath.Join(workerDir, fmt.Sprintf("in_%d.tsv", attempt))
	outPath := filepath.Join(workerDir, fmt.Sprintf("out_%d.jsonl", attempt))
	logName := fmt.Sprintf("ida_w%d_a%d.log", cfg.workerID, attempt)

	if err := writeDecompTargetsTSV(inPath, remaining); err != nil {
		return idaAttemptOutcome{err: err}
	}

	envMap := map[string]string{
		"IDAUSR":           filepath.Join(workerDir, "idausr"),
		"TWDLL_DECOMP_IN":  inPath,
		"TWDLL_DECOMP_OUT": outPath,
	}
	for k, v := range extraEnv {
		envMap[k] = v
	}
	env, err := analyze.PrepareStageEnv(workerDir, false, envMap)
	if err != nil {
		return idaAttemptOutcome{err: err}
	}

	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	args := []string{"-A", "-S" + scriptName, "-L" + logName, filepath.Base(stagedI64)}
	cmd := exec.CommandContext(runCtx, cfg.idaExe, args...)
	cmd.Dir = workerDir
	cmd.Env = env
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	type procResult struct {
		metrics analyze.ProcessMetrics
		err     error
	}
	procDone := make(chan procResult, 1)
	var peakMem atomic.Uint64

	go func() {
		m, rErr := analyze.RunObservedCommand(cmd, func(s analyze.LiveProcessStats) {
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
		if m.PeakJobMemoryBytes > peakMem.Load() {
			peakMem.Store(m.PeakJobMemoryBytes)
		}
		procDone <- procResult{metrics: m, err: rErr}
	}()

	eaIndex := make(map[uint64]int, len(remaining))
	for i, spec := range remaining {
		eaIndex[spec.EA] = i
	}

	var (
		tailer          = &jsonlTailer{path: outPath}
		inFlight        *decompFrameJSON
		inFlightStarted time.Time
		completedUntil  int
		timedOut        bool
	)

	drainJSONL := func() error {
		return tailer.Drain(func(frame decompFrameJSON) {
			switch frame.Event {
			case "begin":
				cp := frame
				inFlight = &cp
				inFlightStarted = time.Now()
			case "result":
				rec := frame.toDecompRecord()
				if !sendDecompRecord(ctx, cfg.outCh, rec) {
					return
				}
				if cfg.onDone != nil {
					cfg.onDone()
				}
				if idx, ok := eaIndex[rec.FuncEA]; ok && idx+1 > completedUntil {
					completedUntil = idx + 1
				}
				inFlight = nil
			}
		})
	}

	var perFuncDeadline time.Duration
	if cfg.timeoutSecs > 0 {
		perFuncDeadline = time.Duration(cfg.timeoutSecs)*time.Second + 2*time.Second
	}
	attemptStarted := time.Now()
	startupDeadline := cfg.startupGrace
	if startupDeadline <= 0 && cfg.timeoutSecs > 0 {
		startupDeadline = perFuncDeadline + 15*time.Second
	}
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()

	var pRes procResult
	procExited := false

	for !procExited {
		select {
		case pRes = <-procDone:
			procExited = true
		case <-ticker.C:
			if dErr := drainJSONL(); dErr != nil {
				runCancel()
				<-procDone
				return idaAttemptOutcome{completed: completedUntil, peakMem: peakMem.Load(), err: dErr}
			}
			if cfg.timeoutSecs > 0 {
				if inFlight != nil {
					if time.Since(inFlightStarted) > perFuncDeadline {
						timedOut = true
						runCancel()
						pRes = <-procDone
						procExited = true
					}
				} else if completedUntil == 0 && startupDeadline > 0 && time.Since(attemptStarted) > startupDeadline {
					timedOut = true
					runCancel()
					pRes = <-procDone
					procExited = true
				}
			}
		}
	}

	if dErr := drainJSONL(); dErr != nil {
		return idaAttemptOutcome{completed: completedUntil, peakMem: peakMem.Load(), err: dErr}
	}

	if ctx.Err() != nil {
		return idaAttemptOutcome{completed: completedUntil, peakMem: peakMem.Load(), err: ctx.Err()}
	}

	if inFlight != nil {
		// Worker either timed out or crashed while processing inFlight.EA.
		status := extractdb.DecompStatusFailed
		errMsg := fmt.Sprintf("ida worker exited prematurely (%v)", pRes.err)
		if timedOut {
			status = extractdb.DecompStatusTimeout
			errMsg = fmt.Sprintf("decompile timeout (>%ds; worker killed)", cfg.timeoutSecs)
		}
		rec := extractdb.DecompRecord{
			FuncEA:    uint64(inFlight.EA),
			FuncSize:  uint64(inFlight.Size),
			FuncName:  inFlight.Name,
			Status:    status,
			ErrorText: errMsg,
			ElapsedMs: time.Since(inFlightStarted).Milliseconds(),
		}
		if !sendDecompRecord(ctx, cfg.outCh, rec) {
			return idaAttemptOutcome{completed: completedUntil, peakMem: peakMem.Load(), err: ctx.Err()}
		}
		if cfg.onDone != nil {
			cfg.onDone()
		}
		if idx, ok := eaIndex[rec.FuncEA]; ok && idx+1 > completedUntil {
			completedUntil = idx + 1
		}
	} else if completedUntil == 0 && len(remaining) > 0 {
		tail := readLogTail(filepath.Join(workerDir, logName), 20)
		stdOut := strings.TrimSpace(outBuf.String())
		if tail == "" {
			tail = stdOut
		} else if stdOut != "" {
			tail = tail + "\nstdout/stderr:\n" + stdOut
		}
		cause := "exited 0 without JSONL progress"
		if timedOut {
			cause = fmt.Sprintf("startup timeout (>%s before first begin frame)", startupDeadline)
		} else if pRes.err != nil {
			cause = pRes.err.Error()
		}
		return idaAttemptOutcome{
			completed: 0,
			peakMem:   peakMem.Load(),
			err:       fmt.Errorf("%w: %s\n%s", errIDANoProgressFatal, cause, tail),
		}
	}

	return idaAttemptOutcome{
		completed: completedUntil,
		peakMem:   peakMem.Load(),
	}
}
