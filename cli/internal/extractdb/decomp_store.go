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

const DecompSchemaVersion = 1

const (
	defaultDecompBatchSize     = 250
	defaultDecompFlushInterval = 5 * time.Second
	defaultDecompWriteTimeout  = 60 * time.Second
	decompDrainTimeout         = 5 * time.Second
	decompCheckpointTimeout    = 12 * time.Second
)

const decompSchemaDDL = `
CREATE TABLE IF NOT EXISTS meta(
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS decomp_funcs(
	id INTEGER PRIMARY KEY,
	func_ea INTEGER NOT NULL,
	engine TEXT NOT NULL,
	func_size INTEGER NOT NULL,
	func_name TEXT NOT NULL DEFAULT '',
	raw_c TEXT NOT NULL DEFAULT '',
	line_ea_json TEXT NOT NULL DEFAULT '[]',
	decompiler_vars_json TEXT NOT NULL DEFAULT '[]',
	status TEXT NOT NULL,
	error_text TEXT NOT NULL DEFAULT '',
	elapsed_ms INTEGER NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL,
	UNIQUE(func_ea, engine)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_decomp_engine_status ON decomp_funcs(engine, status);

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

type FuncSpec struct {
	EA   uint64
	Size uint64
	Name string
}

type DecompRecord struct {
	FuncEA             uint64
	Engine             string
	FuncSize           uint64
	FuncName           string
	RawC               string
	LineEAJSON         string
	DecompilerVarsJSON string
	Status             string
	ErrorText          string
	ElapsedMs          int64
	UpdatedAt          int64
}

type PlanOptions struct {
	Force         bool
	RetryFailed   bool
	RetryTimeouts bool
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

type DecompStore struct {
	path string
	db   *sql.DB
}

func OpenDecompStore(ctx context.Context, path, target, binarySHA256 string) (*DecompStore, error) {
	binarySHA256 = strings.TrimSpace(binarySHA256)
	if binarySHA256 == "" {
		return nil, errors.New("binarySHA256 must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cleanPath := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0o755); err != nil {
		return nil, fmt.Errorf("ensure parent dir for %s: %w", cleanPath, err)
	}

	matches, err := checkDecompStore(ctx, cleanPath, binarySHA256)
	if err != nil {
		return nil, err
	}
	if !matches {
		if err := resetDecompStoreAtomic(ctx, cleanPath, target, binarySHA256); err != nil {
			return nil, err
		}
	}

	dsn := sqliteDSN(
		cleanPath,
		"_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)&_txlock=immediate",
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open decomp store %s: %w", cleanPath, err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, `PRAGMA wal_autocheckpoint=1000;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure wal_autocheckpoint on %s: %w", cleanPath, err)
	}

	return &DecompStore{
		path: cleanPath,
		db:   db,
	}, nil
}

func (s *DecompStore) DB() *sql.DB {
	return s.db
}

func (s *DecompStore) PlanWork(ctx context.Context, engine string, funcs []FuncSpec, opts PlanOptions) ([]FuncSpec, ResumeStats, error) {
	type cachedRow struct {
		size   uint64
		status string
	}

	rows, err := s.db.QueryContext(ctx, `SELECT func_ea, func_size, status FROM decomp_funcs WHERE engine = ?`, engine)
	if err != nil {
		return nil, ResumeStats{}, fmt.Errorf("query cached decomp funcs for %s: %w", engine, err)
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
	for ea := range cached {
		if _, ok := validSet[ea]; !ok {
			orphans = append(orphans, ea)
		}
	}

	if len(orphans) > 0 {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, ResumeStats{}, fmt.Errorf("begin orphan cleanup tx: %w", err)
		}
		stmt, err := tx.PrepareContext(ctx, `DELETE FROM decomp_funcs WHERE engine = ? AND func_ea = ?`)
		if err != nil {
			_ = tx.Rollback()
			return nil, ResumeStats{}, fmt.Errorf("prepare orphan delete: %w", err)
		}
		for _, ea := range orphans {
			if _, err := stmt.ExecContext(ctx, engine, int64(ea)); err != nil {
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
			if !opts.Force {
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
		}
		pending = append(pending, spec)
	}

	stats.Pending = len(pending)
	return pending, stats, nil
}

func (s *DecompStore) WriteBatch(ctx context.Context, records []DecompRecord) error {
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
	func_ea, engine, func_size, func_name, raw_c,
	line_ea_json, decompiler_vars_json, status, error_text, elapsed_ms, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(func_ea, engine) DO UPDATE SET
	func_size = excluded.func_size,
	func_name = excluded.func_name,
	raw_c = excluded.raw_c,
	line_ea_json = excluded.line_ea_json,
	decompiler_vars_json = excluded.decompiler_vars_json,
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
		lineEA := r.LineEAJSON
		if lineEA == "" {
			lineEA = "[]"
		}
		varsJSON := r.DecompilerVarsJSON
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
			r.Engine,
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
			return fmt.Errorf("upsert decomp func 0x%x (%s): %w", r.FuncEA, r.Engine, err)
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

func (s *DecompStore) RunBatchWriter(ctx context.Context, in <-chan DecompRecord, batchSize int, flushInterval time.Duration) (int, error) {
	if batchSize <= 0 {
		batchSize = defaultDecompBatchSize
	}
	if flushInterval <= 0 {
		flushInterval = defaultDecompFlushInterval
	}

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]DecompRecord, 0, batchSize)
	totalWritten := 0

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		writeCtx, cancel := context.WithTimeout(context.Background(), defaultDecompWriteTimeout)
		err := s.WriteBatch(writeCtx, batch)
		cancel()
		if err != nil {
			return err
		}
		totalWritten += len(batch)
		batch = batch[:0]
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
			batch = append(batch, rec)
			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					return totalWritten, err
				}
			}
		case <-ticker.C:
			if err := flush(); err != nil {
				return totalWritten, err
			}
		case <-ctx.Done():
			ticker.Stop()
			drainTimer := time.NewTimer(decompDrainTimeout)
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
					batch = append(batch, rec)
					if len(batch) >= batchSize {
						if err := flush(); err != nil {
							return totalWritten, err
						}
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

func (s *DecompStore) Close() error {
	if s.db == nil {
		return nil
	}
	ckptCtx, cancel := context.WithTimeout(context.Background(), decompCheckpointTimeout)
	_, ckptErr := s.db.ExecContext(ckptCtx, `PRAGMA wal_checkpoint(TRUNCATE);`)
	cancel()
	if isBenignCheckpointErr(ckptErr) {
		ckptErr = nil
	}

	closeErr := s.db.Close()
	s.db = nil
	if closeErr != nil {
		return closeErr
	}
	return ckptErr
}

func checkDecompStore(ctx context.Context, path, binarySHA256 string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat decomp store %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("decomp store %s is not a regular file", path)
	}
	if info.Size() == 0 {
		return false, nil
	}

	db, err := OpenReadOnly(path)
	if err != nil {
		return false, err
	}
	defer db.Close()

	var schemaVer, storedSHA string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&schemaVer); err != nil {
		if isReinitTriggerErr(err) {
			return false, nil
		}
		return false, fmt.Errorf("read decomp store schema_version %s: %w", path, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'binary_sha256'`).Scan(&storedSHA); err != nil {
		if isReinitTriggerErr(err) {
			return false, nil
		}
		return false, fmt.Errorf("read decomp store binary_sha256 %s: %w", path, err)
	}
	if schemaVer != strconv.Itoa(DecompSchemaVersion) || !strings.EqualFold(storedSHA, binarySHA256) {
		return false, nil
	}
	return true, nil
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

func resetDecompStoreAtomic(ctx context.Context, path, target, binarySHA256 string) error {
	builder, err := NewAtomicBuilder(path)
	if err != nil {
		return err
	}
	defer func() { _ = builder.Abort() }()

	if _, err := builder.DB().ExecContext(ctx, decompSchemaDDL); err != nil {
		return fmt.Errorf("init decomp schema in %s: %w", path, err)
	}
	const metaInsert = `INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	for _, kv := range [][2]string{
		{"schema_version", strconv.Itoa(DecompSchemaVersion)},
		{"target", target},
		{"binary_sha256", binarySHA256},
	} {
		if _, err := builder.DB().ExecContext(ctx, metaInsert, kv[0], kv[1]); err != nil {
			return fmt.Errorf("write decomp meta %s: %w", kv[0], err)
		}
	}
	return builder.Commit(ctx)
}
