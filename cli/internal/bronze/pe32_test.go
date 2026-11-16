package bronze

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"unicode/utf16"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

func TestExtractPE32_SyntheticFixture(t *testing.T) {
	ctx := context.Context(t.Context())
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "empire.retail.dll")
	factsPath := filepath.Join(tmpDir, "facts.sqlite")
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	fix := buildSyntheticPE32DLL(t, binPath)

	res, err := ExtractTarget(ctx, Options{
		Target:       workspace.TargetAttila32,
		BinaryPath:   binPath,
		FactsDBPath:  factsPath,
		ManifestPath: manifestPath,
	})
	if err != nil {
		t.Fatalf("ExtractTarget failed: %v", err)
	}
	if res.ImageBase != uint64(fix.imageBase) {
		t.Fatalf("ImageBase = 0x%x, want 0x%x", res.ImageBase, fix.imageBase)
	}

	db, err := extractdb.OpenReadOnly(factsPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	// 1. Verify imports (both named and ordinal).
	impRows, err := db.QueryContext(ctx, `SELECT iat_va, dll_name, sym_name, ordinal FROM imports ORDER BY iat_va`)
	if err != nil {
		t.Fatalf("query imports: %v", err)
	}
	type impRec struct {
		iat uint64
		dll string
		sym string
		ord int
	}
	var imps []impRec
	for impRows.Next() {
		var r impRec
		var iat int64
		if err := impRows.Scan(&iat, &r.dll, &r.sym, &r.ord); err != nil {
			t.Fatal(err)
		}
		r.iat = uint64(iat)
		imps = append(imps, r)
	}
	impRows.Close()
	if len(imps) != 2 {
		t.Fatalf("len(imports) = %d, want 2: %+v", len(imps), imps)
	}
	if imps[0].iat != uint64(fix.iatNamedVA) || imps[0].dll != "KERNEL32.dll" || imps[0].sym != "GetTickCount" || imps[0].ord != 42 {
		t.Errorf("unexpected named import: %+v", imps[0])
	}
	if imps[1].iat != uint64(fix.iatOrdVA) || imps[1].dll != "KERNEL32.dll" || imps[1].sym != "ord_77" || imps[1].ord != 77 {
		t.Errorf("unexpected ordinal import: %+v", imps[1])
	}

	// 2. Verify exports (forwarded export must be excluded!).
	expRows, err := db.QueryContext(ctx, `SELECT ea, name, ordinal FROM exports ORDER BY ordinal`)
	if err != nil {
		t.Fatalf("query exports: %v", err)
	}
	type expRec struct {
		ea   uint64
		name string
		ord  int
	}
	var exps []expRec
	for expRows.Next() {
		var r expRec
		var ea int64
		if err := expRows.Scan(&ea, &r.name, &r.ord); err != nil {
			t.Fatal(err)
		}
		r.ea = uint64(ea)
		exps = append(exps, r)
	}
	expRows.Close()
	if len(exps) != 1 {
		t.Fatalf("len(exports) = %d (forwarded export should be excluded), got %+v", len(exps), exps)
	}
	if exps[0].ea != uint64(fix.fnExportCtorVA) || exps[0].name != "CreateFaction" || exps[0].ord != 1 {
		t.Errorf("unexpected export: %+v", exps[0])
	}

	// 3. Verify packed /GR- vtables split cleanly and switch jump table (FF 24 85) is rejected.
	vtRows, err := db.QueryContext(ctx, `SELECT vtable_ea, slot, target_ea FROM vtables ORDER BY vtable_ea, slot`)
	if err != nil {
		t.Fatalf("query vtables: %v", err)
	}
	type vtRec struct {
		vtEA   uint64
		slot   int
		target uint64
	}
	var vts []vtRec
	for vtRows.Next() {
		var r vtRec
		var vtEA, target int64
		if err := vtRows.Scan(&vtEA, &r.slot, &target); err != nil {
			t.Fatal(err)
		}
		r.vtEA = uint64(vtEA)
		r.target = uint64(target)
		vts = append(vts, r)
	}
	vtRows.Close()

	wantVts := []vtRec{
		{vtEA: uint64(fix.vt1VA), slot: 0, target: uint64(fix.fnVirt0VA)},
		{vtEA: uint64(fix.vt1VA), slot: 1, target: uint64(fix.fnVirt1VA)},
		{vtEA: uint64(fix.vt2VA), slot: 0, target: uint64(fix.fnVirt2VA)},
		{vtEA: uint64(fix.vt2VA), slot: 1, target: uint64(fix.fnVirt0VA)}, // shared ICF slot
	}
	if len(vts) != len(wantVts) {
		t.Fatalf("vtables = %+v, want %+v", vts, wantVts)
	}
	for i := range wantVts {
		if vts[i] != wantVts[i] {
			t.Errorf("vtable[%d] = %+v, want %+v", i, vts[i], wantVts[i])
		}
	}

	// 4. Verify lua_regs (both luaL_Reg 8-byte and Lunar<T> 12-byte tables).
	lrRows, err := db.QueryContext(ctx, `SELECT table_ea, method_name, func_ea FROM lua_regs ORDER BY table_ea, method_name`)
	if err != nil {
		t.Fatalf("query lua_regs: %v", err)
	}
	var luaMethods []string
	for lrRows.Next() {
		var tEA, fEA int64
		var mName string
		if err := lrRows.Scan(&tEA, &mName, &fEA); err != nil {
			t.Fatal(err)
		}
		luaMethods = append(luaMethods, mName)
	}
	lrRows.Close()
	if len(luaMethods) != 6 {
		t.Fatalf("lua_regs methods = %v, want 6 methods (3 luaL_Reg + 3 Lunar)", luaMethods)
	}

	// 5. Verify LoadSeedsFromFacts and function attributes (thunks, calling_conv, push ecx local rejection, ICF flag).
	loadedSeeds, err := LoadSeedsFromFacts(ctx, db)
	if err != nil {
		t.Fatalf("LoadSeedsFromFacts: %v", err)
	}
	if len(loadedSeeds) != len(res.Seeds) {
		t.Fatalf("len(loadedSeeds)=%d != len(res.Seeds)=%d", len(loadedSeeds), len(res.Seeds))
	}
	seedByEA := make(map[uint64]FunctionSeed, len(loadedSeeds))
	for _, s := range loadedSeeds {
		seedByEA[s.EA] = s
	}

	// Check FF 25 IAT thunk.
	thunkIAT, ok := seedByEA[uint64(fix.fnThunkIATVA)]
	if !ok {
		t.Fatalf("missing IAT thunk seed at 0x%x", fix.fnThunkIATVA)
	}
	if thunkIAT.Flags&extractdb.FuncFlagThunk == 0 || thunkIAT.ThunkTargetEA != uint64(fix.iatNamedVA) {
		t.Errorf("IAT thunk seed = %+v, want FuncFlagThunk and ThunkTargetEA=0x%x", thunkIAT, fix.iatNamedVA)
	}

	// Check E9 rel32 thunk.
	thunkJmp, ok := seedByEA[uint64(fix.fnThunkJmpVA)]
	if !ok {
		t.Fatalf("missing E9 thunk seed at 0x%x", fix.fnThunkJmpVA)
	}
	if thunkJmp.Flags&extractdb.FuncFlagThunk == 0 || thunkJmp.ThunkTargetEA != uint64(fix.fnPushECXLocalVA) {
		t.Errorf("E9 thunk seed = %+v, want FuncFlagThunk and ThunkTargetEA=0x%x", thunkJmp, fix.fnPushECXLocalVA)
	}

	// Check standalone `push ecx` local allocation + `ret 4` -> must be `__stdcall` (NOT `__thiscall`!).
	fnStdcall, ok := seedByEA[uint64(fix.fnPushECXLocalVA)]
	if !ok {
		t.Fatalf("missing stdcall seed at 0x%x", fix.fnPushECXLocalVA)
	}
	if fnStdcall.CallingConv != "__stdcall" || fnStdcall.StackPurge != 4 {
		t.Errorf("push-ecx local function = %+v, want __stdcall with StackPurge=4", fnStdcall)
	}

	// Check luaL_Reg method (RET 0, no ECX read) -> `__cdecl` (`bronze_lua_reg`).
	fnLuaC, ok := seedByEA[uint64(fix.fnLuaCdeclVA)]
	if !ok {
		t.Fatalf("missing luaL_Reg seed at 0x%x", fix.fnLuaCdeclVA)
	}
	if fnLuaC.CallingConv != "__cdecl" || fnLuaC.CCSource != extractdb.CCSourceLuaReg {
		t.Errorf("luaL_Reg function = %+v, want __cdecl / %s", fnLuaC, extractdb.CCSourceLuaReg)
	}

	// Check Lunar<T> method (reads [ecx+4], RET 4) -> `__thiscall` (`bronze_lua_reg`).
	fnLunar, ok := seedByEA[uint64(fix.fnLunarMethodVA)]
	if !ok {
		t.Fatalf("missing Lunar seed at 0x%x", fix.fnLunarMethodVA)
	}
	if fnLunar.CallingConv != "__thiscall" || fnLunar.CCSource != extractdb.CCSourceLuaReg || fnLunar.StackPurge != 4 {
		t.Errorf("Lunar<T> function = %+v, want __thiscall / %s / StackPurge=4", fnLunar, extractdb.CCSourceLuaReg)
	}

	// Check ICF shared virtual method (referenced by vt1[0] and vt2[1]).
	fnVirt0, ok := seedByEA[uint64(fix.fnVirt0VA)]
	if !ok {
		t.Fatalf("missing virtual method seed at 0x%x", fix.fnVirt0VA)
	}
	if fnVirt0.Flags&extractdb.FuncFlagICFCandidate == 0 || fnVirt0.CallingConv != "__thiscall" {
		t.Errorf("shared virtual method = %+v, want FuncFlagICFCandidate & __thiscall", fnVirt0)
	}

	// 6. Verify UTF-16LE strings: valid UTF-16LE extracted, .reloc-overlapping UTF-16LE rejected, ASCII string xref counted.
	var validUTF16Count, relocUTF16Count, asciiDFCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM strings WHERE encoding = 'utf16le' AND value = 'WarscapeWideText'`).Scan(&validUTF16Count); err != nil {
		t.Fatal(err)
	}
	if validUTF16Count != 1 {
		t.Errorf("validUTF16Count = %d, want 1", validUTF16Count)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM strings WHERE value = 'RelocFakeWide'`).Scan(&relocUTF16Count); err != nil {
		t.Fatal(err)
	}
	if relocUTF16Count != 0 {
		t.Errorf("relocUTF16Count = %d, want 0 (.reloc-overlapping UTF-16LE must be rejected)", relocUTF16Count)
	}
	if err := db.QueryRowContext(ctx, `SELECT df_count FROM strings WHERE value = 'FACTION_TREASURY_LOG'`).Scan(&asciiDFCount); err != nil {
		t.Fatalf("lookup FACTION_TREASURY_LOG: %v", err)
	}
	if asciiDFCount != 1 {
		t.Errorf("FACTION_TREASURY_LOG df_count = %d, want 1", asciiDFCount)
	}

	// 7. Verify >260-instruction function has full size, RET 4 stack_purge, and string_xref after instruction 260.
	fnLarge, ok := seedByEA[uint64(fix.fnLargeFuncVA)]
	if !ok {
		t.Fatalf("missing large function seed at 0x%x", fix.fnLargeFuncVA)
	}
	if fnLarge.StackPurge != 4 || fnLarge.CallingConv != "__stdcall" {
		t.Errorf("large function seed = %+v, want __stdcall with StackPurge=4", fnLarge)
	}
	var largeSize, largePurge int
	if err := db.QueryRowContext(ctx, `SELECT size, stack_purge FROM functions WHERE ea = ?`, int64(fix.fnLargeFuncVA)).Scan(&largeSize, &largePurge); err != nil {
		t.Fatalf("query large function: %v", err)
	}
	if largeSize != int(fix.fnLargeFuncSize) || largePurge != 4 {
		t.Errorf("large function (size=%d, stack_purge=%d), want (size=%d, stack_purge=4)", largeSize, largePurge, fix.fnLargeFuncSize)
	}
	var largeStrDF int
	if err := db.QueryRowContext(ctx, `SELECT df_count FROM strings WHERE value = 'LARGE_FUNC_TAIL_XREF'`).Scan(&largeStrDF); err != nil {
		t.Fatalf("lookup LARGE_FUNC_TAIL_XREF: %v", err)
	}
	if largeStrDF != 1 {
		t.Errorf("LARGE_FUNC_TAIL_XREF df_count = %d, want 1", largeStrDF)
	}

	// 8. Verify multi-slot switch jump table does NOT create DiscoveryDataPtr functions for slot >= 1
	// (even when targets start with 33 C0 / 56 / E9).
	for idx, swVA := range fix.swCaseVAs {
		if s, exists := seedByEA[uint64(swVA)]; exists {
			t.Errorf("switch jump table slot %d target 0x%x was falsely seeded as function: %+v", idx+1, swVA, s)
		}
	}

	// 9. Verify `cmp [esp+4], 0; sete cl; movzx eax, cl; ret` classifies as __cdecl (NOT __thiscall).
	fnSete, ok := seedByEA[uint64(fix.fnSeteCdeclVA)]
	if !ok {
		t.Fatalf("missing sete function seed at 0x%x", fix.fnSeteCdeclVA)
	}
	if fnSete.CallingConv != "__cdecl" {
		t.Errorf("sete cl function CallingConv = %q, want __cdecl", fnSete.CallingConv)
	}

	// 10. Verify FLD [rdata_float] + CALL [iat_slot] emit AccessKindRead and 0 AccessKindWrite targeting .rdata.
	for _, targetVA := range []uint32{fix.rdataFloatVA, fix.iatNamedVA} {
		var kind string
		if err := db.QueryRowContext(ctx, `
			SELECT access_kind FROM data_xrefs WHERE func_ea = ? AND to_ea = ?
		`, int64(fix.fnSeteCdeclVA), int64(targetVA)).Scan(&kind); err != nil {
			t.Fatalf("lookup data_xref to 0x%x: %v", targetVA, err)
		}
		if kind != extractdb.AccessKindRead {
			t.Errorf("data_xref to 0x%x access_kind = %q, want %q", targetVA, kind, extractdb.AccessKindRead)
		}
	}
	var rdataWrites int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM data_xrefs dx
		JOIN sections s ON dx.to_ea >= s.va AND dx.to_ea < s.va + s.virtual_size
		WHERE s.name IN ('.rdata', '.rodata') AND dx.access_kind = 'write'
	`).Scan(&rdataWrites); err != nil {
		t.Fatal(err)
	}
	if rdataWrites != 0 {
		t.Errorf("rdata write data_xrefs = %d, want 0", rdataWrites)
	}
}

type syntheticPE32Layout struct {
	imageBase        uint32
	iatNamedVA       uint32
	iatOrdVA         uint32
	rdataFloatVA     uint32
	fnExportCtorVA   uint32
	fnVirt0VA        uint32
	fnVirt1VA        uint32
	fnVirt2VA        uint32
	fnLuaCdeclVA     uint32
	fnLunarMethodVA  uint32
	fnThunkIATVA     uint32
	fnThunkJmpVA     uint32
	fnPushECXLocalVA uint32
	fnSeteCdeclVA    uint32
	fnLargeFuncVA    uint32
	fnLargeFuncSize  uint32
	swCaseVAs        []uint32
	vt1VA            uint32
	vt2VA            uint32
}

func buildSyntheticPE32DLL(t *testing.T, outPath string) syntheticPE32Layout {
	t.Helper()
	const (
		imageBase  = 0x10000000
		textRVA    = 0x1000
		rdataRVA   = 0x2000
		dataRVA    = 0x3000
		relocRVA   = 0x4000
		secFileSz  = 0x1000
		headerSize = 0x400
	)

	text := make([]byte, secFileSz)
	for i := range text {
		text[i] = 0xCC
	}
	rdata := make([]byte, secFileSz)
	data := make([]byte, secFileSz)

	var relocRVAs []uint32
	addReloc := func(rva uint32) {
		relocRVAs = append(relocRVAs, rva)
	}

	// Layout .rdata strings and tables first so .text knows their VAs.
	rdPos := 0x400
	writeRdataCString := func(s string) uint32 {
		va := uint32(imageBase + rdataRVA + rdPos)
		copy(rdata[rdPos:], s)
		rdata[rdPos+len(s)] = 0
		rdPos += len(s) + 1
		for rdPos%4 != 0 {
			rdPos++
		}
		return va
	}

	strLogVA := writeRdataCString("FACTION_TREASURY_LOG")
	strLargeTailVA := writeRdataCString("LARGE_FUNC_TAIL_XREF")
	strLua1 := writeRdataCString("get_gold")
	strLua2 := writeRdataCString("set_gold")
	strLua3 := writeRdataCString("reset_gold")
	strLun1 := writeRdataCString("treasury")
	strLun2 := writeRdataCString("home_region")
	strLun3 := writeRdataCString("faction_key")

	// Valid UTF-16LE string in .rdata (no .reloc overlap).
	for rdPos%4 != 0 {
		rdPos++
	}
	for _, u := range utf16.Encode([]rune("WarscapeWideText")) {
		binary.LittleEndian.PutUint16(rdata[rdPos:rdPos+2], u)
		rdPos += 2
	}
	binary.LittleEndian.PutUint16(rdata[rdPos:rdPos+2], 0)
	rdPos += 4

	// Fake UTF-16LE string in .rdata WITH a .reloc entry overlapping it -> must be rejected!
	for rdPos%4 != 0 {
		rdPos++
	}
	fakeWideRVA := uint32(rdataRVA + rdPos)
	for _, u := range utf16.Encode([]rune("RelocFakeWide")) {
		binary.LittleEndian.PutUint16(rdata[rdPos:rdPos+2], u)
		rdPos += 2
	}
	binary.LittleEndian.PutUint16(rdata[rdPos:rdPos+2], 0)
	addReloc(fakeWideRVA + 4)
	rdPos += 4

	// Define function offsets in .text:
	const (
		offExportCtor   = 0x000
		offVirt0        = 0x040
		offVirt1        = 0x060
		offVirt2        = 0x080
		offLuaCdecl     = 0x0A0
		offLunarMethod  = 0x0C0
		offThunkIAT     = 0x0E0
		offThunkJmp     = 0x0F0
		offPushECXLocal = 0x100
		offSeteCdecl    = 0x120
		offSwCase1      = 0x150
		offSwCase2      = 0x160
		offSwCase3      = 0x170
		offLargeFunc    = 0x200
	)
	for rdPos%4 != 0 {
		rdPos++
	}
	rdataFloatVA := uint32(imageBase + rdataRVA + rdPos)
	binary.LittleEndian.PutUint32(rdata[rdPos:rdPos+4], 0x3F800000) // 1.0f
	rdPos += 4

	lay := syntheticPE32Layout{
		imageBase:        imageBase,
		iatNamedVA:       imageBase + rdataRVA + 0x100,
		iatOrdVA:         imageBase + rdataRVA + 0x104,
		rdataFloatVA:     rdataFloatVA,
		fnExportCtorVA:   imageBase + textRVA + offExportCtor,
		fnVirt0VA:        imageBase + textRVA + offVirt0,
		fnVirt1VA:        imageBase + textRVA + offVirt1,
		fnVirt2VA:        imageBase + textRVA + offVirt2,
		fnLuaCdeclVA:     imageBase + textRVA + offLuaCdecl,
		fnLunarMethodVA:  imageBase + textRVA + offLunarMethod,
		fnThunkIATVA:     imageBase + textRVA + offThunkIAT,
		fnThunkJmpVA:     imageBase + textRVA + offThunkJmp,
		fnPushECXLocalVA: imageBase + textRVA + offPushECXLocal,
		fnSeteCdeclVA:    imageBase + textRVA + offSeteCdecl,
		fnLargeFuncVA:    imageBase + textRVA + offLargeFunc,
		swCaseVAs: []uint32{
			imageBase + textRVA + offSwCase1,
			imageBase + textRVA + offSwCase2,
			imageBase + textRVA + offSwCase3,
		},
	}

	// Place packed /GR- vtables vt1 (2 slots) immediately followed by vt2 (2 slots) in .rdata:
	for rdPos%4 != 0 {
		rdPos++
	}
	vt1Off := rdPos
	lay.vt1VA = uint32(imageBase + rdataRVA + vt1Off)
	binary.LittleEndian.PutUint32(rdata[vt1Off:vt1Off+4], lay.fnVirt0VA)
	addReloc(uint32(rdataRVA + vt1Off))
	binary.LittleEndian.PutUint32(rdata[vt1Off+4:vt1Off+8], lay.fnVirt1VA)
	addReloc(uint32(rdataRVA + vt1Off + 4))

	vt2Off := vt1Off + 8
	lay.vt2VA = uint32(imageBase + rdataRVA + vt2Off)
	binary.LittleEndian.PutUint32(rdata[vt2Off:vt2Off+4], lay.fnVirt2VA)
	addReloc(uint32(rdataRVA + vt2Off))
	binary.LittleEndian.PutUint32(rdata[vt2Off+4:vt2Off+8], lay.fnVirt0VA)
	addReloc(uint32(rdataRVA + vt2Off + 4))
	rdPos = vt2Off + 12

	// Place a 4-slot switch jump table in .rdata referenced by `FF 24 85 <jmpTblVA>` ->
	// must NOT be emitted as vtable AND slots 1..3 (starting with 33 C0 / 56 / E9) must NOT be seeded as DiscoveryDataPtr!
	jmpTblOff := rdPos
	jmpTblVA := uint32(imageBase + rdataRVA + jmpTblOff)
	binary.LittleEndian.PutUint32(rdata[jmpTblOff:jmpTblOff+4], lay.fnVirt0VA)
	addReloc(uint32(rdataRVA + jmpTblOff))
	for idx, caseVA := range lay.swCaseVAs {
		slotOff := jmpTblOff + (idx+1)*4
		binary.LittleEndian.PutUint32(rdata[slotOff:slotOff+4], caseVA)
		addReloc(uint32(rdataRVA + slotOff))
	}
	rdPos = jmpTblOff + 20

	// Place luaL_Reg table (3 entries + 8-byte null terminator) in .rdata:
	luaTableOff := rdPos
	for idx, sVA := range []uint32{strLua1, strLua2, strLua3} {
		p := luaTableOff + idx*8
		binary.LittleEndian.PutUint32(rdata[p:p+4], sVA)
		addReloc(uint32(rdataRVA + p))
		binary.LittleEndian.PutUint32(rdata[p+4:p+8], lay.fnLuaCdeclVA)
		addReloc(uint32(rdataRVA + p + 4))
	}
	rdPos = luaTableOff + 4*8

	// Place Lunar<T> table (3 entries of 12 bytes + 12-byte null terminator) in .rdata:
	lunarTableOff := rdPos
	for idx, sVA := range []uint32{strLun1, strLun2, strLun3} {
		p := lunarTableOff + idx*12
		binary.LittleEndian.PutUint32(rdata[p:p+4], sVA)
		addReloc(uint32(rdataRVA + p))
		binary.LittleEndian.PutUint32(rdata[p+4:p+8], lay.fnLunarMethodVA)
		addReloc(uint32(rdataRVA + p + 4))
		binary.LittleEndian.PutUint32(rdata[p+8:p+12], 0) // adj == 0, non-relocated
	}
	rdPos = lunarTableOff + 4*12

	// Populate .text functions:
	// 1. fnExportCtor:
	//   55                push ebp
	//   8B EC             mov ebp, esp
	//   C7 01 <vt1VA>     mov dword ptr [ecx], vt1VA
	//   C7 41 04 <vt2VA>  mov dword ptr [ecx+4], vt2VA
	//   68 <strLogVA>     push strLogVA
	//   E8 <rel_ThunkIAT> call fnThunkIAT
	//   E8 <rel_ThunkJmp> call fnThunkJmp
	//   FF 24 85 <jmpTbl> jmp dword ptr [eax*4 + jmpTblVA]
	p := offExportCtor
	copy(text[p:], []byte{0x55, 0x8B, 0xEC})
	p += 3
	text[p] = 0xC7
	text[p+1] = 0x01
	binary.LittleEndian.PutUint32(text[p+2:p+6], lay.vt1VA)
	addReloc(uint32(textRVA + p + 2))
	p += 6
	text[p] = 0xC7
	text[p+1] = 0x41
	text[p+2] = 0x04
	binary.LittleEndian.PutUint32(text[p+3:p+7], lay.vt2VA)
	addReloc(uint32(textRVA + p + 3))
	p += 7
	text[p] = 0x68
	binary.LittleEndian.PutUint32(text[p+1:p+5], strLogVA)
	addReloc(uint32(textRVA + p + 1))
	p += 5
	text[p] = 0xE8
	binary.LittleEndian.PutUint32(text[p+1:p+5], uint32(int32(offThunkIAT-(p+5))))
	p += 5
	text[p] = 0xE8
	binary.LittleEndian.PutUint32(text[p+1:p+5], uint32(int32(offThunkJmp-(p+5))))
	p += 5
	text[p] = 0xE8
	binary.LittleEndian.PutUint32(text[p+1:p+5], uint32(int32(offLargeFunc-(p+5))))
	p += 5
	text[p] = 0xE8
	binary.LittleEndian.PutUint32(text[p+1:p+5], uint32(int32(offSeteCdecl-(p+5))))
	p += 5
	text[p] = 0xFF
	text[p+1] = 0x24
	text[p+2] = 0x85
	binary.LittleEndian.PutUint32(text[p+3:p+7], jmpTblVA)
	addReloc(uint32(textRVA + p + 3))

	// 2. fnVirt0: mov eax, [ecx+8]; ret (8B 41 08 C3)
	copy(text[offVirt0:], []byte{0x8B, 0x41, 0x08, 0xC3})
	// 3. fnVirt1: mov eax, [ecx+12]; ret 4 (8B 41 0C C2 04 00)
	copy(text[offVirt1:], []byte{0x8B, 0x41, 0x0C, 0xC2, 0x04, 0x00})
	// 4. fnVirt2: xor eax, eax; ret (33 C0 C3)
	copy(text[offVirt2:], []byte{0x33, 0xC0, 0xC3})
	// 5. fnLuaCdecl: push ebp; mov ebp, esp; xor eax, eax; pop ebp; ret (55 8B EC 33 C0 5D C3)
	copy(text[offLuaCdecl:], []byte{0x55, 0x8B, 0xEC, 0x33, 0xC0, 0x5D, 0xC3})
	// 6. fnLunarMethod: mov eax, [ecx+4]; ret 4 (8B 41 04 C2 04 00)
	copy(text[offLunarMethod:], []byte{0x8B, 0x41, 0x04, 0xC2, 0x04, 0x00})
	// 7. fnThunkIAT: FF 25 <iatNamedVA>
	text[offThunkIAT] = 0xFF
	text[offThunkIAT+1] = 0x25
	binary.LittleEndian.PutUint32(text[offThunkIAT+2:offThunkIAT+6], lay.iatNamedVA)
	addReloc(uint32(textRVA + offThunkIAT + 2))
	// 8. fnThunkJmp: E9 <rel32 to fnPushECXLocal>
	text[offThunkJmp] = 0xE9
	binary.LittleEndian.PutUint32(text[offThunkJmp+1:offThunkJmp+5], uint32(int32(offPushECXLocal-(offThunkJmp+5))))
	// 9. fnPushECXLocal: push ebp; mov ebp, esp; push ecx; mov eax, [ebp+8]; mov esp, ebp; pop ebp; ret 4
	copy(text[offPushECXLocal:], []byte{
		0x55, 0x8B, 0xEC,
		0x51,             // standalone prologue push ecx (local slot alloc, NOT __thiscall!)
		0x8B, 0x45, 0x08, // mov eax, [ebp+8]
		0x8B, 0xE5, 0x5D,
		0xC2, 0x04, 0x00, // ret 4 -> __stdcall
	})
	// 10. fnSeteCdecl: cmp [esp+4], 0; sete cl; movzx eax, cl; fld dword ptr [rdataFloatVA]; call dword ptr [iatNamedVA]; ret
	sp := offSeteCdecl
	copy(text[sp:], []byte{
		0x83, 0x7C, 0x24, 0x04, 0x00, // cmp dword ptr [esp+4], 0
		0x0F, 0x94, 0xC1, // sete cl
		0x0F, 0xB6, 0xC1, // movzx eax, cl
	})
	sp += 11
	text[sp] = 0xD9
	text[sp+1] = 0x05
	binary.LittleEndian.PutUint32(text[sp+2:sp+6], rdataFloatVA)
	addReloc(uint32(textRVA + sp + 2))
	sp += 6
	text[sp] = 0xFF
	text[sp+1] = 0x15
	binary.LittleEndian.PutUint32(text[sp+2:sp+6], lay.iatNamedVA)
	addReloc(uint32(textRVA + sp + 2))
	sp += 6
	text[sp] = 0xC3

	// Switch case blocks starting with 33 C0, 56, and E9 (must NOT be seeded by jump table slots 1..3):
	copy(text[offSwCase1:], []byte{0x33, 0xC0, 0xC3})
	copy(text[offSwCase2:], []byte{0x56, 0x5E, 0xC3})
	text[offSwCase3] = 0xE9
	relSw3 := int32(offVirt2 - (offSwCase3 + 5))
	binary.LittleEndian.PutUint32(text[offSwCase3+1:offSwCase3+5], uint32(relSw3))

	// 11. fnLargeFunc: >260 instructions (push ebp; mov ebp, esp; 265x nop; push strLargeTailVA; pop ebp; ret 4)
	lp := offLargeFunc
	copy(text[lp:], []byte{0x55, 0x8B, 0xEC})
	lp += 3
	for k := 0; k < 265; k++ {
		text[lp] = 0x90 // nop (265 instructions -> instruction 267 before push strLargeTailVA)
		lp++
	}
	text[lp] = 0x68
	binary.LittleEndian.PutUint32(text[lp+1:lp+5], strLargeTailVA)
	addReloc(uint32(textRVA + lp + 1))
	lp += 5
	copy(text[lp:], []byte{0x5D, 0xC2, 0x04, 0x00})
	lp += 4
	lay.fnLargeFuncSize = uint32(lp - offLargeFunc)

	// Build Import Directory at rdata+0x000:
	// Descriptor 0 (20B) at +0x000, Descriptor 1 (null, 20B) at +0x014
	// OriginalFirstThunk at +0x040 (3 u32: hint/name RVA, ordinal flag, 0)
	// FirstThunk (IAT) at +0x100
	// DLL name "KERNEL32.dll\0" at +0x060
	// IMAGE_IMPORT_BY_NAME {uint16(42), "GetTickCount\0"} at +0x080
	binary.LittleEndian.PutUint32(rdata[0x000:0x004], rdataRVA+0x040) // OriginalFirstThunk
	binary.LittleEndian.PutUint32(rdata[0x00C:0x010], rdataRVA+0x060) // Name RVA
	binary.LittleEndian.PutUint32(rdata[0x010:0x014], rdataRVA+0x100) // FirstThunk RVA
	copy(rdata[0x060:], "KERNEL32.dll\x00")
	binary.LittleEndian.PutUint16(rdata[0x080:0x082], 42)
	copy(rdata[0x082:], "GetTickCount\x00")
	binary.LittleEndian.PutUint32(rdata[0x040:0x044], rdataRVA+0x080)
	binary.LittleEndian.PutUint32(rdata[0x044:0x048], 0x80000000|77)
	binary.LittleEndian.PutUint32(rdata[0x100:0x104], rdataRVA+0x080)
	binary.LittleEndian.PutUint32(rdata[0x104:0x108], 0x80000000|77)

	// Build Export Directory at rdata+0x200 (size 0x100):
	expOff := 0x200
	binary.LittleEndian.PutUint32(rdata[expOff+12:expOff+16], rdataRVA+uint32(expOff)+0x40) // Name RVA
	binary.LittleEndian.PutUint32(rdata[expOff+16:expOff+20], 1)                            // Base = 1
	binary.LittleEndian.PutUint32(rdata[expOff+20:expOff+24], 2)                            // NumberOfFunctions = 2
	binary.LittleEndian.PutUint32(rdata[expOff+24:expOff+28], 2)                            // NumberOfNames = 2
	binary.LittleEndian.PutUint32(rdata[expOff+28:expOff+32], rdataRVA+uint32(expOff)+0x50) // AddressOfFunctions
	binary.LittleEndian.PutUint32(rdata[expOff+32:expOff+36], rdataRVA+uint32(expOff)+0x58) // AddressOfNames
	binary.LittleEndian.PutUint32(rdata[expOff+36:expOff+40], rdataRVA+uint32(expOff)+0x60) // AddressOfNameOrdinals
	copy(rdata[expOff+0x40:], "empire.retail.dll\x00")
	// Function 0: real code RVA; Function 1: forwarded export pointing inside [expRVA, expRVA+0x100)
	binary.LittleEndian.PutUint32(rdata[expOff+0x50:expOff+0x54], textRVA+offExportCtor)
	binary.LittleEndian.PutUint32(rdata[expOff+0x54:expOff+0x58], rdataRVA+uint32(expOff)+0x90)
	// Names:
	binary.LittleEndian.PutUint32(rdata[expOff+0x58:expOff+0x5C], rdataRVA+uint32(expOff)+0x70)
	binary.LittleEndian.PutUint32(rdata[expOff+0x5C:expOff+0x60], rdataRVA+uint32(expOff)+0x80)
	binary.LittleEndian.PutUint16(rdata[expOff+0x60:expOff+0x62], 0)
	binary.LittleEndian.PutUint16(rdata[expOff+0x62:expOff+0x64], 1)
	copy(rdata[expOff+0x70:], "CreateFaction\x00")
	copy(rdata[expOff+0x80:], "ForwardedSym\x00")
	copy(rdata[expOff+0x90:], "KERNEL32.GetTickCount\x00")

	// Build .reloc section from relocRVAs:
	relocSec, relocDataSize := buildPERelocSection(relocRVAs, secFileSz)

	// Assemble full PE32 file:
	var fileBuf bytes.Buffer
	hdr := make([]byte, headerSize)
	// DOS Header
	hdr[0] = 'M'
	hdr[1] = 'Z'
	binary.LittleEndian.PutUint32(hdr[0x3C:0x40], 0x80)
	// PE Signature + COFF FileHeader at 0x80
	copy(hdr[0x80:], []byte{'P', 'E', 0, 0})
	binary.LittleEndian.PutUint16(hdr[0x84:0x86], 0x014C) // IMAGE_FILE_MACHINE_I386
	binary.LittleEndian.PutUint16(hdr[0x86:0x88], 4)      // NumberOfSections = 4
	binary.LittleEndian.PutUint16(hdr[0x94:0x96], 224)    // SizeOfOptionalHeader = 224 (PE32)
	binary.LittleEndian.PutUint16(hdr[0x96:0x98], 0x2102) // EXECUTABLE_IMAGE | 32BIT_MACHINE | DLL

	// OptionalHeader32 at 0x98 (224 bytes)
	opt := hdr[0x98 : 0x98+224]
	binary.LittleEndian.PutUint16(opt[0:2], 0x10B)        // PE32 magic
	binary.LittleEndian.PutUint32(opt[28:32], imageBase)  // ImageBase
	binary.LittleEndian.PutUint32(opt[32:36], 0x1000)     // SectionAlignment
	binary.LittleEndian.PutUint32(opt[36:40], 0x1000)     // FileAlignment
	binary.LittleEndian.PutUint32(opt[56:60], 0x5000)     // SizeOfImage
	binary.LittleEndian.PutUint32(opt[60:64], headerSize) // SizeOfHeaders
	binary.LittleEndian.PutUint32(opt[92:96], 16)         // NumberOfRvaAndSizes = 16
	// DataDirectory[0] (Export) at opt[96:104]
	binary.LittleEndian.PutUint32(opt[96:100], rdataRVA+uint32(expOff))
	binary.LittleEndian.PutUint32(opt[100:104], 0x100)
	// DataDirectory[1] (Import) at opt[104:112]
	binary.LittleEndian.PutUint32(opt[104:108], rdataRVA)
	binary.LittleEndian.PutUint32(opt[108:112], 40)
	// DataDirectory[5] (BaseReloc) at opt[136:144]
	binary.LittleEndian.PutUint32(opt[136:140], relocRVA)
	binary.LittleEndian.PutUint32(opt[140:144], relocDataSize)

	// 4 Section Headers (40 bytes each) starting at 0x98 + 224 = 0x178
	writeSecHdr := func(idx int, name string, vsize, rva, rsize, rawOff, chars uint32) {
		sh := hdr[0x178+idx*40 : 0x178+(idx+1)*40]
		copy(sh[0:8], name)
		binary.LittleEndian.PutUint32(sh[8:12], vsize)
		binary.LittleEndian.PutUint32(sh[12:16], rva)
		binary.LittleEndian.PutUint32(sh[16:20], rsize)
		binary.LittleEndian.PutUint32(sh[20:24], rawOff)
		binary.LittleEndian.PutUint32(sh[36:40], chars)
	}
	writeSecHdr(0, ".text", secFileSz, textRVA, secFileSz, headerSize, 0x60000020)
	writeSecHdr(1, ".rdata", secFileSz, rdataRVA, secFileSz, headerSize+secFileSz, 0x40000040)
	writeSecHdr(2, ".data", secFileSz, dataRVA, secFileSz, headerSize+2*secFileSz, 0xC0000040)
	writeSecHdr(3, ".reloc", secFileSz, relocRVA, secFileSz, headerSize+3*secFileSz, 0x42000040)

	fileBuf.Write(hdr)
	fileBuf.Write(text)
	fileBuf.Write(rdata)
	fileBuf.Write(data)
	fileBuf.Write(relocSec)

	if err := os.WriteFile(outPath, fileBuf.Bytes(), 0o644); err != nil {
		t.Fatalf("write synthetic PE32: %v", err)
	}
	return lay
}

func buildPERelocSection(rvas []uint32, secSize int) ([]byte, uint32) {
	sort.Slice(rvas, func(i, j int) bool { return rvas[i] < rvas[j] })
	byPage := make(map[uint32][]uint16)
	var pages []uint32
	for _, rva := range rvas {
		page := rva &^ 0xFFF
		off := uint16(rva & 0xFFF)
		if _, exists := byPage[page]; !exists {
			pages = append(pages, page)
		}
		byPage[page] = append(byPage[page], (3<<12)|off)
	}
	var buf bytes.Buffer
	for _, page := range pages {
		entries := byPage[page]
		if len(entries)%2 != 0 {
			entries = append(entries, 0) // IMAGE_REL_BASED_ABSOLUTE padding
		}
		blockSize := uint32(8 + len(entries)*2)
		var hdr [8]byte
		binary.LittleEndian.PutUint32(hdr[0:4], page)
		binary.LittleEndian.PutUint32(hdr[4:8], blockSize)
		buf.Write(hdr[:])
		for _, e := range entries {
			var eb [2]byte
			binary.LittleEndian.PutUint16(eb[:], e)
			buf.Write(eb[:])
		}
	}
	totalLen := uint32(buf.Len())
	out := make([]byte, secSize)
	copy(out, buf.Bytes())
	return out, totalLen
}
