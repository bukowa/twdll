package bronze

import (
	"bytes"
	"context"
	"database/sql"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"twdll/cli/internal/extractdb"
)

type peSectionBuf struct {
	idx   int
	name  string
	rva   uint32
	va    uint32
	vsize uint32
	raw   uint32
	rsize uint32
	flags uint32
	data  []byte
}

type peImageInfo struct {
	imageBase uint32
	maxRVA    uint32
	sections  []peSectionBuf
	textSec   *peSectionBuf
	rdataSec  *peSectionBuf
	dataSec   *peSectionBuf
	// relocWordBitmap has bit rva set when a 4-byte HIGHLOW relocation starts at rva.
	relocWordBitmap []uint64
	// relocByteBitmap has bit rva set when byte rva is inside any 4-byte HIGHLOW relocation.
	relocByteBitmap []uint64
}

func (img *peImageInfo) hasRelocAtVA(va uint32) bool {
	if va < img.imageBase {
		return false
	}
	rva := va - img.imageBase
	return img.hasRelocAtRVA(rva)
}

func (img *peImageInfo) hasRelocAtRVA(rva uint32) bool {
	idx := rva >> 6
	if int(idx) >= len(img.relocWordBitmap) {
		return false
	}
	return (img.relocWordBitmap[idx] & (uint64(1) << (rva & 63))) != 0
}

func (img *peImageInfo) hasRelocByteAtRVA(rva uint32) bool {
	idx := rva >> 6
	if int(idx) >= len(img.relocByteBitmap) {
		return false
	}
	return (img.relocByteBitmap[idx] & (uint64(1) << (rva & 63))) != 0
}

func (img *peImageInfo) setRelocRVA(rva uint32) {
	if rva+4 > img.maxRVA {
		return
	}
	img.relocWordBitmap[rva>>6] |= uint64(1) << (rva & 63)
	for k := uint32(0); k < 4; k++ {
		b := rva + k
		img.relocByteBitmap[b>>6] |= uint64(1) << (b & 63)
	}
}

func (img *peImageInfo) inText(va uint32) bool {
	if img.textSec == nil {
		return false
	}
	return va >= img.textSec.va && va < img.textSec.va+uint32(len(img.textSec.data))
}

func (img *peImageInfo) readSliceAtRVA(rva, size uint32) ([]byte, bool) {
	for i := range img.sections {
		s := &img.sections[i]
		if rva >= s.rva && rva < s.rva+uint32(len(s.data)) {
			off := rva - s.rva
			if off+size > uint32(len(s.data)) {
				return nil, false
			}
			return s.data[off : off+size], true
		}
	}
	return nil, false
}

func (img *peImageInfo) readCStringAtRVA(rva uint32, maxLen int) (string, bool) {
	for i := range img.sections {
		s := &img.sections[i]
		if rva >= s.rva && rva < s.rva+uint32(len(s.data)) {
			off := int(rva - s.rva)
			rem := s.data[off:]
			if len(rem) > maxLen {
				rem = rem[:maxLen]
			}
			end := bytes.IndexByte(rem, 0)
			if end < 0 {
				return "", false
			}
			return string(rem[:end]), true
		}
	}
	return "", false
}

func (img *peImageInfo) readIdentStringAtVA(va uint32) (string, bool) {
	if va < img.imageBase {
		return "", false
	}
	s, ok := img.readCStringAtRVA(va-img.imageBase, 84)
	if !ok || !isValidLuaIdentifier(s) {
		return "", false
	}
	return s, true
}

func isValidLuaIdentifier(s string) bool {
	if len(s) < 2 || len(s) > 80 {
		return false
	}
	c0 := s[0]
	if !((c0 >= 'A' && c0 <= 'Z') || (c0 >= 'a' && c0 <= 'z') || c0 == '_') {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func loadPE32Image(ctx context.Context, db *sql.DB, binaryPath string) (*peImageInfo, []sectionRow, []importRow, map[uint32]importRow, []exportRow, error) {
	pf, err := pe.Open(binaryPath)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("open pe32 %s: %w", binaryPath, err)
	}
	defer pf.Close()

	oh, ok := pf.OptionalHeader.(*pe.OptionalHeader32)
	if !ok {
		return nil, nil, nil, nil, nil, fmt.Errorf("%s is not a 32-bit PE binary", binaryPath)
	}

	img := &peImageInfo{
		imageBase: oh.ImageBase,
	}

	var secRows []sectionRow
	for idx, sec := range pf.Sections {
		name := strings.TrimRight(sec.Name, "\x00")
		var flags uint32
		if sec.Characteristics&0x40000000 != 0 {
			flags |= extractdb.SecFlagRead
		}
		if sec.Characteristics&0x80000000 != 0 {
			flags |= extractdb.SecFlagWrite
		}
		if sec.Characteristics&0x20000000 != 0 {
			flags |= extractdb.SecFlagExec
		}
		if sec.VirtualSize > 0 || sec.Size > 0 {
			flags |= extractdb.SecFlagAlloc
		}

		data, err := sec.Data()
		if err != nil {
			data = nil
		}
		vsize := sec.VirtualSize
		if vsize == 0 {
			vsize = sec.Size
		}
		if vsize > 0 && int(vsize) < len(data) {
			data = data[:vsize]
		}
		endRVA := sec.VirtualAddress + vsize
		if endRVA > img.maxRVA {
			img.maxRVA = endRVA
		}

		sbuf := peSectionBuf{
			idx:   idx,
			name:  name,
			rva:   sec.VirtualAddress,
			va:    oh.ImageBase + sec.VirtualAddress,
			vsize: vsize,
			raw:   sec.Offset,
			rsize: sec.Size,
			flags: flags,
			data:  data,
		}
		img.sections = append(img.sections, sbuf)
		secRows = append(secRows, sectionRow{
			SecIdx:      idx,
			Name:        name,
			VA:          uint64(sbuf.va),
			VirtualSize: uint64(vsize),
			RawOffset:   uint64(sec.Offset),
			RawSize:     uint64(sec.Size),
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
		case ".rdata":
			if img.rdataSec == nil {
				img.rdataSec = s
			}
		case ".data":
			if img.dataSec == nil {
				img.dataSec = s
			}
		}
	}

	bitmapWords := int((img.maxRVA + 64) >> 6)
	img.relocWordBitmap = make([]uint64, bitmapWords)
	img.relocByteBitmap = make([]uint64, bitmapWords)

	if db != nil {
		if err := insertSections(ctx, db, secRows); err != nil {
			return nil, nil, nil, nil, nil, err
		}
	}

	if err := parseAndInsertPERelocs(ctx, db, oh, img); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	imports, iatMap, err := parsePEImports(oh, img)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	exports, err := parsePEExports(oh, img)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return img, secRows, imports, iatMap, exports, nil
}

func extractPE32(ctx context.Context, db *sql.DB, binaryPath string) (*targetExtractResult, error) {
	img, _, imports, iatMap, exports, err := loadPE32Image(ctx, db, binaryPath)
	if err != nil {
		return nil, err
	}

	if err := insertImports(ctx, db, imports); err != nil {
		return nil, err
	}
	if err := insertExports(ctx, db, exports); err != nil {
		return nil, err
	}

	// Scan strings in .rdata and .data using .reloc byte-overlap filter for UTF-16LE.
	var strSecSlices []dataSectionSlice
	for i := range img.sections {
		s := &img.sections[i]
		if s.name == ".rdata" || s.name == ".data" {
			secRVA := s.rva
			strSecSlices = append(strSecSlices, dataSectionSlice{
				name: s.name,
				va:   uint64(s.va),
				data: s.data,
				hasRelocAt: func(off int) bool {
					return img.hasRelocByteAtRVA(secRVA + uint32(off))
				},
			})
		}
	}
	strIdx, err := scanAndPersistStrings(ctx, db, strSecSlices, false)
	if err != nil {
		return nil, err
	}

	// Classify .text relocated immediates to separate indexed jump tables from direct address-taken tables.
	codeRefs := classifyPETextRelocRefs(img)

	// Extract .rdata vtables (with MSVC RTTI CompleteObjectLocator + /GR- split heuristic).
	vtables := scanPEVtables(img, codeRefs)
	if err := insertVtables(ctx, db, vtables); err != nil {
		return nil, err
	}

	// Extract lua_regs (both 8-byte luaL_Reg and 12-byte Lunar<T> tables).
	luaRegs, lunarTargets := scanPELuaRegs(img, codeRefs)
	if err := insertLuaRegs(ctx, db, luaRegs); err != nil {
		return nil, err
	}

	// Discover function seeds, thunks, calling conventions, stack purge, and xrefs via x86asm.
	seeds, err := analyzePE32FunctionsAndXrefs(ctx, db, img, iatMap, exports, vtables, luaRegs, lunarTargets, codeRefs, strIdx)
	if err != nil {
		return nil, err
	}

	return &targetExtractResult{
		imageBase: uint64(img.imageBase),
		arch:      extractdb.ArchX86,
		format:    extractdb.FormatPE32,
		sources:   []string{"pe32", "x86asm"},
		counts:    make(map[string]int64),
		seeds:     seeds,
	}, nil
}

func parseAndInsertPERelocs(ctx context.Context, db *sql.DB, oh *pe.OptionalHeader32, img *peImageInfo) error {
	if len(oh.DataDirectory) <= pe.IMAGE_DIRECTORY_ENTRY_BASERELOC {
		return nil
	}
	dd := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_BASERELOC]
	if dd.VirtualAddress == 0 || dd.Size == 0 {
		return nil
	}
	relocData, ok := img.readSliceAtRVA(dd.VirtualAddress, dd.Size)
	if !ok {
		return nil
	}

	var b *txStmtBatcher
	if db != nil {
		b = newTxStmtBatcher(ctx, db, `INSERT OR IGNORE INTO relocs(va) VALUES (?)`)
		defer func() { _ = b.Close() }()
	}

	pos := 0
	for pos+8 <= len(relocData) {
		pageRVA := binary.LittleEndian.Uint32(relocData[pos : pos+4])
		blockSize := binary.LittleEndian.Uint32(relocData[pos+4 : pos+8])
		if blockSize < 8 || pos+int(blockSize) > len(relocData) {
			break
		}
		entries := (int(blockSize) - 8) / 2
		for i := 0; i < entries; i++ {
			entry := binary.LittleEndian.Uint16(relocData[pos+8+i*2 : pos+10+i*2])
			typ := entry >> 12
			off := uint32(entry & 0x0FFF)
			if typ == 3 { // IMAGE_REL_BASED_HIGHLOW
				rva := pageRVA + off
				img.setRelocRVA(rva)
				if b != nil {
					va := int64(uint64(img.imageBase) + uint64(rva))
					if err := b.Exec(va); err != nil {
						return fmt.Errorf("insert reloc 0x%x: %w", va, err)
					}
				}
			}
		}
		pos += int(blockSize)
	}
	if b != nil {
		return b.Flush()
	}
	return nil
}

func parsePEImports(oh *pe.OptionalHeader32, img *peImageInfo) ([]importRow, map[uint32]importRow, error) {
	var rows []importRow
	iatMap := make(map[uint32]importRow)
	if len(oh.DataDirectory) <= pe.IMAGE_DIRECTORY_ENTRY_IMPORT {
		return rows, iatMap, nil
	}
	dd := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT]
	if dd.VirtualAddress == 0 || dd.Size == 0 {
		return rows, iatMap, nil
	}

	for descOff := uint32(0); ; descOff += 20 {
		descBytes, ok := img.readSliceAtRVA(dd.VirtualAddress+descOff, 20)
		if !ok {
			break
		}
		origFirstThunk := binary.LittleEndian.Uint32(descBytes[0:4])
		nameRVA := binary.LittleEndian.Uint32(descBytes[12:16])
		firstThunk := binary.LittleEndian.Uint32(descBytes[16:20])
		if origFirstThunk == 0 && nameRVA == 0 && firstThunk == 0 {
			break
		}
		dllName, ok := img.readCStringAtRVA(nameRVA, 256)
		if !ok || dllName == "" {
			continue
		}
		lookupRVA := origFirstThunk
		if lookupRVA == 0 {
			lookupRVA = firstThunk
		}
		if lookupRVA == 0 || firstThunk == 0 {
			continue
		}

		for idx := uint32(0); idx < 65536; idx++ {
			thunkBytes, ok := img.readSliceAtRVA(lookupRVA+idx*4, 4)
			if !ok {
				break
			}
			thunkVal := binary.LittleEndian.Uint32(thunkBytes)
			if thunkVal == 0 {
				break
			}
			iatVA := img.imageBase + firstThunk + idx*4
			var symName string
			var ordinal int
			if thunkVal&0x80000000 != 0 {
				ordinal = int(thunkVal & 0xFFFF)
				symName = fmt.Sprintf("ord_%d", ordinal)
			} else {
				hintBytes, ok := img.readSliceAtRVA(thunkVal, 2)
				if ok {
					ordinal = int(binary.LittleEndian.Uint16(hintBytes))
				}
				symName, _ = img.readCStringAtRVA(thunkVal+2, 512)
				if symName == "" {
					symName = fmt.Sprintf("ord_%d", ordinal)
				}
			}
			row := importRow{
				IATVA:   uint64(iatVA),
				DLLName: dllName,
				SymName: symName,
				Ordinal: ordinal,
			}
			rows = append(rows, row)
			iatMap[iatVA] = row
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].IATVA < rows[j].IATVA })
	return rows, iatMap, nil
}

func parsePEExports(oh *pe.OptionalHeader32, img *peImageInfo) ([]exportRow, error) {
	var rows []exportRow
	if len(oh.DataDirectory) <= pe.IMAGE_DIRECTORY_ENTRY_EXPORT {
		return rows, nil
	}
	dd := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_EXPORT]
	if dd.VirtualAddress == 0 || dd.Size < 40 {
		return rows, nil
	}
	dirBytes, ok := img.readSliceAtRVA(dd.VirtualAddress, 40)
	if !ok {
		return rows, nil
	}
	dirStart := dd.VirtualAddress
	dirEnd := dd.VirtualAddress + dd.Size

	base := binary.LittleEndian.Uint32(dirBytes[16:20])
	numFuncs := binary.LittleEndian.Uint32(dirBytes[20:24])
	numNames := binary.LittleEndian.Uint32(dirBytes[24:28])
	addrOfFuncs := binary.LittleEndian.Uint32(dirBytes[28:32])
	addrOfNames := binary.LittleEndian.Uint32(dirBytes[32:36])
	addrOfNameOrds := binary.LittleEndian.Uint32(dirBytes[36:40])

	nameByFuncIdx := make(map[uint32]string, numNames)
	for i := uint32(0); i < numNames && i < 65536; i++ {
		nameRVABytes, ok1 := img.readSliceAtRVA(addrOfNames+i*4, 4)
		ordBytes, ok2 := img.readSliceAtRVA(addrOfNameOrds+i*2, 2)
		if !ok1 || !ok2 {
			break
		}
		nameRVA := binary.LittleEndian.Uint32(nameRVABytes)
		funcIdx := uint32(binary.LittleEndian.Uint16(ordBytes))
		if name, ok := img.readCStringAtRVA(nameRVA, 512); ok && name != "" {
			nameByFuncIdx[funcIdx] = name
		}
	}

	var nextID int64 = 1
	for funcIdx := uint32(0); funcIdx < numFuncs && funcIdx < 65536; funcIdx++ {
		fnBytes, ok := img.readSliceAtRVA(addrOfFuncs+funcIdx*4, 4)
		if !ok {
			break
		}
		fnRVA := binary.LittleEndian.Uint32(fnBytes)
		if fnRVA == 0 {
			continue
		}
		// Exclude forwarded exports pointing inside the export directory.
		if fnRVA >= dirStart && fnRVA < dirEnd {
			continue
		}
		ordinal := int(base + funcIdx)
		name := nameByFuncIdx[funcIdx]
		if name == "" {
			name = fmt.Sprintf("ord_%d", ordinal)
		}
		rows = append(rows, exportRow{
			ExportID: nextID,
			EA:       uint64(img.imageBase + fnRVA),
			Name:     name,
			Ordinal:  ordinal,
		})
		nextID++
	}
	return rows, nil
}

type peCodeRelocRefs struct {
	isIndexedJumpTable       map[uint32]bool
	isIndexedCallTable       map[uint32]bool
	isJumpTableSlot          map[uint32]bool
	hasDirectAddrTakenInCode map[uint32]bool
	hasStoreImmToMem         map[uint32]bool
}

func classifyPETextRelocRefs(img *peImageInfo) *peCodeRelocRefs {
	refs := &peCodeRelocRefs{
		isIndexedJumpTable:       make(map[uint32]bool),
		isIndexedCallTable:       make(map[uint32]bool),
		isJumpTableSlot:          make(map[uint32]bool),
		hasDirectAddrTakenInCode: make(map[uint32]bool),
		hasStoreImmToMem:         make(map[uint32]bool),
	}
	if img.textSec == nil {
		return refs
	}
	tb := img.textSec.data
	baseRVA := img.textSec.rva
	for off := 0; off+4 <= len(tb); off++ {
		if !img.hasRelocAtRVA(baseRVA + uint32(off)) {
			continue
		}
		targetVA := binary.LittleEndian.Uint32(tb[off : off+4])
		if isPrecededByIndexedJumpOrSwitchRead(tb, off) {
			refs.isIndexedJumpTable[targetVA] = true
			continue
		}
		if isPrecededByIndexedCallOrLoad(tb, off) {
			refs.isIndexedCallTable[targetVA] = true
			continue
		}
		refs.hasDirectAddrTakenInCode[targetVA] = true
		if isPrecededByStoreImmToMem(tb, off) {
			refs.hasStoreImmToMem[targetVA] = true
		}
	}

	if img.rdataSec != nil {
		rd := img.rdataSec.data
		rdVA := img.rdataSec.va
		rdRVA := img.rdataSec.rva
		rdEnd := rdVA + uint32(len(rd))
		for baseVA := range refs.isIndexedJumpTable {
			if baseVA < rdVA || baseVA >= rdEnd {
				continue
			}
			for off := int(baseVA - rdVA); off+4 <= len(rd); off += 4 {
				curVA := rdVA + uint32(off)
				if curVA > baseVA && refs.hasDirectAddrTakenInCode[curVA] && !refs.isIndexedJumpTable[curVA] {
					break
				}
				if !img.hasRelocAtRVA(rdRVA + uint32(off)) {
					break
				}
				target := binary.LittleEndian.Uint32(rd[off : off+4])
				if !img.inText(target) {
					break
				}
				refs.isJumpTableSlot[curVA] = true
			}
		}
	}
	return refs
}

func isFollowedByJmpReg(tb []byte, afterImm int) bool {
	limit := afterImm + 6
	if limit > len(tb) {
		limit = len(tb)
	}
	for i := afterImm; i+1 < limit; i++ {
		if tb[i] == 0xFF && tb[i+1] >= 0xE0 && tb[i+1] <= 0xE7 {
			return true
		}
	}
	return false
}

func isPrecededByIndexedJumpOrSwitchRead(tb []byte, immOff int) bool {
	// jmp dword ptr [reg*4 + disp32]: FF 24 x5 <disp32>
	if immOff >= 3 && tb[immOff-3] == 0xFF && tb[immOff-2] == 0x24 && (tb[immOff-1]&0xC7) == 0x85 {
		return true
	}
	// jmp dword ptr [reg + disp32]: FF A0..A7 <disp32> (rm != 4)
	if immOff >= 2 && tb[immOff-2] == 0xFF && (tb[immOff-1]&0xF8) == 0xA0 && (tb[immOff-1]&0x07) != 0x04 {
		return true
	}
	// jmp dword ptr [base + idx*scale + disp32]: FF A4 <sib> <disp32>
	if immOff >= 3 && tb[immOff-3] == 0xFF && tb[immOff-2] == 0xA4 {
		return true
	}
	// mov reg, dword ptr [reg*4 + disp32] followed within 6 bytes by jmp reg: 8B /r 04 x5 <disp32>
	if immOff >= 3 && tb[immOff-3] == 0x8B && (tb[immOff-2]&0xC7) == 0x04 && (tb[immOff-1]&0xC7) == 0x85 {
		if isFollowedByJmpReg(tb, immOff+4) {
			return true
		}
	}
	// movzx reg, byte ptr [reg + disp32]: 0F B6 /r <disp32> (mod=10, e.g. 80..BF, or SIB 84/etc.)
	if immOff >= 3 && tb[immOff-3] == 0x0F && tb[immOff-2] == 0xB6 && (tb[immOff-1]&0xC0) == 0x80 && (tb[immOff-1]&0x07) != 0x04 {
		return true
	}
	if immOff >= 4 && tb[immOff-4] == 0x0F && tb[immOff-3] == 0xB6 && (tb[immOff-2]&0xC7) == 0x84 {
		return true
	}
	return false
}

func isPrecededByIndexedCallOrLoad(tb []byte, immOff int) bool {
	// call dword ptr [reg*4 + disp32]: FF 14 x5 <disp32>
	if immOff >= 3 && tb[immOff-3] == 0xFF && tb[immOff-2] == 0x14 && (tb[immOff-1]&0xC7) == 0x85 {
		return true
	}
	// call dword ptr [reg + disp32]: FF 90..97 <disp32> (rm != 4)
	if immOff >= 2 && tb[immOff-2] == 0xFF && (tb[immOff-1]&0xF8) == 0x90 && (tb[immOff-1]&0x07) != 0x04 {
		return true
	}
	// call dword ptr [base + idx*scale + disp32]: FF 94 <sib> <disp32>
	if immOff >= 3 && tb[immOff-3] == 0xFF && tb[immOff-2] == 0x94 {
		return true
	}
	// mov reg, dword ptr [reg*4 + disp32] (not immediately jumped to): 8B /r 04 x5 <disp32>
	if immOff >= 3 && tb[immOff-3] == 0x8B && (tb[immOff-2]&0xC7) == 0x04 && (tb[immOff-1]&0xC7) == 0x85 {
		return !isFollowedByJmpReg(tb, immOff+4)
	}
	return false
}

func isPrecededByStoreImmToMem(tb []byte, immOff int) bool {
	// C7 /0: mov dword ptr [reg (+ disp8/disp32)], imm32
	// ModRM with reg==0 has (modrm & 0x38) == 0.
	// Look back 1..6 bytes for 0xC7 depending on addressing mode (no disp, disp8, SIB, disp32).
	if immOff >= 2 && tb[immOff-2] == 0xC7 && (tb[immOff-1]&0x38) == 0x00 && (tb[immOff-1]&0xC0) == 0x00 && (tb[immOff-1]&0x07) != 0x04 && (tb[immOff-1]&0x07) != 0x05 {
		return true
	}
	if immOff >= 3 && tb[immOff-3] == 0xC7 && (tb[immOff-2]&0x38) == 0x00 {
		mod := tb[immOff-2] & 0xC0
		rm := tb[immOff-2] & 0x07
		if (mod == 0x00 && rm == 0x04) || (mod == 0x40 && rm != 0x04) {
			return true
		}
	}
	if immOff >= 4 && tb[immOff-4] == 0xC7 && (tb[immOff-3]&0xF8) == 0x40 && (tb[immOff-3]&0x07) == 0x04 {
		return true
	}
	if immOff >= 6 && tb[immOff-6] == 0xC7 && (tb[immOff-5]&0x38) == 0x00 {
		mod := tb[immOff-5] & 0xC0
		rm := tb[immOff-5] & 0x07
		if (mod == 0x80 && rm != 0x04) || (mod == 0x00 && rm == 0x05) {
			return true
		}
	}
	if immOff >= 7 && tb[immOff-7] == 0xC7 && (tb[immOff-6]&0xF8) == 0x80 && (tb[immOff-6]&0x07) == 0x04 {
		return true
	}
	return false
}

func demangleMSVCTypeDescriptor(mangled string) string {
	s := mangled
	if strings.HasPrefix(s, ".?AV") || strings.HasPrefix(s, ".?AU") {
		s = s[4:]
	} else {
		return mangled
	}
	s = strings.TrimSuffix(s, "@@")
	// Simple non-template MSVC scope chain: Class@SubNS@NS -> NS::SubNS::Class
	if !strings.Contains(s, "?") {
		parts := strings.Split(s, "@")
		var clean []string
		for i := len(parts) - 1; i >= 0; i-- {
			p := parts[i]
			if p == "" {
				continue
			}
			if p[0] >= '0' && p[0] <= '9' {
				if len(p) == 1 {
					refIdx := int(p[0] - '0')
					if refIdx < len(parts) && parts[refIdx] != "" && (parts[refIdx][0] < '0' || parts[refIdx][0] > '9') {
						p = parts[refIdx]
					} else {
						return mangled
					}
				} else {
					return mangled
				}
			}
			clean = append(clean, p)
		}
		if len(clean) > 0 {
			return strings.Join(clean, "::")
		}
	}
	return mangled
}

func scanMSVCRTTICompleteObjectLocators(img *peImageInfo) map[uint32]string {
	colOwnerByVA := make(map[uint32]string)
	if img.rdataSec == nil {
		return colOwnerByVA
	}
	rdStart := img.rdataSec.va
	rdEnd := rdStart + uint32(len(img.rdataSec.data))

	typeDescByVA := make(map[uint32]string)
	scanTD := func(sec *peSectionBuf) {
		if sec == nil || len(sec.data) < 16 {
			return
		}
		for _, prefix := range [][]byte{[]byte(".?AV"), []byte(".?AU")} {
			idx := 0
			for {
				f := bytes.Index(sec.data[idx:], prefix)
				if f < 0 {
					break
				}
				strOff := idx + f
				idx = strOff + 4
				if strOff < 8 {
					continue
				}
				tdOff := strOff - 8
				end := bytes.IndexByte(sec.data[strOff:], 0)
				if end <= 4 || end > 256 {
					continue
				}
				pvf := binary.LittleEndian.Uint32(sec.data[tdOff : tdOff+4])
				spare := binary.LittleEndian.Uint32(sec.data[tdOff+4 : tdOff+8])
				if spare == 0 && pvf >= rdStart && pvf < rdEnd {
					tdVA := sec.va + uint32(tdOff)
					rawName := string(sec.data[strOff : strOff+end])
					typeDescByVA[tdVA] = demangleMSVCTypeDescriptor(rawName)
				}
			}
		}
	}
	scanTD(img.dataSec)
	scanTD(img.rdataSec)

	if len(typeDescByVA) == 0 {
		return colOwnerByVA
	}

	rd := img.rdataSec.data
	for off := 0; off+20 <= len(rd); off += 4 {
		sig := binary.LittleEndian.Uint32(rd[off : off+4])
		if sig != 0 {
			continue
		}
		tdVA := binary.LittleEndian.Uint32(rd[off+12 : off+16])
		owner, ok := typeDescByVA[tdVA]
		if !ok {
			continue
		}
		cdVA := binary.LittleEndian.Uint32(rd[off+16 : off+20])
		if cdVA >= rdStart && cdVA < rdEnd {
			colOwnerByVA[rdStart+uint32(off)] = owner
		}
	}
	return colOwnerByVA
}

func scanPEVtables(img *peImageInfo, codeRefs *peCodeRelocRefs) []vtableRow {
	if img.rdataSec == nil || img.textSec == nil {
		return nil
	}
	colOwners := scanMSVCRTTICompleteObjectLocators(img)
	rd := img.rdataSec.data
	rdVA := img.rdataSec.va
	rdRVA := img.rdataSec.rva

	isRelocTextPtr := func(off int) (uint32, bool) {
		if off < 0 || off+4 > len(rd) {
			return 0, false
		}
		if !img.hasRelocAtRVA(rdRVA + uint32(off)) {
			return 0, false
		}
		target := binary.LittleEndian.Uint32(rd[off : off+4])
		if !img.inText(target) {
			return 0, false
		}
		return target, true
	}

	precedingCOLOwner := func(off int) (string, bool) {
		if off < 4 {
			return "", false
		}
		if !img.hasRelocAtRVA(rdRVA + uint32(off-4)) {
			return "", false
		}
		colPtr := binary.LittleEndian.Uint32(rd[off-4 : off])
		owner, ok := colOwners[colPtr]
		return owner, ok
	}

	var rows []vtableRow
	off := 0
	for off+4 <= len(rd) {
		firstTarget, ok := isRelocTextPtr(off)
		if !ok {
			off += 4
			continue
		}
		vtableVA := rdVA + uint32(off)
		owner, hasCOL := precedingCOLOwner(off)
		hasDirectRef := codeRefs.hasDirectAddrTakenInCode[vtableVA] && !codeRefs.isIndexedJumpTable[vtableVA] && !codeRefs.isIndexedCallTable[vtableVA]
		if !hasDirectRef && !hasCOL {
			off += 4
			continue
		}

		// Collect contiguous relocated .text pointers until end of run or split point.
		targets := []uint32{firstTarget}
		nextOff := off + 4
		for nextOff+4 <= len(rd) {
			tVA, ok := isRelocTextPtr(nextOff)
			if !ok {
				break
			}
			curVA := rdVA + uint32(nextOff)
			if _, nextHasCOL := precedingCOLOwner(nextOff); nextHasCOL {
				break
			}
			if codeRefs.hasDirectAddrTakenInCode[curVA] || codeRefs.isIndexedJumpTable[curVA] || codeRefs.isIndexedCallTable[curVA] {
				break
			}
			targets = append(targets, tVA)
			nextOff += 4
		}

		minSlots := 2
		if hasCOL || codeRefs.hasStoreImmToMem[vtableVA] {
			minSlots = 1
		}
		if len(targets) >= minSlots && !codeRefs.isIndexedJumpTable[vtableVA] && !codeRefs.isIndexedCallTable[vtableVA] {
			for slot, tVA := range targets {
				rows = append(rows, vtableRow{
					VtableEA:  uint64(vtableVA),
					Slot:      slot,
					TargetEA:  uint64(tVA),
					OwnerName: owner,
				})
			}
		}
		off = nextOff
	}
	return rows
}

func scanPELuaRegs(img *peImageInfo, codeRefs *peCodeRelocRefs) ([]luaRegRow, map[uint64]bool) {
	var rows []luaRegRow
	lunarTargets := make(map[uint64]bool)

	for i := range img.sections {
		sec := &img.sections[i]
		if sec.name != ".rdata" && sec.name != ".data" {
			continue
		}
		d := sec.data
		claimed := make([]bool, len(d))

		// Pass 1: 12-byte MSVC Lunar<T>::RegType {const char* name, int (T::*mfunc)(lua_State*), int32 this_adj == 0}
		// Terminated by 12 zero bytes.
		for off := 0; off+24 <= len(d); {
			run := 0
			for p := off; p+12 <= len(d); p += 12 {
				sPtr := binary.LittleEndian.Uint32(d[p : p+4])
				fPtr := binary.LittleEndian.Uint32(d[p+4 : p+8])
				adj := binary.LittleEndian.Uint32(d[p+8 : p+12])
				if sPtr == 0 && fPtr == 0 && adj == 0 {
					break
				}
				if adj != 0 || img.hasRelocAtRVA(sec.rva+uint32(p+8)) {
					run = 0
					break
				}
				if !img.hasRelocAtRVA(sec.rva+uint32(p)) || !img.hasRelocAtRVA(sec.rva+uint32(p+4)) {
					run = 0
					break
				}
				if !img.inText(fPtr) {
					run = 0
					break
				}
				if _, ok := img.readIdentStringAtVA(sPtr); !ok {
					run = 0
					break
				}
				run++
			}
			termOff := off + run*12
			tableVA := sec.va + uint32(off)
			hasTerm := termOff+12 <= len(d) &&
				binary.LittleEndian.Uint32(d[termOff:termOff+4]) == 0 &&
				binary.LittleEndian.Uint32(d[termOff+4:termOff+8]) == 0 &&
				binary.LittleEndian.Uint32(d[termOff+8:termOff+12]) == 0 &&
				!img.hasRelocAtRVA(sec.rva+uint32(termOff)) &&
				!img.hasRelocAtRVA(sec.rva+uint32(termOff+4))

			if hasTerm && (run >= 3 || (run >= 1 && codeRefs.hasDirectAddrTakenInCode[tableVA])) {
				for k := 0; k < run; k++ {
					p := off + k*12
					sPtr := binary.LittleEndian.Uint32(d[p : p+4])
					fPtr := binary.LittleEndian.Uint32(d[p+4 : p+8])
					nm, _ := img.readIdentStringAtVA(sPtr)
					rows = append(rows, luaRegRow{
						TableEA:    uint64(tableVA),
						MethodName: nm,
						FuncEA:     uint64(fPtr),
					})
					lunarTargets[uint64(fPtr)] = true
				}
				for b := off; b < termOff+12 && b < len(claimed); b++ {
					claimed[b] = true
				}
				off = termOff + 12
			} else {
				off += 4
			}
		}

		// Pass 2: 8-byte luaL_Reg {const char* name, lua_CFunction func}
		// Terminated by 8 zero bytes.
		for off := 0; off+16 <= len(d); {
			if claimed[off] {
				off += 4
				continue
			}
			run := 0
			for p := off; p+8 <= len(d); p += 8 {
				if claimed[p] {
					run = 0
					break
				}
				sPtr := binary.LittleEndian.Uint32(d[p : p+4])
				fPtr := binary.LittleEndian.Uint32(d[p+4 : p+8])
				if sPtr == 0 && fPtr == 0 {
					break
				}
				if !img.hasRelocAtRVA(sec.rva+uint32(p)) || !img.hasRelocAtRVA(sec.rva+uint32(p+4)) {
					run = 0
					break
				}
				if !img.inText(fPtr) {
					run = 0
					break
				}
				if _, ok := img.readIdentStringAtVA(sPtr); !ok {
					run = 0
					break
				}
				run++
			}
			termOff := off + run*8
			tableVA := sec.va + uint32(off)
			hasTerm := termOff+8 <= len(d) &&
				binary.LittleEndian.Uint32(d[termOff:termOff+4]) == 0 &&
				binary.LittleEndian.Uint32(d[termOff+4:termOff+8]) == 0 &&
				!img.hasRelocAtRVA(sec.rva+uint32(termOff)) &&
				!img.hasRelocAtRVA(sec.rva+uint32(termOff+4))

			if hasTerm && (run >= 3 || (run >= 1 && codeRefs.hasDirectAddrTakenInCode[tableVA])) {
				for k := 0; k < run; k++ {
					p := off + k*8
					sPtr := binary.LittleEndian.Uint32(d[p : p+4])
					fPtr := binary.LittleEndian.Uint32(d[p+4 : p+8])
					nm, _ := img.readIdentStringAtVA(sPtr)
					rows = append(rows, luaRegRow{
						TableEA:    uint64(tableVA),
						MethodName: nm,
						FuncEA:     uint64(fPtr),
					})
				}
				off = termOff + 8
			} else {
				off += 4
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
	return rows, lunarTargets
}
