package bronze

import (
	"context"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"sort"

	"twdll/cli/internal/extractdb"
)

// ELF64Extractor extracts raw ELF64 structural facts (sections, .rela.dyn/.rela.plt,
// dynamic imports/exports, and .symtab/.dynsym symbol tables) without heuristic scanning.
type ELF64Extractor struct{}

func (ELF64Extractor) ID() string { return "elf64.format" }

func (ELF64Extractor) AppliesTo(src *Source) bool {
	return src != nil && src.ELF != nil
}

func prepareELF64(src *Source) error {
	if src == nil || src.ELF == nil {
		return fmt.Errorf("missing ELF handle")
	}
	ef := src.ELF
	if ef.Class != elf.ELFCLASS64 || ef.Machine != elf.EM_X86_64 {
		return fmt.Errorf("%s is not an x86_64 ELF64 binary", src.BinaryPath)
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

	src.ImageBase = imageBase
	src.Arch = extractdb.ArchX86_64
	src.Format = extractdb.FormatELF64

	if len(src.DataSlices) == 0 {
		for _, sec := range ef.Sections {
			if sec.Type == elf.SHT_NULL {
				continue
			}
			if sec.Name == ".rodata" || sec.Name == ".data.rel.ro" || sec.Name == ".data" {
				if data, err := sec.Data(); err == nil && len(data) > 0 {
					src.DataSlices = append(src.DataSlices, DataSlice{
						Name:    sec.Name,
						VA:      sec.Addr,
						Data:    data,
						Is64Bit: true,
					})
				}
			}
		}
		wireELF64DataSliceRelocs(ef, src.DataSlices)
	}
	return nil
}

func wireELF64DataSliceRelocs(ef *elf.File, slices []DataSlice) {
	if ef == nil || len(slices) == 0 {
		return
	}
	bitsets := make([][]uint64, len(slices))
	for _, sec := range ef.Sections {
		if sec.Type != elf.SHT_RELA && sec.Type != elf.SHT_REL {
			continue
		}
		if sec.Flags&elf.SHF_ALLOC == 0 &&
			sec.Name != ".rela.dyn" && sec.Name != ".rela.plt" &&
			sec.Name != ".rel.dyn" && sec.Name != ".rel.plt" {
			continue
		}
		step := 24
		if sec.Type == elf.SHT_REL {
			step = 16
		}
		data, err := sec.Data()
		if err != nil || len(data) < step {
			continue
		}
		for pos := 0; pos+step <= len(data); pos += step {
			rOffset := binary.LittleEndian.Uint64(data[pos : pos+8])
			rInfo := binary.LittleEndian.Uint64(data[pos+8 : pos+16])
			rType := elf.R_X86_64(rInfo & 0xffffffff)
			if rOffset == 0 || rType == elf.R_X86_64_NONE {
				continue
			}
			span := uint64(8)
			switch rType {
			case elf.R_X86_64_32, elf.R_X86_64_32S, elf.R_X86_64_PC32:
				span = 4
			}
			relEnd := rOffset + span
			if relEnd < rOffset {
				continue
			}
			for idx, s := range slices {
				secStart := s.VA
				secEnd := secStart + uint64(len(s.Data))
				if secEnd < secStart || rOffset >= secEnd || relEnd <= secStart {
					continue
				}
				words := bitsets[idx]
				if words == nil {
					words = make([]uint64, (len(s.Data)+63)/64)
					bitsets[idx] = words
				}
				lo := max(rOffset, secStart)
				hi := min(relEnd, secEnd)
				for va := lo; va < hi; va++ {
					off := va - secStart
					words[off>>6] |= uint64(1) << (off & 63)
				}
			}
		}
	}
	for idx, words := range bitsets {
		if words == nil {
			continue
		}
		n := len(slices[idx].Data)
		w := words
		slices[idx].HasRelocAt = func(off int) bool {
			if off < 0 || off >= n {
				return false
			}
			return (w[off>>6] & (uint64(1) << (uint(off) & 63))) != 0
		}
	}
}

func (ELF64Extractor) Extract(ctx context.Context, src *Source, store *extractdb.BronzeStore) error {
	if src.Format == "" {
		if err := prepareELF64(src); err != nil {
			return err
		}
	}
	ef := src.ELF

	secRows := make([]extractdb.BronzeSectionRow, 0, len(ef.Sections))
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
		secRows = append(secRows, extractdb.BronzeSectionRow{
			SecIdx:      idx,
			Name:        sec.Name,
			VA:          sec.Addr,
			VirtualSize: sec.Size,
			RawOffset:   sec.Offset,
			RawSize:     sec.FileSize,
			Flags:       flags,
		})
	}

	if err := store.InsertSections(ctx, secRows); err != nil {
		return err
	}

	dynSyms, _ := ef.DynamicSymbols()
	imports, exports, err := parseELF64RelocsImportsExports(ctx, store, ef, dynSyms)
	if err != nil {
		return err
	}
	if err := store.InsertImports(ctx, imports); err != nil {
		return err
	}
	if err := store.InsertExports(ctx, exports); err != nil {
		return err
	}

	symtabSyms, _ := ef.Symbols()
	elfSyms := collectRawELFSymbols(symtabSyms, dynSyms)
	return store.InsertELFSymbols(ctx, elfSyms)
}

func parseELF64RelocsImportsExports(
	ctx context.Context,
	store *extractdb.BronzeStore,
	ef *elf.File,
	dynSyms []elf.Symbol,
) ([]extractdb.BronzeImportRow, []extractdb.BronzeExportRow, error) {
	const relocChunkSize = 16384
	relocVAs := make([]uint64, 0, relocChunkSize)

	var imports []extractdb.BronzeImportRow
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
			relocVAs = append(relocVAs, rOffset)
			if len(relocVAs) >= relocChunkSize {
				if err := store.InsertRelocs(ctx, relocVAs); err != nil {
					return nil, nil, err
				}
				relocVAs = relocVAs[:0]
			}

			symIdx := uint32(rInfo >> 32)
			if symIdx > 0 && int(symIdx-1) < len(dynSyms) {
				sym := dynSyms[symIdx-1]
				if sym.Name != "" && sym.Section == elf.SHN_UNDEF && !seenIAT[rOffset] {
					seenIAT[rOffset] = true
					imports = append(imports, extractdb.BronzeImportRow{
						IATVA:   rOffset,
						DLLName: sym.Library,
						SymName: sym.Name,
						Ordinal: int(symIdx),
					})
				}
			}
		}
	}
	if len(relocVAs) > 0 {
		if err := store.InsertRelocs(ctx, relocVAs); err != nil {
			return nil, nil, err
		}
	}

	sort.Slice(imports, func(i, j int) bool { return imports[i].IATVA < imports[j].IATVA })

	var exports []extractdb.BronzeExportRow
	var nextExpID int64 = 1
	for idx, sym := range dynSyms {
		if sym.Section == elf.SHN_UNDEF || sym.Value == 0 || sym.Name == "" {
			continue
		}
		st := elf.ST_TYPE(sym.Info)
		if st != elf.STT_FUNC && st != elf.STT_OBJECT {
			continue
		}
		exports = append(exports, extractdb.BronzeExportRow{
			ExportID: nextExpID,
			EA:       sym.Value,
			Name:     sym.Name,
			Ordinal:  idx + 1,
		})
		nextExpID++
	}
	return imports, exports, nil
}

func collectRawELFSymbols(symtabSyms, dynSyms []elf.Symbol) []extractdb.BronzeELFSymbolRow {
	var out []extractdb.BronzeELFSymbolRow
	idx := 1
	appendGroup := func(kind string, syms []elf.Symbol) {
		for _, sym := range syms {
			if sym.Name == "" && sym.Value == 0 {
				continue
			}
			out = append(out, extractdb.BronzeELFSymbolRow{
				SymIdx:    idx,
				EA:        sym.Value,
				Size:      sym.Size,
				Name:      sym.Name,
				SymType:   int(elf.ST_TYPE(sym.Info)),
				SymBind:   int(elf.ST_BIND(sym.Info)),
				SecIdx:    int(sym.Section),
				TableKind: kind,
			})
			idx++
		}
	}
	appendGroup("symtab", symtabSyms)
	appendGroup("dynsym", dynSyms)
	return out
}
