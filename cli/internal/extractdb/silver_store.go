package extractdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSilverBatchSize     = 500
	defaultSilverBatchBytes    = 4 << 20
	defaultSilverFlushInterval = 5 * time.Second
	silverDrainTimeout         = 1 * time.Second
	silverCheckpointTimeout    = 60 * time.Second
)

const silverSchemaDDL = `
CREATE TABLE IF NOT EXISTS meta(
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS silver_funcs(
	ea INTEGER PRIMARY KEY,
	size INTEGER NOT NULL,
	name TEXT NOT NULL,
	signature TEXT NOT NULL DEFAULT '',
	calling_conv TEXT NOT NULL DEFAULT '',
	stack_purge INTEGER NOT NULL DEFAULT 0,
	is_thunk INTEGER NOT NULL DEFAULT 0,
	is_noret INTEGER NOT NULL DEFAULT 0,
	thunk_target_ea INTEGER
) STRICT;

CREATE VIRTUAL TABLE IF NOT EXISTS silver_funcs_fts USING fts5(
	name,
	signature,
	content='silver_funcs',
	content_rowid='ea',
	tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS silver_func_ranges(
	start_ea INTEGER NOT NULL,
	end_ea INTEGER NOT NULL,
	func_ea INTEGER NOT NULL,
	PRIMARY KEY (start_ea, func_ea)
) STRICT;
CREATE INDEX IF NOT EXISTS idx_silver_func_ranges_func ON silver_func_ranges(func_ea);

CREATE TABLE IF NOT EXISTS silver_asm(
	func_ea INTEGER PRIMARY KEY,
	insn_count INTEGER NOT NULL,
	raw_asm TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS decomp_funcs(
	id INTEGER PRIMARY KEY,
	func_ea INTEGER NOT NULL UNIQUE,
	func_size INTEGER NOT NULL,
	func_name TEXT NOT NULL DEFAULT '',
	raw_c TEXT NOT NULL DEFAULT '',
	line_ea_json TEXT NOT NULL DEFAULT '[]',
	vars_json TEXT NOT NULL DEFAULT '[]',
	status TEXT NOT NULL,
	error_text TEXT NOT NULL DEFAULT '',
	elapsed_ms INTEGER NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_decomp_status ON decomp_funcs(status);

CREATE VIRTUAL TABLE IF NOT EXISTS decomp_fts USING fts5(
	func_name,
	raw_c,
	content='decomp_funcs',
	content_rowid='id',
	tokenize='unicode61'
);

CREATE TRIGGER IF NOT EXISTS decomp_funcs_ai AFTER INSERT ON decomp_funcs BEGIN
	INSERT INTO decomp_fts(rowid, func_name, raw_c) VALUES (new.id, new.func_name, new.raw_c);
END;

CREATE TRIGGER IF NOT EXISTS decomp_funcs_ad AFTER DELETE ON decomp_funcs BEGIN
	INSERT INTO decomp_fts(decomp_fts, rowid, func_name, raw_c) VALUES ('delete', old.id, old.func_name, old.raw_c);
END;

CREATE TRIGGER IF NOT EXISTS decomp_funcs_au AFTER UPDATE ON decomp_funcs BEGIN
	INSERT INTO decomp_fts(decomp_fts, rowid, func_name, raw_c) VALUES ('delete', old.id, old.func_name, old.raw_c);
	INSERT INTO decomp_fts(rowid, func_name, raw_c) VALUES (new.id, new.func_name, new.raw_c);
END;
`

const silverStringsSchemaDDL = `
CREATE TABLE IF NOT EXISTS silver_strings(
	str_id INTEGER PRIMARY KEY,
	ea INTEGER NOT NULL UNIQUE,
	encoding TEXT NOT NULL,
	length INTEGER NOT NULL,
	value TEXT NOT NULL,
	ref_count INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX IF NOT EXISTS idx_silver_strings_ea ON silver_strings(ea);

CREATE VIRTUAL TABLE IF NOT EXISTS silver_strings_fts USING fts5(
	value,
	content='silver_strings',
	content_rowid='str_id',
	tokenize='unicode61'
);
`

type SilverStringRow struct {
	EA       uint64
	Encoding string
	Length   int
	Value    string
	RefCount int
}

type SilverFuncRow struct {
	EA            uint64
	Size          uint64
	Name          string
	Signature     string
	CallingConv   string
	StackPurge    int
	IsThunk       bool
	IsNoRet       bool
	ThunkTargetEA uint64
}

type SilverFuncRangeRow struct {
	StartEA uint64
	EndEA   uint64
	FuncEA  uint64
}

type SilverAsmRow struct {
	FuncEA    uint64
	InsnCount int
	RawAsm    string
}

type FuncSpec struct {
	EA   uint64
	Size uint64
	Name string
}

type DecompRecord struct {
	FuncEA    uint64
	FuncSize  uint64
	FuncName  string
	RawC      string
	LineEA    string
	VarsJSON  string
	Status    string
	ErrorText string
	ElapsedMs int64
	UpdatedAt int64
}

type PlanOptions struct {
	RetryFailed   bool
	RetryTimeouts bool
	PruneOrphans  bool
}

type ResumeStats struct {
	Total          int
	SkippedOK      int
	SkippedFailed  int
	SkippedTimeout int
	SizeChanged    int
	OrphanDeleted  int
	Pending        int
}

type SilverStore struct {
	path string
	db   *sql.DB
}

func OpenSilverStore(ctx context.Context, path, target, engine, binarySHA256, enrichersKey string) (*SilverStore, error) {
	binarySHA256 = strings.TrimSpace(binarySHA256)
	if binarySHA256 == "" {
		return nil, errors.New("binarySHA256 must not be empty")
	}
	engine = strings.TrimSpace(engine)
	if engine == "" {
		return nil, errors.New("engine must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cleanPath := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0o755); err != nil {
		return nil, fmt.Errorf("ensure parent dir for %s: %w", cleanPath, err)
	}

	matches, err := checkSilverStore(ctx, cleanPath, engine, binarySHA256, enrichersKey)
	if err != nil {
		return nil, err
	}
	if !matches {
		if err := resetSilverStoreAtomic(ctx, cleanPath, target, engine, binarySHA256, enrichersKey); err != nil {
			return nil, err
		}
	}

	dsn := sqliteDSN(
		cleanPath,
		"_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)&_pragma=temp_store(MEMORY)&_pragma=cache_size(-131072)&_txlock=immediate",
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open silver store %s: %w", cleanPath, err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, `PRAGMA wal_autocheckpoint=10000;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure wal_autocheckpoint on %s: %w", cleanPath, err)
	}
	if matches {
		if _, err := db.ExecContext(ctx, silverStringsSchemaDDL); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("ensure silver_strings schema on %s: %w", cleanPath, err)
		}
	}

	return &SilverStore{
		path: cleanPath,
		db:   db,
	}, nil
}

func (s *SilverStore) DB() *sql.DB {
	return s.db
}

func (s *SilverStore) hasMetaNonEmpty(ctx context.Context, key string) (bool, error) {
	var ts string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&ts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(ts) != "", nil
}

func (s *SilverStore) HasFuncs(ctx context.Context) (bool, error) {
	return s.hasMetaNonEmpty(ctx, MetaFuncsExtractedAt)
}

func (s *SilverStore) HasAsm(ctx context.Context) (bool, error) {
	return s.hasMetaNonEmpty(ctx, MetaAsmExtractedAt)
}

func (s *SilverStore) HasStrings(ctx context.Context) (bool, error) {
	return s.hasMetaNonEmpty(ctx, MetaStringsExtractedAt)
}

func (s *SilverStore) FuncCount(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM silver_funcs`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *SilverStore) AsmCount(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM silver_asm`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *SilverStore) StringCount(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM silver_strings`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *SilverStore) ReplaceFuncs(ctx context.Context, funcs []SilverFuncRow, ranges []SilverFuncRangeRow, aspectsKey string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin silver_funcs tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM silver_func_ranges; DELETE FROM silver_funcs;`); err != nil {
		return fmt.Errorf("clear silver_funcs: %w", err)
	}

	if len(funcs) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO silver_funcs(
				ea, size, name, signature, calling_conv, stack_purge,
				is_thunk, is_noret, thunk_target_ea
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return fmt.Errorf("prepare silver_funcs insert: %w", err)
		}
		for _, f := range funcs {
			var thunkInt, noRetInt int
			if f.IsThunk {
				thunkInt = 1
			}
			if f.IsNoRet {
				noRetInt = 1
			}
			if _, err := stmt.ExecContext(
				ctx,
				int64(f.EA),
				int64(f.Size),
				f.Name,
				f.Signature,
				f.CallingConv,
				f.StackPurge,
				thunkInt,
				noRetInt,
				NullUint64NonZero(f.ThunkTargetEA),
			); err != nil {
				_ = stmt.Close()
				return fmt.Errorf("insert silver_func 0x%x: %w", f.EA, err)
			}
		}
		if err := stmt.Close(); err != nil {
			return fmt.Errorf("close silver_funcs stmt: %w", err)
		}
	}

	if len(ranges) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT OR IGNORE INTO silver_func_ranges(start_ea, end_ea, func_ea)
			VALUES (?, ?, ?)
		`)
		if err != nil {
			return fmt.Errorf("prepare silver_func_ranges insert: %w", err)
		}
		for _, rg := range ranges {
			if _, err := stmt.ExecContext(ctx, int64(rg.StartEA), int64(rg.EndEA), int64(rg.FuncEA)); err != nil {
				_ = stmt.Close()
				return fmt.Errorf("insert silver_func_range [0x%x,0x%x): %w", rg.StartEA, rg.EndEA, err)
			}
		}
		if err := stmt.Close(); err != nil {
			return fmt.Errorf("close silver_func_ranges stmt: %w", err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	const metaUpsert = `INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	for _, kv := range [][2]string{
		{MetaFuncsExtractedAt, now},
		{MetaExtractedAt, now},
		{MetaAspectsApplied, aspectsKey},
		{"count_silver_funcs", strconv.Itoa(len(funcs))},
		{"count_silver_func_ranges", strconv.Itoa(len(ranges))},
	} {
		if _, err := tx.ExecContext(ctx, metaUpsert, kv[0], kv[1]); err != nil {
			return fmt.Errorf("upsert silver meta %s: %w", kv[0], err)
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO silver_funcs_fts(silver_funcs_fts) VALUES('rebuild');`); err != nil {
		return fmt.Errorf("rebuild silver_funcs_fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ANALYZE;`); err != nil {
		return fmt.Errorf("analyze silver db: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit silver_funcs tx: %w", err)
	}
	return nil
}

func (s *SilverStore) ReplaceAsm(ctx context.Context, rows []SilverAsmRow, aspectsKey string) error {
	return s.ReplaceAsmStream(ctx, func(emit func(SilverAsmRow) error) error {
		for _, r := range rows {
			if err := emit(r); err != nil {
				return err
			}
		}
		return nil
	}, aspectsKey)
}

func (s *SilverStore) ReplaceAsmStream(ctx context.Context, produce func(emit func(SilverAsmRow) error) error, aspectsKey string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin silver_asm tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM silver_asm;`); err != nil {
		return fmt.Errorf("clear silver_asm: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO silver_asm(func_ea, insn_count, raw_asm)
		VALUES (?, ?, ?)
		ON CONFLICT(func_ea) DO UPDATE SET
			insn_count = excluded.insn_count,
			raw_asm = excluded.raw_asm
	`)
	if err != nil {
		return fmt.Errorf("prepare silver_asm insert: %w", err)
	}
	stmtClosed := false
	defer func() {
		if !stmtClosed {
			_ = stmt.Close()
		}
	}()

	count := 0
	if produce != nil {
		if err := produce(func(r SilverAsmRow) error {
			if _, err := stmt.ExecContext(ctx, int64(r.FuncEA), r.InsnCount, r.RawAsm); err != nil {
				return fmt.Errorf("insert silver_asm 0x%x: %w", r.FuncEA, err)
			}
			count++
			return nil
		}); err != nil {
			return err
		}
	}

	stmtClosed = true
	if err := stmt.Close(); err != nil {
		return fmt.Errorf("close silver_asm stmt: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	const metaUpsert = `INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	metaRows := [][2]string{
		{MetaAsmExtractedAt, now},
		{MetaExtractedAt, now},
		{"count_silver_asm", strconv.Itoa(count)},
	}
	if aspectsKey != "" {
		metaRows = append(metaRows, [2]string{MetaAspectsApplied, aspectsKey})
	}
	for _, kv := range metaRows {
		if _, err := tx.ExecContext(ctx, metaUpsert, kv[0], kv[1]); err != nil {
			return fmt.Errorf("upsert silver meta %s: %w", kv[0], err)
		}
	}

	if _, err := tx.ExecContext(ctx, `ANALYZE;`); err != nil {
		return fmt.Errorf("analyze silver db after asm: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit silver_asm tx: %w", err)
	}
	return nil
}

func (s *SilverStore) ReplaceStrings(ctx context.Context, rows []SilverStringRow, aspectsKey string) error {
	return s.ReplaceStringsStream(ctx, func(emit func(SilverStringRow) error) error {
		for _, r := range rows {
			if err := emit(r); err != nil {
				return err
			}
		}
		return nil
	}, aspectsKey)
}

func (s *SilverStore) ReplaceStringsStream(ctx context.Context, produce func(emit func(SilverStringRow) error) error, aspectsKey string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin silver_strings tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM silver_strings;`); err != nil {
		return fmt.Errorf("clear silver_strings: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO silver_strings(str_id, ea, encoding, length, value, ref_count)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare silver_strings insert: %w", err)
	}
	stmtClosed := false
	defer func() {
		if !stmtClosed {
			_ = stmt.Close()
		}
	}()

	var nextID int64 = 1
	seenEA := make(map[uint64]bool)
	if produce != nil {
		if err := produce(func(r SilverStringRow) error {
			if r.EA == 0 || seenEA[r.EA] {
				return nil
			}
			seenEA[r.EA] = true
			if _, err := stmt.ExecContext(ctx, nextID, int64(r.EA), r.Encoding, r.Length, r.Value, r.RefCount); err != nil {
				return fmt.Errorf("insert silver_string 0x%x: %w", r.EA, err)
			}
			nextID++
			return nil
		}); err != nil {
			return err
		}
	}

	stmtClosed = true
	if err := stmt.Close(); err != nil {
		return fmt.Errorf("close silver_strings stmt: %w", err)
	}

	count := int(nextID - 1)
	now := time.Now().UTC().Format(time.RFC3339)
	const metaUpsert = `INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	metaRows := [][2]string{
		{MetaStringsExtractedAt, now},
		{MetaExtractedAt, now},
		{"count_silver_strings", strconv.Itoa(count)},
	}
	if aspectsKey != "" {
		metaRows = append(metaRows, [2]string{MetaAspectsApplied, aspectsKey})
	}
	for _, kv := range metaRows {
		if _, err := tx.ExecContext(ctx, metaUpsert, kv[0], kv[1]); err != nil {
			return fmt.Errorf("upsert silver meta %s: %w", kv[0], err)
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO silver_strings_fts(silver_strings_fts) VALUES('rebuild');`); err != nil {
		return fmt.Errorf("rebuild silver_strings_fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ANALYZE;`); err != nil {
		return fmt.Errorf("analyze silver db after strings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit silver_strings tx: %w", err)
	}
	return nil
}

func (s *SilverStore) ListFuncs(ctx context.Context) ([]SilverFuncRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ea, size, name, signature, calling_conv, stack_purge, is_thunk, is_noret, thunk_target_ea
		FROM silver_funcs ORDER BY ea ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query silver_funcs: %w", err)
	}
	defer rows.Close()

	var out []SilverFuncRow
	for rows.Next() {
		var r SilverFuncRow
		var ea, sz int64
		var isThunk, isNoRet int
		var thunkTarget sql.NullInt64
		if err := rows.Scan(
			&ea, &sz, &r.Name, &r.Signature, &r.CallingConv,
			&r.StackPurge, &isThunk, &isNoRet, &thunkTarget,
		); err != nil {
			return nil, fmt.Errorf("scan silver_func: %w", err)
		}
		r.EA = uint64(ea)
		r.Size = uint64(sz)
		r.IsThunk = isThunk != 0
		r.IsNoRet = isNoRet != 0
		if thunkTarget.Valid && thunkTarget.Int64 > 0 {
			r.ThunkTargetEA = uint64(thunkTarget.Int64)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SilverStore) ListFuncSpecs(ctx context.Context) ([]FuncSpec, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ea, size, name FROM silver_funcs ORDER BY ea ASC`)
	if err != nil {
		return nil, fmt.Errorf("query silver_funcs specs: %w", err)
	}
	defer rows.Close()

	var specs []FuncSpec
	for rows.Next() {
		var ea, sz int64
		var name string
		if err := rows.Scan(&ea, &sz, &name); err != nil {
			return nil, fmt.Errorf("scan silver_func spec: %w", err)
		}
		specs = append(specs, FuncSpec{
			EA:   uint64(ea),
			Size: uint64(sz),
			Name: name,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return specs, nil
}

func (s *SilverStore) PlanWork(ctx context.Context, funcs []FuncSpec, opts PlanOptions) ([]FuncSpec, ResumeStats, error) {
	type cachedRow struct {
		size   uint64
		status string
	}

	rows, err := s.db.QueryContext(ctx, `SELECT func_ea, func_size, status FROM decomp_funcs`)
	if err != nil {
		return nil, ResumeStats{}, fmt.Errorf("query cached decomp funcs: %w", err)
	}
	cached := make(map[uint64]cachedRow)
	for rows.Next() {
		var ea, size int64
		var status string
		if err := rows.Scan(&ea, &size, &status); err != nil {
			_ = rows.Close()
			return nil, ResumeStats{}, fmt.Errorf("scan cached decomp func: %w", err)
		}
		cached[uint64(ea)] = cachedRow{
			size:   uint64(size),
			status: status,
		}
	}
	if err := rows.Close(); err != nil {
		return nil, ResumeStats{}, fmt.Errorf("close cached decomp rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, ResumeStats{}, fmt.Errorf("iterate cached decomp rows: %w", err)
	}

	validSet := make(map[uint64]struct{}, len(funcs))
	for _, f := range funcs {
		validSet[f.EA] = struct{}{}
	}

	var orphans []uint64
	if opts.PruneOrphans {
		for ea := range cached {
			if _, ok := validSet[ea]; !ok {
				orphans = append(orphans, ea)
			}
		}
	}

	if opts.PruneOrphans && len(orphans) > 0 {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, ResumeStats{}, fmt.Errorf("begin orphan cleanup tx: %w", err)
		}
		stmt, err := tx.PrepareContext(ctx, `DELETE FROM decomp_funcs WHERE func_ea = ?`)
		if err != nil {
			_ = tx.Rollback()
			return nil, ResumeStats{}, fmt.Errorf("prepare orphan delete: %w", err)
		}
		for _, ea := range orphans {
			if _, err := stmt.ExecContext(ctx, int64(ea)); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return nil, ResumeStats{}, fmt.Errorf("delete orphan func 0x%x: %w", ea, err)
			}
			delete(cached, ea)
		}
		_ = stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, ResumeStats{}, fmt.Errorf("commit orphan cleanup tx: %w", err)
		}
	}

	stats := ResumeStats{
		Total:         len(funcs),
		OrphanDeleted: len(orphans),
	}
	pending := make([]FuncSpec, 0, len(funcs))

	for _, spec := range funcs {
		row, exists := cached[spec.EA]
		if exists {
			if row.size != spec.Size {
				stats.SizeChanged++
				pending = append(pending, spec)
				continue
			}
			switch row.status {
			case DecompStatusOK:
				stats.SkippedOK++
				continue
			case DecompStatusTimeout:
				if !opts.RetryTimeouts {
					stats.SkippedTimeout++
					continue
				}
			case DecompStatusFailed:
				if !opts.RetryFailed {
					stats.SkippedFailed++
					continue
				}
			}
		}
		pending = append(pending, spec)
	}

	stats.Pending = len(pending)
	return pending, stats, nil
}

func (s *SilverStore) WriteBatch(ctx context.Context, records []DecompRecord) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin decomp batch tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const upsertSQL = `
INSERT INTO decomp_funcs(
	func_ea, func_size, func_name, raw_c,
	line_ea_json, vars_json, status, error_text, elapsed_ms, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(func_ea) DO UPDATE SET
	func_size = excluded.func_size,
	func_name = excluded.func_name,
	raw_c = excluded.raw_c,
	line_ea_json = excluded.line_ea_json,
	vars_json = excluded.vars_json,
	status = excluded.status,
	error_text = excluded.error_text,
	elapsed_ms = excluded.elapsed_ms,
	updated_at = excluded.updated_at;
`
	stmt, err := tx.PrepareContext(ctx, upsertSQL)
	if err != nil {
		return fmt.Errorf("prepare decomp batch upsert: %w", err)
	}
	defer stmt.Close()

	nowUnix := time.Now().Unix()
	for _, r := range records {
		lineEA := r.LineEA
		if lineEA == "" {
			lineEA = "[]"
		}
		varsJSON := r.VarsJSON
		if varsJSON == "" {
			varsJSON = "[]"
		}
		updatedAt := r.UpdatedAt
		if updatedAt == 0 {
			updatedAt = nowUnix
		}
		if _, err := stmt.ExecContext(
			ctx,
			int64(r.FuncEA),
			int64(r.FuncSize),
			r.FuncName,
			r.RawC,
			lineEA,
			varsJSON,
			r.Status,
			r.ErrorText,
			r.ElapsedMs,
			updatedAt,
		); err != nil {
			return fmt.Errorf("upsert decomp func 0x%x: %w", r.FuncEA, err)
		}
	}

	if err := stmt.Close(); err != nil {
		return fmt.Errorf("close decomp upsert stmt: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit decomp batch tx: %w", err)
	}
	return nil
}

func decompRecordPayloadBytes(r DecompRecord) int {
	return len(r.FuncName) + len(r.RawC) + len(r.LineEA) + len(r.VarsJSON) + len(r.ErrorText)
}

func (s *SilverStore) RunBatchWriter(ctx context.Context, in <-chan DecompRecord, batchSize int, flushInterval time.Duration) (int, error) {
	return s.RunBatchWriterWithCallback(ctx, in, batchSize, flushInterval, nil)
}

func (s *SilverStore) RunBatchWriterWithCallback(ctx context.Context, in <-chan DecompRecord, batchSize int, flushInterval time.Duration, onRecord func(DecompRecord)) (int, error) {
	if batchSize <= 0 {
		batchSize = defaultSilverBatchSize
	}
	if flushInterval <= 0 {
		flushInterval = defaultSilverFlushInterval
	}

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]DecompRecord, 0, batchSize)
	batchBytes := 0
	totalWritten := 0

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		// Run without a short per-batch deadline so large FTS5 merges or slow disk
		// checkpoints never abort a multi-hour decompilation run mid-commit.
		if err := s.WriteBatch(context.Background(), batch); err != nil {
			return err
		}
		totalWritten += len(batch)
		batch = batch[:0]
		batchBytes = 0
		return nil
	}

	appendRecord := func(rec DecompRecord) error {
		if onRecord != nil {
			onRecord(rec)
		}
		batch = append(batch, rec)
		batchBytes += decompRecordPayloadBytes(rec)
		if len(batch) >= batchSize || batchBytes >= defaultSilverBatchBytes {
			return flush()
		}
		return nil
	}

	for {
		select {
		case rec, ok := <-in:
			if !ok {
				if err := flush(); err != nil {
					return totalWritten, err
				}
				return totalWritten, nil
			}
			if err := appendRecord(rec); err != nil {
				return totalWritten, err
			}
		case <-ticker.C:
			if err := flush(); err != nil {
				return totalWritten, err
			}
		case <-ctx.Done():
			ticker.Stop()
			drainTimer := time.NewTimer(silverDrainTimeout)
			defer drainTimer.Stop()
			for {
				select {
				case rec, ok := <-in:
					if !ok {
						if err := flush(); err != nil {
							return totalWritten, err
						}
						return totalWritten, nil
					}
					if err := appendRecord(rec); err != nil {
						return totalWritten, err
					}
				case <-drainTimer.C:
					if err := flush(); err != nil {
						return totalWritten, err
					}
					return totalWritten, nil
				}
			}
		}
	}
}

func (s *SilverStore) StatusCounts(ctx context.Context) (map[string]int, error) {
	counts := make(map[string]int)
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM decomp_funcs GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("query decomp status counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		var cnt int
		if err := rows.Scan(&st, &cnt); err != nil {
			return nil, fmt.Errorf("scan decomp status count: %w", err)
		}
		counts[st] = cnt
	}
	return counts, rows.Err()
}

func (s *SilverStore) SetMeta(ctx context.Context, pairs ...[2]string) error {
	for _, kv := range pairs {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			kv[0], kv[1],
		); err != nil {
			return fmt.Errorf("upsert silver meta %s: %w", kv[0], err)
		}
	}
	return nil
}

func (s *SilverStore) Close() error {
	if s.db == nil {
		return nil
	}
	ckptCtx, cancel := context.WithTimeout(context.Background(), silverCheckpointTimeout)
	_, ckptErr := s.db.ExecContext(ckptCtx, `PRAGMA wal_checkpoint(TRUNCATE);`)
	if isBenignCheckpointErr(ckptErr) {
		ckptErr = nil
	}
	switchedToDelete := false
	if ckptErr == nil {
		if _, normErr := s.db.ExecContext(ckptCtx, `PRAGMA locking_mode=NORMAL; PRAGMA journal_mode=DELETE;`); normErr != nil {
			if !isBenignCheckpointErr(normErr) {
				ckptErr = normErr
			}
		} else {
			switchedToDelete = true
		}
	}
	cancel()

	closeErr := s.db.Close()
	s.db = nil
	if closeErr != nil {
		return closeErr
	}
	if switchedToDelete {
		_ = removeSQLiteSidecars(s.path)
	}
	return ckptErr
}

func checkSilverStore(ctx context.Context, path, engine, binarySHA256, enrichersKey string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat silver store %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("silver store %s is not a regular file", path)
	}
	if info.Size() == 0 {
		return false, nil
	}

	db, err := OpenReadOnly(path)
	if err != nil {
		return false, err
	}
	defer db.Close()

	metaVal := func(key string) (string, bool, error) {
		var v string
		err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
		if err != nil {
			if isReinitTriggerErr(err) {
				return "", true, nil
			}
			return "", false, fmt.Errorf("read silver meta %s (%s): %w", key, path, err)
		}
		return v, false, nil
	}

	storedEngine, reinit, err := metaVal(MetaSilverEngine)
	if err != nil || reinit {
		return false, err
	}
	storedSHA, reinit, err := metaVal(MetaBinarySHA256)
	if err != nil || reinit {
		return false, err
	}
	storedEnrichers, reinit, err := metaVal(MetaEnrichersApplied)
	if err != nil || reinit {
		return false, err
	}

	if storedEngine != engine ||
		!strings.EqualFold(storedSHA, binarySHA256) ||
		storedEnrichers != enrichersKey {
		return false, nil
	}
	return true, nil
}

func resetSilverStoreAtomic(ctx context.Context, path, target, engine, binarySHA256, enrichersKey string) error {
	builder, err := NewAtomicBuilder(path)
	if err != nil {
		return err
	}
	defer func() { _ = builder.Abort() }()

	if _, err := builder.DB().ExecContext(ctx, silverSchemaDDL); err != nil {
		return fmt.Errorf("init silver schema in %s: %w", path, err)
	}
	if _, err := builder.DB().ExecContext(ctx, silverStringsSchemaDDL); err != nil {
		return fmt.Errorf("init silver strings schema in %s: %w", path, err)
	}
	const metaInsert = `INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	for _, kv := range [][2]string{
		{MetaTarget, target},
		{MetaSilverEngine, engine},
		{MetaBinarySHA256, binarySHA256},
		{MetaEnrichersApplied, enrichersKey},
	} {
		if _, err := builder.DB().ExecContext(ctx, metaInsert, kv[0], kv[1]); err != nil {
			return fmt.Errorf("write silver meta %s: %w", kv[0], err)
		}
	}
	return builder.Commit(ctx)
}

func isReinitTriggerErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	var coder interface{ Code() int }
	if errors.As(err, &coder) {
		switch coder.Code() & 0xff {
		case 11, 26: // SQLITE_CORRUPT, SQLITE_NOTADB
			return true
		case 5, 6, 9: // SQLITE_BUSY, SQLITE_LOCKED, SQLITE_INTERRUPT
			return false
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "malformed") ||
		strings.Contains(msg, "not a database")
}

func isBenignCheckpointErr(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var coder interface{ Code() int }
	if errors.As(err, &coder) {
		switch coder.Code() & 0xff {
		case 5, 6: // SQLITE_BUSY, SQLITE_LOCKED
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "sqlite_busy")
}
