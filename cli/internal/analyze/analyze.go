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
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var stateMu sync.Mutex
	errCh := make(chan error, len(toRun))

	for i, it := range toRun {
		slot := multiSpinner.Slot(i)
		wg.Add(1)

		go func(it Item, slot *ui.TaskSlot) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

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
				runErr = RunGhidraHeadless(ctx, paths.ToolsDir, it, "8G", slot, printer)
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
		}(it, slot)
	}

	wg.Wait()
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
