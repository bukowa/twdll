package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"twdll/cli/internal/bronze"
	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/silver"
	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

type ExtractOptions struct {
	Decompiler    string
	Force         bool
	Jobs          int
	MaxMem        string
	NoDecomp      bool
	TimeoutSecs   int
	NoTimeout     bool
	Retry         bool
	RetryTimeouts bool
	RetryFailed   bool
	JSON          bool
	Targets       []string
}

type ExtractRequest struct {
	Layout        workspace.Layout
	Options       ExtractOptions
	UI            *term.UI
	ExtractBronze func(ctx context.Context, opts bronze.Options) (*bronze.Result, error)
	ExtractSilver func(ctx context.Context, opts silver.Options) (*silver.Result, error)
}

type ExtractEngineReport struct {
	Engine        string         `json:"engine"`
	FuncsRan      bool           `json:"funcs_ran"`
	SilverSkipped bool           `json:"silver_skipped,omitempty"`
	FuncCount     int            `json:"func_count,omitempty"`
	AsmCount      int            `json:"asm_count,omitempty"`
	StringCount   int            `json:"string_count,omitempty"`
	DecompRan     bool           `json:"decomp_ran"`
	DecompWritten int            `json:"decomp_written"`
	StatusCounts  map[string]int `json:"status_counts,omitempty"`
}

type ExtractTargetReport struct {
	Target       workspace.TargetID    `json:"target"`
	BinarySHA256 string                `json:"binary_sha256"`
	BronzeRan    bool                  `json:"bronze_ran"`
	BronzeCounts map[string]int64      `json:"bronze_counts,omitempty"`
	Engines      []ExtractEngineReport `json:"engines,omitempty"`
}

type ExtractReport struct {
	Targets []ExtractTargetReport `json:"targets"`
}

type plannedExtractTarget struct {
	target     workspace.TargetID
	binaryPath string
	sha256     string
	freshDBs   map[workspace.DecompilerID]bool
}

type bronzeDBState struct {
	upToDate bool
	counts   map[string]int64
}

func RunExtract(ctx context.Context, req ExtractRequest) (*ExtractReport, error) {
	if !req.Layout.ConfigExists {
		return nil, &term.DiagnosticError{
			Message:  fmt.Sprintf("workspace is not initialized (missing %s)", filepath.ToSlash(req.Layout.Paths.Config)),
			Expected: "an initialized twdll workspace configuration",
			Help:     []string{"Run `twdll init` to configure games, stage binaries, and install decompiler tools."},
		}
	}

	opts := req.Options
	engines, err := resolveExtractEngines(opts.Decompiler, req.Layout.Config.Decompiler)
	if err != nil {
		return nil, err
	}

	targets, err := resolveExtractTargets(req.Layout, opts.Targets)
	if err != nil {
		return nil, err
	}

	paths := req.Layout.Paths
	state, _, err := workspace.LoadState(paths.StateFile)
	if err != nil {
		return nil, fmt.Errorf("load workspace state: %w", err)
	}

	planned := make([]plannedExtractTarget, 0, len(targets))
	for _, tid := range targets {
		binPath := paths.BinaryPath(tid)
		info, sErr := os.Stat(binPath)
		if sErr != nil || info.IsDir() {
			return nil, &term.DiagnosticError{
				Message:  fmt.Sprintf("staged binary for %s not found at %s", tid, filepath.ToSlash(binPath)),
				Expected: fmt.Sprintf("staged %s binary in workspace", tid.BinaryFileName()),
				Help:     []string{"Run `twdll init` to verify and stage game binaries."},
			}
		}

		currentSHA, hErr := workspace.ComputeFileSHA256(binPath)
		if hErr != nil {
			return nil, fmt.Errorf("hash binary for %s: %w", tid, hErr)
		}

		freshDBs := make(map[workspace.DecompilerID]bool, len(engines))
		for _, engine := range engines {
			hasDiskDB := paths.HasValidDatabase(engine, tid)
			analyzedSHA := strings.TrimSpace(state.Binaries[tid].Analyzed.Get(engine))
			dbFresh := hasDiskDB && strings.EqualFold(analyzedSHA, currentSHA)
			freshDBs[engine] = dbFresh
			if !dbFresh && !opts.NoDecomp && req.ExtractSilver == nil {
				msg := fmt.Sprintf("decompiler database for %s (%s) is not analyzed", tid, engine)
				if hasDiskDB && analyzedSHA != "" {
					msg = fmt.Sprintf("decompiler database for %s (%s) is stale (binary changed since last analyze)", tid, engine)
				}
				return nil, &term.DiagnosticError{
					Message:  msg,
					Expected: fmt.Sprintf("analyzed %s database at %s", engine, filepath.ToSlash(paths.DatabaseDir(engine, tid))),
					Help:     []string{fmt.Sprintf("Run `twdll analyze -d %s %s` before extracting, or pass `--no-decomp` to extract bronze only.", engine, tid)},
				}
			}
		}

		planned = append(planned, plannedExtractTarget{
			target:     tid,
			binaryPath: binPath,
			sha256:     currentSHA,
			freshDBs:   freshDBs,
		})
	}

	extractBronze := req.ExtractBronze
	if extractBronze == nil {
		extractBronze = bronze.ExtractTarget
	}
	extractSilver := req.ExtractSilver
	if extractSilver == nil {
		extractSilver = silver.ExtractTarget
	}

	var logUI *term.UI
	if !opts.JSON {
		logUI = req.UI
	}
	animated := logUI != nil && logUI.CanAnimate()
	var obs *extractObserver
	if animated {
		obs = newExtractObserver(logUI, engines, opts, planned)
		obs.Start()
		defer obs.Close()
	}

	report := &ExtractReport{
		Targets: make([]ExtractTargetReport, 0, len(planned)),
	}

	for idx, item := range planned {
		tid := item.target
		bronzeKey := extractRowKey{target: tid, step: extractStepBronze}
		if err := ctx.Err(); err != nil {
			if obs != nil {
				obs.CancelTargetRemaining(tid)
				obs.Close()
			}
			return nil, err
		}

		if err := os.MkdirAll(paths.ExtractedTargetDir(tid), 0o755); err != nil {
			if obs != nil {
				obs.RowFailed(bronzeKey, false, err.Error())
				obs.Close()
			}
			return nil, fmt.Errorf("create extracted dir for %s: %w", tid, err)
		}

		if !animated && logUI != nil {
			logUI.Step(idx+1, len(planned), fmt.Sprintf("Extract %s", tid))
		}

		bronzeDBPath := paths.BronzeDBPath(tid)
		manifestPath := paths.TargetManifestPath(tid)
		bronzeState := inspectBronzeState(ctx, bronzeDBPath, item.sha256)

		tRep := ExtractTargetReport{
			Target:       tid,
			BinarySHA256: item.sha256,
			BronzeCounts: bronzeState.counts,
			Engines:      make([]ExtractEngineReport, 0, len(engines)),
		}

		// Layer 1 (bronze): deterministic format and DWARF parsing -> bronze.sqlite.
		if opts.Force || !bronzeState.upToDate {
			if obs != nil {
				obs.RowStarted(bronzeKey, "extracting...")
			}
			res, err := extractBronze(ctx, bronze.Options{
				Target:       tid,
				BinaryPath:   item.binaryPath,
				BinarySHA256: item.sha256,
				BronzeDBPath: bronzeDBPath,
				ManifestPath: manifestPath,
			})
			if err != nil {
				if obs != nil {
					obs.RowFailed(bronzeKey, ctx.Err() != nil, err.Error())
					obs.Close()
				}
				return nil, fmt.Errorf("extract bronze for %s: %w", tid, err)
			}
			tRep.BronzeRan = true
			if res != nil {
				tRep.BronzeCounts = res.Counts
			}
			if obs != nil {
				obs.RowFinished(bronzeKey, buildBronzeSummary(tRep.BronzeCounts, false))
			} else if logUI != nil {
				logUI.Success("%s Bronze extracted (sections=%d, strings=%d, dwarf_funcs=%d)",
					tid,
					tRep.BronzeCounts["sections"],
					tRep.BronzeCounts["strings"],
					tRep.BronzeCounts["dwarf_funcs"],
				)
			}
		} else if obs != nil {
			obs.RowFinished(bronzeKey, buildBronzeSummary(tRep.BronzeCounts, true))
		} else if logUI != nil {
			logUI.Success("%s Bronze already up to date", tid)
		}

		// Layer 2 (silver): independent per-engine IDA/Ghidra extraction -> silver_<engine>.sqlite.
		for _, engine := range engines {
			eRep := ExtractEngineReport{
				Engine: string(engine),
			}
			silverKey := extractRowKey{target: tid, step: string(engine)}

			hasDB := item.freshDBs[engine] || req.ExtractSilver != nil
			if !hasDB {
				eRep.SilverSkipped = true
				if obs != nil {
					obs.RowSkipped(silverKey, "skipped (db missing or stale)")
				} else if logUI != nil {
					logUI.Warn("Skipping %s (%s) Silver extract: decompiler database missing or stale", tid, engine)
				}
				tRep.Engines = append(tRep.Engines, eRep)
				continue
			}

			var phaseMu sync.Mutex
			curPhase := "starting"
			getPhase := func() string {
				phaseMu.Lock()
				defer phaseMu.Unlock()
				return curPhase
			}
			sOpts := silver.Options{
				Paths:         paths,
				Target:        tid,
				Engine:        engine,
				IDADir:        req.Layout.Config.IDADir,
				BinaryPath:    item.binaryPath,
				BinarySHA256:  item.sha256,
				SilverDBPath:  paths.SilverDBPath(tid, engine),
				NoDecomp:      opts.NoDecomp,
				Force:         opts.Force,
				TimeoutSecs:   opts.TimeoutSecs,
				NoTimeout:     opts.NoTimeout,
				RetryFailed:   opts.Retry || opts.RetryFailed,
				RetryTimeouts: opts.Retry || opts.RetryTimeouts,
				Workers:       opts.Jobs,
				MaxMem:        opts.MaxMem,
			}
			if obs != nil {
				obs.RowStarted(silverKey, "starting...")
				sOpts.OnPhase = func(sub string) {
					phaseMu.Lock()
					if sub != "" {
						curPhase = sub
					} else {
						curPhase = "running"
					}
					p := curPhase
					phaseMu.Unlock()
					obs.RowPhase(silverKey, p+"...")
				}
				sOpts.OnStatus = func(status string) {
					if status != "" {
						obs.RowStatus(silverKey, status)
					}
				}
				sOpts.OnResume = func(cached, pending int) {
					obs.RowResume(silverKey, cached, pending)
				}
				sOpts.OnProgress = func(done, total int) {
					obs.RowProgress(silverKey, getPhase(), done, total)
				}
				sOpts.OnBytes = func(doneBytes, totalBytes uint64) {
					obs.RowBytes(silverKey, doneBytes, totalBytes)
				}
			} else if logUI != nil {
				logUI.Info("%s (%s): starting silver extraction...", tid, engine)
				var progMu sync.Mutex
				lastAt := time.Now()
				lastDone := 0
				sOpts.OnPhase = func(sub string) {
					if sub != "" {
						progMu.Lock()
						lastDone = 0
						lastAt = time.Now()
						progMu.Unlock()
						logUI.Info("  %s (%s): %s...", tid, engine, sub)
					}
				}
				sOpts.OnResume = func(cached, pending int) {
					if cached > 0 {
						logUI.Info("  %s (%s): resuming (%d cached, %d pending)", tid, engine, cached, pending)
					}
				}
				sOpts.OnStatus = func(status string) {
					if status != "" && status != "decomp" {
						logUI.Info("  %s (%s): %s...", tid, engine, status)
					}
				}
				sOpts.OnProgress = func(done, total int) {
					progMu.Lock()
					defer progMu.Unlock()
					now := time.Now()
					if total > 0 {
						if done == 0 || (done < total && done-lastDone < 2500 && now.Sub(lastAt) < 10*time.Second) {
							return
						}
						lastDone = done
						lastAt = now
						pct := (done * 100) / total
						logUI.Info("  %s (%s): %d/%d (%d%%)", tid, engine, done, total, pct)
						return
					}
					if done > 0 && (done-lastDone >= 5000 || now.Sub(lastAt) >= 10*time.Second) {
						lastDone = done
						lastAt = now
						logUI.Info("  %s (%s): %d fn", tid, engine, done)
					}
				}
			}

			sRes, err := extractSilver(ctx, sOpts)
			if err != nil {
				if obs != nil {
					obs.RowFailed(silverKey, ctx.Err() != nil, err.Error())
					obs.Close()
				}
				return nil, fmt.Errorf("extract %s silver for %s: %w", engine, tid, err)
			}
			var enginePeakMem uint64
			if sRes != nil {
				eRep.FuncsRan = sRes.FuncsRan
				eRep.FuncCount = sRes.FuncCount
				eRep.AsmCount = sRes.AsmCount
				eRep.StringCount = sRes.StringCount
				eRep.DecompRan = sRes.DecompRan
				eRep.DecompWritten = sRes.DecompWritten
				eRep.StatusCounts = sRes.StatusCounts
				enginePeakMem = sRes.PeakJobMemBytes
			}
			if obs != nil {
				obs.RowFinished(silverKey, buildSilverSummary(eRep, enginePeakMem, opts.NoDecomp))
			} else if logUI != nil {
				if opts.NoDecomp {
					logUI.Success("%s (%s) Silver metadata extracted (%d funcs, %d asm, %d strings)", tid, engine, eRep.FuncCount, eRep.AsmCount, eRep.StringCount)
				} else {
					logUI.Success(
						"%s (%s) Silver extraction complete (funcs=%d, written=%d, ok=%d failed=%d timeout=%d)",
						tid,
						engine,
						eRep.FuncCount,
						eRep.DecompWritten,
						eRep.StatusCounts[extractdb.DecompStatusOK],
						eRep.StatusCounts[extractdb.DecompStatusFailed],
						eRep.StatusCounts[extractdb.DecompStatusTimeout],
					)
				}
			}
			tRep.Engines = append(tRep.Engines, eRep)
		}

		report.Targets = append(report.Targets, tRep)
	}

	if obs != nil {
		obs.Close()
	}

	if opts.JSON && req.UI != nil {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode json extract report: %w", err)
		}
		req.UI.WriteData("%s\n", string(data))
	}
	return report, nil
}

func resolveExtractEngines(override string, configured workspace.DecompilerID) ([]workspace.DecompilerID, error) {
	raw := strings.ToLower(strings.TrimSpace(override))
	if raw == "" {
		raw = string(configured)
	}
	if raw == "all" {
		return workspace.AllDecompilers(), nil
	}
	engine, ok := workspace.ParseDecompilerID(raw)
	if !ok {
		return nil, &term.DiagnosticError{
			Message:  fmt.Sprintf("unknown decompiler %q", raw),
			Expected: "ghidra, ida, or all",
			Help:     []string{"Pass `--decompiler ghidra`, `--decompiler ida`, or `--decompiler all`, or set `decompiler` in twdll.toml."},
		}
	}
	return []workspace.DecompilerID{engine}, nil
}

func resolveExtractTargets(layout workspace.Layout, rawTargets []string) ([]workspace.TargetID, error) {
	targets, err := workspace.ExpandTargets(rawTargets, workspace.ConfiguredTargets(layout))
	if err != nil {
		return nil, &term.DiagnosticError{
			Message:  err.Error(),
			Expected: "target IDs or glob patterns matching: attila_64, attila_32, rome2_32, tob_32",
			Help:     []string{"Specify valid target IDs/globs (e.g. `*_32`, `attila_*`) or omit positional arguments to extract all configured targets."},
		}
	}
	return targets, nil
}

func inspectBronzeState(ctx context.Context, dbPath, wantSHA string) bronzeDBState {
	summary := extractdb.InspectBronzeStatus(ctx, dbPath)
	if !summary.Present || summary.Locked || summary.Error != "" {
		return bronzeDBState{}
	}
	upToDate := wantSHA != "" && strings.EqualFold(strings.TrimSpace(summary.BinarySHA256), strings.TrimSpace(wantSHA))
	if !upToDate {
		return bronzeDBState{}
	}
	return bronzeDBState{
		upToDate: true,
		counts: map[string]int64{
			"sections":    int64(summary.Sections),
			"strings":     int64(summary.Strings),
			"dwarf_funcs": int64(summary.DwarfFuncs),
		},
	}
}

func buildBronzeSummary(counts map[string]int64, cached bool) string {
	var parts []string
	if df := counts["dwarf_funcs"]; df > 0 {
		parts = append(parts, fmt.Sprintf("%d dwarf_fn", df))
	}
	if str := counts["strings"]; str > 0 {
		parts = append(parts, fmt.Sprintf("%d str", str))
	}
	if sec := counts["sections"]; sec > 0 {
		parts = append(parts, fmt.Sprintf("%d sec", sec))
	}
	detail := strings.Join(parts, ", ")
	if cached {
		if detail == "" {
			return "up to date"
		}
		return "up to date (" + detail + ")"
	}
	if detail == "" {
		return "done"
	}
	return detail
}

func buildSilverSummary(rep ExtractEngineReport, peakMem uint64, noDecomp bool) string {
	var parts []string
	if rep.FuncCount > 0 {
		fnParts := []string{fmt.Sprintf("%d fn", rep.FuncCount)}
		if rep.AsmCount > 0 {
			fnParts = append(fnParts, fmt.Sprintf("%d asm", rep.AsmCount))
		}
		if rep.StringCount > 0 {
			fnParts = append(fnParts, fmt.Sprintf("%d str", rep.StringCount))
		}
		parts = append(parts, strings.Join(fnParts, ", "))
	}
	if noDecomp {
		if !rep.FuncsRan {
			parts = append(parts, "cached")
		} else {
			parts = append(parts, "no-decomp")
		}
	} else {
		okCnt := rep.StatusCounts[extractdb.DecompStatusOK]
		failCnt := rep.StatusCounts[extractdb.DecompStatusFailed]
		toCnt := rep.StatusCounts[extractdb.DecompStatusTimeout]
		msg := fmt.Sprintf("ok=%d", okCnt)
		if failCnt > 0 {
			msg += fmt.Sprintf(" failed=%d", failCnt)
		}
		if toCnt > 0 {
			msg += fmt.Sprintf(" timeout=%d", toCnt)
		}
		parts = append(parts, msg)
		if peakMem > 0 {
			parts = append(parts, "pk "+formatExtractBytes(peakMem))
		}
	}
	if len(parts) == 0 {
		return "done"
	}
	return strings.Join(parts, " | ")
}
