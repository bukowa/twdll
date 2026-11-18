package silver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

func TestSmokeRealDatabases(t *testing.T) {
	if os.Getenv("TWDLL_SMOKE_SILVER") != "1" {
		t.Skip("TWDLL_SMOKE_SILVER not set; skipping real-database Silver smoke suite")
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(repoRoot, "twdll.toml")
	cfg, _, err := workspace.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig(%s): %v", cfgPath, err)
	}
	paths := workspace.NewPaths(repoRoot, cfgPath, filepath.Join(repoRoot, ".twdll"))

	outDir, err := os.MkdirTemp("", "twdll_silver_smoke_*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outDir) })
	tmpDir := filepath.Join(outDir, "tmp")

	type job struct {
		target        workspace.TargetID
		engine        workspace.DecompilerID
		eas           []uint64
		expectSigs    bool
		expectDWARF   bool
		requireAllOK  bool
	}
	jobs := []job{
		{
			target:       workspace.TargetAttila32,
			engine:       workspace.DecompilerIDA,
			eas:          []uint64{0x10016480, 0x100de9b0, 0x11273910, 0x119f771d, 0x102de176, 0x1194ee60, 0x11965c90, 0x119eb6cf},
			expectSigs:   true,
			requireAllOK: true,
		},
		{
			target:       workspace.TargetAttila32,
			engine:       workspace.DecompilerGhidra,
			eas:          []uint64{0x10016480, 0x101abcc0, 0x1194ee60, 0x11965c90, 0x119eb6cf},
			expectSigs:   true,
			requireAllOK: true,
		},
		{
			target: workspace.TargetAttila64,
			engine: workspace.DecompilerIDA,
			eas: []uint64{
				0x1128c00, 0x8c5600, 0x9c0300, 0xc25420, 0x1b2ba90,
				0x136b370, 0x18a0d50, 0x159e3a0, 0x16f5bf0, 0x42c910,
				0x50cea0, 0x567810, 0x59b400,
			},
			expectSigs:   true,
			requireAllOK: true,
		},
		{
			target: workspace.TargetAttila64,
			engine: workspace.DecompilerGhidra,
			eas: []uint64{
				0x50cea0, 0x567810, 0x59b400, 0x5e5860,
				0x1128c00, 0x18a0d50, 0x159e3a0, 0x16f5bf0,
			},
			expectSigs:   true,
			expectDWARF:  true,
			requireAllOK: true,
		},
	}

	ctx := context.Background()
	for _, j := range jobs {
		j := j
		t.Run(fmt.Sprintf("%s_%s", j.target, j.engine), func(t *testing.T) {
			if !paths.HasValidDatabase(j.engine, j.target) {
				t.Skipf("database not present for %s/%s", j.target, j.engine)
			}
			dbPath := filepath.Join(outDir, fmt.Sprintf("silver_%s_%s.sqlite", j.target, j.engine))
			t0 := time.Now()
			res, err := ExtractTarget(ctx, Options{
				Paths:        paths,
				Target:       j.target,
				Engine:       j.engine,
				IDADir:       cfg.IDADir,
				BinaryPath:   paths.BinaryPath(j.target),
				SilverDBPath: dbPath,
				TmpBaseDir:   tmpDir,
				TimeoutSecs:  30,
				Workers:      4,
				Selector:     EASelector(j.eas...),
			})
			if err != nil {
				t.Fatalf("ExtractTarget: %v", err)
			}
			t.Logf("[%s/%s] elapsed=%v funcs=%d asm=%d decomp=%d status=%v",
				j.target, j.engine, time.Since(t0).Round(time.Millisecond),
				res.FuncCount, res.AsmCount, res.DecompWritten, res.StatusCounts)

			db, err := extractdb.OpenReadOnly(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			var sizeMismatch, emptyRange, sigCnt, distinctStart, totalRanges int
			if err := db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM silver_funcs
				WHERE size != COALESCE((SELECT SUM(end_ea - start_ea) FROM silver_func_ranges WHERE func_ea = silver_funcs.ea), 0)
			`).Scan(&sizeMismatch); err != nil {
				t.Fatal(err)
			}
			if sizeMismatch != 0 {
				t.Errorf("size != sum(ranges) count = %d, want 0", sizeMismatch)
			}
			if err := db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM silver_funcs WHERE ea NOT IN (SELECT DISTINCT func_ea FROM silver_func_ranges)
			`).Scan(&emptyRange); err != nil {
				t.Fatal(err)
			}
			if emptyRange != 0 {
				t.Errorf("functions with 0 range rows = %d, want 0", emptyRange)
			}
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM silver_funcs WHERE signature != ''`).Scan(&sigCnt); err != nil {
				t.Fatal(err)
			}
			if j.expectSigs && sigCnt == 0 {
				t.Errorf("expected signature != '' rows, got 0")
			}
			if err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT start_ea), COUNT(*) FROM silver_func_ranges`).Scan(&distinctStart, &totalRanges); err != nil {
				t.Fatal(err)
			}
			t.Logf("  sigs=%d/%d ranges=%d (distinct_start=%d)", sigCnt, res.FuncCount, totalRanges, distinctStart)

			var asmCnt, asmEmptyNonThunk, asmEmptyThunk int
			if err := db.QueryRowContext(ctx, `
				SELECT
					COUNT(*),
					COALESCE(SUM(CASE WHEN f.is_thunk = 0 AND (a.insn_count <= 0 OR a.raw_asm = '') THEN 1 ELSE 0 END), 0),
					COALESCE(SUM(CASE WHEN f.is_thunk != 0 AND (a.insn_count <= 0 OR a.raw_asm = '') THEN 1 ELSE 0 END), 0)
				FROM silver_asm a
				JOIN silver_funcs f ON f.ea = a.func_ea
			`).Scan(&asmCnt, &asmEmptyNonThunk, &asmEmptyThunk); err != nil {
				t.Fatal(err)
			}
			if asmCnt != res.FuncCount || asmEmptyNonThunk != 0 {
				t.Errorf("silver_asm count=%d emptyNonThunk=%d (emptyThunk=%d), want count=%d emptyNonThunk=0",
					asmCnt, asmEmptyNonThunk, asmEmptyThunk, res.FuncCount)
			}
			t.Logf("  silver_asm=%d (empty_thunk=%d)", asmCnt, asmEmptyThunk)

			// Print one real sample from silver_funcs, silver_asm, and decomp_funcs for inspection.
			var sEA int64
			var sSize, sInsn int
			var sName, sSig, sCC, sAsm string
			if err := db.QueryRowContext(ctx, `
				SELECT f.ea, f.size, f.name, f.signature, f.calling_conv, a.insn_count, a.raw_asm
				FROM silver_funcs f
				JOIN silver_asm a ON a.func_ea = f.ea
				JOIN decomp_funcs d ON d.func_ea = f.ea
				WHERE d.status = 'ok' AND f.signature != '' AND a.insn_count BETWEEN 5 AND 25
				ORDER BY f.ea LIMIT 1
			`).Scan(&sEA, &sSize, &sName, &sSig, &sCC, &sInsn, &sAsm); err == nil {
				t.Logf("  sample silver_funcs: ea=0x%x size=%d cc=%q name=%q sig=%q", sEA, sSize, sCC, sName, sSig)
				t.Logf("  sample silver_asm (%d insns):\n%s", sInsn, sAsm)
			}

			ccRows, err := db.QueryContext(ctx, `SELECT calling_conv, COUNT(*) FROM silver_funcs GROUP BY calling_conv ORDER BY COUNT(*) DESC`)
			if err != nil {
				t.Fatal(err)
			}
			var ccList []string
			for ccRows.Next() {
				var cc string
				var cnt int
				if err := ccRows.Scan(&cc, &cnt); err != nil {
					t.Fatal(err)
				}
				ccList = append(ccList, fmt.Sprintf("%q:%d", cc, cnt))
			}
			ccRows.Close()
			t.Logf("  calling_convs: %s", strings.Join(ccList, ", "))

			dRows, err := db.QueryContext(ctx, `SELECT func_ea, func_size, status, elapsed_ms, raw_c, line_ea_json, vars_json, error_text FROM decomp_funcs ORDER BY func_ea`)
			if err != nil {
				t.Fatal(err)
			}
			defer dRows.Close()

			dwarfSeen := 0
			for dRows.Next() {
				var ea, sz, elMs int64
				var st, rawC, lineEA, varsJSON, errTxt string
				if err := dRows.Scan(&ea, &sz, &st, &elMs, &rawC, &lineEA, &varsJSON, &errTxt); err != nil {
					t.Fatal(err)
				}
				if j.requireAllOK && st != extractdb.DecompStatusOK {
					t.Errorf("0x%x status=%s (err=%q), want ok", ea, st, errTxt)
				}
				lines := strings.Split(rawC, "\n")
				var entries [][]any
				if err := json.Unmarshal([]byte(lineEA), &entries); err != nil {
					t.Errorf("0x%x invalid line_ea_json: %v", ea, err)
				}
				for _, e := range entries {
					if len(e) != 3 {
						t.Errorf("0x%x malformed line_ea entry: %v", ea, e)
						continue
					}
					ln, ok := e[0].(float64)
					if !ok || int(ln) < 1 || int(ln) > len(lines) {
						t.Errorf("0x%x out-of-range line_ea %v (total lines=%d)", ea, e, len(lines))
					}
				}
				if j.engine == workspace.DecompilerIDA && strings.Contains(rawC, "a1: ") {
					t.Errorf("0x%x contains IDA inlay hint 'a1: '", ea)
				}
				if strings.Contains(rawC, "DWARF") {
					dwarfSeen++
				}
				t.Logf("  0x%x size=%d status=%s el=%dms lines=%d line_ea=%d", ea, sz, st, elMs, len(lines), len(entries))
			}
			if j.expectDWARF && dwarfSeen == 0 {
				t.Errorf("expected DWARF comments in Ghidra attila_64 raw_c, got 0")
			}
		})
	}
}
