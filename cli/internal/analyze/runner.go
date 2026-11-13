package analyze

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

func Run(ctx context.Context, req Request) error {
	if !req.Layout.ConfigExists {
		return &term.DiagnosticError{
			Message:  fmt.Sprintf("workspace is not initialized (missing %s)", filepath.ToSlash(req.Layout.Paths.Config)),
			Expected: "an initialized twdll workspace configuration",
			Help:     []string{"Run `twdll init` to configure games, stage binaries, and install decompiler tools."},
		}
	}

	paths := req.Layout.Paths
	state, _, err := workspace.LoadState(paths.StateFile)
	if err != nil {
		return fmt.Errorf("load workspace state: %w", err)
	}

	engine, plan, err := BuildPlan(PlanInput{Layout: req.Layout, State: state, Options: req.Options})
	if err != nil {
		return err
	}

	toRun := make([]PlanItem, 0, len(plan))
	for _, item := range plan {
		if !item.NeedsRun {
			if req.UI != nil {
				req.UI.Success("%s (%s) already analyzed and up to date", item.Target, engine)
			}
			continue
		}
		toRun = append(toRun, item)
	}
	if len(toRun) == 0 {
		if req.UI != nil && len(plan) == 0 {
			req.UI.Info("No targets selected for analysis.")
		}
		return nil
	}

	ghTools, idaExes, err := resolveBackendTools(req.Layout, engine, toRun)
	if err != nil {
		return err
	}
	if req.Options.Clean {
		if err := clearPlannedDatabases(paths, &state, engine, toRun, req.UI); err != nil {
			return err
		}
	}

	releasePower := acquirePowerAssertion()
	defer releasePower()

	mem := ProbeSystemMemory()
	runner := &batchRunner{req: req, engine: engine, state: state, mem: mem, ghTools: ghTools, idaExes: idaExes}

	var batch32, batch64 []PlanItem
	for _, item := range toRun {
		if item.Target == workspace.TargetAttila64 {
			batch64 = append(batch64, item)
		} else {
			batch32 = append(batch32, item)
		}
	}

	totalBatches := 0
	if len(batch32) > 0 {
		totalBatches++
	}
	if len(batch64) > 0 {
		totalBatches++
	}
	batchIdx := 0

	if len(batch32) > 0 {
		batchIdx++
		var workers32 int
		if engine == workspace.DecompilerGhidra {
			workers32 = resolveGhidra32WorkerCount(len(batch32), req.Options.Jobs, mem)
		} else {
			workers32 = resolveIDA32WorkerCount(len(batch32), req.Options.Jobs, runtime.NumCPU(), mem)
		}
		header := fmt.Sprintf("[%d/%d] %s 32-bit (%d workers)", batchIdx, totalBatches, engine, workers32)
		if err := runner.runBatch(ctx, header, batch32, workers32); err != nil {
			return err
		}
	}
	if len(batch64) > 0 {
		batchIdx++
		runner.mem = ProbeSystemMemory()
		header := fmt.Sprintf("[%d/%d] %s 64-bit (solo)", batchIdx, totalBatches, engine)
		if err := runner.runBatch(ctx, header, batch64, 1); err != nil {
			return err
		}
	}
	if req.UI != nil {
		req.UI.Info("Resource telemetry recorded in %s", filepath.ToSlash(req.Layout.Paths.AnalyzeResourceLogPath()))
	}
	return nil
}

func resolveBackendTools(layout workspace.Layout, engine workspace.DecompilerID, items []PlanItem) (ghidraToolchain, map[workspace.TargetID]string, error) {
	if engine == workspace.DecompilerGhidra {
		gh, err := resolveGhidraToolchain(layout.Paths)
		return gh, nil, err
	}
	exes := make(map[workspace.TargetID]string, len(items))
	for _, item := range items {
		exe, err := resolveIDAExecutable(layout.Config.IDADir, item.Target)
		if err != nil {
			return ghidraToolchain{}, nil, err
		}
		exes[item.Target] = exe
	}
	return ghidraToolchain{}, exes, nil
}

func clearPlannedDatabases(paths workspace.Paths, state *workspace.State, engine workspace.DecompilerID, items []PlanItem, ui *term.UI) error {
	dirty := false
	for _, item := range items {
		if err := os.RemoveAll(item.DatabaseDir); err != nil {
			return fmt.Errorf("clean database dir %s: %w", item.DatabaseDir, err)
		}
		if b, ok := state.Binaries[item.Target]; ok && b.Analyzed.Get(engine) != "" {
			b.Analyzed.Set(engine, "")
			state.Binaries[item.Target] = b
			dirty = true
		}
		if ui != nil {
			ui.Info("Cleared existing %s database for %s", engine, item.Target)
		}
	}
	if dirty {
		if err := workspace.SaveState(paths.StateFile, *state); err != nil {
			return fmt.Errorf("save state after clean: %w", err)
		}
	}
	return nil
}

type batchRunner struct {
	req     Request
	engine  workspace.DecompilerID
	mem     SystemMemory
	ghTools ghidraToolchain
	idaExes map[workspace.TargetID]string

	mu    sync.Mutex
	state workspace.State
}

func (r *batchRunner) runBatch(ctx context.Context, header string, items []PlanItem, workers int) error {
	if len(items) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}

	targets := make([]workspace.TargetID, len(items))
	for i, item := range items {
		targets[i] = item.Target
	}
	obs := newBatchObserver(r.req.UI, r.engine, header, targets)
	obs.Start()
	defer obs.Close()

	var (
		g      errgroup.Group
		errMu  sync.Mutex
		runErr []error
	)
	g.SetLimit(workers)

	for _, item := range items {
		item := item
		g.Go(func() error {
			if ctx.Err() != nil {
				obs.TargetFinished(item.Target, targetCanceled, ProcessMetrics{}, "", "canceled")
				errMu.Lock()
				runErr = append(runErr, ctx.Err())
				errMu.Unlock()
				return nil
			}
			if err := r.executeOne(ctx, item, workers, obs); err != nil {
				errMu.Lock()
				runErr = append(runErr, err)
				errMu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	return errors.Join(runErr...)
}

func (r *batchRunner) executeOne(ctx context.Context, item PlanItem, concurrentJobs int, obs *batchObserver) error {
	ui := r.req.UI
	start := time.Now()
	if ui != nil && !ui.CanAnimate() {
		ui.Info("Analyzing %s with %s (log: %s)...", item.Target, r.engine, filepath.ToSlash(item.LogPath))
	}
	obs.TargetStarted(item.Target)

	onLine := func(line string) {
		obs.OnLine(item.Target, line)
	}
	onSample := func(s LiveProcessStats) {
		obs.OnSample(item.Target, s)
	}

	var (
		res targetRunResult
		err error
	)
	if r.engine == workspace.DecompilerGhidra {
		res, err = runGhidraTarget(ctx, ghidraJobSpec{
			toolchain:      r.ghTools,
			item:           item,
			opts:           r.req.Options,
			mem:            r.mem,
			concurrentJobs: concurrentJobs,
			onLogLine:      onLine,
			onSample:       onSample,
		})
	} else {
		res, err = runIDATarget(ctx, idaJobSpec{
			exePath:   r.idaExes[item.Target],
			item:      item,
			opts:      r.req.Options,
			onLogLine: onLine,
			onSample:  onSample,
		})
	}

	if res.Started {
		status := "ok"
		errMsg := ""
		if err != nil {
			if ctx.Err() != nil {
				status = "canceled"
			} else {
				status = "failed"
			}
			errMsg = err.Error()
		}
		rec := ResourceRecord{
			Timestamp:            time.Now().UTC().Format(time.RFC3339),
			Engine:               string(r.engine),
			Target:               string(item.Target),
			Status:               status,
			Error:                errMsg,
			ConcurrentJobs:       concurrentJobs,
			GhidraMaxMem:         res.GhidraMaxMem,
			WallSeconds:          res.Metrics.WallDuration.Seconds(),
			CPUSeconds:           res.Metrics.CPUTime.Seconds(),
			UserSeconds:          res.Metrics.UserTime.Seconds(),
			KernelSeconds:        res.Metrics.KernelTime.Seconds(),
			PeakJobMemoryBytes:   res.Metrics.PeakJobMemoryBytes,
			PeakProcMemoryBytes:  res.Metrics.PeakProcMemoryBytes,
			TotalProcesses:       res.Metrics.TotalProcesses,
			JobAssigned:          res.Metrics.JobAssigned,
			TotalPhysBytes:       r.mem.TotalPhysBytes,
			AvailPhysBeforeBytes: res.Metrics.AvailPhysBeforeBytes,
			AvailPhysMinBytes:    res.Metrics.AvailPhysMinBytes,
			AvailPhysAfterBytes:  res.Metrics.AvailPhysAfterBytes,
		}
		r.mu.Lock()
		logErr := appendResourceRecord(r.req.Layout.Paths.AnalyzeResourceLogPath(), rec)
		r.mu.Unlock()
		if logErr != nil && ui != nil {
			ui.Warn("Could not record resource telemetry for %s: %v", item.Target, logErr)
		}
	}

	if err != nil {
		if ctx.Err() != nil {
			obs.TargetFinished(item.Target, targetCanceled, res.Metrics, res.GhidraMaxMem, "canceled")
		} else {
			obs.TargetFinished(item.Target, targetFailed, res.Metrics, res.GhidraMaxMem, "see "+filepath.Base(item.LogPath))
		}
		return err
	}

	r.mu.Lock()
	sourcePath := r.state.Binaries[item.Target].SourcePath
	if sourcePath == "" {
		sourcePath = item.SourcePath
	}
	r.state.UpsertBinary(item.Target, sourcePath, item.BinarySHA256)
	r.state.SetAnalyzed(item.Target, r.engine, item.BinarySHA256)
	saveErr := workspace.SaveState(r.req.Layout.Paths.StateFile, r.state)
	r.mu.Unlock()
	if saveErr != nil {
		obs.TargetFinished(item.Target, targetFailed, res.Metrics, res.GhidraMaxMem, saveErr.Error())
		return fmt.Errorf("checkpoint state for %s: %w", item.Target, saveErr)
	}

	obs.TargetFinished(item.Target, targetOK, res.Metrics, res.GhidraMaxMem, "")

	if ui != nil && !ui.CanAnimate() {
		elapsed := time.Since(start).Round(100 * time.Millisecond)
		if summary := formatResourceSummary(res.Metrics, res.GhidraMaxMem); summary != "" {
			ui.Success("Analyzed %s (%s) in %s (%s)", item.Target, r.engine, elapsed, summary)
		} else {
			ui.Success("Analyzed %s (%s) in %s", item.Target, r.engine, elapsed)
		}
	}
	return nil
}
