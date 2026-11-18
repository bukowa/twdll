package bronze

import (
	"bytes"
	"context"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"twdll/cli/internal/extractdb"
)

// PE32Extractor extracts raw PE32 structural facts (sections, .reloc, IAT imports, exports)
// without inspecting instructions or guessing function boundaries.
type PE32Extractor struct{}

func (PE32Extractor) ID() string { return "pe32.format" }

func (PE32Extractor) AppliesTo(src *Source) bool {
	return src != nil && src.PE != nil
}

type pe32Section struct {
	name string
	rva  uint32
	va   uint64
	data []byte
}

type pe32Prepared struct {
	oh         *pe.OptionalHeader32
	secs       []pe32Section
	secRows    []extractdb.BronzeSectionRow
	relocRVAs  []uint32
	relocWords []uint64
	maxRVA     uint64
}

func (p *pe32Prepared) readSliceAtRVA(rva uint64, size uint32) ([]byte, bool) {
	size64 := uint64(size)
	for _, s := range p.secs {
		sStart := uint64(s.rva)
		sEnd := sStart + uint64(len(s.data))
		if rva >= sStart && rva < sEnd {
			off := rva - sStart
			if off+size64 > uint64(len(s.data)) {
				return nil, false
			}
			return s.data[off : off+size64], true
		}
	}
	return nil, false
}

func (p *pe32Prepared) readCStringAtRVA(rva uint64, maxLen int) (string, bool) {
	for _, s := range p.secs {
		sStart := uint64(s.rva)
		sEnd := sStart + uint64(len(s.data))
		if rva >= sStart && rva < sEnd {
			off := int(rva - sStart)
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

func (p *pe32Prepared) hasRelocByte(rva uint64) bool {
	idx := rva >> 6
	if idx >= uint64(len(p.relocWords)) {
		return false
	}
	return (p.relocWords[idx] & (uint64(1) << (rva & 63))) != 0
}

func preparePE32(src *Source) error {
	if src == nil || src.PE == nil {
		return fmt.Errorf("missing PE handle")
	}
	oh, ok := src.PE.OptionalHeader.(*pe.OptionalHeader32)
	if !ok {
		return fmt.Errorf("%s is not a 32-bit PE binary", src.BinaryPath)
	}

	var maxRVA uint64
	secs := make([]pe32Section, 0, len(src.PE.Sections))
	secRows := make([]extractdb.BronzeSectionRow, 0, len(src.PE.Sections))

	for idx, sec := range src.PE.Sections {
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
		if vsize > 0 && uint64(vsize) < uint64(len(data)) {
			data = data[:vsize]
		}
		if endRVA := uint64(sec.VirtualAddress) + uint64(vsize); endRVA > maxRVA {
			maxRVA = endRVA
		}

		va := uint64(oh.ImageBase) + uint64(sec.VirtualAddress)
		secs = append(secs, pe32Section{
			name: name,
			rva:  sec.VirtualAddress,
			va:   va,
			data: data,
		})
		secRows = append(secRows, extractdb.BronzeSectionRow{
			SecIdx:      idx,
			Name:        name,
			VA:          va,
			VirtualSize: uint64(vsize),
			RawOffset:   uint64(sec.Offset),
			RawSize:     uint64(sec.Size),
			Flags:       flags,
		})
	}

	prep := &pe32Prepared{
		oh:         oh,
		secs:       secs,
		secRows:    secRows,
		relocWords: make([]uint64, int((maxRVA+64)>>6)),
		maxRVA:     maxRVA,
	}

	markRelocRVA := func(rva uint32) {
		if uint64(rva)+4 > prep.maxRVA {
			return
		}
		for k := uint64(0); k < 4; k++ {
			b := uint64(rva) + k
			prep.relocWords[b>>6] |= uint64(1) << (b & 63)
		}
	}
	prep.relocRVAs = collectPE32Relocs(oh, prep.readSliceAtRVA, markRelocRVA)

	src.ImageBase = uint64(oh.ImageBase)
	src.Arch = extractdb.ArchX86
	src.Format = extractdb.FormatPE32
	if len(src.DataSlices) == 0 {
		for _, s := range secs {
			if (s.name == ".rdata" || s.name == ".data" || s.name == ".rodata" || s.name == "_RDATA") && len(s.data) > 0 {
				secRVA := s.rva
				src.DataSlices = append(src.DataSlices, DataSlice{
					Name:    s.name,
					VA:      s.va,
					Data:    s.data,
					Is64Bit: false,
					HasRelocAt: func(off int) bool {
						if off < 0 {
							return false
						}
						return prep.hasRelocByte(uint64(secRVA) + uint64(off))
					},
				})
			}
		}
	}
	src.pe32 = prep
	return nil
}

func (PE32Extractor) Extract(ctx context.Context, src *Source, store *extractdb.BronzeStore) error {
	if src.pe32 == nil {
		if err := preparePE32(src); err != nil {
			return err
		}
	}
	prep := src.pe32

	if err := store.InsertSections(ctx, prep.secRows); err != nil {
		return err
	}
	if err := insertPE32Relocs(ctx, store, prep.oh.ImageBase, prep.relocRVAs); err != nil {
		return err
	}

	imports, err := parsePE32Imports(prep.oh, prep.readSliceAtRVA, prep.readCStringAtRVA)
	if err != nil {
		return err
	}
	if err := store.InsertImports(ctx, imports); err != nil {
		return err
	}

	exports := parsePE32Exports(prep.oh, prep.readSliceAtRVA, prep.readCStringAtRVA)
	return store.InsertExports(ctx, exports)
}

func collectPE32Relocs(
	oh *pe.OptionalHeader32,
	readSliceAtRVA func(rva uint64, size uint32) ([]byte, bool),
	markRelocRVA func(rva uint32),
) []uint32 {
	if len(oh.DataDirectory) <= pe.IMAGE_DIRECTORY_ENTRY_BASERELOC {
		return nil
	}
	dd := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_BASERELOC]
	if dd.VirtualAddress == 0 || dd.Size == 0 {
		return nil
	}
	relocData, ok := readSliceAtRVA(uint64(dd.VirtualAddress), dd.Size)
	if !ok {
		return nil
	}

	var rvas []uint32
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
			if entry>>12 == 3 { // IMAGE_REL_BASED_HIGHLOW
				rva64 := uint64(pageRVA) + uint64(entry&0x0FFF)
				if rva64 > 0xFFFFFFFF {
					continue
				}
				rva := uint32(rva64)
				markRelocRVA(rva)
				rvas = append(rvas, rva)
			}
		}
		pos += int(blockSize)
	}
	return rvas
}

func insertPE32Relocs(ctx context.Context, store *extractdb.BronzeStore, imageBase uint32, relocRVAs []uint32) error {
	if len(relocRVAs) == 0 {
		return nil
	}
	vas := make([]uint64, len(relocRVAs))
	base := uint64(imageBase)
	for i, rva := range relocRVAs {
		vas[i] = base + uint64(rva)
	}
	return store.InsertRelocs(ctx, vas)
}

func parsePE32Imports(
	oh *pe.OptionalHeader32,
	readSliceAtRVA func(rva uint64, size uint32) ([]byte, bool),
	readCStringAtRVA func(rva uint64, maxLen int) (string, bool),
) ([]extractdb.BronzeImportRow, error) {
	var rows []extractdb.BronzeImportRow
	if len(oh.DataDirectory) <= pe.IMAGE_DIRECTORY_ENTRY_IMPORT {
		return rows, nil
	}
	dd := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT]
	if dd.VirtualAddress == 0 || dd.Size == 0 {
		return rows, nil
	}

	for descOff := uint64(0); ; descOff += 20 {
		descBytes, ok := readSliceAtRVA(uint64(dd.VirtualAddress)+descOff, 20)
		if !ok {
			break
		}
		origFirstThunk := binary.LittleEndian.Uint32(descBytes[0:4])
		nameRVA := binary.LittleEndian.Uint32(descBytes[12:16])
		firstThunk := binary.LittleEndian.Uint32(descBytes[16:20])
		if origFirstThunk == 0 && nameRVA == 0 && firstThunk == 0 {
			break
		}
		dllName, ok := readCStringAtRVA(uint64(nameRVA), 256)
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

		for idx := uint64(0); idx < 65536; idx++ {
			thunkBytes, ok := readSliceAtRVA(uint64(lookupRVA)+idx*4, 4)
			if !ok {
				break
			}
			thunkVal := binary.LittleEndian.Uint32(thunkBytes)
			if thunkVal == 0 {
				break
			}
			iatVA := uint64(oh.ImageBase) + uint64(firstThunk) + idx*4
			var symName string
			var ordinal int
			if thunkVal&0x80000000 != 0 {
				ordinal = int(thunkVal & 0xFFFF)
				symName = fmt.Sprintf("ord_%d", ordinal)
			} else {
				if hintBytes, ok := readSliceAtRVA(uint64(thunkVal), 2); ok {
					ordinal = int(binary.LittleEndian.Uint16(hintBytes))
				}
				symName, _ = readCStringAtRVA(uint64(thunkVal)+2, 512)
				if symName == "" {
					symName = fmt.Sprintf("ord_%d", ordinal)
				}
			}
			rows = append(rows, extractdb.BronzeImportRow{
				IATVA:   iatVA,
				DLLName: dllName,
				SymName: symName,
				Ordinal: ordinal,
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].IATVA < rows[j].IATVA })
	return rows, nil
}

func parsePE32Exports(
	oh *pe.OptionalHeader32,
	readSliceAtRVA func(rva uint64, size uint32) ([]byte, bool),
	readCStringAtRVA func(rva uint64, maxLen int) (string, bool),
) []extractdb.BronzeExportRow {
	var rows []extractdb.BronzeExportRow
	if len(oh.DataDirectory) <= pe.IMAGE_DIRECTORY_ENTRY_EXPORT {
		return rows
	}
	dd := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_EXPORT]
	if dd.VirtualAddress == 0 || dd.Size < 40 {
		return rows
	}
	dirBytes, ok := readSliceAtRVA(uint64(dd.VirtualAddress), 40)
	if !ok {
		return rows
	}
	dirStart := uint64(dd.VirtualAddress)
	dirEnd := dirStart + uint64(dd.Size)

	base := binary.LittleEndian.Uint32(dirBytes[16:20])
	numFuncs := binary.LittleEndian.Uint32(dirBytes[20:24])
	numNames := binary.LittleEndian.Uint32(dirBytes[24:28])
	addrOfFuncs := binary.LittleEndian.Uint32(dirBytes[28:32])
	addrOfNames := binary.LittleEndian.Uint32(dirBytes[32:36])
	addrOfNameOrds := binary.LittleEndian.Uint32(dirBytes[36:40])

	nameByFuncIdx := make(map[uint32]string, min(numNames, 65536))
	for i := uint32(0); i < numNames && i < 65536; i++ {
		nameRVABytes, ok1 := readSliceAtRVA(uint64(addrOfNames)+uint64(i)*4, 4)
		ordBytes, ok2 := readSliceAtRVA(uint64(addrOfNameOrds)+uint64(i)*2, 2)
		if !ok1 || !ok2 {
			break
		}
		nameRVA := binary.LittleEndian.Uint32(nameRVABytes)
		funcIdx := uint32(binary.LittleEndian.Uint16(ordBytes))
		if name, ok := readCStringAtRVA(uint64(nameRVA), 512); ok && name != "" {
			nameByFuncIdx[funcIdx] = name
		}
	}

	var nextID int64 = 1
	for funcIdx := uint32(0); funcIdx < numFuncs && funcIdx < 65536; funcIdx++ {
		fnBytes, ok := readSliceAtRVA(uint64(addrOfFuncs)+uint64(funcIdx)*4, 4)
		if !ok {
			break
		}
		fnRVA := binary.LittleEndian.Uint32(fnBytes)
		if fnRVA == 0 || (uint64(fnRVA) >= dirStart && uint64(fnRVA) < dirEnd) {
			continue
		}
		ordinal := int(uint64(base) + uint64(funcIdx))
		name := nameByFuncIdx[funcIdx]
		if name == "" {
			name = fmt.Sprintf("ord_%d", ordinal)
		}
		rows = append(rows, extractdb.BronzeExportRow{
			ExportID: nextID,
			EA:       uint64(oh.ImageBase) + uint64(fnRVA),
			Name:     name,
			Ordinal:  ordinal,
		})
		nextID++
	}
	return rows
}
