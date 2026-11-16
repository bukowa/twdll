package bronze

import (
	"context"
	"database/sql"
	"fmt"
)

const defaultBatchLimit = 16384

type txStmtBatcher struct {
	ctx   context.Context
	db    *sql.DB
	query string
	limit int
	tx    *sql.Tx
	stmt  *sql.Stmt
	count int
}

func newTxStmtBatcher(ctx context.Context, db *sql.DB, query string) *txStmtBatcher {
	return &txStmtBatcher{
		ctx:   ctx,
		db:    db,
		query: query,
		limit: defaultBatchLimit,
	}
}

func (b *txStmtBatcher) Exec(args ...any) error {
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

func (b *txStmtBatcher) Flush() error {
	if b.tx == nil {
		return nil
	}
	stmtErr := b.stmt.Close()
	txErr := b.tx.Commit()
	b.stmt = nil
	b.tx = nil
	b.count = 0
	if stmtErr != nil {
		return stmtErr
	}
	return txErr
}

func (b *txStmtBatcher) Close() error {
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

// multiStmtTxBatcher multiplexes multiple SQL statements over a single *sql.Tx so
// interleaved streaming writes never deadlock when db.SetMaxOpenConns(1) is active.
type multiStmtTxBatcher struct {
	ctx     context.Context
	db      *sql.DB
	queries []string
	limit   int
	tx      *sql.Tx
	stmts   []*sql.Stmt
	count   int
}

func newMultiStmtTxBatcher(ctx context.Context, db *sql.DB, queries ...string) *multiStmtTxBatcher {
	return &multiStmtTxBatcher{
		ctx:     ctx,
		db:      db,
		queries: queries,
		limit:   defaultBatchLimit,
		stmts:   make([]*sql.Stmt, len(queries)),
	}
}

func (m *multiStmtTxBatcher) Exec(stmtIdx int, args ...any) error {
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

func (m *multiStmtTxBatcher) Flush() error {
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
	if err := m.tx.Commit(); err != nil && firstErr == nil {
		firstErr = err
	}
	m.tx = nil
	m.count = 0
	return firstErr
}

func (m *multiStmtTxBatcher) Close() error {
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

type sectionRow struct {
	SecIdx      int
	Name        string
	VA          uint64
	VirtualSize uint64
	RawOffset   uint64
	RawSize     uint64
	Flags       uint32
}

func insertSections(ctx context.Context, db *sql.DB, rows []sectionRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT INTO sections(sec_idx, name, va, virtual_size, raw_offset, raw_size, flags)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(r.SecIdx, r.Name, int64(r.VA), int64(r.VirtualSize), int64(r.RawOffset), int64(r.RawSize), int64(r.Flags)); err != nil {
			return fmt.Errorf("insert section %s: %w", r.Name, err)
		}
	}
	return b.Flush()
}

type importRow struct {
	IATVA   uint64
	DLLName string
	SymName string
	Ordinal int
}

func insertImports(ctx context.Context, db *sql.DB, rows []importRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT OR REPLACE INTO imports(iat_va, dll_name, sym_name, ordinal)
		VALUES (?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(int64(r.IATVA), r.DLLName, r.SymName, r.Ordinal); err != nil {
			return fmt.Errorf("insert import 0x%x: %w", r.IATVA, err)
		}
	}
	return b.Flush()
}

type exportRow struct {
	ExportID int64
	EA       uint64
	Name     string
	Ordinal  int
}

func insertExports(ctx context.Context, db *sql.DB, rows []exportRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT INTO exports(export_id, ea, name, ordinal)
		VALUES (?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(r.ExportID, int64(r.EA), r.Name, r.Ordinal); err != nil {
			return fmt.Errorf("insert export %s: %w", r.Name, err)
		}
	}
	return b.Flush()
}

type vtableRow struct {
	VtableEA  uint64
	Slot      int
	TargetEA  uint64
	OwnerName string
}

func insertVtables(ctx context.Context, db *sql.DB, rows []vtableRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT OR REPLACE INTO vtables(vtable_ea, slot, target_ea, owner_name)
		VALUES (?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(int64(r.VtableEA), r.Slot, int64(r.TargetEA), r.OwnerName); err != nil {
			return fmt.Errorf("insert vtable 0x%x[%d]: %w", r.VtableEA, r.Slot, err)
		}
	}
	return b.Flush()
}

type luaRegRow struct {
	TableEA    uint64
	MethodName string
	FuncEA     uint64
}

func insertLuaRegs(ctx context.Context, db *sql.DB, rows []luaRegRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT OR IGNORE INTO lua_regs(table_ea, method_name, func_ea)
		VALUES (?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(int64(r.TableEA), r.MethodName, int64(r.FuncEA)); err != nil {
			return fmt.Errorf("insert lua_reg %s: %w", r.MethodName, err)
		}
	}
	return b.Flush()
}

type functionRow struct {
	EA            uint64
	Size          uint64
	Name          string
	MangledName   string
	ClassName     string
	Signature     string
	CallingConv   string
	CCSource      string
	StackPurge    int
	DiscoveryMask uint32
	ThunkTargetEA sql.NullInt64
	SourceFileID  sql.NullInt64
	DeclLine      int
	LowLine       int
	HighLine      int
	VtableEA      sql.NullInt64
	VtableSlot    sql.NullInt64
	Flags         uint32
}

func insertFunctions(ctx context.Context, db *sql.DB, rows []functionRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT INTO functions(
			ea, size, name, mangled_name, class_name, signature,
			calling_conv, cc_source, stack_purge, discovery_mask,
			thunk_target_ea, source_file_id, decl_line, low_line, high_line,
			vtable_ea, vtable_slot, flags
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(
			int64(r.EA), int64(r.Size), r.Name, r.MangledName, r.ClassName, r.Signature,
			r.CallingConv, r.CCSource, r.StackPurge, int64(r.DiscoveryMask),
			r.ThunkTargetEA, r.SourceFileID, r.DeclLine, r.LowLine, r.HighLine,
			r.VtableEA, r.VtableSlot, int64(r.Flags),
		); err != nil {
			return fmt.Errorf("insert function 0x%x: %w", r.EA, err)
		}
	}
	return b.Flush()
}

type funcRangeRow struct {
	StartEA uint64
	EndEA   uint64
	FuncEA  uint64
}

func insertFuncRanges(ctx context.Context, db *sql.DB, rows []funcRangeRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT OR IGNORE INTO func_ranges(start_ea, end_ea, func_ea)
		VALUES (?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(int64(r.StartEA), int64(r.EndEA), int64(r.FuncEA)); err != nil {
			return fmt.Errorf("insert func_range [0x%x, 0x%x): %w", r.StartEA, r.EndEA, err)
		}
	}
	return b.Flush()
}

type callSiteRow struct {
	CallEA   uint64
	CallerEA uint64
	CalleeEA sql.NullInt64
	CallKind string
	BaseReg  string
	MemDisp  sql.NullInt64
}

type dataXrefRow struct {
	FromEA     uint64
	FuncEA     uint64
	ToEA       uint64
	AccessKind string
}

type stringXrefRow struct {
	StrID  int64
	FuncEA uint64
}
