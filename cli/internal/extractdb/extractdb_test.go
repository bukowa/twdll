package extractdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"twdll/cli/internal/workspace"
)

func TestAtomicBuilder_CommitPurgesStaleSidecarsAndRebuildsBronzeFTS(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "attila_32", "bronze.sqlite")

	// Missing DB must fail OpenReadOnly without creating a 0-byte file
	if _, err := OpenReadOnly(dbPath); err == nil {
		t.Fatalf("expected OpenReadOnly on missing file to fail")
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenReadOnly must not create missing file, stat err=%v", err)
	}

	// Pre-plant stale sidecars alongside destination and temp path
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []string{
		dbPath + "-wal",
		dbPath + "-shm",
		dbPath + "-journal",
		dbPath + ".tmp",
		dbPath + ".tmp-wal",
	} {
		if err := os.WriteFile(stale, []byte("stale-sidecar"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	builder, err := NewAtomicBuilder(dbPath)
	if err != nil {
		t.Fatalf("NewAtomicBuilder: %v", err)
	}
	defer func() { _ = builder.Abort() }()

	if err := InitBronzeSchema(ctx, builder.DB()); err != nil {
		t.Fatalf("InitBronzeSchema: %v", err)
	}

	if _, err := builder.DB().ExecContext(ctx, `
		INSERT INTO dwarf_funcs(ea, size, name, linkage_name, class_name)
		VALUES (0x10001000, 64, 'CA::Campaign::Tick', '_ZN2CA8Campaign4TickEv', 'CA::Campaign');
		INSERT INTO types(type_id, kind, name, byte_size)
		VALUES (1, 'class', 'CA::Campaign', 256);
		INSERT INTO strings(str_id, ea, encoding, value)
		VALUES (1, 0x10500000, 'ascii', 'lua_campaign_tick');
	`); err != nil {
		t.Fatalf("insert sample bronze facts: %v", err)
	}

	if err := RebuildBronzeFTS(ctx, builder.DB()); err != nil {
		t.Fatalf("RebuildBronzeFTS: %v", err)
	}

	counts, err := RefreshBronzeCounts(ctx, builder.DB())
	if err != nil {
		t.Fatalf("RefreshBronzeCounts: %v", err)
	}
	if counts["dwarf_funcs"] != 1 || counts["types"] != 1 || counts["strings"] != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}

	if err := builder.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Stale sidecars and .tmp files must be completely gone
	for _, gone := range []string{
		dbPath + "-wal",
		dbPath + "-shm",
		dbPath + "-journal",
		dbPath + ".tmp",
		dbPath + ".tmp-wal",
	} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s to be removed on Commit, stat err=%v", gone, err)
		}
	}

	roDB, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer roDB.Close()

	var integrity string
	if err := roDB.QueryRowContext(ctx, `PRAGMA integrity_check;`).Scan(&integrity); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q, want ok", integrity)
	}

	var matchedEA int64
	if err := roDB.QueryRowContext(ctx, `SELECT rowid FROM dwarf_funcs_fts WHERE dwarf_funcs_fts MATCH 'Campaign'`).Scan(&matchedEA); err != nil {
		t.Fatalf("dwarf_funcs_fts MATCH: %v", err)
	}
	if matchedEA != 0x10001000 {
		t.Fatalf("dwarf_funcs_fts rowid = 0x%x, want 0x10001000", matchedEA)
	}

	var matchedType int64
	if err := roDB.QueryRowContext(ctx, `SELECT rowid FROM types_fts WHERE types_fts MATCH 'Campaign'`).Scan(&matchedType); err != nil {
		t.Fatalf("types_fts MATCH: %v", err)
	}
	if matchedType != 1 {
		t.Fatalf("types_fts rowid = %d, want 1", matchedType)
	}

	var matchedStr int64
	if err := roDB.QueryRowContext(ctx, `SELECT rowid FROM strings_fts WHERE strings_fts MATCH 'lua_campaign_tick'`).Scan(&matchedStr); err != nil {
		t.Fatalf("strings_fts MATCH: %v", err)
	}
	if matchedStr != 1 {
		t.Fatalf("strings_fts rowid = %d, want 1", matchedStr)
	}

	// Query-only enforcement
	if _, err := roDB.ExecContext(ctx, `DELETE FROM dwarf_funcs;`); err == nil {
		t.Fatalf("expected write on OpenReadOnly connection to fail")
	}
}

func TestAtomicBuilder_AbortPreservesPriorDatabaseAndLeavesZeroTempFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bronze.sqlite")

	b1, err := NewAtomicBuilder(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitBronzeSchema(ctx, b1.DB()); err != nil {
		t.Fatal(err)
	}
	if _, err := b1.DB().ExecContext(ctx, `INSERT INTO names(name_id, value) VALUES (1, 'healthy_symbol');`); err != nil {
		t.Fatal(err)
	}
	if err := b1.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	b2, err := NewAtomicBuilder(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitBronzeSchema(ctx, b2.DB()); err != nil {
		t.Fatal(err)
	}
	if _, err := b2.DB().ExecContext(ctx, `INSERT INTO names(name_id, value) VALUES (1, 'corrupted_partial');`); err != nil {
		t.Fatal(err)
	}
	if err := b2.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if err := b2.Abort(); err != nil {
		t.Fatalf("second Abort: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "bronze.sqlite" {
		t.Fatalf("expected only bronze.sqlite in %s after Abort, got %v", dir, entries)
	}

	roDB, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer roDB.Close()

	var val string
	if err := roDB.QueryRowContext(ctx, `SELECT value FROM names WHERE name_id = 1`).Scan(&val); err != nil {
		t.Fatal(err)
	}
	if val != "healthy_symbol" {
		t.Fatalf("prior database was overwritten despite Abort: got %q, want healthy_symbol", val)
	}
}

func TestSilverStore_FuncsDecompResumeUpsertFTSAndWALTruncate(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "silver_ghidra.sqlite")

	store, err := OpenSilverStore(ctx, dbPath, "attila_32", "ghidra", "sha_v1", "")
	if err != nil {
		t.Fatalf("OpenSilverStore: %v", err)
	}

	hasFuncs, err := store.HasFuncs(ctx)
	if err != nil || hasFuncs {
		t.Fatalf("initial HasFuncs = (%v, %v), want (false, nil)", hasFuncs, err)
	}

	silverFuncs := []SilverFuncRow{
		{EA: 0x1000, Size: 32, Name: "FuncAlpha", CallingConv: "__thiscall", StackPurge: 4},
		{EA: 0x2000, Size: 48, Name: "FuncBeta", CallingConv: "__cdecl"},
		{EA: 0x3000, Size: 64, Name: "FuncOrphan", CallingConv: "__stdcall"},
	}
	silverRanges := []SilverFuncRangeRow{
		{StartEA: 0x1000, EndEA: 0x1020, FuncEA: 0x1000},
		{StartEA: 0x2000, EndEA: 0x2030, FuncEA: 0x2000},
		{StartEA: 0x3000, EndEA: 0x3040, FuncEA: 0x3000},
		{StartEA: 0x5000, EndEA: 0x5010, FuncEA: 0x1000},
		{StartEA: 0x5000, EndEA: 0x5010, FuncEA: 0x2000},
	}
	if err := store.ReplaceFuncs(ctx, silverFuncs, silverRanges, "funcs,decomp"); err != nil {
		t.Fatalf("ReplaceFuncs: %v", err)
	}
	var rangeCount int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM silver_func_ranges`).Scan(&rangeCount); err != nil {
		t.Fatalf("count silver_func_ranges: %v", err)
	}
	if rangeCount != 5 {
		t.Fatalf("silver_func_ranges count = %d, want 5", rangeCount)
	}
	var sharedRangeCount int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM silver_func_ranges WHERE start_ea = 0x5000`).Scan(&sharedRangeCount); err != nil {
		t.Fatalf("count shared silver_func_ranges: %v", err)
	}
	if sharedRangeCount != 2 {
		t.Fatalf("silver_func_ranges count for start_ea 0x5000 = %d, want 2", sharedRangeCount)
	}
	hasFuncs, err = store.HasFuncs(ctx)
	if err != nil || !hasFuncs {
		t.Fatalf("HasFuncs after ReplaceFuncs = (%v, %v), want (true, nil)", hasFuncs, err)
	}

	hasAsm, err := store.HasAsm(ctx)
	if err != nil || hasAsm {
		t.Fatalf("initial HasAsm = (%v, %v), want (false, nil)", hasAsm, err)
	}
	if err := store.ReplaceAsm(ctx, []SilverAsmRow{
		{FuncEA: 0x1000, InsnCount: 2, RawAsm: "1000: push ebp\n1001: ret"},
		{FuncEA: 0x2000, InsnCount: 1, RawAsm: "2000: ret"},
	}, ""); err != nil {
		t.Fatalf("ReplaceAsm: %v", err)
	}
	hasAsm, err = store.HasAsm(ctx)
	if err != nil || !hasAsm {
		t.Fatalf("HasAsm after ReplaceAsm = (%v, %v), want (true, nil)", hasAsm, err)
	}
	asmCount, err := store.AsmCount(ctx)
	if err != nil || asmCount != 2 {
		t.Fatalf("AsmCount = (%d, %v), want (2, nil)", asmCount, err)
	}
	var gotInsnCount int
	var gotRawAsm string
	if err := store.DB().QueryRowContext(ctx, `SELECT insn_count, raw_asm FROM silver_asm WHERE func_ea = 0x1000`).Scan(&gotInsnCount, &gotRawAsm); err != nil {
		t.Fatalf("query silver_asm: %v", err)
	}
	if gotInsnCount != 2 || !strings.Contains(gotRawAsm, "push ebp") {
		t.Fatalf("unexpected silver_asm row: insn_count=%d raw_asm=%q", gotInsnCount, gotRawAsm)
	}

	specsFromDB, err := store.ListFuncSpecs(ctx)
	if err != nil {
		t.Fatalf("ListFuncSpecs: %v", err)
	}
	if len(specsFromDB) != 3 || specsFromDB[0].EA != 0x1000 || specsFromDB[2].EA != 0x3000 {
		t.Fatalf("unexpected specsFromDB: %+v", specsFromDB)
	}

	var matchedFnEA int64
	if err := store.DB().QueryRowContext(ctx, `SELECT rowid FROM silver_funcs_fts WHERE silver_funcs_fts MATCH 'FuncAlpha'`).Scan(&matchedFnEA); err != nil {
		t.Fatalf("silver_funcs_fts MATCH: %v", err)
	}
	if matchedFnEA != 0x1000 {
		t.Fatalf("silver_funcs_fts rowid = 0x%x, want 0x1000", matchedFnEA)
	}

	pending, stats, err := store.PlanWork(ctx, specsFromDB, PlanOptions{})
	if err != nil {
		t.Fatalf("initial PlanWork: %v", err)
	}
	if len(pending) != 3 || stats.Pending != 3 || stats.Total != 3 {
		t.Fatalf("unexpected initial plan: pending=%d stats=%+v", len(pending), stats)
	}

	ch := make(chan DecompRecord, 3)
	ch <- DecompRecord{
		FuncEA:   0x1000,
		FuncSize: 32,
		FuncName: "FuncAlpha",
		RawC:     "void FuncAlpha(void) { int alpha_magic = 1; }",
		Status:   DecompStatusOK,
	}
	ch <- DecompRecord{
		FuncEA:    0x2000,
		FuncSize:  48,
		FuncName:  "FuncBeta",
		Status:    DecompStatusFailed,
		ErrorText: "decompile timeout",
	}
	ch <- DecompRecord{
		FuncEA:   0x3000,
		FuncSize: 64,
		FuncName: "FuncOrphan",
		RawC:     "void FuncOrphan(void) { int orphan_token = 99; }",
		Status:   DecompStatusOK,
	}
	close(ch)

	written, err := store.RunBatchWriter(ctx, ch, 2, 15*time.Millisecond)
	if err != nil {
		t.Fatalf("RunBatchWriter: %v", err)
	}
	if written != 3 {
		t.Fatalf("written = %d, want 3", written)
	}

	nextSpecs := []FuncSpec{
		{EA: 0x1000, Size: 40, Name: "FuncAlpha"},
		{EA: 0x2000, Size: 48, Name: "FuncBeta"},
		{EA: 0x4000, Size: 16, Name: "FuncGamma"},
	}

	pendingNoRetry, statsNoRetry, err := store.PlanWork(ctx, nextSpecs, PlanOptions{RetryFailed: false})
	if err != nil {
		t.Fatalf("PlanWork(RetryFailed=false): %v", err)
	}
	if statsNoRetry.OrphanDeleted != 0 || statsNoRetry.SizeChanged != 1 || statsNoRetry.SkippedFailed != 1 || len(pendingNoRetry) != 2 {
		t.Fatalf("unexpected statsNoRetry=%+v pending=%v", statsNoRetry, pendingNoRetry)
	}

	pendingRetry, statsRetry, err := store.PlanWork(ctx, nextSpecs, PlanOptions{RetryFailed: true})
	if err != nil {
		t.Fatalf("PlanWork(RetryFailed=true): %v", err)
	}
	if statsRetry.OrphanDeleted != 0 || statsRetry.SizeChanged != 1 || statsRetry.SkippedFailed != 0 || len(pendingRetry) != 3 {
		t.Fatalf("unexpected statsRetry=%+v pending=%v", statsRetry, pendingRetry)
	}

	pendingPrune, statsPrune, err := store.PlanWork(ctx, nextSpecs, PlanOptions{PruneOrphans: true})
	if err != nil {
		t.Fatalf("PlanWork(PruneOrphans=true): %v", err)
	}
	if statsPrune.OrphanDeleted != 1 || len(pendingPrune) != 2 {
		t.Fatalf("unexpected statsPrune=%+v pending=%v", statsPrune, pendingPrune)
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	ch2 := make(chan DecompRecord, 3)
	ch2 <- DecompRecord{FuncEA: 0x1000, FuncSize: 40, FuncName: "FuncAlpha", RawC: "void FuncAlpha(void) { int alpha_v2 = 2; }", Status: DecompStatusOK}
	ch2 <- DecompRecord{FuncEA: 0x2000, FuncSize: 48, FuncName: "FuncBeta", RawC: "void FuncBeta(void) { int beta_recovered = 42; }", Status: DecompStatusOK}
	ch2 <- DecompRecord{FuncEA: 0x4000, FuncSize: 16, FuncName: "FuncGamma", RawC: "void FuncGamma(void) {}", Status: DecompStatusOK}
	close(ch2)

	written2, err := store.RunBatchWriter(cancelCtx, ch2, 250, time.Second)
	if err != nil {
		t.Fatalf("RunBatchWriter on cancelled ctx: %v", err)
	}
	if written2 != 3 {
		t.Fatalf("expected all 3 queued records drained on cancelled ctx, got %d", written2)
	}

	pendingFinal, statsFinal, err := store.PlanWork(ctx, nextSpecs, PlanOptions{})
	if err != nil {
		t.Fatalf("final PlanWork: %v", err)
	}
	if len(pendingFinal) != 0 || statsFinal.SkippedOK != 3 {
		t.Fatalf("expected all 3 specs SkippedOK, got pending=%v stats=%+v", pendingFinal, statsFinal)
	}

	var count int
	if err := store.DB().QueryRowContext(ctx, `SELECT count(*) FROM decomp_fts WHERE decomp_fts MATCH 'beta_recovered'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("decomp_fts MATCH 'beta_recovered' = %d, want 1", count)
	}
	if err := store.DB().QueryRowContext(ctx, `SELECT count(*) FROM decomp_fts WHERE decomp_fts MATCH 'orphan_token'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("decomp_fts MATCH 'orphan_token' = %d, want 0 after orphan delete", count)
	}

	if _, err := store.DB().ExecContext(ctx, `INSERT INTO decomp_fts(decomp_fts) VALUES('integrity-check');`); err != nil {
		t.Fatalf("decomp_fts integrity-check failed: %v", err)
	}
	var integrity string
	if err := store.DB().QueryRowContext(ctx, `PRAGMA integrity_check;`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("PRAGMA integrity_check = %q, err=%v", integrity, err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		if _, statErr := os.Stat(dbPath + suffix); !os.IsNotExist(statErr) {
			t.Fatalf("expected Close() to normalize out of WAL mode and remove %s%s (statErr=%v)", dbPath, suffix, statErr)
		}
	}

	roClosed, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly after Close(): %v", err)
	}
	var roCount int
	if err := roClosed.QueryRowContext(ctx, `SELECT COUNT(*) FROM decomp_funcs`).Scan(&roCount); err != nil {
		_ = roClosed.Close()
		t.Fatalf("query closed decomp_funcs via OpenReadOnly: %v", err)
	}
	_ = roClosed.Close()
	if roCount != 3 {
		t.Fatalf("OpenReadOnly decomp_funcs count = %d, want 3", roCount)
	}

	// Opening with a different binary SHA-256 or enricher key resets the store cleanly
	storeV2, err := OpenSilverStore(ctx, dbPath, "attila_32", "ghidra", "sha_v2", "")
	if err != nil {
		t.Fatalf("OpenSilverStore(sha_v2): %v", err)
	}
	if err := storeV2.DB().QueryRowContext(ctx, `SELECT count(*) FROM decomp_funcs`).Scan(&count); err != nil {
		_ = storeV2.Close()
		t.Fatal(err)
	}
	if count != 0 {
		_ = storeV2.Close()
		t.Fatalf("expected binary SHA drift to reset decomp_funcs, got %d rows", count)
	}
	if err := storeV2.WriteBatch(ctx, []DecompRecord{{FuncEA: 0x1, FuncSize: 4, Status: DecompStatusOK}}); err != nil {
		_ = storeV2.Close()
		t.Fatal(err)
	}
	_ = storeV2.Close()

	storeEnriched, err := OpenSilverStore(ctx, dbPath, "attila_32", "ghidra", "sha_v2", "custom_enricher")
	if err != nil {
		t.Fatalf("OpenSilverStore(enricher drift): %v", err)
	}
	defer storeEnriched.Close()
	if err := storeEnriched.DB().QueryRowContext(ctx, `SELECT count(*) FROM decomp_funcs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected enricher drift to reset decomp_funcs, got %d rows", count)
	}
}

func TestSilverStore_ConcurrentReadOnlyFTSWhileWritingBatches(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "silver_ghidra.sqlite")

	store, err := OpenSilverStore(ctx, dbPath, "attila_32", "ghidra", "sha_concurrent", "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.WriteBatch(ctx, []DecompRecord{{
		FuncEA:   0x100,
		FuncSize: 16,
		FuncName: "SeedFunc",
		RawC:     "void SeedFunc(void) { int warscape_token = 1; }",
		Status:   DecompStatusOK,
	}}); err != nil {
		t.Fatal(err)
	}

	roDB, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer roDB.Close()

	in := make(chan DecompRecord, 64)
	writerDone := make(chan error, 1)
	go func() {
		_, wErr := store.RunBatchWriter(ctx, in, 10, 10*time.Millisecond)
		writerDone <- wErr
	}()

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	stopReaders := make(chan struct{})
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
					var n int
					if qErr := roDB.QueryRowContext(ctx, `SELECT count(*) FROM decomp_fts WHERE decomp_fts MATCH 'warscape_token'`).Scan(&n); qErr != nil {
						errCh <- qErr
						return
					}
					if n < 1 {
						errCh <- fmt.Errorf("expected at least 1 FTS match, got %d", n)
						return
					}
				}
			}
		}()
	}

	for i := 1; i <= 100; i++ {
		in <- DecompRecord{
			FuncEA:   uint64(0x1000 + i*0x10),
			FuncSize: 32,
			FuncName: fmt.Sprintf("Sub_%d", i),
			RawC:     fmt.Sprintf("void Sub_%d(void) { int warscape_token = %d; }", i, i),
			Status:   DecompStatusOK,
		}
	}
	close(in)

	if wErr := <-writerDone; wErr != nil {
		close(stopReaders)
		wg.Wait()
		t.Fatalf("RunBatchWriter failed: %v", wErr)
	}
	close(stopReaders)
	wg.Wait()
	close(errCh)

	for rErr := range errCh {
		t.Fatalf("concurrent reader error: %v", rErr)
	}

	var finalCount int
	if err := roDB.QueryRowContext(ctx, `SELECT count(*) FROM decomp_fts WHERE decomp_fts MATCH 'warscape_token'`).Scan(&finalCount); err != nil {
		t.Fatal(err)
	}
	if finalCount != 101 {
		t.Fatalf("final FTS count = %d, want 101", finalCount)
	}
}

func TestTargetManifest_WriteAndReadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attila_32", "manifest.json")
	want := TargetManifest{
		Target:       "attila_32",
		BinarySHA256: "abc123def456",
		ExtractedAt:  "2026-11-16T00:00:00Z",
		Extractors:   []string{"pe32.format", "data.strings"},
		Counts:       map[string]int{"sections": 6, "strings": 450},
	}
	if err := WriteManifestAtomic(path, want); err != nil {
		t.Fatalf("WriteManifestAtomic: %v", err)
	}
	got, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if got.Target != want.Target || got.BinarySHA256 != want.BinarySHA256 || got.Counts["sections"] != 6 {
		t.Fatalf("manifest mismatch: got %+v, want %+v", got, want)
	}
}

func TestOpenSilverStore_PreservesExistingCacheOnCancelledCtxOrEmptySHA(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "silver_ghidra.sqlite")

	store, err := OpenSilverStore(ctx, dbPath, "attila_32", "ghidra", "sha_keep_me", "")
	if err != nil {
		t.Fatalf("OpenSilverStore: %v", err)
	}
	if err := store.WriteBatch(ctx, []DecompRecord{{
		FuncEA:   0x5000,
		FuncSize: 24,
		FuncName: "PreciousFunc",
		RawC:     "void PreciousFunc(void) {}",
		Status:   DecompStatusOK,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := OpenSilverStore(ctx, dbPath, "attila_32", "ghidra", "   ", ""); err == nil {
		t.Fatalf("expected OpenSilverStore with empty binarySHA256 to fail")
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := OpenSilverStore(cancelCtx, dbPath, "attila_32", "ghidra", "sha_keep_me", ""); err == nil {
		t.Fatalf("expected OpenSilverStore with cancelled ctx to fail")
	}

	reopened, err := OpenSilverStore(ctx, dbPath, "attila_32", "ghidra", "sha_keep_me", "")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	var count int
	if err := reopened.DB().QueryRowContext(ctx, `SELECT count(*) FROM decomp_funcs WHERE func_ea = 0x5000`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected existing decomp_funcs row preserved, got count=%d", count)
	}
}

func TestRunBatchWriter_DrainsRecordSentAfterCtxCancelBeforeChannelClose(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "silver_ida.sqlite")

	store, err := OpenSilverStore(ctx, dbPath, "attila_32", "ida", "sha_drain", "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	runCtx, cancel := context.WithCancel(ctx)
	in := make(chan DecompRecord)
	doneCh := make(chan int, 1)
	errCh := make(chan error, 1)

	go func() {
		n, wErr := store.RunBatchWriter(runCtx, in, 250, time.Hour)
		doneCh <- n
		errCh <- wErr
	}()

	cancel()
	time.Sleep(20 * time.Millisecond)
	in <- DecompRecord{
		FuncEA:   0x7777,
		FuncSize: 32,
		FuncName: "LateInFlightWorker",
		RawC:     "void LateInFlightWorker(void) {}",
		Status:   DecompStatusOK,
	}
	close(in)

	written := <-doneCh
	if wErr := <-errCh; wErr != nil {
		t.Fatalf("RunBatchWriter: %v", wErr)
	}
	if written != 1 {
		t.Fatalf("written = %d, want 1", written)
	}

	var count int
	if err := store.DB().QueryRowContext(ctx, `SELECT count(*) FROM decomp_funcs WHERE func_ea = 0x7777`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected late in-flight record to be committed, got count=%d", count)
	}
}

func TestSQLiteDSN_HandlesSpecialPathCharactersHashAndPercent(t *testing.T) {
	ctx := context.Background()
	specialDir := filepath.Join(t.TempDir(), "c# dir %20")
	bronzeDBPath := filepath.Join(specialDir, "bronze.sqlite")
	silverPath := filepath.Join(specialDir, "silver_ghidra.sqlite")

	builder, err := NewAtomicBuilder(bronzeDBPath)
	if err != nil {
		t.Fatalf("NewAtomicBuilder in %q: %v", specialDir, err)
	}
	defer func() { _ = builder.Abort() }()

	if err := InitBronzeSchema(ctx, builder.DB()); err != nil {
		t.Fatalf("InitBronzeSchema: %v", err)
	}
	if err := builder.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if _, err := os.Stat(bronzeDBPath); err != nil {
		t.Fatalf("expected literal path %s to exist on disk: %v", bronzeDBPath, err)
	}

	roDB, err := OpenReadOnly(bronzeDBPath)
	if err != nil {
		t.Fatalf("OpenReadOnly in %q: %v", specialDir, err)
	}
	defer roDB.Close()

	var tblCnt int
	if err := roDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'meta'`).Scan(&tblCnt); err != nil || tblCnt != 1 {
		t.Fatalf("query bronze sqlite_master in special dir: cnt=%d err=%v", tblCnt, err)
	}

	store, err := OpenSilverStore(ctx, silverPath, "attila_32", "ghidra", "sha_special", "")
	if err != nil {
		t.Fatalf("OpenSilverStore in %q: %v", specialDir, err)
	}
	if err := store.WriteBatch(ctx, []DecompRecord{{
		FuncEA:   0x9000,
		FuncSize: 16,
		FuncName: "SpecialPathFunc",
		RawC:     "void SpecialPathFunc(void) {}",
		Status:   DecompStatusOK,
	}}); err != nil {
		t.Fatalf("WriteBatch in special dir: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close in special dir: %v", err)
	}
	if _, err := os.Stat(silverPath); err != nil {
		t.Fatalf("expected literal silver path %s on disk: %v", silverPath, err)
	}
}

func TestRenameWithRetry_OverwritesReadOnlyDestinationFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new.tmp")
	dst := filepath.Join(dir, "readonly.sqlite")

	if err := os.WriteFile(dst, []byte("old-readonly-data"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dst, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("new-committed-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := workspace.RenameWithRetry(src, dst); err != nil {
		t.Fatalf("RenameWithRetry over read-only destination failed: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-committed-data" {
		t.Fatalf("dst content = %q, want new-committed-data", string(got))
	}
}

func TestSilverStore_TimeoutDeferralAndRetryTimeouts(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "silver_ida.sqlite")

	store, err := OpenSilverStore(ctx, dbPath, "attila_64", "ida", "sha_timeout_test", "")
	if err != nil {
		t.Fatalf("OpenSilverStore: %v", err)
	}
	defer store.Close()

	specs := []FuncSpec{
		{EA: 0x1000, Size: 64, Name: "FastFn"},
		{EA: 0x2000, Size: 25329, Name: "SlowCtor"},
		{EA: 0x3000, Size: 128, Name: "BrokenFn"},
	}
	if err := store.WriteBatch(ctx, []DecompRecord{
		{FuncEA: 0x1000, FuncSize: 64, FuncName: "FastFn", RawC: "void FastFn() {}", Status: DecompStatusOK},
		{FuncEA: 0x2000, FuncSize: 25329, FuncName: "SlowCtor", Status: DecompStatusTimeout, ErrorText: "timeout"},
		{FuncEA: 0x3000, FuncSize: 128, FuncName: "BrokenFn", Status: DecompStatusFailed, ErrorText: "err"},
	}); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}

	pending, stats, err := store.PlanWork(ctx, specs, PlanOptions{})
	if err != nil {
		t.Fatalf("PlanWork default: %v", err)
	}
	if len(pending) != 0 || stats.Pending != 0 || stats.SkippedOK != 1 || stats.SkippedTimeout != 1 || stats.SkippedFailed != 1 {
		t.Fatalf("unexpected default PlanWork stats=%+v pending=%+v", stats, pending)
	}

	pendingTO, statsTO, err := store.PlanWork(ctx, specs, PlanOptions{RetryTimeouts: true})
	if err != nil {
		t.Fatalf("PlanWork RetryTimeouts: %v", err)
	}
	if len(pendingTO) != 1 || pendingTO[0].EA != 0x2000 || statsTO.Pending != 1 || statsTO.SkippedTimeout != 0 || statsTO.SkippedOK != 1 || statsTO.SkippedFailed != 1 {
		t.Fatalf("unexpected RetryTimeouts PlanWork stats=%+v pending=%+v", statsTO, pendingTO)
	}

	driftedSpecs := []FuncSpec{
		{EA: 0x1000, Size: 64, Name: "FastFn"},
		{EA: 0x2000, Size: 25400, Name: "SlowCtor"},
		{EA: 0x3000, Size: 128, Name: "BrokenFn"},
	}
	pendingDrift, statsDrift, err := store.PlanWork(ctx, driftedSpecs, PlanOptions{})
	if err != nil {
		t.Fatalf("PlanWork SizeChanged: %v", err)
	}
	if len(pendingDrift) != 1 || pendingDrift[0].EA != 0x2000 || statsDrift.SizeChanged != 1 {
		t.Fatalf("unexpected SizeChanged PlanWork stats=%+v pending=%+v", statsDrift, pendingDrift)
	}
}

func TestNewAtomicBuilderFromExisting_CommitAndAbort(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "bronze.sqlite")

	b1, err := NewAtomicBuilder(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitBronzeSchema(ctx, b1.DB()); err != nil {
		t.Fatal(err)
	}
	if _, err := b1.DB().ExecContext(ctx, `INSERT INTO names(name_id, value) VALUES (1, 'bronze_row');`); err != nil {
		t.Fatal(err)
	}
	if err := b1.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	bAbort, err := NewAtomicBuilderFromExisting(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bAbort.DB().ExecContext(ctx, `INSERT INTO names(name_id, value) VALUES (2, 'aborted_row');`); err != nil {
		t.Fatal(err)
	}
	if err := bAbort.Abort(); err != nil {
		t.Fatal(err)
	}

	bCommit, err := NewAtomicBuilderFromExisting(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bCommit.Abort() }()
	if _, err := bCommit.DB().ExecContext(ctx, `INSERT INTO names(name_id, value) VALUES (2, 'committed_row');`); err != nil {
		t.Fatal(err)
	}
	if err := bCommit.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	roDB, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer roDB.Close()

	var cnt int
	if err := roDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM names`).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 2 {
		t.Fatalf("names count = %d, want 2", cnt)
	}
}

type failingCloseStmtConnector struct {
	closeErr   error
	committed  *bool
	rolledBack *bool
}

func (c *failingCloseStmtConnector) Connect(context.Context) (driver.Conn, error) {
	return &failingCloseStmtConn{
		closeErr:   c.closeErr,
		committed:  c.committed,
		rolledBack: c.rolledBack,
	}, nil
}

func (c *failingCloseStmtConnector) Driver() driver.Driver { return nil }

type failingCloseStmtConn struct {
	closeErr   error
	committed  *bool
	rolledBack *bool
}

func (c *failingCloseStmtConn) Prepare(string) (driver.Stmt, error) {
	return &failingCloseStmt{closeErr: c.closeErr}, nil
}
func (c *failingCloseStmtConn) Close() error { return nil }
func (c *failingCloseStmtConn) Begin() (driver.Tx, error) {
	return &failingCloseStmtTx{committed: c.committed, rolledBack: c.rolledBack}, nil
}

type failingCloseStmt struct{ closeErr error }

func (s *failingCloseStmt) Close() error  { return s.closeErr }
func (s *failingCloseStmt) NumInput() int { return 0 }
func (s *failingCloseStmt) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}
func (s *failingCloseStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("unsupported")
}

type failingCloseStmtTx struct {
	committed  *bool
	rolledBack *bool
}

func (t *failingCloseStmtTx) Commit() error {
	*t.committed = true
	return nil
}
func (t *failingCloseStmtTx) Rollback() error {
	*t.rolledBack = true
	return nil
}

func TestTxStmtBatcher_FlushRollbackOnStmtCloseError(t *testing.T) {
	wantErr := errors.New("stmt close failed")
	committed := false
	rolledBack := false
	db := sql.OpenDB(&failingCloseStmtConnector{
		closeErr:   wantErr,
		committed:  &committed,
		rolledBack: &rolledBack,
	})
	defer db.Close()

	b := NewTxStmtBatcher(context.Background(), db, "INSERT INTO t VALUES (1)")
	defer func() { _ = b.Close() }()
	if err := b.Exec(); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	err := b.Flush()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Flush error = %v, want %v", err, wantErr)
	}
	if committed {
		t.Fatalf("Flush committed transaction despite stmt.Close error")
	}
	if !rolledBack {
		t.Fatalf("Flush did not rollback transaction on stmt.Close error")
	}
}

func TestMultiStmtTxBatcher_FlushRollbackOnStmtCloseError(t *testing.T) {
	wantErr := errors.New("multi stmt close failed")
	committed := false
	rolledBack := false
	db := sql.OpenDB(&failingCloseStmtConnector{
		closeErr:   wantErr,
		committed:  &committed,
		rolledBack: &rolledBack,
	})
	defer db.Close()

	b := NewMultiStmtTxBatcher(context.Background(), db, "INSERT INTO t1 VALUES (1)", "INSERT INTO t2 VALUES (2)")
	defer func() { _ = b.Close() }()
	if err := b.Exec(0); err != nil {
		t.Fatalf("Exec(0): %v", err)
	}
	if err := b.Exec(1); err != nil {
		t.Fatalf("Exec(1): %v", err)
	}
	err := b.Flush()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Flush error = %v, want %v", err, wantErr)
	}
	if committed {
		t.Fatalf("Flush committed transaction despite stmt.Close error")
	}
	if !rolledBack {
		t.Fatalf("Flush did not rollback transaction on stmt.Close error")
	}
}

func TestOpenSilverStore_TuningPragmas(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "silver_ghidra.sqlite")

	store, err := OpenSilverStore(ctx, dbPath, "attila_64", "ghidra", "sha_pragmas", "")
	if err != nil {
		t.Fatalf("OpenSilverStore: %v", err)
	}
	defer store.Close()

	var tempStore, cacheSize, walAutockpt int
	if err := store.DB().QueryRowContext(ctx, `PRAGMA temp_store;`).Scan(&tempStore); err != nil {
		t.Fatal(err)
	}
	if tempStore != 2 {
		t.Fatalf("PRAGMA temp_store = %d, want 2 (MEMORY)", tempStore)
	}
	if err := store.DB().QueryRowContext(ctx, `PRAGMA cache_size;`).Scan(&cacheSize); err != nil {
		t.Fatal(err)
	}
	if cacheSize != -131072 {
		t.Fatalf("PRAGMA cache_size = %d, want -131072", cacheSize)
	}
	if err := store.DB().QueryRowContext(ctx, `PRAGMA wal_autocheckpoint;`).Scan(&walAutockpt); err != nil {
		t.Fatal(err)
	}
	if walAutockpt != 10000 {
		t.Fatalf("PRAGMA wal_autocheckpoint = %d, want 10000", walAutockpt)
	}
}

func TestRunBatchWriter_FlushesByPayloadBytes(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "silver_ghidra.sqlite")

	store, err := OpenSilverStore(ctx, dbPath, "attila_64", "ghidra", "sha_byte_flush", "")
	if err != nil {
		t.Fatalf("OpenSilverStore: %v", err)
	}
	defer store.Close()

	in := make(chan DecompRecord, 4)
	doneCh := make(chan int, 1)
	errCh := make(chan error, 1)
	go func() {
		// Large batchSize and flushInterval ensure only the byte threshold can flush early.
		n, wErr := store.RunBatchWriter(ctx, in, 500, time.Hour)
		doneCh <- n
		errCh <- wErr
	}()

	chunk := strings.Repeat("a", (defaultSilverBatchBytes/2)+1024)
	in <- DecompRecord{FuncEA: 0x1000, FuncSize: 100, FuncName: "BigA", RawC: chunk, Status: DecompStatusOK}
	in <- DecompRecord{FuncEA: 0x2000, FuncSize: 100, FuncName: "BigB", RawC: chunk, Status: DecompStatusOK}

	ro, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer ro.Close()

	deadline := time.Now().Add(5 * time.Second)
	flushed := false
	for time.Now().Before(deadline) {
		var cnt int
		if qErr := ro.QueryRowContext(ctx, `SELECT COUNT(*) FROM decomp_funcs`).Scan(&cnt); qErr == nil && cnt == 2 {
			flushed = true
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	close(in)

	if wErr := <-errCh; wErr != nil {
		t.Fatalf("RunBatchWriter: %v", wErr)
	}
	if n := <-doneCh; n != 2 {
		t.Fatalf("written = %d, want 2", n)
	}
	if !flushed {
		t.Fatalf("expected RunBatchWriter to flush before channel close once payload exceeded %d bytes", defaultSilverBatchBytes)
	}
}

func TestSilverStore_ListFuncs(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "silver_ida.sqlite")

	store, err := OpenSilverStore(ctx, dbPath, "attila_32", "ida", "sha_list_funcs", "")
	if err != nil {
		t.Fatalf("OpenSilverStore: %v", err)
	}
	defer store.Close()

	// Empty store returns no rows and no error.
	empty, err := store.ListFuncs(ctx)
	if err != nil {
		t.Fatalf("ListFuncs empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListFuncs empty len = %d, want 0", len(empty))
	}

	// Insert out of EA order to verify ORDER BY ea ASC, flags, and nullable thunk_target_ea.
	input := []SilverFuncRow{
		{
			EA:            0x3000,
			Size:          16,
			Name:          "j_Target",
			Signature:     "void j_Target()",
			CallingConv:   "__cdecl",
			StackPurge:    0,
			IsThunk:       true,
			IsNoRet:       false,
			ThunkTargetEA: 0x1000,
		},
		{
			EA:            0x1000,
			Size:          64,
			Name:          "Target",
			Signature:     "int Target(void *this, int x)",
			CallingConv:   "__thiscall",
			StackPurge:    4,
			IsThunk:       false,
			IsNoRet:       false,
			ThunkTargetEA: 0, // stored as SQL NULL via NullUint64NonZero
		},
		{
			EA:            0x2000,
			Size:          32,
			Name:          "FatalAbort",
			Signature:     "void FatalAbort()",
			CallingConv:   "__stdcall",
			StackPurge:    0,
			IsThunk:       false,
			IsNoRet:       true,
			ThunkTargetEA: 0,
		},
	}
	if err := store.ReplaceFuncs(ctx, input, nil, "funcs"); err != nil {
		t.Fatalf("ReplaceFuncs: %v", err)
	}

	// Also insert explicit 0 and negative thunk_target_ea rows to verify <= 0 maps to 0.
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO silver_funcs(ea, size, name, signature, calling_conv, stack_purge, is_thunk, is_noret, thunk_target_ea)
		VALUES
			(?, 8, 'ZeroThunk', '', '__cdecl', 0, 1, 0, 0),
			(?, 12, 'NegThunk', '', '__cdecl', 0, 1, 0, -5)
	`, int64(0x0800), int64(0x2500)); err != nil {
		t.Fatalf("insert edge thunk_target_ea rows: %v", err)
	}

	got, err := store.ListFuncs(ctx)
	if err != nil {
		t.Fatalf("ListFuncs: %v", err)
	}

	want := []SilverFuncRow{
		{
			EA:            0x0800,
			Size:          8,
			Name:          "ZeroThunk",
			CallingConv:   "__cdecl",
			IsThunk:       true,
			ThunkTargetEA: 0,
		},
		{
			EA:            0x1000,
			Size:          64,
			Name:          "Target",
			Signature:     "int Target(void *this, int x)",
			CallingConv:   "__thiscall",
			StackPurge:    4,
			IsThunk:       false,
			IsNoRet:       false,
			ThunkTargetEA: 0,
		},
		{
			EA:            0x2000,
			Size:          32,
			Name:          "FatalAbort",
			Signature:     "void FatalAbort()",
			CallingConv:   "__stdcall",
			StackPurge:    0,
			IsThunk:       false,
			IsNoRet:       true,
			ThunkTargetEA: 0,
		},
		{
			EA:            0x2500,
			Size:          12,
			Name:          "NegThunk",
			CallingConv:   "__cdecl",
			IsThunk:       true,
			ThunkTargetEA: 0,
		},
		{
			EA:            0x3000,
			Size:          16,
			Name:          "j_Target",
			Signature:     "void j_Target()",
			CallingConv:   "__cdecl",
			StackPurge:    0,
			IsThunk:       true,
			IsNoRet:       false,
			ThunkTargetEA: 0x1000,
		},
	}

	if len(got) != len(want) {
		t.Fatalf("ListFuncs len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ListFuncs[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Canceled context must fail cleanly.
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.ListFuncs(canceledCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListFuncs(canceled) err = %v, want context.Canceled", err)
	}
}
