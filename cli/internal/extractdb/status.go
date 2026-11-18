package extractdb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

const statusQueryTimeout = 5 * time.Second

// BronzeSummary holds lightweight readiness and row-count metadata for a bronze.sqlite store.
type BronzeSummary struct {
	Present      bool
	BinarySHA256 string
	Sections     int
	Strings      int
	DwarfFuncs   int
	Locked       bool
	Error        string
}

// SilverSummary holds lightweight readiness, row-count, and decompilation status totals for a silver_<engine>.sqlite store.
type SilverSummary struct {
	Present      bool
	BinarySHA256 string
	Engine       string
	Funcs        int
	Asm          int
	OK           int
	Failed       int
	Timeout      int
	Skipped      int
	Locked       bool
	Error        string
}

// InspectBronzeStatus reads bronze.sqlite readiness and cached counts without scanning large tables.
func InspectBronzeStatus(ctx context.Context, bronzePath string) BronzeSummary {
	var out BronzeSummary
	if ctx == nil {
		ctx = context.Background()
	}
	qCtx, cancel := context.WithTimeout(ctx, statusQueryTimeout)
	defer cancel()

	db, ok := openStatusDB(bronzePath, &out.Present, &out.Locked, &out.Error)
	if !ok {
		return out
	}
	defer func() { _ = db.Close() }()

	meta, err := readStatusMeta(qCtx, db)
	if err != nil {
		if !isSQLiteMissingTableErr(err) {
			classifyStatusDBErr(err, &out.Locked, &out.Error)
		}
		return out
	}
	out.BinarySHA256 = meta[MetaBinarySHA256]

	for _, item := range []struct {
		metaKey string
		table   string
		dst     *int
	}{
		{"count_sections", "sections", &out.Sections},
		{"count_strings", "strings", &out.Strings},
		{"count_dwarf_funcs", "dwarf_funcs", &out.DwarfFuncs},
	} {
		n, cErr := readStatusCount(qCtx, db, meta, item.metaKey, item.table)
		if cErr != nil {
			if !isSQLiteMissingTableErr(cErr) {
				classifyStatusDBErr(cErr, &out.Locked, &out.Error)
				return out
			}
			continue
		}
		*item.dst = n
	}
	return out
}

// InspectSilverStatus reads silver_<engine>.sqlite readiness, cached counts, and decompilation status totals.
func InspectSilverStatus(ctx context.Context, silverPath string) SilverSummary {
	var out SilverSummary
	if ctx == nil {
		ctx = context.Background()
	}
	qCtx, cancel := context.WithTimeout(ctx, statusQueryTimeout)
	defer cancel()

	db, ok := openStatusDB(silverPath, &out.Present, &out.Locked, &out.Error)
	if !ok {
		return out
	}
	defer func() { _ = db.Close() }()

	meta, err := readStatusMeta(qCtx, db)
	if err != nil {
		if !isSQLiteMissingTableErr(err) {
			classifyStatusDBErr(err, &out.Locked, &out.Error)
		}
		return out
	}
	out.BinarySHA256 = meta[MetaBinarySHA256]
	out.Engine = meta[MetaSilverEngine]

	for _, item := range []struct {
		metaKey string
		table   string
		dst     *int
	}{
		{"count_silver_funcs", "silver_funcs", &out.Funcs},
		{"count_silver_asm", "silver_asm", &out.Asm},
	} {
		n, cErr := readStatusCount(qCtx, db, meta, item.metaKey, item.table)
		if cErr != nil {
			if !isSQLiteMissingTableErr(cErr) {
				classifyStatusDBErr(cErr, &out.Locked, &out.Error)
				return out
			}
			continue
		}
		*item.dst = n
	}

	rows, err := db.QueryContext(qCtx, `SELECT status, COUNT(*) FROM decomp_funcs GROUP BY status`)
	if err != nil {
		if !isSQLiteMissingTableErr(err) {
			classifyStatusDBErr(err, &out.Locked, &out.Error)
		}
		return out
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var rowStatus string
		var cnt int
		if sErr := rows.Scan(&rowStatus, &cnt); sErr != nil {
			continue
		}
		switch rowStatus {
		case DecompStatusOK:
			out.OK += cnt
		case DecompStatusFailed:
			out.Failed += cnt
		case DecompStatusTimeout:
			out.Timeout += cnt
		default:
			out.Skipped += cnt
		}
	}
	if err := rows.Err(); err != nil {
		classifyStatusDBErr(err, &out.Locked, &out.Error)
	}
	return out
}

func openStatusDB(path string, present, locked *bool, errOut *string) (*sql.DB, bool) {
	db, err := OpenReadOnly(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false
		}
		*present = true
		classifyStatusDBErr(err, locked, errOut)
		return nil, false
	}
	*present = true
	db.SetMaxOpenConns(1)
	// Keep lock wait short so active writers report Locked promptly instead of hitting statusQueryTimeout.
	_, _ = db.Exec(`PRAGMA busy_timeout = 100`)
	return db, true
}

func readStatusMeta(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM meta`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	meta := make(map[string]string)
	for rows.Next() {
		var k, v string
		if sErr := rows.Scan(&k, &v); sErr != nil {
			return nil, sErr
		}
		meta[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return meta, nil
}

func readStatusCount(ctx context.Context, db *sql.DB, meta map[string]string, metaKey, table string) (int, error) {
	if raw, ok := meta[metaKey]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n >= 0 {
			return n, nil
		}
	}
	var cnt int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&cnt); err != nil {
		return 0, err
	}
	return cnt, nil
}

func classifyStatusDBErr(err error, locked *bool, errOut *string) {
	if err == nil {
		return
	}
	if IsSQLiteBusyErr(err) {
		*locked = true
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		*errOut = "timeout"
		return
	}
	*errOut = err.Error()
}

func isSQLiteMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such table")
}

// IsSQLiteBusyErr reports whether err represents a SQLite lock/busy condition (never context timeout/cancel).
func IsSQLiteBusyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	var coder interface{ Code() int }
	if errors.As(err, &coder) {
		switch coder.Code() & 0xff {
		case 5, 6: // SQLITE_BUSY, SQLITE_LOCKED
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "locked") || strings.Contains(msg, "busy")
}
