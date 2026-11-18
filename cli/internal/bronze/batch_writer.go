package bronze

import (
	"context"
	"database/sql"
	"fmt"

	"twdll/cli/internal/extractdb"
)

type txStmtBatcher = extractdb.TxStmtBatcher
type multiStmtTxBatcher = extractdb.MultiStmtTxBatcher

var (
	newTxStmtBatcher      = extractdb.NewTxStmtBatcher
	newMultiStmtTxBatcher = extractdb.NewMultiStmtTxBatcher
	nullInt64             = extractdb.NullInt64
)

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

type elfSymbolRow struct {
	SymIdx    int
	EA        uint64
	Size      uint64
	Name      string
	SymType   int
	SymBind   int
	SecIdx    int
	TableKind string
}

func insertELFSymbols(ctx context.Context, db *sql.DB, rows []elfSymbolRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT INTO elf_symbols(sym_idx, ea, size, name, sym_type, sym_bind, sec_idx, table_kind)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(r.SymIdx, int64(r.EA), int64(r.Size), r.Name, r.SymType, r.SymBind, r.SecIdx, r.TableKind); err != nil {
			return fmt.Errorf("insert elf symbol %s: %w", r.Name, err)
		}
	}
	return b.Flush()
}

type dwarfFuncRow struct {
	EA          uint64
	Size        uint64
	Name        string
	LinkageName string
	ClassName   string
	DeclFileID  sql.NullInt64
	DeclLine    int
	LowLine     int
	HighLine    int
}

func insertDwarfFuncs(ctx context.Context, db *sql.DB, rows []dwarfFuncRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT INTO dwarf_funcs(
			ea, size, name, linkage_name, class_name,
			decl_file_id, decl_line, low_line, high_line
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(
			int64(r.EA), int64(r.Size), r.Name, r.LinkageName, r.ClassName,
			r.DeclFileID, r.DeclLine, r.LowLine, r.HighLine,
		); err != nil {
			return fmt.Errorf("insert dwarf_func 0x%x: %w", r.EA, err)
		}
	}
	return b.Flush()
}

type dwarfFuncRangeRow struct {
	StartEA uint64
	EndEA   uint64
	FuncEA  uint64
}

func insertDwarfFuncRanges(ctx context.Context, db *sql.DB, rows []dwarfFuncRangeRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT OR IGNORE INTO dwarf_func_ranges(start_ea, end_ea, func_ea)
		VALUES (?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(int64(r.StartEA), int64(r.EndEA), int64(r.FuncEA)); err != nil {
			return fmt.Errorf("insert dwarf_func_range [0x%x, 0x%x): %w", r.StartEA, r.EndEA, err)
		}
	}
	return b.Flush()
}
