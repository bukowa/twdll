package bronze

import (
	"bytes"
	"context"
	"database/sql"
	"debug/dwarf"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

func TestExtractELF64_SyntheticTwoCUDWARF4(t *testing.T) {
	ctx := context.Context(t.Context())
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "Attila")
	bronzeDBPath := filepath.Join(tmpDir, "bronze.sqlite")
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	fix := buildSyntheticELF64WithTwoCUDWARF4(t, binPath)

	res, err := ExtractTarget(ctx, Options{
		Target:       workspace.TargetAttila64,
		BinaryPath:   binPath,
		BronzeDBPath: bronzeDBPath,
		ManifestPath: manifestPath,
	})
	if err != nil {
		t.Fatalf("ExtractTarget failed: %v", err)
	}
	if res.ImageBase != fix.imageBase {
		t.Fatalf("ImageBase = 0x%x, want 0x%x", res.ImageBase, fix.imageBase)
	}

	db, err := extractdb.OpenReadOnly(bronzeDBPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	// 1. Assert 0 dangling type references across types, type_members, and func_vars.
	var danglingTargetTypes, danglingMemberTypes, danglingVarTypes int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM types t
		WHERE t.target_type_id IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM types p WHERE p.type_id = t.target_type_id)
	`).Scan(&danglingTargetTypes); err != nil {
		t.Fatal(err)
	}
	if danglingTargetTypes != 0 {
		t.Errorf("dangling types.target_type_id = %d, want 0", danglingTargetTypes)
	}

	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM type_members tm
		WHERE tm.member_type_id IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM types p WHERE p.type_id = tm.member_type_id)
	`).Scan(&danglingMemberTypes); err != nil {
		t.Fatal(err)
	}
	if danglingMemberTypes != 0 {
		t.Errorf("dangling type_members.member_type_id = %d, want 0", danglingMemberTypes)
	}

	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM func_vars fv
		WHERE fv.type_id IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM types p WHERE p.type_id = fv.type_id)
	`).Scan(&danglingVarTypes); err != nil {
		t.Fatal(err)
	}
	if danglingVarTypes != 0 {
		t.Errorf("dangling func_vars.type_id = %d, want 0", danglingVarTypes)
	}

	// 2. Verify cross-CU DW_AT_declaration=1 resolution:
	// CU1 had `struct CA::FACTION` with DW_AT_declaration=1 and `CA::FACTION*` pointing to it;
	// CU2 had concrete `class CA::FACTION` (byte_size=64). They must merge into ONE concrete type!
	var factionCount, factionIsDecl, factionByteSize int
	var factionTypeID int64
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*), MIN(type_id), MAX(is_declaration), MAX(byte_size)
		FROM types WHERE name = 'CA::FACTION'
	`).Scan(&factionCount, &factionTypeID, &factionIsDecl, &factionByteSize); err != nil {
		t.Fatal(err)
	}
	if factionCount != 1 || factionIsDecl != 0 || factionByteSize != 64 {
		t.Fatalf("CA::FACTION type = (count=%d, id=%d, is_decl=%d, size=%d), want (1, >0, 0, 64)",
			factionCount, factionTypeID, factionIsDecl, factionByteSize)
	}

	// Verify pointer in CU1 (`CA::FACTION*`) has target_type_id == factionTypeID.
	var ptrTargetID int64
	if err := db.QueryRowContext(ctx, `
		SELECT target_type_id FROM types WHERE kind = 'pointer' AND target_type_id = ?
	`, factionTypeID).Scan(&ptrTargetID); err != nil {
		t.Fatalf("pointer to CA::FACTION did not resolve to concrete factionTypeID %d: %v", factionTypeID, err)
	}

	// 3. Verify type_members of CA::FACTION:
	// - non-virtual base (byte_offset = 0, is_virtual_base = 0)
	// - virtual base (byte_offset IS NULL, is_virtual_base = 1)
	// - field m_treasury via DW_OP_plus_uconst (byte_offset = 16)
	// - bitfield m_is_human (byte_offset = 20, bit_offset = 3, bit_size = 1)
	// - static member s_max_factions (member_kind = 'static', byte_offset IS NULL)
	memRows, err := db.QueryContext(ctx, `
		SELECT ordinal, member_kind, byte_offset, bit_offset, bit_size, name, is_virtual_base, vbase_expr
		FROM type_members WHERE type_id = ? ORDER BY ordinal
	`, factionTypeID)
	if err != nil {
		t.Fatal(err)
	}
	type memRec struct {
		ord       int
		kind      string
		byteOff   sql.NullInt64
		bitOff    int
		bitSz     int
		name      string
		isVirt    int
		vbaseExpr string
	}
	var mems []memRec
	for memRows.Next() {
		var m memRec
		if err := memRows.Scan(&m.ord, &m.kind, &m.byteOff, &m.bitOff, &m.bitSz, &m.name, &m.isVirt, &m.vbaseExpr); err != nil {
			t.Fatal(err)
		}
		mems = append(mems, m)
	}
	memRows.Close()
	if len(mems) != 5 {
		t.Fatalf("len(CA::FACTION members) = %d, want 5: %+v", len(mems), mems)
	}
	// Ord 0: non-virtual base
	if mems[0].kind != extractdb.MemberKindBase || !mems[0].byteOff.Valid || mems[0].byteOff.Int64 != 0 || mems[0].isVirt != 0 {
		t.Errorf("member[0] (non-virt base) = %+v", mems[0])
	}
	// Ord 1: virtual base -> byte_offset MUST be NULL and is_virtual_base = 1
	if mems[1].kind != extractdb.MemberKindBase || mems[1].byteOff.Valid || mems[1].isVirt != 1 || mems[1].vbaseExpr == "" {
		t.Errorf("member[1] (virtual base) = %+v, want byte_offset=NULL & is_virtual_base=1", mems[1])
	}
	// Ord 2: m_treasury via DW_OP_plus_uconst(16)
	if mems[2].kind != extractdb.MemberKindField || !mems[2].byteOff.Valid || mems[2].byteOff.Int64 != 16 || mems[2].name != "m_treasury" {
		t.Errorf("member[2] (m_treasury) = %+v", mems[2])
	}
	// Ord 3: bitfield m_is_human (data_bit_offset=163 -> byte_offset=20, bit_offset=3, bit_size=1)
	if mems[3].kind != extractdb.MemberKindField || !mems[3].byteOff.Valid || mems[3].byteOff.Int64 != 20 || mems[3].bitOff != 3 || mems[3].bitSz != 1 {
		t.Errorf("member[3] (bitfield) = %+v, want byte=20 bit_off=3 bit_sz=1", mems[3])
	}
	// Ord 4: static member s_max_factions -> byte_offset MUST be NULL
	if mems[4].kind != extractdb.MemberKindStatic || mems[4].byteOff.Valid || mems[4].name != "s_max_factions" {
		t.Errorf("member[4] (static) = %+v, want kind=static & byte_offset=NULL", mems[4])
	}

	// 4. Verify func_vars inlineDepth==0 filtering (only root params/locals, no inlined_subroutine locals)
	// and true SLEB128 negative location decoding.
	varRows, err := db.QueryContext(ctx, `
		SELECT kind, name, display_type, loc_summary FROM func_vars WHERE func_ea = ? ORDER BY ordinal
	`, int64(fix.fnAddGoldEA))
	if err != nil {
		t.Fatal(err)
	}
	var varNames []string
	for varRows.Next() {
		var kind, name, disp, locSum string
		if err := varRows.Scan(&kind, &name, &disp, &locSum); err != nil {
			t.Fatal(err)
		}
		varNames = append(varNames, kind+":"+name+":"+disp+":"+locSum)
	}
	varRows.Close()
	wantVars := []string{
		"param:this:CA::FACTION* const:reg5",
		"param:amount:int:reg4",
		"local:local_sum:int:fbreg-8",
		"local:neg_breg:int:breg6-16",
	}
	if len(varNames) != len(wantVars) {
		t.Fatalf("func_vars = %v, want %v (inlined local must be excluded)", varNames, wantVars)
	}
	for i := range wantVars {
		if varNames[i] != wantVars[i] {
			t.Errorf("func_vars[%d] = %q, want %q", i, varNames[i], wantVars[i])
		}
	}

	// Verify anonymous-namespace struct does not collide with global struct of the same name.
	var anonHelperSize, anonHelperMembers int
	if err := db.QueryRowContext(ctx, `
		SELECT t.byte_size, COUNT(tm.ordinal)
		FROM types t
		LEFT JOIN type_members tm ON tm.type_id = t.type_id
		WHERE t.name = '(anonymous namespace)::LOCAL_HELPER'
		GROUP BY t.type_id
	`).Scan(&anonHelperSize, &anonHelperMembers); err != nil {
		t.Fatalf("query (anonymous namespace)::LOCAL_HELPER: %v", err)
	}
	if anonHelperSize != 16 || anonHelperMembers != 1 {
		t.Errorf("(anonymous namespace)::LOCAL_HELPER = (size=%d, members=%d), want (16, 1)", anonHelperSize, anonHelperMembers)
	}

	var globalHelperSize, globalHelperMembers int
	if err := db.QueryRowContext(ctx, `
		SELECT t.byte_size, COUNT(tm.ordinal)
		FROM types t
		LEFT JOIN type_members tm ON tm.type_id = t.type_id
		WHERE t.name = 'LOCAL_HELPER'
		GROUP BY t.type_id
	`).Scan(&globalHelperSize, &globalHelperMembers); err != nil {
		t.Fatalf("query global LOCAL_HELPER: %v", err)
	}
	if globalHelperSize != 32 || globalHelperMembers != 2 {
		t.Errorf("global LOCAL_HELPER = (size=%d, members=%d), want (32, 2)", globalHelperSize, globalHelperMembers)
	}

	// Verify unversioned ELF dynamic imports have dll_name = "".
	var impDLL, impSym string
	if err := db.QueryRowContext(ctx, `
		SELECT dll_name, sym_name FROM imports WHERE iat_va = ?
	`, int64(fix.gotPthreadVA)).Scan(&impDLL, &impSym); err != nil {
		t.Fatalf("query unversioned ELF import: %v", err)
	}
	if impDLL != "" || impSym != "pthread_mutex_lock" {
		t.Errorf("unversioned ELF import = (dll=%q, sym=%q), want (\"\", \"pthread_mutex_lock\")", impDLL, impSym)
	}

	// 5. Verify inlines + .debug_line source_files.
	var inlCallee, inlFile string
	var inlLine, inlDepth int
	if err := db.QueryRowContext(ctx, `
		SELECT n.value, sf.norm_path, i.call_line, i.depth
		FROM inlines i
		JOIN names n ON n.name_id = i.callee_name_id
		JOIN source_files sf ON sf.file_id = i.call_file_id
		WHERE i.host_ea = ?
	`, int64(fix.fnAddGoldEA)).Scan(&inlCallee, &inlFile, &inlLine, &inlDepth); err != nil {
		t.Fatalf("query inlines: %v", err)
	}
	if inlCallee != "CA::FACTION::clamp_gold" || inlFile != "/src/campaign/faction.cpp" || inlLine != 105 || inlDepth != 1 {
		t.Errorf("inline row = (%q, %q, line=%d, depth=%d), want (CA::FACTION::clamp_gold, /src/campaign/faction.cpp, 105, 1)",
			inlCallee, inlFile, inlLine, inlDepth)
	}

	// Verify function low_line and high_line from .debug_line (.h declFile + .cpp .debug_line fallback, excluding inline range).
	var lowLine, highLine int
	if err := db.QueryRowContext(ctx, `
		SELECT low_line, high_line FROM dwarf_funcs WHERE ea = ?
	`, int64(fix.fnAddGoldEA)).Scan(&lowLine, &highLine); err != nil {
		t.Fatal(err)
	}
	if lowLine != 100 || highLine != 110 {
		t.Errorf("function line span = [%d, %d], want [100, 110]", lowLine, highLine)
	}

	// Verify nested lambda subprogram has its own func_vars and 2-hop concrete subprogram resolved name/class.
	var lambdaVarCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM func_vars WHERE func_ea = ? AND name = 'closure'
	`, int64(fix.fnLambdaEA)).Scan(&lambdaVarCount); err != nil {
		t.Fatal(err)
	}
	if lambdaVarCount != 1 {
		t.Errorf("lambda closure param count = %d, want 1", lambdaVarCount)
	}

	var helperName, helperClass string
	if err := db.QueryRowContext(ctx, `
		SELECT name, class_name FROM dwarf_funcs WHERE ea = ?
	`, int64(fix.fnHelperEA)).Scan(&helperName, &helperClass); err != nil {
		t.Fatal(err)
	}
	if helperName != "CA::FACTION::helper" || helperClass != "CA::FACTION" {
		t.Errorf("2-hop concrete subprogram = (%q, %q), want (CA::FACTION::helper, CA::FACTION)", helperName, helperClass)
	}

	// Verify out-of-line method defined inside `namespace CA { void FACTION::ns_out_of_line() {} }`
	// preserves `CA::FACTION` instead of truncating to `CA`.
	var oolName, oolClass string
	if err := db.QueryRowContext(ctx, `
		SELECT name, class_name FROM dwarf_funcs WHERE ea = ?
	`, int64(fix.fnOutOfLineEA)).Scan(&oolName, &oolClass); err != nil {
		t.Fatal(err)
	}
	if oolName != "CA::FACTION::ns_out_of_line" || oolClass != "CA::FACTION" {
		t.Errorf("namespace out-of-line subprogram = (%q, %q), want (CA::FACTION::ns_out_of_line, CA::FACTION)", oolName, oolClass)
	}

	// 6. Verify raw .symtab symbols (including _ZTV*) are preserved verbatim in elf_symbols
	// with zero vtable heuristic decoding or synthetic functions.
	var ztvEA int64
	if err := db.QueryRowContext(ctx, `
		SELECT ea FROM elf_symbols WHERE name = '_ZTVN2CA7FACTIONE' AND table_kind = 'symtab'
	`).Scan(&ztvEA); err != nil {
		t.Fatalf("query _ZTVN2CA7FACTIONE in elf_symbols: %v", err)
	}
	if uint64(ztvEA) != fix.ztvEA {
		t.Errorf("_ZTVN2CA7FACTIONE ea = 0x%x, want 0x%x", uint64(ztvEA), fix.ztvEA)
	}

	var fnFTSCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dwarf_funcs_fts WHERE dwarf_funcs_fts MATCH 'FACTION'`).Scan(&fnFTSCount); err != nil {
		t.Fatal(err)
	}
	if fnFTSCount == 0 {
		t.Errorf("dwarf_funcs_fts MATCH 'FACTION' returned 0 rows")
	}

	// 7. Verify UTF-32LE string & ASCII string extracted statically by data.strings.
	var utf32Count, asciiCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM strings WHERE encoding = 'utf32le' AND value = 'AttilaWide32'
	`).Scan(&utf32Count); err != nil {
		t.Fatal(err)
	}
	if utf32Count != 1 {
		t.Errorf("utf32le string count = %d, want 1", utf32Count)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM strings WHERE encoding = 'ascii' AND value = 'FACTION_GOLD_ADDED'
	`).Scan(&asciiCount); err != nil {
		t.Fatalf("query FACTION_GOLD_ADDED: %v", err)
	}
	if asciiCount != 1 {
		t.Errorf("FACTION_GOLD_ADDED count = %d, want 1", asciiCount)
	}
}

type syntheticELF64Layout struct {
	imageBase     uint64
	fnAddGoldEA   uint64
	fnHelperEA    uint64
	fnLambdaEA    uint64
	fnOutOfLineEA uint64
	ztvEA         uint64
	gotPthreadVA  uint64
}

func buildSyntheticELF64WithTwoCUDWARF4(t *testing.T, outPath string) syntheticELF64Layout {
	t.Helper()
	const (
		imageBase = 0x400000
		textVA    = 0x401000
		rodataVA  = 0x402000
	)
	lay := syntheticELF64Layout{
		imageBase:     imageBase,
		fnAddGoldEA:   textVA + 0x00,
		fnHelperEA:    textVA + 0x40,
		fnLambdaEA:    textVA + 0x60,
		fnOutOfLineEA: textVA + 0x70,
		ztvEA:         rodataVA + 0x100,
		gotPthreadVA:  0x403000,
	}

	// 1. Build .rodata:
	// +0x000: ASCII string "FACTION_GOLD_ADDED\0"
	// +0x040: UTF-32LE string "AttilaWide32\0"
	// +0x0E0: dummy _ZTI for CA::FACTION
	// +0x100: _ZTVN2CA7FACTIONE (7 qwords = 56 bytes):
	//   [0] = 0 (offset_to_top)
	//   [1] = rodataVA + 0x0E0 (rtti_ptr)
	//   [2] = fnAddGoldEA (subvtable 0, slot 0)
	//   [3] = fnHelperEA  (subvtable 0, slot 1)
	//   [4] = ^uint64(15) (-16 offset_to_top for secondary MI base)
	//   [5] = rodataVA + 0x0E0 (rtti_ptr)
	//   [6] = fnHelperEA  (subvtable 1, slot 0)
	rodata := make([]byte, 0x200)
	copy(rodata[0x000:], "FACTION_GOLD_ADDED\x00")
	for idx, r := range "AttilaWide32" {
		binary.LittleEndian.PutUint32(rodata[0x040+idx*4:], uint32(r))
	}
	ztiVA := uint64(rodataVA + 0x0E0)
	ztvOff := 0x100
	binary.LittleEndian.PutUint64(rodata[ztvOff+0:ztvOff+8], 0)
	binary.LittleEndian.PutUint64(rodata[ztvOff+8:ztvOff+16], ztiVA)
	binary.LittleEndian.PutUint64(rodata[ztvOff+16:ztvOff+24], lay.fnAddGoldEA)
	binary.LittleEndian.PutUint64(rodata[ztvOff+24:ztvOff+32], lay.fnHelperEA)
	minus16 := int64(-16)
	binary.LittleEndian.PutUint64(rodata[ztvOff+32:ztvOff+40], uint64(minus16))
	binary.LittleEndian.PutUint64(rodata[ztvOff+40:ztvOff+48], ztiVA)
	binary.LittleEndian.PutUint64(rodata[ztvOff+48:ztvOff+56], lay.fnHelperEA)

	// 2. Build .text:
	// fnAddGoldEA @ 0x401000 (size 0x20):
	//   +0x00: 55                      push rbp
	//   +0x01: 48 89 E5                mov rbp, rsp
	//   +0x04: 48 8D 35 <disp to str>  lea rsi, [rip + disp32] -> rodataVA+0x000 (nextEA = 0x40100B)
	//   +0x0B: E8 <rel to fnHelperEA>  call fnHelperEA         -> 0x401040 (nextEA = 0x401010)
	//   +0x10: 48 8B 07                mov rax, [rdi]
	//   +0x13: FF 50 08                call qword ptr [rax+8]
	//   +0x16: 5D                      pop rbp
	//   +0x17: C3                      ret
	text := make([]byte, 0x80)
	for i := range text {
		text[i] = 0x90
	}
	copy(text[0x00:], []byte{0x55, 0x48, 0x89, 0xE5})
	text[0x04] = 0x48
	text[0x05] = 0x8D
	text[0x06] = 0x35
	binary.LittleEndian.PutUint32(text[0x07:0x0B], uint32(int32(rodataVA-(textVA+0x0B))))
	text[0x0B] = 0xE8
	binary.LittleEndian.PutUint32(text[0x0C:0x10], uint32(int32(int64(lay.fnHelperEA)-int64(textVA+0x10))))
	copy(text[0x10:], []byte{0x48, 0x8B, 0x07, 0xFF, 0x50, 0x08})
	// +0x16: 48 8D 05 <disp to fnLambdaEA> (lea rax, [rip + fnLambdaEA], nextEA = textVA + 0x1D)
	text[0x16] = 0x48
	text[0x17] = 0x8D
	text[0x18] = 0x05
	binary.LittleEndian.PutUint32(text[0x19:0x1D], uint32(int32(int64(lay.fnLambdaEA)-int64(textVA+0x1D))))
	copy(text[0x1D:], []byte{0x5D, 0xC3})

	// fnHelperEA @ 0x401040 (size 0x10):
	//   +0x40: 31 C0 C3 (xor eax, eax; ret)
	copy(text[0x40:], []byte{0x31, 0xC0, 0xC3})
	// fnLambdaEA @ 0x401060 (size 0x10):
	//   +0x60: 31 C0 C3 (xor eax, eax; ret)
	copy(text[0x60:], []byte{0x31, 0xC0, 0xC3})
	// fnOutOfLineEA @ 0x401070 (size 0x10):
	//   +0x70: 31 C0 C3 (xor eax, eax; ret)
	copy(text[0x70:], []byte{0x31, 0xC0, 0xC3})

	// 3. Build DWARF4 sections (.debug_abbrev, .debug_info with 2 CUs, .debug_line).
	debugAbbrev, debugInfo, debugLine := buildTwoCUDWARF4Sections(lay)

	// 4. Build .symtab and .strtab for _ZTVN2CA7FACTIONE, _ZTIN2CA7FACTIONE, and fnHelperEA.
	symtab, strtab := buildELFSymtab(lay, ztiVA)

	// 5. Write complete ELF64 file.
	writeSyntheticELF64File(t, outPath, imageBase, textVA, text, rodataVA, rodata, symtab, strtab, debugAbbrev, debugInfo, debugLine)
	return lay
}

func buildTwoCUDWARF4Sections(lay syntheticELF64Layout) (abbrev, info, line []byte) {
	// Forms:
	const (
		dwFormAddr     = 0x01
		dwFormBlock1   = 0x0a
		dwFormData1    = 0x0b
		dwFormData2    = 0x05
		dwFormData4    = 0x06
		dwFormData8    = 0x07
		dwFormString   = 0x08
		dwFormFlagPres = 0x19
		dwFormRef4     = 0x13
		dwFormSecOff   = 0x17
		dwFormExprloc  = 0x18
	)
	// Tags:
	const (
		dwTagCompileUnit       = 0x11
		dwTagNamespace         = 0x39
		dwTagStructType        = 0x13
		dwTagClassType         = 0x02
		dwTagPointerType       = 0x0f
		dwTagBaseType          = 0x24
		dwTagConstType         = 0x26
		dwTagInheritance       = 0x1c
		dwTagMember            = 0x0d
		dwTagSubprogram        = 0x2e
		dwTagFormalParameter   = 0x05
		dwTagVariable          = 0x34
		dwTagInlinedSubroutine = 0x1d
	)
	// Attrs:
	const (
		dwAtName           = 0x03
		dwAtByteSize       = 0x0b
		dwAtBitSize        = 0x0d
		dwAtStmtList       = 0x10
		dwAtLowPC          = 0x11
		dwAtHighPC         = 0x12
		dwAtCompDir        = 0x1b
		dwAtDeclaration    = 0x3c
		dwAtDataMemberLoc  = 0x38
		dwAtDeclFile       = 0x3a
		dwAtDeclLine       = 0x3b
		dwAtExternal       = 0x3f
		dwAtLocation       = 0x02
		dwAtType           = 0x49
		dwAtVirtuality     = 0x4c
		dwAtAbstractOrigin = 0x31
		dwAtCallFile       = 0x58
		dwAtCallLine       = 0x59
		dwAtSpecification  = 0x47
		dwAtDataBitOffset  = 0x6b
		dwAtLinkageName    = 0x6e
	)

	// Build .debug_abbrev shared by both CUs:
	var ab bytes.Buffer
	writeAbbrev := func(code byte, tag byte, hasKids byte, pairs ...byte) {
		ab.WriteByte(code)
		ab.WriteByte(tag)
		ab.WriteByte(hasKids)
		ab.Write(pairs)
		ab.Write([]byte{0, 0})
	}
	// 1: CompileUnit (children=1): name(string), comp_dir(string), stmt_list(sec_offset)
	writeAbbrev(1, dwTagCompileUnit, 1, dwAtName, dwFormString, dwAtCompDir, dwFormString, dwAtStmtList, dwFormSecOff)
	// 2: Namespace (children=1): name(string)
	writeAbbrev(2, dwTagNamespace, 1, dwAtName, dwFormString)
	// 3: Struct forward declaration WITH children (children=1): name(string), byte_size(data1), declaration(flag_present)
	writeAbbrev(3, dwTagStructType, 1, dwAtName, dwFormString, dwAtByteSize, dwFormData1, dwAtDeclaration, dwFormFlagPres)
	// 4: PointerType (children=0): byte_size(data1), type(ref4)
	writeAbbrev(4, dwTagPointerType, 0, dwAtByteSize, dwFormData1, dwAtType, dwFormRef4)
	// 5: BaseType (children=0): name(string), byte_size(data1)
	writeAbbrev(5, dwTagBaseType, 0, dwAtName, dwFormString, dwAtByteSize, dwFormData1)
	// 6: Concrete ClassType (children=1): name(string), byte_size(data1)
	writeAbbrev(6, dwTagClassType, 1, dwAtName, dwFormString, dwAtByteSize, dwFormData1)
	// 7: Non-virtual Inheritance (children=0): type(ref4), data_member_loc(data1)
	writeAbbrev(7, dwTagInheritance, 0, dwAtType, dwFormRef4, dwAtDataMemberLoc, dwFormData1)
	// 8: Virtual Inheritance (children=0): type(ref4), virtuality(data1), data_member_loc(exprloc)
	writeAbbrev(8, dwTagInheritance, 0, dwAtType, dwFormRef4, dwAtVirtuality, dwFormData1, dwAtDataMemberLoc, dwFormExprloc)
	// 9: Field Member with exprloc DW_OP_plus_uconst (children=0): name(string), type(ref4), data_member_loc(exprloc)
	writeAbbrev(9, dwTagMember, 0, dwAtName, dwFormString, dwAtType, dwFormRef4, dwAtDataMemberLoc, dwFormExprloc)
	// 10: Bitfield Member (children=0): name(string), type(ref4), bit_size(data1), data_bit_offset(data2)
	writeAbbrev(10, dwTagMember, 0, dwAtName, dwFormString, dwAtType, dwFormRef4, dwAtBitSize, dwFormData1, dwAtDataBitOffset, dwFormData2)
	// 11: Static Member (children=0): name(string), type(ref4), external(flag_present), declaration(flag_present)
	writeAbbrev(11, dwTagMember, 0, dwAtName, dwFormString, dwAtType, dwFormRef4, dwAtExternal, dwFormFlagPres, dwAtDeclaration, dwFormFlagPres)
	// 12: Subprogram declaration inside class (children=0): name(string), linkage_name(string), decl_file(data1), decl_line(data1), declaration(flag_present)
	writeAbbrev(12, dwTagSubprogram, 0, dwAtName, dwFormString, dwAtLinkageName, dwFormString, dwAtDeclFile, dwFormData1, dwAtDeclLine, dwFormData1, dwAtDeclaration, dwFormFlagPres)
	// 13: Concrete Subprogram with specification (children=1): specification(ref4), low_pc(addr), high_pc(data8)
	writeAbbrev(13, dwTagSubprogram, 1, dwAtSpecification, dwFormRef4, dwAtLowPC, dwFormAddr, dwAtHighPC, dwFormData8)
	// 14: FormalParameter (children=0): name(string), type(ref4), location(exprloc)
	writeAbbrev(14, dwTagFormalParameter, 0, dwAtName, dwFormString, dwAtType, dwFormRef4, dwAtLocation, dwFormExprloc)
	// 15: Variable (children=0): name(string), type(ref4), location(exprloc)
	writeAbbrev(15, dwTagVariable, 0, dwAtName, dwFormString, dwAtType, dwFormRef4, dwAtLocation, dwFormExprloc)
	// 16: InlinedSubroutine (children=1): abstract_origin(ref4), low_pc(addr), high_pc(data8), call_file(data1), call_line(data1)
	writeAbbrev(16, dwTagInlinedSubroutine, 1, dwAtAbstractOrigin, dwFormRef4, dwAtLowPC, dwFormAddr, dwAtHighPC, dwFormData8, dwAtCallFile, dwFormData1, dwAtCallLine, dwFormData1)
	// 17: Intermediate abstract Subprogram with specification (children=1): specification(ref4)
	writeAbbrev(17, dwTagSubprogram, 1, dwAtSpecification, dwFormRef4)
	// 18: Concrete Subprogram with abstract_origin (children=0): abstract_origin(ref4), low_pc(addr), high_pc(data8)
	writeAbbrev(18, dwTagSubprogram, 0, dwAtAbstractOrigin, dwFormRef4, dwAtLowPC, dwFormAddr, dwAtHighPC, dwFormData8)
	// 19: Concrete Lambda Subprogram with name (children=1): name(string), low_pc(addr), high_pc(data8)
	writeAbbrev(19, dwTagSubprogram, 1, dwAtName, dwFormString, dwAtLowPC, dwFormAddr, dwAtHighPC, dwFormData8)
	// 20: FormalParameter with abstract_origin only (children=0): abstract_origin(ref4), location(exprloc)
	writeAbbrev(20, dwTagFormalParameter, 0, dwAtAbstractOrigin, dwFormRef4, dwAtLocation, dwFormExprloc)
	// 21: Intermediate FormalParameter with specification only (children=0): specification(ref4)
	writeAbbrev(21, dwTagFormalParameter, 0, dwAtSpecification, dwFormRef4)
	// 22: Declaration FormalParameter with name+type (children=0): name(string), type(ref4)
	writeAbbrev(22, dwTagFormalParameter, 0, dwAtName, dwFormString, dwAtType, dwFormRef4)
	// 23: Anonymous Namespace (children=1): no attrs
	writeAbbrev(23, dwTagNamespace, 1)
	// 24: Concrete StructType (children=1): name(string), byte_size(data1)
	writeAbbrev(24, dwTagStructType, 1, dwAtName, dwFormString, dwAtByteSize, dwFormData1)
	// 25: ConstType (children=0): type(ref4)
	writeAbbrev(25, dwTagConstType, 0, dwAtType, dwFormRef4)
	ab.WriteByte(0)
	abbrev = ab.Bytes()

	// Build .debug_line (DWARF4 line table with file 1 = faction.h and file 2 = faction.cpp):
	line = buildDWARFLineTable(lay.fnAddGoldEA)

	// Build CU 1: Forward declaration of CA::FACTION (WITH nested DW_TAG_subprogram child and byte_size=64!) + pointer CA::FACTION*
	// + anonymous-namespace struct LOCAL_HELPER (size 16, 1 member)
	var cu1 bytes.Buffer
	cu1.Write([]byte{0, 0, 0, 0}) // length placeholder
	binary.Write(&cu1, binary.LittleEndian, uint16(4))
	binary.Write(&cu1, binary.LittleEndian, uint32(0)) // abbrev offset = 0
	cu1.WriteByte(8)                                   // addr_size = 8
	// DIE: CompileUnit (abbrev 1)
	cu1.WriteByte(1)
	cu1.WriteString("unit1.cpp\x00")
	cu1.WriteString("/src/campaign\x00")
	binary.Write(&cu1, binary.LittleEndian, uint32(0))
	// DIE: BaseType "int" in CU 1 (abbrev 5)
	intOffCU1 := uint32(cu1.Len())
	cu1.WriteByte(5)
	cu1.WriteString("int\x00")
	cu1.WriteByte(4)
	// DIE: Anonymous Namespace (abbrev 23) containing struct LOCAL_HELPER (size 16, 1 member)
	cu1.WriteByte(23)
	cu1.WriteByte(24)
	cu1.WriteString("LOCAL_HELPER\x00")
	cu1.WriteByte(16)
	cu1.WriteByte(9)
	cu1.WriteString("m_anon\x00")
	binary.Write(&cu1, binary.LittleEndian, intOffCU1)
	cu1.Write([]byte{2, 0x23, 0})
	cu1.WriteByte(0) // end LOCAL_HELPER
	cu1.WriteByte(0) // end Anonymous Namespace
	// DIE: Namespace "CA" (abbrev 2)
	cu1.WriteByte(2)
	cu1.WriteString("CA\x00")
	// DIE: Forward-declared struct FACTION with nested DW_TAG_subprogram child (abbrev 3)
	fwdOffInCU1 := uint32(cu1.Len())
	cu1.WriteByte(3)
	cu1.WriteString("FACTION\x00")
	cu1.WriteByte(64) // byte_size = 64 even on forward-decl with method child
	// Nested subprogram declaration inside forward-declared FACTION (abbrev 12)
	cu1.WriteByte(12)
	cu1.WriteString("early_decl\x00")
	cu1.WriteString("_ZN2CA7FACTION10early_declEv\x00")
	cu1.WriteByte(1)
	cu1.WriteByte(10)
	cu1.WriteByte(0) // end struct FACTION children
	cu1.WriteByte(0) // end Namespace "CA"
	// DIE: Pointer to forward-declared CA::FACTION (abbrev 4)
	cu1.WriteByte(4)
	cu1.WriteByte(8)
	binary.Write(&cu1, binary.LittleEndian, fwdOffInCU1)
	cu1.WriteByte(0) // end CU 1
	cu1Bytes := cu1.Bytes()
	binary.LittleEndian.PutUint32(cu1Bytes[0:4], uint32(len(cu1Bytes)-4))

	// Build CU 2: Concrete class CA::FACTION, base classes, fields, bitfield, static member, and add_gold function
	var cu2 bytes.Buffer
	cu2.Write([]byte{0, 0, 0, 0}) // length placeholder
	binary.Write(&cu2, binary.LittleEndian, uint16(4))
	binary.Write(&cu2, binary.LittleEndian, uint32(0)) // abbrev offset = 0
	cu2.WriteByte(8)                                   // addr_size = 8
	// DIE: CompileUnit (abbrev 1)
	cu2.WriteByte(1)
	cu2.WriteString("faction.cpp\x00")
	cu2.WriteString("/src/campaign\x00")
	binary.Write(&cu2, binary.LittleEndian, uint32(0)) // stmt_list = 0

	// DIE: BaseType "int" (abbrev 5)
	intOff := uint32(cu2.Len())
	cu2.WriteByte(5)
	cu2.WriteString("int\x00")
	cu2.WriteByte(4)

	// DIE: Global struct LOCAL_HELPER in CU 2 (size 32, 2 members) — must NOT collide with CU 1's anonymous-namespace LOCAL_HELPER!
	cu2.WriteByte(24)
	cu2.WriteString("LOCAL_HELPER\x00")
	cu2.WriteByte(32)
	cu2.WriteByte(9)
	cu2.WriteString("m_global1\x00")
	binary.Write(&cu2, binary.LittleEndian, intOff)
	cu2.Write([]byte{2, 0x23, 0})
	cu2.WriteByte(9)
	cu2.WriteString("m_global2\x00")
	binary.Write(&cu2, binary.LittleEndian, intOff)
	cu2.Write([]byte{2, 0x23, 4})
	cu2.WriteByte(0) // end global LOCAL_HELPER

	// DIE: Namespace "CA" (abbrev 2)
	cu2.WriteByte(2)
	cu2.WriteString("CA\x00")

	// DIE: Concrete base class RECORD_BASE (abbrev 6, no members)
	baseOff := uint32(cu2.Len())
	cu2.WriteByte(6)
	cu2.WriteString("RECORD_BASE\x00")
	cu2.WriteByte(8)
	cu2.WriteByte(0) // end RECORD_BASE

	// DIE: Concrete virtual base class I_SERIALIZABLE (abbrev 6, no members)
	vbaseOff := uint32(cu2.Len())
	cu2.WriteByte(6)
	cu2.WriteString("I_SERIALIZABLE\x00")
	cu2.WriteByte(8)
	cu2.WriteByte(0) // end I_SERIALIZABLE

	// DIE: Concrete class FACTION (abbrev 6, size 64)
	factionOff := uint32(cu2.Len())
	cu2.WriteByte(6)
	cu2.WriteString("FACTION\x00")
	cu2.WriteByte(64)

	// Child 0: Non-virtual Inheritance RECORD_BASE at offset 0 (abbrev 7)
	cu2.WriteByte(7)
	binary.Write(&cu2, binary.LittleEndian, baseOff)
	cu2.WriteByte(0)

	// Child 1: Virtual Inheritance I_SERIALIZABLE (abbrev 8, virtuality=1, complex expr [0x12, 0x06])
	cu2.WriteByte(8)
	binary.Write(&cu2, binary.LittleEndian, vbaseOff)
	cu2.WriteByte(1)
	cu2.Write([]byte{2, 0x12, 0x06}) // exprloc len=2: DW_OP_dup; DW_OP_deref

	// Child 2: Field m_treasury via DW_OP_plus_uconst (0x23, 16) (abbrev 9)
	cu2.WriteByte(9)
	cu2.WriteString("m_treasury\x00")
	binary.Write(&cu2, binary.LittleEndian, intOff)
	cu2.Write([]byte{2, 0x23, 16})

	// Child 3: Bitfield m_is_human (data_bit_offset = 163 -> byte 20, bit 3, bit_size = 1) (abbrev 10)
	cu2.WriteByte(10)
	cu2.WriteString("m_is_human\x00")
	binary.Write(&cu2, binary.LittleEndian, intOff)
	cu2.WriteByte(1)
	binary.Write(&cu2, binary.LittleEndian, uint16(163))

	// Child 4: Static member s_max_factions (abbrev 11)
	cu2.WriteByte(11)
	cu2.WriteString("s_max_factions\x00")
	binary.Write(&cu2, binary.LittleEndian, intOff)

	// Child 5: Subprogram declaration "add_gold" inside CA::FACTION (abbrev 12) -> decl_file = 1 (faction.h)
	addGoldSpecOff := uint32(cu2.Len())
	cu2.WriteByte(12)
	cu2.WriteString("add_gold\x00")
	cu2.WriteString("_ZN2CA7FACTION8add_goldEi\x00")
	cu2.WriteByte(1)  // decl_file = 1 (faction.h)
	cu2.WriteByte(99) // decl_line = 99

	// Child 6: Subprogram declaration "clamp_gold" inside CA::FACTION (abbrev 12)
	clampGoldSpecOff := uint32(cu2.Len())
	cu2.WriteByte(12)
	cu2.WriteString("clamp_gold\x00")
	cu2.WriteString("_ZN2CA7FACTION10clamp_goldEi\x00")
	cu2.WriteByte(1)
	cu2.WriteByte(50)

	// Child 7: Subprogram declaration "helper" inside CA::FACTION (abbrev 12)
	helperSpecOff := uint32(cu2.Len())
	cu2.WriteByte(12)
	cu2.WriteString("helper\x00")
	cu2.WriteString("_ZN2CA6helperEv\x00")
	cu2.WriteByte(1)
	cu2.WriteByte(60)

	// Child 8: Subprogram declaration "ns_out_of_line" inside CA::FACTION (abbrev 12)
	nsOutOfLineSpecOff := uint32(cu2.Len())
	cu2.WriteByte(12)
	cu2.WriteString("ns_out_of_line\x00")
	cu2.WriteString("_ZN2CA7FACTION14ns_out_of_lineEv\x00")
	cu2.WriteByte(1)
	cu2.WriteByte(70)

	cu2.WriteByte(0) // end class FACTION
	cu2.WriteByte(0) // end namespace CA

	// Out-of-line definition inside `namespace CA { void FACTION::ns_out_of_line() {} }`
	cu2.WriteByte(2)
	cu2.WriteString("CA\x00")
	cu2.WriteByte(13)
	binary.Write(&cu2, binary.LittleEndian, nsOutOfLineSpecOff)
	binary.Write(&cu2, binary.LittleEndian, lay.fnOutOfLineEA)
	binary.Write(&cu2, binary.LittleEndian, uint64(0x10))
	cu2.WriteByte(0) // end ns_out_of_line
	cu2.WriteByte(0) // end namespace CA

	// DIE: PointerType to CA::FACTION in CU 2 (abbrev 4)
	factionPtrOff := uint32(cu2.Len())
	cu2.WriteByte(4)
	cu2.WriteByte(8)
	binary.Write(&cu2, binary.LittleEndian, factionOff)

	// DIE: ConstType wrapping PointerType `CA::FACTION* const` in CU 2 (abbrev 25)
	factionConstPtrOff := uint32(cu2.Len())
	cu2.WriteByte(25)
	binary.Write(&cu2, binary.LittleEndian, factionPtrOff)

	// DIE: Declaration FormalParameter "amount" (hop 2 target for 2-hop param resolution) (abbrev 22)
	amountSpecOff := uint32(cu2.Len())
	cu2.WriteByte(22)
	cu2.WriteString("amount\x00")
	binary.Write(&cu2, binary.LittleEndian, intOff)

	// DIE: Intermediate abstract Subprogram for clamp_gold referencing clampGoldSpecOff (abbrev 17)
	clampGoldAbsOff := uint32(cu2.Len())
	cu2.WriteByte(17)
	binary.Write(&cu2, binary.LittleEndian, clampGoldSpecOff)
	// Intermediate FormalParameter referencing amountSpecOff (hop 1 target for 2-hop param resolution) (abbrev 21)
	amountAbsOff := uint32(cu2.Len())
	cu2.WriteByte(21)
	binary.Write(&cu2, binary.LittleEndian, amountSpecOff)
	cu2.WriteByte(0) // end clampGoldAbsOff

	// DIE: Intermediate abstract Subprogram for helper referencing helperSpecOff (abbrev 17)
	helperAbsOff := uint32(cu2.Len())
	cu2.WriteByte(17)
	binary.Write(&cu2, binary.LittleEndian, helperSpecOff)
	cu2.WriteByte(0) // end helperAbsOff

	// DIE: Concrete Subprogram for helper via 2-hop abstract_origin -> helperAbsOff -> helperSpecOff (abbrev 18)
	cu2.WriteByte(18)
	binary.Write(&cu2, binary.LittleEndian, helperAbsOff)
	binary.Write(&cu2, binary.LittleEndian, lay.fnHelperEA)
	binary.Write(&cu2, binary.LittleEndian, uint64(0x10))

	// DIE: Concrete Subprogram for add_gold referencing addGoldSpecOff (abbrev 13)
	cu2.WriteByte(13)
	binary.Write(&cu2, binary.LittleEndian, addGoldSpecOff)
	binary.Write(&cu2, binary.LittleEndian, lay.fnAddGoldEA)
	binary.Write(&cu2, binary.LittleEndian, uint64(0x20))

	// Param 0: "this" (CA::FACTION* const, DW_OP_reg5 / rdi) (abbrev 14)
	cu2.WriteByte(14)
	cu2.WriteString("this\x00")
	binary.Write(&cu2, binary.LittleEndian, factionConstPtrOff)
	cu2.Write([]byte{1, 0x55})

	// Param 1: "amount" resolved via 2-hop abstract_origin (amountAbsOff -> amountSpecOff) (abbrev 20)
	cu2.WriteByte(20)
	binary.Write(&cu2, binary.LittleEndian, amountAbsOff)
	cu2.Write([]byte{1, 0x54})

	// Nested concrete Lambda Subprogram inside add_gold BEFORE local_sum & inlined clamp_gold (abbrev 19)
	cu2.WriteByte(19)
	cu2.WriteString("operator()\x00")
	binary.Write(&cu2, binary.LittleEndian, lay.fnLambdaEA)
	binary.Write(&cu2, binary.LittleEndian, uint64(0x10))
	// Lambda param "closure" (must belong to fnLambdaEA, NOT fnAddGoldEA!) (abbrev 14)
	cu2.WriteByte(14)
	cu2.WriteString("closure\x00")
	binary.Write(&cu2, binary.LittleEndian, factionPtrOff)
	cu2.Write([]byte{1, 0x55})
	cu2.WriteByte(0) // end Lambda Subprogram

	// Local 0 (AFTER nested lambda closes!): "local_sum" (int, DW_OP_fbreg -8) (abbrev 15)
	cu2.WriteByte(15)
	cu2.WriteString("local_sum\x00")
	binary.Write(&cu2, binary.LittleEndian, intOff)
	cu2.Write([]byte{2, 0x91, 0x78})

	// Local 1: "neg_breg" (int, DW_OP_breg6 -16) (abbrev 15)
	cu2.WriteByte(15)
	cu2.WriteString("neg_breg\x00")
	binary.Write(&cu2, binary.LittleEndian, intOff)
	cu2.Write([]byte{2, 0x76, 0x70})

	// InlinedSubroutine (AFTER nested lambda closes!) referencing clampGoldAbsOff (2-hop -> clampGoldSpecOff) (abbrev 16)
	cu2.WriteByte(16)
	binary.Write(&cu2, binary.LittleEndian, clampGoldAbsOff)
	binary.Write(&cu2, binary.LittleEndian, lay.fnAddGoldEA+0x04)
	binary.Write(&cu2, binary.LittleEndian, uint64(0x0C))
	cu2.WriteByte(2)   // call_file = 2 (faction.cpp)
	cu2.WriteByte(105) // call_line = 105

	// Local inside InlinedSubroutine: "inlined_tmp" (must be ignored by inlineDepth==0 filter!) (abbrev 15)
	cu2.WriteByte(15)
	cu2.WriteString("inlined_tmp\x00")
	binary.Write(&cu2, binary.LittleEndian, intOff)
	cu2.Write([]byte{1, 0x50})

	cu2.WriteByte(0) // end InlinedSubroutine
	cu2.WriteByte(0) // end Concrete Subprogram
	cu2.WriteByte(0) // end CU 2

	cu2Bytes := cu2.Bytes()
	binary.LittleEndian.PutUint32(cu2Bytes[0:4], uint32(len(cu2Bytes)-4))

	info = append(append([]byte{}, cu1Bytes...), cu2Bytes...)
	return abbrev, info, line
}

func buildDWARFLineTable(fnAddr uint64) []byte {
	var body bytes.Buffer
	var hdr bytes.Buffer
	hdr.WriteByte(1)   // minimum_instruction_length = 1
	hdr.WriteByte(1)   // maximum_operations_per_instruction = 1 (DWARF4)
	hdr.WriteByte(1)   // default_is_stmt = 1
	hdr.WriteByte(251) // line_base = -5
	hdr.WriteByte(14)  // line_range = 14
	hdr.WriteByte(13)  // opcode_base = 13
	// standard_opcode_lengths for opcodes 1..12
	hdr.Write([]byte{0, 1, 1, 1, 1, 0, 0, 0, 1, 0, 0, 1})
	// include_directories: "/src/campaign\0", terminated by 0
	hdr.WriteString("/src/campaign\x00")
	hdr.WriteByte(0)
	// file_names:
	//   file 1: "faction.h\0", dir_idx=1, mtime=0, size=0
	//   file 2: "faction.cpp\0", dir_idx=1, mtime=0, size=0
	hdr.WriteString("faction.h\x00")
	hdr.Write([]byte{1, 0, 0})
	hdr.WriteString("faction.cpp\x00")
	hdr.Write([]byte{1, 0, 0})
	hdr.WriteByte(0)

	hdrBytes := hdr.Bytes()

	var prog bytes.Buffer
	// Extended opcode DW_LNE_set_address (2): 0x00, len=9, 0x02, uint64(fnAddr)
	prog.Write([]byte{0x00, 9, 0x02})
	binary.Write(&prog, binary.LittleEndian, fnAddr)
	// DW_LNS_set_file (4): 2 (faction.cpp)
	prog.Write([]byte{0x04, 0x02})
	// DW_LNS_advance_line (3): +99 (from 1 to 100) -> sleb128(99) = 0xE3, 0x00
	prog.Write([]byte{0x03, 0xE3, 0x00})
	// DW_LNS_copy (1) -> emits row at fnAddr (file 2 = faction.cpp), line 100
	prog.WriteByte(0x01)
	// DW_LNS_advance_pc (2): +8 (into inlined range [fnAddr+4, fnAddr+16))
	prog.Write([]byte{0x02, 8})
	// DW_LNS_set_file (4): 1 (faction.h)
	prog.Write([]byte{0x04, 0x01})
	// DW_LNS_advance_line (3): -50 (to 50) -> sleb128(-50) = 0x4E
	prog.Write([]byte{0x03, 0x4E})
	// DW_LNS_copy (1) -> emits inlined row at fnAddr+8 (file 1 = faction.h), line 50 (must be excluded by inlineRanges!)
	prog.WriteByte(0x01)
	// DW_LNS_advance_pc (2): +8 (to fnAddr+16, after inlined range)
	prog.Write([]byte{0x02, 8})
	// DW_LNS_set_file (4): 2 (faction.cpp)
	prog.Write([]byte{0x04, 0x02})
	// DW_LNS_advance_line (3): +60 (to 110) -> sleb128(60) = 0x3C
	prog.Write([]byte{0x03, 0x3C})
	// DW_LNS_copy (1) -> emits row at fnAddr+16 (file 2 = faction.cpp), line 110
	prog.WriteByte(0x01)
	// DW_LNS_advance_pc (2): +16
	prog.Write([]byte{0x02, 16})
	// Extended opcode DW_LNE_end_sequence (1): 0x00, len=1, 0x01
	prog.Write([]byte{0x00, 1, 0x01})

	totalLen := uint32(2 + 4 + len(hdrBytes) + prog.Len())
	binary.Write(&body, binary.LittleEndian, totalLen)
	binary.Write(&body, binary.LittleEndian, uint16(4)) // version 4
	binary.Write(&body, binary.LittleEndian, uint32(len(hdrBytes)))
	body.Write(hdrBytes)
	body.Write(prog.Bytes())
	return body.Bytes()
}

func buildELFSymtab(lay syntheticELF64Layout, ztiVA uint64) (symtab, strtab []byte) {
	var st bytes.Buffer
	st.WriteByte(0)
	addStr := func(s string) uint32 {
		off := uint32(st.Len())
		st.WriteString(s)
		st.WriteByte(0)
		return off
	}
	nameZTV := addStr("_ZTVN2CA7FACTIONE")
	nameZTI := addStr("_ZTIN2CA7FACTIONE")
	nameHelper := addStr("_ZN2CA6helperEv")

	var sb bytes.Buffer
	// Symbol 0: null symbol (24 bytes)
	sb.Write(make([]byte, 24))

	writeSym := func(nameOff uint32, info byte, shndx uint16, val, size uint64) {
		var sym [24]byte
		binary.LittleEndian.PutUint32(sym[0:4], nameOff)
		sym[4] = info
		binary.LittleEndian.PutUint16(sym[6:8], shndx)
		binary.LittleEndian.PutUint64(sym[8:16], val)
		binary.LittleEndian.PutUint64(sym[16:24], size)
		sb.Write(sym[:])
	}
	// _ZTVN2CA7FACTIONE in .rodata (section 2), STT_OBJECT | STB_GLOBAL
	writeSym(nameZTV, byte(elf.STB_GLOBAL)<<4|byte(elf.STT_OBJECT), 2, lay.ztvEA, 56)
	// _ZTIN2CA7FACTIONE in .rodata (section 2), STT_OBJECT | STB_GLOBAL
	writeSym(nameZTI, byte(elf.STB_GLOBAL)<<4|byte(elf.STT_OBJECT), 2, ztiVA, 16)
	// _ZN2CA6helperEv in .text (section 1), STT_FUNC | STB_GLOBAL
	writeSym(nameHelper, byte(elf.STB_GLOBAL)<<4|byte(elf.STT_FUNC), 1, lay.fnHelperEA, 16)

	return sb.Bytes(), st.Bytes()
}

func writeSyntheticELF64File(
	t *testing.T,
	outPath string,
	imageBase uint64,
	textVA uint64,
	text []byte,
	rodataVA uint64,
	rodata []byte,
	symtab []byte,
	strtab []byte,
	debugAbbrev []byte,
	debugInfo []byte,
	debugLine []byte,
) {
	t.Helper()
	var shstr bytes.Buffer
	shstr.WriteByte(0)
	addShStr := func(s string) uint32 {
		off := uint32(shstr.Len())
		shstr.WriteString(s)
		shstr.WriteByte(0)
		return off
	}
	nText := addShStr(".text")
	nRodata := addShStr(".rodata")
	nSymtab := addShStr(".symtab")
	nStrtab := addShStr(".strtab")
	nDynsym := addShStr(".dynsym")
	nDynstr := addShStr(".dynstr")
	nDynamic := addShStr(".dynamic")
	nRelaPlt := addShStr(".rela.plt")
	nAbbrev := addShStr(".debug_abbrev")
	nInfo := addShStr(".debug_info")
	nLine := addShStr(".debug_line")
	nShstrtab := addShStr(".shstrtab")
	shstrBytes := shstr.Bytes()

	// Build .dynstr, .dynsym, .dynamic (with DT_NEEDED), and .rela.plt for unversioned import test:
	var dynstr bytes.Buffer
	dynstr.WriteByte(0)
	offLibPthread := uint32(dynstr.Len())
	dynstr.WriteString("libpthread.so.0\x00")
	offPthreadLock := uint32(dynstr.Len())
	dynstr.WriteString("pthread_mutex_lock\x00")
	dynstrBytes := dynstr.Bytes()

	var dynsym bytes.Buffer
	dynsym.Write(make([]byte, 24)) // Symbol 0: null
	var dsym1 [24]byte
	binary.LittleEndian.PutUint32(dsym1[0:4], offPthreadLock)
	dsym1[4] = byte(elf.STB_GLOBAL)<<4 | byte(elf.STT_FUNC)
	dynsym.Write(dsym1[:])
	dynsymBytes := dynsym.Bytes()

	var dynamic bytes.Buffer
	// DT_NEEDED (1) -> offLibPthread, DT_NULL (0) -> 0
	var dtNeeded, dtNull [16]byte
	binary.LittleEndian.PutUint64(dtNeeded[0:8], uint64(elf.DT_NEEDED))
	binary.LittleEndian.PutUint64(dtNeeded[8:16], uint64(offLibPthread))
	dynamic.Write(dtNeeded[:])
	dynamic.Write(dtNull[:])
	dynamicBytes := dynamic.Bytes()

	var relaPlt bytes.Buffer
	var rela [24]byte
	binary.LittleEndian.PutUint64(rela[0:8], 0x403000) // gotPthreadVA
	binary.LittleEndian.PutUint64(rela[8:16], (uint64(1)<<32)|uint64(elf.R_X86_64_JMP_SLOT))
	relaPlt.Write(rela[:])
	relaPltBytes := relaPlt.Bytes()

	type secDef struct {
		nameOff uint32
		typ     uint32
		flags   uint64
		addr    uint64
		link    uint32
		info    uint32
		entsize uint64
		data    []byte
		offset  uint64
	}
	secs := []secDef{
		{}, // 0: SHT_NULL
		{nameOff: nText, typ: uint32(elf.SHT_PROGBITS), flags: uint64(elf.SHF_ALLOC | elf.SHF_EXECINSTR), addr: textVA, data: text},
		{nameOff: nRodata, typ: uint32(elf.SHT_PROGBITS), flags: uint64(elf.SHF_ALLOC), addr: rodataVA, data: rodata},
		{nameOff: nSymtab, typ: uint32(elf.SHT_SYMTAB), link: 4, info: 1, entsize: 24, data: symtab},
		{nameOff: nStrtab, typ: uint32(elf.SHT_STRTAB), data: strtab},
		{nameOff: nDynsym, typ: uint32(elf.SHT_DYNSYM), flags: uint64(elf.SHF_ALLOC), link: 6, info: 1, entsize: 24, data: dynsymBytes},
		{nameOff: nDynstr, typ: uint32(elf.SHT_STRTAB), flags: uint64(elf.SHF_ALLOC), data: dynstrBytes},
		{nameOff: nDynamic, typ: uint32(elf.SHT_DYNAMIC), flags: uint64(elf.SHF_ALLOC), link: 6, entsize: 16, data: dynamicBytes},
		{nameOff: nRelaPlt, typ: uint32(elf.SHT_RELA), flags: uint64(elf.SHF_ALLOC), link: 5, entsize: 24, data: relaPltBytes},
		{nameOff: nAbbrev, typ: uint32(elf.SHT_PROGBITS), data: debugAbbrev},
		{nameOff: nInfo, typ: uint32(elf.SHT_PROGBITS), data: debugInfo},
		{nameOff: nLine, typ: uint32(elf.SHT_PROGBITS), data: debugLine},
		{nameOff: nShstrtab, typ: uint32(elf.SHT_STRTAB), data: shstrBytes},
	}

	const (
		ehdrSize = 64
		phdrSize = 56
		shdrSize = 64
	)
	curOff := uint64(ehdrSize + phdrSize)
	for i := 1; i < len(secs); i++ {
		for curOff%16 != 0 {
			curOff++
		}
		secs[i].offset = curOff
		curOff += uint64(len(secs[i].data))
	}
	for curOff%16 != 0 {
		curOff++
	}
	shdrOff := curOff

	var f bytes.Buffer
	var ehdr [ehdrSize]byte
	copy(ehdr[0:4], []byte{0x7f, 'E', 'L', 'F'})
	ehdr[4] = byte(elf.ELFCLASS64)
	ehdr[5] = byte(elf.ELFDATA2LSB)
	ehdr[6] = byte(elf.EV_CURRENT)
	binary.LittleEndian.PutUint16(ehdr[16:18], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(ehdr[18:20], uint16(elf.EM_X86_64))
	binary.LittleEndian.PutUint32(ehdr[20:24], uint32(elf.EV_CURRENT))
	binary.LittleEndian.PutUint64(ehdr[24:32], textVA)
	binary.LittleEndian.PutUint64(ehdr[32:40], ehdrSize) // e_phoff
	binary.LittleEndian.PutUint64(ehdr[40:48], shdrOff)  // e_shoff
	binary.LittleEndian.PutUint16(ehdr[52:54], ehdrSize)
	binary.LittleEndian.PutUint16(ehdr[54:56], phdrSize)
	binary.LittleEndian.PutUint16(ehdr[56:58], 1) // e_phnum = 1
	binary.LittleEndian.PutUint16(ehdr[58:60], shdrSize)
	binary.LittleEndian.PutUint16(ehdr[60:62], uint16(len(secs)))
	binary.LittleEndian.PutUint16(ehdr[62:64], uint16(len(secs)-1)) // e_shstrndx
	f.Write(ehdr[:])

	var phdr [phdrSize]byte
	binary.LittleEndian.PutUint32(phdr[0:4], uint32(elf.PT_LOAD))
	binary.LittleEndian.PutUint32(phdr[4:8], uint32(elf.PF_R|elf.PF_X))
	binary.LittleEndian.PutUint64(phdr[8:16], 0)
	binary.LittleEndian.PutUint64(phdr[16:24], imageBase)
	binary.LittleEndian.PutUint64(phdr[24:32], imageBase)
	binary.LittleEndian.PutUint64(phdr[32:40], 0x4000)
	binary.LittleEndian.PutUint64(phdr[40:48], 0x4000)
	binary.LittleEndian.PutUint64(phdr[48:56], 0x1000)
	f.Write(phdr[:])

	for i := 1; i < len(secs); i++ {
		for uint64(f.Len()) < secs[i].offset {
			f.WriteByte(0)
		}
		f.Write(secs[i].data)
	}
	for uint64(f.Len()) < shdrOff {
		f.WriteByte(0)
	}

	for _, s := range secs {
		var sh [shdrSize]byte
		binary.LittleEndian.PutUint32(sh[0:4], s.nameOff)
		binary.LittleEndian.PutUint32(sh[4:8], s.typ)
		binary.LittleEndian.PutUint64(sh[8:16], s.flags)
		binary.LittleEndian.PutUint64(sh[16:24], s.addr)
		binary.LittleEndian.PutUint64(sh[24:32], s.offset)
		binary.LittleEndian.PutUint64(sh[32:40], uint64(len(s.data)))
		binary.LittleEndian.PutUint32(sh[40:44], s.link)
		binary.LittleEndian.PutUint32(sh[44:48], s.info)
		binary.LittleEndian.PutUint64(sh[48:56], 8)
		binary.LittleEndian.PutUint64(sh[56:64], s.entsize)
		f.Write(sh[:])
	}

	if err := os.WriteFile(outPath, f.Bytes(), 0o644); err != nil {
		t.Fatalf("write synthetic ELF64: %v", err)
	}
}

func TestDWARFSLEB128AndLocSummary(t *testing.T) {
	slebCases := []struct {
		name  string
		bytes []byte
		want  int64
		wantN int
	}{
		{name: "zero", bytes: []byte{0x00}, want: 0, wantN: 1},
		{name: "minus_8", bytes: []byte{0x78}, want: -8, wantN: 1},
		{name: "minus_16", bytes: []byte{0x70}, want: -16, wantN: 1},
		{name: "plus_63", bytes: []byte{0x3F}, want: 63, wantN: 1},
		{name: "minus_64", bytes: []byte{0x40}, want: -64, wantN: 1},
		{name: "minus_128", bytes: []byte{0x80, 0x7F}, want: -128, wantN: 2},
		{name: "minus_256", bytes: []byte{0x80, 0x7E}, want: -256, wantN: 2},
		{name: "plus_128", bytes: []byte{0x80, 0x01}, want: 128, wantN: 2},
		{name: "truncated", bytes: []byte{0x80}, want: 0, wantN: 0},
	}
	for _, tc := range slebCases {
		got, n := decodeSLEB128(tc.bytes)
		if got != tc.want || n != tc.wantN {
			t.Errorf("decodeSLEB128(%s) = (%d, %d), want (%d, %d)", tc.name, got, n, tc.want, tc.wantN)
		}
	}

	locCases := []struct {
		name string
		expr []byte
		want string
	}{
		{name: "fbreg_neg16", expr: []byte{0x91, 0x70}, want: "fbreg-16"},
		{name: "fbreg_neg256", expr: []byte{0x91, 0x80, 0x7E}, want: "fbreg-256"},
		{name: "breg6_neg128", expr: []byte{0x76, 0x80, 0x7F}, want: "breg6-128"},
		{name: "breg7_pos16", expr: []byte{0x77, 0x10}, want: "breg7+16"},
		{name: "fbreg_trailing_deref", expr: []byte{0x91, 0x70, 0x06}, want: "expr:917006"},
		{name: "fbreg_truncated", expr: []byte{0x91}, want: "expr:91"},
	}
	for _, tc := range locCases {
		f := &dwarf.Field{Val: tc.expr}
		if got := summarizeDWARFLocField(f); got != tc.want {
			t.Errorf("summarizeDWARFLocField(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestExtractELF64_DisabledFormatExtractorStillPopulatesStringsAndMeta(t *testing.T) {
	ctx := context.Context(t.Context())
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "Attila")
	bronzeDBPath := filepath.Join(tmpDir, "bronze_no_elf_format.sqlite")

	fix := buildSyntheticELF64WithTwoCUDWARF4(t, binPath)

	res, err := ExtractTarget(ctx, Options{
		Target:             workspace.TargetAttila64,
		BinaryPath:         binPath,
		BronzeDBPath:       bronzeDBPath,
		DisabledExtractors: []string{"elf64.format", "elf64.dwarf4"},
	})
	if err != nil {
		t.Fatalf("ExtractTarget failed: %v", err)
	}
	if res.ImageBase != fix.imageBase {
		t.Fatalf("ImageBase = 0x%x, want 0x%x", res.ImageBase, fix.imageBase)
	}
	if res.Counts["strings"] == 0 {
		t.Fatalf("expected data.strings to still extract strings when elf64.format is disabled")
	}

	db, err := extractdb.OpenReadOnly(bronzeDBPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	meta := make(map[string]string)
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM meta`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		meta[k] = v
	}
	rows.Close()

	if meta[extractdb.MetaFormat] != extractdb.FormatELF64 || meta[extractdb.MetaArch] != extractdb.ArchX86_64 || meta[extractdb.MetaImageBase] != "0x400000" {
		t.Fatalf("unexpected meta when elf64.format disabled: %+v", meta)
	}
}

func TestResolveDWARFMemberLayout_BitfieldsAndLocations(t *testing.T) {
	mkEntry := func(fields ...dwarf.Field) *dwarf.Entry {
		return &dwarf.Entry{Tag: dwarf.TagMember, Field: fields}
	}

	cases := []struct {
		name           string
		entry          *dwarf.Entry
		parentTag      dwarf.Tag
		memberTypeSize int64
		byteOrder      binary.ByteOrder
		wantByteOff    int64
		wantHasByteOff bool
		wantBitOff     int
		wantBitSize    int
	}{
		// Real attila_64 CA::Pixel1555 (16-bit LE bitfields using legacy DW_AT_bit_offset):
		// MASK_BLUE=0x001f (bits 0..4), MASK_GREEN=0x03e0 (bits 5..9),
		// MASK_RED=0x7c00 (bits 10..14), MASK_ALPHA=0x8000 (bit 15).
		{
			name: "pixel1555_blue",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrByteSize, Val: int64(2)},
				dwarf.Field{Attr: dwarf.AttrBitSize, Val: int64(5)},
				dwarf.Field{Attr: dwarf.AttrBitOffset, Val: int64(11)},
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: int64(0)},
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 2, byteOrder: binary.LittleEndian,
			wantByteOff: 0, wantHasByteOff: true, wantBitOff: 0, wantBitSize: 5,
		},
		{
			name: "pixel1555_green",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrByteSize, Val: int64(2)},
				dwarf.Field{Attr: dwarf.AttrBitSize, Val: int64(5)},
				dwarf.Field{Attr: dwarf.AttrBitOffset, Val: int64(6)},
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: int64(0)},
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 2, byteOrder: binary.LittleEndian,
			wantByteOff: 0, wantHasByteOff: true, wantBitOff: 5, wantBitSize: 5,
		},
		{
			name: "pixel1555_red",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrByteSize, Val: int64(2)},
				dwarf.Field{Attr: dwarf.AttrBitSize, Val: int64(5)},
				dwarf.Field{Attr: dwarf.AttrBitOffset, Val: int64(1)},
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: int64(0)},
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 2, byteOrder: binary.LittleEndian,
			wantByteOff: 1, wantHasByteOff: true, wantBitOff: 2, wantBitSize: 5,
		},
		{
			name: "pixel1555_opacity",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrByteSize, Val: int64(2)},
				dwarf.Field{Attr: dwarf.AttrBitSize, Val: int64(1)},
				dwarf.Field{Attr: dwarf.AttrBitOffset, Val: int64(0)},
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: int64(0)},
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 2, byteOrder: binary.LittleEndian,
			wantByteOff: 1, wantHasByteOff: true, wantBitOff: 7, wantBitSize: 1,
		},
		// Go debug/dwarf doc 4-byte int bitfield example: struct S { int q; int j:5; int k:6; int m:5; int n:8; }.
		{
			name: "int32_bitfield_m_cross_byte",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrByteSize, Val: int64(4)},
				dwarf.Field{Attr: dwarf.AttrBitSize, Val: int64(5)},
				dwarf.Field{Attr: dwarf.AttrBitOffset, Val: int64(16)},
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: int64(4)},
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 4, byteOrder: binary.LittleEndian,
			// lsb = 32 - 16 - 5 = 11 -> byte 4 + 1 = 5, bit 3 (total bit 43 == DW_AT_data_bit_offset 43)
			wantByteOff: 5, wantHasByteOff: true, wantBitOff: 3, wantBitSize: 5,
		},
		// Fallback to memberTypeSize when DW_AT_byte_size is omitted.
		{
			name: "bit_offset_fallback_to_member_type_size",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrBitSize, Val: int64(3)},
				dwarf.Field{Attr: dwarf.AttrBitOffset, Val: int64(29)},
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: int64(8)},
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 4, byteOrder: binary.LittleEndian,
			wantByteOff: 8, wantHasByteOff: true, wantBitOff: 0, wantBitSize: 3,
		},
		// GCC 4.x non-bitfield quirk (attila_64 EMPIRE::EMPIRE_APP_MODULE::m_app_window):
		// DW_AT_byte_size=0, DW_AT_bit_size=320, DW_AT_bit_offset=0xfffffffffffffec0 (-320), loc=8.
		{
			name: "gcc_zero_byte_size_negative_bit_offset_quirk",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrByteSize, Val: int64(0)},
				dwarf.Field{Attr: dwarf.AttrBitSize, Val: int64(320)},
				dwarf.Field{Attr: dwarf.AttrBitOffset, Val: uint64(0xfffffffffffffec0)},
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: int64(8)},
			),
			parentTag: dwarf.TagClassType, memberTypeSize: 40, byteOrder: binary.LittleEndian,
			wantByteOff: 8, wantHasByteOff: true, wantBitOff: 0, wantBitSize: 0,
		},
		// DWARF4 DW_AT_data_bit_offset combined with DW_AT_data_member_location must compute exact byte+bit.
		{
			name: "dwarf4_data_bit_offset_with_member_loc",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: int64(20)},
				dwarf.Field{Attr: dwarf.AttrBitSize, Val: int64(1)},
				dwarf.Field{Attr: dwarf.AttrDataBitOffset, Val: int64(171)}, // byte 21, bit 3
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 4, byteOrder: binary.LittleEndian,
			wantByteOff: 21, wantHasByteOff: true, wantBitOff: 3, wantBitSize: 1,
		},
		// Union member omitting DW_AT_data_member_location legitimately lives at offset 0.
		{
			name: "union_omitted_location_defaults_to_zero",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrName, Val: "as_int"},
			),
			parentTag: dwarf.TagUnionType, memberTypeSize: 4, byteOrder: binary.LittleEndian,
			wantByteOff: 0, wantHasByteOff: true, wantBitOff: 0, wantBitSize: 0,
		},
		// Struct member with unrecognized complex DW_AT_data_member_location must preserve NULL (HasByteOffset=false).
		{
			name: "struct_unparsed_location_expr_is_null",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: []byte{0x12, 0x06}}, // DW_OP_dup; DW_OP_deref
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 8, byteOrder: binary.LittleEndian,
			wantByteOff: 0, wantHasByteOff: false, wantBitOff: 0, wantBitSize: 0,
		},
		// Struct member with DW_OP_const1u/2u/4u/constu + DW_OP_plus.
		{
			name: "struct_const1u_plus",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: []byte{0x08, 0x40, 0x22}},
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 4, byteOrder: binary.LittleEndian,
			wantByteOff: 64, wantHasByteOff: true, wantBitOff: 0, wantBitSize: 0,
		},
		{
			name: "struct_const2u_plus",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: []byte{0x0a, 0x10, 0x01, 0x22}}, // 0x0110 = 272
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 4, byteOrder: binary.LittleEndian,
			wantByteOff: 272, wantHasByteOff: true, wantBitOff: 0, wantBitSize: 0,
		},
		{
			name: "struct_const4u_plus",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: []byte{0x0c, 0x00, 0x00, 0x01, 0x00, 0x22}}, // 0x10000 = 65536
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 4, byteOrder: binary.LittleEndian,
			wantByteOff: 65536, wantHasByteOff: true, wantBitOff: 0, wantBitSize: 0,
		},
		{
			name: "struct_constu_plus",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: []byte{0x10, 0x80, 0x02, 0x22}}, // ULEB128 256
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 4, byteOrder: binary.LittleEndian,
			wantByteOff: 256, wantHasByteOff: true, wantBitOff: 0, wantBitSize: 0,
		},
		// Big-endian DW_AT_bit_offset.
		{
			name: "big_endian_bit_offset",
			entry: mkEntry(
				dwarf.Field{Attr: dwarf.AttrByteSize, Val: int64(4)},
				dwarf.Field{Attr: dwarf.AttrBitSize, Val: int64(5)},
				dwarf.Field{Attr: dwarf.AttrBitOffset, Val: int64(11)},
				dwarf.Field{Attr: dwarf.AttrDataMemberLoc, Val: int64(4)},
			),
			parentTag: dwarf.TagStructType, memberTypeSize: 4, byteOrder: binary.BigEndian,
			wantByteOff: 5, wantHasByteOff: true, wantBitOff: 3, wantBitSize: 5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			byteOff, hasByteOff, bitOff, bitSize := resolveDWARFMemberLayout(tc.entry, tc.parentTag, tc.memberTypeSize, tc.byteOrder)
			if byteOff != tc.wantByteOff || hasByteOff != tc.wantHasByteOff || bitOff != tc.wantBitOff || bitSize != tc.wantBitSize {
				t.Fatalf("resolveDWARFMemberLayout() = (byteOff=%d, hasByteOff=%v, bitOff=%d, bitSize=%d), want (%d, %v, %d, %d)",
					byteOff, hasByteOff, bitOff, bitSize,
					tc.wantByteOff, tc.wantHasByteOff, tc.wantBitOff, tc.wantBitSize)
			}
		})
	}
}
