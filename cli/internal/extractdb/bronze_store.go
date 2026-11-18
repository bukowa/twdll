package extractdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// BronzeStore wraps an AtomicBuilder to provide typed, SQL-encapsulated writes
// to a temporary bronze.sqlite database before atomic commit.
type BronzeStore struct {
	builder *AtomicBuilder
}

// NewBronzeStore creates a temporary bronze database at destPath and initializes its schema.
func NewBronzeStore(ctx context.Context, destPath string) (*BronzeStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	builder, err := NewAtomicBuilder(destPath)
	if err != nil {
		return nil, err
	}
	if err := InitBronzeSchema(ctx, builder.DB()); err != nil {
		_ = builder.Abort()
		return nil, err
	}
	return &BronzeStore{builder: builder}, nil
}

// Abort rolls back and deletes the temporary bronze database if not already committed.
func (s *BronzeStore) Abort() error {
	if s == nil || s.builder == nil {
		return nil
	}
	return s.builder.Abort()
}

// FinalizeAndCommit writes meta pairs, refreshes cached table counts, rebuilds FTS5
// indexes, runs ANALYZE, and atomically commits the bronze database to its destination path.
func (s *BronzeStore) FinalizeAndCommit(ctx context.Context, meta [][2]string) (map[string]int64, error) {
	if s == nil || s.builder == nil || s.builder.DB() == nil {
		return nil, errors.New("bronze store already closed or aborted")
	}
	db := s.builder.DB()
	const metaUpsert = `INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	for _, kv := range meta {
		if _, err := db.ExecContext(ctx, metaUpsert, kv[0], kv[1]); err != nil {
			return nil, fmt.Errorf("store bronze meta %s: %w", kv[0], err)
		}
	}

	counts, err := RefreshBronzeCounts(ctx, db)
	if err != nil {
		return nil, err
	}
	if err := RebuildBronzeFTS(ctx, db); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `ANALYZE;`); err != nil {
		return nil, fmt.Errorf("analyze bronze db: %w", err)
	}
	if err := s.builder.Commit(ctx); err != nil {
		return nil, err
	}
	return counts, nil
}

type BronzeSectionRow struct {
	SecIdx      int
	Name        string
	VA          uint64
	VirtualSize uint64
	RawOffset   uint64
	RawSize     uint64
	Flags       uint32
}

func (s *BronzeStore) InsertSections(ctx context.Context, rows []BronzeSectionRow) error {
	if len(rows) == 0 {
		return nil
	}
	b := NewTxStmtBatcher(ctx, s.builder.DB(), `
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

func (s *BronzeStore) InsertRelocs(ctx context.Context, vas []uint64) error {
	if len(vas) == 0 {
		return nil
	}
	b := NewTxStmtBatcher(ctx, s.builder.DB(), `INSERT OR IGNORE INTO relocs(va) VALUES (?)`)
	defer func() { _ = b.Close() }()
	for _, va := range vas {
		if err := b.Exec(int64(va)); err != nil {
			return fmt.Errorf("insert reloc 0x%x: %w", va, err)
		}
	}
	return b.Flush()
}

type BronzeImportRow struct {
	IATVA   uint64
	DLLName string
	SymName string
	Ordinal int
}

func (s *BronzeStore) InsertImports(ctx context.Context, rows []BronzeImportRow) error {
	if len(rows) == 0 {
		return nil
	}
	b := NewTxStmtBatcher(ctx, s.builder.DB(), `
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

type BronzeExportRow struct {
	ExportID int64
	EA       uint64
	Name     string
	Ordinal  int
}

func (s *BronzeStore) InsertExports(ctx context.Context, rows []BronzeExportRow) error {
	if len(rows) == 0 {
		return nil
	}
	b := NewTxStmtBatcher(ctx, s.builder.DB(), `
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

type BronzeELFSymbolRow struct {
	SymIdx    int
	EA        uint64
	Size      uint64
	Name      string
	SymType   int
	SymBind   int
	SecIdx    int
	TableKind string
}

func (s *BronzeStore) InsertELFSymbols(ctx context.Context, rows []BronzeELFSymbolRow) error {
	if len(rows) == 0 {
		return nil
	}
	b := NewTxStmtBatcher(ctx, s.builder.DB(), `
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

// BronzeStringWriter streams string records into the bronze strings table in str_id = 1..N order.
type BronzeStringWriter struct {
	batch  *TxStmtBatcher
	nextID int64
}

func (s *BronzeStore) NewStringWriter(ctx context.Context) *BronzeStringWriter {
	return &BronzeStringWriter{
		batch: NewTxStmtBatcher(ctx, s.builder.DB(), `
			INSERT INTO strings(str_id, ea, encoding, value)
			VALUES (?, ?, ?, ?)
		`),
		nextID: 1,
	}
}

func (w *BronzeStringWriter) Add(ea uint64, encoding, value string) error {
	id := w.nextID
	if err := w.batch.Exec(id, int64(ea), encoding, value); err != nil {
		return fmt.Errorf("insert string 0x%x: %w", ea, err)
	}
	w.nextID++
	return nil
}

func (w *BronzeStringWriter) Flush() error {
	return w.batch.Flush()
}

func (w *BronzeStringWriter) Close() error {
	return w.batch.Close()
}

type BronzeTypeRow struct {
	TypeID        int64
	Kind          string
	Name          string
	ByteSize      int64
	TargetTypeID  int64
	IsDeclaration bool
}

func (s *BronzeStore) InsertDwarfTypes(ctx context.Context, rows []BronzeTypeRow) error {
	if len(rows) == 0 {
		return nil
	}
	b := NewTxStmtBatcher(ctx, s.builder.DB(), `
		INSERT INTO types(type_id, kind, name, byte_size, target_type_id, is_declaration)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		isDeclInt := 0
		if r.IsDeclaration {
			isDeclInt = 1
		}
		if err := b.Exec(
			r.TypeID,
			r.Kind,
			r.Name,
			r.ByteSize,
			NullInt64(r.TargetTypeID, r.TargetTypeID > 0),
			isDeclInt,
		); err != nil {
			return fmt.Errorf("insert canonical type %d: %w", r.TypeID, err)
		}
	}
	return b.Flush()
}

type BronzeTypeMemberRow struct {
	TypeID        int64
	Ordinal       int
	MemberKind    string
	ByteOffset    int64
	HasByteOffset bool
	BitOffset     int
	BitSize       int
	Name          string
	MemberTypeID  int64
	DisplayType   string
	IsVirtualBase bool
	VBaseExpr     string
}

type BronzeEnumValueRow struct {
	TypeID  int64
	Ordinal int
	Name    string
	Value   int64
}

type BronzeFuncVarRow struct {
	FuncEA         uint64
	Ordinal        int
	Kind           string
	Name           string
	TypeID         int64
	DisplayType    string
	ConstVal       string
	LocSummary     string
	IsOptimizedOut bool
}

type BronzeInlineRow struct {
	HostEA       uint64
	InlineIdx    int
	ParentIdx    int
	StartEA      uint64
	EndEA        uint64
	Depth        int
	CalleeNameID int64
	CallFileID   int64
	CallLine     int
}

type BronzeSourceFileRow struct {
	FileID   int64
	NormPath string
	Basename string
}

type BronzeNameRow struct {
	NameID int64
	Value  string
}

const (
	dwarfStmtMember = iota
	dwarfStmtEnum
	dwarfStmtVar
	dwarfStmtInline
	dwarfStmtFile
	dwarfStmtName
)

// BronzeDWARFPass2Writer multiplexes single-pass streaming DWARF Pass 2 writes over a single transaction batcher.
type BronzeDWARFPass2Writer struct {
	batch *MultiStmtTxBatcher
}

func (s *BronzeStore) NewDWARFPass2Writer(ctx context.Context) *BronzeDWARFPass2Writer {
	return &BronzeDWARFPass2Writer{
		batch: NewMultiStmtTxBatcher(ctx, s.builder.DB(),
			`INSERT INTO type_members(
				type_id, ordinal, member_kind, byte_offset, bit_offset, bit_size,
				name, member_type_id, display_type, is_virtual_base, vbase_expr
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			`INSERT INTO enum_values(type_id, ordinal, name, value)
			VALUES (?, ?, ?, ?)`,
			`INSERT OR IGNORE INTO func_vars(
				func_ea, ordinal, kind, name, type_id, display_type,
				const_val, loc_summary, is_optimized_out
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			`INSERT INTO inlines(
				host_ea, inline_idx, parent_idx, start_ea, end_ea,
				depth, callee_name_id, call_file_id, call_line
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			`INSERT INTO source_files(file_id, norm_path, basename)
			VALUES (?, ?, ?)`,
			`INSERT INTO names(name_id, value)
			VALUES (?, ?)`,
		),
	}
}

func (w *BronzeDWARFPass2Writer) AddMember(r BronzeTypeMemberRow) error {
	var byteOff sql.NullInt64
	if r.MemberKind != MemberKindStatic && !r.IsVirtualBase && r.HasByteOffset {
		byteOff = sql.NullInt64{Int64: r.ByteOffset, Valid: true}
	}
	isVirtInt := 0
	if r.IsVirtualBase {
		isVirtInt = 1
	}
	return w.batch.Exec(
		dwarfStmtMember,
		r.TypeID,
		r.Ordinal,
		r.MemberKind,
		byteOff,
		r.BitOffset,
		r.BitSize,
		r.Name,
		NullInt64(r.MemberTypeID, r.MemberTypeID > 0),
		r.DisplayType,
		isVirtInt,
		r.VBaseExpr,
	)
}

func (w *BronzeDWARFPass2Writer) AddEnumValue(r BronzeEnumValueRow) error {
	if err := w.batch.Exec(dwarfStmtEnum, r.TypeID, r.Ordinal, r.Name, r.Value); err != nil {
		return fmt.Errorf("insert enum_value %s: %w", r.Name, err)
	}
	return nil
}

func (w *BronzeDWARFPass2Writer) AddFuncVar(r BronzeFuncVarRow) error {
	isOptOut := 0
	if r.IsOptimizedOut {
		isOptOut = 1
	}
	return w.batch.Exec(
		dwarfStmtVar,
		int64(r.FuncEA),
		r.Ordinal,
		r.Kind,
		r.Name,
		NullInt64(r.TypeID, r.TypeID > 0),
		r.DisplayType,
		r.ConstVal,
		r.LocSummary,
		isOptOut,
	)
}

func (w *BronzeDWARFPass2Writer) AddInline(r BronzeInlineRow) error {
	if err := w.batch.Exec(
		dwarfStmtInline,
		int64(r.HostEA),
		r.InlineIdx,
		r.ParentIdx,
		int64(r.StartEA),
		int64(r.EndEA),
		r.Depth,
		r.CalleeNameID,
		NullInt64(r.CallFileID, r.CallFileID > 0),
		r.CallLine,
	); err != nil {
		return fmt.Errorf("insert inline at 0x%x: %w", r.HostEA, err)
	}
	return nil
}

func (w *BronzeDWARFPass2Writer) AddSourceFile(r BronzeSourceFileRow) error {
	if err := w.batch.Exec(dwarfStmtFile, r.FileID, r.NormPath, r.Basename); err != nil {
		return fmt.Errorf("insert source_file %s: %w", r.NormPath, err)
	}
	return nil
}

func (w *BronzeDWARFPass2Writer) AddName(r BronzeNameRow) error {
	if err := w.batch.Exec(dwarfStmtName, r.NameID, r.Value); err != nil {
		return fmt.Errorf("insert name %s: %w", r.Value, err)
	}
	return nil
}

func (w *BronzeDWARFPass2Writer) Flush() error {
	return w.batch.Flush()
}

func (w *BronzeDWARFPass2Writer) Close() error {
	return w.batch.Close()
}

type BronzeDwarfFuncRow struct {
	EA          uint64
	Size        uint64
	Name        string
	LinkageName string
	ClassName   string
	DeclFileID  int64
	DeclLine    int
	LowLine     int
	HighLine    int
}

func (s *BronzeStore) InsertDwarfFuncs(ctx context.Context, rows []BronzeDwarfFuncRow) error {
	if len(rows) == 0 {
		return nil
	}
	b := NewTxStmtBatcher(ctx, s.builder.DB(), `
		INSERT INTO dwarf_funcs(
			ea, size, name, linkage_name, class_name,
			decl_file_id, decl_line, low_line, high_line
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(
			int64(r.EA),
			int64(r.Size),
			r.Name,
			r.LinkageName,
			r.ClassName,
			NullInt64(r.DeclFileID, r.DeclFileID > 0),
			r.DeclLine,
			r.LowLine,
			r.HighLine,
		); err != nil {
			return fmt.Errorf("insert dwarf_func 0x%x: %w", r.EA, err)
		}
	}
	return b.Flush()
}

type BronzeDwarfFuncRangeRow struct {
	StartEA uint64
	EndEA   uint64
	FuncEA  uint64
}

func (s *BronzeStore) InsertDwarfFuncRanges(ctx context.Context, rows []BronzeDwarfFuncRangeRow) error {
	if len(rows) == 0 {
		return nil
	}
	b := NewTxStmtBatcher(ctx, s.builder.DB(), `
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
