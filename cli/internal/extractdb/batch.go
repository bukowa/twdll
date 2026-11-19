package extractdb

import (
	"context"
	"database/sql"
	"fmt"
)

const defaultBatchLimit = 16384

// TxStmtBatcher batches single-statement writes over SQLite transactions.
type TxStmtBatcher struct {
	ctx   context.Context
	db    *sql.DB
	query string
	limit int
	tx    *sql.Tx
	stmt  *sql.Stmt
	count int
}

func NewTxStmtBatcher(ctx context.Context, db *sql.DB, query string) *TxStmtBatcher {
	return &TxStmtBatcher{
		ctx:   ctx,
		db:    db,
		query: query,
		limit: defaultBatchLimit,
	}
}

func (b *TxStmtBatcher) Exec(args ...any) error {
	if b.tx == nil {
		tx, err := b.db.BeginTx(b.ctx, nil)
		if err != nil {
			return fmt.Errorf("begin batch tx: %w", err)
		}
		stmt, err := tx.PrepareContext(b.ctx, b.query)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("prepare batch stmt: %w", err)
		}
		b.tx = tx
		b.stmt = stmt
		b.count = 0
	}
	if _, err := b.stmt.ExecContext(b.ctx, args...); err != nil {
		return err
	}
	b.count++
	if b.count >= b.limit {
		return b.Flush()
	}
	return nil
}

func (b *TxStmtBatcher) Flush() error {
	if b.tx == nil {
		return nil
	}
	stmtErr := b.stmt.Close()
	b.stmt = nil
	tx := b.tx
	b.tx = nil
	b.count = 0
	if stmtErr != nil {
		_ = tx.Rollback()
		return stmtErr
	}
	return tx.Commit()
}

func (b *TxStmtBatcher) Close() error {
	if b.tx == nil {
		return nil
	}
	if b.stmt != nil {
		_ = b.stmt.Close()
		b.stmt = nil
	}
	err := b.tx.Rollback()
	b.tx = nil
	b.count = 0
	return err
}

// MultiStmtTxBatcher multiplexes multiple SQL statements over a single *sql.Tx so
// interleaved streaming writes never deadlock when db.SetMaxOpenConns(1) is active.
type MultiStmtTxBatcher struct {
	ctx     context.Context
	db      *sql.DB
	queries []string
	limit   int
	tx      *sql.Tx
	stmts   []*sql.Stmt
	count   int
}

func NewMultiStmtTxBatcher(ctx context.Context, db *sql.DB, queries ...string) *MultiStmtTxBatcher {
	return &MultiStmtTxBatcher{
		ctx:     ctx,
		db:      db,
		queries: queries,
		limit:   defaultBatchLimit,
		stmts:   make([]*sql.Stmt, len(queries)),
	}
}

func (m *MultiStmtTxBatcher) Exec(stmtIdx int, args ...any) error {
	if m.tx == nil {
		tx, err := m.db.BeginTx(m.ctx, nil)
		if err != nil {
			return fmt.Errorf("begin multi-batch tx: %w", err)
		}
		m.tx = tx
		m.count = 0
	}
	st := m.stmts[stmtIdx]
	if st == nil {
		var err error
		st, err = m.tx.PrepareContext(m.ctx, m.queries[stmtIdx])
		if err != nil {
			return fmt.Errorf("prepare multi-batch stmt[%d]: %w", stmtIdx, err)
		}
		m.stmts[stmtIdx] = st
	}
	if _, err := st.ExecContext(m.ctx, args...); err != nil {
		return err
	}
	m.count++
	if m.count >= m.limit {
		return m.Flush()
	}
	return nil
}

func (m *MultiStmtTxBatcher) Flush() error {
	if m.tx == nil {
		return nil
	}
	var firstErr error
	for i, st := range m.stmts {
		if st != nil {
			if err := st.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			m.stmts[i] = nil
		}
	}
	tx := m.tx
	m.tx = nil
	m.count = 0
	if firstErr != nil {
		_ = tx.Rollback()
		return firstErr
	}
	return tx.Commit()
}

func (m *MultiStmtTxBatcher) Close() error {
	if m.tx == nil {
		return nil
	}
	for i, st := range m.stmts {
		if st != nil {
			_ = st.Close()
			m.stmts[i] = nil
		}
	}
	err := m.tx.Rollback()
	m.tx = nil
	m.count = 0
	return err
}

func NullInt64(v int64, valid bool) sql.NullInt64 {
	if !valid {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

func NullUint64NonZero(v uint64) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(v), Valid: true}
}
