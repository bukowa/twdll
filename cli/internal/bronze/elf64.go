package bronze

import (
	"bytes"
	"context"
	"database/sql"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/arch/x86/x86asm"
	"golang.org/x/sync/errgroup"

	"twdll/cli/internal/extractdb"
)

type elfSectionBuf struct {
	idx   int
	name  string
	va    uint64
	vsize uint64
	raw   uint64
	rsize uint64
	flags uint32
	data  []byte
}

type elfImageInfo struct {
	imageBase uint64
	sections  []elfSectionBuf
	textSec   *elfSectionBuf
	pltSec    *elfSectionBuf
	pltSec2   *elfSectionBuf
	dataSecs  []*elfSectionBuf
}

func (img *elfImageInfo) inText(va uint64) bool {
	if img.textSec == nil {
		return false
	}
	return va >= img.textSec.va && va < img.textSec.va+uint64(len(img.textSec.data))
}

func (img *elfImageInfo) inTextOrPLT(va uint64) bool {
	if img.inText(va) {
		return true
	}
	if img.pltSec != nil && va >= img.pltSec.va && va < img.pltSec.va+img.pltSec.vsize {
		return true
	}
	if img.pltSec2 != nil && va >= img.pltSec2.va && va < img.pltSec2.va+img.pltSec2.vsize {
		return true
	}
	return false
}

func (img *elfImageInfo) inDataSection(va uint64) bool {
	for _, ds := range img.dataSecs {
		if va >= ds.va && va < ds.va+ds.vsize {
			return true
		}
	}
	return false
}

func (img *elfImageInfo) readSliceAtVA(va, size uint64) ([]byte, bool) {
	for _, ds := range img.dataSecs {
		if va >= ds.va && va+size <= ds.va+uint64(len(ds.data)) {
			off := va - ds.va
			return ds.data[off : off+size], true
		}
	}
	return nil, false
}

func (img *elfImageInfo) readIdentStringAtVA(va uint64) (string, bool) {
	for _, ds := range img.dataSecs {
		if va >= ds.va && va < ds.va+uint64(len(ds.data)) {
			off := int(va - ds.va)
			rem := ds.data[off:]
			if len(rem) > 84 {
				rem = rem[:84]
			}
			end := bytes.IndexByte(rem, 0)
			if end < 0 {
				return "", false
			}
			s := string(rem[:end])
			if !isValidLuaIdentifier(s) {
				return "", false
			}
			return s, true
		}
	}
	return "", false
}

func extractELF64(ctx context.Context, db *sql.DB, binaryPath string) (*targetExtractResult, error) {
	ef, err := elf.Open(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("open elf64 %s: %w", binaryPath, err)
	}
	defer ef.Close()

	if ef.Class != elf.ELFCLASS64 || ef.Machine != elf.EM_X86_64 {
		return nil, fmt.Errorf("%s is not an x86_64 ELF64 binary", binaryPath)
	}

	imageBase := uint64(0x400000)
	foundLoad := false
	for _, p := range ef.Progs {
		if p.Type == elf.PT_LOAD && p.Flags&elf.PF_X != 0 {
			imageBase = p.Vaddr
			foundLoad = true
			break
		}
	}
	if !foundLoad {
		for _, p := range ef.Progs {
			if p.Type == elf.PT_LOAD {
				imageBase = p.Vaddr
				break
			}
		}
	}

	img := &elfImageInfo{imageBase: imageBase}
	var secRows []sectionRow
	for idx, sec := range ef.Sections {
		if sec.Type == elf.SHT_NULL {
			continue
		}
		var flags uint32
		if sec.Flags&elf.SHF_ALLOC != 0 {
			flags |= extractdb.SecFlagAlloc | extractdb.SecFlagRead
		}
		if sec.Flags&elf.SHF_WRITE != 0 {
			flags |= extractdb.SecFlagWrite
		}
		if sec.Flags&elf.SHF_EXECINSTR != 0 {
			flags |= extractdb.SecFlagExec
		}
		rawSz := sec.FileSize
		var data []byte
		if sec.Name == ".text" || sec.Name == ".plt" || sec.Name == ".plt.sec" || sec.Name == ".rodata" || sec.Name == ".data.rel.ro" || sec.Name == ".data" {
			data, _ = sec.Data()
		}
		sbuf := elfSectionBuf{
			idx:   idx,
			name:  sec.Name,
			va:    sec.Addr,
			vsize: sec.Size,
			raw:   sec.Offset,
			rsize: rawSz,
			flags: flags,
			data:  data,
		}
		img.sections = append(img.sections, sbuf)
		secRows = append(secRows, sectionRow{
			SecIdx:      idx,
			Name:        sec.Name,
			VA:          sec.Addr,
			VirtualSize: sec.Size,
			RawOffset:   sec.Offset,
			RawSize:     rawSz,
			Flags:       flags,
		})
	}
	for i := range img.sections {
		s := &img.sections[i]
		switch s.name {
		case ".text":
			if img.textSec == nil {
				img.textSec = s
			}
		case ".plt":
			img.pltSec = s
		case ".plt.sec":
			img.pltSec2 = s
		case ".rodata", ".data.rel.ro", ".data", ".bss":
			img.dataSecs = append(img.dataSecs, s)
		}
	}

	if err := insertSections(ctx, db, secRows); err != nil {
		return nil, err
	}

	imports, exports, iatSymByVA, err := parseAndInsertELFRelocsImportsExports(ctx, db, ef)
	if err != nil {
		return nil, err
	}
	if err := insertImports(ctx, db, imports); err != nil {
		return nil, err
	}
	if err := insertExports(ctx, db, exports); err != nil {
		return nil, err
	}

	// Read .symtab for _ZTV* vtables and STT_FUNC fallback symbols.
	symtabSyms, _ := ef.Symbols()
	dynSyms, _ := ef.DynamicSymbols()
	vtables := scanELFVtables(img, symtabSyms, dynSyms)
	if err := insertVtables(ctx, db, vtables); err != nil {
		return nil, err
	}

	luaRegs := scanELFLuaRegs(img)
	if err := insertLuaRegs(ctx, db, luaRegs); err != nil {
		return nil, err
	}

	// Scan UTF-32LE and ASCII strings in .rodata, .data.rel.ro, .data.
	var strSlices []dataSectionSlice
	for _, ds := range img.dataSecs {
		if len(ds.data) > 0 {
			strSlices = append(strSlices, dataSectionSlice{
				name: ds.name,
				va:   ds.va,
				data: ds.data,
			})
		}
	}
	strIdx, err := scanAndPersistStrings(ctx, db, strSlices, true)
	if err != nil {
		return nil, err
	}

	// 2-Pass Streaming DWARF extraction.
	var (
		funcRanges []funcRangeRow
		seeds      []FunctionSeed
	)
	textStart, textEnd := uint64(0), uint64(0)
	if img.textSec != nil {
		textStart = img.textSec.va
		textEnd = textStart + uint64(len(img.textSec.data))
	}

	dw, dwErr := ef.DWARF()
	if dwErr == nil && dw != nil {
		p1, err := runDWARFPass1(ctx, db, dw)
		if err != nil {
			return nil, err
		}
		funcRanges, seeds, err = runDWARFPass2(ctx, db, dw, p1, symtabSyms, vtables, luaRegs, textStart, textEnd)
		if err != nil {
			return nil, err
		}
		p1 = nil
		dw = nil
	}

	// Close ELF handle and release DWARF allocations before parallel x86asm64 pass and FTS rebuild.
	symtabSyms = nil
	dynSyms = nil
	_ = ef.Close()
	runtime.GC()

	if err := runELF64X86AsmPass(ctx, db, img, funcRanges, seeds, iatSymByVA, strIdx); err != nil {
		return nil, err
	}

	return &targetExtractResult{
		imageBase: imageBase,
		arch:      extractdb.ArchX86_64,
		format:    extractdb.FormatELF64,
		sources:   []string{"elf64", "dwarf"},
		counts:    make(map[string]int64),
		seeds:     seeds,
	}, nil
}

func parseAndInsertELFRelocsImportsExports(
	ctx context.Context,
	db *sql.DB,
	ef *elf.File,
) ([]importRow, []exportRow, map[uint64]string, error) {
	dynSyms, _ := ef.DynamicSymbols()

	relocBatch := newTxStmtBatcher(ctx, db, `INSERT OR IGNORE INTO relocs(va) VALUES (?)`)
	defer func() { _ = relocBatch.Close() }()

	var imports []importRow
	iatSymByVA := make(map[uint64]string)
	seenIAT := make(map[uint64]bool)

	for _, sec := range ef.Sections {
		if sec.Type != elf.SHT_RELA || (sec.Name != ".rela.dyn" && sec.Name != ".rela.plt") {
			continue
		}
		data, err := sec.Data()
		if err != nil {
			continue
		}
		for pos := 0; pos+24 <= len(data); pos += 24 {
			rOffset := binary.LittleEndian.Uint64(data[pos : pos+8])
			rInfo := binary.LittleEndian.Uint64(data[pos+8 : pos+16])
			if rOffset == 0 {
				continue
			}
			if err := relocBatch.Exec(int64(rOffset)); err != nil {
				return nil, nil, nil, fmt.Errorf("insert elf reloc 0x%x: %w", rOffset, err)
			}

			symIdx := uint32(rInfo >> 32)
			relType := elf.R_X86_64(rInfo & 0xffffffff)
			if symIdx > 0 && int(symIdx-1) < len(dynSyms) {
				sym := dynSyms[symIdx-1]
				if sym.Name != "" && (sym.Section == elf.SHN_UNDEF || relType == elf.R_X86_64_JMP_SLOT || relType == elf.R_X86_64_GLOB_DAT) {
					if sym.Section == elf.SHN_UNDEF && !seenIAT[rOffset] {
						seenIAT[rOffset] = true
						imports = append(imports, importRow{
							IATVA:   rOffset,
							DLLName: sym.Library,
							SymName: sym.Name,
							Ordinal: int(symIdx),
						})
						iatSymByVA[rOffset] = sym.Name
					}
				}
			}
		}
	}
	if err := relocBatch.Flush(); err != nil {
		return nil, nil, nil, err
	}

	sort.Slice(imports, func(i, j int) bool { return imports[i].IATVA < imports[j].IATVA })

	var exports []exportRow
	var nextExpID int64 = 1
	for idx, sym := range dynSyms {
		if sym.Section == elf.SHN_UNDEF || sym.Value == 0 || sym.Name == "" {
			continue
		}
		st := elf.ST_TYPE(sym.Info)
		if st != elf.STT_FUNC && st != elf.STT_OBJECT {
			continue
		}
		exports = append(exports, exportRow{
			ExportID: nextExpID,
			EA:       sym.Value,
			Name:     sym.Name,
			Ordinal:  idx + 1,
		})
		nextExpID++
	}
	return imports, exports, iatSymByVA, nil
}

func scanELFVtables(img *elfImageInfo, symtabSyms, dynSyms []elf.Symbol) []vtableRow {
	var allSyms []elf.Symbol
	allSyms = append(allSyms, symtabSyms...)
	if len(allSyms) == 0 {
		allSyms = append(allSyms, dynSyms...)
	}

	ztiAddrs := make(map[uint64]bool)
	var cxaPureVirtEA uint64
	for _, s := range allSyms {
		if s.Value == 0 {
			continue
		}
		if strings.HasPrefix(s.Name, "_ZTI") {
			ztiAddrs[s.Value] = true
		} else if strings.HasPrefix(s.Name, "__cxa_pure_virtual") {
			cxaPureVirtEA = s.Value
		}
	}

	isValidSlotTarget := func(v uint64) bool {
		if v == 0 {
			return false
		}
		if img.inTextOrPLT(v) {
			return true
		}
		return cxaPureVirtEA != 0 && v == cxaPureVirtEA
	}

	isSubVtableHeader := func(vals []uint64, idx int) bool {
		if idx+2 >= len(vals) {
			return false
		}
		if isValidSlotTarget(vals[idx]) || isValidSlotTarget(vals[idx+1]) {
			return false
		}
		offToTop := int64(vals[idx])
		if offToTop > 0 || offToTop < -0x1000000 {
			return false
		}
		rtti := vals[idx+1]
		if rtti != 0 && !ztiAddrs[rtti] && !img.inDataSection(rtti) {
			return false
		}
		return isValidSlotTarget(vals[idx+2])
	}

	var rows []vtableRow
	seenVtableSym := make(map[uint64]bool)
	for _, sym := range allSyms {
		if !strings.HasPrefix(sym.Name, "_ZTV") || sym.Value == 0 || sym.Size < 24 {
			continue
		}
		if seenVtableSym[sym.Value] {
			continue
		}
		seenVtableSym[sym.Value] = true

		rawBytes, ok := img.readSliceAtVA(sym.Value, sym.Size)
		if !ok {
			continue
		}
		nQwords := len(rawBytes) / 8
		if nQwords < 3 {
			continue
		}
		vals := make([]uint64, nQwords)
		for i := 0; i < nQwords; i++ {
			vals[i] = binary.LittleEndian.Uint64(rawBytes[i*8 : (i+1)*8])
		}

		ownerName := decodeItaniumZTVOwner(sym.Name)
		idx := 0
		for idx+2 < nQwords {
			if idx > 0 && !isSubVtableHeader(vals, idx) {
				idx++
				continue
			}
			vtableEA := sym.Value + uint64(idx+2)*8
			slot := 0
			sIdx := idx + 2
			for sIdx < nQwords {
				if sIdx > idx+2 && isSubVtableHeader(vals, sIdx) {
					break
				}
				target := vals[sIdx]
				if isValidSlotTarget(target) {
					rows = append(rows, vtableRow{
						VtableEA:  vtableEA,
						Slot:      slot,
						TargetEA:  target,
						OwnerName: ownerName,
					})
				}
				slot++
				sIdx++
			}
			idx = sIdx
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].VtableEA != rows[j].VtableEA {
			return rows[i].VtableEA < rows[j].VtableEA
		}
		return rows[i].Slot < rows[j].Slot
	})
	return rows
}

func decodeItaniumZTVOwner(symName string) string {
	s := strings.TrimPrefix(symName, "_ZTV")
	if idx := strings.IndexByte(s, '@'); idx >= 0 {
		s = s[:idx]
	}
	if strings.HasPrefix(s, "N") && strings.HasSuffix(s, "E") {
		s = s[1 : len(s)-1]
		var parts []string
		for len(s) > 0 {
			nLen := 0
			for nLen < len(s) && s[nLen] >= '0' && s[nLen] <= '9' {
				nLen++
			}
			if nLen == 0 {
				return symName
			}
			count, err := strconv.Atoi(s[:nLen])
			if err != nil || count <= 0 || nLen+count > len(s) {
				return symName
			}
			parts = append(parts, s[nLen:nLen+count])
			s = s[nLen+count:]
		}
		if len(parts) > 0 {
			return strings.Join(parts, "::")
		}
	} else if len(s) > 0 && s[0] >= '1' && s[0] <= '9' {
		nLen := 0
		for nLen < len(s) && s[nLen] >= '0' && s[nLen] <= '9' {
			nLen++
		}
		count, err := strconv.Atoi(s[:nLen])
		if err == nil && count > 0 && nLen+count == len(s) {
			return s[nLen:]
		}
	}
	return symName
}

func scanELFLuaRegs(img *elfImageInfo) []luaRegRow {
	var rows []luaRegRow
	for _, ds := range img.dataSecs {
		d := ds.data
		if len(d) < 48 {
			continue
		}
		claimed := make([]bool, len(d))

		// Pass 1: 24-byte Itanium Lunar<T>::RegType {const char* name_va, uint64 func_va, int64 adj == 0}
		for off := 0; off+72 <= len(d); {
			run := 0
			for p := off; p+24 <= len(d); p += 24 {
				sVA := binary.LittleEndian.Uint64(d[p : p+8])
				fVA := binary.LittleEndian.Uint64(d[p+8 : p+16])
				adj := binary.LittleEndian.Uint64(d[p+16 : p+24])
				if sVA == 0 && fVA == 0 && adj == 0 {
					break
				}
				if adj != 0 || (fVA&1) != 0 || !img.inText(fVA) {
					run = 0
					break
				}
				if _, ok := img.readIdentStringAtVA(sVA); !ok {
					run = 0
					break
				}
				run++
			}
			termOff := off + run*24
			if run >= 3 && termOff+24 <= len(d) &&
				binary.LittleEndian.Uint64(d[termOff:termOff+8]) == 0 &&
				binary.LittleEndian.Uint64(d[termOff+8:termOff+16]) == 0 &&
				binary.LittleEndian.Uint64(d[termOff+16:termOff+24]) == 0 {
				tableEA := ds.va + uint64(off)
				for k := 0; k < run; k++ {
					p := off + k*24
					sVA := binary.LittleEndian.Uint64(d[p : p+8])
					fVA := binary.LittleEndian.Uint64(d[p+8 : p+16])
					nm, _ := img.readIdentStringAtVA(sVA)
					rows = append(rows, luaRegRow{
						TableEA:    tableEA,
						MethodName: nm,
						FuncEA:     fVA,
					})
				}
				for b := off; b < termOff+24 && b < len(claimed); b++ {
					claimed[b] = true
				}
				off = termOff + 24
			} else {
				off += 8
			}
		}

		// Pass 2: 16-byte luaL_Reg {const char* name_va, uint64 func_va}
		for off := 0; off+48 <= len(d); {
			if claimed[off] {
				off += 8
				continue
			}
			run := 0
			for p := off; p+16 <= len(d); p += 16 {
				if claimed[p] {
					run = 0
					break
				}
				sVA := binary.LittleEndian.Uint64(d[p : p+8])
				fVA := binary.LittleEndian.Uint64(d[p+8 : p+16])
				if sVA == 0 && fVA == 0 {
					break
				}
				if !img.inText(fVA) {
					run = 0
					break
				}
				if _, ok := img.readIdentStringAtVA(sVA); !ok {
					run = 0
					break
				}
				run++
			}
			termOff := off + run*16
			if run >= 3 && termOff+16 <= len(d) &&
				binary.LittleEndian.Uint64(d[termOff:termOff+8]) == 0 &&
				binary.LittleEndian.Uint64(d[termOff+8:termOff+16]) == 0 {
				tableEA := ds.va + uint64(off)
				for k := 0; k < run; k++ {
					p := off + k*16
					sVA := binary.LittleEndian.Uint64(d[p : p+8])
					fVA := binary.LittleEndian.Uint64(d[p+8 : p+16])
					nm, _ := img.readIdentStringAtVA(sVA)
					rows = append(rows, luaRegRow{
						TableEA:    tableEA,
						MethodName: nm,
						FuncEA:     fVA,
					})
				}
				off = termOff + 16
			} else {
				off += 8
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TableEA != rows[j].TableEA {
			return rows[i].TableEA < rows[j].TableEA
		}
		if rows[i].MethodName != rows[j].MethodName {
			return rows[i].MethodName < rows[j].MethodName
		}
		return rows[i].FuncEA < rows[j].FuncEA
	})
	return rows
}

type elfThunkUpdate struct {
	funcEA     uint64
	targetEA   uint64
	isExternal bool
}

type elfWorkerChunkResult struct {
	callSites   []callSiteRow
	dataXrefs   []dataXrefRow
	stringXrefs []stringXrefRow
	thunks      []elfThunkUpdate
}

func scanELFPLTStubs(img *elfImageInfo, iatSymByVA map[uint64]string) map[uint64]uint64 {
	pltGotByVA := make(map[uint64]uint64)
	for _, sec := range []*elfSectionBuf{img.pltSec, img.pltSec2} {
		if sec == nil || len(sec.data) < 16 {
			continue
		}
		for off := 0; off+16 <= len(sec.data); off += 16 {
			stubEA := sec.va + uint64(off)
			if isThunk, targetEA := detectELF64Thunk(sec.data[off:off+16], stubEA, img); isThunk {
				if _, ok := iatSymByVA[targetEA]; ok {
					pltGotByVA[stubEA] = targetEA
				}
			}
		}
	}
	return pltGotByVA
}

func runELF64X86AsmPass(
	ctx context.Context,
	db *sql.DB,
	img *elfImageInfo,
	funcRanges []funcRangeRow,
	seeds []FunctionSeed,
	iatSymByVA map[uint64]string,
	strIdx *stringIndex,
) error {
	if img.textSec == nil || len(img.textSec.data) == 0 || len(funcRanges) == 0 {
		return nil
	}
	tb := img.textSec.data
	textVA := img.textSec.va
	textEnd := textVA + uint64(len(tb))
	pltGotByVA := scanELFPLTStubs(img, iatSymByVA)

	numWorkers := runtime.GOMAXPROCS(0)
	if numWorkers < 1 {
		numWorkers = 1
	}
	if numWorkers > len(funcRanges) {
		numWorkers = len(funcRanges)
	}

	chunkSize := (len(funcRanges) + numWorkers - 1) / numWorkers
	results := make([]elfWorkerChunkResult, numWorkers)

	g, _ := errgroup.WithContext(ctx)
	for w := 0; w < numWorkers; w++ {
		wIdx := w
		startIdx := wIdx * chunkSize
		if startIdx >= len(funcRanges) {
			break
		}
		endIdx := startIdx + chunkSize
		if endIdx > len(funcRanges) {
			endIdx = len(funcRanges)
		}
		slice := funcRanges[startIdx:endIdx]

		g.Go(func() error {
			var out elfWorkerChunkResult
			for _, rg := range slice {
				if rg.StartEA < textVA || rg.StartEA >= textEnd {
					continue
				}
				endEA := rg.EndEA
				if endEA > textEnd {
					endEA = textEnd
				}
				if endEA <= rg.StartEA {
					continue
				}
				startOff := int(rg.StartEA - textVA)
				endOff := int(endEA - textVA)

				if rg.StartEA == rg.FuncEA && (endOff-startOff) <= 16 {
					if isThunk, targetEA := detectELF64Thunk(tb[startOff:endOff], rg.StartEA, img); isThunk {
						_, isExt := iatSymByVA[targetEA]
						if !isExt {
							_, isExt = pltGotByVA[targetEA]
						}
						out.thunks = append(out.thunks, elfThunkUpdate{
							funcEA:     rg.FuncEA,
							targetEA:   targetEA,
							isExternal: isExt,
						})
					}
				}

				decodeELF64Range(tb, startOff, endOff, rg.StartEA, rg.EndEA, rg.FuncEA, img, strIdx, &out)
			}
			results[wIdx] = out
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// Flush results deterministically across chunks using a single active transaction at a time.
	const (
		stmtCall = iota
		stmtData
	)
	xrefBatch := newMultiStmtTxBatcher(ctx, db,
		`INSERT OR IGNORE INTO call_sites(call_ea, caller_ea, callee_ea, call_kind, base_reg, mem_disp)
		VALUES (?, ?, ?, ?, ?, ?)`,
		`INSERT OR IGNORE INTO data_xrefs(from_ea, func_ea, to_ea, access_kind)
		VALUES (?, ?, ?, ?)`,
	)
	defer func() { _ = xrefBatch.Close() }()

	var allStringXrefs []stringXrefRow
	thunkByEA := make(map[uint64]elfThunkUpdate)
	for i := range seeds {
		if gotVA, ok := pltGotByVA[seeds[i].EA]; ok {
			thunkByEA[seeds[i].EA] = elfThunkUpdate{
				funcEA:     seeds[i].EA,
				targetEA:   gotVA,
				isExternal: true,
			}
		}
	}

	for i := range results {
		r := &results[i]
		for _, cs := range r.callSites {
			if err := xrefBatch.Exec(stmtCall, int64(cs.CallEA), int64(cs.CallerEA), cs.CalleeEA, cs.CallKind, cs.BaseReg, cs.MemDisp); err != nil {
				return fmt.Errorf("insert elf64 call_site 0x%x: %w", cs.CallEA, err)
			}
		}
		for _, dx := range r.dataXrefs {
			if err := xrefBatch.Exec(stmtData, int64(dx.FromEA), int64(dx.FuncEA), int64(dx.ToEA), dx.AccessKind); err != nil {
				return fmt.Errorf("insert elf64 data_xref 0x%x: %w", dx.FromEA, err)
			}
		}
		allStringXrefs = append(allStringXrefs, r.stringXrefs...)
		for _, th := range r.thunks {
			thunkByEA[th.funcEA] = th
		}
		results[i] = elfWorkerChunkResult{}
	}

	if err := xrefBatch.Flush(); err != nil {
		return err
	}
	if err := applyStringXrefsAndDFCount(ctx, db, allStringXrefs); err != nil {
		return err
	}

	if len(thunkByEA) > 0 {
		upd := newTxStmtBatcher(ctx, db, `
			UPDATE functions SET flags = flags | ?, thunk_target_ea = ? WHERE ea = ?
		`)
		defer func() { _ = upd.Close() }()
		for i := range seeds {
			if th, ok := thunkByEA[seeds[i].EA]; ok {
				addFlags := extractdb.FuncFlagThunk
				if th.isExternal {
					addFlags |= extractdb.FuncFlagExternal
				}
				seeds[i].Flags |= addFlags
				seeds[i].ThunkTargetEA = th.targetEA
				if err := upd.Exec(int64(addFlags), int64(th.targetEA), int64(th.funcEA)); err != nil {
					return fmt.Errorf("update elf64 thunk 0x%x: %w", th.funcEA, err)
				}
			}
		}
		if err := upd.Flush(); err != nil {
			return err
		}
	}

	return nil
}

func detectELF64Thunk(buf []byte, startEA uint64, img *elfImageInfo) (bool, uint64) {
	off := 0
	// Skip optional endbr64 (F3 0F 1E FA).
	if len(buf) >= 4 && buf[0] == 0xF3 && buf[1] == 0x0F && buf[2] == 0x1E && buf[3] == 0xFA {
		off = 4
		startEA += 4
	}
	rem := buf[off:]
	// FF 25 <disp32> (jmp qword ptr [rip + disp32])
	if len(rem) >= 6 && rem[0] == 0xFF && rem[1] == 0x25 {
		disp := int32(binary.LittleEndian.Uint32(rem[2:6]))
		target := uint64(int64(startEA) + 6 + int64(disp))
		return true, target
	}
	// E9 <rel32> (jmp target_ea)
	if len(rem) >= 5 && rem[0] == 0xE9 {
		rel := int32(binary.LittleEndian.Uint32(rem[1:5]))
		target := uint64(int64(startEA) + 5 + int64(rel))
		if img.inTextOrPLT(target) {
			return true, target
		}
	}
	return false, 0
}

func decodeELF64Range(
	tb []byte,
	startOff int,
	endOff int,
	rangeStartEA uint64,
	rangeEndEA uint64,
	funcEA uint64,
	img *elfImageInfo,
	strIdx *stringIndex,
	out *elfWorkerChunkResult,
) {
	textVA := img.textSec.va
	off := startOff
	for off < endOff {
		inst, err := x86asm.Decode(tb[off:endOff], 64)
		if err != nil || inst.Len <= 0 {
			off++
			continue
		}
		instEA := textVA + uint64(off)
		nextEA := instEA + uint64(inst.Len)

		switch inst.Op {
		case x86asm.CALL:
			cs := extractELF64CallSite(inst, instEA, nextEA, funcEA, tb, off)
			out.callSites = append(out.callSites, cs)
		case x86asm.JMP:
			if tb[off] == 0xE9 && inst.Len == 5 {
				rel := int32(binary.LittleEndian.Uint32(tb[off+1 : off+5]))
				target := uint64(int64(nextEA) + int64(rel))
				if (target < rangeStartEA || target >= rangeEndEA) && img.inTextOrPLT(target) {
					out.callSites = append(out.callSites, callSiteRow{
						CallEA:   instEA,
						CallerEA: funcEA,
						CalleeEA: sql.NullInt64{Int64: int64(target), Valid: true},
						CallKind: extractdb.CallKindTailJump,
					})
				}
			}
		}

		// Inspect operands for RIP-relative and non-PIC 32-bit absolute data/string references.
		for argIdx, arg := range inst.Args {
			if arg == nil {
				break
			}
			switch a := arg.(type) {
			case x86asm.Mem:
				var targetEA uint64
				if a.Base == x86asm.RIP {
					targetEA = uint64(int64(nextEA) + a.Disp)
				} else if a.Disp > 0 && ((a.Base == 0 && a.Index == 0) || (img.imageBase >= 0x100000 && uint64(a.Disp) >= img.imageBase)) {
					targetEA = uint64(a.Disp)
				}
				if targetEA != 0 && img.inDataSection(targetEA) {
					if strID, ok := strIdx.LookupEA(targetEA); ok {
						out.stringXrefs = append(out.stringXrefs, stringXrefRow{
							StrID:  strID,
							FuncEA: funcEA,
						})
					}
					kind := extractdb.AccessKindRead
					if inst.Op == x86asm.LEA {
						kind = extractdb.AccessKindAddr
					} else if argIdx == 0 && !isReadOnlyMemArg0Op(inst.Op) {
						kind = extractdb.AccessKindWrite
					}
					out.dataXrefs = append(out.dataXrefs, dataXrefRow{
						FromEA:     instEA,
						FuncEA:     funcEA,
						ToEA:       targetEA,
						AccessKind: kind,
					})
				}
			case x86asm.Imm:
				// Non-PIC `mov reg, imm32` or `push imm32` pointing into .rodata/.data.rel.ro/.data.
				if (inst.Op == x86asm.MOV || inst.Op == x86asm.PUSH) && a > 0 {
					targetEA := uint64(a)
					if img.inDataSection(targetEA) {
						if strID, ok := strIdx.LookupEA(targetEA); ok {
							out.stringXrefs = append(out.stringXrefs, stringXrefRow{
								StrID:  strID,
								FuncEA: funcEA,
							})
						}
						out.dataXrefs = append(out.dataXrefs, dataXrefRow{
							FromEA:     instEA,
							FuncEA:     funcEA,
							ToEA:       targetEA,
							AccessKind: extractdb.AccessKindAddr,
						})
					}
				}
			}
		}

		off += inst.Len
	}
}

func extractELF64CallSite(inst x86asm.Inst, instEA, nextEA, funcEA uint64, tb []byte, off int) callSiteRow {
	if tb[off] == 0xE8 && inst.Len == 5 {
		rel := int32(binary.LittleEndian.Uint32(tb[off+1 : off+5]))
		callee := uint64(int64(nextEA) + int64(rel))
		return callSiteRow{
			CallEA:   instEA,
			CallerEA: funcEA,
			CalleeEA: sql.NullInt64{Int64: int64(callee), Valid: true},
			CallKind: extractdb.CallKindDirect,
		}
	}
	if mem, ok := inst.Args[0].(x86asm.Mem); ok {
		if mem.Base == x86asm.RIP {
			callee := uint64(int64(nextEA) + mem.Disp)
			return callSiteRow{
				CallEA:   instEA,
				CallerEA: funcEA,
				CalleeEA: sql.NullInt64{Int64: int64(callee), Valid: true},
				CallKind: extractdb.CallKindIndirectMem,
				BaseReg:  "RIP",
				MemDisp:  sql.NullInt64{Int64: mem.Disp, Valid: true},
			}
		}
		baseReg := ""
		if mem.Base != 0 {
			baseReg = mem.Base.String()
		}
		return callSiteRow{
			CallEA:   instEA,
			CallerEA: funcEA,
			CallKind: extractdb.CallKindIndirectMem,
			BaseReg:  baseReg,
			MemDisp:  nullInt64(mem.Disp, mem.Disp != 0 || mem.Base != 0),
		}
	}
	if reg, ok := inst.Args[0].(x86asm.Reg); ok {
		return callSiteRow{
			CallEA:   instEA,
			CallerEA: funcEA,
			CallKind: extractdb.CallKindIndirectReg,
			BaseReg:  reg.String(),
		}
	}
	return callSiteRow{
		CallEA:   instEA,
		CallerEA: funcEA,
		CallKind: extractdb.CallKindIndirectReg,
	}
}
