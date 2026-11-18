package extractdb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBronzeStore_AbortRollsBackAndPreservesExisting(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bronze.sqlite")

	// 1. Abort on a new database leaves neither dest nor .tmp behind.
	s1, err := NewBronzeStore(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewBronzeStore: %v", err)
	}
	if err := s1.InsertSections(ctx, []BronzeSectionRow{{
		SecIdx: 0, Name: ".text", VA: 0x401000, VirtualSize: 0x100, RawOffset: 0x400, RawSize: 0x100, Flags: SecFlagRead | SecFlagExec,
	}}); err != nil {
		t.Fatalf("InsertSections: %v", err)
	}
	if err := s1.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no %s after Abort, stat err=%v", dbPath, err)
	}
	if _, err := os.Stat(dbPath + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no %s.tmp after Abort, stat err=%v", dbPath, err)
	}

	// 2. Commit a valid baseline, then Abort a subsequent rebuild -> original stays intact.
	s2, err := NewBronzeStore(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewBronzeStore baseline: %v", err)
	}
	if err := s2.InsertSections(ctx, []BronzeSectionRow{{
		SecIdx: 0, Name: ".text", VA: 0x1000, VirtualSize: 0x20, RawOffset: 0x200, RawSize: 0x20, Flags: SecFlagRead,
	}}); err != nil {
		t.Fatalf("InsertSections baseline: %v", err)
	}
	counts, err := s2.FinalizeAndCommit(ctx, [][2]string{{MetaBinarySHA256, "baseline_sha"}})
	if err != nil {
		t.Fatalf("FinalizeAndCommit baseline: %v", err)
	}
	if counts["sections"] != 1 {
		t.Fatalf("baseline sections = %d, want 1", counts["sections"])
	}

	s3, err := NewBronzeStore(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewBronzeStore second: %v", err)
	}
	if err := s3.InsertSections(ctx, []BronzeSectionRow{
		{SecIdx: 0, Name: ".a", VA: 0x2000},
		{SecIdx: 1, Name: ".b", VA: 0x3000},
	}); err != nil {
		t.Fatalf("InsertSections second: %v", err)
	}
	if err := s3.Abort(); err != nil {
		t.Fatalf("Abort second: %v", err)
	}

	summary := InspectBronzeStatus(ctx, dbPath)
	if !summary.Present || summary.BinarySHA256 != "baseline_sha" || summary.Sections != 1 {
		t.Fatalf("expected baseline bronze store preserved after Abort, got %+v", summary)
	}
	if _, err := os.Stat(dbPath + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no %s.tmp after second Abort, stat err=%v", dbPath, err)
	}
}

func TestBronzeStore_TypedWritesNullSemanticsAndFinalize(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "bronze.sqlite")

	store, err := NewBronzeStore(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewBronzeStore: %v", err)
	}
	defer func() { _ = store.Abort() }()

	if err := store.InsertSections(ctx, []BronzeSectionRow{
		{SecIdx: 0, Name: ".text", VA: 0x401000, VirtualSize: 0x1000, RawOffset: 0x400, RawSize: 0x1000, Flags: SecFlagAlloc | SecFlagRead | SecFlagExec},
	}); err != nil {
		t.Fatalf("InsertSections: %v", err)
	}
	if err := store.InsertRelocs(ctx, []uint64{0x402000, 0x402000, 0x402008}); err != nil {
		t.Fatalf("InsertRelocs: %v", err)
	}
	if err := store.InsertImports(ctx, []BronzeImportRow{
		{IATVA: 0x403000, DLLName: "KERNEL32.dll", SymName: "OldSym", Ordinal: 1},
		{IATVA: 0x403000, DLLName: "KERNEL32.dll", SymName: "GetTickCount", Ordinal: 42},
	}); err != nil {
		t.Fatalf("InsertImports: %v", err)
	}
	if err := store.InsertExports(ctx, []BronzeExportRow{
		{ExportID: 1, EA: 0x401000, Name: "CreateFaction", Ordinal: 1},
	}); err != nil {
		t.Fatalf("InsertExports: %v", err)
	}
	if err := store.InsertELFSymbols(ctx, []BronzeELFSymbolRow{
		{SymIdx: 1, EA: 0x401000, Size: 32, Name: "_ZN2CA7FACTION3addEv", SymType: 2, SymBind: 1, SecIdx: 1, TableKind: "symtab"},
	}); err != nil {
		t.Fatalf("InsertELFSymbols: %v", err)
	}

	sw := store.NewStringWriter(ctx)
	if err := sw.Add(0x404000, StringEncASCII, "FACTION_GOLD"); err != nil {
		t.Fatalf("sw.Add 1: %v", err)
	}
	if err := sw.Add(0x404020, StringEncUTF16LE, "CampaignMap"); err != nil {
		t.Fatalf("sw.Add 2: %v", err)
	}
	if err := sw.Flush(); err != nil {
		t.Fatalf("sw.Flush: %v", err)
	}
	_ = sw.Close()

	if err := store.InsertDwarfTypes(ctx, []BronzeTypeRow{
		{TypeID: 1, Kind: TypeKindBase, Name: "int", ByteSize: 4, TargetTypeID: 0, IsDeclaration: false},
		{TypeID: 2, Kind: TypeKindClass, Name: "CA::FACTION", ByteSize: 64, TargetTypeID: 0, IsDeclaration: false},
		{TypeID: 3, Kind: TypeKindPointer, Name: "", ByteSize: 8, TargetTypeID: 2, IsDeclaration: false},
	}); err != nil {
		t.Fatalf("InsertDwarfTypes: %v", err)
	}

	p2 := store.NewDWARFPass2Writer(ctx)
	if err := p2.AddSourceFile(BronzeSourceFileRow{FileID: 1, NormPath: "/src/faction.cpp", Basename: "faction.cpp"}); err != nil {
		t.Fatalf("AddSourceFile: %v", err)
	}
	if err := p2.AddName(BronzeNameRow{NameID: 1, Value: "CA::clamp_gold"}); err != nil {
		t.Fatalf("AddName: %v", err)
	}
	// Member 0: non-virtual base (byte_offset = 0)
	if err := p2.AddMember(BronzeTypeMemberRow{
		TypeID: 2, Ordinal: 0, MemberKind: MemberKindBase, ByteOffset: 0, HasByteOffset: true,
		Name: "RECORD_BASE", MemberTypeID: 0, DisplayType: "RECORD_BASE",
	}); err != nil {
		t.Fatalf("AddMember non-virt base: %v", err)
	}
	// Member 1: virtual base -> byte_offset MUST be forced NULL even if caller passed HasByteOffset=true
	if err := p2.AddMember(BronzeTypeMemberRow{
		TypeID: 2, Ordinal: 1, MemberKind: MemberKindBase, ByteOffset: 8, HasByteOffset: true,
		Name: "I_SERIALIZABLE", MemberTypeID: 0, DisplayType: "I_SERIALIZABLE", IsVirtualBase: true, VBaseExpr: "virtual",
	}); err != nil {
		t.Fatalf("AddMember virt base: %v", err)
	}
	// Member 2: regular field with known offset
	if err := p2.AddMember(BronzeTypeMemberRow{
		TypeID: 2, Ordinal: 2, MemberKind: MemberKindField, ByteOffset: 16, HasByteOffset: true,
		BitOffset: 3, BitSize: 1, Name: "m_is_human", MemberTypeID: 1, DisplayType: "int",
	}); err != nil {
		t.Fatalf("AddMember field: %v", err)
	}
	// Member 3: regular field without byte offset -> byte_offset NULL, member_type_id NULL
	if err := p2.AddMember(BronzeTypeMemberRow{
		TypeID: 2, Ordinal: 3, MemberKind: MemberKindField, HasByteOffset: false,
		Name: "m_unknown", MemberTypeID: 0,
	}); err != nil {
		t.Fatalf("AddMember unknown field: %v", err)
	}
	// Member 4: static member -> byte_offset MUST be forced NULL even if caller passed HasByteOffset=true
	if err := p2.AddMember(BronzeTypeMemberRow{
		TypeID: 2, Ordinal: 4, MemberKind: MemberKindStatic, ByteOffset: 99, HasByteOffset: true,
		Name: "s_max", MemberTypeID: 1, DisplayType: "int",
	}); err != nil {
		t.Fatalf("AddMember static: %v", err)
	}
	if err := p2.AddEnumValue(BronzeEnumValueRow{TypeID: 2, Ordinal: 0, Name: "E_NONE", Value: 0}); err != nil {
		t.Fatalf("AddEnumValue: %v", err)
	}
	if err := p2.AddFuncVar(BronzeFuncVarRow{
		FuncEA: 0x401000, Ordinal: 0, Kind: VarKindParam, Name: "this", TypeID: 3, DisplayType: "CA::FACTION*", LocSummary: "reg5",
	}); err != nil {
		t.Fatalf("AddFuncVar 0: %v", err)
	}
	if err := p2.AddFuncVar(BronzeFuncVarRow{
		FuncEA: 0x401000, Ordinal: 1, Kind: VarKindLocal, Name: "opt_tmp", TypeID: 0, IsOptimizedOut: true,
	}); err != nil {
		t.Fatalf("AddFuncVar 1: %v", err)
	}
	if err := p2.AddInline(BronzeInlineRow{
		HostEA: 0x401000, InlineIdx: 0, ParentIdx: -1, StartEA: 0x401010, EndEA: 0x401020,
		Depth: 1, CalleeNameID: 1, CallFileID: 1, CallLine: 42,
	}); err != nil {
		t.Fatalf("AddInline 0: %v", err)
	}
	if err := p2.AddInline(BronzeInlineRow{
		HostEA: 0x401000, InlineIdx: 1, ParentIdx: 0, StartEA: 0x401014, EndEA: 0x401018,
		Depth: 2, CalleeNameID: 1, CallFileID: 0, CallLine: 0,
	}); err != nil {
		t.Fatalf("AddInline 1: %v", err)
	}
	if err := p2.Flush(); err != nil {
		t.Fatalf("p2.Flush: %v", err)
	}
	_ = p2.Close()

	if err := store.InsertDwarfFuncs(ctx, []BronzeDwarfFuncRow{
		{EA: 0x401000, Size: 64, Name: "CA::FACTION::add_gold", LinkageName: "_ZN2CA7FACTION8add_goldEv", ClassName: "CA::FACTION", DeclFileID: 1, DeclLine: 10, LowLine: 12, HighLine: 25},
		{EA: 0x401100, Size: 16, Name: "orphan_helper", DeclFileID: 0},
	}); err != nil {
		t.Fatalf("InsertDwarfFuncs: %v", err)
	}
	if err := store.InsertDwarfFuncRanges(ctx, []BronzeDwarfFuncRangeRow{
		{StartEA: 0x401000, EndEA: 0x401040, FuncEA: 0x401000},
		{StartEA: 0x401000, EndEA: 0x401040, FuncEA: 0x401000}, // duplicate ignored
	}); err != nil {
		t.Fatalf("InsertDwarfFuncRanges: %v", err)
	}

	counts, err := store.FinalizeAndCommit(ctx, [][2]string{
		{MetaTarget, "attila_64"},
		{MetaBinarySHA256, "deadbeef"},
	})
	if err != nil {
		t.Fatalf("FinalizeAndCommit: %v", err)
	}

	wantCounts := map[string]int64{
		"sections":          1,
		"relocs":            2,
		"imports":           1,
		"exports":           1,
		"strings":           2,
		"elf_symbols":       1,
		"dwarf_funcs":       2,
		"dwarf_func_ranges": 1,
		"types":             3,
		"type_members":      5,
		"enum_values":       1,
		"func_vars":         2,
		"source_files":      1,
		"names":             1,
		"inlines":           2,
	}
	for k, want := range wantCounts {
		if counts[k] != want {
			t.Errorf("counts[%q] = %d, want %d", k, counts[k], want)
		}
	}

	// Verify status inspection reads cached counts & meta.
	status := InspectBronzeStatus(ctx, dbPath)
	if !status.Present || status.BinarySHA256 != "deadbeef" || status.Sections != 1 || status.Strings != 2 || status.DwarfFuncs != 2 {
		t.Fatalf("unexpected InspectBronzeStatus: %+v", status)
	}

	db, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	// Verify imports replace-on-conflict.
	var symName string
	if err := db.QueryRowContext(ctx, `SELECT sym_name FROM imports WHERE iat_va = ?`, int64(0x403000)).Scan(&symName); err != nil {
		t.Fatal(err)
	}
	if symName != "GetTickCount" {
		t.Errorf("import sym_name = %q, want GetTickCount", symName)
	}

	// Verify strings str_id = 1..N ordering.
	var strID1, strID2 int64
	if err := db.QueryRowContext(ctx, `SELECT str_id FROM strings WHERE value = 'FACTION_GOLD'`).Scan(&strID1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT str_id FROM strings WHERE value = 'CampaignMap'`).Scan(&strID2); err != nil {
		t.Fatal(err)
	}
	if strID1 != 1 || strID2 != 2 {
		t.Errorf("str_ids = (%d, %d), want (1, 2)", strID1, strID2)
	}

	// Verify types.target_type_id NULL vs non-NULL.
	var target1, target3 sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT target_type_id FROM types WHERE type_id = 1`).Scan(&target1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT target_type_id FROM types WHERE type_id = 3`).Scan(&target3); err != nil {
		t.Fatal(err)
	}
	if target1.Valid || !target3.Valid || target3.Int64 != 2 {
		t.Errorf("types.target_type_id = (%v, %v), want (NULL, 2)", target1, target3)
	}

	// Verify type_members NULL semantics.
	type memCheck struct {
		ord       int
		wantOff   sql.NullInt64
		wantType  sql.NullInt64
		wantVirt  int
	}
	for _, tc := range []memCheck{
		{ord: 0, wantOff: sql.NullInt64{Int64: 0, Valid: true}, wantType: sql.NullInt64{}, wantVirt: 0},
		{ord: 1, wantOff: sql.NullInt64{}, wantType: sql.NullInt64{}, wantVirt: 1},
		{ord: 2, wantOff: sql.NullInt64{Int64: 16, Valid: true}, wantType: sql.NullInt64{Int64: 1, Valid: true}, wantVirt: 0},
		{ord: 3, wantOff: sql.NullInt64{}, wantType: sql.NullInt64{}, wantVirt: 0},
		{ord: 4, wantOff: sql.NullInt64{}, wantType: sql.NullInt64{Int64: 1, Valid: true}, wantVirt: 0},
	} {
		var gotOff, gotType sql.NullInt64
		var gotVirt int
		if err := db.QueryRowContext(ctx, `
			SELECT byte_offset, member_type_id, is_virtual_base FROM type_members WHERE type_id = 2 AND ordinal = ?
		`, tc.ord).Scan(&gotOff, &gotType, &gotVirt); err != nil {
			t.Fatalf("query type_member ord=%d: %v", tc.ord, err)
		}
		if gotOff != tc.wantOff || gotType != tc.wantType || gotVirt != tc.wantVirt {
			t.Errorf("type_member[%d] = (off=%+v, type=%+v, virt=%d), want (off=%+v, type=%+v, virt=%d)",
				tc.ord, gotOff, gotType, gotVirt, tc.wantOff, tc.wantType, tc.wantVirt)
		}
	}

	// Verify func_vars.type_id NULL vs non-NULL and is_optimized_out.
	var fvType0, fvType1 sql.NullInt64
	var fvOpt1 int
	if err := db.QueryRowContext(ctx, `SELECT type_id FROM func_vars WHERE func_ea = ? AND ordinal = 0`, int64(0x401000)).Scan(&fvType0); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT type_id, is_optimized_out FROM func_vars WHERE func_ea = ? AND ordinal = 1`, int64(0x401000)).Scan(&fvType1, &fvOpt1); err != nil {
		t.Fatal(err)
	}
	if !fvType0.Valid || fvType0.Int64 != 3 || fvType1.Valid || fvOpt1 != 1 {
		t.Errorf("func_vars = (t0=%+v, t1=%+v, opt1=%d), want (3, NULL, 1)", fvType0, fvType1, fvOpt1)
	}

	// Verify inlines.call_file_id NULL vs non-NULL.
	var cf0, cf1 sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT call_file_id FROM inlines WHERE host_ea = ? AND inline_idx = 0`, int64(0x401000)).Scan(&cf0); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT call_file_id FROM inlines WHERE host_ea = ? AND inline_idx = 1`, int64(0x401000)).Scan(&cf1); err != nil {
		t.Fatal(err)
	}
	if !cf0.Valid || cf0.Int64 != 1 || cf1.Valid {
		t.Errorf("inlines.call_file_id = (%+v, %+v), want (1, NULL)", cf0, cf1)
	}

	// Verify dwarf_funcs.decl_file_id NULL vs non-NULL.
	var df0, df1 sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT decl_file_id FROM dwarf_funcs WHERE ea = ?`, int64(0x401000)).Scan(&df0); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT decl_file_id FROM dwarf_funcs WHERE ea = ?`, int64(0x401100)).Scan(&df1); err != nil {
		t.Fatal(err)
	}
	if !df0.Valid || df0.Int64 != 1 || df1.Valid {
		t.Errorf("dwarf_funcs.decl_file_id = (%+v, %+v), want (1, NULL)", df0, df1)
	}

	// Verify FTS5 rebuild across all three bronze FTS tables.
	for _, q := range []string{
		`SELECT COUNT(*) FROM strings_fts WHERE strings_fts MATCH 'FACTION_GOLD'`,
		`SELECT COUNT(*) FROM dwarf_funcs_fts WHERE dwarf_funcs_fts MATCH 'add_gold'`,
		`SELECT COUNT(*) FROM types_fts WHERE types_fts MATCH 'FACTION'`,
	} {
		var n int
		if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
			t.Fatalf("FTS query %q: %v", q, err)
		}
		if n != 1 {
			t.Errorf("FTS query %q = %d, want 1", q, n)
		}
	}
}

func TestBronzeStore_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dbPath := filepath.Join(t.TempDir(), "bronze.sqlite")
	if _, err := NewBronzeStore(ctx, dbPath); !errors.Is(err, context.Canceled) {
		t.Fatalf("NewBronzeStore(canceled) err = %v, want context.Canceled", err)
	}
}
