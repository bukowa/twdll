package bronze

import (
	"context"
	"database/sql"
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
	}
	return nil
}

func (ELF64Extractor) Extract(ctx context.Context, src *Source, db *sql.DB) error {
	if src.Format == "" {
		if err := prepareELF64(src); err != nil {
			return err
		}
	}
	ef := src.ELF

	secRows := make([]sectionRow, 0, len(ef.Sections))
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
		secRows = append(secRows, sectionRow{
			SecIdx:      idx,
			Name:        sec.Name,
			VA:          sec.Addr,
			VirtualSize: sec.Size,
			RawOffset:   sec.Offset,
			RawSize:     sec.FileSize,
			Flags:       flags,
		})
	}

	if err := insertSections(ctx, db, secRows); err != nil {
		return err
	}

	dynSyms, _ := ef.DynamicSymbols()
	imports, exports, err := parseELF64RelocsImportsExports(ctx, db, ef, dynSyms)
	if err != nil {
		return err
	}
	if err := insertImports(ctx, db, imports); err != nil {
		return err
	}
	if err := insertExports(ctx, db, exports); err != nil {
		return err
	}

	symtabSyms, _ := ef.Symbols()
	elfSyms := collectRawELFSymbols(symtabSyms, dynSyms)
	return insertELFSymbols(ctx, db, elfSyms)
}

func parseELF64RelocsImportsExports(
	ctx context.Context,
	db *sql.DB,
	ef *elf.File,
	dynSyms []elf.Symbol,
) ([]importRow, []exportRow, error) {
	relocBatch := newTxStmtBatcher(ctx, db, `INSERT OR IGNORE INTO relocs(va) VALUES (?)`)
	defer func() { _ = relocBatch.Close() }()

	var imports []importRow
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
				return nil, nil, fmt.Errorf("insert elf reloc 0x%x: %w", rOffset, err)
			}

			symIdx := uint32(rInfo >> 32)
			if symIdx > 0 && int(symIdx-1) < len(dynSyms) {
				sym := dynSyms[symIdx-1]
				if sym.Name != "" && sym.Section == elf.SHN_UNDEF && !seenIAT[rOffset] {
					seenIAT[rOffset] = true
					imports = append(imports, importRow{
						IATVA:   rOffset,
						DLLName: sym.Library,
						SymName: sym.Name,
						Ordinal: int(symIdx),
					})
				}
			}
		}
	}
	if err := relocBatch.Flush(); err != nil {
		return nil, nil, err
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
	return imports, exports, nil
}

func collectRawELFSymbols(symtabSyms, dynSyms []elf.Symbol) []elfSymbolRow {
	var out []elfSymbolRow
	idx := 1
	appendGroup := func(kind string, syms []elf.Symbol) {
		for _, sym := range syms {
			if sym.Name == "" && sym.Value == 0 {
				continue
			}
			out = append(out, elfSymbolRow{
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
