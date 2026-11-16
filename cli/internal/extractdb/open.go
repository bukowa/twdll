package extractdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"twdll/cli/internal/workspace"
)

const buildPragmasQuery = "_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)&_pragma=locking_mode(EXCLUSIVE)&_pragma=foreign_keys(1)&_pragma=temp_store(MEMORY)"

var sqliteURIPathEscaper = strings.NewReplacer(
	"%", "%25",
	"?", "%3F",
	"#", "%23",
)

// OpenReadOnly opens an existing SQLite database in query-only mode.
// It verifies the file exists and is a regular file first so SQLite never
// creates an empty 0-byte database when queried before extraction.
func OpenReadOnly(path string) (*sql.DB, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat sqlite db %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("sqlite db %s is not a regular file", path)
	}

	dsn := sqliteDSN(path, "_pragma=query_only(1)&_pragma=busy_timeout(5000)&_pragma=temp_store(MEMORY)")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open readonly sqlite %s: %w", path, err)
	}
	return db, nil
}

// AtomicBuilder builds a disposable SQLite database in <destPath>.tmp with
// in-memory journaling and exclusive locking, then normalizes it into a single
// self-contained DELETE-mode file and renames it atomically to destPath on Commit.
type AtomicBuilder struct {
	destPath  string
	tmpPath   string
	db        *sql.DB
	committed bool
	aborted   bool
}

func NewAtomicBuilder(destPath string) (*AtomicBuilder, error) {
	cleanDest := filepath.Clean(destPath)
	if err := os.MkdirAll(filepath.Dir(cleanDest), 0o755); err != nil {
		return nil, fmt.Errorf("ensure parent dir for %s: %w", cleanDest, err)
	}

	tmpPath := cleanDest + ".tmp"
	if err := removeSQLiteFileAndSidecars(tmpPath); err != nil {
		return nil, fmt.Errorf("clean stale temp db %s: %w", tmpPath, err)
	}

	dsn := sqliteDSN(tmpPath, buildPragmasQuery)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open temp sqlite %s: %w", tmpPath, err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		_ = removeSQLiteFileAndSidecars(tmpPath)
		return nil, fmt.Errorf("initialize temp sqlite %s: %w", tmpPath, err)
	}

	return &AtomicBuilder{
		destPath: cleanDest,
		tmpPath:  tmpPath,
		db:       db,
	}, nil
}

func (b *AtomicBuilder) DB() *sql.DB {
	return b.db
}

func (b *AtomicBuilder) Commit(ctx context.Context) error {
	if b.committed {
		return nil
	}
	if b.aborted || b.db == nil {
		return errors.New("atomic builder already closed or aborted")
	}

	if _, err := b.db.ExecContext(ctx, `PRAGMA locking_mode=NORMAL; PRAGMA journal_mode=DELETE;`); err != nil {
		return fmt.Errorf("normalize sqlite journal mode on %s: %w", b.tmpPath, err)
	}
	if err := b.db.Close(); err != nil {
		b.db = nil
		return fmt.Errorf("close temp sqlite %s: %w", b.tmpPath, err)
	}
	b.db = nil

	if err := workspace.SyncFileWithRetry(b.tmpPath); err != nil {
		return fmt.Errorf("sync temp sqlite %s: %w", b.tmpPath, err)
	}

	if err := removeSQLiteSidecars(b.destPath); err != nil {
		return fmt.Errorf("clean stale sqlite sidecars for %s: %w", b.destPath, err)
	}

	if err := workspace.RenameWithRetry(b.tmpPath, b.destPath); err != nil {
		return err
	}

	_ = removeSQLiteSidecars(b.destPath)

	b.committed = true
	return nil
}

func (b *AtomicBuilder) Abort() error {
	if b.committed || b.aborted {
		return nil
	}
	b.aborted = true

	var firstErr error
	if b.db != nil {
		if err := b.db.Close(); err != nil {
			firstErr = err
		}
		b.db = nil
	}
	if err := removeSQLiteFileAndSidecars(b.tmpPath); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func sqliteDSN(path, query string) string {
	slashPath := filepath.ToSlash(filepath.Clean(path))
	escapedPath := sqliteURIPathEscaper.Replace(slashPath)
	prefix := "file:"
	if strings.HasPrefix(escapedPath, "//") {
		prefix = "file://"
	}
	if query == "" {
		return prefix + escapedPath
	}
	return prefix + escapedPath + "?" + query
}

func removeSQLiteSidecars(basePath string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.Remove(basePath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func removeSQLiteFileAndSidecars(basePath string) error {
	if err := os.Remove(basePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return removeSQLiteSidecars(basePath)
}
