package extractdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"twdll/cli/internal/workspace"
)

func TestAtomicBuilder_CommitPurgesStaleSidecarsAndRebuildsFactsFTS(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "attila_32", "binary.sqlite")

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

	if err := InitFactsSchema(ctx, builder.DB()); err != nil {
		t.Fatalf("InitFactsSchema: %v", err)
	}

	if _, err := builder.DB().ExecContext(ctx, `
		INSERT INTO functions(ea, size, name, mangled_name, class_name)
		VALUES (0x10001000, 64, 'CA::Campaign::Tick', '?Tick@Campaign@CA@@QAEXXZ', 'CA::Campaign');
		INSERT INTO types(type_id, kind, name, byte_size)
		VALUES (1, 'class', 'CA::Campaign', 256);
		INSERT INTO strings(str_id, ea, encoding, value)
		VALUES (1, 0x10500000, 'ascii', 'lua_campaign_tick');
	`); err != nil {
		t.Fatalf("insert sample facts: %v", err)
	}

	if err := RebuildFactsFTS(ctx, builder.DB()); err != nil {
		t.Fatalf("RebuildFactsFTS: %v", err)
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
	if err := roDB.QueryRowContext(ctx, `SELECT rowid FROM functions_fts WHERE functions_fts MATCH 'Campaign'`).Scan(&matchedEA); err != nil {
		t.Fatalf("functions_fts MATCH: %v", err)
	}
	if matchedEA != 0x10001000 {
		t.Fatalf("functions_fts rowid = 0x%x, want 0x10001000", matchedEA)
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
	if _, err := roDB.ExecContext(ctx, `DELETE FROM functions;`); err == nil {
		t.Fatalf("expected write on OpenReadOnly connection to fail")
	}
}

func TestAtomicBuilder_AbortPreservesPriorDatabaseAndLeavesZeroTempFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "binary.sqlite")

	// 1. Commit an initial healthy database
	b1, err := NewAtomicBuilder(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitFactsSchema(ctx, b1.DB()); err != nil {
		t.Fatal(err)
	}
	if _, err := b1.DB().ExecContext(ctx, `INSERT INTO names(name_id, value) VALUES (1, 'healthy_symbol');`); err != nil {
		t.Fatal(err)
	}
	if err := b1.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// 2. Start a second build, write partial data, then Abort
	b2, err := NewAtomicBuilder(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitFactsSchema(ctx, b2.DB()); err != nil {
		t.Fatal(err)
	}
	if _, err := b2.DB().ExecContext(ctx, `INSERT INTO names(name_id, value) VALUES (1, 'corrupted_partial');`); err != nil {
		t.Fatal(err)
	}
	if err := b2.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	// Idempotent second Abort call
	if err := b2.Abort(); err != nil {
		t.Fatalf("second Abort: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "binary.sqlite" {
		t.Fatalf("expected only binary.sqlite in %s after Abort, got %v", dir, entries)
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

func TestDecompStore_CheckpointResumeUpsertFTSAndWALTruncate(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "decomp_ghidra.sqlite")

	store, err := OpenDecompStore(ctx, dbPath, "attila_32", "sha_v1")
	if err != nil {
		t.Fatalf("OpenDecompStore: %v", err)
	}

	initialSpecs := []FuncSpec{
		{EA: 0x1000, Size: 32, Name: "FuncAlpha"},
		{EA: 0x2000, Size: 48, Name: "FuncBeta"},
		{EA: 0x3000, Size: 64, Name: "FuncOrphan"},
	}

	pending, stats, err := store.PlanWork(ctx, "ghidra", initialSpecs, PlanOptions{})
	if err != nil {
		t.Fatalf("initial PlanWork: %v", err)
	}
	if len(pending) != 3 || stats.Pending != 3 || stats.Total != 3 {
		t.Fatalf("unexpected initial plan: pending=%d stats=%+v", len(pending), stats)
	}

	// Stream 3 records through RunBatchWriter: 2 via batchSize=2, 1 via timer flush
	ch := make(chan DecompRecord, 3)
	ch <- DecompRecord{
		FuncEA:   0x1000,
		Engine:   "ghidra",
		FuncSize: 32,
		FuncName: "FuncAlpha",
		RawC:     "void FuncAlpha(void) { int alpha_magic = 1; }",
		Status:   "ok",
	}
	ch <- DecompRecord{
		FuncEA:    0x2000,
		Engine:    "ghidra",
		FuncSize:  48,
		FuncName:  "FuncBeta",
		RawC:      "",
		Status:    "failed",
		ErrorText: "decompile timeout",
	}
	ch <- DecompRecord{
		FuncEA:   0x3000,
		Engine:   "ghidra",
		FuncSize: 64,
		FuncName: "FuncOrphan",
		RawC:     "void FuncOrphan(void) { int orphan_token = 99; }",
		Status:   "ok",
	}
	close(ch)

	written, err := store.RunBatchWriter(ctx, ch, 2, 15*time.Millisecond)
	if err != nil {
		t.Fatalf("RunBatchWriter: %v", err)
	}
	if written != 3 {
		t.Fatalf("written = %d, want 3", written)
	}

	// Next pass:
	// - 0x1000 has drifted size (32 -> 40) -> SizeChanged
	// - 0x2000 is failed -> skipped when RetryFailed=false, queued when RetryFailed=true
	// - 0x3000 is removed -> OrphanDeleted
	// - 0x4000 is brand new -> Pending
	nextSpecs := []FuncSpec{
		{EA: 0x1000, Size: 40, Name: "FuncAlpha"},
		{EA: 0x2000, Size: 48, Name: "FuncBeta"},
		{EA: 0x4000, Size: 16, Name: "FuncGamma"},
	}

	pendingNoRetry, statsNoRetry, err := store.PlanWork(ctx, "ghidra", nextSpecs, PlanOptions{RetryFailed: false})
	if err != nil {
		t.Fatalf("PlanWork(RetryFailed=false): %v", err)
	}
	if statsNoRetry.OrphanDeleted != 1 || statsNoRetry.SizeChanged != 1 || statsNoRetry.SkippedFailed != 1 || len(pendingNoRetry) != 2 {
		t.Fatalf("unexpected statsNoRetry=%+v pending=%v", statsNoRetry, pendingNoRetry)
	}

	// Verify RetryFailed=true queues 0x2000 while keeping 0x1000 (size changed) and 0x4000 (missing)
	pendingRetry, statsRetry, err := store.PlanWork(ctx, "ghidra", nextSpecs, PlanOptions{RetryFailed: true})
	if err != nil {
		t.Fatalf("PlanWork(RetryFailed=true): %v", err)
	}
	if statsRetry.OrphanDeleted != 0 || statsRetry.SizeChanged != 1 || statsRetry.SkippedFailed != 0 || len(pendingRetry) != 3 {
		t.Fatalf("unexpected statsRetry=%+v pending=%v", statsRetry, pendingRetry)
	}

	// Upsert 0x2000 from failed -> ok, update 0x1000 size=40, insert 0x4000 under a cancelled parent ctx to verify drain safety
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	ch2 := make(chan DecompRecord, 3)
	ch2 <- DecompRecord{
		FuncEA:   0x1000,
		Engine:   "ghidra",
		FuncSize: 40,
		FuncName: "FuncAlpha",
		RawC:     "void FuncAlpha(void) { int alpha_v2 = 2; }",
		Status:   "ok",
	}
	ch2 <- DecompRecord{
		FuncEA:   0x2000,
		Engine:   "ghidra",
		FuncSize: 48,
		FuncName: "FuncBeta",
		RawC:     "void FuncBeta(void) { int beta_recovered = 42; }",
		Status:   "ok",
	}
	ch2 <- DecompRecord{
		FuncEA:   0x4000,
		Engine:   "ghidra",
		FuncSize: 16,
		FuncName: "FuncGamma",
		RawC:     "void FuncGamma(void) {}",
		Status:   "ok",
	}
	close(ch2)

	written2, err := store.RunBatchWriter(cancelCtx, ch2, 250, time.Second)
	if err != nil {
		t.Fatalf("RunBatchWriter on cancelled ctx: %v", err)
	}
	if written2 != 3 {
		t.Fatalf("expected all 3 queued records drained on cancelled ctx, got %d", written2)
	}

	// Verify all 3 are now SkippedOK
	pendingFinal, statsFinal, err := store.PlanWork(ctx, "ghidra", nextSpecs, PlanOptions{})
	if err != nil {
		t.Fatalf("final PlanWork: %v", err)
	}
	if len(pendingFinal) != 0 || statsFinal.SkippedOK != 3 {
		t.Fatalf("expected all 3 specs SkippedOK, got pending=%v stats=%+v", pendingFinal, statsFinal)
	}

	// Verify FTS5 reflects failed->ok upsert, updated 0x1000, and deleted orphan 0x3000
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

	// Verify FTS5 and DB integrity
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

	if walInfo, err := os.Stat(dbPath + "-wal"); err == nil && walInfo.Size() > 0 {
		t.Fatalf("expected Close() to truncate WAL to 0 bytes (or remove it), got %d bytes", walInfo.Size())
	}

	// Opening with a different binary SHA-256 resets the store cleanly
	storeV2, err := OpenDecompStore(ctx, dbPath, "attila_32", "sha_v2")
	if err != nil {
		t.Fatalf("OpenDecompStore(sha_v2): %v", err)
	}
	defer storeV2.Close()

	if err := storeV2.DB().QueryRowContext(ctx, `SELECT count(*) FROM decomp_funcs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected binary SHA drift to reset decomp_funcs, got %d rows", count)
	}
}

func TestDecompStore_ConcurrentReadOnlyFTSWhileWritingBatches(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "decomp_ghidra.sqlite")

	store, err := OpenDecompStore(ctx, dbPath, "attila_32", "sha_concurrent")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed one record so readers have data from the start
	if err := store.WriteBatch(ctx, []DecompRecord{{
		FuncEA:   0x100,
		Engine:   "ghidra",
		FuncSize: 16,
		FuncName: "SeedFunc",
		RawC:     "void SeedFunc(void) { int warscape_token = 1; }",
		Status:   "ok",
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

	// Launch concurrent readers querying FTS while writer commits batches
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
			Engine:   "ghidra",
			FuncSize: 32,
			FuncName: fmt.Sprintf("Sub_%d", i),
			RawC:     fmt.Sprintf("void Sub_%d(void) { int warscape_token = %d; }", i, i),
			Status:   "ok",
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
		Sources:      []string{"pe", "ghidra"},
		Counts:       map[string]int{"functions": 1200, "strings": 450},
	}
	if err := WriteManifestAtomic(path, want); err != nil {
		t.Fatalf("WriteManifestAtomic: %v", err)
	}
	got, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if got.SchemaVersion != ManifestSchemaVersion || got.FactsSchemaVersion != FactsSchemaVersion {
		t.Fatalf("unexpected default schema versions: %+v", got)
	}
	if got.Target != want.Target || got.BinarySHA256 != want.BinarySHA256 || got.Counts["functions"] != 1200 {
		t.Fatalf("manifest mismatch: got %+v, want %+v", got, want)
	}
}

func TestOpenDecompStore_PreservesExistingCacheOnCancelledCtxOrEmptySHA(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "decomp_ghidra.sqlite")

	store, err := OpenDecompStore(ctx, dbPath, "attila_32", "sha_keep_me")
	if err != nil {
		t.Fatalf("OpenDecompStore: %v", err)
	}
	if err := store.WriteBatch(ctx, []DecompRecord{{
		FuncEA:   0x5000,
		Engine:   "ghidra",
		FuncSize: 24,
		FuncName: "PreciousFunc",
		RawC:     "void PreciousFunc(void) {}",
		Status:   "ok",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// 1. Empty binarySHA256 must fail fast without resetting existing cache
	if _, err := OpenDecompStore(ctx, dbPath, "attila_32", "   "); err == nil {
		t.Fatalf("expected OpenDecompStore with empty binarySHA256 to fail")
	}

	// 2. Cancelled ctx must return error without resetting existing cache
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := OpenDecompStore(cancelCtx, dbPath, "attila_32", "sha_keep_me"); err == nil {
		t.Fatalf("expected OpenDecompStore with cancelled ctx to fail")
	}

	// Verify PreciousFunc is still intact
	reopened, err := OpenDecompStore(ctx, dbPath, "attila_32", "sha_keep_me")
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
	dbPath := filepath.Join(t.TempDir(), "decomp_ida.sqlite")

	store, err := OpenDecompStore(ctx, dbPath, "attila_32", "sha_drain")
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

	// Cancel context first, then simulate an in-flight worker finishing and sending its record before closing `in`
	cancel()
	time.Sleep(20 * time.Millisecond)
	in <- DecompRecord{
		FuncEA:   0x7777,
		Engine:   "ida",
		FuncSize: 32,
		FuncName: "LateInFlightWorker",
		RawC:     "void LateInFlightWorker(void) {}",
		Status:   "ok",
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
	binaryDBPath := filepath.Join(specialDir, "binary.sqlite")
	decompPath := filepath.Join(specialDir, "decomp_ghidra.sqlite")

	builder, err := NewAtomicBuilder(binaryDBPath)
	if err != nil {
		t.Fatalf("NewAtomicBuilder in %q: %v", specialDir, err)
	}
	defer func() { _ = builder.Abort() }()

	if err := InitFactsSchema(ctx, builder.DB()); err != nil {
		t.Fatalf("InitFactsSchema: %v", err)
	}
	if err := builder.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if _, err := os.Stat(binaryDBPath); err != nil {
		t.Fatalf("expected literal path %s to exist on disk: %v", binaryDBPath, err)
	}

	roDB, err := OpenReadOnly(binaryDBPath)
	if err != nil {
		t.Fatalf("OpenReadOnly in %q: %v", specialDir, err)
	}
	defer roDB.Close()

	var ver string
	if err := roDB.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&ver); err != nil {
		t.Fatalf("query facts meta in special dir: %v", err)
	}

	store, err := OpenDecompStore(ctx, decompPath, "attila_32", "sha_special")
	if err != nil {
		t.Fatalf("OpenDecompStore in %q: %v", specialDir, err)
	}
	if err := store.WriteBatch(ctx, []DecompRecord{{
		FuncEA:   0x9000,
		Engine:   "ghidra",
		FuncSize: 16,
		FuncName: "SpecialPathFunc",
		RawC:     "void SpecialPathFunc(void) {}",
		Status:   "ok",
	}}); err != nil {
		t.Fatalf("WriteBatch in special dir: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close in special dir: %v", err)
	}
	if _, err := os.Stat(decompPath); err != nil {
		t.Fatalf("expected literal decomp path %s on disk: %v", decompPath, err)
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

func TestDecompStore_TimeoutDeferralAndRetryTimeouts(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "decomp_ida.sqlite")

	store, err := OpenDecompStore(ctx, dbPath, "attila_64", "sha_timeout_test")
	if err != nil {
		t.Fatalf("OpenDecompStore: %v", err)
	}
	defer store.Close()

	specs := []FuncSpec{
		{EA: 0x1000, Size: 64, Name: "FastFn"},
		{EA: 0x2000, Size: 25329, Name: "SlowCtor"},
		{EA: 0x3000, Size: 128, Name: "BrokenFn"},
	}
	if err := store.WriteBatch(ctx, []DecompRecord{
		{FuncEA: 0x1000, Engine: "ida", FuncSize: 64, FuncName: "FastFn", RawC: "void FastFn() {}", Status: DecompStatusOK},
		{FuncEA: 0x2000, Engine: "ida", FuncSize: 25329, FuncName: "SlowCtor", Status: DecompStatusTimeout, ErrorText: "timeout"},
		{FuncEA: 0x3000, Engine: "ida", FuncSize: 128, FuncName: "BrokenFn", Status: DecompStatusFailed, ErrorText: "err"},
	}); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}

	// Default resume skips ok, timeout, and failed.
	pending, stats, err := store.PlanWork(ctx, "ida", specs, PlanOptions{})
	if err != nil {
		t.Fatalf("PlanWork default: %v", err)
	}
	if len(pending) != 0 || stats.Pending != 0 || stats.SkippedOK != 1 || stats.SkippedTimeout != 1 || stats.SkippedFailed != 1 {
		t.Fatalf("unexpected default PlanWork stats=%+v pending=%+v", stats, pending)
	}

	// RetryTimeouts=true queues only the timeout row.
	pendingTO, statsTO, err := store.PlanWork(ctx, "ida", specs, PlanOptions{RetryTimeouts: true})
	if err != nil {
		t.Fatalf("PlanWork RetryTimeouts: %v", err)
	}
	if len(pendingTO) != 1 || pendingTO[0].EA != 0x2000 || statsTO.Pending != 1 || statsTO.SkippedTimeout != 0 || statsTO.SkippedOK != 1 || statsTO.SkippedFailed != 1 {
		t.Fatalf("unexpected RetryTimeouts PlanWork stats=%+v pending=%+v", statsTO, pendingTO)
	}

	// Force=true queues all rows.
	pendingForce, statsForce, err := store.PlanWork(ctx, "ida", specs, PlanOptions{Force: true})
	if err != nil {
		t.Fatalf("PlanWork Force: %v", err)
	}
	if len(pendingForce) != 3 || statsForce.Pending != 3 {
		t.Fatalf("unexpected Force PlanWork stats=%+v pending=%+v", statsForce, pendingForce)
	}

	// Changing FuncSize on a timeout row re-queues it even when RetryTimeouts=false.
	driftedSpecs := []FuncSpec{
		{EA: 0x1000, Size: 64, Name: "FastFn"},
		{EA: 0x2000, Size: 25400, Name: "SlowCtor"},
		{EA: 0x3000, Size: 128, Name: "BrokenFn"},
	}
	pendingDrift, statsDrift, err := store.PlanWork(ctx, "ida", driftedSpecs, PlanOptions{})
	if err != nil {
		t.Fatalf("PlanWork SizeChanged: %v", err)
	}
	if len(pendingDrift) != 1 || pendingDrift[0].EA != 0x2000 || statsDrift.SizeChanged != 1 {
		t.Fatalf("unexpected SizeChanged PlanWork stats=%+v pending=%+v", statsDrift, pendingDrift)
	}
}

func TestNewAtomicBuilderFromExisting_CommitAndAbort(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "binary.sqlite")

	b1, err := NewAtomicBuilder(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitFactsSchema(ctx, b1.DB()); err != nil {
		t.Fatal(err)
	}
	if _, err := b1.DB().ExecContext(ctx, `INSERT INTO names(name_id, value) VALUES (1, 'bronze_row');`); err != nil {
		t.Fatal(err)
	}
	if err := b1.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Abort must leave original intact.
	bAbort, err := NewAtomicBuilderFromExisting(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bAbort.DB().ExecContext(ctx, `INSERT INTO names(name_id, value) VALUES (2, 'aborted_silver');`); err != nil {
		t.Fatal(err)
	}
	if err := bAbort.Abort(); err != nil {
		t.Fatal(err)
	}

	// Commit must retain bronze_row and add committed_silver.
	bCommit, err := NewAtomicBuilderFromExisting(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bCommit.Abort() }()
	if _, err := bCommit.DB().ExecContext(ctx, `INSERT INTO names(name_id, value) VALUES (2, 'committed_silver');`); err != nil {
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
