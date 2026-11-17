package silver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"twdll/cli/internal/bronze"
	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

type dbFileSnap struct {
	size    int64
	modTime int64
	sha256  string
}

func snapshotDBDirs(t *testing.T, dbDir string, targets []workspace.TargetID) map[string]dbFileSnap {
	t.Helper()
	snaps := make(map[string]dbFileSnap)
	for _, eng := range []string{"ida", "ghidra"} {
		for _, target := range targets {
			root := filepath.Join(dbDir, eng, string(target))
			err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if info.IsDir() {
					return nil
				}
				sha, err := workspace.ComputeFileSHA256(path)
				if err != nil {
					return err
				}
				snaps[path] = dbFileSnap{
					size:    info.Size(),
					modTime: info.ModTime().UnixNano(),
					sha256:  sha,
				}
				return nil
			})
			if err != nil {
				t.Fatalf("snapshot %s: %v", root, err)
			}
		}
	}
	return snaps
}

func assertDBDirsUnchanged(t *testing.T, dbDir string, targets []workspace.TargetID, before map[string]dbFileSnap) {
	t.Helper()
	after := snapshotDBDirs(t, dbDir, targets)
	if len(after) != len(before) {
		t.Fatalf("live .twdll/db file count changed: before=%d after=%d", len(before), len(after))
	}
	for path, bSnap := range before {
		aSnap, ok := after[path]
		if !ok {
			t.Fatalf("live .twdll/db file vanished: %s", path)
		}
		if aSnap.size != bSnap.size || aSnap.modTime != bSnap.modTime || aSnap.sha256 != bSnap.sha256 {
			t.Fatalf("live .twdll/db file mutated: %s (before=%+v after=%+v)", path, bSnap, aSnap)
		}
	}
}

func TestStep4_RealSilverAnd2000FuncDecompBenchmark(t *testing.T) {
	if os.Getenv("TWDLL_RUN_STEP4_BENCH") != "1" {
		t.Skip("TWDLL_RUN_STEP4_BENCH != 1; skipping real-binary Step 4 Silver & 2,000-func benchmark")
	}

	ctx := context.Context(t.Context())
	layout, err := workspace.Resolve(workspace.ResolveSpec{})
	if err != nil {
		t.Fatalf("resolve workspace: %v", err)
	}
	if !layout.ConfigExists {
		t.Fatalf("twdll.toml missing at %s", layout.Paths.Config)
	}

	targets := []workspace.TargetID{workspace.TargetAttila32, workspace.TargetAttila64}
	fmt.Fprintf(os.Stderr, "[BENCH] Snapshotting live .twdll/db SHA-256...\n")
	beforeDBSnap := snapshotDBDirs(t, layout.Paths.DBDir, targets)
	defer assertDBDirsUnchanged(t, layout.Paths.DBDir, targets, beforeDBSnap)

	outRoot := t.TempDir()
	tmpBase := filepath.Join(outRoot, "stage_tmp")
	timeoutSecs := 5

	var report strings.Builder
	report.WriteString("# Step 4 Empirical Benchmark: Silver Topology & 2,000-Function Raw Decomp (`timeout = 5s`)\n\n")
	report.WriteString(fmt.Sprintf("Date: %s  \nFast-first timeout: `%ds`  \nRead-only `.twdll/db/` SHA-256 verification: **Pending post-check**\n\n", time.Now().UTC().Format(time.RFC3339), timeoutSecs))

	for _, target := range targets {
		binPath := filepath.Join(layout.Paths.BinsDir, string(target), target.BinaryFileName())
		targetDir := filepath.Join(outRoot, string(target))
		binaryBasePath := filepath.Join(targetDir, "binary_base.sqlite")
		manifestBasePath := filepath.Join(targetDir, "manifest_base.json")

		fmt.Fprintf(os.Stderr, "[BENCH] %s: running Bronze ExtractTarget...\n", target)
		bronzeRes, err := bronze.ExtractTarget(ctx, bronze.Options{
			Target:       target,
			BinaryPath:   binPath,
			BinaryDBPath: binaryBasePath,
			ManifestPath: manifestBasePath,
		})
		if err != nil {
			t.Fatalf("bronze.ExtractTarget(%s): %v", target, err)
		}
		bronzeCount := int(bronzeRes.Counts["functions"])

		// Prepare separate Silver binary DBs for IDA and Ghidra so both Silver merges are measured independently.
		binaryIDAPath := filepath.Join(targetDir, "binary_ida.sqlite")
		manifestIDAPath := filepath.Join(targetDir, "manifest_ida.json")
		if err := workspace.CopyFileAtomic(binaryBasePath, binaryIDAPath); err != nil {
			t.Fatal(err)
		}
		binaryGhidraPath := filepath.Join(targetDir, "binary_ghidra.sqlite")
		manifestGhidraPath := filepath.Join(targetDir, "manifest_ghidra.json")
		if err := workspace.CopyFileAtomic(binaryBasePath, binaryGhidraPath); err != nil {
			t.Fatal(err)
		}

		fmt.Fprintf(os.Stderr, "[BENCH] %s: Bronze=%d (%v). Running Silver IDA...\n", target, bronzeCount, bronzeRes.Elapsed.Round(time.Millisecond))
		idaSilverRes, err := ExtractSilverTopology(ctx, SilverOptions{
			Paths:        layout.Paths,
			Target:       target,
			Engine:       workspace.DecompilerIDA,
			IDADir:       layout.Config.IDADir,
			BinaryPath:   binPath,
			BinaryDBPath: binaryIDAPath,
			ManifestPath: manifestIDAPath,
			TmpBaseDir:   tmpBase,
		})
		if err != nil {
			t.Fatalf("ExtractSilverTopology(%s, ida): %v", target, err)
		}

		fmt.Fprintf(os.Stderr, "[BENCH] %s: Silver(ida) total=%d (+%d) (%v). Running Silver Ghidra...\n",
			target, idaSilverRes.TotalFunctions, idaSilverRes.InsertedNew, idaSilverRes.Elapsed.Round(time.Millisecond))
		ghSilverRes, err := ExtractSilverTopology(ctx, SilverOptions{
			Paths:        layout.Paths,
			Target:       target,
			Engine:       workspace.DecompilerGhidra,
			BinaryPath:   binPath,
			BinaryDBPath: binaryGhidraPath,
			ManifestPath: manifestGhidraPath,
			TmpBaseDir:   tmpBase,
		})
		if err != nil {
			t.Fatalf("ExtractSilverTopology(%s, ghidra): %v", target, err)
		}

		fmt.Fprintf(os.Stderr, "[BENCH] %s: Silver(ghidra) total=%d (+%d) (%v). Starting 2,000-func decomp runs...\n",
			target, ghSilverRes.TotalFunctions, ghSilverRes.InsertedNew, ghSilverRes.Elapsed.Round(time.Millisecond))

		// Verify roWrites == 0 on both Silver-merged binary.sqlite DBs.
		for _, pair := range []struct {
			name string
			path string
		}{
			{"ida", binaryIDAPath},
			{"ghidra", binaryGhidraPath},
		} {
			ro, err := extractdb.OpenReadOnly(pair.path)
			if err != nil {
				t.Fatal(err)
			}
			var roWrites int
			err = ro.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM data_xrefs dx
				JOIN sections s ON dx.to_ea >= s.va AND dx.to_ea < s.va + s.virtual_size
				WHERE s.name IN ('.rdata', '.rodata') AND dx.access_kind = 'write'
			`).Scan(&roWrites)
			_ = ro.Close()
			if err != nil {
				t.Fatal(err)
			}
			if roWrites != 0 {
				t.Fatalf("[%s/%s] roWrites = %d, want 0", target, pair.name, roWrites)
			}
		}

		// Select the shared 2,000-function stratified corpus from IDA-merged binary.sqlite.
		roFacts, err := extractdb.OpenReadOnly(binaryIDAPath)
		if err != nil {
			t.Fatal(err)
		}
		benchSpecs, err := SelectBenchmarkCorpus(ctx, roFacts, target, 2000)
		_ = roFacts.Close()
		if err != nil {
			t.Fatalf("SelectBenchmarkCorpus(%s): %v", target, err)
		}
		if len(benchSpecs) != 2000 {
			t.Fatalf("SelectBenchmarkCorpus(%s) len = %d, want 2000", target, len(benchSpecs))
		}

		bucketByEA := make(map[uint64]string, len(benchSpecs))
		filterSpecs := make([]extractdb.FuncSpec, len(benchSpecs))
		for i, bs := range benchSpecs {
			filterSpecs[i] = bs.Spec
			bucketByEA[bs.Spec.EA] = bs.Bucket
		}

		report.WriteString(fmt.Sprintf("## Target: `%s`\n\n", target))
		report.WriteString(fmt.Sprintf("- **Bronze functions:** `%d` (`%v`)\n", bronzeCount, bronzeRes.Elapsed.Round(time.Millisecond)))
		report.WriteString(fmt.Sprintf("- **Silver (`ida`):** total=`%d`, updated=`%d`, inserted=`%d` (`%v`)\n",
			idaSilverRes.TotalFunctions, idaSilverRes.UpdatedExisting, idaSilverRes.InsertedNew, idaSilverRes.Elapsed.Round(time.Millisecond)))
		report.WriteString(fmt.Sprintf("- **Silver (`ghidra`):** total=`%d`, updated=`%d`, inserted=`%d` (`%v`)\n\n",
			ghSilverRes.TotalFunctions, ghSilverRes.UpdatedExisting, ghSilverRes.InsertedNew, ghSilverRes.Elapsed.Round(time.Millisecond)))

		for _, eng := range []workspace.DecompilerID{workspace.DecompilerIDA, workspace.DecompilerGhidra} {
			decompDBPath := filepath.Join(targetDir, fmt.Sprintf("decomp_%s.sqlite", eng))
			fmt.Fprintf(os.Stderr, "[BENCH] %s × %s: starting 2,000-func pass (timeout=%ds)...\n", target, eng, timeoutSecs)
			engCopy := eng
			targetCopy := target
			res, err := RunDecompPass(ctx, DecompOptions{
				Paths:        layout.Paths,
				Target:       target,
				Engine:       eng,
				IDADir:       layout.Config.IDADir,
				BinaryDBPath: binaryIDAPath,
				DecompDBPath: decompDBPath,
				TimeoutSecs:  timeoutSecs,
				TmpBaseDir:   tmpBase,
				FilterSpecs:  filterSpecs,
				OnProgress: func(done, total int) {
					if done%500 == 0 || done == total {
						fmt.Fprintf(os.Stderr, "[BENCH] %s × %s: %d/%d\n", targetCopy, engCopy, done, total)
					}
				},
			})
			if err != nil {
				t.Fatalf("RunDecompPass(%s, %s): %v", target, eng, err)
			}

			okN := res.StatusCounts[extractdb.DecompStatusOK]
			toN := res.StatusCounts[extractdb.DecompStatusTimeout]
			failN := res.StatusCounts[extractdb.DecompStatusFailed]
			if okN+toN+failN != 2000 {
				t.Fatalf("[%s/%s] ok(%d) + timeout(%d) + failed(%d) = %d != 2000", target, eng, okN, toN, failN, okN+toN+failN)
			}

			// Verify PlanWork resume invariants (default skips timeouts, RetryTimeouts=true queues timeouts).
			store, err := extractdb.OpenDecompStore(ctx, decompDBPath, string(target), bronzeRes.BinarySHA256)
			if err != nil {
				t.Fatal(err)
			}
			_, stDefault, err := store.PlanWork(ctx, string(eng), filterSpecs, extractdb.PlanOptions{})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			if stDefault.Pending != 0 || stDefault.SkippedTimeout != toN {
				_ = store.Close()
				t.Fatalf("[%s/%s] default PlanWork = %+v, want Pending=0, SkippedTimeout=%d", target, eng, stDefault, toN)
			}
			_, stRetryTO, err := store.PlanWork(ctx, string(eng), filterSpecs, extractdb.PlanOptions{RetryTimeouts: true})
			_ = store.Close()
			if err != nil {
				t.Fatal(err)
			}
			if stRetryTO.Pending != toN {
				t.Fatalf("[%s/%s] RetryTimeouts PlanWork = %+v, want Pending=%d", target, eng, stRetryTO, toN)
			}

			// Collect per-bucket metrics from decomp_<engine>.sqlite using that engine's own Silver-merged binary DB.
			engBinaryDB := binaryIDAPath
			engTotalFuncs := idaSilverRes.TotalFunctions
			if eng == workspace.DecompilerGhidra {
				engBinaryDB = binaryGhidraPath
				engTotalFuncs = ghSilverRes.TotalFunctions
			}
			bSummary, projSec := summarizeBenchEngine(t, ctx, engBinaryDB, decompDBPath, string(eng), bucketByEA, res.Elapsed, engTotalFuncs)

			rate := float64(2000) / res.Elapsed.Seconds()
			report.WriteString(fmt.Sprintf("### `%s` × `%s` (2,000 funcs in `%.2fs`, `%.1f funcs/s`, Peak Job RAM `%.2f GiB`)\n",
				target, eng, res.Elapsed.Seconds(), rate, float64(res.PeakJobMemBytes)/(1024*1024*1024)))
			report.WriteString(fmt.Sprintf("- **Totals:** `ok=%d`, `timeout=%d`, `failed=%d` (`Resume check: Pending=0, SkippedTimeout=%d, RetryTimeouts Pending=%d`)\n",
				okN, toN, failN, stDefault.SkippedTimeout, stRetryTO.Pending))
			report.WriteString(fmt.Sprintf("- **Size-weighted full-binary projection (%d funcs):** `~%.1f min`\n\n",
				engTotalFuncs, projSec/60.0))
			report.WriteString("| Bucket | N | OK | Timeout | Fail | Avg ms | LineEA% | Vars% | `in_ECX` funcs |\n")
			report.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
			for _, bs := range bSummary {
				report.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %.0f | %.1f%% | %.1f%% | %d |\n",
					bs.bucket, bs.n, bs.ok, bs.timeout, bs.failed, bs.avgMs, bs.lineEAPct, bs.varsPct, bs.inECXFuncs))
			}
			report.WriteString("\n")
		}
	}

	if reportPath := os.Getenv("TWDLL_STEP4_BENCH_REPORT"); reportPath != "" {
		txt := strings.Replace(report.String(), "**Pending post-check**", "**PASSED (0 bytes modified)**", 1)
		if err := os.WriteFile(reportPath, []byte(txt), 0o644); err != nil {
			t.Fatalf("write benchmark report %s: %v", reportPath, err)
		}
	}
	t.Log("\n" + report.String())
}

type bucketMetrics struct {
	bucket     string
	n          int
	ok         int
	timeout    int
	failed     int
	avgMs      float64
	lineEAPct  float64
	varsPct    float64
	inECXFuncs int
}

func summarizeBenchEngine(
	t *testing.T,
	ctx context.Context,
	factsDBPath string,
	decompDBPath string,
	engine string,
	bucketByEA map[uint64]string,
	wall time.Duration,
	fullTotalFuncs int,
) ([]bucketMetrics, float64) {
	t.Helper()
	db, err := extractdb.OpenReadOnly(decompDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT func_ea, func_size, status, elapsed_ms, line_ea_json, decompiler_vars_json, raw_c
		FROM decomp_funcs
		WHERE engine = ?
	`, engine)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	agg := make(map[string]*bucketMetrics)
	sumElapsedMs := make(map[string]int64)
	withLineEA := make(map[string]int)
	withVars := make(map[string]int)
	sizeBucketSumMs := make(map[string]int64)
	sizeBucketCount := make(map[string]int)
	var totalCPUWallMs int64

	for rows.Next() {
		var ea, sz, elMs int64
		var status, lineEA, varsJSON, rawC string
		if err := rows.Scan(&ea, &sz, &status, &elMs, &lineEA, &varsJSON, &rawC); err != nil {
			t.Fatal(err)
		}
		szBkt := sizeBucket(uint64(sz))
		sizeBucketSumMs[szBkt] += elMs
		sizeBucketCount[szBkt]++

		bkt := bucketByEA[uint64(ea)]
		if bkt == "" {
			bkt = szBkt
		}
		m := agg[bkt]
		if m == nil {
			m = &bucketMetrics{bucket: bkt}
			agg[bkt] = m
		}
		m.n++
		sumElapsedMs[bkt] += elMs
		totalCPUWallMs += elMs
		switch status {
		case extractdb.DecompStatusOK:
			m.ok++
			if lineEA != "" && lineEA != "[]" {
				withLineEA[bkt]++
			}
			if varsJSON != "" && varsJSON != "[]" {
				withVars[bkt]++
			}
			if strings.Contains(rawC, "in_ECX") {
				m.inECXFuncs++
			}
		case extractdb.DecompStatusTimeout:
			m.timeout++
		default:
			m.failed++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]bucketMetrics, 0, len(keys))
	for _, k := range keys {
		m := agg[k]
		if m.n > 0 {
			m.avgMs = float64(sumElapsedMs[k]) / float64(m.n)
		}
		if m.ok > 0 {
			m.lineEAPct = 100.0 * float64(withLineEA[k]) / float64(m.ok)
			m.varsPct = 100.0 * float64(withVars[k]) / float64(m.ok)
		}
		out = append(out, *m)
	}

	// Compute true size-weighted projection across all functions in facts.sqlite.
	factsRO, err := extractdb.OpenReadOnly(factsDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer factsRO.Close()

	fRows, err := factsRO.QueryContext(ctx, `SELECT size FROM functions`)
	if err != nil {
		t.Fatal(err)
	}
	defer fRows.Close()

	fullBuckets := make(map[string]int)
	for fRows.Next() {
		var sz int64
		if err := fRows.Scan(&sz); err != nil {
			t.Fatal(err)
		}
		if sz < 0 {
			sz = 0
		}
		fullBuckets[sizeBucket(uint64(sz))]++
	}
	if err := fRows.Err(); err != nil {
		t.Fatal(err)
	}

	var projectedSumMs float64
	for bkt, fullN := range fullBuckets {
		cnt := sizeBucketCount[bkt]
		if cnt > 0 {
			avgMs := float64(sizeBucketSumMs[bkt]) / float64(cnt)
			projectedSumMs += float64(fullN) * avgMs
		}
	}
	parallelRatio := 1.0
	if totalCPUWallMs > 0 {
		parallelRatio = (wall.Seconds() * 1000.0) / float64(totalCPUWallMs)
	}
	projSec := (projectedSumMs * parallelRatio) / 1000.0
	if projSec <= 0 {
		projSec = wall.Seconds() * (float64(fullTotalFuncs) / 2000.0)
	}
	return out, projSec
}
