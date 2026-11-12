package analyze

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"twdll/cli/internal/ui"
	"twdll/cli/internal/workspace"
)

// RunAnalyze coordinates planning, headless execution, and atomic state checkpointing.
func RunAnalyze(ctx context.Context, paths workspace.Paths, opts Options, printer *ui.Printer) error {
	cfg, cfgExists, err := workspace.LoadConfig(paths.Config)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if !cfgExists {
		return fmt.Errorf("workspace is not initialized (run `twdll init` first)")
	}

	state, _, err := workspace.LoadState(paths.StateFile)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	items, err := BuildPlan(cfg, &state, paths, opts)
	if err != nil {
		return err
	}

	var toRun []Item
	for _, it := range items {
		if it.NeedsRun {
			toRun = append(toRun, it)
		} else {
			if printer != nil {
				printer.Info("  [analyze] %s: %s %s", printer.Bold(string(it.Target)), printer.Green("up to date"), printer.Dim("("+it.SkipReason+")"))
			}
		}
	}

	if len(toRun) == 0 {
		if printer != nil {
			printer.Info("[analyze] All requested databases are up to date. (Use --force to re-analyze)")
		}
		return nil
	}

	releasePower := acquirePowerAssertion()
	defer releasePower()

	sysMem, _ := getSystemMemory()

	concurrency := opts.Jobs
	if concurrency <= 0 {
		if toRun[0].Decompiler == workspace.DecompilerIDA {
			concurrency = len(toRun) // Run all games at once for lightweight native IDA!
		} else {
			concurrency = 2
			if concurrency > len(toRun) {
				concurrency = len(toRun)
			}
		}
	}
	if concurrency > len(toRun) {
		concurrency = len(toRun)
	}

	if printer != nil {
		engine := toRun[0].Decompiler
		if engine == workspace.DecompilerGhidra && sysMem.TotalPhysBytes > 0 {
			const gb = 1024 * 1024 * 1024
			printer.Info("[analyze] System memory: %d GB RAM (%d GB virtual commit limit)", sysMem.TotalPhysBytes/gb, sysMem.TotalCommitBytes/gb)
		}
		if concurrency > 1 {
			printer.Info("[analyze] Starting analysis of %d target(s) using %s (parallel jobs: %d)...", len(toRun), printer.Bold(string(engine)), concurrency)
		} else {
			printer.Info("[analyze] Starting analysis of %d target(s) using %s...", len(toRun), printer.Bold(string(engine)))
		}
	}

	multiSpinner := ui.NewMultiSpinner(printer)
	for i, it := range toRun {
		multiSpinner.AddSlot(i+1, len(toRun), string(it.Target), it.LogFile)
	}
	multiSpinner.Start()
	defer multiSpinner.Stop()

	overallStart := time.Now()
	var stateMu sync.Mutex
	errCh := make(chan error, len(toRun))

	executeItem := func(it Item, slot *ui.TaskSlot, workerConcurrency int) {
		slot.Start()

		if opts.Clean {
			_ = os.RemoveAll(it.OutputDir)
			_ = os.Remove(it.LogFile)
		}

		var runErr error
		switch it.Decompiler {
		case workspace.DecompilerIDA:
			runErr = RunIDABatch(ctx, cfg.IDADir, it, slot, printer)
		case workspace.DecompilerGhidra:
			maxMem := opts.MaxMem
			if maxMem == "" {
				maxMem = ComputeGhidraMaxMem(sysMem, it.Target, workerConcurrency)
			}
			stripHeavy := it.Target == workspace.TargetAttila64 && !opts.FullAnalysis
			runErr = RunGhidraHeadless(ctx, paths.ToolsDir, it, maxMem, stripHeavy, slot, printer)
		default:
			runErr = fmt.Errorf("unsupported decompiler: %q", it.Decompiler)
		}

		if runErr != nil {
			prefix := fmt.Sprintf("  [%s] %s failed", it.Decompiler, it.Target)
			if slot.Total > 1 {
				prefix = fmt.Sprintf("  [%d/%d] [%s] %s failed", slot.Index, slot.Total, it.Decompiler, it.Target)
			}
			slot.Finish(fmt.Sprintf("%s: %v", prefix, runErr))
			errCh <- fmt.Errorf("failed analyzing %s: %w", it.Target, runErr)
			return
		}

		// Atomically record completion in state
		stateMu.Lock()
		binState, exists := state.Binaries[it.Target]
		if !exists {
			binState = workspace.BinaryState{
				SourcePath:   it.BinaryPath,
				BinarySHA256: it.BinarySHA256,
			}
		}
		binState.BinarySHA256 = it.BinarySHA256
		binState.Analyzed.Set(it.Decompiler, it.BinarySHA256)
		state.SetBinary(it.Target, binState)
		_ = workspace.SaveState(paths.StateFile, state)
		stateMu.Unlock()
	}

	var soloIndices []int
	var parallelIndices []int
	for i, it := range toRun {
		if it.Decompiler == workspace.DecompilerGhidra && it.Target == workspace.TargetAttila64 {
			soloIndices = append(soloIndices, i)
		} else {
			parallelIndices = append(parallelIndices, i)
		}
	}

	// Phase 1: Run heavy 64-bit Ghidra reference binary solo with full RAM headroom.
	for _, idx := range soloIndices {
		if ctx.Err() != nil {
			break
		}
		executeItem(toRun[idx], multiSpinner.Slot(idx), 1)
	}

	// Phase 2: Run remaining targets concurrently up to concurrency limit.
	if len(parallelIndices) > 0 && ctx.Err() == nil {
		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup

		for _, idx := range parallelIndices {
			it := toRun[idx]
			slot := multiSpinner.Slot(idx)
			wg.Add(1)

			go func(it Item, slot *ui.TaskSlot) {
				defer wg.Done()

				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()

				executeItem(it, slot, concurrency)
			}(it, slot)
		}

		wg.Wait()
	}

	close(errCh)
	multiSpinner.Stop()

	var firstErr error
	for err := range errCh {
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}

	if printer != nil {
		printer.Info("%s Completed %d target(s) in %s", printer.Green("[analyze] Success!"), len(toRun), time.Since(overallStart).Round(time.Second))
	}

	return nil
}
