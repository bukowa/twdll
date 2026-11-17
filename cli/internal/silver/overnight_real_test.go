package silver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"twdll/cli/internal/analyze"
	"twdll/cli/internal/bronze"
	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

type overnightConfig struct {
	targets         []workspace.TargetID
	engines         []workspace.DecompilerID
	fastTimeoutSecs int
	retryTimeoutsTo int
	forceBinary     bool
}

type overnightEvent struct {
	Timestamp string         `json:"timestamp"`
	Event     string         `json:"event"`
	Target    string         `json:"target,omitempty"`
	Engine    string         `json:"engine,omitempty"`
	Phase     string         `json:"phase,omitempty"`
	Done      int            `json:"done,omitempty"`
	Total     int            `json:"total,omitempty"`
	Rate      float64        `json:"rate_funcs_per_sec,omitempty"`
	ETASecs   float64        `json:"eta_seconds,omitempty"`
	ElapsedMs int64          `json:"elapsed_ms,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type overnightLogger struct {
	mu      sync.Mutex
	jsonl   *os.File
	encoder *json.Encoder
}

func newOvernightLogger(logDir string) (*overnightLogger, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(logDir, "overnight.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return &overnightLogger{jsonl: f, encoder: enc}, nil
}

func (l *overnightLogger) Close() error {
	if l == nil || l.jsonl == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.jsonl.Close()
}

func (l *overnightLogger) Log(ev overnightEvent, consoleLine string) {
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.encoder != nil {
		_ = l.encoder.Encode(ev)
	}
	if consoleLine != "" {
		fmt.Fprintf(os.Stderr, "[OVERNIGHT %s] %s\n", time.Now().Format("15:04:05"), consoleLine)
	}
}

type enginePassMetrics struct {
	Target          workspace.TargetID
	Engine          workspace.DecompilerID
	Phase           string
	Elapsed         time.Duration
	PeakJobMemBytes uint64
	Total           int
	Written         int
	SkippedOK       int
	OK              int
	Timeout         int
	Failed          int
}

func TestOvernight_FullExtractionAndAudit(t *testing.T) {
	if os.Getenv("TWDLL_RUN_OVERNIGHT") != "1" {
		t.Skip("TWDLL_RUN_OVERNIGHT != 1; skipping full overnight extraction and audit")
	}

	releasePower := analyze.AcquirePowerAssertion()
	defer releasePower()

	ctx := context.Context(t.Context())
	layout, err := workspace.Resolve(workspace.ResolveSpec{})
	if err != nil {
		t.Fatalf("resolve workspace: %v", err)
	}
	if !layout.ConfigExists {
		t.Fatalf("twdll.toml missing at %s", layout.Paths.Config)
	}

	cfg := loadOvernightConfig(t, layout)
	if len(cfg.targets) == 0 {
		t.Fatalf("no runnable target binaries found under %s", layout.Paths.BinsDir)
	}

	logDir := filepath.Join(layout.Paths.ExtractedDir, "overnight_logs")
	logger, err := newOvernightLogger(logDir)
	if err != nil {
		t.Fatalf("open overnight logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	tmpBase := filepath.Join(layout.Paths.ExtractedDir, "tmp")
	cleanStaleTmpDirs(tmpBase)

	dbSnapTargets := discoverDBSnapshotTargets(layout.Paths)
	logger.Log(overnightEvent{
		Event: "phase_start",
		Phase: "suite_init",
		Details: map[string]any{
			"targets":            targetStrings(cfg.targets),
			"engines":            engineStrings(cfg.engines),
			"fast_timeout_secs":  cfg.fastTimeoutSecs,
			"retry_timeout_secs": cfg.retryTimeoutsTo,
			"snapshot_targets":   targetStrings(dbSnapTargets),
		},
	}, fmt.Sprintf("Snapshotting .twdll/db for %v; targets=%v engines=%v fast=%ds retry=%ds",
		dbSnapTargets, cfg.targets, cfg.engines, cfg.fastTimeoutSecs, cfg.retryTimeoutsTo))

	beforeDBSnap := snapshotDBDirs(t, layout.Paths.DBDir, dbSnapTargets)
	defer assertDBDirsUnchanged(t, layout.Paths.DBDir, dbSnapTargets, beforeDBSnap)

	suiteStart := time.Now()
	var results []enginePassMetrics
	retriedTargets := make(map[workspace.TargetID]bool)

	for _, target := range cfg.targets {
		binPath := layout.Paths.BinaryPath(target)
		binDBPath := layout.Paths.BinaryDBPath(target)
		manifestPath := layout.Paths.TargetManifestPath(target)

		if err := ensureTargetBinaryAndSilver(ctx, layout, target, binPath, binDBPath, manifestPath, tmpBase, cfg, logger); err != nil {
			t.Fatalf("prepare binary/silver (%s): %v", target, err)
		}

		for _, eng := range cfg.engines {
			if err := runAndLogDecompPass(ctx, layout, target, eng, binDBPath, tmpBase, "fast_15s", cfg.fastTimeoutSecs, false, logger, &results); err != nil {
				t.Fatalf("RunDecompPass(%s, %s, fast): %v", target, eng, err)
			}
		}

		auditPath := filepath.Join(logDir, fmt.Sprintf("audit_%s.md", target))
		if err := writeTargetAuditReport(ctx, layout, target, cfg.engines, auditPath, logger); err != nil {
			t.Fatalf("writeTargetAuditReport(%s): %v", target, err)
		}
		assertDBDirsUnchanged(t, layout.Paths.DBDir, dbSnapTargets, beforeDBSnap)
	}

	// Phase 5: Heavy-Later Timeout Retry Pass on attila_32 and attila_64 (RetryTimeouts: true, TimeoutSecs: 60).
	targetSet := toTargetSet(cfg.targets)
	retryCandidates := []workspace.TargetID{workspace.TargetAttila32, workspace.TargetAttila64}
	for _, target := range retryCandidates {
		if !targetSet[target] {
			continue
		}
		binDBPath := layout.Paths.BinaryDBPath(target)
		didRetry := false
		for _, eng := range cfg.engines {
			if err := runAndLogDecompPass(ctx, layout, target, eng, binDBPath, tmpBase, "retry_60s", cfg.retryTimeoutsTo, true, logger, &results); err != nil {
				t.Fatalf("RunDecompPass(%s, %s, retry): %v", target, eng, err)
			}
			didRetry = true
		}
		if didRetry {
			retriedTargets[target] = true
			auditPath := filepath.Join(logDir, fmt.Sprintf("audit_%s.md", target))
			if err := writeTargetAuditReport(ctx, layout, target, cfg.engines, auditPath, logger); err != nil {
				t.Fatalf("writeTargetAuditReport(%s post-retry): %v", target, err)
			}
			assertDBDirsUnchanged(t, layout.Paths.DBDir, dbSnapTargets, beforeDBSnap)
		}
	}

	summaryPath := filepath.Join(logDir, "overnight_summary.md")
	if err := writeOvernightSummaryReport(ctx, layout, cfg, results, time.Since(suiteStart), summaryPath, logger); err != nil {
		t.Fatalf("writeOvernightSummaryReport: %v", err)
	}
	cleanStaleTmpDirs(tmpBase)
}

func loadOvernightConfig(t *testing.T, layout workspace.Layout) overnightConfig {
	t.Helper()
	defaultTargets := []workspace.TargetID{
		workspace.TargetAttila32,
		workspace.TargetAttila64,
		workspace.TargetRome2_32,
		workspace.TargetToB32,
	}
	var targets []workspace.TargetID
	if raw := strings.TrimSpace(os.Getenv("TWDLL_OVERNIGHT_TARGETS")); raw != "" {
		for _, tok := range strings.Split(raw, ",") {
			tid := workspace.TargetID(strings.TrimSpace(tok))
			if !tid.IsValid() {
				t.Fatalf("invalid target in TWDLL_OVERNIGHT_TARGETS: %q", tok)
			}
			if _, err := os.Stat(layout.Paths.BinaryPath(tid)); err == nil {
				targets = append(targets, tid)
			} else {
				fmt.Fprintf(os.Stderr, "[OVERNIGHT] Skipping %s (binary missing at %s)\n", tid, layout.Paths.BinaryPath(tid))
			}
		}
	} else {
		for _, tid := range defaultTargets {
			if _, err := os.Stat(layout.Paths.BinaryPath(tid)); err == nil {
				targets = append(targets, tid)
			}
		}
	}

	engines := []workspace.DecompilerID{workspace.DecompilerGhidra, workspace.DecompilerIDA}
	if raw := strings.TrimSpace(os.Getenv("TWDLL_OVERNIGHT_ENGINES")); raw != "" {
		engines = nil
		for _, tok := range strings.Split(raw, ",") {
			eng := workspace.DecompilerID(strings.TrimSpace(tok))
			if !eng.IsValid() {
				t.Fatalf("invalid engine in TWDLL_OVERNIGHT_ENGINES: %q", tok)
			}
			engines = append(engines, eng)
		}
	}

	fastTimeout := DefaultFastTimeoutSecs
	if raw := strings.TrimSpace(os.Getenv("TWDLL_OVERNIGHT_FAST_TIMEOUT_SECS")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			t.Fatalf("invalid TWDLL_OVERNIGHT_FAST_TIMEOUT_SECS: %q", raw)
		}
		fastTimeout = v
	}

	retryTimeout := 60
	if raw := strings.TrimSpace(os.Getenv("TWDLL_OVERNIGHT_RETRY_TIMEOUT_SECS")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			t.Fatalf("invalid TWDLL_OVERNIGHT_RETRY_TIMEOUT_SECS: %q", raw)
		}
		retryTimeout = v
	}

	return overnightConfig{
		targets:         targets,
		engines:         engines,
		fastTimeoutSecs: fastTimeout,
		retryTimeoutsTo: retryTimeout,
		forceBinary:     os.Getenv("TWDLL_OVERNIGHT_FORCE_BINARY") == "1",
	}
}

func discoverDBSnapshotTargets(paths workspace.Paths) []workspace.TargetID {
	var out []workspace.TargetID
	for _, tid := range workspace.AllTargets() {
		_, errGH := os.Stat(paths.DatabaseDir(workspace.DecompilerGhidra, tid))
		_, errIDA := os.Stat(paths.DatabaseDir(workspace.DecompilerIDA, tid))
		if errGH == nil && errIDA == nil {
			out = append(out, tid)
		}
	}
	return out
}

func cleanStaleTmpDirs(tmpBase string) {
	entries, err := os.ReadDir(tmpBase)
	if err != nil {
		return
	}
	for _, ent := range entries {
		if ent.IsDir() && strings.HasPrefix(ent.Name(), "twdll_") {
			_ = os.RemoveAll(filepath.Join(tmpBase, ent.Name()))
		}
	}
}

func ensureTargetBinaryAndSilver(
	ctx context.Context,
	layout workspace.Layout,
	target workspace.TargetID,
	binPath, binDBPath, manifestPath, tmpBase string,
	cfg overnightConfig,
	logger *overnightLogger,
) error {
	needBronze := cfg.forceBinary
	if !needBronze {
		if _, err := os.Stat(binDBPath); err != nil {
			needBronze = true
		}
	}

	if needBronze {
		logger.Log(overnightEvent{
			Event:  "phase_start",
			Target: string(target),
			Phase:  "bronze",
		}, fmt.Sprintf("%s: starting Bronze extraction -> %s", target, filepath.ToSlash(binDBPath)))

		bRes, err := bronze.ExtractTarget(ctx, bronze.Options{
			Target:       target,
			BinaryPath:   binPath,
			BinaryDBPath: binDBPath,
			ManifestPath: manifestPath,
		})
		if err != nil {
			return err
		}
		logger.Log(overnightEvent{
			Event:     "phase_done",
			Target:    string(target),
			Phase:     "bronze",
			ElapsedMs: bRes.Elapsed.Milliseconds(),
			Details:   map[string]any{"functions": bRes.Counts["functions"]},
		}, fmt.Sprintf("%s: Bronze finished (%d functions in %s)", target, bRes.Counts["functions"], bRes.Elapsed.Round(time.Millisecond)))
	}

	for _, eng := range cfg.engines {
		merged, err := binaryHasSilverDiscovery(ctx, binDBPath, eng)
		if err != nil {
			return err
		}
		if merged && !needBronze {
			logger.Log(overnightEvent{
				Event:  "phase_done",
				Target: string(target),
				Engine: string(eng),
				Phase:  "silver_skip",
			}, fmt.Sprintf("%s: Silver(%s) already present in binary.sqlite, skipping", target, eng))
			continue
		}

		logger.Log(overnightEvent{
			Event:  "phase_start",
			Target: string(target),
			Engine: string(eng),
			Phase:  "silver",
		}, fmt.Sprintf("%s: running Silver(%s) topology merge...", target, eng))

		sRes, err := ExtractSilverTopology(ctx, SilverOptions{
			Paths:        layout.Paths,
			Target:       target,
			Engine:       eng,
			IDADir:       layout.Config.IDADir,
			BinaryPath:   binPath,
			BinaryDBPath: binDBPath,
			ManifestPath: manifestPath,
			TmpBaseDir:   tmpBase,
		})
		if err != nil {
			return err
		}
		logger.Log(overnightEvent{
			Event:     "phase_done",
			Target:    string(target),
			Engine:    string(eng),
			Phase:     "silver",
			ElapsedMs: sRes.Elapsed.Milliseconds(),
			Details: map[string]any{
				"total_functions": sRes.TotalFunctions,
				"updated":         sRes.UpdatedExisting,
				"inserted":        sRes.InsertedNew,
			},
		}, fmt.Sprintf("%s: Silver(%s) finished (total=%d updated=%d inserted=%d in %s)",
			target, eng, sRes.TotalFunctions, sRes.UpdatedExisting, sRes.InsertedNew, sRes.Elapsed.Round(time.Millisecond)))
	}
	return nil
}

func binaryHasSilverDiscovery(ctx context.Context, binDBPath string, eng workspace.DecompilerID) (bool, error) {
	ro, err := extractdb.OpenReadOnly(binDBPath)
	if err != nil {
		return false, err
	}
	defer ro.Close()

	bit := extractdb.DiscoveryGhidra
	if eng == workspace.DecompilerIDA {
		bit = extractdb.DiscoveryIDA
	}
	var cnt int
	err = ro.QueryRowContext(ctx, `SELECT COUNT(*) FROM functions WHERE (discovery_mask & ?) != 0`, int64(bit)).Scan(&cnt)
	if err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func runAndLogDecompPass(
	ctx context.Context,
	layout workspace.Layout,
	target workspace.TargetID,
	eng workspace.DecompilerID,
	binDBPath, tmpBase, phase string,
	timeoutSecs int,
	retryTimeouts bool,
	logger *overnightLogger,
	results *[]enginePassMetrics,
) error {
	decompDBPath := layout.Paths.DecompDBPath(target, eng)
	workers, maxMem := resolveOvernightDecompResources(target, eng)

	logger.Log(overnightEvent{
		Event:  "phase_start",
		Target: string(target),
		Engine: string(eng),
		Phase:  phase,
		Details: map[string]any{
			"workers":        workers,
			"max_mem":        maxMem,
			"timeout_secs":   timeoutSecs,
			"retry_timeouts": retryTimeouts,
		},
	}, fmt.Sprintf("%s × %s [%s]: starting (workers=%d maxMem=%s timeout=%ds retryTimeouts=%v)",
		target, eng, phase, workers, maxMem, timeoutSecs, retryTimeouts))

	passStart := time.Now()
	var progMu sync.Mutex
	lastLogTime := time.Now()
	lastLogDone := 0

	res, err := RunDecompPass(ctx, DecompOptions{
		Paths:         layout.Paths,
		Target:        target,
		Engine:        eng,
		IDADir:        layout.Config.IDADir,
		BinaryDBPath:  binDBPath,
		DecompDBPath:  decompDBPath,
		TimeoutSecs:   timeoutSecs,
		NoTimeout:     timeoutSecs == 0,
		Workers:       workers,
		MaxMem:        maxMem,
		RetryTimeouts: retryTimeouts,
		TmpBaseDir:    tmpBase,
		OnProgress: func(done, total int) {
			progMu.Lock()
			now := time.Now()
			shouldLog := done == total || done-lastLogDone >= 2500 || now.Sub(lastLogTime) >= 30*time.Second
			if !shouldLog {
				progMu.Unlock()
				return
			}
			lastLogDone = done
			lastLogTime = now
			progMu.Unlock()

			elapsed := now.Sub(passStart).Seconds()
			rate := 0.0
			etaSecs := 0.0
			if elapsed > 0.1 {
				rate = float64(done) / elapsed
				if rate > 0 && total > done {
					etaSecs = float64(total-done) / rate
				}
			}
			etaStr := (time.Duration(etaSecs) * time.Second).Round(time.Second).String()
			logger.Log(overnightEvent{
				Event:   "progress",
				Target:  string(target),
				Engine:  string(eng),
				Phase:   phase,
				Done:    done,
				Total:   total,
				Rate:    rate,
				ETASecs: etaSecs,
			}, fmt.Sprintf("%s × %s [%s]: %d/%d (%.1f%%) | %.2f funcs/s | ETA %s",
				target, eng, phase, done, total, 100.0*float64(done)/float64(max(1, total)), rate, etaStr))
		},
	})
	if err != nil {
		return err
	}

	okN := res.StatusCounts[extractdb.DecompStatusOK]
	toN := res.StatusCounts[extractdb.DecompStatusTimeout]
	failN := res.StatusCounts[extractdb.DecompStatusFailed]

	logger.Log(overnightEvent{
		Event:     "phase_done",
		Target:    string(target),
		Engine:    string(eng),
		Phase:     phase,
		ElapsedMs: res.Elapsed.Milliseconds(),
		Details: map[string]any{
			"total":          res.Resume.Total,
			"written":        res.Written,
			"skipped_ok":     res.Resume.SkippedOK,
			"ok":             okN,
			"timeout":        toN,
			"failed":         failN,
			"peak_mem_bytes": res.PeakJobMemBytes,
		},
	}, fmt.Sprintf("%s × %s [%s] DONE in %s: ok=%d timeout=%d failed=%d (written=%d skippedOK=%d peakRAM=%.2f GiB)",
		target, eng, phase, res.Elapsed.Round(time.Second), okN, toN, failN, res.Written, res.Resume.SkippedOK,
		float64(res.PeakJobMemBytes)/(1024*1024*1024)))

	*results = append(*results, enginePassMetrics{
		Target:          target,
		Engine:          eng,
		Phase:           phase,
		Elapsed:         res.Elapsed,
		PeakJobMemBytes: res.PeakJobMemBytes,
		Total:           res.Resume.Total,
		Written:         res.Written,
		SkippedOK:       res.Resume.SkippedOK,
		OK:              okN,
		Timeout:         toN,
		Failed:          failN,
	})
	return nil
}

func resolveOvernightDecompResources(target workspace.TargetID, eng workspace.DecompilerID) (workers int, maxMem string) {
	if eng == workspace.DecompilerGhidra {
		if target.Is64Bit() {
			return 10, "24G"
		}
		return 10, "12G"
	}
	if target.Is64Bit() {
		return 4, ""
	}
	return 6, ""
}

type auditBucketAgg struct {
	bucket    string
	total     int
	ok        int
	timeout   int
	failed    int
	sumMs     int64
	elapsedMs []int64
	withEA    int
	withVars  int
}

type auditDefectStats struct {
	okTotal          int
	defects          map[string]int
	ge64MissingEA    int
	ge64MissingVars  int
	crossOK_OK       int
	crossIDA_GHTO    int
	crossIDA_GHFail  int
	crossGH_IDATO    int
	crossGH_IDAFail  int
	crossBothTO      int
	crossBothFail    int
	crossOther       int
}

var defectTokens = []string{
	"in_ECX",
	"unaff_EBX",
	"unaff_ESI",
	"unaff_EDI",
	"unaff_EBP",
	"in_stack_",
	"BADSPACEBASE",
	"halt_baddata",
	"JUMPOUT",
	"WARNING: Removing unreachable block",
}

func writeTargetAuditReport(
	ctx context.Context,
	layout workspace.Layout,
	target workspace.TargetID,
	engines []workspace.DecompilerID,
	outPath string,
	logger *overnightLogger,
) error {
	binDBPath := layout.Paths.BinaryDBPath(target)
	binDB, err := extractdb.OpenReadOnly(binDBPath)
	if err != nil {
		return fmt.Errorf("open binary db %s for audit: %w", binDBPath, err)
	}
	defer binDB.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Diagnostic Extraction Audit: `%s`\n\n", target))
	sb.WriteString(fmt.Sprintf("Generated: `%s`\n\n", time.Now().UTC().Format(time.RFC3339)))

	if err := appendTopologyAuditSection(ctx, &sb, binDB, target); err != nil {
		return err
	}

	anchorSet, funcMeta, err := loadFunctionAuditMeta(ctx, binDB)
	if err != nil {
		return err
	}

	engineStatuses := make(map[workspace.DecompilerID]map[uint64]string)
	for _, eng := range engines {
		decompPath := layout.Paths.DecompDBPath(target, eng)
		if _, statErr := os.Stat(decompPath); statErr != nil {
			continue
		}
		stMap, err := appendEngineDecompAuditSection(ctx, &sb, decompPath, eng, anchorSet)
		if err != nil {
			return fmt.Errorf("audit %s %s: %w", target, eng, err)
		}
		engineStatuses[eng] = stMap
	}

	if len(engineStatuses[workspace.DecompilerIDA]) > 0 && len(engineStatuses[workspace.DecompilerGhidra]) > 0 {
		appendCrossEngineSection(&sb, engineStatuses[workspace.DecompilerIDA], engineStatuses[workspace.DecompilerGhidra])
	}

	for _, eng := range engines {
		decompPath := layout.Paths.DecompDBPath(target, eng)
		if _, statErr := os.Stat(decompPath); statErr != nil {
			continue
		}
		var otherStatus map[uint64]string
		if eng == workspace.DecompilerIDA {
			otherStatus = engineStatuses[workspace.DecompilerGhidra]
		} else {
			otherStatus = engineStatuses[workspace.DecompilerIDA]
		}
		if err := appendFailureAndTimeoutCatalogs(ctx, &sb, decompPath, eng, funcMeta, otherStatus); err != nil {
			return err
		}
	}

	if err := workspace.WriteFileAtomic(outPath, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	logger.Log(overnightEvent{
		Event:  "audit_summary",
		Target: string(target),
		Details: map[string]any{
			"report_path": filepath.ToSlash(outPath),
		},
	}, fmt.Sprintf("%s: wrote diagnostic audit report -> %s", target, filepath.ToSlash(outPath)))
	return nil
}

type funcMetaAudit struct {
	name          string
	discoveryMask uint32
}

func loadFunctionAuditMeta(ctx context.Context, binDB *sql.DB) (map[uint64]bool, map[uint64]funcMetaAudit, error) {
	anchors := make(map[uint64]bool)
	for _, q := range []string{`SELECT DISTINCT func_ea FROM lua_regs`, `SELECT DISTINCT target_ea FROM vtables`} {
		rows, err := binDB.QueryContext(ctx, q)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			var ea int64
			if scanErr := rows.Scan(&ea); scanErr != nil {
				_ = rows.Close()
				return nil, nil, scanErr
			}
			anchors[uint64(ea)] = true
		}
		if err := rows.Close(); err != nil {
			return nil, nil, err
		}
	}

	rows, err := binDB.QueryContext(ctx, `SELECT ea, name, discovery_mask FROM functions`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	meta := make(map[uint64]funcMetaAudit)
	for rows.Next() {
		var ea, mask int64
		var name string
		if err := rows.Scan(&ea, &name, &mask); err != nil {
			return nil, nil, err
		}
		meta[uint64(ea)] = funcMetaAudit{name: name, discoveryMask: uint32(mask)}
	}
	return anchors, meta, rows.Err()
}

func appendTopologyAuditSection(ctx context.Context, sb *strings.Builder, binDB *sql.DB, target workspace.TargetID) error {
	sb.WriteString("## 1. Topology & Discovery Matrix (`binary.sqlite`)\n\n")

	var total, bronzeAny, bronzeOnly, ghAny, ghOnly, idaAny, idaOnly, allThree, ghIdaOnly int
	const bronzeBits = int64(extractdb.DiscoveryDWARF | extractdb.DiscoverySymtab | extractdb.DiscoveryExport |
		extractdb.DiscoveryVtable | extractdb.DiscoveryLuaReg | extractdb.DiscoveryCallTarget |
		extractdb.DiscoveryDataPtr | extractdb.DiscoveryPrologue)
	err := binDB.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN (discovery_mask & ?) != 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (discovery_mask & ?) != 0 AND (discovery_mask & ?) = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (discovery_mask & ?) != 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN discovery_mask = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (discovery_mask & ?) != 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN discovery_mask = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (discovery_mask & ?) != 0 AND (discovery_mask & ?) != 0 AND (discovery_mask & ?) != 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (discovery_mask & ?) = 0 AND (discovery_mask & ?) != 0 AND (discovery_mask & ?) != 0 THEN 1 ELSE 0 END), 0)
		FROM functions
	`,
		bronzeBits,
		bronzeBits, int64(extractdb.DiscoveryGhidra|extractdb.DiscoveryIDA),
		int64(extractdb.DiscoveryGhidra),
		int64(extractdb.DiscoveryGhidra),
		int64(extractdb.DiscoveryIDA),
		int64(extractdb.DiscoveryIDA),
		bronzeBits, int64(extractdb.DiscoveryGhidra), int64(extractdb.DiscoveryIDA),
		bronzeBits, int64(extractdb.DiscoveryGhidra), int64(extractdb.DiscoveryIDA),
	).Scan(&total, &bronzeAny, &bronzeOnly, &ghAny, &ghOnly, &idaAny, &idaOnly, &allThree, &ghIdaOnly)
	if err != nil {
		return err
	}

	sb.WriteString("| Metric | Count |\n|---|---:|\n")
	sb.WriteString(fmt.Sprintf("| Total `functions` | %d |\n", total))
	sb.WriteString(fmt.Sprintf("| Bronze any bit | %d |\n", bronzeAny))
	sb.WriteString(fmt.Sprintf("| Bronze-only (neither Silver) | %d |\n", bronzeOnly))
	sb.WriteString(fmt.Sprintf("| Silver Ghidra bit set | %d |\n", ghAny))
	sb.WriteString(fmt.Sprintf("| Ghidra-only | %d |\n", ghOnly))
	sb.WriteString(fmt.Sprintf("| Silver IDA bit set | %d |\n", idaAny))
	sb.WriteString(fmt.Sprintf("| IDA-only | %d |\n", idaOnly))
	sb.WriteString(fmt.Sprintf("| Ghidra+IDA (no Bronze) | %d |\n", ghIdaOnly))
	sb.WriteString(fmt.Sprintf("| Bronze+Ghidra+IDA | %d |\n\n", allThree))

	var vtRows, vtDistinct, vtMissing, luaRows, luaDistinct, luaMissing int
	_ = binDB.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT target_ea) FROM vtables`).Scan(&vtRows, &vtDistinct)
	_ = binDB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT v.target_ea) FROM vtables v LEFT JOIN functions f ON v.target_ea = f.ea WHERE f.ea IS NULL`).Scan(&vtMissing)
	_ = binDB.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT func_ea) FROM lua_regs`).Scan(&luaRows, &luaDistinct)
	_ = binDB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT l.func_ea) FROM lua_regs l LEFT JOIN functions f ON l.func_ea = f.ea WHERE f.ea IS NULL`).Scan(&luaMissing)

	sb.WriteString(fmt.Sprintf("- **`.rdata` VTables:** rows=`%d`, distinct targets=`%d`, missing from `functions`=`%d`\n", vtRows, vtDistinct, vtMissing))
	sb.WriteString(fmt.Sprintf("- **`lua_regs`:** rows=`%d`, distinct targets=`%d`, missing from `functions`=`%d`\n", luaRows, luaDistinct, luaMissing))

	for _, tbl := range []string{"types", "type_members", "enum_values", "func_vars", "inlines", "source_files", "call_sites", "data_xrefs", "strings", "string_xrefs"} {
		var c int
		_ = binDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tbl).Scan(&c)
		if target.Is64Bit() || c > 0 {
			sb.WriteString(fmt.Sprintf("- **Table `%s`:** `%d` rows\n", tbl, c))
		}
	}
	sb.WriteString("\n### Calling Convention Breakdown\n\n| Calling Conv | Source | Count |\n|---|---|---:|\n")
	rows, err := binDB.QueryContext(ctx, `SELECT calling_conv, cc_source, COUNT(*) FROM functions GROUP BY calling_conv, cc_source ORDER BY COUNT(*) DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cc, src string
		var cnt int
		if err := rows.Scan(&cc, &src, &cnt); err != nil {
			return err
		}
		if cc == "" {
			cc = "(empty)"
		}
		if src == "" {
			src = "(empty)"
		}
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | %d |\n", cc, src, cnt))
	}
	sb.WriteString("\n")
	return rows.Err()
}

func appendEngineDecompAuditSection(
	ctx context.Context,
	sb *strings.Builder,
	decompPath string,
	eng workspace.DecompilerID,
	anchors map[uint64]bool,
) (map[uint64]string, error) {
	db, err := extractdb.OpenReadOnly(decompPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT func_ea, func_size, status, elapsed_ms, line_ea_json, decompiler_vars_json, raw_c
		FROM decomp_funcs
		WHERE engine = ?
	`, string(eng))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := make(map[string]*auditBucketAgg)
	allAgg := &auditBucketAgg{bucket: "ALL"}
	defects := auditDefectStats{defects: make(map[string]int)}
	statusByEA := make(map[uint64]string)

	for rows.Next() {
		var ea, sz, elMs int64
		var status, lineEA, varsJSON, rawC string
		if err := rows.Scan(&ea, &sz, &status, &elMs, &lineEA, &varsJSON, &rawC); err != nil {
			return nil, err
		}
		uEA := uint64(ea)
		statusByEA[uEA] = status

		bkt := sizeBucket(uint64(max(0, int(sz))))
		if anchors[uEA] {
			bkt = BucketAnchors
		}
		agg := buckets[bkt]
		if agg == nil {
			agg = &auditBucketAgg{bucket: bkt}
			buckets[bkt] = agg
		}

		for _, targetAgg := range []*auditBucketAgg{agg, allAgg} {
			targetAgg.total++
			targetAgg.sumMs += elMs
			targetAgg.elapsedMs = append(targetAgg.elapsedMs, elMs)
			switch status {
			case extractdb.DecompStatusOK:
				targetAgg.ok++
				if lineEA != "" && lineEA != "[]" {
					targetAgg.withEA++
				}
				if varsJSON != "" && varsJSON != "[]" {
					targetAgg.withVars++
				}
			case extractdb.DecompStatusTimeout:
				targetAgg.timeout++
			default:
				targetAgg.failed++
			}
		}

		if status == extractdb.DecompStatusOK {
			defects.okTotal++
			for _, tok := range defectTokens {
				if strings.Contains(rawC, tok) {
					defects.defects[tok]++
				}
			}
			if sz >= 64 {
				if lineEA == "" || lineEA == "[]" {
					defects.ge64MissingEA++
				}
				if varsJSON == "" || varsJSON == "[]" {
					defects.ge64MissingVars++
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sb.WriteString(fmt.Sprintf("## 2. Engine `%s` Decompilation Metrics\n\n", eng))
	sb.WriteString("| Bucket | Total | OK | Timeout | Failed | Avg ms | P95 ms | LineEA% | Vars% |\n")
	sb.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")

	order := []string{BucketAnchors, BucketTiny, BucketSmall, BucketLarge, BucketHuge, BucketGiant}
	for _, bkt := range order {
		if agg := buckets[bkt]; agg != nil {
			writeAuditBucketRow(sb, agg)
		}
	}
	writeAuditBucketRow(sb, allAgg)

	sb.WriteString(fmt.Sprintf("\n### Zero-Optimism Quality Defect Counters (`%s` on `ok=%d`)\n\n", eng, defects.okTotal))
	sb.WriteString("| Defect / Missing Indicator | Count | % of OK |\n|---|---:|---:|\n")
	for _, tok := range defectTokens {
		cnt := defects.defects[tok]
		sb.WriteString(fmt.Sprintf("| `%s` | %d | %.2f%% |\n", tok, cnt, pct(cnt, defects.okTotal)))
	}
	sb.WriteString(fmt.Sprintf("| `func_size >= 64` empty `line_ea_json` | %d | %.2f%% |\n", defects.ge64MissingEA, pct(defects.ge64MissingEA, defects.okTotal)))
	sb.WriteString(fmt.Sprintf("| `func_size >= 64` empty `decompiler_vars_json` | %d | %.2f%% |\n\n", defects.ge64MissingVars, pct(defects.ge64MissingVars, defects.okTotal)))

	return statusByEA, nil
}

func writeAuditBucketRow(sb *strings.Builder, agg *auditBucketAgg) {
	avgMs := 0.0
	if agg.total > 0 {
		avgMs = float64(agg.sumMs) / float64(agg.total)
	}
	p95 := percentileMs(agg.elapsedMs, 0.95)
	sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %.0f | %d | %.1f%% | %.1f%% |\n",
		agg.bucket, agg.total, agg.ok, agg.timeout, agg.failed, avgMs, p95, pct(agg.withEA, agg.ok), pct(agg.withVars, agg.ok)))
}

func percentileMs(vals []int64, q float64) int64 {
	if len(vals) == 0 {
		return 0
	}
	cp := make([]int64, len(vals))
	copy(cp, vals)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := int(q * float64(len(cp)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

func appendCrossEngineSection(sb *strings.Builder, idaSt, ghSt map[uint64]string) {
	allEA := make(map[uint64]struct{}, len(idaSt))
	for ea := range idaSt {
		allEA[ea] = struct{}{}
	}
	for ea := range ghSt {
		allEA[ea] = struct{}{}
	}
	var s auditDefectStats
	for ea := range allEA {
		si := idaSt[ea]
		sg := ghSt[ea]
		switch {
		case si == extractdb.DecompStatusOK && sg == extractdb.DecompStatusOK:
			s.crossOK_OK++
		case si == extractdb.DecompStatusOK && sg == extractdb.DecompStatusTimeout:
			s.crossIDA_GHTO++
		case si == extractdb.DecompStatusOK && sg == extractdb.DecompStatusFailed:
			s.crossIDA_GHFail++
		case sg == extractdb.DecompStatusOK && si == extractdb.DecompStatusTimeout:
			s.crossGH_IDATO++
		case sg == extractdb.DecompStatusOK && si == extractdb.DecompStatusFailed:
			s.crossGH_IDAFail++
		case si == extractdb.DecompStatusTimeout && sg == extractdb.DecompStatusTimeout:
			s.crossBothTO++
		case si == extractdb.DecompStatusFailed && sg == extractdb.DecompStatusFailed:
			s.crossBothFail++
		default:
			s.crossOther++
		}
	}

	sb.WriteString("## 3. Cross-Engine Discrepancy Summary (`ida` vs `ghidra`)\n\n")
	sb.WriteString("| Pairing | Count |\n|---|---:|\n")
	sb.WriteString(fmt.Sprintf("| `IDA=ok, Ghidra=ok` | %d |\n", s.crossOK_OK))
	sb.WriteString(fmt.Sprintf("| `IDA=ok, Ghidra=timeout` | %d |\n", s.crossIDA_GHTO))
	sb.WriteString(fmt.Sprintf("| `IDA=ok, Ghidra=failed` | %d |\n", s.crossIDA_GHFail))
	sb.WriteString(fmt.Sprintf("| `Ghidra=ok, IDA=timeout` | %d |\n", s.crossGH_IDATO))
	sb.WriteString(fmt.Sprintf("| `Ghidra=ok, IDA=failed` | %d |\n", s.crossGH_IDAFail))
	sb.WriteString(fmt.Sprintf("| `Both=timeout` | %d |\n", s.crossBothTO))
	sb.WriteString(fmt.Sprintf("| `Both=failed` | %d |\n", s.crossBothFail))
	sb.WriteString(fmt.Sprintf("| `Other / Missing on one side` | %d |\n\n", s.crossOther))
}

func appendFailureAndTimeoutCatalogs(
	ctx context.Context,
	sb *strings.Builder,
	decompPath string,
	eng workspace.DecompilerID,
	funcMeta map[uint64]funcMetaAudit,
	otherStatus map[uint64]string,
) error {
	db, err := extractdb.OpenReadOnly(decompPath)
	if err != nil {
		return err
	}
	defer db.Close()

	var totalFailed, totalTimeout int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM decomp_funcs WHERE engine = ? AND status = 'failed'`, string(eng)).Scan(&totalFailed)
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM decomp_funcs WHERE engine = ? AND status = 'timeout'`, string(eng)).Scan(&totalTimeout)

	sb.WriteString(fmt.Sprintf("## 4. Actionable `%s` Failures (`%d` total, showing up to 100)\n\n", eng, totalFailed))
	fRows, err := db.QueryContext(ctx, `
		SELECT func_ea, func_size, func_name, error_text
		FROM decomp_funcs
		WHERE engine = ? AND status = 'failed'
		ORDER BY func_size DESC, func_ea ASC
		LIMIT 100
	`, string(eng))
	if err != nil {
		return err
	}
	sb.WriteString("| EA | Size | Name | Discovery Mask | Error |\n|---|---:|---|---|---|\n")
	for fRows.Next() {
		var ea, sz int64
		var name, errTxt string
		if err := fRows.Scan(&ea, &sz, &name, &errTxt); err != nil {
			_ = fRows.Close()
			return err
		}
		fm := funcMeta[uint64(ea)]
		if name == "" {
			name = fm.name
		}
		sb.WriteString(fmt.Sprintf("| `0x%x` | %d | `%s` | `0x%x` | %s |\n",
			uint64(ea), sz, escMD(name), fm.discoveryMask, escMD(truncateStr(errTxt, 120))))
	}
	if err := fRows.Close(); err != nil {
		return err
	}

	sb.WriteString(fmt.Sprintf("\n### Top `%s` Timeouts (`%d` total, showing up to 50)\n\n", eng, totalTimeout))
	tRows, err := db.QueryContext(ctx, `
		SELECT func_ea, func_size, func_name, elapsed_ms
		FROM decomp_funcs
		WHERE engine = ? AND status = 'timeout'
		ORDER BY func_size DESC, elapsed_ms DESC, func_ea ASC
		LIMIT 50
	`, string(eng))
	if err != nil {
		return err
	}
	defer tRows.Close()

	sb.WriteString("| EA | Size | Name | Elapsed ms | Other Engine |\n|---|---:|---|---:|---|\n")
	for tRows.Next() {
		var ea, sz, elMs int64
		var name string
		if err := tRows.Scan(&ea, &sz, &name, &elMs); err != nil {
			return err
		}
		other := "n/a"
		if st, ok := otherStatus[uint64(ea)]; ok {
			other = st
		}
		sb.WriteString(fmt.Sprintf("| `0x%x` | %d | `%s` | %d | `%s` |\n",
			uint64(ea), sz, escMD(name), elMs, other))
	}
	sb.WriteString("\n")
	return tRows.Err()
}

func writeOvernightSummaryReport(
	ctx context.Context,
	layout workspace.Layout,
	cfg overnightConfig,
	results []enginePassMetrics,
	wall time.Duration,
	outPath string,
	logger *overnightLogger,
) error {
	var sb strings.Builder
	sb.WriteString("# Overnight Full-Binary Extraction & Audit Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Completed:** `%s`\n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- **Total Wall Time:** `%s`\n", wall.Round(time.Second)))
	sb.WriteString(fmt.Sprintf("- **Targets:** `%v`\n", targetStrings(cfg.targets)))
	sb.WriteString(fmt.Sprintf("- **Engines:** `%v`\n", engineStrings(cfg.engines)))
	sb.WriteString(fmt.Sprintf("- **Read-only `.twdll/db/` check:** `PASSED (0 bytes modified)`\n\n"))

	sb.WriteString("## Pass Breakdown\n\n")
	sb.WriteString("| Target | Engine | Phase | Elapsed | Total | OK | Timeout | Failed | Written | Skipped OK | Peak RAM |\n")
	sb.WriteString("|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | %s | %d | %d | %d | %d | %d | %d | %.2f GiB |\n",
			r.Target, r.Engine, r.Phase, r.Elapsed.Round(time.Second),
			r.Total, r.OK, r.Timeout, r.Failed, r.Written, r.SkippedOK,
			float64(r.PeakJobMemBytes)/(1024*1024*1024)))
	}

	sb.WriteString("\n## Artifacts Generated\n\n")
	for _, target := range cfg.targets {
		sb.WriteString(fmt.Sprintf("- **`%s`:** `binary.sqlite`, audit report `overnight_logs/audit_%s.md`", target, target))
		for _, eng := range cfg.engines {
			p := layout.Paths.DecompDBPath(target, eng)
			if st, err := os.Stat(p); err == nil {
				sb.WriteString(fmt.Sprintf(", `decomp_%s.sqlite` (%.1f MiB)", eng, float64(st.Size())/(1024*1024)))
			}
		}
		sb.WriteString("\n")
	}
	_ = ctx

	if err := workspace.WriteFileAtomic(outPath, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	logger.Log(overnightEvent{
		Event:     "phase_done",
		Phase:     "suite_complete",
		ElapsedMs: wall.Milliseconds(),
		Details:   map[string]any{"summary_path": filepath.ToSlash(outPath)},
	}, fmt.Sprintf("ALL DONE in %s! Summary -> %s", wall.Round(time.Second), filepath.ToSlash(outPath)))
	return nil
}

func targetStrings(in []workspace.TargetID) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func engineStrings(in []workspace.DecompilerID) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func toTargetSet(in []workspace.TargetID) map[workspace.TargetID]bool {
	m := make(map[workspace.TargetID]bool, len(in))
	for _, v := range in {
		m[v] = true
	}
	return m
}

func pct(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return 100.0 * float64(part) / float64(whole)
}

func escMD(s string) string {
	return strings.NewReplacer("|", `\|`, "\r", " ", "\n", " ", "`", "'").Replace(strings.TrimSpace(s))
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
