package analyze

import (
	"bytes"
	"debug/elf"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func buildSyntheticELF64(textSize, dwarfSize uint64) []byte {
	shstrtab := []byte("\x00.shstrtab\x00.text\x00.debug_info\x00")
	for len(shstrtab)%8 != 0 {
		shstrtab = append(shstrtab, 0)
	}
	shstrOff := uint64(64)
	shOff := shstrOff + uint64(len(shstrtab))

	sections := []elf.Section64{
		{}, // SHT_NULL
		{
			Name: 1, // ".shstrtab"
			Type: uint32(elf.SHT_STRTAB),
			Off:  shstrOff,
			Size: uint64(len(shstrtab)),
		},
		{
			Name: 11, // ".text"
			Type: uint32(elf.SHT_PROGBITS),
			Off:  shOff,
			Size: textSize,
		},
	}
	if dwarfSize > 0 {
		sections = append(sections, elf.Section64{
			Name: 17, // ".debug_info"
			Type: uint32(elf.SHT_PROGBITS),
			Off:  shOff,
			Size: dwarfSize,
		})
	}

	var hdr elf.Header64
	hdr.Ident[0] = 0x7f
	hdr.Ident[1] = 'E'
	hdr.Ident[2] = 'L'
	hdr.Ident[3] = 'F'
	hdr.Ident[4] = byte(elf.ELFCLASS64)
	hdr.Ident[5] = byte(elf.ELFDATA2LSB)
	hdr.Ident[6] = byte(elf.EV_CURRENT)
	hdr.Type = uint16(elf.ET_EXEC)
	hdr.Machine = uint16(elf.EM_X86_64)
	hdr.Version = uint32(elf.EV_CURRENT)
	hdr.Shoff = shOff
	hdr.Ehsize = 64
	hdr.Shentsize = 64
	hdr.Shnum = uint16(len(sections))
	hdr.Shstrndx = 1

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, &hdr)
	_, _ = buf.Write(shstrtab)
	for _, s := range sections {
		_ = binary.Write(&buf, binary.LittleEndian, &s)
	}
	return buf.Bytes()
}

func buildSyntheticPE32(virtualSize, rawSize uint32) []byte {
	var buf bytes.Buffer
	dos := make([]byte, 64)
	dos[0] = 'M'
	dos[1] = 'Z'
	binary.LittleEndian.PutUint32(dos[0x3c:], 64)
	_, _ = buf.Write(dos)

	// PE\0\0 signature
	_, _ = buf.Write([]byte{'P', 'E', 0, 0})

	coff := pe.FileHeader{
		Machine:          pe.IMAGE_FILE_MACHINE_I386,
		NumberOfSections: 1,
		Characteristics:  0x0102,
	}
	_ = binary.Write(&buf, binary.LittleEndian, &coff)

	var sec pe.SectionHeader32
	copy(sec.Name[:], ".text")
	sec.VirtualSize = virtualSize
	sec.VirtualAddress = 0x1000
	sec.SizeOfRawData = rawSize
	sec.PointerToRawData = 128
	_ = binary.Write(&buf, binary.LittleEndian, &sec)

	return buf.Bytes()
}

func TestInspectBinary_DetectsELFHeavyDWARFStrippedELFPEAndSidecarPDB(t *testing.T) {
	dir := t.TempDir()

	// 1. Heavy DWARF ELF64 (>= 100 MiB DWARF sections)
	heavyELFPath := filepath.Join(dir, "Attila_heavy")
	const textWant = 26 * 1024 * 1024
	const dwarfWant = 220 * 1024 * 1024
	if err := os.WriteFile(heavyELFPath, buildSyntheticELF64(textWant, dwarfWant), 0o644); err != nil {
		t.Fatal(err)
	}
	heavyTraits := InspectBinary(heavyELFPath, "")
	if heavyTraits.Format != "elf" || !heavyTraits.HasDWARF || !heavyTraits.IsHeavyDWARF {
		t.Fatalf("unexpected heavy ELF traits: %+v", heavyTraits)
	}
	if heavyTraits.TextBytes != textWant || heavyTraits.DWARFBytes != dwarfWant {
		t.Fatalf("unexpected ELF sizes: text=%d dwarf=%d", heavyTraits.TextBytes, heavyTraits.DWARFBytes)
	}

	// 2. Stripped ELF64 (0 DWARF bytes)
	strippedELFPath := filepath.Join(dir, "Attila_stripped")
	if err := os.WriteFile(strippedELFPath, buildSyntheticELF64(textWant, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	strippedTraits := InspectBinary(strippedELFPath, "")
	if strippedTraits.Format != "elf" || strippedTraits.HasDWARF || strippedTraits.IsHeavyDWARF || strippedTraits.DWARFBytes != 0 {
		t.Fatalf("unexpected stripped ELF traits: %+v", strippedTraits)
	}

	// 3. PE32 without sidecar PDB vs with sidecar PDB next to sourcePath or binaryPath
	peDir := filepath.Join(dir, "staged")
	srcDir := filepath.Join(dir, "steam")
	if err := os.MkdirAll(peDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stagedPE := filepath.Join(peDir, "empire.retail.dll")
	sourcePE := filepath.Join(srcDir, "empire.retail.dll")
	const peTextWant = 20 * 1024 * 1024
	if err := os.WriteFile(stagedPE, buildSyntheticPE32(peTextWant, peTextWant), 0o644); err != nil {
		t.Fatal(err)
	}

	peNoPDB := InspectBinary(stagedPE, sourcePE)
	if peNoPDB.Format != "pe" || peNoPDB.HasLocalPDB || peNoPDB.TextBytes != peTextWant {
		t.Fatalf("unexpected PE traits without PDB: %+v", peNoPDB)
	}

	// Empty 0-byte .pdb should not count as valid local PDB
	sourcePDB := filepath.Join(srcDir, "empire.retail.pdb")
	if err := os.WriteFile(sourcePDB, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := InspectBinary(stagedPE, sourcePE); got.HasLocalPDB {
		t.Fatalf("0-byte PDB should not set HasLocalPDB: %+v", got)
	}

	// Non-empty .pdb next to sourcePath sets HasLocalPDB = true
	if err := os.WriteFile(sourcePDB, []byte("Microsoft C/C++ MSF 7.00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := InspectBinary(stagedPE, sourcePE); !got.HasLocalPDB {
		t.Fatalf("expected HasLocalPDB=true with source sidecar PDB, got %+v", got)
	}

	// 4. Non-binary stub ("ELF64-OK") returns Format: "unknown" with FileSizeBytes populated
	stubPath := filepath.Join(dir, "stub.bin")
	if err := os.WriteFile(stubPath, []byte("ELF64-OK"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubTraits := InspectBinary(stubPath, "")
	if stubTraits.Format != "unknown" || stubTraits.FileSizeBytes != 8 {
		t.Fatalf("unexpected stub traits: %+v", stubTraits)
	}
}
