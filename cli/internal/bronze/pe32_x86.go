package bronze

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"sort"

	"golang.org/x/arch/x86/x86asm"

	"twdll/cli/internal/extractdb"
)

type peFuncSeedInfo struct {
	ea            uint32
	name          string
	discoveryMask uint32
	vtableEA      uint64
	vtableSlot    int
	hasVtable     bool
	flags         uint32
	thunkTargetEA uint64
	isThunk       bool
	thunkSize     uint32
	refCount      int
}

func analyzePE32FunctionsAndXrefs(
	ctx context.Context,
	db *sql.DB,
	img *peImageInfo,
	iatMap map[uint32]importRow,
	exports []exportRow,
	vtables []vtableRow,
	luaRegs []luaRegRow,
	lunarTargets map[uint64]bool,
	codeRefs *peCodeRelocRefs,
	strIdx *stringIndex,
) ([]FunctionSeed, error) {
	if img.textSec == nil || len(img.textSec.data) == 0 {
		return nil, nil
	}
	tb := img.textSec.data
	textVA := img.textSec.va
	textEnd := textVA + uint32(len(tb))

	seedMap := make(map[uint32]*peFuncSeedInfo, 32768)
	getOrCreateSeed := func(va uint32) *peFuncSeedInfo {
		s, ok := seedMap[va]
		if !ok {
			s = &peFuncSeedInfo{
				ea:         va,
				vtableSlot: -1,
			}
			seedMap[va] = s
		}
		return s
	}

	// 1. Seed Tier-1 entrypoints: exports, vtables, lua_regs, and non-jump-table data pointers.
	for _, exp := range exports {
		va := uint32(exp.EA)
		if !img.inText(va) {
			continue
		}
		s := getOrCreateSeed(va)
		s.discoveryMask |= extractdb.DiscoveryExport
		if s.name == "" && exp.Name != "" {
			s.name = exp.Name
		}
	}

	for _, vt := range vtables {
		va := uint32(vt.TargetEA)
		if !img.inText(va) {
			continue
		}
		s := getOrCreateSeed(va)
		s.discoveryMask |= extractdb.DiscoveryVtable
		s.flags |= extractdb.FuncFlagVirtual
		s.refCount++
		if !s.hasVtable {
			s.hasVtable = true
			s.vtableEA = vt.VtableEA
			s.vtableSlot = vt.Slot
		}
	}

	for _, lr := range luaRegs {
		va := uint32(lr.FuncEA)
		if !img.inText(va) {
			continue
		}
		s := getOrCreateSeed(va)
		s.discoveryMask |= extractdb.DiscoveryLuaReg
		s.refCount++
		if s.name == "" && lr.MethodName != "" {
			s.name = fmt.Sprintf("lua_%x_%s", lr.TableEA, lr.MethodName)
		}
	}

	// Scan .rdata and .data relocated pointers into .text that start with a valid prologue.
	for i := range img.sections {
		sec := &img.sections[i]
		if sec.name != ".rdata" && sec.name != ".data" {
			continue
		}
		for off := 0; off+4 <= len(sec.data); off += 4 {
			if !img.hasRelocAtRVA(sec.rva + uint32(off)) {
				continue
			}
			ptrVA := sec.va + uint32(off)
			if codeRefs.isJumpTableSlot[ptrVA] || codeRefs.isIndexedJumpTable[ptrVA] {
				continue
			}
			targetVA := binary.LittleEndian.Uint32(sec.data[off : off+4])
			if !img.inText(targetVA) {
				continue
			}
			tOff := int(targetVA - textVA)
			if isStrongPE32FunctionPrologue(tb, tOff, textVA, img) ||
				(isPrecededByFunctionBoundary(tb, tOff, textVA, img) && isValidPE32FunctionStart(tb, tOff, img)) {
				s := getOrCreateSeed(targetVA)
				s.discoveryMask |= extractdb.DiscoveryDataPtr
			}
		}
	}

	// Also seed direct code address-taken targets in .text if they start with a valid prologue.
	for targetVA := range codeRefs.hasDirectAddrTakenInCode {
		if codeRefs.isJumpTableSlot[targetVA] || codeRefs.isIndexedJumpTable[targetVA] {
			continue
		}
		if img.inText(targetVA) {
			tOff := int(targetVA - textVA)
			if isStrongPE32FunctionPrologue(tb, tOff, textVA, img) ||
				(isPrecededByFunctionBoundary(tb, tOff, textVA, img) && isValidPE32FunctionStart(tb, tOff, img)) {
				s := getOrCreateSeed(targetVA)
				s.discoveryMask |= extractdb.DiscoveryDataPtr
			}
		}
	}

	// Scan .text PUSH imm32 (0x68 <reloc>) operands (e.g. _atexit destructor thunks) pointing into .text.
	textRVA := img.textSec.rva
	for off := 1; off+4 <= len(tb); off++ {
		if tb[off-1] != 0x68 || !img.hasRelocAtRVA(textRVA+uint32(off)) {
			continue
		}
		targetVA := binary.LittleEndian.Uint32(tb[off : off+4])
		if codeRefs.isJumpTableSlot[targetVA] || codeRefs.isIndexedJumpTable[targetVA] || !img.inText(targetVA) {
			continue
		}
		tOff := int(targetVA - textVA)
		if isPrecededByFunctionBoundary(tb, tOff, textVA, img) && isValidPE32FunctionStart(tb, tOff, img) {
			s := getOrCreateSeed(targetVA)
			s.discoveryMask |= extractdb.DiscoveryDataPtr
		}
	}

	// 2. Iterative instruction-synchronized decode from known seeds to discover E8 call targets.
	decodedInstStart := make([]bool, len(tb))
	queue := make([]uint32, 0, len(seedMap))
	for va := range seedMap {
		queue = append(queue, va)
	}
	sort.Slice(queue, func(i, j int) bool { return queue[i] < queue[j] })

	visitedSeed := make(map[uint32]bool, len(seedMap)*2)
	for len(queue) > 0 {
		fnVA := queue[0]
		queue = queue[1:]
		if visitedSeed[fnVA] || !img.inText(fnVA) {
			continue
		}
		visitedSeed[fnVA] = true

		startOff := int(fnVA - textVA)
		// Check if this function is an immediate thunk first.
		if isThunk, tTarget, _ := detectPE32Thunk(tb, startOff, textVA, img); isThunk {
			decodedInstStart[startOff] = true
			if img.inText(uint32(tTarget)) {
				ts := getOrCreateSeed(uint32(tTarget))
				ts.discoveryMask |= extractdb.DiscoveryCallTarget
				if !visitedSeed[uint32(tTarget)] {
					queue = append(queue, uint32(tTarget))
				}
			}
			continue
		}

		// Decode up to 32768 bytes across forward branches to harvest synchronized E8 calls.
		off := startOff
		furthestBranchOff := startOff
		maxOff := startOff + 32768
		if maxOff > len(tb) {
			maxOff = len(tb)
		}
		for off < maxOff {
			if off > startOff && seedMap[textVA+uint32(off)] != nil {
				break
			}
			if off > furthestBranchOff && tb[off] == 0xCC {
				break
			}
			inst, err := x86asm.Decode(tb[off:], 32)
			if err != nil || inst.Len <= 0 {
				break
			}
			decodedInstStart[off] = true
			instVA := textVA + uint32(off)
			if inst.Op == x86asm.CALL && tb[off] == 0xE8 && inst.Len == 5 {
				// Ensure the E8 displacement doesn't overlap a .reloc entry (HIGHLOW reloc is absolute, not rel32).
				if !img.hasRelocByteAtRVA(img.textSec.rva+uint32(off+1)) && !img.hasRelocByteAtRVA(img.textSec.rva+uint32(off+4)) {
					rel := int32(binary.LittleEndian.Uint32(tb[off+1 : off+5]))
					calleeVA := uint32(int64(textVA) + int64(off+5) + int64(rel))
					if calleeVA >= textVA && calleeVA < textEnd {
						s := getOrCreateSeed(calleeVA)
						s.discoveryMask |= extractdb.DiscoveryCallTarget
						if !visitedSeed[calleeVA] {
							queue = append(queue, calleeVA)
						}
					}
				}
			} else if inst.Op == x86asm.JMP {
				if tb[off] == 0xE9 && inst.Len == 5 &&
					!img.hasRelocByteAtRVA(img.textSec.rva+uint32(off+1)) &&
					!img.hasRelocByteAtRVA(img.textSec.rva+uint32(off+4)) {
					rel := int32(binary.LittleEndian.Uint32(tb[off+1 : off+5]))
					target := uint32(int64(instVA) + 5 + int64(rel))
					if off == startOff+5 && tb[startOff] == 0xB9 && img.hasRelocAtRVA(textRVA+uint32(startOff+1)) {
						if target >= textVA && target < textEnd {
							s := getOrCreateSeed(target)
							s.discoveryMask |= extractdb.DiscoveryCallTarget
							if !visitedSeed[target] {
								queue = append(queue, target)
							}
						}
					} else if tOff := int(target - textVA); tOff > furthestBranchOff && tOff < maxOff {
						furthestBranchOff = tOff
					}
				} else if tb[off] == 0xEB && inst.Len == 2 {
					rel := int8(tb[off+1])
					target := uint32(int64(instVA) + 2 + int64(rel))
					if tOff := int(target - textVA); tOff > furthestBranchOff && tOff < maxOff {
						furthestBranchOff = tOff
					}
				}
			} else {
				for _, arg := range inst.Args {
					if rel, ok := arg.(x86asm.Rel); ok {
						target := uint32(int64(instVA) + int64(inst.Len) + int64(rel))
						if tOff := int(target - textVA); tOff > furthestBranchOff && tOff < maxOff {
							furthestBranchOff = tOff
						}
					}
				}
			}
			off += inst.Len
			if (inst.Op == x86asm.RET || inst.Op == x86asm.JMP) && off > furthestBranchOff {
				break
			}
		}
	}

	// 3. Residual .text sweep for E8 rel32 call targets with prologue + boundary validation.
	for off := 0; off+5 <= len(tb); off++ {
		if tb[off] != 0xE8 {
			continue
		}
		if img.hasRelocByteAtRVA(textRVA+uint32(off)) ||
			img.hasRelocByteAtRVA(textRVA+uint32(off+1)) ||
			img.hasRelocByteAtRVA(textRVA+uint32(off+2)) ||
			img.hasRelocByteAtRVA(textRVA+uint32(off+3)) ||
			img.hasRelocByteAtRVA(textRVA+uint32(off+4)) {
			continue
		}
		rel := int32(binary.LittleEndian.Uint32(tb[off+1 : off+5]))
		calleeVA := uint32(int64(textVA) + int64(off+5) + int64(rel))
		if calleeVA < textVA || calleeVA >= textEnd {
			continue
		}
		if seedMap[calleeVA] != nil {
			continue
		}
		cOff := int(calleeVA - textVA)
		if !isPrecededByFunctionBoundary(tb, cOff, textVA, img) {
			continue
		}
		if !isValidPE32FunctionStart(tb, cOff, img) {
			continue
		}
		s := getOrCreateSeed(calleeVA)
		if decodedInstStart[off] {
			s.discoveryMask |= extractdb.DiscoveryCallTarget
		} else {
			s.discoveryMask |= extractdb.DiscoveryCallTarget | extractdb.DiscoveryPrologue
		}
	}

	// 4. Sort seeds by EA and analyze each function's boundary, thunk status, calling convention, and xrefs.
	sortedVAs := make([]uint32, 0, len(seedMap))
	for va := range seedMap {
		sortedVAs = append(sortedVAs, va)
	}
	sort.Slice(sortedVAs, func(i, j int) bool { return sortedVAs[i] < sortedVAs[j] })

	funcRows := make([]functionRow, 0, len(sortedVAs))
	rangeRows := make([]funcRangeRow, 0, len(sortedVAs))
	seeds := make([]FunctionSeed, 0, len(sortedVAs))
	helperCache := make(map[uint32]bool, 64)
	var (
		callSites   []callSiteRow
		dataXrefs   []dataXrefRow
		stringXrefs []stringXrefRow
	)

	for i, va := range sortedVAs {
		s := seedMap[va]
		startOff := int(va - textVA)
		nextVA := textEnd
		if i+1 < len(sortedVAs) {
			nextVA = sortedVAs[i+1]
		}
		maxEndOff := int(nextVA - textVA)
		if maxEndOff > len(tb) {
			maxEndOff = len(tb)
		}

		// Detect FF 25 [iat_va] or E9 rel32 thunks.
		isThunk, thunkTarget, thunkSize := detectPE32Thunk(tb, startOff, textVA, img)
		var fnSize uint32
		var cc, ccSrc string
		var stackPurge int

		if isThunk {
			s.isThunk = true
			s.flags |= extractdb.FuncFlagThunk
			s.thunkTargetEA = thunkTarget
			fnSize = thunkSize
			if imp, ok := iatMap[uint32(thunkTarget)]; ok {
				s.flags |= extractdb.FuncFlagExternal
				if s.name == "" {
					s.name = "j_" + imp.SymName
				}
			}
			cc = "__cdecl"
			ccSrc = extractdb.CCSourceX86Asm
		} else {
			res := analyzePE32Body(tb, startOff, maxEndOff, va, img, strIdx, helperCache)
			fnSize = res.size
			stackPurge = res.stackPurge
			callSites = append(callSites, res.callSites...)
			dataXrefs = append(dataXrefs, res.dataXrefs...)
			stringXrefs = append(stringXrefs, res.stringXrefs...)

			// Classify calling convention and cc_source.
			switch {
			case s.discoveryMask&extractdb.DiscoveryVtable != 0:
				cc = "__thiscall"
				ccSrc = extractdb.CCSourceVtable
			case s.discoveryMask&extractdb.DiscoveryLuaReg != 0:
				if lunarTargets[uint64(va)] || stackPurge == 4 || res.readsECX {
					cc = "__thiscall"
				} else {
					cc = "__cdecl"
				}
				ccSrc = extractdb.CCSourceLuaReg
			default:
				if res.readsECX {
					cc = "__thiscall"
				} else if stackPurge > 0 {
					cc = "__stdcall"
				} else {
					cc = "__cdecl"
				}
				ccSrc = extractdb.CCSourceX86Asm
			}
		}

		if fnSize == 0 {
			fnSize = uint32(maxEndOff - startOff)
		}
		if !isThunk && isPE32CRTStackOrEHHelper(tb, startOff, fnSize, textVA, va, helperCache) {
			s.flags |= extractdb.FuncFlagNoSeedOverride
		}
		if s.refCount >= 2 || (!isThunk && fnSize <= 16 && (s.discoveryMask&(extractdb.DiscoveryVtable|extractdb.DiscoveryLuaReg)) != 0) {
			s.flags |= extractdb.FuncFlagICFCandidate
		}
		if s.name == "" {
			s.name = formatSubName(uint64(va))
		}

		fRow := functionRow{
			EA:            uint64(va),
			Size:          uint64(fnSize),
			Name:          s.name,
			CallingConv:   cc,
			CCSource:      ccSrc,
			StackPurge:    stackPurge,
			DiscoveryMask: s.discoveryMask,
			ThunkTargetEA: nullUint64NonZero(s.thunkTargetEA),
			VtableEA:      nullInt64(int64(s.vtableEA), s.hasVtable),
			VtableSlot:    nullInt64(int64(s.vtableSlot), s.hasVtable),
			Flags:         s.flags,
		}
		funcRows = append(funcRows, fRow)
		rangeRows = append(rangeRows, funcRangeRow{
			StartEA: uint64(va),
			EndEA:   uint64(va + fnSize),
			FuncEA:  uint64(va),
		})
		seeds = append(seeds, FunctionSeed{
			EA:            uint64(va),
			Name:          s.name,
			CallingConv:   cc,
			CCSource:      ccSrc,
			StackPurge:    stackPurge,
			DiscoveryMask: s.discoveryMask,
			ThunkTargetEA: s.thunkTargetEA,
			VtableEA:      s.vtableEA,
			VtableSlot:    s.vtableSlot,
			Flags:         s.flags,
		})
	}

	if err := insertFunctions(ctx, db, funcRows); err != nil {
		return nil, err
	}
	if err := insertFuncRanges(ctx, db, rangeRows); err != nil {
		return nil, err
	}
	if err := insertCallSites(ctx, db, callSites); err != nil {
		return nil, err
	}
	if err := insertDataXrefs(ctx, db, dataXrefs); err != nil {
		return nil, err
	}
	if err := applyStringXrefsAndDFCount(ctx, db, stringXrefs); err != nil {
		return nil, err
	}

	return seeds, nil
}

func isPrecededByFunctionBoundary(tb []byte, off int, textVA uint32, img *peImageInfo) bool {
	if off == 0 {
		return true
	}
	prev := tb[off-1]
	if prev == 0xCC || prev == 0x90 || prev == 0xC3 {
		return true
	}
	if off >= 3 && tb[off-3] == 0xC2 && tb[off-1] == 0x00 {
		return true
	}
	if off >= 5 && tb[off-5] == 0xE9 {
		if isThunk, _, sz := detectPE32Thunk(tb, off-5, textVA, img); isThunk && sz == 5 {
			return true
		}
	}
	if off >= 6 && tb[off-6] == 0xFF && tb[off-5] == 0x25 {
		if isThunk, _, sz := detectPE32Thunk(tb, off-6, textVA, img); isThunk && sz == 6 {
			return true
		}
	}
	return false
}

func isStrongPE32FunctionPrologue(tb []byte, off int, textVA uint32, img *peImageInfo) bool {
	if off < 0 || off+2 > len(tb) {
		return false
	}
	if isThunk, _, _ := detectPE32Thunk(tb, off, textVA, img); isThunk {
		return true
	}
	b0, b1 := tb[off], tb[off+1]
	if b0 == 0x55 && off+3 <= len(tb) && ((b1 == 0x8B && tb[off+2] == 0xEC) || (b1 == 0x89 && tb[off+2] == 0xE5)) {
		return true
	}
	if b0 == 0x8B && b1 == 0xFF && off+3 < len(tb) && tb[off+2] == 0x55 {
		return true
	}
	if (b0 == 0x83 || b0 == 0x81) && b1 == 0xEC {
		return true
	}
	if b0 == 0x6A && off+7 <= len(tb) && tb[off+2] == 0x68 {
		return true
	}
	return false
}

func isValidPE32FunctionStart(tb []byte, off int, img *peImageInfo) bool {
	if off < 0 || off+2 > len(tb) {
		return false
	}
	textVA := img.textSec.va
	textRVA := img.textSec.rva
	if isStrongPE32FunctionPrologue(tb, off, textVA, img) {
		return true
	}
	b0, b1 := tb[off], tb[off+1]
	// 51 / 53 / 56 / 57 (push ecx/ebx/esi/edi) followed by valid non-privileged instruction
	if b0 == 0x51 || b0 == 0x53 || b0 == 0x56 || b0 == 0x57 {
		inst, err := x86asm.Decode(tb[off:], 32)
		if err == nil && inst.Len > 0 && off+inst.Len < len(tb) {
			inst2, err2 := x86asm.Decode(tb[off+inst.Len:], 32)
			if err2 == nil && inst2.Len > 0 && !isPrivilegedOrInvalidOp(inst2.Op) {
				return true
			}
		}
	}
	// Short getter/method starting with mov eax, [ecx+disp] (8B 41 xx / 8B 81 ... / 8B 01) or xor eax, eax (33 C0)
	if b0 == 0x8B && (b1 == 0x41 || b1 == 0x81 || b1 == 0x01 || b1 == 0x44) {
		return true
	}
	if b0 == 0x33 && b1 == 0xC0 {
		return true
	}
	if isPrecededByFunctionBoundary(tb, off, textVA, img) {
		// 0x68 <imm32> (PUSH imm32) where imm32 is relocated and followed within 20 bytes by a valid CALL rel32 (0xE8)
		if b0 == 0x68 && off+10 <= len(tb) && img.hasRelocAtRVA(textRVA+uint32(off+1)) {
			p := off + 5
			for p+5 <= len(tb) && p-off <= 20 {
				if tb[p] == 0xE8 &&
					!img.hasRelocByteAtRVA(textRVA+uint32(p+1)) &&
					!img.hasRelocByteAtRVA(textRVA+uint32(p+4)) {
					rel := int32(binary.LittleEndian.Uint32(tb[p+1 : p+5]))
					callee := uint32(int64(textVA) + int64(p+5) + int64(rel))
					if img.inText(callee) {
						return true
					}
				}
				inst, err := x86asm.Decode(tb[p:], 32)
				if err != nil || inst.Len <= 0 || isPrivilegedOrInvalidOp(inst.Op) || inst.Op == x86asm.RET || inst.Op == x86asm.JMP {
					break
				}
				p += inst.Len
			}
		}
		// 0xB9 <imm32> (MOV ECX, imm32) where imm32 is relocated and the next instruction is 0xE9 (JMP rel32) or 0xE8 (CALL rel32)
		if b0 == 0xB9 && off+10 <= len(tb) && img.hasRelocAtRVA(textRVA+uint32(off+1)) {
			nextOp := tb[off+5]
			if (nextOp == 0xE9 || nextOp == 0xE8) &&
				!img.hasRelocByteAtRVA(textRVA+uint32(off+6)) &&
				!img.hasRelocByteAtRVA(textRVA+uint32(off+9)) {
				rel := int32(binary.LittleEndian.Uint32(tb[off+6 : off+10]))
				target := uint32(int64(textVA) + int64(off+10) + int64(rel))
				if img.inText(target) {
					return true
				}
			}
		}
	}
	return false
}

func isPE32ChkstkPrefix(tb []byte, off int) bool {
	if off < 0 || off+4 > len(tb) {
		return false
	}
	// 51 8D 4C 24: push ecx; lea ecx, [esp+...]
	if tb[off] == 0x51 && tb[off+1] == 0x8D && tb[off+2] == 0x4C && tb[off+3] == 0x24 {
		return true
	}
	// 51 3D 00 10 00 00: push ecx; cmp eax, 0x1000
	if off+6 <= len(tb) && tb[off] == 0x51 && tb[off+1] == 0x3D &&
		tb[off+2] == 0x00 && tb[off+3] == 0x10 && tb[off+4] == 0x00 && tb[off+5] == 0x00 {
		return true
	}
	// 3D 00 10 00 00: cmp eax, 0x1000
	if off+5 <= len(tb) && tb[off] == 0x3D &&
		tb[off+1] == 0x00 && tb[off+2] == 0x10 && tb[off+3] == 0x00 && tb[off+4] == 0x00 {
		return true
	}
	return false
}

func instHasFSSegment(inst x86asm.Inst, raw []byte) bool {
	if len(raw) > 0 && raw[0] == 0x64 {
		return true
	}
	for _, p := range inst.Prefix {
		if p == 0 {
			break
		}
		if (p&0xFF) == 0x64 || p == x86asm.PrefixFS {
			return true
		}
	}
	for _, arg := range inst.Args {
		if mem, ok := arg.(x86asm.Mem); ok && mem.Segment == x86asm.FS {
			return true
		}
	}
	return false
}

func isPE32PrologueHelperCall(tb []byte, textVA uint32, calleeVA uint32, helperCache map[uint32]bool) bool {
	if helperCache != nil {
		if v, ok := helperCache[calleeVA]; ok {
			return v
		}
	}
	res := computePE32PrologueHelper(tb, textVA, calleeVA)
	if helperCache != nil {
		helperCache[calleeVA] = res
	}
	return res
}

func computePE32PrologueHelper(tb []byte, textVA uint32, calleeVA uint32) bool {
	if calleeVA < textVA {
		return false
	}
	startOff := int(calleeVA - textVA)
	if startOff < 0 || startOff >= len(tb) {
		return false
	}
	if isPE32ChkstkPrefix(tb, startOff) {
		return true
	}
	if startOff+3 <= len(tb) {
		b0, b1, b2 := tb[startOff], tb[startOff+1], tb[startOff+2]
		// Exclude standard frame pointer prologue (55 8B EC / 55 89 E5 / 8B FF 55)
		// and standard inlined SEH frame setup (6A FF 68 <handler>).
		if b0 == 0x55 && ((b1 == 0x8B && b2 == 0xEC) || (b1 == 0x89 && b2 == 0xE5)) {
			return false
		}
		if b0 == 0x8B && b1 == 0xFF && b2 == 0x55 {
			return false
		}
		if b0 == 0x6A && b1 == 0xFF && b2 == 0x68 {
			return false
		}
	}

	off := startOff
	maxOff := startOff + 96
	if maxOff > len(tb) {
		maxOff = len(tb)
	}
	touchesFS := false
	for step := 0; step < 22 && off < maxOff; step++ {
		if off+3 <= len(tb) && tb[off] == 0x6A && tb[off+1] == 0xFF && tb[off+2] == 0x68 {
			return false
		}
		inst, err := x86asm.Decode(tb[off:], 32)
		if err != nil || inst.Len <= 0 || off+inst.Len > maxOff {
			break
		}
		if instHasFSSegment(inst, tb[off:off+inst.Len]) {
			touchesFS = true
		}
		if inst.Op == x86asm.CALL {
			return false
		}
		if inst.Op == x86asm.RET || inst.Op == x86asm.JMP {
			return touchesFS
		}
		if _, writesECX := classifyECXAccess(inst); writesECX {
			return false
		}
		off += inst.Len
	}
	return false
}

func isPE32CRTStackOrEHHelper(tb []byte, startOff int, fnSize uint32, textVA uint32, va uint32, helperCache map[uint32]bool) bool {
	if fnSize > 160 || startOff < 0 || startOff >= len(tb) {
		return false
	}
	if isPE32ChkstkPrefix(tb, startOff) {
		return true
	}
	if isPE32PrologueHelperCall(tb, textVA, va, helperCache) {
		return true
	}
	// Check MSVC __EH_epilog* / __SEH_epilog* (e.g. mov ecx, [ebp-0xc]; mov fs:[0], ecx; pop ecx; ...; ret):
	if startOff+3 <= len(tb) {
		b0, b1, b2 := tb[startOff], tb[startOff+1], tb[startOff+2]
		if b0 == 0x55 && ((b1 == 0x8B && b2 == 0xEC) || (b1 == 0x89 && b2 == 0xE5)) {
			return false
		}
		if b0 == 0x6A && b1 == 0xFF && b2 == 0x68 {
			return false
		}
	}
	off := startOff
	limit := startOff + int(fnSize)
	if limit > len(tb) {
		limit = len(tb)
	}
	for step := 0; step < 4 && off+7 <= limit; step++ {
		if tb[off] == 0x64 && tb[off+1] == 0x89 && tb[off+2] == 0x0D &&
			tb[off+3] == 0x00 && tb[off+4] == 0x00 && tb[off+5] == 0x00 && tb[off+6] == 0x00 {
			return true
		}
		inst, err := x86asm.Decode(tb[off:], 32)
		if err != nil || inst.Len <= 0 {
			break
		}
		off += inst.Len
	}
	return false
}

// ClassifyPE32FunctionAt classifies a single PE32 .text function at va (useful for Silver Ghidra-only seeds).
func ClassifyPE32FunctionAt(tb []byte, textVA, textRVA uint32, img *peImageInfo, va uint32) FunctionSeed {
	seed := FunctionSeed{
		EA:          uint64(va),
		Name:        formatSubName(uint64(va)),
		CallingConv: "__cdecl",
		CCSource:    extractdb.CCSourceX86Asm,
		VtableSlot:  -1,
	}
	if va < textVA || int(va-textVA) >= len(tb) {
		return seed
	}
	startOff := int(va - textVA)
	maxEndOff := startOff + 32768
	if maxEndOff > len(tb) {
		maxEndOff = len(tb)
	}
	if img != nil {
		if isThunk, thunkTarget, _ := detectPE32Thunk(tb, startOff, textVA, img); isThunk {
			seed.Flags |= extractdb.FuncFlagThunk
			seed.ThunkTargetEA = thunkTarget
			return seed
		}
		res := analyzePE32Body(tb, startOff, maxEndOff, va, img, nil, nil)
		seed.StackPurge = res.stackPurge
		if res.readsECX {
			seed.CallingConv = "__thiscall"
		} else if res.stackPurge > 0 {
			seed.CallingConv = "__stdcall"
		}
		if isPE32CRTStackOrEHHelper(tb, startOff, res.size, textVA, va, nil) {
			seed.Flags |= extractdb.FuncFlagNoSeedOverride
		}
	}
	return seed
}

func isPrivilegedOrInvalidOp(op x86asm.Op) bool {
	switch op {
	case x86asm.IN, x86asm.OUT, x86asm.HLT, x86asm.IRET,
		x86asm.CLI, x86asm.STI, x86asm.LIDT, x86asm.LGDT:
		return true
	default:
		return false
	}
}

func detectPE32Thunk(tb []byte, off int, textVA uint32, img *peImageInfo) (bool, uint64, uint32) {
	if off < 0 || off+5 > len(tb) {
		return false, 0, 0
	}
	// FF 25 <disp32> (jmp dword ptr [iat_va])
	if off+6 <= len(tb) && tb[off] == 0xFF && tb[off+1] == 0x25 {
		target := binary.LittleEndian.Uint32(tb[off+2 : off+6])
		return true, uint64(target), 6
	}
	// E9 <rel32> (jmp target_ea)
	if tb[off] == 0xE9 && !img.hasRelocByteAtRVA(img.textSec.rva+uint32(off+1)) && !img.hasRelocByteAtRVA(img.textSec.rva+uint32(off+4)) {
		rel := int32(binary.LittleEndian.Uint32(tb[off+1 : off+5]))
		target := uint32(int64(textVA) + int64(off+5) + int64(rel))
		if img.inText(target) {
			return true, uint64(target), 5
		}
	}
	return false, 0, 0
}

type peBodyAnalysis struct {
	size             uint32
	readsECX         bool
	stackPurge       int
	hasConsistentRet bool
	callSites        []callSiteRow
	dataXrefs        []dataXrefRow
	stringXrefs      []stringXrefRow
}

func analyzePE32Body(
	tb []byte,
	startOff int,
	maxEndOff int,
	fnVA uint32,
	img *peImageInfo,
	strIdx *stringIndex,
	helperCache map[uint32]bool,
) peBodyAnalysis {
	textVA := img.textSec.va
	textRVA := img.textSec.rva
	rdataStart, rdataEnd := uint32(0), uint32(0)
	if img.rdataSec != nil {
		rdataStart = img.rdataSec.va
		rdataEnd = rdataStart + uint32(len(img.rdataSec.data))
	}
	dataStart, dataEnd := uint32(0), uint32(0)
	if img.dataSec != nil {
		dataStart = img.dataSec.va
		dataEnd = dataStart + uint32(len(img.dataSec.data))
	}
	inDataSec := func(va uint32) bool {
		return (va >= rdataStart && va < rdataEnd) || (va >= dataStart && va < dataEnd)
	}

	var (
		res         peBodyAnalysis
		ecxOverw    bool
		ecxRead     bool
		seenRet     bool
		retPurge    int
		retConflict bool
		endOff      = startOff
		instCount   = 0
	)

	// Walk sequentially within [startOff, maxEndOff) up to 16384 instructions for ABI + xref extraction.
	off := startOff
	furthestBranchOff := startOff
	for off < maxEndOff && instCount < 16384 {
		if off > furthestBranchOff && tb[off] == 0xCC {
			break
		}
		inst, err := x86asm.Decode(tb[off:], 32)
		if err != nil || inst.Len <= 0 || off+inst.Len > maxEndOff {
			break
		}
		instVA := textVA + uint32(off)
		instCount++
		endOff = off + inst.Len

		// Check ECX usage before ECX is overwritten or clobbered by a CALL.
		// Explicitly ignore standalone `push ecx` (0x51) in the prologue (compiler local-slot allocation).
		if !ecxOverw && !ecxRead {
			isProloguePushECX := inst.Op == x86asm.PUSH && inst.Len == 1 && tb[off] == 0x51 && instCount <= 6
			if !isProloguePushECX {
				r, w := classifyECXAccess(inst)
				if r {
					ecxRead = true
				}
				if w {
					ecxOverw = true
				}
			}
		}

		// Record call sites & tail jumps.
		switch inst.Op {
		case x86asm.CALL:
			isPrologueHelper := false
			if instCount <= 10 && tb[off] == 0xE8 && inst.Len == 5 &&
				!img.hasRelocByteAtRVA(textRVA+uint32(off+1)) &&
				!img.hasRelocByteAtRVA(textRVA+uint32(off+4)) {
				rel := int32(binary.LittleEndian.Uint32(tb[off+1 : off+5]))
				calleeVA := uint32(int64(instVA) + 5 + int64(rel))
				isPrologueHelper = isPE32PrologueHelperCall(tb, textVA, calleeVA, helperCache)
			}
			if !isPrologueHelper {
				ecxOverw = true
			}
			cs := extractPE32CallSite(inst, instVA, fnVA, tb, off, img)
			res.callSites = append(res.callSites, cs)
		case x86asm.JMP:
			if tb[off] == 0xE9 && inst.Len == 5 && !img.hasRelocByteAtRVA(textRVA+uint32(off+1)) && !img.hasRelocByteAtRVA(textRVA+uint32(off+4)) {
				rel := int32(binary.LittleEndian.Uint32(tb[off+1 : off+5]))
				target := uint32(int64(instVA) + 5 + int64(rel))
				isMovECXTailJmp := off == startOff+5 && tb[startOff] == 0xB9 && img.hasRelocAtRVA(textRVA+uint32(startOff+1))
				if isMovECXTailJmp || target < fnVA || target >= textVA+uint32(maxEndOff) {
					res.callSites = append(res.callSites, callSiteRow{
						CallEA:   uint64(instVA),
						CallerEA: uint64(fnVA),
						CalleeEA: nullInt64(int64(target), true),
						CallKind: extractdb.CallKindTailJump,
					})
				} else if tOff := int(target - textVA); tOff > furthestBranchOff {
					furthestBranchOff = tOff
				}
			} else if tb[off] == 0xEB && inst.Len == 2 {
				rel := int8(tb[off+1])
				target := uint32(int64(instVA) + 2 + int64(rel))
				if tOff := int(target - textVA); tOff > furthestBranchOff && tOff < maxEndOff {
					furthestBranchOff = tOff
				}
			}
		case x86asm.RET:
			p := 0
			if a0, ok := inst.Args[0].(x86asm.Imm); ok {
				p = int(a0)
			}
			if p >= 0 && p <= 256 && p%4 == 0 {
				if !seenRet {
					seenRet = true
					retPurge = p
				} else if retPurge != p {
					retConflict = true
				}
			} else {
				retConflict = true
			}
		default:
			// Track forward short/near conditional branches so we don't stop at an early return.
			for _, arg := range inst.Args {
				if rel, ok := arg.(x86asm.Rel); ok {
					target := uint32(int64(instVA) + int64(inst.Len) + int64(rel))
					if tOff := int(target - textVA); tOff > furthestBranchOff && tOff < maxEndOff {
						furthestBranchOff = tOff
					}
				}
			}
		}

		// Check relocated immediates inside instruction for string_xrefs and data_xrefs.
		for k := 1; k+4 <= inst.Len; k++ {
			if !img.hasRelocAtRVA(textRVA + uint32(off+k)) {
				continue
			}
			targetVA := binary.LittleEndian.Uint32(tb[off+k : off+k+4])
			if !inDataSec(targetVA) {
				continue
			}
			if strID, ok := strIdx.LookupEA(uint64(targetVA)); ok {
				res.stringXrefs = append(res.stringXrefs, stringXrefRow{
					StrID:  strID,
					FuncEA: uint64(fnVA),
				})
			}
			kind := classifyDataAccessKind(inst, targetVA)
			res.dataXrefs = append(res.dataXrefs, dataXrefRow{
				FromEA:     uint64(instVA),
				FuncEA:     uint64(fnVA),
				ToEA:       uint64(targetVA),
				AccessKind: kind,
			})
		}

		off += inst.Len
		if (inst.Op == x86asm.RET || inst.Op == x86asm.JMP) && off > furthestBranchOff {
			break
		}
	}

	if endOff > startOff {
		res.size = uint32(endOff - startOff)
	} else {
		res.size = uint32(maxEndOff - startOff)
	}
	res.readsECX = ecxRead
	if seenRet && !retConflict {
		res.hasConsistentRet = true
		res.stackPurge = retPurge
	}
	return res
}

func classifyECXAccess(inst x86asm.Inst) (readsECX, writesECX bool) {
	if inst.Op == x86asm.CPUID || inst.Op == x86asm.RDTSC {
		return false, true
	}
	// Zeroing idiom `xor ecx, ecx` / `sub ecx, ecx` / `sbb ecx, ecx` writes ECX without reading prior value.
	if inst.Op == x86asm.XOR || inst.Op == x86asm.SUB || inst.Op == x86asm.SBB {
		if r0, ok0 := inst.Args[0].(x86asm.Reg); ok0 && isECXFamily(r0) {
			if r1, ok1 := inst.Args[1].(x86asm.Reg); ok1 && r0 == r1 {
				return false, true
			}
		}
	}

	// Check memory operands first: any `[ecx + ...]` reads ECX even if ECX is destination (`mov ecx, [ecx+4]`).
	for _, arg := range inst.Args {
		if mem, ok := arg.(x86asm.Mem); ok {
			if isECXFamily(mem.Base) || isECXFamily(mem.Index) {
				readsECX = true
			}
		}
	}
	if readsECX {
		return true, false
	}

	// Check source operands (Args[1], Args[2]).
	for idx := 1; idx < len(inst.Args); idx++ {
		if reg, ok := inst.Args[idx].(x86asm.Reg); ok && isECXFamily(reg) {
			return true, false
		}
	}

	// Check destination operand (Args[0]).
	if reg, ok := inst.Args[0].(x86asm.Reg); ok && isECXFamily(reg) {
		switch inst.Op {
		case x86asm.MOV, x86asm.LEA, x86asm.POP, x86asm.MOVZX, x86asm.MOVSX,
			x86asm.SETE, x86asm.SETNE, x86asm.SETB, x86asm.SETAE, x86asm.SETBE, x86asm.SETA,
			x86asm.SETL, x86asm.SETGE, x86asm.SETLE, x86asm.SETG, x86asm.SETS, x86asm.SETNS,
			x86asm.SETO, x86asm.SETNO, x86asm.SETP, x86asm.SETNP,
			x86asm.BSF, x86asm.BSR:
			return false, true
		case x86asm.TEST, x86asm.CMP, x86asm.ADD, x86asm.ADC, x86asm.AND, x86asm.OR,
			x86asm.INC, x86asm.DEC, x86asm.NEG, x86asm.NOT, x86asm.SHL, x86asm.SHR, x86asm.SAR, x86asm.IMUL:
			return true, true
		}
	}
	return false, false
}

func isECXFamily(r x86asm.Reg) bool {
	return r == x86asm.ECX || r == x86asm.CX || r == x86asm.CL || r == x86asm.CH
}

func extractPE32CallSite(inst x86asm.Inst, instVA, fnVA uint32, tb []byte, off int, img *peImageInfo) callSiteRow {
	if tb[off] == 0xE8 && inst.Len == 5 {
		rel := int32(binary.LittleEndian.Uint32(tb[off+1 : off+5]))
		callee := uint32(int64(instVA) + 5 + int64(rel))
		return callSiteRow{
			CallEA:   uint64(instVA),
			CallerEA: uint64(fnVA),
			CalleeEA: nullInt64(int64(callee), true),
			CallKind: extractdb.CallKindDirect,
		}
	}
	if mem, ok := inst.Args[0].(x86asm.Mem); ok {
		baseReg := ""
		if mem.Base != 0 {
			baseReg = mem.Base.String()
		}
		var callee sql.NullInt64
		memDisp := mem.Disp
		if mem.Base == 0 && mem.Index == 0 && mem.Disp != 0 {
			uDisp := int64(uint64(uint32(mem.Disp)))
			callee = nullInt64(uDisp, true)
			memDisp = uDisp
		}
		return callSiteRow{
			CallEA:   uint64(instVA),
			CallerEA: uint64(fnVA),
			CalleeEA: callee,
			CallKind: extractdb.CallKindIndirectMem,
			BaseReg:  baseReg,
			MemDisp:  nullInt64(memDisp, mem.Disp != 0 || mem.Base != 0),
		}
	}
	if reg, ok := inst.Args[0].(x86asm.Reg); ok {
		return callSiteRow{
			CallEA:   uint64(instVA),
			CallerEA: uint64(fnVA),
			CallKind: extractdb.CallKindIndirectReg,
			BaseReg:  reg.String(),
		}
	}
	return callSiteRow{
		CallEA:   uint64(instVA),
		CallerEA: uint64(fnVA),
		CallKind: extractdb.CallKindIndirectReg,
	}
}

func isReadOnlyMemArg0Op(op x86asm.Op) bool {
	switch op {
	case x86asm.CMP, x86asm.TEST, x86asm.CALL, x86asm.JMP, x86asm.PUSH, x86asm.BT,
		x86asm.MUL, x86asm.IMUL, x86asm.DIV, x86asm.IDIV,
		x86asm.FLD, x86asm.FILD, x86asm.FBLD, x86asm.FLDCW, x86asm.FLDENV, x86asm.FRSTOR,
		x86asm.LDMXCSR, x86asm.FXRSTOR,
		x86asm.FADD, x86asm.FMUL, x86asm.FSUB, x86asm.FSUBR, x86asm.FDIV, x86asm.FDIVR,
		x86asm.FCOM, x86asm.FCOMP,
		x86asm.FIADD, x86asm.FIMUL, x86asm.FISUB, x86asm.FISUBR, x86asm.FIDIV, x86asm.FIDIVR,
		x86asm.FICOM, x86asm.FICOMP,
		x86asm.FTST, x86asm.FUCOM, x86asm.FUCOMP:
		return true
	default:
		return false
	}
}

func classifyDataAccessKind(inst x86asm.Inst, targetVA uint32) string {
	if inst.Op == x86asm.LEA {
		return extractdb.AccessKindAddr
	}
	if mem, ok := inst.Args[0].(x86asm.Mem); ok && uint32(mem.Disp) == targetVA {
		if isReadOnlyMemArg0Op(inst.Op) {
			return extractdb.AccessKindRead
		}
		if mem.Base != 0 && mem.Index != 0 {
			return extractdb.AccessKindAddr
		}
		return extractdb.AccessKindWrite
	}
	for idx := 1; idx < len(inst.Args); idx++ {
		if mem, ok := inst.Args[idx].(x86asm.Mem); ok && uint32(mem.Disp) == targetVA {
			return extractdb.AccessKindRead
		}
	}
	return extractdb.AccessKindAddr
}

func insertCallSites(ctx context.Context, db *sql.DB, rows []callSiteRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT OR IGNORE INTO call_sites(call_ea, caller_ea, callee_ea, call_kind, base_reg, mem_disp)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(int64(r.CallEA), int64(r.CallerEA), r.CalleeEA, r.CallKind, r.BaseReg, r.MemDisp); err != nil {
			return fmt.Errorf("insert call_site 0x%x: %w", r.CallEA, err)
		}
	}
	return b.Flush()
}

func insertDataXrefs(ctx context.Context, db *sql.DB, rows []dataXrefRow) error {
	b := newTxStmtBatcher(ctx, db, `
		INSERT OR IGNORE INTO data_xrefs(from_ea, func_ea, to_ea, access_kind)
		VALUES (?, ?, ?, ?)
	`)
	defer func() { _ = b.Close() }()
	for _, r := range rows {
		if err := b.Exec(int64(r.FromEA), int64(r.FuncEA), int64(r.ToEA), r.AccessKind); err != nil {
			return fmt.Errorf("insert data_xref 0x%x->0x%x: %w", r.FromEA, r.ToEA, err)
		}
	}
	return b.Flush()
}
