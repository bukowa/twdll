package extractdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestInspectStatus_UsesCachedMetaCountsAndFallsBackWhenMissing(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	const wantSHA = "deadbeef1234"

	// Bronze DB with cached meta counts that differ from empty tables, plus missing count_dwarf_funcs fallback.
	bronzePath := filepath.Join(tmp, "bronze.sqlite")
	b, err := NewAtomicBuilder(bronzePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Abort() }()
	if err := InitBronzeSchema(ctx, b.DB()); err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{
		{MetaBinarySHA256, wantSHA},
		{"count_sections", "7"},
		{"count_strings", "42"},
	} {
		if _, err := b.DB().ExecContext(ctx, `INSERT INTO meta(key, value) VALUES (?, ?)`, kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	bSum := InspectBronzeStatus(ctx, bronzePath)
	if !bSum.Present || bSum.Locked || bSum.Error != "" || bSum.BinarySHA256 != wantSHA {
		t.Fatalf("unexpected bronze summary: %+v", bSum)
	}
	if bSum.Sections != 7 || bSum.Strings != 42 || bSum.DwarfFuncs != 0 {
		t.Fatalf("expected meta counts 7/42 and fallback 0, got %d/%d/%d", bSum.Sections, bSum.Strings, bSum.DwarfFuncs)
	}

	// Silver DB with cached count_silver_funcs / count_silver_asm in meta while tables stay empty.
	silverPath := filepath.Join(tmp, "silver_ghidra.sqlite")
	store, err := OpenSilverStore(ctx, silverPath, "attila_64", "ghidra", wantSHA, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	rw, err := sql.Open("sqlite", silverPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{
		{"count_silver_funcs", "123"},
		{"count_silver_asm", "123"},
	} {
		if _, err := rw.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, kv[0], kv[1]); err != nil {
			_ = rw.Close()
			t.Fatal(err)
		}
	}
	_ = rw.Close()

	sSum := InspectSilverStatus(ctx, silverPath)
	if !sSum.Present || sSum.Locked || sSum.Error != "" || sSum.BinarySHA256 != wantSHA || sSum.Engine != "ghidra" {
		t.Fatalf("unexpected silver summary: %+v", sSum)
	}
	if sSum.Funcs != 123 || sSum.Asm != 123 {
		t.Fatalf("expected meta counts 123/123, got %d/%d", sSum.Funcs, sSum.Asm)
	}
}

func TestInspectStatus_DistinguishesLockTimeoutAndMissingMeta(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	// 1. Missing file reports Present=false without Error/Locked.
	missingSum := InspectBronzeStatus(ctx, filepath.Join(tmp, "missing.sqlite"))
	if missingSum.Present || missingSum.Locked || missingSum.Error != "" {
		t.Fatalf("expected missing DB to report Present=false, got %+v", missingSum)
	}

	// 2. DeadlineExceeded must NEVER classify as Locked.
	if IsSQLiteBusyErr(context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded not to classify as SQLite busy")
	}
	var locked bool
	var errMsg string
	classifyStatusDBErr(context.DeadlineExceeded, &locked, &errMsg)
	if locked || errMsg != "timeout" {
		t.Fatalf("expected locked=false, errMsg=timeout; got locked=%v errMsg=%q", locked, errMsg)
	}

	// 3. Real exclusive SQLite lock must set Locked=true (and not Error).
	lockedDBPath := filepath.Join(tmp, "locked.sqlite")
	store, err := OpenSilverStore(ctx, lockedDBPath, "attila_32", "ida", "sha1", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	locker, err := sql.Open("sqlite", lockedDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = locker.Close() }()
	lockConn, err := locker.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lockConn.Close() }()
	if _, err := lockConn.ExecContext(ctx, `BEGIN EXCLUSIVE`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = lockConn.ExecContext(ctx, `ROLLBACK`) }()

	sLocked := InspectSilverStatus(ctx, lockedDBPath)
	if !sLocked.Present || !sLocked.Locked || sLocked.Error != "" {
		t.Fatalf("expected Present=true Locked=true and empty Error on exclusive lock, got %+v", sLocked)
	}
	bLocked := InspectBronzeStatus(ctx, lockedDBPath)
	if !bLocked.Present || !bLocked.Locked || bLocked.Error != "" {
		t.Fatalf("expected Bronze Present=true Locked=true and empty Error on exclusive lock, got %+v", bLocked)
	}

	// 4. DB without meta table stays clean (neither Locked nor Error).
	emptyDBPath := filepath.Join(tmp, "empty.sqlite")
	emptyConn, err := sql.Open("sqlite", emptyDBPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = emptyConn.ExecContext(ctx, `CREATE TABLE dummy(id INTEGER)`)
	_ = emptyConn.Close()

	sEmpty := InspectSilverStatus(ctx, emptyDBPath)
	if !sEmpty.Present || sEmpty.Locked || sEmpty.Error != "" || sEmpty.BinarySHA256 != "" {
		t.Fatalf("expected missing meta to report clean uninitialized summary, got %+v", sEmpty)
	}
}
